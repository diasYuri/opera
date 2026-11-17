package app

import (
	"context"
	"encoding/json"
	"github.com/diasYuri/y/pkg/tools"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestOutputBudgetPreservesCompletedEffectAndBoundsEscapedUnicode(t *testing.T) {
	calls := 0
	for _, text := range []string{strings.Repeat("a", 5000), strings.Repeat("界", 5000), strings.Repeat("\x00", 5000)} {
		response, err := OutputLimit(1024)(context.Background(), tools.ToolRequest{}, tools.ToolDescriptor{}, func(context.Context, tools.ToolRequest) (tools.ToolResponse, error) {
			calls++
			return tools.ToolResponse{Content: []tools.ContentBlock{{Type: tools.ContentText, Text: text}}}, nil
		})
		if err != nil || response.IsError {
			t.Fatal("completed effect became a failure", err)
		}
		raw, err := json.Marshal(response)
		if err != nil || len(raw) > 1024 {
			t.Fatal("output exceeded budget", len(raw), err)
		}
		if !utf8.ValidString(response.Content[0].Text) {
			t.Fatal("split Unicode")
		}
	}
	if calls != 3 {
		t.Fatal("effect repeated", calls)
	}
}
