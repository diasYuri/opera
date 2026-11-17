package execution

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/policy"
)

func mutation(name string) bool {
	switch name {
	case "memory.search", "memory.read", "memory.list", "task_create", "task_list", "task_start", "task_complete", "task_refine", "subagent_spawn", "subagent_list", "subagent_wait", "subagent_cancel", "read_file", "list_directory", "list_files", "search", "search_text", "search_ast", "outline", "find_files", "git_status", "git_diff", "git_log", "git_show", "git_history", "git_blame", "git_search_history", "definition", "references", "implementations", "hover", "document_symbols", "workspace_symbols", "callers", "callees", "type_definition":
		return false
	default:
		return true
	}
}

func approvalFor(ctx context.Context, workspace, mode string, pending *agent.PendingApproval, read ApprovalFileReader) (*Approval, error) {
	if pending == nil {
		return nil, nil
	}
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Patch   string `json:"patch"`
		Diff    string `json:"diff"`
	}
	if err := json.Unmarshal(pending.ToolCall.Arguments, &args); err != nil {
		return nil, err
	}
	path := args.Path
	var old []byte
	var err error
	if pending.ToolCall.Name == "write_file" || pending.ToolCall.Name == "edit" || pending.ToolCall.Name == "patch" {
		path, old, err = readApprovalFile(ctx, read, workspace, args.Path)
	}
	if err != nil {
		return nil, err
	}
	if pending.ToolCall.Name == "patch" {
		patch := args.Patch
		if patch == "" {
			patch = args.Diff
		}
		seen := map[string]bool{}
		for _, line := range strings.Split(patch, "\n") {
			if !strings.HasPrefix(line, "--- ") && !strings.HasPrefix(line, "+++ ") {
				continue
			}
			name := strings.Split(strings.TrimSpace(line[4:]), "\t")[0]
			if name == "/dev/null" {
				continue
			}
			if strings.HasPrefix(name, "a/") {
				name = strings.TrimPrefix(name, "a/")
			} else {
				name = strings.TrimPrefix(name, "b/")
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			p, data, e := readApprovalFile(ctx, read, workspace, name)
			if e != nil {
				return nil, e
			}
			path += p + "\n"
			old = append(old, []byte(p+"\x00")...)
			old = append(old, data...)
		}
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(append(append([]byte(mode+"\x00"+path+"\x00"), pending.ToolCall.Arguments...), old...)))
	view := &Approval{ID: fmt.Sprintf("%x", sha256.Sum256([]byte(pending.Request.ApprovalID+fingerprint))), Agent: "main", Tool: pending.ToolCall.Name, Arguments: policy.Redact(pending.ToolCall.Arguments), Path: path, Fingerprint: fingerprint, RuntimeID: pending.Request.ApprovalID}
	if pending.ToolCall.Name == "write_file" {
		view.Diff = "--- " + path + "\n+++ " + path + "\n-" + strings.ReplaceAll(string(old), "\n", "\n-") + "\n+" + strings.ReplaceAll(args.Content, "\n", "\n+")
	}
	if pending.ToolCall.Name == "edit" || pending.ToolCall.Name == "patch" {
		view.Diff = "Current file (review arguments for the requested change):\n" + string(old) + "\nRequested edit:\n" + string(view.Arguments)
	}
	return view, nil
}

func Policy(mode string, audits ...policy.AuditSink) policy.Engine {
	base := policy.NewEngine(policy.Config{DisableSensitiveApproval: mode == "trusted"})
	engine := policy.Func(func(ctx context.Context, request policy.Request) (policy.Decision, error) {
		if mutation(request.ToolName) || strings.HasPrefix(request.ToolName, "mcp_") || strings.HasPrefix(request.ToolName, "mcp.") || strings.HasPrefix(request.ToolName, "wasm_") || strings.HasPrefix(request.ToolName, "opera_") || request.ToolName == "memory.save" || request.ToolName == "memory.forget" {
			request.Sensitive = true
		}
		if mode == "read-only" && (mutation(request.ToolName) || request.Sensitive) {
			return policy.Decision{Kind: policy.DecisionDeny, Reason: "workspace is read-only"}, nil
		}
		return base.Decide(ctx, request)
	})
	return allToolsPolicy{policy.Func(func(ctx context.Context, request policy.Request) (policy.Decision, error) {
		decision, err := engine.Decide(ctx, request)
		if err != nil {
			return decision, err
		}
		arguments := policy.Redact(request.Arguments)
		var redacted any
		if json.Unmarshal(arguments, &redacted) == nil {
			scrub(redacted)
			arguments, _ = json.Marshal(redacted)
		} else {
			arguments = nil
		}
		hash := sha256.Sum256(request.Arguments)
		identity := request.Identity
		identity.RunID = request.RunID
		event := policy.AuditEvent{SchemaVersion: 1, EventID: rand.Text(), Timestamp: time.Now().UTC(), RequestID: request.RequestID, Identity: identity, ToolName: request.ToolName, ToolCallID: request.ToolCallID, Decision: decision.Kind, Reason: decision.Reason, PolicyVersion: request.PolicyVersion, ArgumentsHash: fmt.Sprintf("%x", hash), Arguments: arguments}
		for _, sink := range audits {
			if err := sink.Append(ctx, event); err != nil {
				return policy.Decision{}, err
			}
		}
		return decision, nil
	})}
}

// approvedRequest constructs the SDK decision only after the application has
// verified the caller's exact approval fingerprint against persisted metadata.
func approvedRequest(view *Approval, denied bool) *policy.ApprovalResolution {
	state := policy.ApprovalApproved
	if denied {
		state = policy.ApprovalDenied
	}
	return &policy.ApprovalResolution{ApprovalID: view.RuntimeID, State: state, Actor: "local-user"}
}

// The public registry recognizes this marker to authorize descriptors even when
// an external server declares no capabilities or sensitivity annotations.
type allToolsPolicy struct{ policy.Engine }

func (allToolsPolicy) AuditAllTools() bool { return true }

// ApprovalFileReader supplies the filesystem observation used by approval policy.
// It does not grant permission to execute the requested tool.
type ApprovalFileReader func(context.Context, string, string) (string, []byte, error)

func readApprovalFile(ctx context.Context, read ApprovalFileReader, workspace, path string) (string, []byte, error) {
	if path == "" {
		return "", nil, nil
	}
	if read == nil {
		return "", nil, fmt.Errorf("approval file reader is unavailable")
	}
	return read(ctx, workspace, path)
}
