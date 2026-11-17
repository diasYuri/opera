package ui

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/iterm2"
	"github.com/charmbracelet/x/ansi/kitty"
	"golang.org/x/term"
)

type AttachmentPreview struct {
	Path, Kind, MIMEType string
	Bytes, Width, Height int
	Data                 []byte
	Inline               string
}

type AttachmentPreviewProvider interface {
	AttachmentPreviews(context.Context) ([]AttachmentPreview, error)
}

// ClipboardImageProvider is implemented by the delivery adapter because
// clipboard access is an operating-system effect, not a presentation concern.
type ClipboardImageProvider interface {
	ReadClipboardImage(context.Context) ([]byte, error)
}

type attachmentPreviewsMsg struct {
	items []AttachmentPreview
	err   error
}

type imageProtocol string

const (
	imageProtocolNone  imageProtocol = ""
	imageProtocolKitty imageProtocol = "kitty"
	imageProtocolITerm imageProtocol = "iterm2"
)

func terminalImageProtocol(output io.Writer) imageProtocol {
	file, ok := output.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return imageProtocolNone
	}
	if term := strings.ToLower(os.Getenv("TERM")); term == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "" || strings.EqualFold(os.Getenv("TERM_PROGRAM"), "ghostty") {
		return imageProtocolKitty
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm":
		return imageProtocolITerm
	default:
		return imageProtocolNone
	}
}

func (m model) loadAttachmentPreviews() tea.Cmd {
	provider, ok := m.service.(AttachmentPreviewProvider)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		items, err := provider.AttachmentPreviews(m.ctx)
		return attachmentPreviewsMsg{items: items, err: err}
	}
}

func (m model) readClipboardImage() tea.Cmd {
	provider, ok := m.service.(ClipboardImageProvider)
	if !ok {
		return func() tea.Msg { return clipboardImageMsg{err: fmt.Errorf("image clipboard is unavailable")} }
	}
	return func() tea.Msg {
		data, err := provider.ReadClipboardImage(m.ctx)
		if err != nil || len(data) == 0 || len(data) > 16<<20 {
			return clipboardImageMsg{err: fmt.Errorf("no supported image found in clipboard")}
		}
		if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
			return clipboardImageMsg{err: fmt.Errorf("clipboard image is not a valid PNG")}
		}
		return clipboardImageMsg{data: data}
	}
}

func savePastedImage(root string, data []byte) tea.Cmd {
	return func() tea.Msg {
		file, err := os.CreateTemp(root, ".opera-paste-*.png")
		if err != nil {
			return pastedImageSavedMsg{err: err}
		}
		path := file.Name()
		if _, err = file.Write(data); err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
			return pastedImageSavedMsg{err: err}
		}
		return pastedImageSavedMsg{path: path}
	}
}

func removePastedImages(paths []string, root string) {
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil || filepath.Dir(absolute) != filepath.Clean(root) || !strings.HasPrefix(filepath.Base(absolute), ".opera-paste-") {
			continue
		}
		_ = os.Remove(absolute)
	}
}

func (m model) attachmentPreviewView() string {
	if len(m.attachments) == 0 && len(m.pastedImages) == 0 {
		return ""
	}
	maxItems := min(3, len(m.attachments))
	rows := []string{m.theme().muted.Render(fmt.Sprintf("Attachments · %d", len(m.attachments)))}
	previewUsed := false
	for _, item := range m.attachments[:maxItems] {
		label := "file"
		if item.Kind == "image" {
			label = item.MIMEType
			if label == "" {
				label = "image"
			}
			if item.Width > 0 && item.Height > 0 {
				label += fmt.Sprintf(" · %d×%d", item.Width, item.Height)
			}
		}
		row := fmt.Sprintf("  · %s · %s · %s", safeText(item.Path), label, formatBytes(item.Bytes))
		canPreview := item.Kind == "image" && !previewUsed && item.Inline != "" && m.attachmentInlineAllowed()
		if canPreview {
			rows = append(rows, ansi.Truncate(row, m.contentWidth(), "…"), item.Inline)
			for range 5 {
				rows = append(rows, "")
			}
			previewUsed = true
			continue
		}
		if item.Kind == "image" {
			row += " · visual preview unavailable; attached to request"
		}
		rows = append(rows, ansi.Truncate(row, m.contentWidth(), "…"))
	}
	if maxItems < len(m.attachments) {
		rows = append(rows, m.theme().muted.Render(fmt.Sprintf("  … +%d more", len(m.attachments)-maxItems)))
	}
	if len(m.pastedImages) > 0 {
		rows = append(rows, m.theme().info.Render(fmt.Sprintf("  · %d clipboard image(s) ready · ⌘V to add more", len(m.pastedImages))))
	}
	return strings.Join(rows, "\n")
}

