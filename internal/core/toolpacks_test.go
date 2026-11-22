package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/diasYuri/y/pkg/policy"
	"github.com/diasYuri/y/pkg/search"
	"github.com/diasYuri/y/pkg/tools"
)

type fakeSearchProvider struct{ calls int }

func (*fakeSearchProvider) Capabilities() search.Capabilities { return search.Capabilities{} }
func (p *fakeSearchProvider) Search(_ context.Context, request search.Request) (search.Response, error) {
	p.calls++
	return search.Response{Results: []search.Result{{Title: "Source", URL: "https://example.com/source", Snippet: request.Query, Metadata: map[string]string{"provider": "test"}}}}, nil
}

func TestToolPackSelectionAllowsCustomPackAndRejectsInvalidSelection(t *testing.T) {
	registry := tools.NewRegistry()
	t.Cleanup(func() {
		if err := registry.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	var registered int
	custom := ToolPack{ID: "custom", Register: func(context.Context, *tools.Registry, ToolPackContext) error { registered++; return nil }}
	disabled := []string{"filesystem", "shell", "git", "code-search", "lsp", "skills", "work", "user-question", "web-search"}
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

func TestWebSearchToolPackRegistersAuditedCitationTool(t *testing.T) {
	registry := tools.NewRegistry()
	t.Cleanup(func() {
		if err := registry.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	provider := &fakeSearchProvider{}
	disabled := []string{"filesystem", "shell", "git", "code-search", "lsp", "skills", "work", "user-question"}
	if err := registerToolPacks(context.Background(), registry, ToolPackContext{Search: provider}, nil, disabled); err != nil {
		t.Fatal(err)
	}
	descriptors := registry.List()
	if len(descriptors) != 1 || descriptors[0].Name != tools.WebSearchToolName || !descriptors[0].Sensitive || len(descriptors[0].Capabilities) != 1 || descriptors[0].Capabilities[0] != tools.CapabilityWebSearch {
		t.Fatalf("registered descriptors = %#v", descriptors)
	}
	response, err := registry.Handle(context.Background(), tools.ToolRequest{Name: tools.WebSearchToolName, Arguments: json.RawMessage(`{"query":"current product information"}`), Approval: &policy.ApprovalResolution{State: policy.ApprovalApproved}})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || len(response.Content) != 1 || response.Content[0].Text == "" || !strings.Contains(response.Content[0].Text, "https://example.com/source") {
		t.Fatalf("search call/response = %d, %#v", provider.calls, response)
	}
}

func registerToolPacks(ctx context.Context, registry *tools.Registry, inputs ToolPackContext, custom []ToolPack, disabled []string) error {
	selection, err := selectToolPacks(custom, disabled)
	if err != nil {
		return err
	}
	return installToolPacks(ctx, registry, inputs, selection)
}
