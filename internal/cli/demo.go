package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/diasYuri/opera/internal/adapters/platform"
	"github.com/diasYuri/opera/internal/demo"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
)

func (a App) demo(opts options, jsonl bool) int {
	mode := "run"
	if len(opts.args) > 1 {
		mode = opts.args[1]
	}
	if len(opts.args) > 2 {
		return a.fail(2, errors.New("usage: opera demo [chat|run|worker|headless]"), jsonl)
	}
	if mode == "headless" {
		binary, err := os.Executable()
		if err != nil {
			return a.fail(1, err, jsonl)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		evidence, err := demo.ProcessProof(ctx, binary)
		if err != nil {
			return a.fail(1, err, jsonl)
		}
		if jsonl {
			return a.emit("demo.verified", evidence)
		}
		return a.writeText("OPERA_DEMO_OK: headless exit 3, approved resume and denied effect verified across CLI processes.\n")
	}
	root, err := os.MkdirTemp("", "opera-demo-")
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	defer func() { _ = os.RemoveAll(root) }()
	cfg, err := config.Load(config.Options{Paths: platform.Paths{Workspace: filepath.Join(root, "workspace"), Data: filepath.Join(root, "data")}, Flags: map[string]string{"provider": "openai", "model": "opera-offline-demo", "policy": "ask"}})
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	binary, err := os.Executable()
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	evidence, err := demo.Run(ctx, cfg, mode, demo.Provider{Command: binary, Args: []string{"--version"}}, func(event execution.Event) error {
		if jsonl {
			return json.NewEncoder(a.Out).Encode(event)
		}
		return nil
	})
	if err != nil {
		return a.fail(1, err, jsonl)
	}
	if jsonl {
		return a.emit("demo.verified", evidence)
	}
	return a.writeText("OPERA_DEMO_OK: " + mode + " verified planning, child read, approval/denial, command, checkpoint handoff and usage in a disposable workspace.\n")
}
