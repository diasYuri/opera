package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/diasYuri/y/pkg/extensions/subagents"
	taskmanager "github.com/diasYuri/y/pkg/extensions/task-manager"
	"github.com/diasYuri/y/pkg/tools"
)

func (w *Work) Register(registry *tools.Registry) error {
	sample := taskmanager.New()
	source := tools.NewRegistry()
	defer func() { _ = source.Close(context.Background()) }()
	if err := sample.Register(source); err != nil {
		return err
	}
	for _, name := range []string{"task_create", "task_list", "task_start", "task_complete", "task_refine"} {
		desc, _, ok := source.Get(name)
		if !ok {
			return fmt.Errorf("task descriptor unavailable")
		}
		name := name
		if err := registry.Add(desc, tools.ToolHandlerFunc(func(ctx context.Context, req tools.ToolRequest) (tools.ToolResponse, error) {
			w.taskMu.Lock()
			defer w.taskMu.Unlock()
			snap, err := w.Tasks(ctx, req.RunID)
			if err != nil {
				return tools.ToolResponse{}, err
			}
			manager := taskmanager.New()
			if err := manager.Restore(snap); err != nil {
				return tools.ToolResponse{}, err
			}
			r := tools.NewRegistry()
			defer func() { _ = r.Close(context.Background()) }()
			if err := manager.Register(r); err != nil {
				return tools.ToolResponse{}, err
			}
			_, handler, _ := r.Get(name)
			result, err := handler.Handle(ctx, req)
			if err == nil {
				err = w.Store.Put(ctx, "tasks", req.RunID, manager.Snapshot())
			}
			return result, err
		})); err != nil {
			return err
		}
	}
	for _, name := range []string{"subagent_spawn", "subagent_list", "subagent_wait", "subagent_cancel"} {
		name := name
		schema := json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"},"name":{"type":"string"},"id":{"type":"string"},"task_id":{"type":"string"}},"additionalProperties":false}`)
		if err := registry.Add(tools.ToolDescriptor{Name: name, Description: "Manage process-owned local child executions. Spawn returns an ID; list/wait shows durable results and approvals. Children share the workspace.", InputSchema: schema}, tools.ToolHandlerFunc(func(ctx context.Context, req tools.ToolRequest) (tools.ToolResponse, error) {
			var in struct {
				Prompt, Name, ID string
				TaskID           string `json:"task_id"`
			}
			if err := json.Unmarshal(req.Arguments, &in); err != nil {
				return tools.ToolResponse{}, err
			}
			var value any
			var err error
			switch name {
			case "subagent_spawn":
				w.Service.activeMu.Lock()
				root := w.Service.contexts[req.RunID]
				w.Service.activeMu.Unlock()
				if root == nil {
					return tools.ToolResponse{}, errors.New("parent execution is not active")
				}
				value, err = w.Spawn(root, req.RunID, in.Name, in.Prompt, in.TaskID)
			case "subagent_list":
				value, err = w.Children(ctx, req.RunID)
			case "subagent_wait":
				if in.ID == "" {
					err = w.WaitChildren(ctx, req.RunID)
					value = "children settled"
				} else {
					child, e := w.child(ctx, in.ID)
					if e != nil || child.ParentID != req.RunID {
						return tools.ToolResponse{}, errors.New("child does not belong to parent")
					}
					w.yield(req.RunID)
					err = w.Wait(ctx, in.ID)
					value, _ = w.child(ctx, in.ID)
				}
			case "subagent_cancel":
				child, e := w.child(ctx, in.ID)
				if e != nil || child.ParentID != req.RunID {
					return tools.ToolResponse{}, errors.New("child does not belong to parent")
				}
				child.ExplicitCancel = true
				child.Status = subagents.StatusCanceled
				child.Outcome.Approval = nil
				child.Outcome.ExitCode = 130
				child.Outcome.State = "canceled"
				if err := w.Store.Put(ctx, "children", child.ID, child); err != nil {
					return tools.ToolResponse{}, err
				}
				w.mu.Lock()
				job := w.jobs[in.ID]
				if job != nil {
					job.cancel()
				}
				w.mu.Unlock()
				value = "cancellation requested"
			}
			if err != nil {
				return tools.ToolResponse{}, err
			}
			raw, err := json.Marshal(value)
			return tools.ToolResponse{Content: []tools.ContentBlock{{Type: tools.ContentText, Text: string(raw)}}, Details: raw}, err
		})); err != nil {
			return err
		}
	}
	return nil
}
