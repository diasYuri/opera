package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/diasYuri/opera/internal/persistence"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/extensions/subagents"
	taskmanager "github.com/diasYuri/y/pkg/extensions/task-manager"
	"github.com/diasYuri/y/pkg/providers"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrChildrenPending = errors.New("child executions require explicit approval or recovery")

type Child struct {
	ExplicitCancel bool   `json:"explicit_cancel,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	subagents.Record
	Outcome Result `json:"outcome"`
}
type childJob struct {
	reacquire func(context.Context) error
	done      chan struct{}
	cancel    context.CancelFunc
	release   func()
	err       error
}
type Work struct {
	MaxBudget               int64
	budget                  map[string]int64
	Store                   *persistence.Store
	Service                 *Service
	MaxConcurrent, MaxDepth int
	mu                      sync.Mutex
	taskMu                  sync.Mutex
	jobs                    map[string]*childJob
	slots                   chan struct{}
	closed                  bool
	wg                      sync.WaitGroup
}

func NewWork(store *persistence.Store, max, depth int) *Work {
	return &Work{Store: store, MaxConcurrent: max, MaxDepth: depth, jobs: map[string]*childJob{}, slots: make(chan struct{}, max)}
}
func (w *Work) Tasks(ctx context.Context, id string) (taskmanager.Snapshot, error) {
	doc, err := w.Store.Get(ctx, "tasks", id)
	if errors.Is(err, sql.ErrNoRows) {
		return taskmanager.New().Snapshot(), nil
	}
	if err != nil {
		return taskmanager.Snapshot{}, err
	}
	var snap taskmanager.Snapshot
	err = json.Unmarshal(doc.Data, &snap)
	return snap, err
}
func (w *Work) CopyTasks(ctx context.Context, from, to string) error {
	snap, err := w.Tasks(ctx, from)
	if err != nil {
		return err
	}
	return w.Store.Put(ctx, "tasks", to, snap)
}
func (w *Work) Children(ctx context.Context, parent string) ([]Child, error) {
	docs, err := w.Store.List(ctx, "children")
	if err != nil {
		return nil, err
	}
	out := []Child{}
	for _, doc := range docs {
		var child Child
		if err := json.Unmarshal(doc.Data, &child); err != nil {
			return nil, err
		}
		if parent == "" || child.ParentID == parent {
			out = append(out, child)
		}
	}
	for i := range out {
		if out[i].Status == subagents.StatusQueued || out[i].Status == subagents.StatusRunning {
			w.mu.Lock()
			job := w.jobs[out[i].ID]
			active := false
			if job != nil {
				select {
				case <-job.done:
				default:
					active = true
				}
			}
			w.mu.Unlock()
			if !active {
				out[i].Status = subagents.Status("interrupted")
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}
func (w *Work) child(ctx context.Context, id string) (Child, error) {
	var child Child
	doc, err := w.Store.Get(ctx, "children", id)
	if err != nil {
		return child, err
	}
	err = json.Unmarshal(doc.Data, &child)
	return child, err
}
func (w *Work) Spawn(ctx context.Context, parent, name, prompt, taskID string) (Child, error) {
	if prompt == "" {
		return Child{}, errors.New("child prompt is required")
	}
	depth := 1
	if owner, err := w.child(ctx, parent); err == nil {
		depth = owner.Depth + 1
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Child{}, err
	}
	if depth > w.MaxDepth {
		return Child{}, errors.New("subagent depth limit exceeded")
	}
	children, err := w.Children(ctx, parent)
	if err != nil {
		return Child{}, err
	}
	if len(children) >= 64 {
		return Child{}, errors.New("parent child limit exceeded (64)")
	}
	metadata, err := w.Service.Metadata(ctx, parent)
	if err != nil {
		return Child{}, err
	}
	tasks, e := w.Tasks(ctx, parent)
	if e != nil {
		return Child{}, e
	}
	if taskID == "" && len(tasks.Tasks) > 0 {
		taskID = tasks.Tasks[0].ID
	}
	if taskID != "" {
		found := false
		for _, task := range tasks.Tasks {
			if task.ID == taskID {
				found = true
			}
		}
		if !found {
			return Child{}, errors.New("task does not belong to parent")
		}
	}
	child := Child{TaskID: taskID, Record: subagents.Record{ID: rand.Text(), ParentID: parent, Name: name, Depth: depth, Mode: subagents.ModeDurable, Status: subagents.StatusQueued, CreatedAt: time.Now().UTC(), Request: subagents.SpawnRequest{ParentID: parent, Prompt: prompt}}}
	m := metadata
	m.TaskID = taskID
	m.ParentID = parent
	m.Pending = nil
	m.InitialPrompt = prompt
	m.Worker = false
	m.Origin = ""
	m.OriginVersion = 0
	if err := w.Service.saveMetadata(ctx, child.ID, m); err != nil {
		return Child{}, err
	}
	if err := w.Store.Put(ctx, "children", child.ID, child); err != nil {
		return Child{}, err
	}
	if err := w.launch(ctx, child, false); err != nil {
		return child, err
	}
	return child, nil
}
func (w *Work) launch(parent context.Context, child Child, resume bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("worker scope is closed")
	}
	queued := 0
	for _, job := range w.jobs {
		select {
		case <-job.done:
		default:
			queued++
		}
	}
	if queued >= w.MaxConcurrent*16 {
		return errors.New("subagent queue is full")
	}
	if _, ok := w.jobs[child.ID]; ok {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	job := &childJob{done: make(chan struct{}), cancel: cancel}
	w.jobs[child.ID] = job
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer close(job.done)
		defer cancel()
		select {
		case w.slots <- struct{}{}:
		case <-ctx.Done():
			job.err = ctx.Err()
			if err := w.finish(child, Result{ID: child.ID, State: "canceled", ExitCode: 130}, job.err); err != nil {
				job.err = err
			}
			return
		}
		var slotMu sync.Mutex
		held := true
		release := func() {
			slotMu.Lock()
			defer slotMu.Unlock()
			if held {
				<-w.slots
				held = false
			}
		}
		w.mu.Lock()
		job.release = release
		job.reacquire = func(ctx context.Context) error {
			slotMu.Lock()
			defer slotMu.Unlock()
			if held {
				return nil
			}
			select {
			case w.slots <- struct{}{}:
				held = true
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		w.mu.Unlock()
		defer release()
		child.Status = subagents.StatusRunning
		child.StartedAt = time.Now().UTC()
		if err := w.Store.Put(ctx, "children", child.ID, child); err != nil {
			job.err = err
			return
		}
		req := Request{ID: child.ID, ParentID: child.ParentID}
		if !resume {
			req.newRun = true
			req.Prompt = child.Request.Prompt
			req.SessionID = ""
		}
		result, err := w.Service.Execute(ctx, req, nil)
		job.err = err
		if saveErr := w.finish(child, result, err); saveErr != nil {
			job.err = saveErr
		}
	}()
	return nil
}
func (w *Work) finish(child Child, result Result, err error) error {
	if current, e := w.child(context.Background(), child.ID); e == nil {
		child.ExplicitCancel = current.ExplicitCancel
	}
	child.Outcome = result
	child.FinishedAt = time.Now().UTC()
	switch result.ExitCode {
	case 0:
		child.Status = subagents.StatusCompleted
	case 3:
		child.Status = subagents.StatusWaitingApproval
	case 130:
		child.Status = subagents.StatusCanceled
	default:
		child.Status = subagents.StatusFailed
	}
	if err != nil {
		child.Error = err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return w.Store.Put(ctx, "children", child.ID, child)
}

// Waiting yields a child worker slot, preventing nested wait deadlocks.
func (w *Work) Wait(ctx context.Context, id string) error {
	w.mu.Lock()
	job := w.jobs[id]
	w.mu.Unlock()
	if job == nil {
		return nil
	}
	select {
	case <-job.done:
		return job.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (w *Work) WaitChildren(ctx context.Context, parent string) error {
	w.mu.Lock()
	job := w.jobs[parent]
	if job != nil && job.release != nil {
		job.release()
	}
	w.mu.Unlock()
	children, err := w.Children(ctx, parent)
	if err != nil {
		return err
	}
	for _, child := range children {
		waitErr := w.Wait(ctx, child.ID)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		current, err := w.child(ctx, child.ID)
		if err != nil {
			return err
		}
		if current.Outcome.ExitCode == 4 {
			return agent.ErrOperationInDoubt
		}
		if current.Status == subagents.StatusCanceled && current.ExplicitCancel {
			continue
		}
		if current.Status != subagents.StatusCompleted {
			if waitErr != nil && current.Status == subagents.StatusRunning {
				return waitErr
			}
			return ErrChildrenPending
		}
	}
	return ctx.Err()
}
func (w *Work) Recover(ctx context.Context, parent string) error {
	children, err := w.Children(ctx, parent)
	if err != nil {
		return err
	}
	for _, child := range children {
		if child.Depth > w.MaxDepth {
			return errors.New("persisted child exceeds current depth limit")
		}
		if child.ExplicitCancel || child.Status == subagents.StatusCompleted || child.Status == subagents.StatusWaitingApproval {
			continue
		}
		w.mu.Lock()
		old := w.jobs[child.ID]
		if old != nil {
			select {
			case <-old.done:
				delete(w.jobs, child.ID)
			default:
			}
		}
		w.mu.Unlock()
		if err := w.launch(ctx, child, true); err != nil {
			return err
		}
	}
	return nil
}
func (w *Work) CancelParent(parent string) {
	children, err := w.Children(context.Background(), parent)
	if err != nil {
		return
	}
	for _, child := range children {
		w.mu.Lock()
		job := w.jobs[child.ID]
		if job != nil {
			job.cancel()
		}
		w.mu.Unlock()
		w.CancelParent(child.ID)
	}
}
func (w *Work) Close(ctx context.Context) error {
	w.mu.Lock()
	w.closed = true
	for _, job := range w.jobs {
		job.cancel()
	}
	w.mu.Unlock()
	done := make(chan struct{})
	go func() { w.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (w *Work) AgentOption() agent.Option {
	return func(a *agent.Agent) {
		agent.WithRuntimeHooks(agent.RuntimeHooks{BeforeRequest: func(ctx context.Context, request *providers.StreamRequest) (*agent.HookedResponse, error) {
			if err := w.acquire(ctx, request.Options.RunID); err != nil {
				return nil, err
			}
			return nil, nil
		},
			BeforeComplete: func(ctx context.Context, _ agent.RunResult) (agent.CompletionDecision, error) {
				id := a.Snapshot().RunID
				if err := w.WaitChildren(ctx, id); err != nil {
					return agent.CompletionAllow, err
				}
				snap, err := w.Tasks(ctx, id)
				if err != nil {
					return agent.CompletionAllow, err
				}
				manager := taskmanager.New()
				if err := manager.Restore(snap); err != nil {
					return agent.CompletionAllow, err
				}
				if len(snap.Tasks) > 0 && !manager.AllComplete() {
					a.Steer(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{{Type: ai.ContentText, Text: "Finish the persisted task plan using task tools before completing."}}})
					return agent.CompletionContinue, nil
				}
				return w.completeEvidence(ctx, a, id)
			}})(a)
	}
}

func (w *Work) Charge(ctx context.Context, id string, tokens int64) error {
	root := id
	for i := 0; i <= w.MaxDepth; i++ {
		child, err := w.child(ctx, root)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		root = child.ParentID
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.budget == nil {
		w.budget = map[string]int64{}
	}
	used, ok := w.budget[root]
	if !ok {
		doc, err := w.Store.Get(ctx, "budgets", root)
		if err == nil {
			if err := json.Unmarshal(doc.Data, &used); err != nil {
				return err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if used+tokens > w.MaxBudget {
		return errors.New("shared worker token budget exhausted; increase limits.worker_tokens explicitly")
	}
	used += tokens
	if err := w.Store.Put(ctx, "budgets", root, used); err != nil {
		return err
	}
	w.budget[root] = used
	return nil
}

func (w *Work) yield(id string) {
	w.mu.Lock()
	job := w.jobs[id]
	w.mu.Unlock()
	if job != nil && job.release != nil {
		job.release()
	}
}
func (w *Work) acquire(ctx context.Context, id string) error {
	w.mu.Lock()
	job := w.jobs[id]
	var f func(context.Context) error
	if job != nil {
		f = job.reacquire
	}
	w.mu.Unlock()
	if f != nil {
		return f(ctx)
	}
	return nil
}

func (w *Work) EnsurePlan(ctx context.Context, id, prompt string) error {
	w.taskMu.Lock()
	defer w.taskMu.Unlock()
	snap, err := w.Tasks(ctx, id)
	if err != nil {
		return err
	}
	if len(snap.Tasks) > 0 {
		return nil
	}
	manager := taskmanager.New()
	if _, err := manager.Create(taskmanager.Task{Name: "Complete the requested coding task", Description: prompt}); err != nil {
		return err
	}
	return w.Store.Put(ctx, "tasks", id, manager.Snapshot())
}

func (w *Work) Descendant(ctx context.Context, id, parent string) bool {
	for depth := 0; depth <= w.MaxDepth; depth++ {
		child, err := w.child(ctx, id)
		if err != nil {
			return false
		}
		if child.ParentID == parent {
			return true
		}
		id = child.ParentID
	}
	return false
}

func (w *Work) completeEvidence(ctx context.Context, a *agent.Agent, id string) (agent.CompletionDecision, error) {
	children, err := w.Children(ctx, id)
	if err != nil {
		return agent.CompletionAllow, err
	}
	if len(children) == 0 {
		return agent.CompletionAllow, nil
	}
	type evidence struct {
		Status     string
		Error      string
		ID         string
		Checkpoint uint64
		Text       string
	}
	items := []evidence{}
	for _, child := range children {
		text := child.Outcome.Text
		if len(text) > 16384 {
			text = text[:16384] + " [truncated; inspect child session]"
		}
		items = append(items, evidence{Status: string(child.Status), Error: child.Error, ID: child.ID, Checkpoint: child.Outcome.Checkpoint, Text: text})
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return agent.CompletionAllow, err
	}
	marker := fmt.Sprintf("Opera child evidence %x", sha256.Sum256(raw))
	snapshot := a.Snapshot()
	for _, message := range append(snapshot.Transcript, snapshot.PendingSteering...) {
		for _, block := range message.Content {
			if strings.Contains(block.Text, marker) {
				return agent.CompletionAllow, nil
			}
		}
	}
	a.Steer(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{{Type: ai.ContentText, Text: marker + "\nReview these durable child results before completing:\n" + string(raw)}}})
	return agent.CompletionContinue, nil
}
