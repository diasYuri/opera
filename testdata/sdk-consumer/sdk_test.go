package consumer_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/extensions/task-manager"
	"github.com/diasYuri/y/pkg/memory/filesystem"
	"github.com/diasYuri/y/pkg/providers/oauth"
	"github.com/diasYuri/y/pkg/runtime"
	"github.com/diasYuri/y/pkg/storage/sqlite"
)

// This independent module has no Opera imports, no local replacement and no
// dependency on the SDK checkout. CI resolves the published tag with GOWORK=off.
func TestPublishedLocalSDKContracts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := sqlite.Open(ctx, filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	state := agent.NewSnapshot("consumer", agent.AgentSnapshot{RunID: "consumer"})
	if err := store.Save(ctx, "consumer", 0, state); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx, "consumer"); err != nil {
		t.Fatal(err)
	}
	manager := taskmanager.New()
	if _, err := manager.Create(taskmanager.Task{Name: "public API"}); err != nil {
		t.Fatal(err)
	}
	restored := taskmanager.New()
	if err := restored.Restore(manager.Snapshot()); err != nil {
		t.Fatal(err)
	}
	memory, err := filesystem.NewFilesystemStore(filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	if err := memory.CheckReadiness(ctx); err != nil {
		t.Fatal(err)
	}
	client := oauth.Client{Config: oauth.Config{ClientID: "consumer"}, Notify: func(context.Context, oauth.Notice) error { return nil }}
	if client.Config.ClientID == "" {
		t.Fatal("public OAuth contract missing")
	}
	runner := agent.NewAgentRunner(nil, nil)
	unsubscribe := runner.Subscribe(func(agent.EventEnvelope) {})
	unsubscribe()
	if err := runner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.NewHarness()
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := harness.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
