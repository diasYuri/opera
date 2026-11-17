package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/diasYuri/opera/internal/app"
)

func newModel(ctx context.Context, service Executor, initial app.Request, noMotion bool) model {
	keys := defaultChatKeyMap()
	editor := textarea.New()
	editor.Placeholder = "Message Opera… · ⌘V attaches an image"
	editor.Prompt = "› "
	editor.ShowLineNumbers = false
	editor.KeyMap.InsertNewline = keys.newline
	editor.Focus()
	cwd, _ := os.Getwd()
	m := model{ctx: ctx, service: service, editor: editor, conversation: viewport.New(), spinner: spinner.New(spinner.WithSpinner(spinner.Line)), keys: keys, workspace: filepath.Base(cwd), workspaceRoot: cwd, noMotion: noMotion, dark: true, systemDark: true, themeMode: "system", status: "Ready", resume: initial, mode: app.ModeDefault, width: 80, height: 24, actionPending: initial.ID != "" || initial.Prompt != ""}
	if initial.Mode != "" {
		m.mode = initial.Mode
	}
	if initial.Prompt != "" {
		m.appendUser(initial.Prompt, "You")
	}
	m.refreshContext()
	m.styleEditor()
	m.resize()
	return m
}

func (m *model) refreshContext() {
	if info, ok := m.service.(SessionInfo); ok {
		workspace, provider, name := info.SessionInfo()
		m.workspace, m.provider, m.modelName = filepath.Base(workspace), displayProvider(provider), name
		if workspace != "" {
			m.workspaceRoot = workspace
		}
	}
	if preferences, ok := m.service.(ThemePreferenceProvider); ok {
		if mode := preferences.ThemePreference(); mode != "" {
			m.themeMode = mode
		}
	}
	m.applyThemePreference()
}

func (m model) contentWidth() int { return max(1, m.width-4) }
func (m model) viewerHeight() int { return max(1, min(12, m.height-8)) }
func (m *model) resize() {
	m.editor.SetWidth(max(1, m.contentWidth()))
	m.sizeEditor()
	m.conversation.SetWidth(max(1, m.conversationWidth()))
	menuHeight := 0
	if m.selecting {
		menuHeight = m.selector.Height() + 1
	}
	if m.approvalForm != nil && m.formKind != "approval" {
		menuHeight += m.formHeight + 2
	}
	if m.selecting {
		rowHeight := 2
		if m.modelMenu {
			rowHeight = 1
		}
		height := min(m.viewerHeight(), min(4, len(m.selector.Items()))*rowHeight+2, max(1, m.height-m.editor.Height()-10))
		m.selector.SetSize(max(1, m.contentWidth()-4), height)
	}
	if m.panelOpen {
		m.panel.SetWidth(max(1, m.contentWidth()-4))
		m.panel.SetHeight(m.viewerHeight())
	}
	if m.approvalForm != nil {
		height := 5
		if m.formKind != "approval" {
			height = min(m.formHeight, max(1, m.height-m.editor.Height()-10))
		}
		m.approvalForm.WithWidth(max(1, m.contentWidth()-4)).WithHeight(height)
	}
	if !m.panelOpen && m.login == nil {
		fixedLines := 0
		for _, part := range m.defaultFixedParts() {
			fixedLines += strings.Count(part, "\n") + 1
		}
		m.conversation.SetHeight(max(1, m.height-1-fixedLines))
	} else {
		m.conversation.SetHeight(max(1, m.height-m.editor.Height()-8-m.queuedStackHeight()-m.attachmentStackHeight()-m.extensionReservedHeight()-m.todoListHeight()-m.subagentListHeight()-menuHeight))
	}
	m.refreshConversation()
}

func (m *model) sizeEditor() {
	width := max(1, m.contentWidth()-2)
	lines := strings.Count(ansi.Hardwrap(m.editor.Value(), width, true), "\n") + 1
	m.editor.SetHeight(min(5, max(1, m.height-8), lines))
}

func (m model) sessionLine() string {
	parts := []string{}
	if m.modelName != "" {
		parts = append(parts, safeText(m.provider)+"/"+safeText(m.modelName))
	}
	if m.workspace != "" {
		parts = append(parts, safeText(m.workspace))
	}
	line := strings.Join(parts, " · ")
	if m.mode != "" && m.mode != app.ModeDefault {
		if mode, err := app.ResolveMode(m.mode); err == nil {
			label := strings.ToLower(mode.Name)
			if line != "" {
				line += " · "
			}
			line += m.theme().info.Render(safeText(label))
		}
	}
	return line
}

