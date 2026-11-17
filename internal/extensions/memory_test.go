package extensions

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	ycontext "github.com/diasYuri/y/pkg/context"
	memoryext "github.com/diasYuri/y/pkg/extensions/memory"
	"github.com/diasYuri/y/pkg/memory"
	"github.com/diasYuri/y/pkg/tools"
)

func TestMemoryFailOpenDoesNotHideWriteOrReadinessFailure(t *testing.T) {
	for _, failOpen := range []bool{false, true} {
		t.Run(fmt.Sprint(failOpen), func(t *testing.T) {
			ctx := context.Background()
			cfg := testConfig(t)
			if err := Save(cfg, Settings{Version: 1, Memory: Memory{Enabled: true, FailOpen: failOpen}}); err != nil {
				t.Fatal(err)
			}
			reg := tools.NewRegistry()
			defer func() { _ = reg.Close(ctx) }()
			m, err := New(ctx, cfg, reg, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = m.Close(ctx) }()
			root := filepath.Join(cfg.Paths.Data, "memory", fmt.Sprintf("%x", sha256.Sum256([]byte(cfg.Paths.Workspace))))
			if err := os.RemoveAll(root); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(root, []byte("unavailable backend"), 0600); err != nil {
				t.Fatal(err)
			}
			_, readErr := m.Memory.Source().Load(ctx, ycontext.Request{WorkspaceID: cfg.Paths.Workspace, Query: "test"})
			if failOpen && readErr != nil || !failOpen && readErr == nil {
				t.Fatal("incorrect recall behavior", readErr)
			}
			value := memory.Memory{Kind: memory.KindProjectFact, Scope: ycontext.ScopeWorkspace, Summary: "test", Content: "test"}
			if err := m.Memory.Store().Save(ctx, value); err == nil {
				t.Fatal("write error swallowed")
			}
			if m.Memory.PersistenceStatus() != memoryext.PersistenceStatusDegraded {
				t.Fatal("missing degraded persistence state")
			}
			other := tools.NewRegistry()
			defer func() { _ = other.Close(ctx) }()
			if next, err := New(ctx, cfg, other, nil, false); err == nil {
				_ = next.Close(ctx)
				t.Fatal("readiness failure ignored")
			}
		})
	}
}
