package chatgpt

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/tools"
)

// Exercise the real Y agent/tool loop through the subscription transport,
// including replay of namespaced tool calls after local tool execution.
func TestAgentExecutesSubscriptionToolAndReplaysHistory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "evidence.txt"), []byte("local evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	if err := tools.RegisterFilesystem(registry, tools.FilesystemOptions{WorkspaceRoot: root}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = registry.Close(context.Background()) }()
	p := NewProvider(func(context.Context, string) (string, error) { return "subscription-token", nil })
	calls := 0
	p.http.Transport = routeTransport{roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer subscription-token" {
			t.Error("credential not passed")
		}
		var body struct {
			Input []map[string]json.RawMessage `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if calls == 1 {
			return fixtureResponse("data: " + `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"item-1","call_id":"call-1","name":"read_file","namespace":"opera"}}` + "\n\ndata: " + `{"type":"response.function_call_arguments.done","item_id":"item-1","name":"read_file","arguments":"{\"path\":\"evidence.txt\"}"}` + "\n\ndata: " + `{"type":"response.completed","response":{"id":"response-1","status":"completed"}}` + "\n\n"), nil
		}
		if calls > 2 {
			t.Error("unexpected extra inference call")
		}
		var seenCall, seenResult bool
		for _, item := range body.Input {
			if string(item["type"]) == `"function_call"` {
				seenCall = true
				if string(item["namespace"]) != `"opera"` {
					t.Error("function call lost namespace")
				}
			}
			if string(item["type"]) == `"function_call_output"` {
				seenResult = true
				if string(item["namespace"]) != `"opera"` || !strings.Contains(string(item["output"]), "local evidence") {
					t.Error("tool result lost namespace or content")
				}
			}
		}
		if !seenCall || !seenResult {
			t.Error("tool history missing")
		}
		return fixtureResponse("data: " + `{"type":"response.output_text.delta","delta":"Evidence read"}` + "\n\ndata: " + `{"type":"response.completed","response":{"id":"response-2","status":"completed"}}` + "\n\n"), nil
	})}
	runner := agent.NewAgentRunner(p, registry)
	defer func() { _ = runner.Close(context.Background()) }()
	response, err := runner.Run(context.Background(), agent.RunRequest{RunID: "subscription-tool-proof", WorkspaceRoot: root, Model: ai.Model{ID: "fixture", Provider: ProviderID}, Prompt: "Read evidence.txt"})
	if err != nil || calls != 2 || response.Snapshot.Status != agent.RunCompleted {
		t.Fatalf("tool loop failed: calls=%d status=%s err=%v", calls, response.Snapshot.Status, err)
	}
}
