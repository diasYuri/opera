package search

import (
	"context"
	"reflect"
	"testing"

	searchcontract "github.com/diasYuri/y/pkg/search"
)

type configuredTestProvider struct{ request searchcontract.Request }

func (p *configuredTestProvider) Capabilities() searchcontract.Capabilities {
	return searchcontract.Capabilities{AllowedDomains: true, ExcludedDomains: true, ContextSize: true, MaxResults: true, Country: true, Language: true, SafeSearch: true}
}

func (p *configuredTestProvider) Search(_ context.Context, request searchcontract.Request) (searchcontract.Response, error) {
	p.request = request
	return searchcontract.Response{}, nil
}

func TestWithDefaultsFillsUnsetControlsAndPreservesExplicitValues(t *testing.T) {
	provider := &configuredTestProvider{}
	defaults := searchcontract.Request{
		AllowedDomains: []string{"example.com"}, ExcludedDomains: []string{"private.example.com"},
		ContextSize: searchcontract.ContextSizeLow, MaxResults: 7, Country: "BR", Language: "pt", SafeSearch: searchcontract.SafeSearchStrict,
	}
	configured := WithDefaults(provider, defaults)
	if _, err := configured.Search(context.Background(), searchcontract.Request{Query: "first"}); err != nil {
		t.Fatal(err)
	}
	if got := provider.request; got.ContextSize != defaults.ContextSize || got.MaxResults != defaults.MaxResults || got.Country != defaults.Country || got.Language != defaults.Language || got.SafeSearch != defaults.SafeSearch || !reflect.DeepEqual(got.AllowedDomains, defaults.AllowedDomains) || !reflect.DeepEqual(got.ExcludedDomains, defaults.ExcludedDomains) {
		t.Fatalf("configured defaults were not applied: %#v", got)
	}
	explicit := searchcontract.Request{Query: "second", AllowedDomains: []string{"docs.example.com"}, ExcludedDomains: []string{"private.example.com"}, ContextSize: searchcontract.ContextSizeHigh, MaxResults: 3, Country: "US", Language: "en", SafeSearch: searchcontract.SafeSearchOff}
	if _, err := configured.Search(context.Background(), explicit); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(provider.request, explicit) {
		t.Fatalf("explicit request settings were overwritten: got %#v want %#v", provider.request, explicit)
	}
}
