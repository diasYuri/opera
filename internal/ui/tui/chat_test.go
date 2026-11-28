package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/diasYuri/opera/internal/modules/execution"
)

func TestFullscreenKeepsCompletedConversationAndStaysBounded(t *testing.T) {
	streamText := strings.Repeat("é界", 50000)
	m := model{composer: composerModel{editor: textarea.New()}, text: streamText, width: 40, height: 12}
	view := m.View()
	if !view.AltScreen || strings.Count(view.Content, "\n") > 12 {
		t.Fatal("fullscreen view exceeded terminal height or missed alternate screen")
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 8})
	m = next.(model)
	if !m.View().AltScreen || strings.Count(m.View().Content, "\n") > 10 {
		t.Fatal("resize reprinted unbounded content")
	}
	result := execution.Result{ID: "run", Text: "SCROLLBACK_ONLY_MARKER", State: "completed"}
	next, command := m.Update(updateMsg{update: execution.Update{Result: &result}, open: true})
	m = next.(model)
	if m.text != "" || command != nil || len(m.conversation.entries) != 1 || m.conversation.entries[0].text != streamText {
		t.Fatal("completed assistant message was not retained in the conversation viewport ledger")
	}
}

func TestChatSubmissionAfterFailureResumesActualRun(t *testing.T) {
	m := newModel(context.Background(), terminalExecutor{}, execution.Request{}, true)
	failed := &execution.Result{ID: "failed-run", State: "failed", Checkpoint: 3, ExitCode: 1}
	next, _ := m.Update(updateMsg{open: true, update: execution.Update{Result: failed, Error: errors.New("request rejected")}})
	m = next.(model)
	request := m.chatRequest("try again")
	if request.ID != "failed-run" || request.ContinueFrom != "" || request.Prompt != "try again" {
		t.Fatal("failed run was submitted as a new conversation continuation")
	}
	// A validation error has no durable checkpoint. It must not replace the
	// last resumable run with a freshly generated, nonexistent run ID.
	next, _ = m.Update(updateMsg{open: true, update: execution.Update{Result: &execution.Result{ID: "not-persisted", ExitCode: 1}, Error: errors.New("validation failed")}})
	m = next.(model)
	if m.chatRequest("retry").ID != "failed-run" {
		t.Fatal("validation error lost the resumable run")
	}
	next, _ = m.Update(updateMsg{open: true, update: execution.Update{Result: &execution.Result{ID: "failed-run", State: "completed", Checkpoint: 4}}})
	m = next.(model)
	request = m.chatRequest("next question")
	if request.ID != "" || request.ContinueFrom != "failed-run" {
		t.Fatal("successful recovery did not restore normal conversation")
	}
}

type requestCaptureExecutor struct {
	terminalExecutor
	request execution.Request
}

func (e *requestCaptureExecutor) Start(_ context.Context, request execution.Request) *execution.Execution {
	e.request = request
	return &execution.Execution{ID: "review-run"}
}

func TestReviewCommandSelectsProfileWithoutChangingRuntimeMode(t *testing.T) {
	executor := &requestCaptureExecutor{}
	m := newModel(context.Background(), executor, execution.Request{}, true)
	m.runtimeMode = execution.RuntimeModePlan
	m.previous = "prior-run"
	m.composer.editor.SetValue("/review inspect parser")
	next, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("review command did not start an execution")
	}
	_ = command()
	request := executor.request
	if request.AgentProfile != execution.AgentProfileReview || request.RuntimeMode != execution.RuntimeModePlan || request.Prompt != "inspect parser" || request.ContinueFrom != "prior-run" {
		t.Fatalf("review request lost one of its independent selections: %+v", request)
	}
	if next.(model).runtimeMode != execution.RuntimeModePlan {
		t.Fatal("invoking a profile changed the selected runtime mode")
	}
}

