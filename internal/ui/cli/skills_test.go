package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsCLIListShowAndCheck(t *testing.T) {
	f := newFixture(t)
	t.Setenv("HOME", f.root)
	dir := filepath.Join(f.root, ".agents", "skills", "release-notes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: release-notes\ndescription: Prepare a release summary\n---\nGroup changes by user impact."), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := f.app.Run([]string{"skills", "list"}); code != 0 || !strings.Contains(f.out.String(), "$release-notes (workspace)") || !strings.Contains(f.out.String(), "$opera (builtin)") {
		t.Fatalf("skills list failed: code=%d out=%s err=%s", code, &f.out, &f.err)
	}
	f.out.Reset()
	if code := f.app.Run([]string{"skills", "show", "release-notes"}); code != 0 || !strings.Contains(f.out.String(), "Group changes by user impact.") {
		t.Fatalf("skills show failed: code=%d out=%s err=%s", code, &f.out, &f.err)
	}
	f.out.Reset()
	if code := f.app.Run([]string{"skills", "check"}); code != 0 || !strings.Contains(f.out.String(), "skill(s) valid") {
		t.Fatalf("skills check failed: code=%d out=%s err=%s", code, &f.out, &f.err)
	}
}

func TestSkillsCheckRejectsInvalidSkill(t *testing.T) {
	f := newFixture(t)
	t.Setenv("HOME", f.root)
	dir := filepath.Join(f.root, ".agents", "skills", "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("missing frontmatter"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := f.app.Run([]string{"skills", "check"}); code != 1 || !strings.Contains(f.err.String(), "invalid skill broken") {
		t.Fatalf("invalid skill was not reported: code=%d out=%s err=%s", code, &f.out, &f.err)
	}
}
