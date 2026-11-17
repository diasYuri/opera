package execution

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/diasYuri/opera/internal/modules/workspace"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/policy"
	"github.com/diasYuri/y/pkg/tools"
)

type Guard struct {
	ReadApprovalFile ApprovalFileReader
	Workspace, Mode  string
	Queue            *workspace.Queue
	mu               sync.Mutex
	expected         map[string]*Approval
	modes            map[string]Mode
}

func (g *Guard) SetMode(id string, mode Mode) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.modes == nil {
		g.modes = map[string]Mode{}
	}
	g.modes[id] = mode
}

func (g *Guard) ClearMode(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.modes, id)
}

func (g *Guard) Set(id string, view *Approval) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.expected == nil {
		g.expected = map[string]*Approval{}
	}
	if view == nil {
		delete(g.expected, id)
	} else {
		g.expected[id] = view
	}
}
func (g *Guard) Middleware(ctx context.Context, request tools.ToolRequest, _ tools.ToolDescriptor, next tools.ToolExecutionNext) (tools.ToolResponse, error) {
	ctx = context.WithValue(ctx, toolRequestKey{}, request)
	g.mu.Lock()
	mode, configured := g.modes[request.RunID]
	g.mu.Unlock()
	if configured {
		if len(mode.Tools.AllowedTools) > 0 && !contains(mode.Tools.AllowedTools, request.Name) || contains(mode.Tools.DeniedTools, request.Name) {
			return tools.ToolResponse{}, fmt.Errorf("tool %q is not available in %s mode", request.Name, mode.Name)
		}
		if !mode.Tools.AllowMutations && mutation(request.Name) {
			return tools.ToolResponse{}, fmt.Errorf("tool %q cannot run in %s mode", request.Name, mode.Name)
		}
		if !mode.Tools.AllowDelegation && isDelegationTool(request.Name) {
			return tools.ToolResponse{}, fmt.Errorf("delegation is disabled in %s mode", mode.Name)
		}
		if !mode.Tools.AllowTaskManagement && isTaskTool(request.Name) {
			return tools.ToolResponse{}, fmt.Errorf("task management is disabled in %s mode", mode.Name)
		}
	}
	if !mutation(request.Name) {
		return next(ctx, request)
	}
	release, err := g.Queue.Acquire(ctx)
	if err != nil {
		return tools.ToolResponse{}, err
	}
	defer release()
	g.mu.Lock()
	expected := g.expected[request.RunID]
	g.mu.Unlock()
	if expected != nil && request.Approval != nil && request.Approval.State == policy.ApprovalApproved {
		pending := &agent.PendingApproval{Request: policy.ApprovalRequest{ApprovalID: expected.RuntimeID}, ToolCall: ai.ToolCall{ID: request.ID, Name: request.Name, Arguments: request.Arguments}}
		current, err := approvalFor(ctx, g.Workspace, g.Mode, pending, g.ReadApprovalFile)
		if err != nil {
			return tools.ToolResponse{}, err
		}
		if current.ID != expected.ID {
			return tools.ToolResponse{}, &tools.ApprovalError{Request: policy.ApprovalRequest{ApprovalID: expected.RuntimeID, RequestID: request.RequestID, RunID: request.RunID, ToolCallID: request.ID, ToolName: request.Name, Reason: "file changed after approval; review the new diff"}, Cause: tools.ErrApprovalRequired}
		}
	}
	return next(ctx, request)
}

func isDelegationTool(name string) bool { return strings.HasPrefix(name, "subagent_") }

func isTaskTool(name string) bool { return strings.HasPrefix(name, "task_") }

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

type toolRequestKey struct{}

func ToolRequestFromContext(ctx context.Context) (tools.ToolRequest, bool) {
	request, ok := ctx.Value(toolRequestKey{}).(tools.ToolRequest)
	return request, ok
}
func IsWorkspaceMutation(name string) bool { return mutation(name) }
