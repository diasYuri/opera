package providercatalog

import (
	"context"
	"errors"
	"testing"

	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/y/pkg/providers"
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
