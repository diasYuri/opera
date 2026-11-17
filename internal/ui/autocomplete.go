package ui

import (
	"context"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/diasYuri/opera/internal/modules/execution"
)

const maxCompletions = 6

type completionItem struct {
	start, end  int
	replacement string
	label       string
	detail      string
}

// CompletionCandidate is a presentation-safe workspace suggestion.
type CompletionCandidate struct{ Value, Label, Detail string }

// ComposerCompleter is an optional delivery capability; discovery runs in Cmd.
type ComposerCompleter interface {
	CompleteComposer(context.Context, string) ([]CompletionCandidate, error)
}

type completionsMsg struct {
	generation   uint64
	input        string
	line, column int
	start, end   int
	items        []CompletionCandidate
	err          error
}

func (m *model) refreshCompletions() tea.Cmd {
	if m.composer.completionCancel != nil {
		m.composer.completionCancel()
		m.composer.completionCancel = nil
	}
	_, start, end, token := editorToken(m.composer.editor.Value(), m.composer.editor.Line(), m.composer.editor.Column())
	m.completionGeneration++
	m.completions = nil
	m.completionIndex = 0
	if token == "" || m.composer.editor.HasSelection() {
		return nil
	}
	if strings.HasPrefix(token, "/") && start == 0 {
		for _, command := range execution.Commands {
			if strings.HasPrefix(command.Name, token) && command.Name != token {
				m.completions = append(m.completions, completionItem{start: start, end: end, replacement: command.Name + " ", label: command.Usage, detail: command.Description})
			}
		}
	}
	for _, contribution := range m.extensionContributions {
		if contribution.Kind != string(ExtensionAutocomplete) || !strings.HasPrefix(token, contribution.Trigger) {
			continue
		}
		m.completions = append(m.completions, completionItem{start: start, end: end, replacement: contribution.Value, label: contribution.Text, detail: contribution.Detail})
	}
	if len(m.completions) > maxCompletions {
		m.completions = m.completions[:maxCompletions]
	}
	service, ok := m.service.(ComposerCompleter)
	if !ok || !strings.HasPrefix(token, "$") && !strings.HasPrefix(token, "@") {
		return nil
	}
	generation := m.completionGeneration
	input, line, column := m.composer.editor.Value(), m.composer.editor.Line(), m.composer.editor.Column()
	ctx, cancel := context.WithCancel(m.ctx)
	m.composer.completionCancel = cancel
	return func() tea.Msg {
		defer cancel()
		items, err := service.CompleteComposer(ctx, token)
		return completionsMsg{generation: generation, input: input, line: line, column: column, start: start, end: end, items: items, err: err}
	}
}

func (m *model) receiveCompletions(msg completionsMsg) {
	if msg.generation != m.completionGeneration || msg.err != nil || msg.input != m.composer.editor.Value() || msg.line != m.composer.editor.Line() || msg.column != m.composer.editor.Column() {
		return
	}
	for _, item := range msg.items {
		m.completions = append(m.completions, completionItem{start: msg.start, end: msg.end, replacement: item.Value, label: safeText(item.Label), detail: safeText(item.Detail)})
	}
	if len(m.completions) > maxCompletions {
		m.completions = m.completions[:maxCompletions]
	}
}

func editorToken(value string, lineIndex, column int) (string, int, int, string) {
	lines := strings.Split(value, "\n")
	lineIndex = max(0, min(lineIndex, len(lines)-1))
	line := lines[lineIndex]
	runes := []rune(line)
	column = max(0, min(column, len(runes)))
	startColumn := column
	for startColumn > 0 && runes[startColumn-1] != ' ' && runes[startColumn-1] != '\t' {
		startColumn--
	}
	endColumn := column
	for endColumn < len(runes) && runes[endColumn] != ' ' && runes[endColumn] != '\t' {
		endColumn++
	}
	lineOffset := 0
	for i := 0; i < lineIndex; i++ {
		lineOffset += len(lines[i]) + 1
	}
	start := lineOffset + len(string(runes[:startColumn]))
	end := lineOffset + len(string(runes[:endColumn]))
	return line, start, end, string(runes[startColumn:column])
}

func applyCompletion(editor *textarea.Model, completion completionItem) {
	value := editor.Value()
	if completion.start < 0 || completion.end < completion.start || completion.end > len(value) || !utf8.ValidString(value[:completion.start]) {
		return
	}
	updated := value[:completion.start] + completion.replacement + value[completion.end:]
	editor.SetValue(updated)
	newPosition := completion.start + len(completion.replacement)
	before := updated[:newPosition]
	targetLine := strings.Count(before, "\n")
	for editor.Line() > targetLine {
		editor.CursorUp()
	}
	lineStart := strings.LastIndex(before, "\n") + 1
	editor.SetCursorColumn(utf8.RuneCountInString(before[lineStart:]))
}
