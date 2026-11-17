package core

import (
	"context"
	"encoding/json"

	operasqlite "github.com/diasYuri/opera/internal/adapters/persistence/sqlite"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/component"
	"github.com/diasYuri/y/pkg/storage/sqlite"
	"path/filepath"
	"strings"
)

func (b *compositionBuilder) storage() component.Definition {
	cfg, opts := b.cfg, b.opts

	return componentDefinition("storage", []component.ServiceRef{component.Reference(b.keys.store), component.Reference(b.keys.records)}, nil, func(ctx context.Context, scope *component.Scope) error {
		store, err := sqlite.Open(ctx, filepath.Join(cfg.Paths.Data, "opera.db"))
		if err != nil {
			return err
		}
		if err := scope.Track(func(context.Context) error { return store.Close() }); err != nil {
			_ = store.Close()
			return err
		}
		records, err := operasqlite.Open(ctx, filepath.Join(cfg.Paths.Data, "opera.db"), cfg.Paths.Workspace)
		if err != nil {
			return err
		}
		if err := scope.Track(func(context.Context) error { return records.Close() }); err != nil {
			_ = records.Close()
			return err
		}
		if err := component.Provide(scope, b.keys.records, records); err != nil {
			return err
		}
		if opts.ResumeID != "" {
			raw, e := store.GetMetadata(ctx, "run:"+opts.ResumeID)
			if e != nil {
				return e
			}
			var meta execution.Metadata
			if e := json.Unmarshal(raw, &meta); e != nil {
				return e
			}
			if meta.Management {
				opts.Management = true
				var action Action
				if json.Unmarshal([]byte(meta.InitialPrompt), &action) == nil {
					opts.Offline = strings.HasPrefix(action.Tool, "opera_")
				}
			}
		}
		return component.Provide(scope, b.keys.store, store)
	})
}
