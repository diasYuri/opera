package core_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/y/pkg/agent/agenttest"
)

func TestUsageSurvivesReplayBranchAndCompaction(t *testing.T) {
	cfg := configuration(t)
	r := open(t, cfg, agenttest.TextTurn(strings.Repeat("context ", 1000)), agenttest.TextTurn("second answer"))
	defer closeRuntime(t, r)
	ctx := context.Background()
	first, err := r.Service.Execute(ctx, app.Request{Prompt: "one"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, err := r.Service.Usage(ctx, "run", first.ID, "")
	if err != nil || before.Totals.Total.Count != 1 {
		t.Fatal(before, err)
	}
	if _, err := r.Service.Execute(ctx, app.Request{ID: first.ID}, nil); err != nil {
		t.Fatal(err)
	}
	after, err := r.Service.Usage(ctx, "run", first.ID, "")
	if err != nil || after.Totals.Total.Count != before.Totals.Total.Count {
		t.Fatal("replay added a provider charge", after, err)
	}
	second, err := r.Service.Execute(ctx, app.Request{Prompt: "two", ContinueFrom: first.ID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := r.Service.Usage(ctx, "session", first.SessionID, "")
	if err != nil || session.Totals.Total.Count != 2 {
		t.Fatal(session, err)
	}
	branch, err := r.Service.Fork(ctx, second.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	branchUsage, err := r.Service.Usage(ctx, "run", branch, "")
	if err != nil || branchUsage.Totals.Total.Count != 0 {
		t.Fatal("branch copied usage", branchUsage, err)
	}
	if err := r.Service.Compact(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	all, err := r.Service.Usage(ctx, "session", first.SessionID, "")
	if err != nil {
		t.Fatal(err)
	}
	if all.Totals.Total.Count < 2 {
		t.Fatal("compaction erased usage")
	}
	for _, row := range all.Records {
		if row.RunID != first.ID && row.RunID != second.ID {
			t.Fatal("branch charged to source session")
		}
	}
	raw, err := r.Service.InspectRecords(ctx, first.ID, "events", app.InspectFilter{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil || len(rows) != 1 {
		t.Fatal(string(raw), err)
	}
}
