package provideradapters

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"time"
)

const maxAPIKeyHelperOutput = 64 << 10

type boundedSecretBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedSecretBuffer) Write(value []byte) (int, error) {
	length := len(value)
	remaining := maxAPIKeyHelperOutput - b.Len()
	if remaining <= 0 {
		b.overflow = true
		return length, nil
	}
	if length > remaining {
		b.overflow = true
		value = value[:remaining]
	}
	_, _ = b.Buffer.Write(value)
	return length, nil
}

func runAPIKeyHelper(ctx context.Context, executable string) (string, error) {
	if ctx == nil {
		return "", errors.New("API key helper requires a context")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable)
	var output boundedSecretBuffer
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errors.New("API key helper timed out")
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("API key helper failed")
	}
	if output.overflow {
		return "", errors.New("API key helper output exceeds 64 KiB")
	}
	key := strings.TrimSpace(output.String())
	if key == "" || strings.ContainsAny(key, "\x00\r\n\x1b") {
		return "", errors.New("API key helper returned an invalid key")
	}
	return key, nil
}
