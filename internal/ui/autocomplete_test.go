package ui

import (
	"context"
	"testing"

	"github.com/diasYuri/opera/internal/modules/execution"
)

type completionExecutor struct {
	terminalExecutor
	calls    int
	contexts []context.Context
}

func (e *completionExecutor) CompleteComposer(ctx context.Context, token string) ([]CompletionCandidate, error) {
	e.calls++
	e.contexts = append(e.contexts, ctx)
	return []CompletionCandidate{{Value: token + ".txt ", Label: token + ".txt", Detail: "file"}}, ctx.Err()
}

func TestCompletionDiscoveryRunsAsCommandAndRejectsStaleResults(t *testing.T) {
	executor := &completionExecutor{}
	m := newModel(context.Background(), executor, execution.Request{}, true)
	m.composer.editor.SetValue("@first")
	first := m.refreshCompletions()
	if executor.calls != 0 || first == nil {
		t.Fatal("discovery ran in Update instead of Cmd")
	}
	old := first().(completionsMsg)
	m.composer.editor.SetValue("@second")
	second := m.refreshCompletions()
	m.receiveCompletions(old)
	if len(m.completions) != 0 {
		t.Fatal("accepted stale generation")
	}
	m.receiveCompletions(second().(completionsMsg))
	if len(m.completions) != 1 || m.completions[0].label != "@second.txt" {
		t.Fatalf("current suggestions = %#v", m.completions)
	}
	m.composer.editor.SetValue("@third")
	third := m.refreshCompletions()().(completionsMsg)
	m.composer.editor.Reset()
	m.receiveCompletions(third)
	if len(m.completions) != 0 {
		t.Fatal("accepted completion after prompt reset")
	}
}
