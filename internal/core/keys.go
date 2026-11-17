package core

import (
	operasqlite "github.com/diasYuri/opera/internal/adapters/persistence/sqlite"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/opera/internal/modules/extensions"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/component"
	ysqlite "github.com/diasYuri/y/pkg/storage/sqlite"
)

type compositionKeys struct {
	records    component.Key[*operasqlite.Store]
	store      component.Key[*ysqlite.Store]
	runner     component.Key[*agent.AgentRunner]
	service    component.Key[*execution.Service]
	work       component.Key[*execution.Work]
	extensions component.Key[*extensions.Manager]
}

func newCompositionKeys() compositionKeys {
	return compositionKeys{
		records:    component.NewKey[*operasqlite.Store]("opera.records", "1.0.0"),
		store:      component.NewKey[*ysqlite.Store]("opera.store", "1.0.0"),
		runner:     component.NewKey[*agent.AgentRunner]("opera.runner", "1.0.0"),
		service:    component.NewKey[*execution.Service]("opera.execution", "1.0.0"),
		work:       component.NewKey[*execution.Work]("opera.work", "1.0.0"),
		extensions: component.NewKey[*extensions.Manager]("opera.extensions", "1.0.0"),
	}
}
