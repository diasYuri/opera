package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/policy"
)

func TestApprovalFingerprintUsesInjectedObservation(t *testing.T) {
	pending := &agent.PendingApproval{Request: policy.ApprovalRequest{ApprovalID: "approval"}, ToolCall: ai.ToolCall{Name: "write_file", Arguments: json.RawMessage(`{"path":"file.txt","content":"new"}`)}}
	content := "original"
	reader := func(ctx context.Context, workspace, path string) (string, []byte, error) {
		if workspace != "workspace" || path != "file.txt" {
			t.Fatalf("unexpected observation: %s %s", workspace, path)
		}
		return path, []byte(content), ctx.Err()
	}
	first, err := approvalFor(context.Background(), "workspace", "review", pending, reader)
	if err != nil {
		t.Fatal(err)
	}
	content = "changed after review"
	second, err := approvalFor(context.Background(), "workspace", "review", pending, reader)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.Fingerprint == second.Fingerprint {
		t.Fatal("file change did not invalidate approval")
	}
	if _, err := approvalFor(context.Background(), "workspace", "review", pending, nil); err == nil {
		t.Fatal("missing observation capability allowed approval")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := approvalFor(canceled, "workspace", "review", pending, reader); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
