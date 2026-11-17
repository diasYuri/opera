package ui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/diasYuri/opera/internal/app"
)

type commandExecutor struct {
	terminalExecutor
	command string
}

func (e *commandExecutor) Command(_ context.Context, text string) (string, error) {
	e.command = text
	return "Applied", nil
}
func TestModelSelectorWorksEntirelyFromKeyboard(t *testing.T) {
	executor := &commandExecutor{}
	m := model{service: executor, editor: textarea.New(), width: 80, height: 24}
	next, _ := m.Update(choicesMsg{values: []string{"openai first", "google second"}})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.selecting || cmd == nil {
		t.Fatal("selector did not confirm from keyboard")
	}
	_ = cmd()
	if executor.command != "/model google second" {
		t.Fatal(executor.command)
	}
	next, _ = m.Update(choicesMsg{values: []string{"openai first"}})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.(model).selecting {
		t.Fatal("Escape did not close selector")
	}
}
func TestNoMotionAndColorlessStatusCarryTextualMeaning(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m := model{editor: textarea.New(), width: 80, height: 24, noMotion: true, stream: &app.Execution{ID: "run"}, status: "Running", text: "界"}
	content := m.View().Content
	if !strings.Contains(content, "Running") || strings.Contains(content, "· ·") {
		t.Fatal(content)
	}
}
