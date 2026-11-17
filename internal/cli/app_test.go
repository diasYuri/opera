package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/diasYuri/opera/internal/cli"
	"github.com/diasYuri/opera/internal/credentials"
	"github.com/diasYuri/opera/internal/output"
	"github.com/diasYuri/opera/internal/platform"
)

type fixture struct {
	app      cli.App
	out, err bytes.Buffer
	root     string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{root: t.TempDir()}
	f.app = cli.App{
		In: strings.NewReader(""), Out: &f.out, Err: &f.err, Version: "test-version",
		Getwd: func() (string, error) { return f.root, nil },
		Env:   func(string) (string, bool) { return "", false },
		Paths: func(workspace string) (platform.Paths, error) {
			return platform.Resolve(workspace, f.root, runtime.GOOS, func(key string) string {
				return map[string]string{
					"OPERA_CONFIG_DIR": filepath.Join(f.root, "config"),
					"OPERA_DATA_DIR":   filepath.Join(f.root, "data"),
					"OPERA_CACHE_DIR":  filepath.Join(f.root, "cache"),
				}[key]
			})
		},
		LookPath:  func(string) (string, error) { return "", errors.New("not installed") },
		OpenStore: credentials.Open,
	}
	return f
}

func TestHelpVersionAndSchemaNeedNoWorkspaceOrSecrets(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--version"}, {"config", "schema"}} {
		f := newFixture(t)
		f.app.Paths = func(string) (platform.Paths, error) { t.Fatal("read workspace"); return platform.Paths{}, nil }
		f.app.Getwd = func() (string, error) { t.Fatal("read working directory"); return "", nil }
		f.app.OpenStore = func(string, string) (credentials.Store, error) { t.Fatal("read secrets"); return nil, nil }
		if code := f.app.Run(args); code != 0 || f.out.Len() == 0 || f.err.Len() != 0 {
			t.Fatalf("command %v: code %d stderr %s", args, code, &f.err)
		}
	}
}

