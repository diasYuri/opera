package contextfiles

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
)

const maxInlinePreviewBytes = 512 << 10

type AttachmentPreview struct {
	Source
	MIMEType string
	Width    int
	Height   int
	Data     []byte
}

// Previews returns safe attachment metadata and bounded bytes for inline image
// rendering. Load applies the same workspace, symlink, ignore and size checks
// used by request construction before any preview bytes are read.
func Previews(ctx context.Context, root string, paths []string, limit int) ([]AttachmentPreview, error) {
	bundle, err := Load(ctx, root, paths, limit)
	if err != nil {
		return nil, err
	}
	out := make([]AttachmentPreview, 0, len(bundle.Sources))
	previewBudget := 1 << 20
	for _, source := range bundle.Sources {
		if source.Kind == "instructions" {
			continue
		}
		preview := AttachmentPreview{Source: source}
		if source.Kind == "image" {
			data, readErr := read(root, source.Path, limit)
			if readErr != nil {
				return nil, readErr
			}
			preview.MIMEType = http.DetectContentType(data)
			if len(data) <= maxInlinePreviewBytes && len(data) <= previewBudget {
				if config, _, decodeErr := image.DecodeConfig(bytes.NewReader(data)); decodeErr == nil {
					preview.Width, preview.Height = config.Width, config.Height
				}
				preview.Data = data
				previewBudget -= len(data)
			}
		}
		out = append(out, preview)
	}
	return out, nil
}
