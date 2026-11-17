package harness

import (
	"context"
	"fmt"

	"github.com/diasYuri/y/pkg/tools"
)

func Example_customToolPack() {
	registry := tools.NewRegistry()
	defer func() { _ = registry.Close(context.Background()) }()
	disabled := []string{"filesystem", "shell", "git", "code-search", "lsp", "skills", "work", "user-question"}
	pack := ToolPack{ID: "greeting", Register: func(_ context.Context, registry *tools.Registry, _ ToolPackContext) error {
		return registry.Add(tools.ToolDescriptor{Name: "greet", Description: "Return a greeting", InputSchema: []byte(`{"type":"object","properties":{},"additionalProperties":false}`)}, tools.ToolHandlerFunc(func(context.Context, tools.ToolRequest) (tools.ToolResponse, error) {
			return tools.ToolResponse{Content: []tools.ContentBlock{{Type: tools.ContentText, Text: "hello from a replaceable pack"}}}, nil
		}))
	}}
	if err := registerToolPacks(context.Background(), registry, ToolPackContext{}, []ToolPack{pack}, disabled); err != nil {
		panic(err)
	}
	fmt.Println(registry.List()[0].Name)
	// Output: greet
}
