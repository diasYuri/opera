package execution

import (
	"context"
	"strings"
	"testing"

	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/tools"
)

func TestGuardIntersectsRuntimeModeAndProfileToolConstraints(t *testing.T) {
	mode, err := ResolveRuntimeMode(RuntimeModeDefault)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := ResolveAgentProfile(AgentProfileReview)
	if err != nil {
		t.Fatal(err)
	}
	guard := &Guard{}
	guard.SetMode("run-1", mode)
	guard.SetProfile("run-1", profile)
	called := false
	_, err = guard.Middleware(context.Background(), tools.ToolRequest{RunID: "run-1", Name: "write_file"}, tools.ToolDescriptor{Name: "write_file"}, func(context.Context, tools.ToolRequest) (tools.ToolResponse, error) {
		called = true
		return tools.ToolResponse{}, nil
	})
	if err == nil || called || !strings.Contains(strings.ToLower(err.Error()), "agent profile reviewer") {
		t.Fatalf("reviewer mutation result: called=%v err=%v", called, err)
	}
	_, err = guard.Middleware(context.Background(), tools.ToolRequest{RunID: "run-1", Name: "read_file"}, tools.ToolDescriptor{Name: "read_file"}, func(context.Context, tools.ToolRequest) (tools.ToolResponse, error) {
		called = true
		return tools.ToolResponse{}, nil
	})
	if err != nil || !called {
		t.Fatalf("reviewer read result: called=%v err=%v", called, err)
	}
}

func TestGuardFiltersAdvertisedToolsByBothModeAndProfile(t *testing.T) {
	mode, _ := ResolveRuntimeMode(RuntimeModeDefault)
	profile, _ := ResolveAgentProfile(AgentProfileReview)
	guard := &Guard{}
	guard.SetMode("run-1", mode)
	guard.SetProfile("run-1", profile)
	filtered := guard.FilterTools("run-1", []ai.Tool{{Name: "read_file"}, {Name: "write_file"}, {Name: "subagent_spawn"}})
	if len(filtered) != 1 || filtered[0].Name != "read_file" {
		t.Fatalf("reviewer advertised tools = %#v", filtered)
	}
}
