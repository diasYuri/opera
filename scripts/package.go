package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

func packageVersion() (string, error) {
	version := os.Getenv("OPERA_VERSION")
	if version == "" {
		version = "0.1.0-local"
	}
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$`).MatchString(version) {
		return "", errors.New("OPERA_VERSION must be a safe semantic version")
	}
	return version, nil
}

type artifactMetadata struct{ Version, GOOS, GOARCH, Go, SDK, Source, CGO string }

func packages() error {
	version, err := packageVersion()
	if err != nil {
		return err
	}
	notices, err := licenseNotices()
	if err != nil {
		return err
	}
	license, err := os.ReadFile("LICENSE")
	if err != nil {
		return err
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		return err
	}
	sums := []string{}
	for _, targetOS := range []string{"linux", "darwin", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := "opera_" + version + "_" + targetOS + "_" + arch
			binary := "opera"
			if targetOS == "windows" {
				binary += ".exe"
			}
			output := filepath.Join("dist", name, binary)
			if err := goCommand(map[string]string{"GOOS": targetOS, "GOARCH": arch}, "build", "-trimpath", "-buildvcs=false", "-ldflags=-X main.version="+version, "-o", output, "./cmd/opera"); err != nil {
				return err
			}
			raw, err := os.ReadFile(output)
			if err != nil {
				return err
			}
			metadata, err := json.MarshalIndent(artifactMetadata{Version: version, GOOS: targetOS, GOARCH: arch, Go: toolchain, SDK: "v1.4.0", Source: "local source; unpublished", CGO: "0"}, "", "  ")
			if err != nil {
				return err
			}
			files := map[string][]byte{binary: raw, "LICENSE": license, "THIRD_PARTY_NOTICES.txt": notices, "README.md": readme, "version.json": append(metadata, '\n')}
			// Include the English operational documentation and executable examples.
			for _, dir := range []string{"docs", "examples"} {
				if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if entry.IsDir() {
						return nil
					}
					if entry.Name() == "y-public-api.json" {
						return nil
					}
					raw, err := os.ReadFile(path)
					if err == nil {
						files[filepath.ToSlash(path)] = raw
					}
					return err
				}); err != nil {
					return err
				}
			}
			var archive []byte
			extension := ".tar.gz"
			if targetOS == "windows" {
				archive, err = zipFiles(name, files)
				extension = ".zip"
			} else {
				archive, err = tarFiles(name, files)
			}
			if err != nil {
				return err
			}
			fileName := name + extension
			if err := os.WriteFile(filepath.Join("dist", fileName), archive, 0644); err != nil {
				return err
			}
			digest := sha256.Sum256(archive)
			sums = append(sums, fmt.Sprintf("%x  %s", digest, fileName))
		}
	}
	sort.Strings(sums)
	return os.WriteFile("dist/SHA256SUMS", []byte(strings.Join(sums, "\n")+"\n"), 0644)
}
func sortedFiles(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func archiveMode(name string) int64 {
	if name == "opera" || name == "opera.exe" || strings.HasSuffix(name, ".sh") {
		return 0755
	}
	return 0644
}
func tarFiles(prefix string, files map[string][]byte) ([]byte, error) {
	var output bytes.Buffer
	gz := gzip.NewWriter(&output)
	tw := tar.NewWriter(gz)
	for _, name := range sortedFiles(files) {
		data := files[name]
		if err := tw.WriteHeader(&tar.Header{Name: prefix + "/" + name, Mode: archiveMode(name), Size: int64(len(data)), ModTime: time.Unix(0, 0), Typeflag: tar.TypeReg}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
func zipFiles(prefix string, files map[string][]byte) ([]byte, error) {
	var output bytes.Buffer
	zw := zip.NewWriter(&output)
	for _, name := range sortedFiles(files) {
		h := &zip.FileHeader{Name: prefix + "/" + name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
		h.SetMode(fs.FileMode(archiveMode(name)))
		writer, err := zw.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(files[name]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
func packageSmoke() error {
	version, err := packageVersion()
	if err != nil {
		return err
	}
	prefix := "opera_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH
	extension := ".tar.gz"
	if runtime.GOOS == "windows" {
		extension = ".zip"
	}
	raw, err := os.ReadFile(filepath.Join("dist", prefix+extension))
	if err != nil {
		return err
	}
	sums, err := os.ReadFile("dist/SHA256SUMS")
	if err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	if !strings.Contains(string(sums), hex.EncodeToString(digest[:])+"  "+prefix+extension) {
		return errors.New("package checksum mismatch")
	}
	root, err := os.MkdirTemp("", "opera-package-smoke-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(root) }()
	write := func(name string, reader io.Reader) error {
		if !fs.ValidPath(name) {
			return errors.New("invalid archive path")
		}
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0755)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(file, reader)
		return errors.Join(copyErr, file.Close())
	}
	if runtime.GOOS == "windows" {
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			return err
		}
		for _, f := range zr.File {
			reader, err := f.Open()
			if err != nil {
				return err
			}
			writeErr := write(f.Name, reader)
			closeErr := reader.Close()
			if err := errors.Join(writeErr, closeErr); err != nil {
				return err
			}
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return err
		}
		defer func() { _ = gz.Close() }()
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			if h.Typeflag != tar.TypeReg {
				return errors.New("unexpected archive entry")
			}
			if err := write(h.Name, tr); err != nil {
				return err
			}
		}
	}
	dir := filepath.Join(root, prefix)
	binary := filepath.Join(dir, executable("opera"))
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		return err
	}
	cgo, sdk := false, false
	for _, setting := range info.Settings {
		if setting.Key == "CGO_ENABLED" && setting.Value == "0" {
			cgo = true
		}
	}
	for _, dep := range info.Deps {
		if dep.Replace != nil {
			return errors.New("package embeds dependency replacement")
		}
		if dep.Path == "github.com/diasYuri/y" && dep.Version == "v1.4.0" {
			sdk = true
		}
	}
	if !cgo || !sdk {
		return errors.New("package does not use pinned public SDK and CGO=0")
	}
	for _, name := range []string{"LICENSE", "THIRD_PARTY_NOTICES.txt", "version.json"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || len(data) == 0 {
			return errors.New("missing package notice/metadata")
		}
	}
	for _, args := range [][]string{{"--version"}, {"--help"}, {"doctor", "--format=jsonl"}, {"demo", "chat", "--format=jsonl"}, {"demo", "run", "--format=jsonl"}, {"demo", "worker", "--format=jsonl"}, {"demo", "headless", "--format=jsonl"}} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "OPERA_CONFIG_DIR="+filepath.Join(root, "config"), "OPERA_DATA_DIR="+filepath.Join(root, "data"), "OPERA_CACHE_DIR="+filepath.Join(root, "cache"))
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("packaged %s: %w: %s", strings.Join(args, " "), err, output)
		}
		if args[0] == "--version" && strings.TrimSpace(string(output)) != "opera "+version {
			return errors.New("packaged version mismatch")
		}
		if args[0] == "--help" && !bytes.Contains(output, []byte("Usage: opera")) {
			return errors.New("packaged help missing")
		}
		if args[0] == "doctor" || args[0] == "demo" {
			for _, line := range bytes.Split(bytes.TrimSpace(output), []byte{'\n'}) {
				if !json.Valid(line) {
					return errors.New("packaged command emitted invalid JSONL")
				}
			}
		}
		if args[0] == "demo" && !bytes.Contains(output, []byte(`"verified":true`)) {
			return errors.New("packaged demo missing acceptance evidence")
		}
		fmt.Println("Packaged smoke passed:", strings.Join(args, " "))
	}
	return nil
}
