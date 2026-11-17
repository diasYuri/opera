package harness

import (
	"context"
	"testing"

	"github.com/diasYuri/y/pkg/tools"
)

func TestToolPackSelectionAllowsCustomPackAndRejectsInvalidSelection(t *testing.T) {
	registry := tools.NewRegistry()
	defer registry.Close(context.Background())
	var registered int
	custom := ToolPack{ID: "custom", Register: func(context.Context, *tools.Registry, ToolPackContext) error { registered++; return nil }}
	disabled := []string{"filesystem", "shell", "git", "code-search", "lsp", "skills", "work", "user-question"}
	if err := registerToolPacks(context.Background(), registry, ToolPackContext{}, []ToolPack{custom}, disabled); err != nil {
		t.Fatal(err)
	}
	if registered != 1 {
		t.Fatalf("custom pack registered %d times", registered)
	}
	if err := registerToolPacks(context.Background(), registry, ToolPackContext{}, nil, []string{"missing"}); err == nil {
		t.Fatal("unknown pack selection accepted")
	}
	if err := registerToolPacks(context.Background(), registry, ToolPackContext{}, []ToolPack{custom, custom}, disabled); err == nil {
		t.Fatal("duplicate pack ID accepted")
	}
}
