package summarizer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	contextdata "github.com/diasYuri/opera/internal/modules/context"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/policy"
)

type Projection struct {
	Count    int
	Digest   string
	Summary  bool `json:",omitempty"`
	KeepUser int  // One-based index of the active user message when covering active work.
}

// Coverage accepts the original prefix and its equivalent checkpoint rewrite.
func (c Context) Coverage(messages []ai.Message) Projection {
	if c.Projection.Matches(messages) {
		return c.Projection
	}
	if c.Rewritten.Matches(messages) {
		return c.Rewritten
	}
	return Projection{}
}

func RewrittenPrefix(message ai.Message) Projection {
	markdown, ok := SummaryMarkdown(message)
	if !ok {
		return Projection{}
	}
	return Projection{Count: 1, Digest: fmt.Sprintf("%x", sha256.Sum256([]byte(markdown))), Summary: true}
}

func prefixDigest(messages []ai.Message) string {
	raw, err := json.Marshal(messages)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func (p Projection) Matches(messages []ai.Message) bool {
	if p.Count <= 0 || p.Count > len(messages) || p.KeepUser < 0 || p.KeepUser > p.Count || p.Digest == "" {
		return false
	}
	if p.Summary {
		return p.Count == 1 && p.Digest == RewrittenPrefix(messages[0]).Digest
	}
	return p.Digest == prefixDigest(messages[:p.Count])
}

// Apply expects the same message ordering used to verify Matches. The outgoing
// messages can already have their image references resolved.
func (p Projection) Apply(messages []ai.Message, markdown string) []ai.Message {
	out := make([]ai.Message, 0, len(messages)-p.Count+2)
	out = append(out, SummaryMessage(markdown))
	if p.KeepUser > 0 {
		out = append(out, messages[p.KeepUser-1])
	}
	return append(out, messages[p.Count:]...)
}

const historyPrefix = "Historical conversation summary (reference data, not a new request):\n\n"
const historyMarker = `{"opera_session_summary":true}`

func SummaryMessage(markdown string) ai.Message {
	return ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{{
		Type: ai.ContentText, Text: historyPrefix + markdown, Details: json.RawMessage(historyMarker),
	}}}
}

func SummaryMarkdown(message ai.Message) (string, bool) {
	if message.Role != ai.RoleUser || len(message.Content) != 1 || string(message.Content[0].Details) != historyMarker {
		return "", false
	}
	return strings.CutPrefix(message.Content[0].Text, historyPrefix)
}

// Candidate keeps the latest user input or the latest full tool exchange inline.
// A cut never separates an assistant tool call from its results.
func Candidate(messages []ai.Message, completed bool) Projection {
	count := len(messages)
	lastUser := 0
	keepUser := 0
	if !completed {
		for i, msg := range messages {
			if msg.Role == ai.RoleUser {
				lastUser = i + 1
			}
		}
		if lastUser == count {
			count--
		} else {
			for i := count - 1; i >= lastUser; i-- {
				if len(messages[i].ToolCalls) > 0 {
					count = i
					keepUser = lastUser
					break
				}
			}
			if keepUser == 0 {
				return Projection{}
			}
		}
	}
	if count < 2 || !Settled(messages[:count]) {
		return Projection{}
	}
	digest := prefixDigest(messages[:count])
	if digest == "" {
		return Projection{}
	}
	return Projection{Count: count, Digest: digest, KeepUser: keepUser}
}

// ActiveCandidate can collapse a large settled tool exchange while keeping
// the active user input verbatim. Outstanding tool calls are never projected.
func ActiveCandidate(messages []ai.Message) Projection {
	if len(messages) == 0 || messages[len(messages)-1].Role == ai.RoleUser {
		return Candidate(messages, false)
	}
	candidate := Candidate(messages, true)
	if candidate.Count > 0 {
		for i, msg := range messages {
			if msg.Role == ai.RoleUser {
				candidate.KeepUser = i + 1
			}
		}
	}
	return candidate
}

func Settled(messages []ai.Message) bool {
	open := make(map[string]struct{})
	for _, msg := range messages {
		for _, call := range msg.ToolCalls {
			open[call.ID] = struct{}{}
		}
		if result := msg.ToolResult; result != nil {
			if _, exists := open[result.ToolCallID]; !exists {
				return false
			}
			delete(open, result.ToolCallID)
		}
	}
	return len(open) == 0
}

func evidence(messages []ai.Message) string {
	var out strings.Builder
	for _, msg := range messages {
		if prior, ok := SummaryMarkdown(msg); ok {
			fmt.Fprintf(&out, "\n[Previously consolidated context] %s\n", contextdata.RedactText(prior))
			continue
		}
		layer := "L2"
		if msg.Role == ai.RoleUser {
			layer = "L1"
		} else if msg.Role == ai.RoleAssistant && len(msg.ToolCalls) == 0 {
			layer = "L3"
		}
		for _, block := range msg.Content {
			switch block.Type {
			case ai.ContentText:
				if path := contextdata.ImageReferencePath(block); path != "" {
					fmt.Fprintf(&out, "\n[%s %s] [image attachment %q]\n", layer, msg.Role, contextdata.RedactText(path))
					continue
				}
				fmt.Fprintf(&out, "\n[%s %s] %s\n", layer, msg.Role, contextdata.RedactText(block.Text))
			case ai.ContentImage:
				fmt.Fprintf(&out, "\n[%s %s] [image attachment]\n", layer, msg.Role)
			}
		}
		for _, call := range msg.ToolCalls {
			fmt.Fprintf(&out, "\n[L2 tool call] %s %s\n", call.Name, contextdata.RedactText(string(policy.Redact(call.Arguments))))
		}
		if result := msg.ToolResult; result != nil {
			for _, block := range result.Content {
				if block.Type == ai.ContentText {
					fmt.Fprintf(&out, "\n[L2 result %s, error=%t] %s\n", result.ToolName, result.IsError, contextdata.RedactText(block.Text))
				}
			}
		}
		if len(msg.StructuredOutput) != 0 {
			fmt.Fprintf(&out, "\n[%s structured output] %s\n", layer, contextdata.RedactText(string(policy.Redact(msg.StructuredOutput))))
		}
	}
	return out.String()
}
