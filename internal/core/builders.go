package core

import (
	"context"

	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/y/pkg/component"
)

type compositionBuilder struct {
	cfg    config.Resolved
	opts   Options
	keys   compositionKeys
	packs  []ToolPack
	search searchSelection
}

func componentDefinition(id, capability string, provides, requires []component.ServiceRef, start func(context.Context, *component.Scope) error) component.Definition {
	descriptor := component.Descriptor{
		ID: id, Version: "1.0.0", Provides: provides, Requires: requires,
		Capabilities: []component.Capability{{ID: capability}},
	}
	return component.Define(descriptor, func() (component.Component, error) {
		return component.ComponentFunc(start), nil
	})
}
