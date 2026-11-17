package core

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
	"io"
	"strings"
	"sync"
)

type Action struct {
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
	Intent    string          `json:"intent,omitempty"`
}
type actionProvider struct{}

func (actionProvider) ID() string { return "opera-admin" }
func (actionProvider) Models(context.Context) ([]ai.Model, error) {
	return []ai.Model{{ID: "tools", Provider: "opera-admin"}}, nil
}
func (actionProvider) Capabilities(string) providers.Capabilities { return providers.Capabilities{} }
func (actionProvider) Close() error                               { return nil }
func (actionProvider) CountTokens(_ context.Context, _ string, c ai.Context) (int64, error) {
	return providers.EstimateTokens(c), nil
}
func (actionProvider) Stream(_ context.Context, request providers.StreamRequest) (providers.EventStream, error) {
	for i := len(request.Context.Messages) - 1; i >= 0; i-- {
		message := request.Context.Messages[i]
		if message.ToolResult != nil {
			events := []ai.Event{}
			if message.ToolResult.IsError {
				events = append(events, ai.NewErrorEvent("management_tool_failed", errors.New("management operation failed; inspect its durable tool result")))
			} else {
				var b strings.Builder
				for _, block := range message.ToolResult.Content {
					b.WriteString(block.Text)
				}
				events = append(events, ai.TextDelta{Text: b.String()})
			}
			events = append(events, ai.StopEvent{Reason: ai.StopReasonStop})
			return &actionStream{events: events}, nil
		}
	}
	var action Action
	for _, message := range request.Context.Messages {
		if message.Role == ai.RoleUser {
			for _, block := range message.Content {
				if json.Unmarshal([]byte(block.Text), &action) == nil && action.Tool != "" {
					events := []ai.Event{ai.ToolCallEvent{ToolCall: ai.ToolCall{ID: "opera-action", Name: action.Tool, Arguments: action.Arguments}, Complete: true}, ai.StopEvent{Reason: ai.StopReasonStop}}
					return &actionStream{events: events}, nil
				}
			}
		}
	}
	return nil, errors.New("invalid management action")
}

type actionStream struct {
	mu     sync.Mutex
	closed bool
	events []ai.Event
	index  int
}

func (s *actionStream) Next(ctx context.Context) (ai.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, io.EOF
	}
	if s.index >= len(s.events) {
		return nil, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}
func (s *actionStream) Close() error { s.mu.Lock(); defer s.mu.Unlock(); s.closed = true; return nil }
