package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/diasYuri/opera/internal/app"
)

func TestInlineViewKeepsCompletedConversationAndStaysBounded(t *testing.T) {
	streamText := strings.Repeat("é界", 50000)
	m := model{composer: composerModel{editor: textarea.New()}, text: streamText, width: 40, height: 12}
	view := m.View()
	if view.AltScreen || strings.Count(view.Content, "\n") > 12 {
		t.Fatal("inline dynamic view exceeded terminal height or entered alternate screen")
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 8})
	m = next.(model)
	if m.View().AltScreen || strings.Count(m.View().Content, "\n") > 10 {
		t.Fatal("resize reprinted unbounded content")
	}
	result := app.Result{ID: "run", Text: "SCROLLBACK_ONLY_MARKER", State: "completed"}
	next, command := m.Update(updateMsg{update: app.Update{Result: &result}, open: true})
	m = next.(model)
	if m.text != "" || command != nil || len(m.conversation.entries) != 1 || m.conversation.entries[0].text != streamText {
		t.Fatal("completed assistant message was not retained in the conversation viewport ledger")
	}
}

func TestChatSubmissionAfterFailureResumesActualRun(t *testing.T) {
	m := newModel(context.Background(), terminalExecutor{}, app.Request{}, true)
	failed := &app.Result{ID: "failed-run", State: "failed", Checkpoint: 3, ExitCode: 1}
	next, _ := m.Update(updateMsg{open: true, update: app.Update{Result: failed, Error: errors.New("request rejected")}})
	m = next.(model)
	request := m.chatRequest("try again")
	if request.ID != "failed-run" || request.ContinueFrom != "" || request.Prompt != "try again" {
		t.Fatal("failed run was submitted as a new conversation continuation")
	}
	// A validation error has no durable checkpoint. It must not replace the
	// last resumable run with a freshly generated, nonexistent run ID.
	next, _ = m.Update(updateMsg{open: true, update: app.Update{Result: &app.Result{ID: "not-persisted", ExitCode: 1}, Error: errors.New("validation failed")}})
	m = next.(model)
	if m.chatRequest("retry").ID != "failed-run" {
		t.Fatal("validation error lost the resumable run")
	}
	next, _ = m.Update(updateMsg{open: true, update: app.Update{Result: &app.Result{ID: "failed-run", State: "completed", Checkpoint: 4}}})
	m = next.(model)
	request = m.chatRequest("next question")
	if request.ID != "" || request.ContinueFrom != "failed-run" {
		t.Fatal("successful recovery did not restore normal conversation")
	}
}

func TestCtrlCCancelsExecutionAndWaitsForCancelledResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan app.Update, 1)
	execution := app.NewExecution("cancel-run", updates, cancel)
	m := newModel(context.Background(), terminalExecutor{}, app.Request{}, true)
	m.stream = execution
	m.status = "Running"

	nextModel, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = nextModel.(model)
	if cmd == nil || m.status != "Cancelling execution…" {
		t.Fatal("Ctrl+C did not request cancellation or expose the cancelling state")
	}
	cmd()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("execution cancel function was not called")
	}
	if m.stream == nil || m.status != "Cancelling execution…" {
		t.Fatal("UI settled before the executor returned its cancellation result")
	}

	update := app.Update{Result: &app.Result{ID: execution.ID, State: "cancelled", ExitCode: 130}}
	nextModel, _ = m.Update(updateMsg{open: true, update: update})
	m = nextModel.(model)
	if m.stream != nil || m.status != "Cancelled · /retry to resume" || !m.retryPrevious {
		t.Fatalf("cancelled result did not restore the resumable chat state: status=%q retry=%v", m.status, m.retryPrevious)
	}
}

type navigationTestExecutor struct {
	terminalExecutor
	summary       SessionSummary
	readID        string
	readVersion   uint64
	branchID      string
	branchVersion uint64
	settingKey    string
	settingValue  string
}

func (e *navigationTestExecutor) SessionChoices(context.Context) ([]SessionSummary, error) {
	return []SessionSummary{e.summary}, nil
}
func (e *navigationTestExecutor) ReadSession(_ context.Context, id string, version uint64) (SessionView, error) {
	e.readID, e.readVersion = id, version
	summary := e.summary
	summary.ID = id
	if version > 0 {
		summary.Checkpoint = version
	}
	return SessionView{Summary: summary, Messages: []HistoryMessage{{ID: "user-1", Role: "You", Text: "continue this work"}}}, nil
}
func (e *navigationTestExecutor) BranchSession(_ context.Context, id string, version uint64) (string, error) {
	e.branchID, e.branchVersion = id, version
	return "branch-1", nil
}
func (e *navigationTestExecutor) Settings() []SettingItem {
	return []SettingItem{{Key: "thinking", Value: "medium", Source: "workspace", Scope: "workspace", Description: "Reasoning level"}}
}
func (e *navigationTestExecutor) SetSetting(_ context.Context, key, value string) (string, error) {
	e.settingKey, e.settingValue = key, value
	return "Saved " + key, nil
}

