package accounting_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/diasYuri/opera/internal/accounting"
	"github.com/diasYuri/opera/internal/persistence"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/providertest"
)

func TestCumulativeSamplesAreOneDurableRequestWithPriceProvenance(t *testing.T) {
	ctx := context.Background()
	store, err := persistence.Open(ctx, filepath.Join(t.TempDir(), "db"), "workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	rate := 2.0
	base := providertest.NewFakeProvider(providertest.WithFakeResponses(providertest.FakeResponse{Events: []ai.Event{ai.UsageEvent{Usage: ai.Usage{InputTokens: 12, OutputTokens: 3, TotalTokens: 15}}, ai.UsageEvent{Usage: ai.Usage{InputTokens: 12, OutputTokens: 3, TotalTokens: 15}}, ai.StopEvent{Reason: ai.StopReasonStop}}}))
	tracked := &accounting.Provider{Provider: base, Store: store, Identity: func(context.Context, string) (accounting.Identity, error) {
		return accounting.Identity{SessionID: "session"}, nil
	}, Prices: []accounting.Price{{Provider: base.ID(), Model: "test", Source: "fixture price table", Date: "2026-11-16", Input: &rate, Output: &rate}}}
	stream, err := tracked.Stream(ctx, providers.StreamRequest{Model: ai.Model{ID: "test"}, Options: providers.StreamOptions{RunID: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, err := stream.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	docs, err := store.List(ctx, "usage")
	if err != nil || len(docs) != 1 {
		t.Fatal(err, len(docs))
	}
	var row accounting.Record
	if err := json.Unmarshal(docs[0].Data, &row); err != nil {
		t.Fatal(err)
	}
	report := accounting.Summarize([]accounting.Record{row, row})
	if report.Totals.Total.Count != 1 || report.Totals.Total.Usage.TotalTokens != 15 || row.CostStatus != "estimated" || row.PriceSource != "fixture price table" || row.CostUSD == nil || *row.CostUSD != 0.00003 || row.State != "completed" {
		t.Fatal(report, row)
	}
}
func TestMissingUsageIsEstimatedAndCostUnavailable(t *testing.T) {
	ctx := context.Background()
	store, err := persistence.Open(ctx, filepath.Join(t.TempDir(), "db"), "workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	p := &accounting.Provider{Provider: providertest.NewFakeProvider(providertest.WithFakeResponses(providertest.FakeResponse{Events: []ai.Event{ai.TextDelta{Text: "estimate"}}})), Store: store, Identity: func(context.Context, string) (accounting.Identity, error) { return accounting.Identity{}, nil }}
	stream, err := p.Stream(ctx, providers.StreamRequest{Options: providers.StreamOptions{RunID: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, err := stream.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = stream.Close()
	rows, err := store.List(ctx, "usage")
	if err != nil {
		t.Fatal(err)
	}
	var row accounting.Record
	if err := json.Unmarshal(rows[0].Data, &row); err != nil {
		t.Fatal(err)
	}
	if row.Tokens != "estimated" || row.CostUSD != nil || row.CostStatus != "unavailable" || row.EstimatedOutput != 2 {
		t.Fatal(row)
	}
}
