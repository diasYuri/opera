package subscription

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type apiFailure struct {
	Code  string `json:"code"`
	Param string `json:"param"`
}

// APIError identifies a permanent request rejection, not a transport failure.
type APIError struct {
	StatusCode             int
	Code, Param, RequestID string
}

func (e *APIError) Error() string {
	return failureMessage(apiFailure{Code: e.Code, Param: e.Param}, e.StatusCode, e.RequestID)
}

func diagnosticField(value string) string {
	if len(value) > 160 {
		return ""
	}
	for _, r := range value {
		allowed := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_-.[]:/", r)
		if !allowed {
			return ""
		}
	}
	return value
}

func failureMessage(failure apiFailure, status int, requestID string) string {
	message := subscriptionError(failure.Code).Error()
	var details []string
	if status >= 400 {
		details = append(details, fmt.Sprintf("HTTP %d", status))
	}
	if code := diagnosticField(failure.Code); code != "" {
		details = append(details, "code="+code)
	}
	if param := diagnosticField(failure.Param); param != "" {
		details = append(details, "param="+param)
	}
	if id := diagnosticField(requestID); id != "" {
		details = append(details, "request="+id)
	}
	if len(details) > 0 {
		message += " (" + strings.Join(details, "; ") + ")"
	}
	return message
}

// Preserve the HTTP response so Y/OpenAI classify API rejections by status,
// rather than turning every HTTP 400 into a network error. Keep diagnostic
// identifiers, never arbitrary response messages that may echo credentials/input.
func sanitizeErrorResponse(resp *http.Response) apiFailure {
	var body struct {
		Error apiFailure `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body)
	_ = resp.Body.Close()
	requestID := resp.Header.Get("x-request-id")
	if requestID == "" {
		requestID = resp.Header.Get("openai-request-id")
	}
	message := failureMessage(body.Error, resp.StatusCode, requestID)
	if resp.StatusCode == http.StatusUnauthorized && body.Error.Code == "" {
		message = "ChatGPT authorization failed; use /login to sign in again"
	}
	raw, _ := json.Marshal(map[string]any{"error": map[string]string{"message": message, "code": diagnosticField(body.Error.Code), "param": diagnosticField(body.Error.Param), "type": "invalid_request_error"}})
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	resp.ContentLength = int64(len(raw))
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length")
	return body.Error
}
