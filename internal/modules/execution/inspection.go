package execution

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/policy"
)

type InspectFilter struct {
	Type, Tool, ID string
	After          uint64
	Limit          int
}

func (s *Service) InspectRecords(ctx context.Context, run, kind string, filter InspectFilter) (json.RawMessage, error) {
	if filter.Limit < 1 || filter.Limit > 1000 {
		return nil, errors.New("inspection limit must be 1..1000")
	}
	view, err := s.Inspect(ctx, run)
	if err != nil {
		return nil, err
	}
	var value any
	switch kind {
	case "decisions":
		rows := []policy.AuditEvent{}
		for i, event := range view.Decisions {
			if uint64(i) < filter.After || filter.Type != "" && string(event.Decision) != filter.Type || filter.ID != "" && event.EventID != filter.ID && event.ToolCallID != filter.ID || filter.Tool != "" && event.ToolName != filter.Tool {
				continue
			}
			rows = append(rows, event)
			if len(rows) == filter.Limit {
				break
			}
		}
		value = rows
	case "events":
		rows := []agent.EventEnvelope{}
		for _, event := range view.Events {
			if event.Sequence <= filter.After {
				continue
			}
			if filter.Type != "" && string(event.Type) != filter.Type {
				continue
			}
			if filter.ID != "" && event.EventID != filter.ID {
				continue
			}
			if filter.Tool != "" {
				var payload struct {
					ToolCall struct {
						Name string `json:"name"`
					} `json:"tool_call"`
					ToolResult struct {
						ToolName string `json:"tool_name"`
					} `json:"tool_result"`
				}
				if err := json.Unmarshal(event.Payload, &payload); err != nil {
					return nil, err
				}
				if payload.ToolCall.Name != filter.Tool && payload.ToolResult.ToolName != filter.Tool {
					continue
				}
			}
			rows = append(rows, event)
			if len(rows) == filter.Limit {
				break
			}
		}
		value = rows
	case "operations":
		rows := []agent.OperationRecord{}
		for i, op := range view.Operations {
			if uint64(i) < filter.After || filter.Tool != "" && op.ToolName != filter.Tool || filter.ID != "" && op.IdempotencyKey != filter.ID && op.ToolCallID != filter.ID {
				continue
			}
			rows = append(rows, op)
			if len(rows) == filter.Limit {
				break
			}
		}
		value = rows
	case "output":
		if filter.ID == "" {
			return nil, errors.New("output inspection requires --operation-id <tool-call-id>")
		}
		doc, err := s.Records.Get(ctx, "tool-output", run+":"+filter.ID)
		if err != nil {
			return nil, err
		}
		value = doc.Data
	default:
		return nil, errors.New("unknown inspection record kind")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var redacted any
	if err := json.Unmarshal(policy.Redact(raw), &redacted); err != nil {
		return nil, err
	}
	scrub(redacted)
	return json.Marshal(redacted)
}
