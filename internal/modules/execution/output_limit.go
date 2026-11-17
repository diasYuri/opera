package execution

import (
	"context"
	"encoding/json"
	"github.com/diasYuri/y/pkg/tools"
	"strings"
	"unicode/utf8"
)

// Preserve a successful effect result while bounding what reaches events/context.
func OutputLimit(limit int, archive ...func(context.Context, tools.ToolRequest, tools.ToolResponse) error) tools.ToolExecutionMiddleware {
	return func(ctx context.Context, request tools.ToolRequest, desc tools.ToolDescriptor, next tools.ToolExecutionNext) (tools.ToolResponse, error) {
		response, err := next(ctx, request)
		if err != nil {
			return response, err
		}
		raw, marshalErr := json.Marshal(response)
		if marshalErr != nil {
			return tools.ToolResponse{}, marshalErr
		}
		if len(raw) <= limit {
			return response, nil
		}
		for _, save := range archive {
			if err := save(ctx, request, response); err != nil {
				return response, err
			}
		}
		remaining := max(0, limit/2)
		var text strings.Builder
		for _, block := range response.Content {
			if remaining == 0 {
				break
			}
			part := validPrefix(block.Text, remaining)
			text.WriteString(part)
			remaining -= len(part)
			if remaining > 0 {
				text.WriteByte('\n')
				remaining--
			}
		}
		response.Details = nil
		response.Logs = nil
		response.Metadata = json.RawMessage(`{"truncated":true}`)
		excerpt := text.String()
		for {
			response.Content = []tools.ContentBlock{{Type: tools.ContentText, Text: excerpt + "\n[tool output truncated by configured output budget]"}}
			encoded, err := json.Marshal(response)
			if err != nil {
				return tools.ToolResponse{}, err
			}
			if len(encoded) <= limit || len(excerpt) == 0 {
				break
			}
			excerpt = validPrefix(excerpt, len(excerpt)/2)
		}

		return response, nil
	}
}

func validPrefix(text string, limit int) string {
	if len(text) > limit {
		text = text[:limit]
	}
	for !utf8.ValidString(text) && len(text) > 0 {
		text = text[:len(text)-1]
	}
	return text
}
