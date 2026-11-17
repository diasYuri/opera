// Package contextdata defines I/O-free workspace context data and projections.
package contextdata

import (
	"encoding/json"
	"github.com/diasYuri/y/pkg/ai"
	"path/filepath"
	"regexp"
)

type Source struct {
	Path  string
	Bytes int
	Kind  string
}
type Bundle struct {
	Text    string
	Sources []Source
}

var credentialLine = regexp.MustCompile(`(?im)([a-z0-9_]*(?:api[_-]?key|password|secret|token|authorization)[a-z0-9_]*\s*[:=]\s*)[^\r\n]+`)

func redact(data []byte) []byte { return credentialLine.ReplaceAll(data, []byte("${1}[REDACTED]")) }

// ImageReference records an attachment path in the transcript without storing
// the image bytes. The request hook resolves it into a transient image block.
func ImageReference(path string) ai.ContentBlock {
	details, _ := json.Marshal(struct {
		Path string `json:"opera_image_path"`
	}{Path: filepath.ToSlash(path)})
	return ai.ContentBlock{Type: ai.ContentText, Text: "", Details: details}
}

// RedactText applies the same credential-assignment redaction to exported strings.
func RedactText(text string) string { return string(redact([]byte(text))) }