func TestCtrlCCancelsExecutionAndWaitsForCancelledResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan execution.Update, 1)
	executionStream := execution.NewExecution("cancel-run", updates, cancel)
	m := newModel(context.Background(), terminalExecutor{}, execution.Request{}, true)
	m.stream = executionStream
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

	update := execution.Update{Result: &execution.Result{ID: executionStream.ID, State: "cancelled", ExitCode: 130}}
	nextModel, _ = m.Update(updateMsg{open: true, update: update})
	m = nextModel.(model)
	if m.stream != nil || m.status != "Cancelled · /retry to resume" || !m.retryPrevious {
		t.Fatalf("cancelled result did not restore the resumable chat state: status=%q retry=%v", m.status, m.retryPrevious)
	}
}

type navigationTestExecutor struct {
	terminalExecutor
	summary             SessionSummary
	readID              string
	readVersion         uint64
	branchID            string
	branchVersion       uint64
	settingKey          string
	settingValue        string
	webSearchAuthOption WebSearchAuthOption
	webSearchAuthCalled bool
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
	items := []SettingItem{{Key: "thinking", Value: "medium", Source: "workspace", Scope: "workspace", Description: "Reasoning level"}}
	if e.settingKey == "ui.activity" {
		items = append(items, SettingItem{Key: e.settingKey, Value: e.settingValue})
	}
	return items
}
func (e *navigationTestExecutor) WebSearchProviders() []WebSearchProviderOption {
	return []WebSearchProviderOption{{ID: "brave", Name: "Brave Search", Description: "Requires an API key", Current: true, AuthOptions: []WebSearchAuthOption{{SearchProviderID: "brave", CredentialProviderID: "brave-search", MethodID: "api-key", Kind: "api-key", Name: "Brave Search API key", Description: "Set a key"}}}}
}
func (e *navigationTestExecutor) WebSearchSettings(provider string) []SettingItem {
	if provider != "brave" {
		return nil
	}
	return []SettingItem{{Key: "search.options.brave.safe_search", Label: "Safe search", Value: "", DisplayValue: "Provider default", InputKind: "select", Choices: []string{"", "off", "moderate", "strict"}, Source: "default", Scope: "user workspace"}}
}
func (e *navigationTestExecutor) WebSearchAuthOptions(provider string) []WebSearchAuthOption {
	if provider != "brave" {
		return nil
	}
	return e.WebSearchProviders()[0].AuthOptions
}
func (e *navigationTestExecutor) LoginWebSearch(_ context.Context, option WebSearchAuthOption, _ func(execution.LoginNotice) error) error {
	e.webSearchAuthOption = option
	e.webSearchAuthCalled = true
	return nil
}
func (e *navigationTestExecutor) SetSetting(_ context.Context, key, value string) (string, error) {
	e.settingKey, e.settingValue = key, value
	return "Saved " + key, nil
}

func TestSettingsWebSearchProviderOptionsNavigation(t *testing.T) {
	ctx := context.Background()
	e := &navigationTestExecutor{}
	m := newModel(ctx, e, execution.Request{}, true)
	m.openWebSearchMenu(SettingItem{Key: "web-search", Value: "brave", DisplayValue: "Brave Search"})
	if m.overlay.selectionKind != "web-search" {
		t.Fatalf("web search menu kind = %q", m.overlay.selectionKind)
	}
	m.overlay.selector.Select(1)
	updated, _ := m.selectCurrent()
	m = updated.(model)
	if m.overlay.selectionKind != "web-search-options" {
		t.Fatalf("options menu kind = %q", m.overlay.selectionKind)
	}
	m.overlay.selector.Select(0)
	updated, _ = m.selectCurrent()
	m = updated.(model)
	if m.overlay.selectionKind != "web-search-provider" {
		t.Fatalf("provider menu kind = %q", m.overlay.selectionKind)
	}
	m.overlay.selector.Select(1)
	updated, _ = m.selectCurrent()
	m = updated.(model)
	if m.overlay.selectionKind != "web-search-config-options" {
		t.Fatalf("config options menu kind = %q", m.overlay.selectionKind)
	}
	m.overlay.selector.Select(0)
	updated, command := m.selectCurrent()
	m = updated.(model)
	if command == nil || m.formKind != "search-option" || m.formKey != "search.options.brave.safe_search" {
		t.Fatalf("provider config option did not open an editor: kind=%q key=%q command=%v", m.formKind, m.formKey, command != nil)
	}
}

