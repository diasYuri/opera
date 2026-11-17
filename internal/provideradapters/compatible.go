package provideradapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
)

// compatibleProvider gives a named OpenAI-compatible endpoint its own provider
// identity and discovers model IDs from that endpoint on explicit catalog
// requests. Streaming and lifecycle remain owned by the published Y adapter.
type compatibleProvider struct {
	providers.Provider
	id      string
	baseURL string
	lookup  func(context.Context, string) (string, error)
}

func (p compatibleProvider) ID() string { return p.id }

func (p compatibleProvider) Models(ctx context.Context) ([]ai.Model, error) {
	if p.lookup == nil {
		return nil, fmt.Errorf("provider %q requires an API credential to list models", p.id)
	}
	key, err := p.lookup(ctx, p.id)
	if err != nil {
		return nil, fmt.Errorf("resolve %s credential for model discovery: %w", p.id, err)
	}
	if key == "" {
		return nil, fmt.Errorf("provider %q requires an API credential to list models", p.id)
	}
	url := strings.TrimRight(p.baseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build %s model discovery request: %w", p.id, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list %s models: %w", p.id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list %s models: provider returned HTTP %d", p.id, resp.StatusCode)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode %s model list: %w", p.id, err)
	}
	if body.Data == nil {
		return nil, fmt.Errorf("decode %s model list: response has no data array", p.id)
	}
	models := make([]ai.Model, 0, len(body.Data))
	for _, item := range body.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" || len(id) > 256 || strings.ContainsAny(id, "\x00\r\n\x1b") {
			continue
		}
		models = append(models, ai.Model{ID: id, API: "openai-completions", Provider: ai.ProviderID(p.id), BaseURL: p.baseURL, Input: []ai.InputKind{ai.InputText}})
	}
	return models, nil
}
