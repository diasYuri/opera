package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/diasYuri/opera/internal/modules/execution"
)

type conversationModel struct {
	viewport.Model
	entries      []conversationEntry
	entryIndexes map[string]int
	nextMessage  uint64
	committed    map[string]bool
	display      string
	activity     string
}

func newConversationModel() conversationModel {
	return conversationModel{Model: viewport.New(), display: "full"}
}

type conversationKind string

const (
	conversationUser      conversationKind = "user"
	conversationAssistant conversationKind = "assistant"
	conversationThinking  conversationKind = "thinking"
	conversationTool      conversationKind = "tool"
	conversationError     conversationKind = "error"
	conversationSystem    conversationKind = "system"
	conversationVisual    conversationKind = "visual-event"
)

type visualEventKind string

const (
	visualCompaction   visualEventKind = "compaction"
	visualBranch       visualEventKind = "branch"
	visualSkill        visualEventKind = "skill"
	visualAgent        visualEventKind = "agent"
	visualAnnouncement visualEventKind = "announcement"
)

type visualEvent struct {
	Kind  visualEventKind
	Title string
	Body  string
	State string
}

type conversationEntry struct {
	id, text, toolID, toolName, arguments, state string
	visual                                       visualEvent
	rendered                                     string
	renderedWidth                                int
	renderedDark, renderedNoColor                bool
	kind                                         conversationKind
	failed                                       bool
	intermediate, awaitingClassification         bool
}

