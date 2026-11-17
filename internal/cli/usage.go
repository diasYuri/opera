package cli

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
)

func (a App) usage(opts options, cfg config.Resolved, jsonl bool) (code int) {
	args := opts.args[1:]
	scope, id, turn := "workspace", "", ""
	if len(args) > 0 {
		scope = args[0]
	}
	if len(args) > 1 {
		id = args[1]
	}
	if len(args) > 2 {
		turn = args[2]
	}
	if len(args) > 3 || scope != "workspace" && id == "" {
		return a.fail(2, errors.New("usage: opera usage [workspace|session <id>|run <id>|agent <id>|turn <run-id> <turn-id>]"), jsonl)
	}
	ctx := context.Background()
	r, err := core.Open(ctx, cfg, core.Options{Inspection: true})
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	defer func() {
		if err := r.Close(ctx); err != nil {
			code = a.fail(1, err, jsonl)
		}
	}()
	report, err := r.Service.Usage(ctx, scope, id, turn)
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	if jsonl {
		return a.emit("usage.report", report)
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return a.fail(1, err, false)
	}
	return a.writeText(string(raw) + "\n")
}
