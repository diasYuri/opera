package core

import (
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/opera/internal/modules/extensions"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/component"
)

type compositionKeys struct {
	records    component.Key[execution.Records]
	store      component.Key[execution.ExecutionStore]
	runner     component.Key[*agent.AgentRunner]
	service    component.Key[*execution.Service]
	settings   component.Key[startupSettings]
	state      component.Key[*executionState]
	extensions component.Key[*extensions.Manager]
}

func newCompositionKeys() compositionKeys {
	return compositionKeys{
		records:    component.NewKey[execution.Records]("opera.records", "1.0.0"),
		store:      component.NewKey[execution.ExecutionStore]("opera.store", "1.0.0"),
		runner:     component.NewKey[*agent.AgentRunner]("opera.runner", "1.0.0"),
		service:    component.NewKey[*execution.Service]("opera.execution", "1.0.0"),
		settings:   component.NewKey[startupSettings]("opera.startup-settings", "1.0.0"),
		state:      component.NewKey[*executionState]("opera.execution-state", "1.0.0"),
		extensions: component.NewKey[*extensions.Manager]("opera.extensions", "1.0.0"),
	}
}
