// Package accounting records actual provider requests separately from UI event replay.
package accounting

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"github.com/diasYuri/opera/internal/persistence"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/telemetry"
)

type Identity struct{ SessionID, ParentID string }
type Price struct {
	Provider   string   `json:"provider"`
	Model      string   `json:"model"`
	Source     string   `json:"source"`
	Date       string   `json:"date"`
	Input      *float64 `json:"input_usd_per_million"`
	Output     *float64 `json:"output_usd_per_million"`
	CacheRead  *float64 `json:"cache_read_usd_per_million"`
	CacheWrite *float64 `json:"cache_write_usd_per_million"`
}

func LoadPrices(path string) ([]Price, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("price table exceeds 1 MiB")
	}
	var prices []Price
	if err := json.Unmarshal(raw, &prices); err != nil {
		return nil, errors.New("invalid price table")
	}
	seen := map[string]bool{}
	for _, p := range prices {
		_, err := time.Parse("2006-01-02", p.Date)
		key := p.Provider + "\x00" + p.Model
		if err != nil || p.Source == "" || p.Provider == "" || p.Model == "" || seen[key] {
			return nil, errors.New("prices require unique provider/model, source and YYYY-MM-DD date")
		}
		seen[key] = true
		for _, rate := range []*float64{p.Input, p.Output, p.CacheRead, p.CacheWrite} {
			if rate != nil && (*rate < 0 || math.IsNaN(*rate) || math.IsInf(*rate, 0)) {
				return nil, errors.New("invalid price rate")
			}
		}
	}
	return prices, nil
}

type Record struct {
	ID              string        `json:"id"`
	RunID           string        `json:"run_id"`
	ParentID        string        `json:"parent_id,omitempty"`
	SessionID       string        `json:"session_id"`
	TurnID          string        `json:"turn_id"`
	Provider        string        `json:"provider"`
	Model           string        `json:"model"`
	Kind            string        `json:"kind"`
	Started         time.Time     `json:"started"`
	Finished        *time.Time    `json:"finished,omitempty"`
	State           string        `json:"state"`
	Tokens          string        `json:"tokens_status"`
	Usage           ai.Usage      `json:"usage"`
	EstimatedInput  int64         `json:"estimated_input_tokens"`
	EstimatedOutput int64         `json:"estimated_output_tokens"`
	CostStatus      string        `json:"cost_status"`
	CostUSD         *float64      `json:"cost_usd"`
	PriceSource     string        `json:"price_source,omitempty"`
	PriceDate       string        `json:"price_date,omitempty"`
	Latency         time.Duration `json:"latency_ns"`
}
type Provider struct {
	providers.Provider
	Store    *persistence.Store
	Identity func(context.Context, string) (Identity, error)
	Prices   []Price
}

func (p *Provider) Stream(ctx context.Context, req providers.StreamRequest) (providers.EventStream, error) {
	identity, err := p.Identity(ctx, req.Options.RunID)
	if err != nil {
		return nil, err
	}
	row := Record{ID: rand.Text(), RunID: req.Options.RunID, TurnID: req.Options.TurnID, SessionID: identity.SessionID, ParentID: identity.ParentID, Provider: p.ID(), Model: req.Model.ID, Kind: "inference", Started: time.Now().UTC(), State: "started", Tokens: "unavailable", CostStatus: "unavailable", EstimatedInput: providers.EstimateTokens(req.Context)}
	providerTurn := req.Options.TurnID
	row.TurnID = row.ID
	req.Options.RequestID = "opera-provider-" + row.ID
	req.Options.IdempotencyKey = req.Options.RequestID
	req.Options.TurnID = row.TurnID
	if providerTurn == "compaction" {
		row.Kind = "compaction"
	}
	if err := p.Store.Put(ctx, "usage", row.ID, row); err != nil {
		return nil, err
	}
	stream, err := p.Provider.Stream(ctx, req)
	s := &streamRecord{base: stream, owner: p, row: row}
	if err != nil {
		if saveErr := s.finish("failed"); saveErr != nil {
			return nil, errors.Join(err, saveErr)
		}
		return nil, err
	}
	return s, nil
}

type streamRecord struct {
	base        providers.EventStream
	owner       *Provider
	row         Record
	mu          sync.Mutex
	closed      bool
	reported    bool
	outputBytes int64
}

