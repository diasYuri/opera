package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	operasqlite "github.com/diasYuri/opera/internal/adapters/persistence/sqlite"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/agent/agenttest"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/providers"
	"github.com/diasYuri/y/pkg/providers/providertest"
	ysqlite "github.com/diasYuri/y/pkg/storage/sqlite"
	"github.com/diasYuri/y/pkg/tools"
)

const alternateStorage ComponentID = "storage.fixture"

type fixtureModelProvider struct{ *agenttest.ScriptProvider }

func (fixtureModelProvider) ID() string { return "openai" }

// This compiled registration uses the same service keys as storage. The
// application, runner and tools can use it without knowing how it opens data.
func exampleStorage(onClose func(string)) ComponentRegistration {
	return ComponentRegistration{ID: alternateStorage, Slot: StorageSlot, Build: func(input ComponentBuildInput) (component.Definition, error) {
		path := filepath.Join(input.Config.Paths.Data, "fixture.db")
		keys := input.keys
		provides := []component.ServiceRef{component.Reference(keys.store), component.Reference(keys.records)}
		return componentDefinition(string(alternateStorage), "durable-storage", provides, nil, func(ctx context.Context, scope *component.Scope) error {
			store, err := ysqlite.Open(ctx, path)
			if err != nil {
				return fmt.Errorf("open fixture checkpoints: %w", err)
			}
			if err := scope.Track(func(context.Context) error {
				onClose("checkpoints")
				return store.Close()
			}); err != nil {
				return errors.Join(err, store.Close())
			}
			records, err := operasqlite.Open(ctx, path, input.Config.Paths.Workspace)
			if err != nil {
				return fmt.Errorf("open fixture records: %w", err)
			}
			if err := scope.Track(func(context.Context) error {
				onClose("records")
				return records.Close()
			}); err != nil {
				return errors.Join(err, records.Close())
			}
			if err := component.Provide(scope, keys.store, execution.ExecutionStore(store)); err != nil {
				return err
			}
			return component.Provide(scope, keys.records, execution.Records(records))
		}), nil
	}}
}

func exampleOptions(t *testing.T, registration ComponentRegistration) Options {
	t.Helper()
	catalog, err := DefaultComponentCatalog(registration)
	if err != nil {
		t.Fatal(err)
	}
	return Options{
		Profile: ProfileHeadless, Catalog: catalog, Selections: map[ComponentSlot]ComponentID{StorageSlot: alternateStorage},
		ProviderFactory: func(context.Context, config.Resolved) (providers.Provider, error) {
			return providertest.NewFakeProvider(), nil
		},
	}
}

