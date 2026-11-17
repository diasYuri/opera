package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/diasYuri/opera/internal/app"
)

func TestLayoutFitsTerminalAcrossStates(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {80, 24}, {40, 12}, {20, 8}, {5, 3}} {
		for _, state := range []string{"idle", "streaming", "error", "approval", "models", "login-menu", "login-progress"} {
			t.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], state), func(t *testing.T) {
				m := newModel(context.Background(), terminalExecutor{}, app.Request{}, true)
				updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				m = updated.(model)
				m.composer.editor.SetValue("a multiline\nmessage 界🙂")
				m.sizeEditor()
				switch state {
				case "streaming":
					m.stream = &app.Execution{ID: "run"}
					m.text = strings.Repeat("Unicode 界🙂 long stream\n", 1000)
					m.status = "Running"
				case "error":
					m.lastError = "failure"
					m.status = strings.Repeat("long error ", 100)
				case "approval":
					m.approval = &app.Approval{ID: "approval"}
					m.openPanel("Review operation", strings.Repeat("long/path/界🙂", 100))
				case "login-menu":
					m.service = &loginExecutor{providers: []app.LoginProvider{{ID: "openai", Name: "OpenAI", Description: "ChatGPT subscription · Continue with ChatGPT"}}}
					m.openLoginProviders()
				case "login-progress":
					m.login = &loginFlow{name: "OpenAI"}
					m.actionPending = true
					m.status = "Finish sign-in in your browser. The authorization link is above."
				case "models":
					updated, _ = m.Update(choicesMsg{values: []string{strings.Repeat("model", 100), "another model"}})
					m = updated.(model)
				}
				view := m.View()
				if view.AltScreen {
					t.Fatal("alternate screen breaks scrollback")
				}
				lines := strings.Split(view.Content, "\n")
				if len(lines) > max(1, size[1]-1) {
					t.Fatalf("%d rows exceed terminal", len(lines))
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > size[0] {
						t.Fatalf("line exceeds terminal: %q", line)
					}
				}
			})
		}
	}
}

