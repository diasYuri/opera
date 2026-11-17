package subscription

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/openai"
)

// Provider reuses Y's Responses mapping, with an account-specific catalog and
// strict endpoint routing. Subscription credentials never reach custom base URLs.
type Provider struct {
	providers.Provider
	lookup func(context.Context, string) (string, error)
	http   *http.Client
}

func NewProvider(lookup func(context.Context, string) (string, error)) *Provider {
	client := &http.Client{Transport: routeTransport{http.DefaultTransport}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Provider{Provider: openai.New(openai.WithEnvLookup(func(string) string { return "" }), openai.WithHTTPClient(client), openai.WithRetryPolicy(providers.RetryPolicy{})), lookup: lookup, http: client}
}

func (*Provider) ID() string { return ProviderID }
func (*Provider) CountTokens(ctx context.Context, _ string, input ai.Context) (int64, error) {
	return providers.EstimateTokens(input), ctx.Err()
}

func (p *Provider) Models(ctx context.Context) ([]ai.Model, error) {
	if p.lookup == nil {
		return nil, errors.New("ChatGPT model catalog requires an authenticated account")
	}
	token, err := p.lookup(ctx, ProviderID)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resource+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, errors.New("ChatGPT model catalog unavailable")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ChatGPT model catalog failed (HTTP %d); check subscription access or sign in again", resp.StatusCode)
	}
	var catalog struct {
		Models *[]struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
			Visibility  string `json:"visibility"`
		} `json:"models"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&catalog) != nil || catalog.Models == nil {
		return nil, errors.New("invalid ChatGPT model catalog")
	}
	models := []ai.Model{}
	for _, m := range *catalog.Models {
		if m.Visibility == "list" && m.Slug != "" {
			models = append(models, ai.Model{ID: m.Slug, Name: m.DisplayName, Provider: ProviderID, API: "openai-responses", BaseURL: resource, Reasoning: p.Capabilities(m.Slug).Reasoning, Input: []ai.InputKind{ai.InputText, ai.InputImage}, Capabilities: ai.ModelCapabilities{StructuredOutput: p.Capabilities(m.Slug).StructuredOutput}})
		}
	}
	return models, nil
}

func (p *Provider) Stream(ctx context.Context, req providers.StreamRequest) (providers.EventStream, error) {
	state := &streamState{}
	ctx = context.WithValue(ctx, streamStateKey{}, state)
	req.Model.BaseURL = resource
	if req.Options.Reasoning != "" && req.Options.Reasoning != ai.ThinkingOff {
		req.Model.Reasoning = p.Capabilities(req.Model.ID).Reasoning
	}
	if req.Options.APIKey == "" && p.lookup != nil {
		token, err := p.lookup(ctx, ProviderID)
		if err != nil {
			return nil, err
		}
		req.Options.APIKey = token
	}
	stream, err := p.Provider.Stream(ctx, req)
	if err != nil {
		if apiErr := state.httpError(); apiErr != nil {
			return nil, apiErr
		}
		return nil, err
	}
	return &completedStream{EventStream: stream, state: state}, nil
}

type routeTransport struct{ base http.RoundTripper }

func (t routeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	allowed := req.Method == "GET" && req.URL.Path == "/v1/models" || req.Method == "POST" && req.URL.Path == "/v1/responses"
	if req.URL.Scheme != "https" || req.URL.Host != "api.openai.com" || req.URL.User != nil || !allowed {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, errors.New("unsupported ChatGPT subscription route")
	}
	if req.Method == http.MethodPost {
		if err := subscriptionBody(req); err != nil {
			return nil, err
		}
	}
	resp, err := t.base.RoundTrip(req)
	if err == nil && resp.StatusCode != http.StatusOK {
		failure := sanitizeErrorResponse(resp)
		if state, ok := req.Context().Value(streamStateKey{}).(*streamState); ok {
			requestID := resp.Header.Get("x-request-id")
			if requestID == "" {
				requestID = resp.Header.Get("openai-request-id")
			}
			state.setHTTP(failure, resp.StatusCode, requestID)
		}
	}
	if err == nil && resp.StatusCode == http.StatusOK && req.Method == http.MethodPost {
		if state, ok := req.Context().Value(streamStateKey{}).(*streamState); ok {
			requestID := resp.Header.Get("x-request-id")
			if requestID == "" {
				requestID = resp.Header.Get("openai-request-id")
			}
			state.setHTTP(apiFailure{}, resp.StatusCode, requestID)
			resp.Body = &observedBody{ReadCloser: resp.Body, state: state}
		}
	}
	return resp, err
}

type completedStream struct {
	providers.EventStream
	terminal bool
	state    *streamState
}

func (s *completedStream) Next(ctx context.Context) (ai.Event, error) {
	event, err := s.EventStream.Next(ctx)
	if errors.Is(err, io.EOF) && !s.terminal {
		return nil, errors.New("ChatGPT stream interrupted before response.completed")
	}
	if stop, ok := event.(ai.StopEvent); ok {
		s.terminal = true
		if stop.Reason == ai.StopReasonLength {
			return nil, errors.New("ChatGPT response.incomplete: generation did not complete")
		}
	}
	if _, ok := event.(ai.ErrorEvent); ok {
		s.terminal = true
		return nil, s.state.streamError()
	}
	return event, err
}

type streamStateKey struct{}
type streamState struct {
	mu        sync.Mutex
	failure   apiFailure
	status    int
	requestID string
}

func (s *streamState) setHTTP(failure apiFailure, status int, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failure, s.status, s.requestID = failure, status, id
}
func (s *streamState) httpError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status < 400 {
		return nil
	}
	message := failureMessage(s.failure, s.status, s.requestID)
	if s.status >= 400 && s.status < 500 && s.status != 401 && s.status != 403 && s.status != 413 && s.status != 429 {
		return &APIError{StatusCode: s.status, Code: s.failure.Code, Param: s.failure.Param, RequestID: s.requestID}
	}
	return providers.ClassifyHTTPError(ProviderID, s.status, 0, message, errors.New(message))
}

func (s *streamState) streamError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return errors.New(failureMessage(s.failure, s.status, s.requestID))
}
func (s *streamState) setFailure(failure apiFailure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failure = failure
}

type observedBody struct {
	io.ReadCloser
	state   *streamState
	pending []byte
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.pending = append(b.pending, p[:n]...)
	for {
		line, rest, found := bytes.Cut(b.pending, []byte("\n"))
		if !found {
			break
		}
		if bytes.HasPrefix(line, []byte("data: ")) {
			var event struct {
				Type     string     `json:"type"`
				Code     string     `json:"code"`
				Param    string     `json:"param"`
				Error    apiFailure `json:"error"`
				Response struct {
					Error apiFailure `json:"error"`
				} `json:"response"`
			}
			if json.Unmarshal(bytes.TrimPrefix(line, []byte("data: ")), &event) == nil && (event.Type == "response.failed" || event.Type == "error") {
				failure := event.Response.Error
				if failure.Code == "" {
					failure = event.Error
				}
				if failure.Code == "" {
					failure = apiFailure{Code: event.Code, Param: event.Param}
				}
				b.state.setFailure(failure)
			}
		}
		b.pending = rest
	}
	if len(b.pending) > 1<<20 {
		return n, errors.New("ChatGPT stream event exceeds size limit")
	}
	return n, err
}

func subscriptionError(code string) error {
	switch code {
	case "subscription_sharing_usage_limit_exceeded":
		return errors.New("ChatGPT plan usage limit reached; review https://chatgpt.com/settings/usage")
	case "subscription_sharing_user_not_eligible":
		return errors.New("ChatGPT plan usage unavailable for this account or workspace")
	case "subscription_sharing_usage_unavailable", "subscription_sharing_user_unavailable":
		return errors.New("ChatGPT plan usage temporarily unavailable; retry later")
	case "subscription_sharing_unsupported_capability":
		return errors.New("ChatGPT plan does not support a requested capability")
	default:
		return errors.New("ChatGPT inference failed")
	}
}

// Keep the compiler checking the adapter's interface as Y evolves.
var _ providers.Provider = (*Provider)(nil)
