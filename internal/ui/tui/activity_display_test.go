package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/diasYuri/opera/internal/modules/execution"
)

func TestActivityDisplayProjectsTheSameRunInThreeModes(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m := newModel(context.Background(), terminalExecutor{}, execution.Request{}, true)
	startedModel, _ := m.Update(started{execution: &execution.Execution{ID: "run"}})
	m = startedModel.(model)
	m.conversation.display = "auto"
	send := func(kind, text, payload string) {
		m.updateConversationEvent(&execution.Event{RunID: "run", Type: kind, Text: text, Data: []byte(payload)})
	}
	send("thinking_delta", "", `{"thinking_delta":"reasoning detail"}`)
	if got := m.statusLine(); got != "Thinking…" {
		t.Fatalf("thinking status = %q", got)
	}
	send("message_delta", "\x1b[2JChecking 界🙂 files\nsecondary line", "")
	if got := m.statusLine(); got != "Checking 界🙂 files" {
		t.Fatalf("comment status = %q", got)
	}
	send("message_completed", "", `{"message":{"role":"assistant","tool_calls":[{"name":"read_file"}]}}`)
	if got := m.renderConversation(); !strings.Contains(got, "Checking") || !strings.Contains(got, "› Thought") {
		t.Fatalf("confirmed commentary and collapsed thinking missing from %q", got)
	}
	send("tool_started", "", `{"tool_call":{"id":"read","name":"read_file","arguments":{"path":"file.txt"}}}`)
	if got := m.statusLine(); got != "Running · read_file" {
		t.Fatalf("tool status = %q", got)
	}
	send("tool_completed", "", `{"tool_result":{"tool_call_id":"read","tool_name":"read_file","is_error":true,"content":[{"text":"raw tool output"}]}}`)
	send("tool_started", "", `{"tool_call":{"id":"list","name":"list_files"}}`)
	send("tool_completed", "", `{"tool_result":{"tool_call_id":"list","tool_name":"list_files"}}`)
	send("message_started", "", "")
	send("message_delta", "Final answer", "")
	if strings.Contains(m.renderConversation(), "Final answer") {
		t.Fatal("unclassified text was committed to history")
	}
	send("message_completed", "", `{"message":{"role":"assistant"}}`)
	for _, want := range []string{"Final answer", "› Ran 2 tools · 1 failed", "secondary line"} {
		if got := m.renderConversation(); !strings.Contains(got, want) || strings.Contains(got, "raw tool output") {
			t.Fatalf("auto history is missing %q or includes raw output: %q", want, got)
		}
	}
	inspecting, _ := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	inspector := inspecting.(model)
	if !inspector.overlay.selecting || len(inspector.overlay.selector.Items()) != 2 {
		t.Fatal("activity inspector did not expose both collapsed groups")
	}
	selected, _ := inspector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	inspector = selected.(model)
	if got := inspector.overlay.panel.GetContent(); !strings.Contains(got, "raw tool output") || !strings.Contains(got, "list_files") {
		t.Fatalf("tool group detail = %q", got)
	}

	m.conversation.display = "hidden"
	if got := m.statusLine(); got != "Running" {
		t.Fatalf("hidden status = %q", got)
	}
	if got := m.renderConversation(); !strings.Contains(got, "Final answer") || strings.Contains(got, "read_file") || strings.Contains(got, "Checking") {
		t.Fatalf("hidden history = %q", got)
	}

	m.conversation.display = "full"
	m.resize()
	for _, want := range []string{"reasoning detail", "Checking", "raw tool output", "Final answer"} {
		if got := m.renderConversation(); !strings.Contains(got, want) {
			t.Fatalf("full history is missing %q: %q", want, got)
		}
	}
}

func TestCompactActivityPreservesApprovalsErrorsAndStopsAnimation(t *testing.T) {
	m := newModel(context.Background(), terminalExecutor{}, execution.Request{}, false)
	startedModel, _ := m.Update(started{execution: &execution.Execution{ID: "run"}})
	m = startedModel.(model)
	m.conversation.display = "hidden"
	m.updateConversationEvent(&execution.Event{RunID: "run", Type: "message_delta", Text: "Before approval"})
	m.updateConversationEvent(&execution.Event{RunID: "run", Type: "message_completed", Data: []byte(`{"message":{"role":"assistant","tool_calls":[{}]}}`)})
	frame := frameMsg{m.activity.generation}
	finished, _ := m.Update(updateMsg{open: true, update: execution.Update{
		Result: &execution.Result{ID: "run", State: "awaiting_approval", ExitCode: 3, Approval: &execution.Approval{ID: "approval", Tool: "edit"}},
	}})
	m = finished.(model)
	if got := m.renderConversation(); !strings.Contains(got, "Approval required for edit") || strings.Contains(got, "Before approval") {
		t.Fatalf("approval history = %q", got)
	}
	if _, cmd := m.Update(frame); cmd != nil {
		t.Fatal("settled run scheduled an additional frame")
	}
	failed, _ := m.Update(updateMsg{open: true, update: execution.Update{
		Result: &execution.Result{ID: "run", State: "failed", ExitCode: 1, Checkpoint: 2},
		Error:  errors.New("provider unavailable"),
	}})
	if got := failed.(model).renderConversation(); !strings.Contains(got, "provider unavailable") {
		t.Fatalf("run error missing from %q", got)
	}
}

func TestActivitySelectorRestoresFocusAndSavesSelection(t *testing.T) {
	service := &navigationTestExecutor{}
	m := newModel(context.Background(), service, execution.Request{}, true)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	m.composer.editor.SetValue("/activity")
	opened, _ := m.Update(enter)
	m = opened.(model)
	if m.formKind != "activity-display" || m.composer.editor.Focused() {
		t.Fatal("activity selector did not take focus")
	}
	cancelled, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m = cancelled.(model); m.overlay.panelOpen || !m.composer.editor.Focused() {
		t.Fatal("Escape did not close activity selector and restore focus")
	}
	m.composer.editor.SetValue("/activity")
	opened, _ = m.Update(enter)
	m = opened.(model)
	navigated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = navigated.(model)
	setting, save := m.Update(enter)
	m = setting.(model)
	if save == nil || m.overlay.panelOpen {
		t.Fatal("Enter did not select an activity mode")
	}
	result := save()
	if service.settingKey != "ui.activity" || service.settingValue != "auto" {
		t.Fatalf("saved %q = %q", service.settingKey, service.settingValue)
	}
	settled, _ := m.Update(result)
	m = settled.(model)
	if !m.composer.editor.Focused() || m.conversation.display != "auto" {
		t.Fatal("save did not apply the selected mode and restore focus")
	}
	m.width = 16
	m.stream = &execution.Execution{ID: "run"}
	m.status = "Running"
	m.conversation.activity = strings.Repeat("界🙂", 30)
	if got := m.statusLine(); ansi.StringWidth(got) > m.contentWidth() || strings.ContainsRune(got, '\n') {
		t.Fatalf("activity exceeds narrow layout: %q", got)
	}
}
