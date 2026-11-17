package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/harness"
	"strconv"
)

func (a App) sessions(opts options, cfg config.Resolved, jsonl bool) (code int) {
	ctx := context.Background()
	runtime, err := harness.Open(ctx, cfg, harness.Options{Inspection: true})
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	defer func() {
		if err := runtime.Close(ctx); err != nil {
			code = a.fail(1, err, jsonl)
		}
	}()
	args := opts.args[1:]
	if len(args) == 0 {
		args = []string{"list"}
	}
	var value any
	switch args[0] {
	case "list":
		offset, limit := 0, 20
		if len(args) > 3 {
			return a.fail(2, errors.New("usage: opera sessions list [offset] [limit]"), jsonl)
		}
		if len(args) > 1 {
			offset, err = strconv.Atoi(args[1])
			if err != nil {
				return a.fail(2, err, jsonl)
			}
		}
		if len(args) > 2 {
			limit, err = strconv.Atoi(args[2])
			if err != nil {
				return a.fail(2, err, jsonl)
			}
		}
		value, err = runtime.Service.Sessions(ctx, offset, limit)
	case "events", "operations", "decisions", "output":
		if len(args) != 2 {
			return a.fail(2, errors.New("record inspection requires a run ID"), jsonl)
		}
		filter := app.InspectFilter{Type: opts.eventType, Tool: opts.tool, ID: opts.operationID, Limit: 100}
		if opts.limit != "" {
			filter.Limit, err = strconv.Atoi(opts.limit)
			if err != nil {
				return a.fail(2, err, jsonl)
			}
		}
		if opts.after != "" {
			filter.After, err = strconv.ParseUint(opts.after, 10, 64)
			if err != nil {
				return a.fail(2, err, jsonl)
			}
		}
		value, err = runtime.Service.InspectRecords(ctx, args[1], args[0], filter)
	case "inspect", "export":
		if len(args) != 2 {
			return a.fail(2, errors.New("usage: opera sessions inspect|export <run-id>"), jsonl)
		}
		value, err = runtime.Service.Export(ctx, args[1])
	case "branch":
		if len(args) < 2 || len(args) > 3 {
			return a.fail(2, errors.New("usage: opera sessions branch <run-id> [checkpoint-version]"), jsonl)
		}
		var version uint64
		if len(args) == 3 {
			version, err = strconv.ParseUint(args[2], 10, 64)
			if err != nil {
				return a.fail(2, err, jsonl)
			}
		}
		id, e := runtime.Service.Fork(ctx, args[1], version)
		value = map[string]string{"run_id": id}
		err = e
	default:
		return a.fail(2, errors.New("usage: opera sessions list|inspect|export|branch"), jsonl)
	}
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	if jsonl {
		return a.emit("sessions."+args[0], value)
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return a.fail(1, err, false)
	}
	return a.writeText(string(raw) + "\n")
}
func (a App) compact(opts options, cfg config.Resolved, jsonl bool) (code int) {
	if len(opts.args) != 2 {
		return a.fail(2, errors.New("usage: opera compact <run-id>"), jsonl)
	}
	ctx := context.Background()
	runtime, err := harness.Open(ctx, cfg, harness.Options{ProviderCatalog: a.ProviderCatalog, ResumeID: opts.args[1], Credential: a.credentialLookup(cfg)})
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	defer func() {
		if err := runtime.Close(ctx); err != nil {
			code = a.fail(1, err, jsonl)
		}
	}()
	if err := runtime.Service.Compact(ctx, opts.args[1]); err != nil {
		return a.fail(1, err, jsonl)
	}
	if jsonl {
		return a.emit("session.compacted", map[string]string{"run_id": opts.args[1]})
	}
	return a.writeText(fmt.Sprintf("Compacted %s; the original checkpoint is archived.\n", opts.args[1]))
}
