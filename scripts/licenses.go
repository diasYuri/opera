package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func licenseNotices() ([]byte, error) {
	if err := checkModules(); err != nil {
		return nil, err
	}
	if err := goCommand(nil, "mod", "download"); err != nil {
		return nil, err
	}
	modules := map[string]licenseModule{}
	for _, targetOS := range []string{"linux", "darwin", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			cmd := exec.Command("go", "list", "-deps", "-json", "-tags=feature_lsp,feature_wasm_ext", "./cmd/opera")
			cmd.Env = append(os.Environ(), "GOOS="+targetOS, "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
			raw, err := cmd.Output()
			if err != nil {
				return nil, err
			}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			for {
				var pkg struct{ Module *licenseModule }
				if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
					break
				} else if err != nil {
					return nil, err
				}
				if pkg.Module != nil && !pkg.Module.Main {
					modules[pkg.Module.Path] = *pkg.Module
				}
			}
		}
	}
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	var out strings.Builder
	out.WriteString("Opera third-party notices\nThese are dependency notices, not a grant to their trademarks.\n\n")
	standard, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return nil, err
	}
	license, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(standard)), "LICENSE"))
	if err != nil {
		return nil, err
	}
	out.WriteString("Go standard library and runtime (" + toolchain + ")\n" + string(license) + "\n\n")
	for _, name := range names {
		module := modules[name]
		if module.Main {
			continue
		}
		if module.Replace != nil {
			return nil, errors.New("license inventory forbids replaced modules")
		}
		entries, err := os.ReadDir(module.Dir)
		if err != nil {
			return nil, err
		}
		names := []string{}
		for _, entry := range entries {
			upper := strings.ToUpper(entry.Name())
			if !entry.IsDir() && (strings.HasPrefix(upper, "LICENSE") || strings.HasPrefix(upper, "LICENCE") || strings.HasPrefix(upper, "COPYING") || upper == "NOTICE" || upper == "NOTICE.TXT" || upper == "PATENTS") {
				names = append(names, entry.Name())
			}
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("no root license/notice found for %s %s; resolve before packaging", module.Path, module.Version)
		}
		sort.Strings(names)
		out.WriteString("============================================================\n" + module.Path + " " + module.Version + "\nChecksum: " + module.Sum + "\n")
		for _, name := range names {
			data, err := os.ReadFile(filepath.Join(module.Dir, name))
			if err != nil {
				return nil, err
			}
			out.WriteString("\n--- " + name + " ---\n" + string(data) + "\n")
		}
	}
	return []byte(out.String()), nil
}

type licenseModule struct {
	Path, Version, Dir, Sum string
	Main                    bool
	Replace                 any
}