func TestSessionResumeBranchAndSettingsActions(t *testing.T) {
	ctx := context.Background()
	e := &navigationTestExecutor{summary: SessionSummary{ID: "run-1", SessionID: "session-1", Title: "Saved work", State: "completed", Checkpoint: 4}}

	resumeModel := newModel(ctx, e, app.Request{}, true)
	resumeModel.openSelector("Resume a session", []list.Item{sessionChoice{e.summary}}, resumeModel.modelDelegate())
	resumeModel.overlay.selectionKind = "session-resume"
	selected, command := resumeModel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("keyboard selection of a saved session did not schedule a read")
	}
	updated, _ := selected.(model).Update(command())
	resumed := updated.(model)
	if e.readID != "run-1" || e.readVersion != 0 || resumed.previous != "run-1" || resumed.retryPrevious || !strings.Contains(resumed.status, "next message continues") {
		t.Fatalf("completed session did not resume from its saved checkpoint: read=%s/%d previous=%s retry=%v status=%q", e.readID, e.readVersion, resumed.previous, resumed.retryPrevious, resumed.status)
	}

	branchModel := newModel(ctx, e, app.Request{}, true)
	branchCommand := branchModel.branch(e.summary, 2)
	if branchCommand == nil {
		t.Fatal("branch action did not schedule persistence")
	}
	branchResult := branchCommand().(branchMsg)
	if branchResult.err != nil || e.branchID != "run-1" || e.branchVersion != 2 || branchResult.id != "branch-1" {
		t.Fatalf("branch action used the wrong source checkpoint: result=%+v source=%s/%d", branchResult, e.branchID, e.branchVersion)
	}
	updated, loadBranch := branchModel.Update(branchResult)
	if loadBranch == nil {
		t.Fatal("persisted branch did not schedule its session read")
	}
	branchView := loadBranch().(sessionViewMsg)
	updated, _ = updated.(model).Update(branchView)
	ready := updated.(model)
	if ready.previous != "branch-1" || !ready.retryPrevious || !strings.Contains(ready.status, "Branch ready") {
		t.Fatalf("new branch was not made resumable: previous=%q retry=%v status=%q", ready.previous, ready.retryPrevious, ready.status)
	}

	settingsModel := newModel(ctx, e, app.Request{}, true)
	settingsCommand := settingsModel.applySetting("thinking", "high")
	if settingsCommand == nil {
		t.Fatal("settings action did not schedule persistence")
	}
	updated, _ = settingsModel.Update(settingsCommand())
	if e.settingKey != "thinking" || e.settingValue != "high" || updated.(model).status != "Saved thinking" {
		t.Fatalf("settings action did not persist and report the selected value: %s=%s status=%q", e.settingKey, e.settingValue, updated.(model).status)
	}
}

func TestPasteHistoryAndCheckpointCommitIdentity(t *testing.T) {
	editor := textarea.New()
	editor.Focus()
	m := model{composer: composerModel{editor: editor}, width: 60, height: 16}
	next, _ := m.Update(tea.PasteMsg{Content: "first\n/approve\n界"})
	m = next.(model)
	if m.stream != nil || !strings.Contains(m.composer.editor.Value(), "/approve") {
		t.Fatal("paste executed or lost text")
	}
	m.composer.history = []string{"old", "new"}
	m.composer.historyIndex = 2
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModAlt})
	m = next.(model)
	if m.composer.editor.Value() != "new" {
		t.Fatal("history navigation", m.composer.editor.Value())
	}
	for _, checkpoint := range []uint64{1, 2} {
		result := app.Result{ID: "same-run", Checkpoint: checkpoint, Text: "block"}
		next, _ = m.Update(updateMsg{update: app.Update{Result: &result}, open: true})
		m = next.(model)
	}
	if len(m.conversation.committed) != 2 {
		t.Fatal("approval continuation suppressed a later response")
	}
	next, _ = m.Update(updateMsg{update: app.Update{Result: &app.Result{ID: "same-run", Checkpoint: 2, Text: "block"}}, open: true})
	m = next.(model)
	if len(m.conversation.committed) != 2 {
		t.Fatal("replay duplicated checkpoint identity")
	}
}
