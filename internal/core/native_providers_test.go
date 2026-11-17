package core_test

import (
	"context"
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
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/core"
)

func TestNativeProviderFixturesUseOperaToolsAndAccounting(t *testing.T) {
	for _, provider := range []string{"anthropic", "google"} {
		t.Run(provider, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if provider == "anthropic" && r.Header.Get("X-Api-Key") != "fixture-key" {
					t.Error("credential injection missing")
				}
				if provider == "google" && r.Header.Get("X-Goog-Api-Key") != "fixture-key" {
					t.Error("credential injection missing")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				first := calls.Add(1) == 1
				if provider == "anthropic" {
					emit := func(event string, value any) {
						raw, _ := json.Marshal(value)
						_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
					}
					if first {
						emit("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": "native-read", "name": "read_file", "input": map[string]any{}}})
						emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": `{"path":"sample.txt"}`}})
						emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
					} else {
						emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "native proof"}})
					}
					reason := "end_turn"
					if first {
						reason = "tool_use"
					}
					emit("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": reason}, "usage": map[string]int{"input_tokens": 10, "output_tokens": 2}})
					emit("message_stop", map[string]any{"type": "message_stop"})
				} else {
					part := map[string]any{"text": "native proof"}
					if first {
						part = map[string]any{"functionCall": map[string]any{"id": "native-read", "name": "read_file", "args": map[string]any{"path": "sample.txt"}}}
					}
					raw, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"role": "model", "parts": []any{part}}, "finishReason": "STOP"}}, "usageMetadata": map[string]int{"promptTokenCount": 10, "candidatesTokenCount": 2, "totalTokenCount": 12}})
					_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
				}
			}))
			defer server.Close()
			cfg := configuration(t)
			cfg.Values["provider"] = config.Value{Value: provider}
			cfg.Values["model"] = config.Value{Value: map[string]string{"anthropic": "claude-test", "google": "gemini-test"}[provider]}
			cfg.Values["base_url"] = config.Value{Value: server.URL}
			if err := os.WriteFile(filepath.Join(cfg.Paths.Workspace, "sample.txt"), []byte("native input"), 0600); err != nil {
				t.Fatal(err)
			}
			r, err := core.Open(context.Background(), cfg, core.Options{Credential: func(context.Context, string) (string, error) { return "fixture-key", nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer closeRuntime(t, r)
			result, err := r.Service.Execute(context.Background(), app.Request{Prompt: "read the input"}, nil)
			if err != nil || !strings.Contains(result.Text, "native proof") {
				t.Fatal(result, err)
			}
			usage, err := r.Service.Usage(context.Background(), "run", result.ID, "")
			if err != nil || usage.Totals.Total.Count != 2 || usage.EstimatedRequests != 0 {
				t.Fatal(usage, err)
			}
			view, err := r.Service.Inspect(context.Background(), result.ID)
			if err != nil || len(view.Operations) != 1 || view.Operations[0].ToolName != "read_file" {
				t.Fatal(view.Operations, err)
			}
		})
	}
}