func (m model) banner() string {
	t := m.theme()
	text := "\n  " + t.text.Bold(true).Render("Opera") + "\n  " + m.sessionLine() + "\n  " + "/login connect · /model choose model · /help commands" + "\n"
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		text = ansi.Strip(text)
	}
	return text
}

func (m *model) openPanel(title, text string) {
	m.panelTitle = title
	m.panel = viewport.New(viewport.WithWidth(max(1, m.contentWidth()-4)), viewport.WithHeight(m.viewerHeight()))
	m.panel.SoftWrap = true
	m.panel.SetContent(safeText(text))
	m.panelOpen = true
	m.editor.Blur()
}

func (m model) animate() tea.Cmd {
	if m.noMotion {
		return nil
	}
	generation := m.activity.generation
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return frameMsg{generation} })
}

func (m model) updateFrame(msg frameMsg) (tea.Model, tea.Cmd) {
	if m.noMotion || m.stream == nil && m.login == nil || !m.activity.Frame(msg.generation) {
		return m, nil
	}
	m.spinner, _ = m.spinner.Update(m.spinner.Tick())
	return m, m.animate()
}

func (m model) header() string {
	t := m.theme()
	title := t.accent.Bold(true).Render("Opera")
	context := "  /  " + safeText(m.workspace)
	if m.workspace == "" {
		context = "  /  chat"
	}
	if m.width >= 76 && m.modelName != "" {
		context += "  ·  " + safeText(m.provider) + "/" + safeText(m.modelName)
	}
	line := title + t.muted.Render(context)
	if contributed := m.extensionHeader(); contributed != "" {
		line += "\n" + contributed
	}
	return line
}

func (m model) statusLine() string {
	t := m.theme()
	status := safeText(m.status)
	if status == "" {
		status = "Ready"
	}
	style := t.muted
	if m.approval != nil {
		style = t.warning
	} else if m.lastError != "" {
		style = t.error
	}
	if m.stream != nil {
		style = t.accent
		if !m.noMotion {
			// An indeterminate activity trace, never a fabricated completion percentage.
			position := m.activity.Position()
			status = m.spinner.View() + " " + status + "  " + strings.Repeat("─", position) + "━" + strings.Repeat("─", 5-position)
		}
	}
	if m.login != nil {
		style = t.accent
		if !m.noMotion {
			status = m.spinner.View() + " " + status
		}
	}
	if m.actionPending && m.login == nil && (status == "" || status == "Ready") {
		status = "Starting…"
	}
	return style.Render(ansi.Truncate(status, m.contentWidth(), "…"))
}

func (m model) helpLine() string {
	keys := m.keyBindings()
	enter := keyLabel(keys.submit, "Enter")
	complete := keyLabel(keys.complete, "Tab")
	newline := keyLabel(keys.newline, "Alt+Enter")
	pageUp := keyLabel(keys.pageUp, "PgUp")
	pageDown := keyLabel(keys.pageDown, "PgDn")
	bottom := keyLabel(keys.bottom, "Ctrl+End")
	details := keyLabel(keys.details, "Ctrl+O")
	exit := keyLabel(keys.exit, "Ctrl+D")
	if m.approval != nil {
		return "/approve allow · /deny reject · /details review"
	}
	if m.status == "Outcome uncertain · reconcile before retry" {
		return "Inspect operations, then reconcile before retry · /details"
	}
	if m.lastError != "" {
		if m.retryPrevious {
			if m.width < 76 {
				return enter + " retry · /retry · /details"
			}
			return enter + " send / retry · /model change model · /details error"
		}
		return enter + " send a new request · /details error"
	}
	if m.stream != nil {
		if m.width < 64 {
			return enter + " queue · Ctrl+C cancel · " + details + " details"
		}
		return enter + " queue · /steer guide · Ctrl+C cancel · " + details + " tool details"
	}
	if m.retryPrevious {
		return enter + " retry · /retry resume · /details"
	}
	if m.modelName == "" {
		return "/login connect · /model choose · /resume sessions · /help"
	}
	if m.width < 48 {
		return enter + " send · /mode · Shift+Tab plan · /resume · /help"
	}
	if m.newConversationItems {
		return pageUp + "/" + pageDown + " scroll · " + bottom + " latest messages · /messages find"
	}
	if m.conversation.TotalLineCount() > 0 && m.width >= 48 {
		return pageUp + "/" + pageDown + " scroll · " + bottom + " latest · " + details + " tool details · /messages find"
	}
	if m.width < 76 {
		return enter + " send · " + complete + " complete · " + newline + " newline · /mode · Shift+Tab plan"
	}
	return enter + " send · " + complete + " complete · " + newline + " newline · /mode · Shift+Tab plan · /resume · /help · " + exit + " exit"
}

