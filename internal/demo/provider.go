// Package demo supplies an explicit offline acceptance scenario in disposable workspaces.
package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
)

type Provider struct {
	Command string
	Args    []string
	worker  bool
}

func (Provider) ID() string { return "openai" }
func (Provider) Models(context.Context) ([]ai.Model, error) {
	return []ai.Model{{ID: "opera-offline-demo", Provider: "openai"}}, nil
}
func (Provider) CountTokens(_ context.Context, _ string, c ai.Context) (int64, error) {
	return providers.EstimateTokens(c), nil
}
func (Provider) Capabilities(string) providers.Capabilities {
	return providers.Capabilities{Tools: true, Streaming: true, StructuredOutput: true, JSONMode: true}
}
func (Provider) Close() error { return nil }
func (p Provider) Stream(_ context.Context, req providers.StreamRequest) (providers.EventStream, error) {
	child := false
	done := map[string]bool{}
	for _, message := range req.Context.Messages {
		if message.Role == ai.RoleUser {
			for _, block := range message.Content {
				if strings.HasPrefix(block.Text, "opera-demo-child") {
					child = true
				}
			}
		}
		if message.ToolResult != nil {
			done[message.ToolResult.ToolCallID] = true
		}
	}
	for _, message := range req.Context.Messages {
		if message.ToolResult != nil && message.ToolResult.IsError && message.ToolResult.ToolCallID != "demo-7" {
			return &stream{events: []ai.Event{ai.NewErrorEvent("demo_tool_failed", fmt.Errorf("demo tool %s failed", message.ToolResult.ToolName))}}, nil
		}
	}
	if p.worker && !child {
		done["demo-0"] = true
	}
	type step struct{ name, args string }
	stages := []step{
		{"task_create", `{"tasks":[{"name":"Implement and verify demo change"}]}`},
		{"task_refine", `{"parent_id":"task-1","tasks":[{"name":"Read instructions","order":1},{"name":"Implement and verify change","order":2}]}`},
		{"subagent_spawn", `{"name":"reader","prompt":"opera-demo-child: inspect AGENTS.md","task_id":"task-2"}`},
		{"subagent_wait", `{}`},
		{"task_complete", `{"id":"task-2"}`},
		{"task_start", `{"id":"task-3"}`},
		{"write_file", `{"path":"approved.txt","content":"opera verified"}`},
		{"write_file", `{"path":"denied.txt","content":"must not exist"}`},
	}
	command, _ := json.Marshal(map[string]any{"command": p.Command, "args": p.Args})
	stages = append(stages, step{"run_command", string(command)}, step{"read_file", `{"path":"approved.txt"}`}, step{"task_complete", `{"id":"task-3"}`}, step{"task_complete", `{"id":"task-1"}`})
	if child {
		stages = []step{{"read_file", `{"path":"AGENTS.md"}`}}
	}
	events := []ai.Event{}
	found := false
	for i, step := range stages {
		id := "demo-" + strconv.Itoa(i)
		if !done[id] {
			events = append(events, ai.ToolCallEvent{ToolCall: ai.ToolCall{ID: id, Name: step.name, Arguments: json.RawMessage(step.args)}, Complete: true})
			found = true
			break
		}
	}
	if !found {
		text := "OPERA_DEMO_OK"
		if child {
			text = "Child inspection verified"
		}
		if req.Options.ResponseFormat != nil {
			text = `{"status":"verified"}`
		}
		events = append(events, ai.TextDelta{Text: text})
	}
	events = append(events, ai.UsageEvent{Usage: ai.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110}}, ai.StopEvent{Reason: ai.StopReasonStop})
	return &stream{events: events}, nil
}

type stream struct {
	mu     sync.Mutex
	events []ai.Event
	closed bool
}

func (s *stream) Next(ctx context.Context) (ai.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.closed || len(s.events) == 0 {
		return nil, io.EOF
	}
	e := s.events[0]
	s.events = s.events[1:]
	return e, nil
}
func (s *stream) Close() error { s.mu.Lock(); defer s.mu.Unlock(); s.closed = true; return nil }
