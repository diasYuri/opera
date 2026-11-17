package ui

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/skills"
)

const maxCompletions = 6
const maxFileCompletionScan = 512

type completionItem struct {
	start, end  int
	replacement string
	label       string
	detail      string
}

func (m *model) refreshCompletions() {
	_, start, end, token := editorToken(m.composer.editor.Value(), m.composer.editor.Line(), m.composer.editor.Column())
	m.completions = nil
	m.completionIndex = 0
	if token == "" || m.composer.editor.HasSelection() {
		return
	}
	if strings.HasPrefix(token, "/") && start == 0 {
		for _, command := range app.Commands {
			if strings.HasPrefix(command.Name, token) && command.Name != token {
				m.completions = append(m.completions, completionItem{start: start, end: end, replacement: command.Name + " ", label: command.Usage, detail: command.Description})
			}
		}
	} else if strings.HasPrefix(token, "$") {
		query := strings.TrimPrefix(token, "$")
		for _, skill := range m.skillsCatalog().Skills {
			if strings.HasPrefix(strings.ToLower(skill.Name), strings.ToLower(query)) {
				m.completions = append(m.completions, completionItem{start: start, end: end, replacement: "$" + skill.Name + " ", label: "$" + skill.Name, detail: skill.Description})
			}
		}
	} else if strings.HasPrefix(token, "@") {
		m.completions = m.fileCompletions(token, start, end)
	}
	for _, contribution := range m.extensionContributions {
		if contribution.Kind != string(ExtensionAutocomplete) || !strings.HasPrefix(token, contribution.Trigger) {
			continue
		}
		m.completions = append(m.completions, completionItem{
			start: start, end: end, replacement: contribution.Value,
			label: contribution.Text, detail: contribution.Detail,
		})
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

func (m *model) skillsCatalog() skills.Catalog {
	if !m.skillCatalogAt.IsZero() && time.Since(m.skillCatalogAt) < 5*time.Second {
		return m.skillCatalogCache
	}
	root := m.workspaceRoot
	if root == "" {
		root, _ = os.Getwd()
	}
	home, _ := os.UserHomeDir()
	m.skillCatalogCache = skills.Discover(root, home)
	m.skillCatalogAt = time.Now()
	return m.skillCatalogCache
}

func (m *model) fileCompletions(token string, start, end int) []completionItem {
	root := m.workspaceRoot
	if root == "" {
		root, _ = os.Getwd()
	}
	query := strings.TrimPrefix(token, "@")
	dirPart, base := filepath.Split(query)
	dir := filepath.Clean(filepath.Join(root, dirPart))
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	parent, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer func() { _ = parent.Close() }()
	entries, err := parent.ReadDir(maxFileCompletionScan + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil
	}
	scanLimited := len(entries) > maxFileCompletionScan
	if scanLimited {
		entries = entries[:maxFileCompletionScan]
	}
	var items []completionItem
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") || name == ".git" || name == ".opera" || name == "node_modules" || name == "vendor" {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(base)) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.ToSlash(filepath.Join(dirPart, name))
		detail := "file"
		if entry.IsDir() {
			detail = "directory"
		}
		if scanLimited {
			detail += " · large directory scan capped"
		}
		if entry.IsDir() {
			path += "/"
			items = append(items, completionItem{start: start, end: end, replacement: "@" + path, label: "@" + path, detail: detail})
		} else {
			items = append(items, completionItem{start: start, end: end, replacement: "@" + path + " ", label: "@" + path, detail: detail})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].detail != items[j].detail {
			return strings.HasPrefix(items[i].detail, "directory")
		}
		return strings.ToLower(items[i].label) < strings.ToLower(items[j].label)
	})
	return items
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
