package core

import (
	"regexp"

	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/y/pkg/component"
)

type CompositionSummary struct {
	SearchProviders []string            `json:"search_provider_ids,omitempty"`
	Profile         ProfileID           `json:"profile"`
	Components      []SelectedComponent `json:"components"`
	ToolPacks       []string            `json:"tool_packs,omitempty"`
	ProviderIDs     []string            `json:"provider_ids,omitempty"`
	Provider        string              `json:"provider,omitempty"`
	Search          string              `json:"search,omitempty"`
}

type SelectedComponent struct {
	ID           string                         `json:"id"`
	Slot         ComponentSlot                  `json:"slot,omitempty"`
	Version      string                         `json:"version"`
	Provides     []component.ServiceRef         `json:"provides,omitempty"`
	Requires     []component.ServiceRef         `json:"requires,omitempty"`
	Bindings     []component.ServiceRequirement `json:"bindings,omitempty"`
	Capabilities []string                       `json:"capabilities"`
	Config       []ConfigProvenance             `json:"config,omitempty"`
}

type ConfigProvenance struct {
	Key    string `json:"key"`
	Source string `json:"source"`
}

var providerDiagnosticID = regexp.MustCompile(`^(custom:)?[a-z0-9][a-z0-9._-]{0,63}$`)

// DescribeComposition describes the same graph used by Open and permits a
// missing provider/model so local diagnostics work before onboarding.
func DescribeComposition(cfg config.Resolved, opts Options) (CompositionSummary, error) {
	plan, err := resolveComposition(cfg, opts, true)
	if err != nil {
		return CompositionSummary{}, err
	}
	return plan.Summary, nil
}

func summarizeComposition(profile ProfileID, descriptors []component.Descriptor, catalog *ComponentCatalog, input ComponentBuildInput) CompositionSummary {
	out := CompositionSummary{Profile: profile}
	for _, descriptor := range descriptors {
		entry := catalog.entries[ComponentID(descriptor.ID)]
		item := SelectedComponent{
			ID: descriptor.ID, Slot: entry.Slot, Version: descriptor.Version,
			Provides: descriptor.Provides, Requires: descriptor.Requires, Bindings: descriptor.Requirements,
		}
		for _, capability := range descriptor.Capabilities {
			item.Capabilities = append(item.Capabilities, capability.ID)
		}
		for _, key := range entry.ConfigKeys {
			item.Config = append(item.Config, ConfigProvenance{Key: key, Source: provenanceSource(input.Config.Values[key].Source)})
		}
		if descriptor.ID == "tools" {
			for _, option := range config.WebSearchOptions(input.search.provider) {
				item.Config = append(item.Config, ConfigProvenance{Key: option.Key, Source: provenanceSource(input.Config.Values[option.Key].Source)})
			}
		}
		out.Components = append(out.Components, item)
	}
	if profile == ProfileInspection {
		return out
	}
	out.Search = input.search.id
	provider := input.Config.String("provider")
	switch {
	case input.Options.ResumeID != "":
		out.Provider, out.Search = "deferred-to-checkpoint", "deferred-to-checkpoint"
	case input.Options.Management:
		out.Provider = "opera-admin"
	case input.Options.ProviderFactory != nil:
		out.Provider = "provided"
	case input.Options.ProviderCatalog.Has(provider) && providerDiagnosticID.MatchString(provider):
		out.Provider = provider
	default:
		out.Provider = "unconfigured"
	}
	for _, id := range input.Options.ProviderCatalog.IDs() {
		if providerDiagnosticID.MatchString(id) {
			out.ProviderIDs = append(out.ProviderIDs, id)
		}
	}
	for _, entry := range webSearchEntries(input.Options.ProviderCatalog) {
		if providerDiagnosticID.MatchString(entry.ID) {
			out.SearchProviders = append(out.SearchProviders, entry.ID)
		}
	}
	for _, pack := range input.packs {
		if pack.ID != "web-search" || out.Search != "disabled" {
			out.ToolPacks = append(out.ToolPacks, pack.ID)
		}
	}
	return out
}

func provenanceSource(source string) string {
	switch source {
	case "default", "user", "user-workspace", "project", "environment", "flag", "interactive", "checkpoint", "management":
		return source
	default:
		return "unspecified"
	}
}
