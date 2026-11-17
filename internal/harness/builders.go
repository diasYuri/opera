package harness

import (
	"context"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/config"
	operaext "github.com/diasYuri/opera/internal/extensions"
	"github.com/diasYuri/opera/internal/skills"
	"github.com/diasYuri/y/pkg/component"
)

type compositionBuilder struct {
	cfg          config.Resolved
	opts         *Options
	keys         compositionKeys
	guard        *app.Guard
	skillManager *skills.Manager
	questions    *app.QuestionBroker
	contextPaths *contextPathState
	extensions   *operaext.Manager
}

func componentDefinition(id string, provides, requires []component.ServiceRef, start func(context.Context, *component.Scope) error) component.Definition {
	return component.Define(component.Descriptor{ID: id, Version: "1.0.0", Provides: provides, Requires: requires}, func() (component.Component, error) { return component.ComponentFunc(start), nil })
}
