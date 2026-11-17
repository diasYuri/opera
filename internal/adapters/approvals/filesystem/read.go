// Package approvalfiles reads workspace files for approval previews.
package approvalfiles

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Read rejects paths outside the workspace, symlinks and oversized files.
func Read(ctx context.Context, workspace, path string) (string, []byte, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if path == "" {
		return "", nil, nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	path = filepath.Clean(path)
	relative, err := filepath.Rel(workspace, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("tool path escapes workspace")
	}
	// Validate every existing ancestor so approval previews never read outside
	// the workspace through symlinks; tools repeat their own policy validation.
	for ancestor := path; ancestor != workspace; ancestor = filepath.Dir(ancestor) {
		info, err := os.Lstat(ancestor)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("approval paths must not contain symlinks")
		}
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return path, []byte("<file does not exist>"), nil
	}
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return "", nil, fmt.Errorf("approval preview requires a regular file of at most 1 MiB")
	}
	data, err := os.ReadFile(path)
	return path, data, err
}