func (m model) attachmentInlineAllowed() bool {
	if m.width < 48 || m.height-m.queuedStackHeight() < 26 || m.stream != nil {
		return false
	}
	for _, item := range m.attachments[:min(3, len(m.attachments))] {
		if item.Kind == "image" && item.Inline != "" {
			return true
		}
	}
	return false
}

func (m model) attachmentStackHeight() int {
	if len(m.attachments) == 0 && len(m.pastedImages) == 0 {
		return 0
	}
	height := 1 + min(3, len(m.attachments))
	if len(m.attachments) == 0 && len(m.pastedImages) > 0 {
		height = 1
	}
	if len(m.attachments) > 3 {
		height++
	}
	if m.attachmentInlineAllowed() {
		height += 6
	}
	return height
}

func (m *model) prepareAttachmentPreviews() {
	used := false
	for i := range m.attachments {
		item := &m.attachments[i]
		item.Inline = ""
		if used || item.Kind != "image" || m.imageProtocol == imageProtocolNone || len(item.Data) == 0 {
			item.Data = nil
			continue
		}
		if graphic, ok := renderInlineImage(*item, m.imageProtocol, 20, 5); ok && len(graphic) <= 1<<20 {
			item.Inline = graphic
			used = true
		}
		item.Data = nil
	}
}

func renderInlineImage(item AttachmentPreview, protocol imageProtocol, columns, rows int) (string, bool) {
	if len(item.Data) == 0 || columns < 8 || rows < 2 {
		return "", false
	}
	switch protocol {
	case imageProtocolITerm:
		sequence := ansi.ITerm2(iterm2.File{
			Name: "Opera attachment", Size: int64(len(item.Data)), Width: iterm2.Cells(columns), Height: iterm2.Cells(rows),
			Inline: true, DoNotMoveCursor: true, Content: []byte(base64.StdEncoding.EncodeToString(item.Data)),
		})
		return sequence, true
	case imageProtocolKitty:
		decoded, _, err := image.Decode(bytes.NewReader(item.Data))
		if err != nil {
			return "", false
		}
		var out strings.Builder
		err = kitty.EncodeGraphics(&out, decoded, &kitty.Options{
			Action: kitty.TransmitAndPut, Format: kitty.PNG, Columns: columns, Rows: rows,
			DoNotMoveCursor: true, Quiet: 2,
		})
		return out.String(), err == nil
	default:
		return "", false
	}
}

func formatBytes(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f KiB", float64(size)/1024)
}

// NO_COLOR removes style escapes while retaining trusted inline-image protocol
// sequences generated by this package.
func stripColorKeepGraphics(content string) string {
	type sequence struct{ token, value string }
	var saved []sequence
	for index := 0; ; {
		kittyStart := strings.Index(content[index:], "\x1b_G")
		itermStart := strings.Index(content[index:], "\x1b]1337;")
		start, end := -1, -1
		if kittyStart >= 0 && (itermStart < 0 || kittyStart < itermStart) {
			start = index + kittyStart
			if terminator := strings.Index(content[start+3:], "\x1b\\"); terminator >= 0 {
				end = start + 3 + terminator + 2
			}
		} else if itermStart >= 0 {
			start = index + itermStart
			if terminator := strings.IndexByte(content[start:], '\a'); terminator >= 0 {
				end = start + terminator + 1
			}
		}
		if start < 0 || end < 0 {
			break
		}
		token := fmt.Sprintf("OPERA_GRAPHIC_SEQUENCE_%d_TOKEN", len(saved))
		saved = append(saved, sequence{token: token, value: content[start:end]})
		content = content[:start] + token + content[end:]
		index = start + len(token)
	}
	content = ansi.Strip(content)
	for _, item := range saved {
		content = strings.ReplaceAll(content, item.token, item.value)
	}
	return content
}
