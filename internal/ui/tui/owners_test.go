package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/diasYuri/opera/internal/modules/execution"
)

func TestConversationOwnerKeepsStableIdentityAndCheckpointCommit(t *testing.T) {
	conversation := newConversationModel()
	if got := conversation.nextIdentity("user"); got != "user:1" {
		t.Fatalf("first identity = %q", got)
	}
	conversation.append(conversationEntry{id: "assistant:run:1", text: "first"})
	conversation.append(conversationEntry{id: "assistant:run:1", text: "updated"})
	if len(conversation.entries) != 1 || conversation.entries[0].text != "updated" {
		t.Fatalf("stable entry update = %#v", conversation.entries)
	}
	if !conversation.commitCheckpoint("run:1") || conversation.commitCheckpoint("run:1") {
		t.Fatal("checkpoint identity was not committed exactly once")
	}
	conversation.resetLedger()
	if len(conversation.entries) != 0 || len(conversation.entryIndexes) != 0 {
		t.Fatal("reset retained projected entries")
	}
}

func TestOverlayOwnerSwitchesSelectionAndPanelExclusively(t *testing.T) {
	overlay := overlayModel{}
	selector := list.New(nil, list.NewDefaultDelegate(), 20, 5)
	overlay.showSelector(selector)
	if !overlay.selecting || overlay.panelOpen {
		t.Fatal("selector transition did not close the panel")
	}
	panel := viewport.New()
	overlay.showPanel("Details", panel)
	if overlay.selecting || !overlay.panelOpen || overlay.panelTitle != "Details" {
		t.Fatal("panel transition did not close selection")
	}
	overlay.dismissPanel()
	if overlay.panelOpen {
		t.Fatal("panel dismissal left the panel open")
	}
}

func TestComposerOwnerNavigatesHistoryWithoutLosingDraftSlot(t *testing.T) {
	composer := composerModel{editor: textarea.New(), history: []string{"older", "newer"}, historyIndex: 2}
	composer.navigateHistory(true)
	if composer.editor.Value() != "newer" {
		t.Fatalf("previous history = %q", composer.editor.Value())
	}
	composer.navigateHistory(false)
	if composer.editor.Value() != "" || composer.historyIndex != 2 {
		t.Fatal("next history did not return to the empty draft")
	}
}

func TestComposerHistoryArrowsStayInMultilineDrafts(t *testing.T) {
	composer := composerModel{editor: textarea.New(), history: []string{"older"}, historyIndex: 1}
	composer.editor.SetValue("single-line draft")
	if !composer.shouldNavigateHistory(true) || !composer.shouldNavigateHistory(false) {
		t.Fatal("single-line boundary arrows should navigate prompt history")
	}
	composer.editor.SetValue("first line\nsecond line")
	if composer.shouldNavigateHistory(true) || composer.shouldNavigateHistory(false) {
		t.Fatal("multiline arrows should move through the draft, not prompt history")
	}
}

func TestComposerShiftEnterInsertsNewline(t *testing.T) {
	m := newModel(context.Background(), terminalExecutor{}, execution.Request{}, true)
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if got := next.(model).composer.editor.Value(); got != "\n" {
		t.Fatalf("Shift+Enter draft = %q, want newline", got)
	}
}

func TestComposerHelpMatchesDraftNavigation(t *testing.T) {
	m := newModel(context.Background(), terminalExecutor{}, execution.Request{}, true)
	m.modelName, m.provider = "fixture", "openai"
	m.composer.editor.SetValue("single line")
	if !strings.Contains(m.helpLine(), "↑↓ history") {
		t.Fatal("single-line composer help omitted history navigation")
	}
	m.composer.editor.SetValue("first line\nsecond line")
	if !strings.Contains(m.helpLine(), "↑↓ edit text") {
		t.Fatal("multiline composer help advertised history navigation")
	}
}
