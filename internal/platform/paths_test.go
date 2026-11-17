package platform_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/diasYuri/opera/internal/platform"
)

func TestPlatformPaths(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	for _, goos := range []string{"linux", "darwin", "windows"} {
		paths, err := platform.Resolve(workspace, home, goos, func(string) string { return "" })
		if err != nil {
			t.Fatal(err)
		}
		if !filepath.IsAbs(paths.Data) || paths.ProjectConfig != filepath.Join(paths.Workspace, ".opera", "config.toml") {
			t.Fatalf("invalid %s paths: %#v", goos, paths)
		}
		bases := map[string][3]string{
			"linux":   {filepath.Join(home, ".config", "opera"), filepath.Join(home, ".local", "share", "opera"), filepath.Join(home, ".cache", "opera")},
			"darwin":  {filepath.Join(home, "Library", "Application Support", "opera"), filepath.Join(home, "Library", "Application Support", "opera"), filepath.Join(home, "Library", "Caches", "opera")},
			"windows": {filepath.Join(home, "AppData", "Roaming", "opera"), filepath.Join(home, "AppData", "Local", "opera"), filepath.Join(home, "AppData", "Local", "Cache", "opera")},
		}
		want := bases[goos]
		if paths.UserConfig != filepath.Join(want[0], "config.toml") || paths.Data != want[1] || paths.Cache != want[2] {
			t.Fatalf("incorrect %s platform convention: %#v", goos, paths)
		}
		if _, err := os.Stat(paths.Data); !os.IsNotExist(err) {
			t.Fatalf("path discovery created data directory: %v", err)
		}
	}
}

func TestOverridesAndCanonicalWorkspace(t *testing.T) {
	root := t.TempDir()
	values := map[string]string{"OPERA_CONFIG_DIR": filepath.Join(root, "config"), "OPERA_DATA_DIR": filepath.Join(root, "data"), "OPERA_CACHE_DIR": filepath.Join(root, "cache")}
	paths, err := platform.Resolve(root, root, runtime.GOOS, func(k string) string { return values[k] })
	if err != nil || paths.UserConfig != filepath.Join(values["OPERA_CONFIG_DIR"], "config.toml") {
		t.Fatalf("override: %#v %v", paths, err)
	}
	values["OPERA_DATA_DIR"] = "relative"
	if _, err := platform.Resolve(root, root, runtime.GOOS, func(k string) string { return values[k] }); err == nil {
		t.Fatal("accepted relative application directory")
	}
	if runtime.GOOS == "windows" {
		return // Symlink creation can require privileges; native ACL tests run separately.
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	paths, err = platform.Resolve(link, root, runtime.GOOS, func(string) string { return "" })
	want, canonicalErr := filepath.EvalSymlinks(root)
	if err != nil || canonicalErr != nil || paths.Workspace != want {
		t.Fatalf("workspace not canonical: %#v %v %v", paths, err, canonicalErr)
	}
}
