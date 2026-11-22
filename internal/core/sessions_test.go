package core_test

import (
	"context"
	"encoding/json"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/agent/agenttest"
	"strings"
	"testing"
)

const layeredSummary = "## L1 — Current objective\nInspect the code.\n\n## L2 — Work and evidence\nReviewed evidence.\n\n## L3 — Outcomes\nInspection reported."

func TestSessionCatalogBranchCompactionAndExport(t *testing.T) {
	cfg := configuration(t)
	turns := []agenttest.ScriptedTurn{}
	for i := 0; i < 7; i++ {
		turns = append(turns, agenttest.TextTurn(strings.Repeat("Evidence for the coding task. ", 50)))
	}
	turns = append(turns, agenttest.TextTurn(layeredSummary), agenttest.TextTurn("new branch task complete"))
	r := open(t, cfg, turns...)
	defer closeRuntime(t, r)
	ctx := context.Background()
	previous := ""
	for i := 0; i < 7; i++ {
		prompt := "continue"
		if i == 0 {
			prompt = "API_KEY=private-export-value\nPlease inspect the code"
		}
		result, err := r.Service.Execute(ctx, execution.Request{Prompt: prompt, ContinueFrom: previous}, nil)
		if err != nil || result.ExitCode != 0 {
			t.Fatalf("%+v %v", result, err)
		}
		previous = result.ID
	}
	sessions, err := r.Service.Sessions(ctx, 0, 3)
	if err != nil || len(sessions) != 1 {
		t.Fatal(sessions, err)
	}
	before, err := r.Service.Store.Load(ctx, previous)
	if err != nil {
		t.Fatal(err)
	}
	branch, err := r.Service.Fork(ctx, previous, before.Version)
	if err != nil {
		t.Fatal(err)
	}
	branched, err := r.Service.Store.Load(ctx, branch)
	if err != nil || branched.Agent.PendingApproval != nil || branched.Agent.RunID == previous || branched.Agent.RunNonce != "" {
		t.Fatal("branch inherited execution grants", err)
	}
	operations, err := r.Service.Store.Operations(ctx, branch)
	if err != nil || len(operations) != 0 {
		t.Fatal("branch inherited operations", err)
	}
	if err := r.Service.Compact(ctx, previous); err != nil {
		t.Fatal(err)
	}
	after, err := r.Service.Store.Load(ctx, previous)
	if err != nil || len(after.Agent.Transcript) >= len(before.Agent.Transcript) {
		t.Fatal("transcript was not compacted", err)
	}
	archive, err := r.Service.Records.List(ctx, "compactions")
	if err != nil || len(archive) != 1 {
		t.Fatal("compaction archive missing", err)
	}
	raw, err := r.Service.Export(ctx, previous)
	if err != nil || !json.Valid(raw) || strings.Contains(string(raw), "private-export-value") {
		t.Fatal("export leaked credentials or is invalid", err)
	}
	historical, err := r.Service.Fork(ctx, previous, before.Version)
	if err != nil {
		t.Fatal("historical checkpoint branch", err)
	}
	historicalState, err := r.Service.Store.Load(ctx, historical)
	if err != nil || len(historicalState.Agent.Transcript) != len(before.Agent.Transcript) {
		t.Fatal("historical boundary lost transcript", err)
	}
	resumed, err := r.Service.Execute(ctx, execution.Request{ID: branch, Prompt: "new branch task"}, nil)
	if err != nil || resumed.ExitCode != 0 {
		t.Fatal("branch resume failed", err)
	}
	meta, err := r.Service.Metadata(ctx, branch)
	if err != nil || meta.Origin != previous || meta.OriginVersion != before.Version {
		t.Fatal("branch provenance missing", err)
	}
}
