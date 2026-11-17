package harness_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/y/pkg/agent/agenttest"
	"github.com/diasYuri/y/pkg/tools"
)

func TestLargeToolOutputIsInspectableAfterVisualTruncation(t *testing.T) {
	cfg := configuration(t)
	cfg.Values["limits.output_bytes"] = config.Value{Value: 1024}
	r := open(t, cfg, agenttest.ToolTurn("large_output", `{}`), agenttest.TextTurn("done"))
	defer closeRuntime(t, r)
	text := strings.Repeat("diagnostic ", 1000)
	err := r.Extensions.Registry.Add(tools.ToolDescriptor{Name: "large_output", InputSchema: json.RawMessage(`{"type":"object"}`)}, tools.ToolHandlerFunc(func(context.Context, tools.ToolRequest) (tools.ToolResponse, error) {
		return tools.ToolResponse{Content: []tools.ContentBlock{{Type: tools.ContentText, Text: text}}}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// Policy belongs to the registry, so authorize this unknown external tool normally.
	result, err := r.Service.Execute(ctx, app.Request{Prompt: "inspect a large diagnostic"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Approval != nil {
		result, err = r.Service.Execute(ctx, app.Request{ID: result.ID, Approve: result.Approval.ID}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	view, err := r.Service.Inspect(ctx, result.ID)
	if err != nil || len(view.ArchivedOutputs) != 1 {
		t.Fatal("diagnostic archive missing", err, len(view.ArchivedOutputs))
	}
	if !strings.Contains(string(view.ArchivedOutputs[0].Data), text) {
		t.Fatal("full diagnostic was lost")
	}
	if len(view.Decisions) == 0 {
		t.Fatal("policy decisions were not persisted")
	}
	decisions, err := r.Service.InspectRecords(ctx, result.ID, "decisions", app.InspectFilter{Tool: "large_output", Limit: 10})
	if err != nil || !strings.Contains(string(decisions), "decision") {
		t.Fatal("decision filter missing", err)
	}
	if len(view.Operations) == 0 || len(view.Operations[0].Response.Content[0].Text) >= len(text) {
		t.Fatal("tool preview was not bounded")
	}
	raw, err := r.Service.InspectRecords(ctx, result.ID, "operations", app.InspectFilter{Tool: "large_output", Limit: 1})
	if err != nil || !json.Valid(raw) {
		t.Fatal(err)
	}
}
