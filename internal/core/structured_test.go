package core_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/agent/agenttest"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
)

func TestStructuredOutputCapabilitySchemaAndInvalidResponse(t *testing.T) {
	format := &ai.ResponseFormat{Type: ai.ResponseFormatJSONSchema, Name: "answer", Strict: true, Schema: json.RawMessage(`{"type":"object","properties":{"answer":{"type":"integer"}},"required":["answer"],"additionalProperties":false}`)}
	for _, tc := range []struct {
		name, text    string
		supported, ok bool
	}{{"valid", `{"answer":42}`, true, true}, {"tool then JSON", `{"answer":42}`, true, true}, {"wrong type", `{"answer":"no"}`, true, false}, {"invalid JSON", `oops`, true, false}, {"unsupported", `{"answer":42}`, false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configuration(t)
			turns := []agenttest.ScriptedTurn{}
			if tc.name == "tool then JSON" {
				turns = append(turns, agenttest.ToolTurn("list_directory", `{"path":"."}`))
			}
			if tc.supported {
				turns = append(turns, agenttest.TextTurn(tc.text))
			}
			p := agenttest.NewScriptProvider(t, turns...)
			capable := capabilityProvider{Provider: namedProvider{p}, supported: tc.supported}
			r, err := core.Open(context.Background(), cfg, core.Options{ResponseFormat: format, ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) { return capable, nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer closeRuntime(t, r)
			result, err := r.Service.Execute(context.Background(), execution.Request{Prompt: "answer in JSON"}, nil)
			if tc.ok {
				if err != nil || !json.Valid(result.StructuredOutput) {
					t.Fatal(result, err)
				}
			} else if err == nil || result.ExitCode == 0 {
				t.Fatal("invalid contract was accepted", result)
			}
			if !tc.supported && len(p.Requests()) != 0 {
				t.Fatal("unsupported request reached provider")
			}
		})
	}

}

type capabilityProvider struct {
	providers.Provider
	supported bool
}

func (p capabilityProvider) Capabilities(string) providers.Capabilities {
	return providers.Capabilities{Streaming: true, StructuredOutput: p.supported}
}
