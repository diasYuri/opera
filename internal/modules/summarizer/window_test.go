package summarizer

import (
	"strings"
	"testing"

	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
)

func TestWindowIncludesInstructionsToolsAndImages(t *testing.T) {
	request := providers.StreamRequest{Options: providers.StreamOptions{MaxTokens: 4096}, Context: ai.Context{
		SystemPrompt: strings.Repeat("a", 4000),
		Messages:     []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{{Type: ai.ContentImage}}}},
		Tools:        []ai.Tool{{Name: "read", Description: strings.Repeat("b", 4000)}},
	}}
	window := Measure(request, ai.Model{ContextWindow: 5000, MaxTokens: 2000}, 8000)
	if window.InputTokens != 3025 || window.ContextTokens != 5000 || window.InputLimit != 3750 || window.ThresholdTokens != 3000 || !window.ShouldCompact() {
		t.Fatalf("wrong model window measurement: %+v", window)
	}
	window = Measure(request, ai.Model{}, 8000)
	if window.ContextTokens != 8000 || window.InputLimit != 8000 || window.ThresholdTokens != 4800 || window.ShouldCompact() {
		t.Fatalf("wrong fallback window measurement: %+v", window)
	}
	window.InputTokens = window.ThresholdTokens
	if !window.ShouldCompact() {
		t.Fatal("exactly 60% did not trigger compaction")
	}
}
