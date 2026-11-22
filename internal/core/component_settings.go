package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/component"
)

// startupSettings holds immutable effective inputs for one Harness activation.
type startupSettings struct {
	cfg  config.Resolved
	opts Options
	// Search follows the configured model provider even in management runs.
	search searchSelection
}

func (b *compositionBuilder) settings() component.Definition {
	requires := []component.ServiceRef{}
	if b.opts.ResumeID != "" {
		requires = append(requires, component.Reference(b.keys.store))
	}
	return componentDefinition("startup-settings", "effective-settings", []component.ServiceRef{component.Reference(b.keys.settings)}, requires, func(ctx context.Context, scope *component.Scope) error {
		cfg, opts := cloneConfig(b.cfg), b.opts
		if opts.ResumeID != "" {
			store, err := component.Require(scope, b.keys.store)
			if err != nil {
				return err
			}
			metadata, err := readCheckpointSettings(ctx, store, opts.ResumeID)
			if err != nil {
				return err
			}
			if metadata.Workspace != cfg.Paths.Workspace {
				return errors.New("checkpoint belongs to another workspace")
			}
			if opts.ResponseFormat == nil {
				opts.ResponseFormat = metadata.ResponseFormat
				if err := validateResponseFormat(opts.ResponseFormat); err != nil {
					return err
				}
			}
			if len(opts.ContextPaths) == 0 {
				opts.ContextPaths = slices.Clone(metadata.ContextPaths)
			}
			for key, value := range map[string]string{"provider": metadata.Provider, "model": metadata.Model} {
				if cfg.String(key) == "" {
					cfg.Values[key] = config.Value{Value: value, Source: "checkpoint"}
				}
			}
			if metadata.Management {
				opts.Management = true
				var action Action
				if json.Unmarshal([]byte(metadata.InitialPrompt), &action) == nil {
					opts.Offline = strings.HasPrefix(action.Tool, "opera_")
				}
			}
		}
		search := b.search
		if opts.Profile != ProfileInspection && opts.ResumeID != "" {
			if err := validateExecutionSelection(cfg, opts, false); err != nil {
				return err
			}
			var err error
			search, err = selectSearch(cfg, opts)
			if err != nil {
				return err
			}
			if err := search.checkInputs(opts); err != nil {
				return err
			}
		}
		if opts.Management && opts.Profile != ProfileInspection {
			opts.Thinking = "off"
			cfg.Values["provider"] = config.Value{Value: "opera-admin", Source: "management"}
			cfg.Values["model"] = config.Value{Value: "tools", Source: "management"}
		}
		return component.Provide(scope, b.keys.settings, startupSettings{cfg: cfg, opts: opts, search: search})
	})
}

func cloneConfig(cfg config.Resolved) config.Resolved {
	values := make(map[string]config.Value, len(cfg.Values))
	maps.Copy(values, cfg.Values)
	cfg.Values = values
	return cfg
}

func readCheckpointSettings(ctx context.Context, store execution.ExecutionStore, id string) (execution.Metadata, error) {
	var metadata execution.Metadata
	raw, err := store.GetMetadata(ctx, "run:"+id)
	if err != nil {
		return metadata, fmt.Errorf("read checkpoint settings: %w", err)
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return metadata, fmt.Errorf("decode checkpoint settings: %w", err)
	}
	return metadata, nil
}

func validateResponseFormat(format *ai.ResponseFormat) error {
	if format == nil {
		return nil
	}
	return ai.ValidateResponseFormat(*format)
}
