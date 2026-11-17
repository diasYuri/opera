package harness

import (
	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/extensions"
	"github.com/diasYuri/opera/internal/persistence"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/storage/sqlite"
)

type compositionKeys struct {
	records    component.Key[*persistence.Store]
	store      component.Key[*sqlite.Store]
	runner     component.Key[*agent.AgentRunner]
	service    component.Key[*app.Service]
	work       component.Key[*app.Work]
	extensions component.Key[*extensions.Manager]
}

func newCompositionKeys() compositionKeys {
	return compositionKeys{
		records:    component.NewKey[*persistence.Store]("opera.records", "1.0.0"),
		store:      component.NewKey[*sqlite.Store]("opera.store", "1.0.0"),
		runner:     component.NewKey[*agent.AgentRunner]("opera.runner", "1.0.0"),
		service:    component.NewKey[*app.Service]("opera.execution", "1.0.0"),
		work:       component.NewKey[*app.Work]("opera.work", "1.0.0"),
		extensions: component.NewKey[*extensions.Manager]("opera.extensions", "1.0.0"),
	}
}
