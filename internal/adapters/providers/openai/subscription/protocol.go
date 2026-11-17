package subscription

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// The subscription preview accepts a narrower Responses contract than API keys.
func subscriptionBody(req *http.Request) error {
	original := req.Body
	defer func() { _ = original.Close() }()
	var body map[string]json.RawMessage
	if json.NewDecoder(io.LimitReader(req.Body, 16<<20)).Decode(&body) != nil {
		return errors.New("invalid ChatGPT inference request")
	}
	for _, key := range []string{"background", "conversation", "max_output_tokens", "max_tool_calls", "metadata", "moderation", "multi_agent", "prompt", "prompt_cache_retention", "safety_identifier", "temperature", "top_logprobs", "top_p", "truncation", "user", "previous_response_id"} {
		delete(body, key)
	}
	body["store"], body["stream"] = json.RawMessage(`false`), json.RawMessage(`true`)
	var input []map[string]json.RawMessage
	if json.Unmarshal(body["input"], &input) != nil {
		return errors.New("ChatGPT inference requires input history as an array")
	}
	for _, item := range input {
		if string(item["role"]) == `"system"` {
			item["role"] = json.RawMessage(`"developer"`)
		}
		// Y's easy-message encoder uses input_text for both roles. The
		// subscription route requires prior assistant content as output_text.
		if string(item["role"]) == `"assistant"` {
			var content []map[string]json.RawMessage
			if json.Unmarshal(item["content"], &content) != nil {
				return errors.New("invalid assistant history content")
			}
			for _, part := range content {
				if string(part["type"]) == `"input_text"` {
					part["type"] = json.RawMessage(`"output_text"`)
					part["annotations"] = json.RawMessage(`[]`)
				}
			}
			item["content"], _ = json.Marshal(content)
			item["type"] = json.RawMessage(`"message"`)
			item["status"] = json.RawMessage(`"completed"`)
		}
		if string(item["type"]) == `"function_call"` || string(item["type"]) == `"function_call_output"` {
			item["namespace"] = json.RawMessage(`"opera"`)
		}
	}
	body["input"], _ = json.Marshal(input)
	var tools []json.RawMessage
	if raw, ok := body["tools"]; ok {
		if json.Unmarshal(raw, &tools) != nil {
			return errors.New("invalid ChatGPT tools")
		}
		if len(tools) > 0 {
			wrapped, _ := json.Marshal([]any{map[string]any{"type": "namespace", "name": "opera", "description": "Opera local tools", "tools": tools}})
			body["tools"] = wrapped
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return errors.New("cannot encode ChatGPT inference request")
	}
	req.Body = io.NopCloser(bytes.NewReader(raw))
	req.ContentLength = int64(len(raw))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
	return nil
}
