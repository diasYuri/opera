package core_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/agent"
)

// This test is opt-in, uses only a temporary workspace and reports no credential
// or provider response body. Default tests never contact a paid provider.
func TestLiveVerticalProof(t *testing.T) {
	if os.Getenv("OPERA_LIVE_TEST") != "1" {
		t.Skip("set OPERA_LIVE_TEST=1 with OPENAI_API_KEY to run the live proof")
	}
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Fatal("OPENAI_API_KEY is required")
	}
	cfg := configuration(t)
	model := os.Getenv("OPERA_LIVE_MODEL")
	if model == "" {
		model = "gpt-5-mini"
	}
	cfg.Values["model"] = config.Value{Value: model, Source: "live-test"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	options := core.Options{Credential: func(context.Context, string) (string, error) { return key, nil }}
	one, err := core.Open(ctx, cfg, options)
	if err != nil {
		t.Fatalf("live harness startup failed (%T)", err)
	}
	first, err := one.Service.Execute(ctx, execution.Request{Prompt: "This is an integration test in a disposable workspace. Your next response must contain exactly one write_file tool call creating live-proof.txt with the text opera-f1 and no newline. Do not reply with prose or use another tool first. After the write has been approved and its result returned, use read_file to verify that file and reply LIVE_PROOF_OK."}, nil)
	if err != nil || first.ExitCode != 3 || first.Approval == nil {
		closeRuntime(t, one)
		t.Fatalf("live approval proof failed: exit=%d error=%s", first.ExitCode, safeLiveError(err, key))
	}
	closeRuntime(t, one)
	two, err := core.Open(ctx, cfg, options)
	if err != nil {
		t.Fatalf("live reopen failed (%T)", err)
	}
	defer closeRuntime(t, two)
	second, err := two.Service.Execute(ctx, execution.Request{ID: first.ID, Approve: first.Approval.ID}, nil)
	if err != nil || second.ExitCode != 0 {
		t.Fatalf("live resume failed: exit=%d error=%s", second.ExitCode, safeLiveError(err, key))
	}
	data, err := os.ReadFile(filepath.Join(cfg.Paths.Workspace, "live-proof.txt"))
	if err != nil || string(data) != "opera-f1" {
		t.Fatal("live provider did not produce the approved effect")
	}
	state, err := two.Service.Store.Load(ctx, first.ID)
	if err != nil {
		t.Fatal("live checkpoint missing")
	}
	raw, err := json.Marshal(state)
	if err != nil || strings.Contains(string(raw), key) {
		t.Fatal("credential reached a durable checkpoint")
	}
	ops, err := two.Service.Store.Operations(ctx, first.ID)
	if err != nil {
		t.Fatal("live operation inspection failed")
	}
	verified := func(operations []agent.OperationRecord) bool {
		for _, op := range operations {
			if op.ToolName == "read_file" && op.State == agent.OperationCompleted && op.Response != nil && !op.Response.IsError {
				for _, block := range op.Response.Content {
					if strings.Contains(block.Text, "opera-f1") {
						return true
					}
				}
			}
		}
		return false
	}
	if !verified(ops) {
		follow, err := two.Service.Execute(ctx, execution.Request{ContinueFrom: first.ID, Prompt: "Explicit verification is still required. Call read_file on live-proof.txt now. Do not write files. Then reply LIVE_PROOF_OK with the observed content."}, nil)
		if err != nil || follow.ExitCode != 0 {
			t.Fatalf("live verification follow-up failed: %s", safeLiveError(err, key))
		}
		reads, err := two.Service.Store.Operations(ctx, follow.ID)
		if err != nil || !verified(reads) {
			t.Fatal("live read did not verify the completed write through durable operations")
		}
		ops = append(ops, reads...)
	}
	t.Logf("Live proof passed: provider=openai model=%s run=%s checkpoint=%d operations=%d", model, first.ID, second.Checkpoint, len(ops))
}

func safeLiveError(err error, key string) string {
	text := fmt.Sprint(err)
	text = strings.ReplaceAll(text, key, "[REDACTED]")
	return regexp.MustCompile(`(?i)(sk-|sess-|Bearer\s+)[a-z0-9_.-]+`).ReplaceAllString(text, "[REDACTED]")
}
