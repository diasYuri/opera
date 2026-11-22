package core

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/y/pkg/component"
	yruntime "github.com/diasYuri/y/pkg/runtime"
)

type CompositionPlan struct {
	ProfileID ProfileID
	Summary   CompositionSummary
	keys      compositionKeys
	harness   *yruntime.Harness
}

// ResolveComposition builds and validates a graph without opening resources.
func ResolveComposition(cfg config.Resolved, opts Options) (*CompositionPlan, error) {
	return resolveComposition(cfg, opts, false)
}

func resolveComposition(cfg config.Resolved, opts Options, allowUnconfigured bool) (*CompositionPlan, error) {
	profile, err := selectRuntimeProfile(opts)
	if err != nil {
		return nil, err
	}
	plan, err := buildComposition(profile, cfg, opts, allowUnconfigured)
	if err != nil {
		return nil, fmt.Errorf("composition profile %q: %w", profile.id, err)
	}
	return plan, nil
}

func buildComposition(profile runtimeProfile, cfg config.Resolved, opts Options, allowUnconfigured bool) (*CompositionPlan, error) {
	cfg = cloneConfig(cfg)
	opts.Profile = profile.id
	opts.Selections = maps.Clone(opts.Selections)
	opts.ContextPaths = slices.Clone(opts.ContextPaths)
	if opts.ResponseFormat != nil {
		format := *opts.ResponseFormat
		format.Schema = slices.Clone(format.Schema)
		opts.ResponseFormat = &format
	}
	if err := validateResponseFormat(opts.ResponseFormat); err != nil {
		return nil, err
	}
	switch opts.Thinking {
	case "", "off", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return nil, errors.New("unsupported thinking level")
	}

	input := ComponentBuildInput{Config: cfg, Options: opts, keys: newCompositionKeys()}
	if profile.id != ProfileInspection {
		if input.Options.ProviderCatalog == nil {
			var err error
			input.Options.ProviderCatalog, err = DefaultCatalog(cfg.CustomProviders...)
			if err != nil {
				return nil, err
			}
		}
		var err error
		input.packs, err = selectToolPacks(opts.ToolPacks, opts.DisabledToolPacks)
		if err != nil {
			return nil, err
		}
		if opts.ResumeID == "" {
			if err := validateExecutionSelection(cfg, input.Options, allowUnconfigured); err != nil {
				return nil, err
			}
			input.search, err = selectSearch(cfg, input.Options)
			if err != nil {
				return nil, err
			}
			if !allowUnconfigured {
				if err := input.search.checkInputs(input.Options); err != nil {
					return nil, err
				}
			}
		}
	}
	catalog := opts.Catalog
	if catalog == nil {
		var err error
		catalog, err = DefaultComponentCatalog()
		if err != nil {
			return nil, err
		}
	}
	definitions := make([]component.Definition, 0, len(profile.components))
	var terminal bool
	for _, selection := range profile.components {
		registration, ok := catalog.entries[selection.id]
		if !ok {
			return nil, fmt.Errorf("component %q is not registered", selection.id)
		}
		if registration.Slot != selection.slot {
			return nil, fmt.Errorf("component %q belongs to slot %q, cannot use in slot %q", selection.id, registration.Slot, selection.slot)
		}
		definition, err := registration.Build(input)
		if err != nil {
			return nil, fmt.Errorf("build component %q: %w", selection.id, err)
		}
		if definition.Descriptor.ID != string(selection.id) {
			return nil, fmt.Errorf("component %q built descriptor for %q", selection.id, definition.Descriptor.ID)
		}
		if len(definition.Descriptor.Capabilities) == 0 {
			return nil, fmt.Errorf("component %q must declare its capabilities in the Y descriptor", selection.id)
		}
		if slices.Contains(definition.Descriptor.Provides, component.Reference(input.keys.service)) {
			terminal = slices.ContainsFunc(definition.Descriptor.Capabilities, func(c component.Capability) bool { return c.ID == profile.capability })
		}
		definitions = append(definitions, definition)
	}
	// Graph conflicts, service bindings, versions and topological order are Y's.
	harness, err := yruntime.NewHarness(definitions...)
	if err != nil {
		return nil, err
	}
	if !terminal {
		return nil, fmt.Errorf("execution service %q must declare capability %q", component.Reference(input.keys.service).ID, profile.capability)
	}
	return &CompositionPlan{
		ProfileID: profile.id, keys: input.keys, harness: harness,
		Summary: summarizeComposition(profile.id, harness.Descriptors(), catalog, input),
	}, nil
}

func validateExecutionSelection(cfg config.Resolved, opts Options, allowUnconfigured bool) error {
	if opts.Management {
		return nil
	}
	id := cfg.String("provider")
	if id != "" && opts.ProviderFactory == nil && !opts.ProviderCatalog.Has(id) {
		return fmt.Errorf("provider %q is not registered", id)
	}
	if !allowUnconfigured && (id == "" || cfg.String("model") == "") {
		return errors.New("choose a provider and model before execution; use opera onboarding or --provider/--model")
	}
	return nil
}

func (p *CompositionPlan) runtime() (*Runtime, error) {
	service, err := yruntime.Resolve(p.harness, p.keys.service)
	if err != nil {
		return nil, err
	}
	settings, err := yruntime.Resolve(p.harness, p.keys.settings)
	if err != nil {
		return nil, err
	}
	result := &Runtime{Harness: p.harness, Service: service, cfg: settings.cfg, opts: settings.opts}
	if p.ProfileID != ProfileInspection {
		result.Extensions, err = yruntime.Resolve(p.harness, p.keys.extensions)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
