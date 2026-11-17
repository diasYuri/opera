package cli

import (
	"context"
	"errors"
	"github.com/diasYuri/opera/internal/modules/skills/catalog"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diasYuri/opera/internal/adapters/skills/filesystem"
	ui "github.com/diasYuri/opera/internal/ui/tui"
)

const maxFileCompletionScan = 512

// CompleteComposer performs workspace discovery outside the presentation loop.
func (s *chatSession) CompleteComposer(ctx context.Context, token string) ([]ui.CompletionCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	root := s.cfg.Paths.Workspace
	s.mu.Unlock()
	if strings.HasPrefix(token, "@") {
		return completeWorkspaceFiles(ctx, root, token), ctx.Err()
	}
	if !strings.HasPrefix(token, "$") {
		return nil, nil
	}
	catalog, err := s.completionDiscovery.skills(ctx, root)
	if err != nil {
		return nil, err
	}
	query := strings.TrimPrefix(token, "$")
	var out []ui.CompletionCandidate
	for _, skill := range catalog.Skills {
		if strings.HasPrefix(strings.ToLower(skill.Name), strings.ToLower(query)) {
			out = append(out, ui.CompletionCandidate{Value: "$" + skill.Name + " ", Label: "$" + skill.Name, Detail: skill.Description})
		}
	}
	return out, ctx.Err()
}

func completeWorkspaceFiles(ctx context.Context, root, token string) []ui.CompletionCandidate {
	if ctx.Err() != nil {
		return nil
	}
	query := strings.TrimPrefix(token, "@")
	dirPart, base := filepath.Split(query)
	dir := filepath.Clean(filepath.Join(root, dirPart))
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	workspace, err := os.OpenRoot(root)
	if err != nil {
		return nil
	}
	defer func() { _ = workspace.Close() }()
	parent, err := workspace.Open(rel)
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
	var items []ui.CompletionCandidate
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
			items = append(items, ui.CompletionCandidate{Value: "@" + path, Label: "@" + path, Detail: detail})
		} else {
			items = append(items, ui.CompletionCandidate{Value: "@" + path + " ", Label: "@" + path, Detail: detail})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Detail != items[j].Detail {
			return strings.HasPrefix(items[i].Detail, "directory")
		}
		return strings.ToLower(items[i].Label) < strings.ToLower(items[j].Label)
	})
	return items
}

// composerDiscovery retains the previous five-second discovery cache outside UI.
// The lock also coalesces overlapping requests; canceled waiters do no I/O.
type composerDiscovery struct {
	mu      sync.Mutex
	root    string
	at      time.Time
	catalog skillcatalog.Catalog
}

func (d *composerDiscovery) skills(ctx context.Context, root string) (skillcatalog.Catalog, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return skillcatalog.Catalog{}, err
	}
	if d.root != root || d.at.IsZero() || time.Since(d.at) >= 5*time.Second {
		home, _ := os.UserHomeDir()
		d.catalog = skills.Discover(root, home)
		d.root, d.at = root, time.Now()
	}
	return d.catalog, ctx.Err()
}
