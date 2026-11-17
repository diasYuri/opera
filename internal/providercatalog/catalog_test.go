package providercatalog

import (
	"context"
	"errors"
	"testing"

	"github.com/diasYuri/opera/internal/config"
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

type failedProvider struct {
	*providertest.FakeProvider
	closeError error
	closed     int
}

func (p *failedProvider) Close() error { p.closed++; return p.closeError }
