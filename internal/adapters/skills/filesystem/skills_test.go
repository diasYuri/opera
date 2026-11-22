package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diasYuri/y/pkg/tools"
)

func writeSkill(t *testing.T, root, directory, frontmatter, body string) string {
	t.Helper()
	dir := filepath.Join(root, directory)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\n" + frontmatter + "\n---\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func validSkill(name, description string) string {
	return "name: " + name + "\ndescription: " + description
}

func TestDiscoverParsesMetadataAndWorkspaceWins(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	localRoot := filepath.Join(workspace, ".agents", "skills")
	globalRoot := filepath.Join(home, ".agents", "skills")
	writeSkill(t, localRoot, "review", "name: review\ndescription: Review project changes\nlicense: MIT\ncompatibility: Requires git\nmetadata:\n  author: team\nallowed-tools: \"read_file search\"", "Local instructions")
	writeSkill(t, globalRoot, "review", validSkill("review", "Global copy"), "Global instructions")
	writeSkill(t, globalRoot, "writing", validSkill("writing", "Write clear docs"), "Writing instructions")

	catalog := Discover(workspace, home)
	if len(catalog.Skills) != 3 || len(catalog.Issues) != 0 {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
	review, ok := catalog.Find("review")
	if !ok || review.Source != "workspace" || review.Body != "Local instructions" || review.License != "MIT" || review.Compatibility != "Requires git" || review.Metadata["author"] != "team" {
		t.Fatalf("workspace skill metadata not loaded: %+v", review)
	}
	if strings.Join(review.AllowedTools, ",") != "read_file,search" {
		t.Fatalf("allowed-tools not parsed: %v", review.AllowedTools)
	}
}

func TestBuiltinSkillIsReservedAndWorksWithoutLocalFiles(t *testing.T) {
	workspace, home := t.TempDir(), t.TempDir()
	fresh := Discover(workspace, "")
	if len(fresh.Skills) != 1 || fresh.Skills[0].Source != "builtin" || fresh.LegacyDigest == nil || *fresh.LegacyDigest != "" {
		t.Fatalf("unexpected empty workspace catalog: %+v", fresh)
	}
	writeSkill(t, filepath.Join(workspace, ".agents", "skills"), "opera", "invalid", "")
	writeSkill(t, filepath.Join(home, ".agents", "skills"), "opera", validSkill("opera", "User copy"), "User instructions")
	manager := NewManager(workspace, home)
	catalog := manager.PinRun("run")
	if catalog.Digest != fresh.Digest || len(catalog.Issues) != 0 {
		t.Fatalf("disk copies affected the built-in catalog: %+v", catalog)
	}
	if instructions := manager.InstructionsForNames("run", manager.ExplicitNames("run", "Use $opera")); !strings.Contains(instructions, "references/develop.md") {
		t.Fatalf("built-in skill was not activated: %s", instructions)
	}
	for _, path := range []string{"references/operate.md", "references/develop.md", "references/extend.md"} {
		if data, err := manager.ReadResource("opera", path); err != nil || len(data) == 0 {
			t.Fatalf("built-in resource %s: %v", path, err)
		}
	}
	for _, path := range []string{"../SKILL.md", "/SKILL.md", "references/../SKILL.md", "references/missing.md"} {
		if _, err := manager.ReadResource("opera", path); err == nil {
			t.Fatalf("accepted invalid built-in resource %q", path)
		}
	}
}

func TestInvalidWorkspaceSkillShadowsGlobalCopy(t *testing.T) {
	workspace, home := t.TempDir(), t.TempDir()
	writeSkill(t, filepath.Join(workspace, ".agents", "skills"), "broken", "name: broken", "bad")
	writeSkill(t, filepath.Join(home, ".agents", "skills"), "broken", validSkill("broken", "valid global"), "global")

	catalog := Discover(workspace, home)
	if _, ok := catalog.Find("broken"); ok {
		t.Fatal("invalid workspace skill fell back to the global copy")
	}
	if len(catalog.Issues) != 1 || catalog.Issues[0].Source != "workspace" {
		t.Fatalf("missing workspace validation issue: %+v", catalog.Issues)
	}
}

func TestRejectsSymlinkAndResourceTraversal(t *testing.T) {
	workspace, home := t.TempDir(), t.TempDir()
	localRoot := filepath.Join(workspace, ".agents", "skills")
	outside := writeSkill(t, t.TempDir(), "outside", validSkill("outside", "outside"), "instructions")
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(localRoot, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	dir := writeSkill(t, localRoot, "safe", validSkill("safe", "safe"), "read a resource")
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "guide.md"), []byte("guide"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := Discover(workspace, home)
	if len(catalog.Issues) != 1 || catalog.Issues[0].Name != "linked" {
		t.Fatalf("symlink skill was not reported: %+v", catalog.Issues)
	}
	manager := NewManager(workspace, home)
	if data, err := manager.ReadResource("safe", "references/guide.md"); err != nil || string(data) != "guide" {
		t.Fatalf("resource read failed: %q, %v", data, err)
	}
	for _, path := range []string{"../outside/SKILL.md", filepath.Join(dir, "SKILL.md"), "references/../SKILL.md"} {
		if _, err := manager.ReadResource("safe", path); err == nil {
			t.Fatalf("accepted unsafe resource path %q", path)
		}
	}
}

func TestToolsExposeCatalogLoadAndResourcesForPinnedRun(t *testing.T) {
	workspace, home := t.TempDir(), t.TempDir()
	dir := writeSkill(t, filepath.Join(workspace, ".agents", "skills"), "sample", validSkill("sample", "Sample skill"), "follow the sample steps")
	if err := os.WriteFile(filepath.Join(dir, "REFERENCE.md"), []byte("supporting details"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(workspace, home)
	manager.PinRun("run-1")
	defer manager.ReleaseRun("run-1")
	registry := tools.NewRegistry()
	defer func() { _ = registry.Close(context.Background()) }()
	if err := manager.Register(registry); err != nil {
		t.Fatal(err)
	}
	call := func(name, args string) (tools.ToolResponse, error) {
		return registry.Handle(context.Background(), tools.ToolRequest{Name: name, Arguments: json.RawMessage(args), RunID: "run-1", WorkspaceRoot: workspace})
	}
	loaded, err := call("skill_load", `{"name":"sample"}`)
	if err != nil || !strings.Contains(loaded.Content[0].Text, "follow the sample steps") {
		t.Fatalf("skill_load failed: %+v, %v", loaded, err)
	}
	resource, err := call("skill_resource", `{"name":"sample","path":"REFERENCE.md"}`)
	if err != nil || resource.Content[0].Text != "supporting details" {
		t.Fatalf("skill_resource failed: %+v, %v", resource, err)
	}
	page, err := call("skill_catalog", `{}`)
	if err != nil || !strings.Contains(page.Content[0].Text, "Sample skill") {
		t.Fatalf("skill_catalog failed: %+v, %v", page, err)
	}
}

func TestExplicitSkillNamesAndPinnedPackageChanges(t *testing.T) {
	workspace, home := t.TempDir(), t.TempDir()
	dir := writeSkill(t, filepath.Join(workspace, ".agents", "skills"), "release-notes", validSkill("release-notes", "Write release notes"), "group changes by impact")
	if err := os.WriteFile(filepath.Join(dir, "REFERENCE.md"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(workspace, home)
	manager.PinRun("run-2")
	names := manager.ExplicitNames("run-2", "Use $release-notes, then $release-notes.")
	if len(names) != 1 || names[0] != "release-notes" {
		t.Fatalf("explicit skill parsing failed: %v", names)
	}
	if instructions := manager.InstructionsForNames("run-2", names); !strings.Contains(instructions, "group changes by impact") {
		t.Fatalf("explicit skill instructions missing: %s", instructions)
	}
	if err := os.WriteFile(filepath.Join(dir, "REFERENCE.md"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ReadResource("release-notes", "REFERENCE.md"); err == nil {
		t.Fatal("resource from a changed pinned skill package was returned")
	}
	manager.ReleaseRun("run-2")
}
