package demo_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/diasYuri/opera/internal/adapters/platform"
	"github.com/diasYuri/opera/internal/demo"
	"github.com/diasYuri/opera/internal/modules/configuration"
)

func TestFinalCodingAcceptanceAcrossSharedModes(t *testing.T) {
	for _, mode := range []string{"chat", "run", "worker"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			cfg, err := config.Load(config.Options{Paths: platform.Paths{Workspace: filepath.Join(root, "workspace"), Data: filepath.Join(root, "data")}, Flags: map[string]string{"provider": "openai", "model": "opera-offline-demo", "policy": "ask"}})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			evidence, err := demo.Run(ctx, cfg, mode, demo.Provider{Command: "go", Args: []string{"version"}}, nil)
			if err != nil || !evidence.Verified {
				t.Fatal(evidence, err)
			}
		})
	}
}
