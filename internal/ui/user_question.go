package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/diasYuri/opera/internal/app"
)

type questionAnswerer interface {
	AnswerQuestion(string, string, []app.UserQuestionAnswer) error
}

type questionFlow struct {
	runID, toolCallID string
	questions         []app.UserQuestion
	index             int
	selected          []int
	answers           []app.UserQuestionAnswer
	notes             []string
	input             textarea.Model
	focusInput        bool
}

func newQuestionFlow(runID string, data json.RawMessage) *questionFlow {
	var payload struct {
		ToolCall struct {
			ID        string          `json:"id"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"tool_call"`
	}
	if json.Unmarshal(data, &payload) != nil || payload.ToolCall.ID == "" {
		return nil
	}
	var input struct {
		Questions []app.UserQuestion `json:"questions"`
	}
	if json.Unmarshal(payload.ToolCall.Arguments, &input) != nil || len(input.Questions) == 0 {
		return nil
	}
	flow := &questionFlow{runID: runID, toolCallID: payload.ToolCall.ID, questions: input.Questions, selected: make([]int, len(input.Questions)), answers: make([]app.UserQuestionAnswer, len(input.Questions)), notes: make([]string, len(input.Questions)), input: textarea.New()}
	flow.input.Placeholder = "Add details or type a custom answer…"
	flow.input.ShowLineNumbers = false
	flow.input.SetHeight(2)
	return flow
}

func (m *model) openUserQuestion(event *app.Event) {
	if event == nil || event.Type != "tool_started" || event.Tool != "user_question" {
		return
	}
	flow := newQuestionFlow(event.RunID, event.Data)
	if flow == nil {
		return
	}
	m.question = flow
	m.status = "Opera is waiting for your answer"
	m.resize()
}

func (m model) questionView() string {
	q := m.question
	if q == nil || q.index >= len(q.questions) {
		return ""
	}
	t := m.theme()
	item := q.questions[q.index]
	options := append([]app.UserQuestionOption(nil), item.Options...)
	options = append(options, app.UserQuestionOption{Label: "None of the above", Description: "Add your own answer or details with Tab"})
	width := max(1, m.contentWidth())
	rows := []string{t.muted.Render(fmt.Sprintf("Question %d/%d (%d unanswered)", q.index+1, len(q.questions), len(q.questions)-q.index)), t.accent.Render(safeText(item.Question))}
	for i, option := range options {
		marker, rowStyle := "  ", t.text
		if i == q.selected[q.index] {
			marker, rowStyle = "› ", t.accent.Bold(true)
		}
		label := fmt.Sprintf("%s%d. %s", marker, i+1, safeText(option.Label))
		description := safeText(option.Description)
		if m.width >= 72 && description != "" {
			labelWidth := max(10, width/3)
			rows = append(rows, rowStyle.Render(ansi.Truncate(label, labelWidth, "…"))+"  "+t.muted.Render(ansi.Truncate(description, max(1, width-labelWidth-3), "…")))
		} else {
			rows = append(rows, rowStyle.Render(ansi.Truncate(label, width, "…")))
			if m.width < 72 && description != "" {
				rows = append(rows, "   "+t.muted.Render(ansi.Truncate(description, max(1, width-3), "…")))
			}
		}
	}
	if q.focusInput {
		rows = append(rows, "", q.input.View())
	}
	hint := "↑↓ choose · Tab add notes / custom text · Enter answer · ←/→ questions · Esc interrupt"
	if m.width < 60 {
		hint = "↑↓ choose · Tab notes · Enter answer · ←/→ questions · Esc interrupt"
	}
	rows = append(rows, t.muted.Render(ansi.Truncate(hint, width, "…")))
	return strings.Join(rows, "\n")
}

func (m model) updateQuestionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	q := m.question
	if q == nil {
		return m, nil, false
	}
	if q.focusInput {
		switch msg.String() {
		case "tab", "enter":
			q.notes[q.index] = strings.TrimSpace(q.input.Value())
			q.focusInput = false
			q.input.Blur()
			return m, nil, true
		case "esc":
			q.focusInput = false
			q.input.Blur()
			return m, nil, true
		default:
			var cmd tea.Cmd
			q.input, cmd = q.input.Update(msg)
			return m, cmd, true
		}
	}
	options := len(q.questions[q.index].Options) + 1
	switch msg.String() {
	case "up", "k":
		q.selected[q.index] = (q.selected[q.index] + options - 1) % options
	case "down", "j":
		q.selected[q.index] = (q.selected[q.index] + 1) % options
	case "left":
		if q.index > 0 {
			q.index--
		}
	case "right":
		if q.index+1 < len(q.questions) {
			q.index++
		}
	case "tab":
		q.focusInput = true
		q.input.SetValue(q.notes[q.index])
		q.input.Focus()
	case "enter":
		question := q.questions[q.index]
		optionIndex := q.selected[q.index]
		answer := app.UserQuestionAnswer{Question: question.Question, Notes: q.notes[q.index]}
		if optionIndex < len(question.Options) {
			answer.Option = question.Options[optionIndex].Label
		}
		if optionIndex == len(question.Options) {
			answer.Text = q.notes[q.index]
			answer.Notes = ""
		}
		q.answers[q.index] = answer
		if q.index+1 < len(q.questions) {
			q.index++
		} else {
			answerer, ok := m.service.(questionAnswerer)
			if !ok {
				m.status = "Question input is unavailable"
				return m, nil, true
			}
			if err := answerer.AnswerQuestion(q.runID, q.toolCallID, q.answers); err != nil {
				m.status = summarizeError(err.Error())
				m.lastError = err.Error()
				return m, nil, true
			}
			m.question = nil
			m.status = "Answer sent to Opera"
			m.resize()
		}
	case "esc":
		if m.stream != nil {
			m.stream.Cancel()
			m.status = "Interrupting execution…"
		}
		m.question = nil
		m.resize()
	case "ctrl+c", "ctrl+d":
		return m, nil, false
	default:
		return m, nil, true
	}
	m.resize()
	return m, nil, true
}
