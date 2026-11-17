// Package contextfiles loads bounded workspace instructions and text attachments.
package contextfiles

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/diasYuri/opera/internal/contextdata"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/tools/gitignore"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type Source = contextdata.Source
type Bundle = contextdata.Bundle

// Load orders root instructions before scoped instructions, then explicit files.
// Symlinks, ignored files, binary data and files outside the workspace are rejected.
func Load(ctx context.Context, root string, paths []string, limit int) (Bundle, error) {
	var out Bundle
	seen := map[string]bool{}
	add := func(name, kind string, optional bool) error {
		absolute := filepath.Join(root, name)
		if filepath.IsAbs(name) {
			absolute = filepath.Clean(name)
		}
		rel, err := filepath.Rel(root, absolute)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("context escapes workspace: %s", name)
		}
		if seen[rel] {
			return nil
		}
		seen[rel] = true
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			if part == ".git" || part == ".opera" {
				return fmt.Errorf("private context path: %s", rel)
			}
		}
		for p := absolute; p != root; p = filepath.Dir(p) {
			info, err := os.Lstat(p)
			if os.IsNotExist(err) && optional {
				return nil
			}
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("context symlink rejected: %s", rel)
			}
		}
		matcher := gitignore.NewWalkIgnore()
		if err := matcher.AddDir(root); err != nil {
			return err
		}
		current := root
		parts := strings.Split(rel, string(filepath.Separator))
		for _, part := range parts[:len(parts)-1] {
			current = filepath.Join(current, part)
			if matcher.Match(current, true) {
				return fmt.Errorf("context directory is ignored: %s", rel)
			}
			if err := matcher.AddDir(current); err != nil {
				return err
			}
		}
		if matcher.Match(absolute, false) {
			return fmt.Errorf("context is ignored: %s", rel)
		}
		if exec.CommandContext(ctx, "git", "-C", root, "check-ignore", "--quiet", "--", rel).Run() == nil {
			return fmt.Errorf("context is ignored: %s", rel)
		}
		info, err := os.Stat(absolute)
		if os.IsNotExist(err) && optional {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > int64(limit) {
			return fmt.Errorf("context exceeds remaining byte budget: %s", rel)
		}
		data, err := read(root, rel, limit)
		if err != nil {
			return err
		}
		if imagePath(rel) {
			mime := http.DetectContentType(data)
			if mime != "image/png" && mime != "image/jpeg" && mime != "image/gif" && mime != "image/webp" {
				return fmt.Errorf("unsupported image attachment: %s", rel)
			}
			label := fmt.Sprintf("\nImage attachment source=%q sha256=%x (payload supplied transiently)\n", filepath.ToSlash(rel), sha256.Sum256(data))
			limit -= len(data) + len(label)
			if limit < 0 {
				return fmt.Errorf("attachment budget exceeded")
			}
			out.Text += label
			out.Sources = append(out.Sources, Source{Path: filepath.ToSlash(rel), Bytes: len(data), Kind: "image"})
			return nil
		}
		if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
			return fmt.Errorf("unsupported binary attachment: %s; attach UTF-8 text", rel)
		}
		data = redact(data)
		block := fmt.Sprintf("\n<workspace-context source=%q kind=%q>\n%s\n</workspace-context>\n", filepath.ToSlash(rel), kind, data)
		if len(block) > limit {
			return fmt.Errorf("context budget exceeded: %s", rel)
		}
		limit -= len(block)
		out.Text += block
		out.Sources = append(out.Sources, Source{Path: filepath.ToSlash(rel), Bytes: len(data), Kind: kind})
		return nil
	}
	if err := add("AGENTS.md", "instructions", true); err != nil {
		return out, err
	}
	for _, name := range paths {
		rel := name
		if filepath.IsAbs(name) {
			var err error
			rel, err = filepath.Rel(root, name)
			if err != nil {
				return out, err
			}
		}
		dirs := []string{}
		for d := filepath.Dir(rel); d != "." && d != ""; d = filepath.Dir(d) {
			if d == ".." || filepath.IsAbs(d) {
				return out, fmt.Errorf("context escapes workspace")
			}
			dirs = append(dirs, d)
		}
		for i := len(dirs) - 1; i >= 0; i-- {
			if err := add(filepath.Join(dirs[i], "AGENTS.md"), "instructions", true); err != nil {
				return out, err
			}
		}
		if err := add(name, "attachment", false); err != nil {
			return out, err
		}
	}
	return out, nil
}

func redact(data []byte) []byte { return []byte(contextdata.RedactText(string(data))) }
func imagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	}
	return false
}
func read(root, rel string, limit int) ([]byte, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	f, err := r.Open(rel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if len(data) > limit {
		return nil, fmt.Errorf("attachment grew beyond budget")
	}
	return data, err
}

// Images are injected into the provider request only; checkpoints contain hashes.
func Images(ctx context.Context, root string, paths []string, vision bool, limit int) ([]ai.ContentBlock, error) {
	bundle, err := Load(ctx, root, paths, limit)
	if err != nil {
		return nil, err
	}
	var blocks []ai.ContentBlock
	for _, source := range bundle.Sources {
		if source.Kind != "image" {
			continue
		}
		if !vision {
			return nil, fmt.Errorf("selected model does not advertise vision support: %s", source.Path)
		}
		data, err := read(root, source.Path, limit)
		if err != nil {
			return nil, err
		}
		limit -= len(data)
		blocks = append(blocks, ai.ContentBlock{Type: ai.ContentImage, ImageData: data, ImageMIMEType: http.DetectContentType(data)})
	}
	return blocks, nil
}

// ImageReference records a transient image attachment path.
func ImageReference(path string) ai.ContentBlock { return contextdata.ImageReference(path) }

// RedactText redacts credential assignments in exported strings.
func RedactText(text string) string { return contextdata.RedactText(text) }
