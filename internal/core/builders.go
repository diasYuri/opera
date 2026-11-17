package core

import (
	"context"

	"github.com/diasYuri/opera/internal/adapters/skills/filesystem"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	operaext "github.com/diasYuri/opera/internal/modules/extensions"
	"github.com/diasYuri/y/pkg/component"
)

type compositionBuilder struct {
	cfg          config.Resolved
	opts         *Options
	keys         compositionKeys
	guard        *execution.Guard
	skillManager *skills.Manager
	questions    *execution.QuestionBroker
	contextPaths *contextPathState
	extensions   *operaext.Manager
}

func componentDefinition(id string, provides, requires []component.ServiceRef, start func(context.Context, *component.Scope) error) component.Definition {
	return component.Define(component.Descriptor{ID: id, Version: "1.0.0", Provides: provides, Requires: requires}, func() (component.Component, error) { return component.ComponentFunc(start), nil })
}