func TestErrorSummaryKeepsDiagnosticsAndCommitsOnce(t *testing.T) {
	raw := "openai: network error: POST https://api.openai.com/v1/responses: 404 Not Found {\"message\":\"The model `gpt` does not exist\",\"code\":\"model_not_found\"}"
	m := newModel(context.Background(), terminalExecutor{}, app.Request{}, true)
	result := &app.Result{ID: "failed-run", Checkpoint: 2, State: "failed", ExitCode: 1}
	update := updateMsg{open: true, update: app.Update{Result: result, Error: errors.New(raw)}}
	updated, cmd := m.Update(update)
	m = updated.(model)
	if cmd != nil || m.lastError != raw {
		t.Fatal("conversation error was not retained or diagnostics are missing")
	}
	if strings.Contains(m.View().Content, "POST") || !strings.Contains(m.errorBlock(raw), "Model unavailable") {
		t.Fatal("raw transport error leaked into primary UI")
	}
	updated, duplicate := m.Update(update)
	m = updated.(model)
	if duplicate != nil {
		t.Fatal("replayed result duplicated the error block")
	}
	m.composer.editor.SetValue("/details")
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(model)
	if !m.overlay.panelOpen || !strings.Contains(m.overlay.panel.GetContent(), "POST") {
		t.Fatal("raw diagnostics not inspectable")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(model)
	if m.overlay.panelOpen || !m.composer.editor.Focused() {
		t.Fatal("Escape did not restore composer focus")
	}
}

func TestNoColorCoversBannerMessagesAndViewers(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m := newModel(context.Background(), terminalExecutor{}, app.Request{}, true)
	for _, text := range []string{m.banner(), m.View().Content, m.messageBlock("Opera", "answer"), m.errorBlock("failure")} {
		if strings.Contains(text, "\x1b") {
			t.Fatalf("NO_COLOR emitted terminal styling: %q", text)
		}
	}
	updated, _ := m.Update(choicesMsg{values: []string{"openai model"}})
	m = updated.(model)
	if strings.Contains(m.View().Content, "\x1b") {
		t.Fatal("model selector ignored NO_COLOR")
	}
}

func TestActivityStopsAtIdleAndWithNoMotion(t *testing.T) {
	idle := newModel(context.Background(), terminalExecutor{}, app.Request{}, false)
	if idle.composer.editor.Styles().Cursor.Blink || idle.composer.editor.Focus() != nil {
		t.Fatal("idle cursor scheduled a blink timer")
	}
	m := newModel(context.Background(), terminalExecutor{}, app.Request{}, false)
	m.stream = &app.Execution{ID: "run"}
	m.activity.generation = 7
	m.activity.target = 1
	updated, cmd := m.Update(frameMsg{7})
	m = updated.(model)
	if cmd == nil || m.activity.progress <= 0 {
		t.Fatal("active spring did not advance")
	}
	_, cmd = m.Update(frameMsg{6})
	if cmd != nil {
		t.Fatal("stale animation scheduled another timer")
	}
	m.noMotion = true
	_, cmd = m.Update(frameMsg{7})
	if cmd != nil || m.animate() != nil {
		t.Fatal("no-motion scheduled animation")
	}
	m.noMotion = false
	m.stream = nil
	_, cmd = m.Update(frameMsg{7})
	if cmd != nil {
		t.Fatal("idle scheduled animation")
	}
}

func TestComposerGrowsForPasteAndShrinksAfterSend(t *testing.T) {
	m := newModel(context.Background(), terminalExecutor{}, app.Request{}, true)
	if m.composer.editor.ShowLineNumbers || m.composer.editor.Height() != 1 {
		t.Fatal("composer should start with a single plain line")
	}
	updated, _ := m.Update(tea.PasteMsg{Content: "one\ntwo\nthree"})
	m = updated.(model)
	if m.composer.editor.Height() != 3 || m.actionPending {
		t.Fatal("paste did not grow composer safely")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(model)
	if m.composer.editor.Height() != 1 || !m.actionPending {
		t.Fatal("send did not reset composer")
	}
}

func TestCompletionsAppearBelowComposer(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, width := range []int{40, 80, 120} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			m := newModel(context.Background(), terminalExecutor{}, app.Request{}, true)
			m.width = width
			m.resize()
			m.composer.editor.SetValue("/")
			m.refreshCompletions()
			if len(m.completions) == 0 {
				t.Fatal("expected slash command suggestions")
			}
			view := m.View().Content
			input := strings.Index(view, "› /")
			if input < 0 {
				t.Fatalf("composer missing: %q", view)
			}
			belowInput := view[input:]
			border := strings.Index(belowInput, "─")
			suggestion := strings.Index(belowInput, "› "+m.completions[0].label)
			if border < 0 || suggestion <= border {
				t.Fatalf("suggestions must follow the composer's bottom border: %q", view)
			}
		})
	}
}

func TestComposerStaysPinnedWhileHistoryScrolls(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := newModel(context.Background(), terminalExecutor{}, app.Request{}, true)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(model)
	for i := 0; i < 30; i++ {
		m.appendUser(fmt.Sprintf("history message %02d", i), "You")
	}
	m.resize()
	lines := strings.Split(m.View().Content, "\n")
	composerLine := -1
	for i, line := range lines {
		if strings.Contains(line, "› ") {
			composerLine = i
			break
		}
	}
	if composerLine < 0 || composerLine < len(lines)-8 {
		t.Fatalf("composer was not anchored near the terminal bottom: line=%d total=%d", composerLine, len(lines))
	}
	bottom := strings.Join(lines[composerLine:], "\n")
	m.conversation.ScrollUp(5)
	afterScroll := strings.Split(m.View().Content, "\n")
	newComposerLine := -1
	for i, line := range afterScroll {
		if strings.Contains(line, "› ") {
			newComposerLine = i
			break
		}
	}
	if newComposerLine != composerLine || strings.Join(afterScroll[newComposerLine:], "\n") != bottom {
		t.Fatal("scrolling history moved or changed the composer/footer block")
	}
}

func TestExternalControlCharactersCannotMoveTerminalCursor(t *testing.T) {
	got := safeText("safe\x1b[2J\r\x00\b\a\ttext\nnext")
	if got != "safe text\nnext" {
		t.Fatalf("unsafe terminal content: %q", got)
	}
}