func TestDoctorHeadlessWithMissingDependencies(t *testing.T) {
	f := newFixture(t)
	f.app.In = failingReader{}
	f.app.OpenStore = func(string, string) (credentials.Store, error) { t.Fatal("doctor read keyring"); return nil, nil }
	if code := f.app.Run([]string{"doctor", "--format", "jsonl"}); code != 0 {
		t.Fatalf("doctor code %d: %s", code, &f.err)
	}
	var event output.Event
	if err := json.Unmarshal(f.out.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "doctor.report" || event.SchemaVersion != 1 || !strings.Contains(f.out.String(), "fallback") {
		t.Fatalf("missing diagnostic contract: %s", &f.out)
	}
	if _, err := os.Stat(filepath.Join(f.root, "data")); !os.IsNotExist(err) {
		t.Fatal("doctor created persistent state")
	}
}

func TestConfigFlagsAndMalformedProject(t *testing.T) {
	f := newFixture(t)
	if code := f.app.Run([]string{"config", "get", "model", "--model=selected"}); code != 0 || f.out.String() != "selected (flag)\n" {
		t.Fatalf("config: code %d stdout %s stderr %s", code, &f.out, &f.err)
	}
	f.out.Reset()
	if err := os.Mkdir(filepath.Join(f.root, ".opera"), 0700); err != nil {
		t.Fatal(err)
	}
	const secret = "never-print-this-token"
	if err := os.WriteFile(filepath.Join(f.root, ".opera", "config.toml"), []byte("api_key='"+secret+"'"), 0600); err != nil {
		t.Fatal(err)
	}
	code := f.app.Run([]string{"config", "--format=jsonl"})
	if code != cli.ExitUsage || strings.Contains(f.out.String()+f.err.String(), secret) || !json.Valid(f.err.Bytes()) {
		t.Fatalf("invalid config: code %d stdout %s stderr %s", code, &f.out, &f.err)
	}
	f.err.Reset()
	if code := f.app.Run([]string{"--help"}); code != 0 {
		t.Fatal("invalid config prevented help")
	}
}

func TestAuthNeverPrintsSecretsAndPersists(t *testing.T) {
	f := newFixture(t)
	const secret = "sentinel-secret-value"
	f.app.In = strings.NewReader(secret + "\n")
	for _, action := range []string{"set", "status", "delete", "status"} {
		f.out.Reset()
		f.err.Reset()
		code := f.app.Run([]string{"auth", action, "default", "--store", "file", "--format", "jsonl"})
		if code != 0 || !json.Valid(f.out.Bytes()) || strings.Contains(f.out.String()+f.err.String(), secret) {
			t.Fatalf("auth %s: code %d stdout %s stderr %s", action, code, &f.out, &f.err)
		}
	}
	if !strings.Contains(f.out.String(), "missing") {
		t.Fatal("deletion was not persistent")
	}
}

func TestAuthDoesNotFallBackOrReadTerminal(t *testing.T) {
	f := newFixture(t)
	f.app.IsTTY = true
	f.app.In = failingReader{}
	if code := f.app.Run([]string{"auth", "set", "default", "--store=file"}); code != cli.ExitUsage {
		t.Fatalf("TTY: code %d stderr %s", code, &f.err)
	}
	f = newFixture(t)
	f.app.OpenStore = func(kind, _ string) (credentials.Store, error) {
		if kind != "keyring" {
			t.Fatal("silently selected fallback")
		}
		return nil, errors.New("keyring unavailable")
	}
	if code := f.app.Run([]string{"auth", "status", "default"}); code != cli.ExitFailure {
		t.Fatalf("keyring failure: code %d", code)
	}
}

func TestAuthStripsOnlyOneConventionalLineEnding(t *testing.T) {
	for _, pair := range [][2]string{{"value\n", "value"}, {"value\r\n", "value"}, {" value \r", " value \r"}, {"value\n\n", "value\n"}} {
		f := newFixture(t)
		f.app.In = strings.NewReader(pair[0])
		if code := f.app.Run([]string{"auth", "set", "default", "--store=file"}); code != cli.ExitOK {
			t.Fatalf("auth input normalization failed: %s", &f.err)
		}
		store := credentials.FileStore{Directory: filepath.Join(f.root, "data", "credentials")}
		if value, err := store.Get("default"); err != nil || value != pair[1] {
			t.Fatalf("secret normalization changed meaningful input: %v", err)
		}
	}
}

func TestJSONLErrorsAndExplicitFalse(t *testing.T) {
	f := newFixture(t)
	if code := f.app.Run([]string{"--format=jsonl", "--unknown"}); code != cli.ExitUsage || !json.Valid(f.err.Bytes()) {
		t.Fatal("parse error broke explicit JSONL output")
	}
	f = newFixture(t)
	if code := f.app.Run([]string{"config", "get", "ui.no_motion", "--no-motion=false"}); code != cli.ExitOK || f.out.String() != "false (flag)\n" {
		t.Fatal("explicit false was lost")
	}
	for _, args := range [][]string{{"--workspace="}, {"--user-config="}, {"--help=false"}, {"--version=false"}} {
		f = newFixture(t)
		if code := f.app.Run(args); code != cli.ExitUsage {
			t.Errorf("invalid flag accepted: %v", args)
		}
	}
}

func TestInvalidCommandsAndOutputFailure(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"--model"}, {"--set", "invalid"}, {"run", "prompt"}, {"doctor", "extra"}, {"config", "get", "unknown"}, {"auth", "get", "default"}, {"config", "--user-config", "not-a-real-file"}} {
		f := newFixture(t)
		if code := f.app.Run(args); code != cli.ExitUsage {
			t.Errorf("%v: code %d", args, code)
		}
	}
	f := newFixture(t)
	f.app.Out = failingWriter{}
	if code := f.app.Run([]string{"doctor", "--format=jsonl"}); code != cli.ExitFailure {
		t.Fatal("ignored broken output")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
