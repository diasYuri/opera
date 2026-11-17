package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/diasYuri/opera/internal/app"
)

// TaskItem is the presentation-safe portion of a persisted Opera task.
type TaskItem struct {
	ID          string
	ParentID    string
	Name        string
	Description string
	Order       int
	Status      string
}

type TaskSnapshot struct {
	Tasks []TaskItem
}

// TaskSnapshotReader lets the terminal UI observe task state without coupling
// the UI package to the persistence or task-manager implementation.
type TaskSnapshotReader interface {
	TaskSnapshot(context.Context, string) (TaskSnapshot, error)
}

type todoRow struct {
	text   string
	active bool
}

func (m *model) loadTaskSnapshot(runID string) tea.Cmd {
	if runID == "" {
		return nil
	}
	reader, ok := m.service.(TaskSnapshotReader)
	if !ok {
		return nil
	}
	m.todoGeneration++
	generation := m.todoGeneration
	ctx := m.ctx
	return func() tea.Msg {
		snapshot, err := reader.TaskSnapshot(ctx, runID)
		return taskSnapshotMsg{runID: runID, generation: generation, snapshot: snapshot, err: err}
	}
}

func (m model) isTaskToolCompletion(event *app.Event) bool {
	if event == nil || event.Type != "tool_completed" {
		return false
	}
	var payload eventPayload
	if json.Unmarshal(event.Data, &payload) != nil {
		return false
	}
	return strings.HasPrefix(payload.ToolResult.ToolName, "task_")
}

func (m model) todoListHeight() int {
	return len(m.todoLines())
}

func (m model) todoLines() []string {
	if m.stream == nil || len(m.todoTasks) == 0 {
		return nil
	}
	t := m.theme()
	done := 0
	for _, task := range m.todoTasks {
		if task.Status == "completed" {
			done++
		}
	}
	lines := []string{t.accent.Bold(true).Render(fmt.Sprintf("● Todos (%d/%d)", done, len(m.todoTasks)))}
	rows := m.todoRows(t)
	if len(rows) == 0 {
		return lines
	}
	visible := min(4, max(1, m.height-12))
	active := -1
	for i := range rows {
		if rows[i].active {
			active = i
		}
	}
	start := 0
	if active >= visible {
		start = active - visible + 1
	}
	end := min(len(rows), start+visible)
	if start > 0 {
		lines = append(lines, t.muted.Render(fmt.Sprintf("… +%d earlier", start)))
	}
	for _, row := range rows[start:end] {
		lines = append(lines, ansi.Truncate(row.text, max(1, m.contentWidth()), "…"))
	}
	if end < len(rows) {
		lines = append(lines, t.muted.Render(fmt.Sprintf("… +%d more", len(rows)-end)))
	}
	return lines
}

func (m model) todoRows(t palette) []todoRow {
	children := make(map[string][]TaskItem, len(m.todoTasks))
	known := make(map[string]bool, len(m.todoTasks))
	for _, task := range m.todoTasks {
		known[task.ID] = true
	}
	for _, task := range m.todoTasks {
		parent := task.ParentID
		if parent != "" && !known[parent] {
			parent = ""
		}
		children[parent] = append(children[parent], task)
	}
	for parent := range children {
		sort.SliceStable(children[parent], func(i, j int) bool {
			if children[parent][i].Order == children[parent][j].Order {
				return children[parent][i].ID < children[parent][j].ID
			}
			return children[parent][i].Order < children[parent][j].Order
		})
	}

	rows := make([]todoRow, 0, len(m.todoTasks))
	visited := make(map[string]bool, len(m.todoTasks))
	var walk func(string, string)
	walk = func(parent, prefix string) {
		items := children[parent]
		for i, task := range items {
			if visited[task.ID] {
				continue
			}
			visited[task.ID] = true
			last := i == len(items)-1
			branch := "├─"
			if last {
				branch = "└─"
			}
			marker, name := t.muted.Render("○"), t.text.Render(safeText(task.Name))
			active := task.Status == "in_progress"
			switch task.Status {
			case "completed":
				marker = t.success.Render("✓")
			case "in_progress":
				marker = t.warning.Render("◐")
				name = t.warning.Bold(true).Render(safeText(task.Name))
			}
			detail := strings.TrimSpace(safeText(task.Description))
			if detail != "" && detail != strings.TrimSpace(safeText(task.Name)) {
				name += " " + t.muted.Render("("+detail+")")
			}
			rows = append(rows, todoRow{text: t.muted.Render(prefix+branch) + " " + marker + " " + name, active: active})
			childPrefix := prefix + "   "
			if !last {
				childPrefix = prefix + "│  "
			}
			walk(task.ID, childPrefix)
		}
	}
	walk("", "")
	return rows
}

func (m model) todoListView() string {
	return strings.Join(m.todoLines(), "\n")
}
