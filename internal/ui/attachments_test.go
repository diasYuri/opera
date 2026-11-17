package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var encoded bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 30, G: 100, B: 220, A: 255})
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestRenderInlineImageProtocols(t *testing.T) {
	data := tinyPNG(t)
	item := AttachmentPreview{Kind: "image", MIMEType: "image/png", Data: data}

	t.Run("kitty graphics", func(t *testing.T) {
		sequence, ok := renderInlineImage(item, imageProtocolKitty, 20, 5)
		if !ok || !strings.HasPrefix(sequence, "\x1b_G") || !strings.Contains(sequence, "\x1b\\") {
			t.Fatalf("invalid Kitty graphics sequence: ok=%v sequence=%q", ok, sequence)
		}
	})

	t.Run("iTerm2 inline file", func(t *testing.T) {
		sequence, ok := renderInlineImage(item, imageProtocolITerm, 20, 5)
		if !ok || !strings.HasPrefix(sequence, "\x1b]1337;File=") || !strings.HasSuffix(sequence, "\a") {
			t.Fatalf("invalid iTerm2 inline image sequence: ok=%v sequence=%q", ok, sequence)
		}
	})

	t.Run("unsupported protocol", func(t *testing.T) {
		if sequence, ok := renderInlineImage(item, imageProtocolNone, 20, 5); ok || sequence != "" {
			t.Fatalf("unsupported protocol produced output: ok=%v sequence=%q", ok, sequence)
		}
	})
}
