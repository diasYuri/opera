package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/diasYuri/y/pkg/tools"
)

// UserQuestion is a question Opera can ask during a chat execution.
type UserQuestion struct {
	ID       string               `json:"id,omitempty"`
	Question string               `json:"question"`
	Options  []UserQuestionOption `json:"options,omitempty"`
}

type UserQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type UserQuestionAnswer struct {
	Question string `json:"question"`
	Option   string `json:"option,omitempty"`
	Text     string `json:"text,omitempty"`
	Notes    string `json:"notes,omitempty"`
}

// QuestionBroker blocks the tool call until the chat UI submits its answers.
type QuestionBroker struct {
	mu      sync.Mutex
	pending map[string]chan json.RawMessage
}

func NewQuestionBroker() *QuestionBroker {
	return &QuestionBroker{pending: map[string]chan json.RawMessage{}}
}

func (b *QuestionBroker) Register(registry *tools.Registry) error {
	schema := json.RawMessage(`{"type":"object","properties":{"questions":{"type":"array","minItems":1,"items":{"type":"object","properties":{"id":{"type":"string"},"question":{"type":"string"},"options":{"type":"array","items":{"type":"object","properties":{"label":{"type":"string"},"description":{"type":"string"}},"required":["label"],"additionalProperties":false}},"required":["question"],"additionalProperties":false}}},"required":["questions"],"additionalProperties":false}`)
	desc := tools.ToolDescriptor{Name: "user_question", Description: "Ask the user one or more questions. Offer concise recommended options when useful; the user can choose an option, add notes, or provide an open text answer.", InputSchema: schema}
	return registry.Add(desc, tools.ToolHandlerFunc(func(ctx context.Context, req tools.ToolRequest) (tools.ToolResponse, error) {
		var input struct {
			Questions []UserQuestion `json:"questions"`
		}
		if err := json.Unmarshal(req.Arguments, &input); err != nil {
			return tools.ToolResponse{}, err
		}
		if len(input.Questions) == 0 {
			return tools.ToolResponse{}, errors.New("user_question requires at least one question")
		}
		key := req.RunID + ":" + req.ID
		ch := make(chan json.RawMessage, 1)
		b.mu.Lock()
		if b.pending == nil {
			b.pending = map[string]chan json.RawMessage{}
		}
		b.pending[key] = ch
		b.mu.Unlock()
		defer func() { b.mu.Lock(); delete(b.pending, key); b.mu.Unlock() }()
		select {
		case raw := <-ch:
			return tools.ToolResponse{Content: []tools.ContentBlock{{Type: tools.ContentText, Text: string(raw)}}, Details: raw}, nil
		case <-ctx.Done():
			return tools.ToolResponse{}, ctx.Err()
		}
	}))
}

func (b *QuestionBroker) Answer(runID, toolCallID string, answers []UserQuestionAnswer) error {
	raw, err := json.Marshal(answers)
	if err != nil {
		return err
	}
	b.mu.Lock()
	ch := b.pending[runID+":"+toolCallID]
	if ch != nil {
		delete(b.pending, runID+":"+toolCallID)
	}
	b.mu.Unlock()
	if ch == nil {
		return errors.New("user question is no longer pending")
	}
	ch <- raw
	return nil
}
