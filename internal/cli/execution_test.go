package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/diasYuri/opera/internal/app"
)

func TestHeadlessHTTPProviderApprovalAndResume(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("credential was not passed to provider")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			call := map[string]any{"id": "fixture", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "write-1", "type": "function", "function": map[string]string{"name": "write_file", "arguments": `{"path":"proof.txt","content":"durable"}`}}}}}}}
			raw, _ := json.Marshal(call)
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", raw)
		} else {
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"verified\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer server.Close()
	f := newFixture(t)
	environment := map[string]string{"OPERA_PROVIDER": "openai-compatible", "OPERA_MODEL": "fixture", "OPERA_POLICY": "ask", "OPERA_BASE_URL": server.URL, "OPENAI_COMPATIBLE_API_KEY": "fixture-key"}
	f.app.Env = func(key string) (string, bool) { value, ok := environment[key]; return value, ok }
	if code := f.app.Run([]string{"run", "create proof", "--format=jsonl"}); code != 3 {
		t.Fatalf("headless approval code %d: %s", code, &f.err)
	}
	if _, err := os.Stat(filepath.Join(f.root, "proof.txt")); !os.IsNotExist(err) {
		t.Fatal("headless tool executed without approval")
	}
	var first app.Result
	for _, line := range strings.Split(strings.TrimSpace(f.out.String()), "\n") {
		var event struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal("invalid JSONL", err)
		}
		if event.Type == "execution.result" {
			if err := json.Unmarshal(event.Data, &first); err != nil {
				t.Fatal(err)
			}
		}
	}
	if first.Approval == nil || first.Checkpoint == 0 {
		t.Fatal("missing durable approval/checkpoint")
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"resume", first.ID, "--approve", first.Approval.ID, "--format=jsonl"}); code != 0 {
		t.Fatalf("resume code %d: %s", code, &f.err)
	}
	content, err := os.ReadFile(filepath.Join(f.root, "proof.txt"))
	if err != nil || string(content) != "durable" {
		t.Fatal("approved effect missing", err)
	}
	if calls.Load() != 2 {
		t.Fatal("unexpected provider calls")
	}
	if strings.Contains(f.out.String()+f.err.String(), "fixture-key") {
		t.Fatal("credential leaked into output")
	}
}
