package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/opera/internal/adapters/platform"
	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
)

// Opt-in: two tiny text-only requests using the selected subscription account.
// No workspace files, tools, credentials or generated text are printed/sent.
func TestLiveSubscriptionConversationHistory(t *testing.T) {
	a, cfg := subscriptionLiveConfig(t)
	for key, value := range map[string]string{"provider": "openai-subscription", "base_url": "", "policy": "read-only", "extensions.file": "", "accounting.prices_file": ""} {
		cfg.Values[key] = config.Value{Value: value, Source: "live-test"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	runtime, err := core.Open(ctx, cfg, core.Options{Credential: a.credentialLookup(cfg)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := runtime.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	previous := ""
	for _, prompt := range []string{"Reply with only: first", "Reply with only: second"} {
		result, err := runtime.Service.Execute(ctx, execution.Request{Prompt: prompt, ContinueFrom: previous}, nil)
		if err != nil || result.ExitCode != 0 || strings.TrimSpace(result.Text) == "" {
			t.Fatal("subscription returned no text")
		}
		previous = result.ID
	}
}

func subscriptionLiveConfig(t *testing.T) (App, config.Resolved) {
	t.Helper()
	if os.Getenv("OPERA_SUBSCRIPTION_LIVE_TEST") != "1" {
		t.Skip("set OPERA_SUBSCRIPTION_LIVE_TEST=1 to use the connected subscription")
	}
	workspace := os.Getenv("OPERA_SUBSCRIPTION_WORKSPACE")
	if workspace == "" {
		t.Fatal("OPERA_SUBSCRIPTION_WORKSPACE must identify the configured workspace")
	}
	paths, err := platform.Discover(workspace)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Options{Paths: paths, Env: os.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	a := New(os.Stdin, io.Discard, io.Discard, "test")
	return a, cfg
}

func TestLiveSubscriptionHarnessConversation(t *testing.T) {
	a, cfg := subscriptionLiveConfig(t)
	root := t.TempDir()
	cfg.Paths.Workspace = filepath.Join(root, "workspace")
	cfg.Paths.Data = filepath.Join(root, "data")
	if err := os.Mkdir(cfg.Paths.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"provider": "openai-subscription", "policy": "read-only", "extensions.file": "", "accounting.prices_file": "", "base_url": ""} {
		cfg.Values[key] = config.Value{Value: value, Source: "live-test"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	r, err := core.Open(ctx, cfg, core.Options{Credential: a.credentialLookup(cfg)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	previous := ""
	for _, word := range []string{"first", "second", "third"} {
		result, err := r.Service.Execute(ctx, execution.Request{Prompt: "This is a chat integration test. Do not use tools. Reply with exactly this word: " + word, ContinueFrom: previous}, nil)
		if err != nil || result.ExitCode != 0 {
			t.Fatalf("live conversation failed: exit=%d error=%v", result.ExitCode, err)
		}
		if strings.TrimSpace(result.Text) != word {
			t.Fatal("live conversation repeated a prior answer or ignored the latest prompt")
		}
		previous = result.ID
	}
}
