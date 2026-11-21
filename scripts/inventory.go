package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type apiSymbol struct {
	Package string `json:"package"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Build   string `json:"build,omitempty"`
}

func inventory() error {
	data, err := exec.Command("go", "list", "-m", "-json", "github.com/diasYuri/y").Output()
	if err != nil {
		return err
	}
	var module struct{ Path, Version, Dir, Sum string }
	if err := json.Unmarshal(data, &module); err != nil {
		return err
	}
	if module.Version != "v1.4.0" || module.Dir == "" {
		return fmt.Errorf("inventory requires the pinned published Y v1.4.0")
	}
	symbols := []apiSymbol{}
	err = filepath.WalkDir(filepath.Join(module.Dir, "pkg"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == "internal" {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(module.Dir, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		fileset := token.NewFileSet()
		file, err := parser.ParseFile(fileset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		build := ""
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if strings.HasPrefix(comment.Text, "//go:build ") {
					build = strings.TrimPrefix(comment.Text, "//go:build ")
				}
			}
		}
		add := func(name, kind string, position token.Pos) {
			symbols = append(symbols, apiSymbol{
				Package: module.Path + "/" + filepath.ToSlash(filepath.Dir(relative)),
				File:    relative, Line: fileset.Position(position).Line, Name: name, Kind: kind, Build: build,
			})
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if ast.IsExported(d.Name.Name) {
					name := d.Name.Name
					if d.Recv != nil {
						name = receiverName(d.Recv.List[0].Type) + "." + name
						// Methods on private receivers are not consumer APIs.
						if !ast.IsExported(strings.SplitN(name, ".", 2)[0]) {
							continue
						}
					}
					add(name, "func", d.Pos())
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(s.Name.Name) {
							add(s.Name.Name, "type", s.Pos())
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if ast.IsExported(name.Name) {
								add(name.Name, d.Tok.String(), name.Pos())
							}
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].File == symbols[j].File {
			return symbols[i].Line < symbols[j].Line
		}
		return symbols[i].File < symbols[j].File
	})
	result := struct {
		Module   string      `json:"module"`
		Version  string      `json:"version"`
		Checksum string      `json:"checksum"`
		Symbols  []apiSymbol `json:"symbols"`
	}{module.Path, module.Version, module.Sum, symbols}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll("docs", 0755); err != nil {
		return err
	}
	if err := os.WriteFile("docs/y-public-api.json", append(encoded, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("Indexed %d public declarations from %s %s\n", len(symbols), module.Path, module.Version)
	return nil
}

func receiverName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return receiverName(value.X)
	case *ast.IndexExpr:
		return receiverName(value.X)
	case *ast.IndexListExpr:
		return receiverName(value.X)
	default:
		return ""
	}
}