func (m model) completionView() string {
	t := m.theme()
	rows := make([]string, 0, len(m.completions))
	for i, item := range m.completions {
		row := item.label + "  " + t.muted.Render(item.detail)
		if i == m.completionIndex {
			row = t.accent.Bold(true).Render("› " + row)
		} else {
			row = "  " + row
		}
		rows = append(rows, ansi.Truncate(row, m.contentWidth(), "…"))
	}
	keys := m.keyBindings()
	return strings.Join(rows, "\n") + "\n" + t.muted.Render("↑↓ choose · "+keyLabel(keys.complete, "Tab")+"/"+keyLabel(keys.submit, "Enter")+" complete · "+keyLabel(keys.cancel, "Esc")+" close")
}

func (m model) queuedStackHeight() int {
	if len(m.queuedMessages) == 0 {
		return 0
	}
	if m.width < 32 || m.height < 12 {
		return 1
	}
	visible := min(3, len(m.queuedMessages))
	height := 1 + visible
	if visible < len(m.queuedMessages) {
		height++
	}
	return height
}

func (m model) queuedStackView() string {
	if len(m.queuedMessages) == 0 {
		return ""
	}
	t := m.theme()
	width := m.contentWidth()
	count := len(m.queuedMessages)
	header := t.accent.Bold(true).Render(fmt.Sprintf("Queued · %d", count))
	if m.width < 32 || m.height < 12 {
		text := strings.Join(strings.Fields(safeText(m.queuedMessages[0])), " ")
		return header + "  " + t.muted.Render(ansi.Truncate(text, max(1, width-12), "…"))
	}
	visible := min(3, count)
	lines := []string{header}
	for i, text := range m.queuedMessages[:visible] {
		branch := "├─"
		if i == visible-1 && visible == count {
			branch = "└─"
		}
		text = strings.Join(strings.Fields(safeText(text)), " ")
		lines = append(lines, t.accent.Render(branch)+" "+ansi.Truncate(text, max(1, width-9), "…"))
	}
	if visible < count {
		lines = append(lines, t.muted.Render(fmt.Sprintf("… +%d more queued", count-visible)))
	}
	return strings.Join(lines, "\n")
}

// defaultFixedParts returns every rendered block outside the message history.
// resize uses these same blocks to give the history the remaining terminal rows.
func (m model) defaultFixedParts() []string {
	t := m.theme()
	width := m.contentWidth()
	var parts []string
	if header := m.extensionHeader(); header != "" {
		parts = append(parts, header)
	}
	if todos := m.todoListView(); todos != "" {
		parts = append(parts, todos)
	}
	if widgets := m.extensionWidgets(); widgets != "" {
		parts = append(parts, widgets)
	}
	if m.stream != nil || m.actionPending || m.previous != "" || m.lastError != "" || m.status != "Ready" && m.status != "" {
		parts = append(parts, m.statusLine())
	}
	if queued := m.queuedStackView(); queued != "" {
		parts = append(parts, queued)
	}
	if attachments := m.attachmentPreviewView(); attachments != "" {
		parts = append(parts, attachments)
	}
	if question := m.questionView(); question != "" {
		parts = append(parts, question)
	}
	composer := lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false, true, false).BorderForeground(t.border.GetForeground())
	parts = append(parts, composer.Render(m.editor.View()))
	if subagents := m.subagentListView(); subagents != "" {
		parts = append(parts, subagents)
	}
	if m.selecting {
		if !m.modelMenu && m.selector.Title != "" {
			parts = append(parts, t.accent.Bold(true).Render(safeText(m.selector.Title)))
		}
		parts = append(parts, m.selector.View(), t.muted.Render("↑↓ scroll · Enter select · Esc close"))
	}
	if len(m.completions) > 0 {
		parts = append(parts, m.completionView())
	}
	if m.sessionLine() != "" {
		parts = append(parts, t.muted.Render(ansi.Truncate(m.sessionLine(), width, "…")))
	}
	parts = append(parts, t.muted.Render(ansi.Truncate(m.helpLine(), width, "…")))
	if footer := m.extensionFooter(); footer != "" {
		parts = append(parts, footer)
	}
	return parts
}

