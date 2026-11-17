package providercatalog

import (
	"context"
	"errors"
	"testing"

	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/providertest"
)

func TestCatalogRejectsDuplicateAndUnknownProviders(t *testing.T) {
	factory := func(context.Context, config.Resolved, func(context.Context, string) (string, error)) (providers.Provider, error) {
		return nil, errors.New("factory reached")
	}
	if _, err := New(Entry{ID: "custom", Factory: factory}, Entry{ID: "custom", Factory: factory}); err == nil {
		t.Fatal("duplicate provider ID accepted")
	}
	catalog, err := New(Entry{ID: "custom", Factory: factory})
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.IDs(); len(got) != 1 || got[0] != "custom" {
		t.Fatalf("IDs() = %v", got)
	}
	if _, err := catalog.Create(context.Background(), config.Resolved{}, nil); err == nil {
		t.Fatal("unregistered provider accepted")
	}
	if _, err := catalog.Create(context.Background(), config.Resolved{Values: map[string]config.Value{"provider": {Value: "custom"}}}, nil); err == nil || err.Error() != "create provider \"custom\": factory reached" {
		t.Fatalf("custom provider factory error = %v", err)
	}
}

func TestCatalogRejectsFactoryWithMismatchedProviderID(t *testing.T) {
	provider := providertest.NewFakeProvider()
	catalog, err := New(Entry{ID: "custom", Factory: func(context.Context, config.Resolved, func(context.Context, string) (string, error)) (providers.Provider, error) {
		return provider, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = catalog.Create(context.Background(), config.Resolved{Values: map[string]config.Value{"provider": {Value: "custom"}}}, nil)
	if err == nil || err.Error() != `provider "custom" factory returned provider "fake"` {
		t.Fatalf("mismatched provider error = %v", err)
	}
	if !provider.IsClosed() {
		t.Fatal("mismatched provider was not closed")
	}
}

func TestCatalogClosesPartialProviderAndPreservesErrors(t *testing.T) {
	factoryFailure := errors.New("factory failure")
	cleanupFailure := errors.New("cleanup failure")
	provider := &failedProvider{FakeProvider: providertest.NewFakeProvider(), closeError: cleanupFailure}
	catalog, err := New(Entry{ID: "custom", Factory: func(context.Context, config.Resolved, func(context.Context, string) (string, error)) (providers.Provider, error) {
		return provider, factoryFailure
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = catalog.Create(context.Background(), config.Resolved{Values: map[string]config.Value{"provider": {Value: "custom"}}}, nil)
	if !errors.Is(err, factoryFailure) || !errors.Is(err, cleanupFailure) {
		t.Fatalf("lost factory/cleanup errors: %v", err)
	}
	if provider.closed != 1 {
		t.Fatalf("partial provider closed %d times", provider.closed)
	}
}

func TestCatalogAcceptsAuthenticationDoubleWithoutProviderImplementation(t *testing.T) {
	handler := authDouble{}
	catalog, err := New(Entry{ID: "fixture", FixedEndpoint: true, Factory: func(context.Context, config.Resolved, func(context.Context, string) (string, error)) (providers.Provider, error) {
		return providertest.NewFakeProvider(), nil
	}, Login: &LoginDescriptor{ID: "fixture", Name: "Fixture", Methods: []LoginMethod{{ID: "oauth", Name: "Fixture login", Kind: "oauth", ProviderID: "fixture", Authenticator: handler}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Authenticator("fixture"); got != handler {
		t.Fatalf("authentication handler = %#v", got)
	}
	if !catalog.FixedEndpoint("fixture") {
		t.Fatal("fixed endpoint metadata was lost")
	}
}

type authDouble struct{}

func (authDouble) Supports(string) bool { return true }
func (authDouble) Authenticate(context.Context, string, AuthIO) (AuthResult, error) {
	return AuthResult{Status: "stored"}, nil
}
func (authDouble) Credential(context.Context, AuthIO) (string, error) { return "fixture", nil }

type failedProvider struct {
	*providertest.FakeProvider
	closeError error
	closed     int
}

func (p *failedProvider) Close() error { p.closed++; return p.closeError }