type eventPayload struct {
	ToolCall struct {
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"tool_call"`
	ToolResult struct {
		ToolCallID string `json:"tool_call_id"`
		ToolName   string `json:"tool_name"`
		IsError    bool   `json:"is_error"`
		Content    []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"tool_result"`
	ToolProgress struct {
		ToolCallID string `json:"tool_call_id"`
		ToolName   string `json:"tool_name"`
		Stream     string `json:"stream"`
		Text       string `json:"text"`
		Done       bool   `json:"done"`
	} `json:"tool_progress"`
	ThinkingDelta string `json:"thinking_delta"`
	Error         *struct {
		Message string `json:"message"`
	} `json:"error"`
	Agent struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"agent"`
	Skill struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"skill"`
	Announcement struct {
		Title   string `json:"title"`
		Message string `json:"message"`
	} `json:"announcement"`
	Compaction struct {
		Message string `json:"message"`
		Summary string `json:"summary"`
	} `json:"compaction"`
}

func (c *conversationModel) append(entry conversationEntry) int {
	if c.entryIndexes == nil {
		c.entryIndexes = make(map[string]int)
	}
	if index, exists := c.entryIndexes[entry.id]; exists {
		c.entries[index] = entry
		return index
	}
	c.entryIndexes[entry.id] = len(c.entries)
	c.entries = append(c.entries, entry)
	return len(c.entries) - 1
}

func (c *conversationModel) nextIdentity(prefix string) string {
	c.nextMessage++
	return fmt.Sprintf("%s:%d", prefix, c.nextMessage)
}

func (c *conversationModel) resetLedger() {
	c.entries = nil
	c.entryIndexes = make(map[string]int)
	c.activity = ""
}

func (c *conversationModel) commitCheckpoint(identity string) bool {
	if c.committed == nil {
		c.committed = make(map[string]bool)
	}
	if c.committed[identity] {
		return false
	}
	c.committed[identity] = true
	return true
}

func (m *model) appendEntry(entry conversationEntry) int { return m.conversation.append(entry) }

func (m *model) appendUser(text, label string) {
	if label == "" {
		label = "You"
	}
	m.appendEntry(conversationEntry{id: m.conversation.nextIdentity("user"), kind: conversationUser, text: text, state: label})
	m.refreshConversation()
}

func (m *model) appendVisualEvent(id string, kind visualEventKind, title, body, state string) {
	m.appendEntry(conversationEntry{
		id: id, kind: conversationVisual, state: state,
		visual: visualEvent{Kind: kind, Title: safeText(title), Body: safeText(body), State: safeText(state)},
	})
}

func (m *model) commitQueuedMessages() {
	for _, text := range m.queuedMessages {
		m.appendEntry(conversationEntry{id: m.conversation.nextIdentity("user"), kind: conversationUser, text: text, state: "You"})
	}
	m.queuedMessages = nil
	m.resize()
}

func (m *model) refreshConversation() {
	wasAtBottom := m.conversation.AtBottom()
	if len(m.conversation.entries) == 0 {
		wasAtBottom = true
	}
	m.conversation.SoftWrap = true
	m.conversation.SetWidth(max(1, m.conversationWidth()))
	_, noColor := os.LookupEnv("NO_COLOR")
	for i := range m.conversation.entries {
		entry := &m.conversation.entries[i]
		if entry.kind == conversationAssistant && m.conversation.shows(*entry) && (entry.rendered == "" || entry.renderedWidth != m.conversationWidth() || entry.renderedDark != m.dark || entry.renderedNoColor != noColor) {
			entry.rendered = m.renderMarkdown(entry.text)
			entry.renderedWidth = m.conversationWidth()
			entry.renderedDark = m.dark
			entry.renderedNoColor = noColor
		}
	}
	content := m.renderConversation()
	changed := content != m.conversation.GetContent()
	if changed {
		m.conversation.SetContent(content)
	}
	if wasAtBottom {
		m.conversation.GotoBottom()
		m.newConversationItems = false
	} else if changed {
		m.newConversationItems = true
	}
}

func (m *model) updateConversationEvent(event *execution.Event) {
	var payload eventPayload
	if len(event.Data) > 0 {
		_ = json.Unmarshal(event.Data, &payload)
	}
	if handled, changed := m.updateTypedVisualEvent(event, payload); handled {
		if changed {
			m.refreshConversation()
		}
		return
	}
	switch event.Type {
	case "message_started":
		if _, exists := m.conversation.entryIndexes[m.currentAssistantID]; exists {
			m.advanceAssistantSegment(event)
		}
		m.conversation.activity = "Thinking…"
	case "message_delta", "text_delta":
		if event.Text != "" {
			if m.streamBuffer == nil {
				m.streamBuffer = &strings.Builder{}
				_, _ = m.streamBuffer.WriteString(m.text)
			}
			_, _ = m.streamBuffer.WriteString(event.Text)
			m.text = m.streamBuffer.String()
			id := m.currentAssistantID
			if id == "" {
				id = "assistant:" + event.RunID
				m.currentAssistantID = id
			}
			entry := m.entry(id, conversationEntry{id: id, kind: conversationAssistant, state: "Opera", awaitingClassification: true})
			entry.text += event.Text
			entry.rendered = ""
			entry.renderedWidth = 0
			m.appendEntry(entry)
			m.conversation.activity = activityPreview(entry.text)
		}
	case "message_completed":
		if text, intermediate, ok := event.CompletedAssistantMessage(); ok {
			id := m.currentAssistantID
			if id == "" {
				id = "assistant:" + event.RunID
				m.currentAssistantID = id
			}
			entry := m.entry(id, conversationEntry{id: id, kind: conversationAssistant, state: "Opera", text: text})
			entry.intermediate, entry.awaitingClassification = intermediate, false
			if entry.text != "" {
				m.appendEntry(entry)
			}
			m.conversation.activity = "Finishing response…"
			if intermediate {
				m.conversation.activity = activityPreview(entry.text)
				if m.conversation.activity == "" {
					m.conversation.activity = "Preparing tools…"
				}
			}
		}
	case "thinking_delta":
		if payload.ThinkingDelta != "" {
			m.conversation.activity = "Thinking…"
			id := m.currentAssistantID + ":thinking"
			if id == ":thinking" {
				id = "thinking:" + event.RunID
			}
			index := m.conversation.entryIndexes[id]
			text := payload.ThinkingDelta
			if _, exists := m.conversation.entryIndexes[id]; exists {
				text = m.conversation.entries[index].text + text
			}
			m.appendEntry(conversationEntry{id: id, kind: conversationThinking, text: text, state: "Thinking"})
		}
	case "tool_started":
		m.markAssistantCommentary()
		id := payload.ToolCall.ID
		if id == "" {
			id = fmt.Sprintf("%s:%d", event.RunID, event.Sequence)
		}
		toolName := strings.ToLower(payload.ToolCall.Name)
		if strings.Contains(toolName, "skill") && toolName != "skill_catalog" {
			name := payload.Skill.Name
			var skillArgs struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(payload.ToolCall.Arguments, &skillArgs)
			if name == "" {
				name = skillArgs.Name
			}
			if name == "" {
				name = payload.ToolCall.Name
			}
			body := payload.Skill.Description
			if body == "" {
				if toolName == "skill_resource" {
					body = "Skill resource read"
				} else {
					body = "Skill loaded for this execution"
				}
			}
			m.appendVisualEvent("skill:"+id, visualSkill, name, body, "Active")
		}
		m.appendEntry(conversationEntry{id: "tool:" + id, kind: conversationTool, toolID: id, toolName: payload.ToolCall.Name, arguments: string(payload.ToolCall.Arguments), state: "Running"})
		m.advanceAssistantSegment(event)
		m.conversation.activity = toolActivity("Running", payload.ToolCall.Name)
	case "tool_progress":
		id := payload.ToolProgress.ToolCallID
		if id == "" {
			id = fmt.Sprintf("%s:%d", event.RunID, event.Sequence)
		}
		key := "tool:" + id
		entry := m.entry(key, conversationEntry{id: key, kind: conversationTool, toolID: id, toolName: payload.ToolProgress.ToolName})
		if payload.ToolProgress.Text != "" {
			chunk := payload.ToolProgress.Text
			if payload.ToolProgress.Stream != "" {
				chunk = "[" + payload.ToolProgress.Stream + "] " + chunk
			}
			separator := ""
			if entry.text != "" {
				separator = "\n"
			}
			entry.text += separator + chunk
		}
		if payload.ToolProgress.Done {
			entry.state = "Completed"
		} else {
			entry.state = "Running"
			m.conversation.activity = toolActivity("Running", entry.toolName)
		}
		m.appendEntry(entry)
	case "tool_completed":
		id := payload.ToolResult.ToolCallID
		if id == "" {
			id = fmt.Sprintf("%s:%d", event.RunID, event.Sequence)
		}
		key := "tool:" + id
		entry := m.entry(key, conversationEntry{id: key, kind: conversationTool, toolID: id, toolName: payload.ToolResult.ToolName})
		if payload.ToolResult.ToolName != "" {
			entry.toolName = payload.ToolResult.ToolName
		}
		if len(payload.ToolResult.Content) > 0 {
			var lines []string
			for _, block := range payload.ToolResult.Content {
				if block.Text != "" {
					lines = append(lines, block.Text)
				}
			}
			entry.text = strings.Join(lines, "\n")
		}
		entry.failed = payload.ToolResult.IsError
		entry.state = "Completed"
		if entry.failed {
			entry.state = "Failed"
		}
		m.conversation.activity = toolActivity(entry.state, entry.toolName)
		m.appendEntry(entry)
	case "retry_scheduled":
		text := "A retry is scheduled"
		if payload.Error != nil && payload.Error.Message != "" {
			text += ": " + payload.Error.Message
		}
		m.appendEntry(conversationEntry{id: fmt.Sprintf("system:%s:%d", event.RunID, event.Sequence), kind: conversationSystem, text: text})
	}
	m.refreshConversation()
}

func (m *model) advanceAssistantSegment(event *execution.Event) {
	m.assistantSegment++
	runID := event.RunID
	if runID == "" && m.stream != nil {
		runID = m.stream.ID
	}
	m.currentAssistantID = fmt.Sprintf("assistant:%s:%d:%d", runID, m.runGeneration, m.assistantSegment)
}

func (m *model) markAssistantCommentary() {
	if index, exists := m.conversation.entryIndexes[m.currentAssistantID]; exists {
		entry := &m.conversation.entries[index]
		entry.intermediate, entry.awaitingClassification = true, false
	}
}

// updateTypedVisualEvent handles state-machine notices before normal speech and
// tool rendering. Child run deltas stay attached to their agent event instead
// of appearing as parent assistant speech.
func (m *model) updateTypedVisualEvent(event *execution.Event, payload eventPayload) (handled, changed bool) {
	if event.ParentRunID != "" {
		switch event.Type {
		case "agent_started", "agent_completed", "agent_settled", "completed":
		default:
			return true, false
		}
		id := "agent:" + event.RunID
		title := payload.Agent.Name
		if title == "" {
			title = "Subagent"
		}
		state := "Running"
		body := "Working on a delegated task"
		switch event.Type {
		case "agent_started":
			m.appendVisualEvent(id, visualAgent, title, body, state)
		case "agent_completed", "agent_settled", "completed":
			if payload.Agent.Status != "" {
				state = payload.Agent.Status
			} else {
				state = "Completed"
			}
			body = "Delegated task finished"
			m.appendVisualEvent(id, visualAgent, title, body, state)
		}
		return true, true
	}
	switch event.Type {
	case "compaction_started":
		body := payload.Compaction.Message
		if body == "" {
			body = "Preparing a compact context for the next turn"
		}
		m.appendVisualEvent(fmt.Sprintf("compaction:%s:%d", event.RunID, event.Sequence), visualCompaction, "Context compaction", body, "Running")
	case "compaction_completed":
		body := payload.Compaction.Summary
		if body == "" {
			body = payload.Compaction.Message
		}
		if body == "" {
			body = "Context compaction completed"
		}
		m.appendVisualEvent(fmt.Sprintf("compaction:%s:%d", event.RunID, event.Sequence), visualCompaction, "Context compaction", body, "Completed")
	case "system_announcement", "announcement", "system_notice":
		title, body := payload.Announcement.Title, payload.Announcement.Message
		if title == "" {
			title = "Announcement"
		}
		if body == "" {
			body = event.Text
		}
		m.appendVisualEvent(fmt.Sprintf("announcement:%s:%d", event.RunID, event.Sequence), visualAnnouncement, title, body, "")
	default:
		return false, false
	}
	return true, true
}

func (m *model) entry(id string, fallback conversationEntry) conversationEntry {
	if index, exists := m.conversation.entryIndexes[id]; exists {
		return m.conversation.entries[index]
	}
	return fallback
}

func (m model) renderConversation() string {
	var blocks []string
	for block := range m.conversation.blocks {
		if rendered := m.renderConversationBlock(block); rendered != "" {
			blocks = append(blocks, rendered)
		}
	}
	return strings.Join(blocks, "\n")
}

func (m model) renderEntry(entry conversationEntry) string {
	t := m.theme()
	label := entry.state
	var body string
	switch entry.kind {
	case conversationAssistant:
		body = entry.rendered
		if body == "" {
			return ""
		}
		return "\n" + lipgloss.NewStyle().PaddingLeft(2).Render(body)
	case conversationUser:
		return m.renderUserEntry(entry, t)
	case conversationThinking:
		label = m.extensionRendererLabel("thinking", "Thinking")
		body = ansi.Hardwrap(safeText(entry.text), max(1, m.conversationWidth()-2), true)
	case conversationTool:
		stateStyle := t.muted
		if entry.failed {
			stateStyle = t.error
		} else if entry.state == "Completed" {
			stateStyle = t.success
		}
		toolLabel := safeText(entry.toolName)
		if entry.toolName == "" {
			toolLabel = "Tool"
		}
		label = m.extensionRendererLabel("tool", toolLabel) + " · " + entry.state
		body = toolBody(entry, m.conversationWidth(), m.toolDetailsExpanded)
		return "\n" + t.accent.Bold(true).Render("  ▸ "+label) + "\n" + lipgloss.NewStyle().PaddingLeft(4).Render(stateStyle.Render(body))
	case conversationError:
		label = m.extensionRendererLabel("error", "Opera · error")
		body = ansi.Hardwrap(safeText(summarizeError(entry.text)), max(1, m.conversationWidth()-2), true)
	case conversationSystem:
		label = m.extensionRendererLabel("system", "Status")
		body = ansi.Hardwrap(safeText(entry.text), max(1, m.conversationWidth()-2), true)
	case conversationVisual:
		label = m.extensionRendererLabel(string(entry.visual.Kind), entry.visual.Title)
		body = ansi.Hardwrap(safeText(entry.visual.Body), max(1, m.conversationWidth()-2), true)
		style := t.info.Bold(true)
		switch entry.visual.Kind {
		case visualBranch, visualSkill:
			style = t.accent.Bold(true)
		case visualAgent:
			style = t.info.Bold(true)
		case visualAnnouncement:
			style = t.warning.Bold(true)
		case visualCompaction:
			style = t.muted.Bold(true)
		}
		if entry.visual.State != "" {
			label += " · " + entry.visual.State
		}
		return "\n" + lipgloss.NewStyle().PaddingLeft(2).Render(style.Render("· "+safeText(label))) + "\n" + lipgloss.NewStyle().PaddingLeft(4).Render(body)
	}
	labelStyle := t.text.Bold(true)
	if entry.kind == conversationAssistant {
		labelStyle = t.accent.Bold(true)
	}
	if entry.kind == conversationError {
		labelStyle = t.error.Bold(true)
	}
	return "\n" + lipgloss.NewStyle().PaddingLeft(2).Render(labelStyle.Render(label)) + "\n" + lipgloss.NewStyle().PaddingLeft(4).Render(body)
}

func (m model) renderUserEntry(entry conversationEntry, t palette) string {
	width := m.conversationWidth()
	text := safeText(entry.text)
	padding := 1
	if width < 4 {
		padding = 0
	}
	textWidth := max(1, width-2*padding)
	bubble := t.userBubble.Width(textWidth).Padding(0, padding).Render(ansi.Wrap(text, textWidth, " "))
	return "\n" + lipgloss.PlaceHorizontal(width, lipgloss.Right, bubble)
}

func (m model) renderMarkdown(text string) string {
	clean := safeText(text)
	if clean == "" {
		return ""
	}
	style := "dark"
	if !m.dark {
		style = "light"
	}
	if _, noColor := os.LookupEnv("NO_COLOR"); noColor {
		style = "notty"
	}
	width := max(1, m.conversationWidth()-4)
	if len(clean) > 64*1024 {
		return ansi.Hardwrap(clean, width, true)
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(width))
	if err != nil {
		return ansi.Hardwrap(clean, width, true)
	}
	rendered, err := renderer.Render(clean)
	closeErr := renderer.Close()
	if err != nil || closeErr != nil {
		return ansi.Hardwrap(clean, width, true)
	}
	return strings.TrimSpace(rendered)
}

func (m model) conversationWidth() int { return max(1, m.contentWidth()-2) }

func toolBody(entry conversationEntry, width int, expanded bool) string {
	var parts []string
	if !expanded {
		if entry.text == "" && entry.state == "Running" {
			return "Operation in progress · press Ctrl+O to inspect details"
		}
		lines := strings.Split(safeText(entry.text), "\n")
		if entry.text == "" {
			return "Completed with no text output"
		}
		if len(lines) > 3 {
			lines = append(lines[:3], "… (Ctrl+O to expand)")
		}
		return ansi.Hardwrap(strings.Join(lines, "\n"), max(1, width-6), true)
	}
	if entry.toolID != "" {
		parts = append(parts, "ID: "+entry.toolID)
	}
	if entry.arguments != "" {
		var value any
		if json.Unmarshal([]byte(entry.arguments), &value) == nil {
			if formatted, err := json.MarshalIndent(value, "", "  "); err == nil {
				entry.arguments = string(formatted)
			}
		}
		parts = append(parts, "Arguments: "+entry.arguments)
	}
	if entry.text != "" {
		parts = append(parts, entry.text)
	}
	text := safeText(strings.Join(parts, "\n"))
	return ansi.Hardwrap(text, max(1, width-6), true)
}
