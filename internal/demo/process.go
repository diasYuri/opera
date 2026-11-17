package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/diasYuri/opera/internal/modules/execution"
)

// ProcessProof verifies headless exit-3 recovery across actual CLI processes.
func ProcessProof(ctx context.Context, binary string) (Evidence, error) {
	evidence := Evidence{Mode: "headless"}
	root, err := os.MkdirTemp("", "opera-headless-proof-")
	if err != nil {
		return evidence, err
	}
	defer func() { _ = os.RemoveAll(root) }()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		return evidence, err
	}
	env := []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "OPERA_") {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, "OPERA_CONFIG_DIR="+filepath.Join(root, "config"), "OPERA_DATA_DIR="+filepath.Join(root, "data"), "OPERA_CACHE_DIR="+filepath.Join(root, "cache"))
	call := func(args ...string) (execution.Result, int, error) {
		cmd := exec.CommandContext(ctx, binary, append(args, "--workspace", workspace, "--format=jsonl")...)
		cmd.Env = env
		cmd.Dir = workspace
		var output bytes.Buffer
		cmd.Stdout = &output
		err := cmd.Run()
		code := 0
		if err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				return execution.Result{}, 1, err
			}
			code = exit.ExitCode()
		}
		var result execution.Result
		for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte{'\n'}) {
			var event struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(line, &event); err != nil {
				return result, code, errors.New("headless child emitted invalid JSONL")
			}
			if event.Type == "execution.result" {
				if err := json.Unmarshal(event.Data, &result); err != nil {
					return result, code, err
				}
			}
		}
		return result, code, nil
	}
	for _, denied := range []bool{false, true} {
		name := "approved.txt"
		if denied {
			name = "denied.txt"
		}
		args, _ := json.Marshal(map[string]string{"path": name, "content": "verified"})
		pending, code, err := call("tools", "call", "write_file", string(args), "--policy=ask")
		if err != nil || code != 3 || pending.Approval == nil {
			return evidence, errors.Join(fmt.Errorf("headless approval for %s did not persist with exit 3: code %d", name, code), err)
		}
		verb := "--approve"
		if denied {
			verb = "--deny"
			evidence.Denials++
		} else {
			evidence.Approvals++
		}
		result, code, err := call("resume", pending.ID, verb, pending.Approval.ID)
		if err != nil || !denied && code != 0 || denied && code != 0 && code != 1 {
			return evidence, errors.Join(fmt.Errorf("headless resume for %s failed: code %d state %s", name, code, result.State), err)
		}
		evidence.RunID = result.ID
		data, readErr := os.ReadFile(filepath.Join(workspace, name))
		if denied {
			if !os.IsNotExist(readErr) {
				return evidence, errors.New("denied headless operation produced an effect")
			}
		} else if readErr != nil || string(data) != "verified" {
			return evidence, errors.New("approved headless effect missing")
		}
	}
	evidence.Verified = true
	return evidence, nil
}
