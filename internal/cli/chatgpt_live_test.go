package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/chatgpt"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/harness"
	"github.com/diasYuri/opera/internal/platform"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
)

// Opt-in: two tiny text-only requests using the selected subscription account.
// No workspace files, tools, credentials or generated text are printed/sent.
func TestLiveSubscriptionConversationHistory(t *testing.T) {
	a, cfg := subscriptionLiveConfig(t)
	p := chatgpt.NewProvider(a.credentialLookup(cfg))
	defer func() { _ = p.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	conversation := ai.Context{}
	for _, prompt := range []string{"Reply with only: first", "Reply with only: second"} {
		conversation.Messages = append(conversation.Messages, ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{{Type: ai.ContentText, Text: prompt}}})
		stream, err := p.Stream(ctx, providers.StreamRequest{Model: ai.Model{ID: cfg.String("model")}, Context: conversation})
		if err != nil {
			t.Fatal(err)
		}
		var text strings.Builder
		for {
			event, err := stream.Next(ctx)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				_ = stream.Close()
				t.Fatal(err)
			}
			if delta, ok := event.(ai.TextDelta); ok {
				text.WriteString(delta.Text)
			}
		}
		_ = stream.Close()
		if text.Len() == 0 {
			t.Fatal("subscription returned no text")
		}
		conversation.Messages = append(conversation.Messages, ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{{Type: ai.ContentText, Text: text.String()}}})
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
	for key, value := range map[string]string{"provider": chatgpt.ProviderID, "policy": "read-only", "extensions.file": "", "accounting.prices_file": "", "base_url": ""} {
		cfg.Values[key] = config.Value{Value: value, Source: "live-test"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	r, err := harness.Open(ctx, cfg, harness.Options{Credential: a.credentialLookup(cfg)})
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
		result, err := r.Service.Execute(ctx, app.Request{Prompt: "This is a chat integration test. Do not use tools. Reply with exactly this word: " + word, ContinueFrom: previous}, nil)
		if err != nil || result.ExitCode != 0 {
			t.Fatalf("live conversation failed: exit=%d error=%v", result.ExitCode, err)
		}
		if strings.TrimSpace(result.Text) != word {
			t.Fatal("live conversation repeated a prior answer or ignored the latest prompt")
		}
		previous = result.ID
	}
}
