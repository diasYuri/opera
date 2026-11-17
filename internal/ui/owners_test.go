package ui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
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