func (s *streamRecord) Next(ctx context.Context) (ai.Event, error) {
	event, err := s.base.Next(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return event, err
	}
	switch value := event.(type) {
	case ai.UsageEvent:
		// SDK adapters emit cumulative counters for a request. Repeated samples update
		// one row, never become additive ledger entries.
		s.reported = true
		u := value.Usage
		s.row.Usage.InputTokens = max(s.row.Usage.InputTokens, u.InputTokens)
		s.row.Usage.OutputTokens = max(s.row.Usage.OutputTokens, u.OutputTokens)
		s.row.Usage.CacheReadTokens = max(s.row.Usage.CacheReadTokens, u.CacheReadTokens)
		s.row.Usage.CacheWriteTokens = max(s.row.Usage.CacheWriteTokens, u.CacheWriteTokens)
		s.row.Usage.ReasoningTokens = max(s.row.Usage.ReasoningTokens, u.ReasoningTokens)
		s.row.Usage.TotalTokens = max(s.row.Usage.TotalTokens, u.TotalTokens)
		if u.Cost.Total > 0 {
			s.row.Usage.Cost = u.Cost
		}
		s.row.Tokens = "known"
		s.price()
		if saveErr := s.save(); saveErr != nil {
			return nil, saveErr
		}
	case ai.TextDelta:
		s.outputBytes += int64(len([]rune(value.Text)))
	case ai.ThinkingDelta:
		s.outputBytes += int64(len([]rune(value.Thinking)))
	case ai.ErrorEvent:
		s.row.State = "failed"
	}
	if err != nil {
		state := "failed"
		if errors.Is(err, io.EOF) {
			state = "completed"
		}
		if s.row.State == "failed" {
			state = "failed"
		}
		if saveErr := s.finishLocked(state); saveErr != nil {
			return nil, errors.Join(err, saveErr)
		}
	}
	return event, err
}
func (s *streamRecord) price() {
	if s.row.Usage.Cost.Total > 0 {
		v := s.row.Usage.Cost.Total
		s.row.CostUSD = &v
		s.row.CostStatus = "known"
		s.row.PriceSource = "normalized provider usage"
		s.row.PriceDate = s.row.Started.Format("2006-01-02")
		return
	}
	for _, p := range s.owner.Prices {
		if p.Provider != s.row.Provider || p.Model != s.row.Model {
			continue
		}
		total := float64(0)
		for _, part := range []struct {
			n    int64
			rate *float64
		}{{s.row.Usage.InputTokens, p.Input}, {s.row.Usage.OutputTokens, p.Output}, {s.row.Usage.CacheReadTokens, p.CacheRead}, {s.row.Usage.CacheWriteTokens, p.CacheWrite}} {
			if part.n > 0 && part.rate == nil {
				return
			}
			if part.rate != nil {
				total += float64(part.n) * *part.rate / 1e6
			}
		}
		s.row.CostUSD = &total
		s.row.CostStatus = "estimated"
		s.row.PriceSource = p.Source
		s.row.PriceDate = p.Date
		return
	}
}
func (s *streamRecord) save() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.owner.Store.Put(ctx, "usage", s.row.ID, s.row)
}
func (s *streamRecord) finish(state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finishLocked(state)
}
func (s *streamRecord) finishLocked(state string) error {
	if s.closed {
		return nil
	}
	s.closed = true
	now := time.Now().UTC()
	s.row.Finished = &now
	s.row.Latency = now.Sub(s.row.Started)
	if s.row.State == "failed" {
		state = "failed"
	}
	s.row.State = state
	if !s.reported {
		s.row.Tokens = "estimated"
		s.row.EstimatedOutput = (s.outputBytes + 3) / 4
	}
	return s.save()
}
func (s *streamRecord) Close() error {
	err := s.base.Close()
	state := "interrupted"
	return errors.Join(err, s.finish(state))
}

type Summary struct {
	Totals             telemetry.Snapshot `json:"totals"`
	KnownCostsUSD      float64            `json:"known_costs_usd"`
	EstimatedCostsUSD  float64            `json:"estimated_costs_usd"`
	UnavailableCosts   int                `json:"unavailable_costs"`
	EstimatedRequests  int                `json:"estimated_requests"`
	IncompleteRequests int                `json:"incomplete_requests"`
	Records            []Record           `json:"records"`
}

func Summarize(rows []Record) Summary {
	collector := telemetry.NewAccounting()
	out := Summary{Records: []Record{}}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		out.Records = append(out.Records, r)
		collector.Record(telemetry.Measurement{Dimensions: telemetry.Dimensions{RunID: r.RunID, SessionID: r.SessionID, TurnID: r.RunID + ":" + r.TurnID, ProviderID: r.Provider, ModelID: r.Model}, Usage: r.Usage, Cost: r.Usage.Cost.Total, EstimatedContextTokens: r.EstimatedInput, Latency: r.Latency})
		if r.Tokens == "estimated" {
			out.EstimatedRequests++
		}
		if r.Finished == nil {
			out.IncompleteRequests++
		}
		switch r.CostStatus {
		case "known":
			if r.CostUSD != nil {
				out.KnownCostsUSD += *r.CostUSD
			}
		case "estimated":
			if r.CostUSD != nil {
				out.EstimatedCostsUSD += *r.CostUSD
			}
		default:
			out.UnavailableCosts++
		}
	}
	out.Totals = collector.Snapshot()
	return out
}
