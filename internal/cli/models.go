package cli

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/harness"
)

func (a App) models(opts options, cfg config.Resolved, jsonl bool) int {
	if len(opts.args) > 1 {
		return a.fail(2, errors.New("usage: opera models [--provider <id>]"), jsonl)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	catalog, err := harness.Models(ctx, cfg, a.credentialLookup(cfg))
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	if jsonl {
		return a.emit("models.catalog", catalog)
	}
	raw, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return a.fail(1, err, false)
	}
	return a.writeText(string(raw) + "\n")
}
