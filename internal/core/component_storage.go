package core

import (
	"context"
	"errors"
	"path/filepath"

	operasqlite "github.com/diasYuri/opera/internal/adapters/persistence/sqlite"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/storage/sqlite"
)

func (b *compositionBuilder) storage() component.Definition {
	cfg := b.cfg

	return componentDefinition("storage", "durable-storage", []component.ServiceRef{component.Reference(b.keys.store), component.Reference(b.keys.records)}, nil, func(ctx context.Context, scope *component.Scope) error {
		store, err := sqlite.Open(ctx, filepath.Join(cfg.Paths.Data, "opera.db"))
		if err != nil {
			return err
		}
		if err := scope.Track(func(context.Context) error { return store.Close() }); err != nil {
			return errors.Join(err, store.Close())
		}
		records, err := operasqlite.Open(ctx, filepath.Join(cfg.Paths.Data, "opera.db"), cfg.Paths.Workspace)
		if err != nil {
			return err
		}
		if err := scope.Track(func(context.Context) error { return records.Close() }); err != nil {
			return errors.Join(err, records.Close())
		}
		if err := component.Provide(scope, b.keys.records, execution.Records(records)); err != nil {
			return err
		}
		return component.Provide(scope, b.keys.store, execution.ExecutionStore(store))
	})
}
