package core_test

import (
	"context"
	"testing"

	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/agent/agenttest"
	"github.com/diasYuri/y/pkg/providers"
)

func TestApprovalHandoffRepairsOtherUnresolvedToolResults(t *testing.T) {
	cfg := configuration(t)
	p := namedProvider{agenttest.NewScriptProvider(t, agenttest.ScriptedTurn{ToolCalls: []agenttest.ScriptedToolCall{{Name: "write_file", Arguments: `{"path":"approved.txt","content":"ok"}`}, {Name: "write_file", Arguments: `{"path":"deferred.txt","content":"later"}`}}}, agenttest.TextTurn("done"))}
	r, err := core.Open(context.Background(), cfg, core.Options{ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return p, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRuntime(t, r)
	ctx := context.Background()
	first, err := r.Service.Execute(ctx, execution.Request{Prompt: "two separate writes"}, nil)
	if err != nil || first.Approval == nil {
		t.Fatal(first, err)
	}
	result, err := r.Service.Execute(ctx, execution.Request{ID: first.ID, Approve: first.Approval.ID}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatal(result, err)
	}
	requests := p.Requests()
	last := requests[len(requests)-1]
	calls, results := map[string]bool{}, map[string]bool{}
	for _, m := range last.Context.Messages {
		for _, call := range m.ToolCalls {
			calls[call.ID] = true
		}
		if m.ToolResult != nil {
			results[m.ToolResult.ToolCallID] = true
		}
	}
	for id := range calls {
		if !results[id] {
			t.Fatal("provider received an orphaned tool call", id)
		}
	}
}
