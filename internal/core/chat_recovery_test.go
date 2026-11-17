package core_test

import (
	"context"
	"errors"
	"testing"

	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/agent/agenttest"
	"github.com/diasYuri/y/pkg/ai"
)

func TestFailedChatTurnDoesNotReplayPreviousAnswerAndCanResume(t *testing.T) {
	cfg := configuration(t)
	r := open(t, cfg, agenttest.TextTurn("previous answer"), agenttest.ScriptedTurn{Err: errors.New("request rejected")}, agenttest.TextTurn("recovered answer"), agenttest.TextTurn("third answer"))
	defer closeRuntime(t, r)
	ctx := context.Background()
	first, err := r.Service.Execute(ctx, execution.Request{Prompt: "first question"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := r.Service.Execute(ctx, execution.Request{Prompt: "second question", ContinueFrom: first.ID}, nil)
	if err == nil || failed.ExitCode == 0 || failed.Text != "" {
		t.Fatalf("failed request replayed an old answer: text=%q err=%v", failed.Text, err)
	}
	recovered, err := r.Service.Execute(ctx, execution.Request{ID: failed.ID, Prompt: "second question"}, nil)
	if err != nil || recovered.ExitCode != 0 || recovered.Text != "recovered answer" {
		t.Fatal("failed turn did not resume", err)
	}
	snapshot, err := r.Service.Store.Load(ctx, recovered.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, message := range snapshot.Agent.Transcript {
		if message.Role != ai.RoleUser {
			continue
		}
		for _, block := range message.Content {
			if block.Text == "second question" {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatal("explicit retry duplicated the unanswered prompt", count)
	}
	third, err := r.Service.Execute(ctx, execution.Request{Prompt: "third question", ContinueFrom: recovered.ID}, nil)
	if err != nil || third.Text != "third answer" {
		t.Fatal("conversation remained blocked after recovery", err)
	}
}
