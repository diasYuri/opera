package custom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/providercatalog"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/anthropic"
	"github.com/diasYuri/y/pkg/providers/openai"
)

func Entry(def config.CustomProvider) providercatalog.Entry {
	id := def.ID()
	return providercatalog.Entry{ID: id, Name: def.Name, Description: "Custom " + def.ProviderType + " provider", Factory: func(_ context.Context, _ config.Resolved, _ func(context.Context, string) (string, error)) (providers.Provider, error) {
		var provider providers.Provider
		var api string
		switch def.ProviderType {
		case "openai":
			provider = openai.NewCompatible(def.BaseURL, openai.WithEnvLookup(func(string) string { return "" }))
			api = "openai-completions"
		case "anthropic":
			provider = anthropic.New(anthropic.WithBaseURL(def.BaseURL), anthropic.WithEnvLookup(func(string) string { return "" }))
			api = "anthropic-messages"
		default:
			return nil, fmt.Errorf("custom provider %q has unsupported provider_type", def.Name)
		}
		return newCustomProvider(provider, id, api, def), nil
	}}
}

type customProvider struct {
	providers.Provider
	id     string
	api    string
	models map[string]config.CustomProviderModel
	listed []ai.Model
}

func newCustomProvider(provider providers.Provider, id, api string, def config.CustomProvider) *customProvider {
	out := &customProvider{Provider: provider, id: id, api: api, models: make(map[string]config.CustomProviderModel, len(def.Models)), listed: make([]ai.Model, 0, len(def.Models))}
	for _, model := range def.Models {
		out.models[model.ID] = model
		input := make([]ai.InputKind, 0, len(model.Input))
		for _, kind := range model.Input {
			if kind == "image" {
				input = append(input, ai.InputImage)
			} else {
				input = append(input, ai.InputText)
			}
		}
		metadata, _ := json.Marshal(map[string][]string{"thinkingLevel": model.ThinkingLevel})
		out.listed = append(out.listed, ai.Model{
			ID: model.ID, Name: model.Name, API: ai.API(api), Provider: ai.ProviderID(id), BaseURL: def.BaseURL,
			Reasoning: supportsThinking(model.ThinkingLevel), Input: input,
			Cost:          ai.Cost{Input: model.Cost.Input, Output: model.Cost.Output, CacheRead: model.Cost.CacheRead, CacheWrite: model.Cost.CacheWrite},
			ContextWindow: model.ContextWindow, MaxTokens: model.MaxTokens, Metadata: metadata,
		})
	}
	return out
}

func (p *customProvider) ID() string { return p.id }

func (p *customProvider) Models(ctx context.Context) ([]ai.Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	models := append([]ai.Model(nil), p.listed...)
	for i := range models {
		models[i].Input = append([]ai.InputKind(nil), models[i].Input...)
		models[i].Metadata = append([]byte(nil), models[i].Metadata...)
	}
	return models, nil
}

func (p *customProvider) Stream(ctx context.Context, req providers.StreamRequest) (providers.EventStream, error) {
	model, ok := p.models[req.Model.ID]
	if !ok {
		return nil, fmt.Errorf("custom provider %q does not define model %q", p.id, req.Model.ID)
	}
	requested := string(req.Options.Reasoning)
	if requested != "" && requested != "off" && !contains(model.ThinkingLevel, requested) {
		return nil, fmt.Errorf("custom provider %q model %q does not support thinking level %q", p.id, model.ID, requested)
	}
	canonical, ok := p.findModel(model.ID)
	if !ok {
		return nil, fmt.Errorf("custom provider %q model metadata is unavailable", p.id)
	}
	req.Model = canonical
	return p.Provider.Stream(ctx, req)
}

func (p *customProvider) findModel(id string) (ai.Model, bool) {
	for _, model := range p.listed {
		if model.ID == id {
			return model, true
		}
	}
	return ai.Model{}, false
}

func (p *customProvider) Capabilities(modelID string) providers.Capabilities {
	model, ok := p.models[modelID]
	if !ok {
		return providers.Capabilities{}
	}
	capabilities := providers.Capabilities{Tools: true, Streaming: true, Reasoning: supportsThinking(model.ThinkingLevel)}
	for _, input := range model.Input {
		if input == "image" {
			capabilities.Vision = true
			break
		}
	}
	return capabilities
}

func supportsThinking(levels []string) bool {
	for _, level := range levels {
		if level != "" && level != "off" {
			return true
		}
	}
	return false
}

func CustomAPIKey(ctx context.Context, provider config.CustomProvider) (string, error) {
	if value := strings.TrimSpace(provider.APIKey); value != "" {
		return value, nil
	}
	if strings.TrimSpace(provider.APIKeyHelper) == "" {
		return "", errors.New("custom provider has no API key source")
	}
	return runAPIKeyHelper(ctx, provider.APIKeyHelper)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
