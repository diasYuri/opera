package core

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/y/pkg/component"
)

type ComponentID string
type ComponentSlot string

const StorageSlot ComponentSlot = "storage"

type ComponentBuildInput struct {
	Config  config.Resolved
	Options Options
	keys    compositionKeys
	packs   []ToolPack
	search  searchSelection
}

type ComponentRegistration struct {
	ID         ComponentID
	Slot       ComponentSlot
	ConfigKeys []string
	Build      func(ComponentBuildInput) (component.Definition, error)
}

type ComponentCatalog struct {
	entries map[ComponentID]ComponentRegistration
}

var compositionID = regexp.MustCompile(`^[a-z][a-z0-9._-]*$`)

func NewComponentCatalog(registrations ...ComponentRegistration) (*ComponentCatalog, error) {
	entries := make(map[ComponentID]ComponentRegistration, len(registrations))
	fields := make(map[string]bool)
	for _, field := range config.Schema() {
		fields[field.Key] = true
	}
	for _, registration := range registrations {
		if !compositionID.MatchString(string(registration.ID)) || registration.Build == nil {
			return nil, fmt.Errorf("component %q requires a valid ID and definition builder", registration.ID)
		}
		if registration.Slot != "" && registration.Slot != StorageSlot {
			return nil, fmt.Errorf("component %q declares unknown slot %q", registration.ID, registration.Slot)
		}
		if _, exists := entries[registration.ID]; exists {
			return nil, fmt.Errorf("duplicate component %q", registration.ID)
		}
		for _, key := range registration.ConfigKeys {
			if !fields[key] {
				return nil, fmt.Errorf("component %q references unknown configuration key %q; declare it in the configuration schema", registration.ID, key)
			}
		}
		registration.ConfigKeys = slices.Clone(registration.ConfigKeys)
		entries[registration.ID] = registration
	}
	return &ComponentCatalog{entries: entries}, nil
}

func DefaultComponentCatalog(additional ...ComponentRegistration) (*ComponentCatalog, error) {
	entry := func(id ComponentID, slot ComponentSlot, build func(*compositionBuilder) component.Definition, keys ...string) ComponentRegistration {
		return ComponentRegistration{ID: id, Slot: slot, ConfigKeys: keys, Build: func(input ComponentBuildInput) (component.Definition, error) {
			builder := &compositionBuilder{cfg: input.Config, opts: input.Options, keys: input.keys, packs: input.packs, search: input.search}
			return build(builder), nil
		}}
	}
	return NewComponentCatalog(append([]ComponentRegistration{
		entry("storage", StorageSlot, (*compositionBuilder).storage),
		entry("startup-settings", "", (*compositionBuilder).settings, "provider", "model", "thinking"),
		entry("execution-state", "", (*compositionBuilder).executionState, "policy", "agents.max_concurrent", "agents.max_depth", "limits.worker_tokens"),
		entry("provider", "", (*compositionBuilder).provider, "provider", "model", "base_url", "credential_ref", "accounting.prices_file"),
		entry("tools", "", (*compositionBuilder).tools, "policy", "limits.output_bytes", "extensions.file", "search.provider"),
		entry("runner", "", (*compositionBuilder).runner, "thinking", "limits.process_timeout", "limits.context_tokens", "limits.attachment_bytes"),
		entry("application", "", (*compositionBuilder).application, "limits.context_tokens", "limits.attachment_bytes"),
		entry("inspection", "", (*compositionBuilder).inspection, "provider", "model", "policy", "agents.max_concurrent", "agents.max_depth", "limits.worker_tokens"),
	}, additional...)...)
}
