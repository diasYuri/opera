package core

import (
	"context"

	"github.com/diasYuri/y/pkg/component"
)

func (b *compositionBuilder) inspection() component.Definition {
	return componentDefinition("inspection", "inspect-records", []component.ServiceRef{component.Reference(b.keys.service)}, []component.ServiceRef{component.Reference(b.keys.settings), component.Reference(b.keys.store), component.Reference(b.keys.records)}, func(_ context.Context, scope *component.Scope) error {
		service, _, err := b.baseExecutionService(scope)
		if err != nil {
			return err
		}
		if err := scope.Track(service.Close); err != nil {
			return err
		}
		return component.Provide(scope, b.keys.service, service)
	})
}
