package cli

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// ReadClipboardImage is the CLI adapter for the UI's clipboard capability.
func (s *chatSession) ReadClipboardImage(parent context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		const script = `import AppKit
let board = NSPasteboard.general
let source = board.data(forType: .png) ?? board.data(forType: .tiff) ?? board.data(forType: .jpeg)
guard let data = source, let bitmap = NSBitmapImageRep(data: data), let png = bitmap.representation(using: .png, properties: [:]) else { exit(2) }
try FileHandle.standardOutput.write(contentsOf: png)`
		cmd = exec.CommandContext(ctx, "swift", "-e", script)
	case "linux":
		if _, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.CommandContext(ctx, "wl-paste", "--no-newline", "--type", "image/png")
		} else {
			cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-t", "image/png", "-o")
		}
	default:
		return nil, fmt.Errorf("image clipboard is unsupported on %s", runtime.GOOS)
	}
	return cmd.Output()
}