func compositionConfig(t *testing.T) config.Resolved {
	t.Helper()
	cfg, err := config.Load(config.Options{
		Flags: map[string]string{"provider": "openai", "model": "fixture", "policy": "ask"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Paths.Workspace = t.TempDir()
	cfg.Paths.Data = filepath.Join(t.TempDir(), "data")
	return cfg
}

func TestCompiledStorageSubstitutionResumesApproval(t *testing.T) {
	ctx := context.Background()
	cfg := compositionConfig(t)
	var closed []string
	opts := exampleOptions(t, exampleStorage(func(name string) { closed = append(closed, name) }))
	if err := os.WriteFile(filepath.Join(cfg.Paths.Workspace, "notes.md"), []byte("fixture context"), 0600); err != nil {
		t.Fatal(err)
	}
	opts.ContextPaths = []string{"notes.md"}
	opts.ProviderFactory = func(context.Context, config.Resolved) (providers.Provider, error) {
		return fixtureModelProvider{agenttest.NewScriptProvider(t, agenttest.ScriptedTurn{
			ToolCalls: []agenttest.ScriptedToolCall{{Name: "write_file", Arguments: `{"path":"approved.txt","content":"done"}`}},
		})}, nil
	}
	plan, err := ResolveComposition(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(plan.Summary.Components, func(c SelectedComponent) bool { return c.ID == string(alternateStorage) && c.Slot == StorageSlot }) {
		t.Fatal("alternative storage missing from plan")
	}
	if _, err := os.Stat(cfg.Paths.Data); !os.IsNotExist(err) {
		t.Fatalf("resolving a graph touched storage: %v", err)
	}
	first, err := Open(ctx, cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	run, err := first.Service.Execute(ctx, execution.Request{Prompt: "write the file"}, nil)
	if err != nil || run.Approval == nil {
		t.Fatalf("approval handoff: %+v, %v", run, err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(closed, []string{"records", "checkpoints"}) {
		t.Fatalf("close order or idempotence: %v", closed)
	}
	if _, err := os.Stat(filepath.Join(cfg.Paths.Data, "opera.db")); !os.IsNotExist(err) {
		t.Fatalf("default storage was opened: %v", err)
	}

	opts.Profile, opts.ResumeID, opts.ContextPaths = ProfileDefault, run.ID, nil
	var providerStarts int
	opts.ProviderFactory = func(_ context.Context, effective config.Resolved) (providers.Provider, error) {
		providerStarts++
		if effective.String("provider") != "openai" || effective.String("model") != "fixture" || effective.Values["model"].Source != "checkpoint" {
			t.Fatal("provider did not receive checkpoint defaults")
		}
		return fixtureModelProvider{agenttest.NewScriptProvider(t, agenttest.TextTurn("done"))}, nil
	}
	cfg.Values["provider"], cfg.Values["model"] = config.Value{}, config.Value{}
	foreign := cfg
	foreign.Paths.Workspace = t.TempDir()
	if other, err := Open(ctx, foreign, opts); err == nil {
		_ = other.Close(ctx)
		t.Fatal("checkpoint opened in a foreign workspace")
	} else if !strings.Contains(err.Error(), "checkpoint belongs to another workspace") || providerStarts != 0 {
		t.Fatalf("workspace was checked after provider activation: %v", err)
	}
	resumed, err := Open(ctx, cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resumed.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	if !slices.Equal(resumed.ContextPaths(), []string{"notes.md"}) {
		t.Fatalf("checkpoint attachments were not restored: %v", resumed.ContextPaths())
	}
	result, err := resumed.Service.Execute(ctx, execution.Request{ID: run.ID, Approve: run.Approval.ID}, nil)
	if err != nil || result.ExitCode != 0 || result.ID != run.ID || result.SessionID != run.SessionID {
		t.Fatalf("resume: %+v, %v", result, err)
	}
	if raw, err := os.ReadFile(filepath.Join(cfg.Paths.Workspace, "approved.txt")); string(raw) != "done" || err != nil {
		t.Fatalf("approved effect did not complete: %q, %v", raw, err)
	}
	if cfg.String("provider") != "" || cfg.String("model") != "" {
		t.Fatal("checkpoint settings mutated caller configuration")
	}
}

func TestCompositionRollsBackPartialStart(t *testing.T) {
	for _, cancelStart := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelStart), func(t *testing.T) {
			cfg := compositionConfig(t)
			var closed []string
			opts := exampleOptions(t, exampleStorage(func(name string) { closed = append(closed, name) }))
			start, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("fixture pack failed")
			if cancelStart {
				failure = context.Canceled
			}
			opts.ToolPacks = []ToolPack{{ID: "fixture", Register: func(_ context.Context, registry *tools.Registry, _ ToolPackContext) error {
				if err := registry.AddCloser(func(context.Context) error {
					closed = append(closed, "pack")
					return nil
				}); err != nil {
					return err
				}
				if cancelStart {
					cancel()
					return nil
				}
				return failure
			}}}
			if runtime, err := Open(start, cfg, opts); !errors.Is(err, failure) {
				if runtime != nil {
					_ = runtime.Close(context.Background())
				}
				t.Fatalf("expected failure %v, got %v", failure, err)
			}
			if !slices.Equal(closed, []string{"pack", "records", "checkpoints"}) {
				t.Fatalf("cleanup order after startup failure: %v", closed)
			}
		})
	}
}

func TestCompositionRejectsInvalidGraphWithoutStartingResources(t *testing.T) {
	cases := []struct {
		name   string
		change func(*component.Descriptor, compositionKeys)
		want   string
	}{
		{"missing dependency", func(d *component.Descriptor, _ compositionKeys) {
			d.Requires = []component.ServiceRef{{ID: "test.missing", Version: "1.0.0"}}
		}, "test.missing"},
		{"incompatible version", func(d *component.Descriptor, _ compositionKeys) {
			d.Provides[0].Version = "2.0.0"
		}, "opera.store"},
		{"cyclic dependency", func(d *component.Descriptor, k compositionKeys) {
			d.Requires = []component.ServiceRef{component.Reference(k.runner)}
		}, "cycle"},
		{"duplicate service", func(d *component.Descriptor, k compositionKeys) {
			d.Provides = append(d.Provides, component.Reference(k.runner))
		}, "opera.runner"},
		{"missing version", func(d *component.Descriptor, _ compositionKeys) {
			d.Version = ""
		}, "version"},
		{"missing capability", func(d *component.Descriptor, _ compositionKeys) {
			d.Capabilities = nil
		}, "capabilities"},
		{"ID mismatch", func(d *component.Descriptor, _ compositionKeys) {
			d.ID = "unexpected"
		}, "unexpected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := compositionConfig(t)
			registration := exampleStorage(func(string) {})
			build := registration.Build
			var factoryCalls atomic.Int32
			registration.Build = func(input ComponentBuildInput) (component.Definition, error) {
				definition, err := build(input)
				tc.change(&definition.Descriptor, input.keys)
				factory := definition.Factory
				definition.Factory = func() (component.Component, error) {
					factoryCalls.Add(1)
					return factory()
				}
				return definition, err
			}
			opts := exampleOptions(t, registration)
			_, err := ResolveComposition(cfg, opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "headless") {
				t.Fatalf("want contextual %q error, got %v", tc.want, err)
			}
			if _, err := os.Stat(cfg.Paths.Data); !os.IsNotExist(err) || factoryCalls.Load() != 0 {
				t.Fatalf("invalid composition acquired resources: %v", err)
			}
		})
	}
}

func TestCompositionRejectsInvalidSlotAndMissingTerminalCapability(t *testing.T) {
	cfg := compositionConfig(t)
	for _, selections := range []map[ComponentSlot]ComponentID{
		{StorageSlot: "missing"}, {StorageSlot: "runner"}, {"provider": "storage"},
	} {
		if _, err := ResolveComposition(cfg, Options{Selections: selections}); err == nil {
			t.Fatalf("accepted selections %+v", selections)
		}
	}

	catalog, err := DefaultComponentCatalog()
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]ComponentRegistration, 0, len(catalog.entries))
	for id, entry := range catalog.entries {
		if id == "application" {
			build := entry.Build
			entry.Build = func(input ComponentBuildInput) (component.Definition, error) {
				def, err := build(input)
				def.Descriptor.Capabilities = []component.Capability{{ID: "other"}}
				return def, err
			}
		}
		entries = append(entries, entry)
	}
	catalog, err = NewComponentCatalog(entries...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveComposition(cfg, Options{Catalog: catalog}); err == nil || !strings.Contains(err.Error(), `capability "execute"`) {
		t.Fatalf("accepted missing terminal capability: %v", err)
	}
}

func TestStaticCompositionFailureKeepsPreviousRuntime(t *testing.T) {
	ctx := context.Background()
	cfg := compositionConfig(t)
	rejected := errors.New("fixture rejected candidate")
	opts := Options{ProviderFactory: func(_ context.Context, candidate config.Resolved) (providers.Provider, error) {
		if candidate.String("model") == "rejected" {
			return nil, rejected
		}
		return fixtureModelProvider{agenttest.NewScriptProvider(t, agenttest.TextTurn("previous runtime still runs"))}, nil
	}}
	r, err := Open(ctx, cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	previous := r.Harness
	if _, err := r.Command(ctx, "/model openai rejected"); !errors.Is(err, rejected) {
		t.Fatalf("candidate failure: %v", err)
	}
	if r.Harness != previous || r.Service.Model != "fixture" {
		t.Fatal("failed candidate replaced the runtime")
	}

	err = r.Service.AtReconfigurationPoint(ctx, "", func() error {
		result, err := r.Service.Execute(ctx, execution.Request{Prompt: "cannot start during reconfiguration"}, nil)
		if err == nil || result.ExitCode != 1 || !strings.Contains(err.Error(), "composition change") {
			t.Errorf("execution entered during a reconfiguration: %+v, %v", result, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := r.Service.Execute(ctx, execution.Request{Prompt: "verify the previous runtime"}, nil)
	if err != nil || result.ExitCode != 0 || result.Text != "previous runtime still runs" {
		t.Fatalf("previous runtime did not recover admission: %+v, %v", result, err)
	}
}