func (m model) View() tea.View {
	width := m.contentWidth()
	t := m.theme()
	var parts []string
	if m.login != nil && m.loginPrompt == nil {
		keys := m.keyBindings()
		parts = []string{m.header(), t.accent.Bold(true).Render("Connect to " + safeText(m.login.name)), m.statusLine(), t.muted.Render(keyLabel(keys.cancel, "Esc") + " or " + keyLabel(keys.quit, "Ctrl+C") + " cancel · session stays open")}
	} else if m.panelOpen && m.approvalForm != nil && m.formKind != "approval" {
		if header := m.extensionHeader(); header != "" {
			parts = append(parts, header)
		}
		if content := m.conversation.View(); content != "" {
			parts = append(parts, content)
		}
		if m.stream != nil || m.actionPending || m.previous != "" || m.lastError != "" || m.status != "Ready" && m.status != "" {
			parts = append(parts, m.statusLine())
		}
		composer := lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false, true, false).BorderForeground(t.border.GetForeground())
		parts = append(parts, composer.Render(m.editor.View()))
		parts = append(parts, t.accent.Bold(true).Render(safeText(m.panelTitle)), m.approvalForm.View())
		keys := m.keyBindings()
		parts = append(parts, t.muted.Render(keyLabel(keys.submit, "Enter")+" select · "+keyLabel(keys.cancel, "Esc")+" back"))
		if m.sessionLine() != "" {
			parts = append(parts, t.muted.Render(ansi.Truncate(m.sessionLine(), width, "…")))
		}
		parts = append(parts, t.muted.Render(ansi.Truncate(m.helpLine(), width, "…")))
	} else if m.panelOpen {
		title, body := m.panelTitle, m.panel.View()
		keys := m.keyBindings()
		hint := "↑↓ scroll · " + keyLabel(keys.cancel, "Esc") + " back"
		if m.approval != nil {
			hint = "Esc back · then /approve or /deny · ↑↓ scroll"
		}
		if m.selecting {
			hint = "/ filter · type to search · ↑↓ select · " + keyLabel(keys.submit, "Enter") + " open · " + keyLabel(keys.cancel, "Esc") + " back"
			if m.width < 48 {
				hint = "/ filter · ↑↓ · Enter · Esc"
			}
			title, body = m.selector.Title, m.selector.View()
		}
		parts = []string{m.header(), t.accent.Bold(true).Render(title), body, t.muted.Render(hint)}
		if m.approvalForm != nil {
			formHint := keyLabel(keys.submit, "Enter") + " select · " + keyLabel(keys.cancel, "Esc") + " back"
			switch m.formKind {
			case "approval":
				formHint = "Select allow or deny · Esc returns to the pending review"
			case "branch-confirm", "policy-confirm":
				formHint = keyLabel(keys.submit, "Enter") + " confirm · " + keyLabel(keys.cancel, "Esc") + " cancel"
			}
			parts = []string{m.header(), t.accent.Bold(true).Render(title)}
			if m.formKind == "approval" {
				parts = append(parts, body)
			}
			parts = append(parts, m.approvalForm.View(), t.muted.Render(formHint))
		}
	} else {
		fixed := m.defaultFixedParts()
		prefixCount := 0
		if m.extensionHeader() != "" {
			prefixCount++
		}
		if m.todoListView() != "" {
			prefixCount++
		}
		parts = append(parts, fixed[:prefixCount]...)
		parts = append(parts, m.conversation.View())
		parts = append(parts, fixed[prefixCount:]...)
	}
	// A terminal can shrink below the minimum useful editor. Clip each dimension
	// here as a final guard, including selectors, long IDs and wide Unicode.
	limit := max(1, m.height-1)
	var lines []string
	for _, part := range parts {
		lines = append(lines, strings.Split(part, "\n")...)
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	margin := ""
	if m.width > 4 {
		margin = "  "
	}
	for i := range lines {
		lines[i] = margin + ansi.Truncate(lines[i], width, "…")
	}
	content := strings.Join(lines, "\n")
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		content = stripColorKeepGraphics(content)
	}
	view := tea.NewView(content)
	view.AltScreen = false
	return view
}

func (m model) messageBlock(author, text string) string {
	t := m.theme()
	label := t.text.Bold(true)
	if strings.HasPrefix(author, "Opera") {
		label = t.accent.Bold(true)
	}
	body := ansi.Hardwrap(safeText(text), max(1, min(100, m.contentWidth()-2)), true)
	block := "\n  " + label.Render(author) + "\n" + lipgloss.NewStyle().PaddingLeft(2).Render(body) + "\n"
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		block = ansi.Strip(block)
	}
	return block
}

// Strip terminal escapes and control characters from external content. Newlines
// and tabs remain useful formatting; CR cannot move the cursor over UI chrome.
func safeText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if r == '\n' || r >= 32 && r != 127 {
			return r
		}
		return -1
	}, ansi.Strip(text))
}

func (m model) errorBlock(raw string) string {
	return m.messageBlock("Opera · error", summarizeError(raw))
}