func TestSettingsWebSearchAuthenticationNavigation(t *testing.T) {
	ctx := context.Background()
	e := &navigationTestExecutor{}
	m := newModel(ctx, e, execution.Request{}, true)
	m.openWebSearchMenu(SettingItem{Key: "web-search", Value: "brave", DisplayValue: "Brave Search"})
	m.overlay.selector.Select(1)
	updated, _ := m.selectCurrent()
	m = updated.(model)
	m.overlay.selector.Select(0)
	updated, _ = m.selectCurrent()
	m = updated.(model)
	if m.overlay.selectionKind != "web-search-provider" {
		t.Fatalf("provider menu kind = %q", m.overlay.selectionKind)
	}
	m.overlay.selector.Select(0)
	updated, _ = m.selectCurrent()
	m = updated.(model)
	if m.overlay.selectionKind != "web-search-auth-options" {
		t.Fatalf("authentication menu kind = %q", m.overlay.selectionKind)
	}
	m.overlay.selector.Select(0)
	updated, command := m.selectCurrent()
	m = updated.(model)
	if command == nil || m.login == nil || !m.login.searchOnly || m.login.name != "Brave Search API key" {
		t.Fatalf("web search auth did not start an independent login flow: flow=%#v command=%v", m.login, command != nil)
	}
}

func TestSessionResumeBranchAndSettingsActions(t *testing.T) {
	ctx := context.Background()
	e := &navigationTestExecutor{summary: SessionSummary{ID: "run-1", SessionID: "session-1", Title: "Saved work", State: "completed", Checkpoint: 4}}

	resumeModel := newModel(ctx, e, execution.Request{}, true)
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

	branchModel := newModel(ctx, e, execution.Request{}, true)
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

	settingsModel := newModel(ctx, e, execution.Request{}, true)
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
		result := execution.Result{ID: "same-run", Checkpoint: checkpoint, Text: "block"}
		next, _ = m.Update(updateMsg{update: execution.Update{Result: &result}, open: true})
		m = next.(model)
	}
	if len(m.conversation.committed) != 2 {
		t.Fatal("approval continuation suppressed a later response")
	}
	next, _ = m.Update(updateMsg{update: execution.Update{Result: &execution.Result{ID: "same-run", Checkpoint: 2, Text: "block"}}, open: true})
	m = next.(model)
	if len(m.conversation.committed) != 2 {
		t.Fatal("replay duplicated checkpoint identity")
	}
}

func TestPolicySelectionUsesHighlightedOption(t *testing.T) {
	e := &navigationTestExecutor{}
	m := newModel(context.Background(), e, execution.Request{}, true)
	m.openSetting(SettingItem{Key: "policy", Value: "ask"})

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)

	if m.formKind != "policy-confirm" || m.formValue != "trusted" {
		t.Fatalf("policy selection did not carry highlighted option into confirmation: kind=%q value=%q", m.formKind, m.formValue)
	}
	confirm, ok := m.approvalForm.GetFocusedField().GetValue().(bool)
	if !ok || !confirm {
		t.Fatal("policy confirmation should select Apply policy by default")
	}
	next, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if command == nil {
		t.Fatal("confirming the policy did not schedule persistence")
	}
	result := command()
	if e.settingKey != "policy" || e.settingValue != "trusted" {
		t.Fatalf("policy confirmation persisted %s=%s", e.settingKey, e.settingValue)
	}
	next, _ = m.Update(result)
	if got := next.(model).status; got != "Saved policy" {
		t.Fatalf("policy save result was not reported: status=%q", got)
	}
}
