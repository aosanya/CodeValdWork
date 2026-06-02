package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

const (
	topicTaskStarted   = "ai.task.started"
	topicTaskCompleted = "ai.task.completed"
	topicTaskFailed    = "ai.task.failed"
	topicTodoCreated   = "ai.todo.created"
	topicTaskUpdate    = codevaldwork.TopicTaskUpdate
	// topicFileWritten is consumed from CodeValdGit to close the BUG-09-020
	// flush-race gate on work.todo.completed. String literal (not imported)
	// to keep CodeValdWork's no-cross-service-import rule intact.
	topicFileWritten = "git.file.written"
)

// aiTaskPayload is the common shape of ai.task.started/completed/failed payloads.
type aiTaskPayload struct {
	TaskID        string   `json:"TaskID"`
	RunID         string   `json:"RunID"`
	Reason        string   `json:"Reason"`
	HasSubtasks   bool     `json:"has_subtasks,omitempty"`
	EmittedWrites []string `json:"emitted_writes,omitempty"`
	WorkflowRunID string   `json:"workflow_run_id,omitempty"`
}

// aiTodoCreatedPayload mirrors the CodeValdAI TodoCreatedPayload — the ai.todo.created event body.
type aiTodoCreatedPayload struct {
	ParentTaskID  string       `json:"parent_task_id"`
	RunID         string       `json:"run_id"`
	AgentID       string       `json:"agent_id"`
	WorkflowRunID string       `json:"workflow_run_id,omitempty"`
	Todos         []aiTodoItem `json:"todos"`
}

type aiTodoItem struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Instructions   string `json:"instructions"`
	Ordinality     int    `json:"ordinality"`
	CanRunParallel bool   `json:"can_run_parallel"`
	DependsOn      []int  `json:"depends_on"`
	// Precalls is a JSON-encoded []PrecallSpec emitted by the LLM when it
	// creates the todo; stored on the TaskTodo entity and threaded through the
	// TodoDispatchedPayload so HydrateEventContext can execute them before the
	// agent runs.
	Precalls string `json:"precalls,omitempty"`
}

// TaskEventDispatcher bridges ai.task.* events into work task / todo status transitions
// and materialises ai.task.todo decompositions into TaskTodo entities.
type TaskEventDispatcher struct {
	mgr       codevaldwork.TaskManager
	agencyID  string
	pub       eventbus.Publisher // optional; nil = skip work.task.failed bridging
	writes    *writeTracker      // gates ai.task.completed on git.file.written (BUG-09-020 Phase 2)
	runStatus *RunStatusHandler  // drives WorkflowRun status transitions
}

// NewTaskEventDispatcher constructs a TaskEventDispatcher.
// pub may be nil — work.task.failed bridging is skipped when no publisher is set.
func NewTaskEventDispatcher(mgr codevaldwork.TaskManager, agencyID string, pub eventbus.Publisher) *TaskEventDispatcher {
	return &TaskEventDispatcher{
		mgr:       mgr,
		agencyID:  agencyID,
		pub:       pub,
		writes:    newWriteTracker(writeGateTimeout),
		runStatus: NewRunStatusHandler(mgr, agencyID),
	}
}

// Dispatch handles an incoming event by topic.
func (d *TaskEventDispatcher) Dispatch(ctx context.Context, topic, payload string) {
	switch topic {
	case topicTodoCreated:
		d.handleAITodoCreated(ctx, payload)
	case topicTaskUpdate:
		d.handleTaskUpdate(ctx, payload)
	case topicTaskStarted, topicTaskCompleted, topicTaskFailed:
		d.handleAITaskStatus(ctx, topic, payload)
	case codevaldwork.TopicTaskCompleted:
		d.handleWorkTaskCompleted(ctx, payload)
	case topicFileWritten:
		d.handleFileWritten(ctx, payload)
	}
	// Always check for WorkflowRun status transitions on every event that
	// carries a workflow_run_id — handles both the hardcoded failure topics
	// and any terminal_event topic configured on the run.
	if d.runStatus != nil {
		d.runStatus.HandleEvent(ctx, topic, payload)
	}
}

// handleWorkTaskCompleted runs the auto-unblock cascade when a Task lands in a
// terminal status. Only the `completed` terminus opens dependent gates —
// `failed` and `cancelled` leave dependents blocked (a follow-up gap will
// decide whether to cascade-fail or release them).
//
// Self-loop safety: CodeValdWork publishes work.task.completed too, so this
// handler will receive its own emissions. UnblockDependents is idempotent —
// dependents already pending/in-progress are filtered by the status==blocked
// check — so re-delivery and self-receipts are no-ops.
func (d *TaskEventDispatcher) handleWorkTaskCompleted(ctx context.Context, payloadStr string) {
	var p codevaldwork.TaskCompletedPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.TaskID == "" {
		log.Printf("codevaldwork: handleWorkTaskCompleted: bad payload: %v", err)
		return
	}
	if p.TerminalStatus != codevaldwork.TaskStatusCompleted {
		return
	}
	if err := d.mgr.UnblockDependents(ctx, d.agencyID, p.TaskID); err != nil {
		log.Printf("codevaldwork: handleWorkTaskCompleted: UnblockDependents %s: %v", p.TaskID, err)
		return
	}
	log.Printf("codevaldwork: handleWorkTaskCompleted: ran unblock cascade for task=%s", p.TaskID)
}

// handleAITaskStatus routes ai.task.started/completed/failed to a Task or
// TaskTodo status update depending on which entity the TaskID refers to.
//
// For ai.task.completed payloads that carry EmittedWrites, the actual status
// transition is deferred until every emitted path has been confirmed by a
// matching git.file.written event (BUG-09-020 Phase 2). A timeout falls back
// to firing anyway with a warning so the pipeline cannot stall.
func (d *TaskEventDispatcher) handleAITaskStatus(ctx context.Context, topic, payloadStr string) {
	var p aiTaskPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.TaskID == "" {
		log.Printf("codevaldwork: TaskEventDispatcher: bad payload for topic=%s: %v", topic, err)
		return
	}

	if topic == topicTaskCompleted && len(p.EmittedWrites) > 0 && p.RunID != "" && d.writes != nil {
		log.Printf("codevaldwork: TaskEventDispatcher: gating ai.task.completed task=%s run=%s on %d emitted write(s)",
			p.TaskID, p.RunID, len(p.EmittedWrites))
		d.writes.WaitForWrites(p.RunID, p.EmittedWrites, func() {
			d.applyAITaskStatus(context.Background(), topic, p)
		})
		return
	}

	d.applyAITaskStatus(ctx, topic, p)
}

// applyAITaskStatus performs the actual Task/Todo status transition described
// by an ai.task.* event. It is called either directly from handleAITaskStatus
// (no writes to wait for) or by writeTracker once all emitted writes have been
// confirmed (or the 30 s gate timeout has expired).
func (d *TaskEventDispatcher) applyAITaskStatus(ctx context.Context, topic string, p aiTaskPayload) {
	var nextStatus codevaldwork.TaskStatus
	switch topic {
	case topicTaskStarted:
		nextStatus = codevaldwork.TaskStatusInProgress
	case topicTaskCompleted:
		nextStatus = codevaldwork.TaskStatusCompleted
	case topicTaskFailed:
		nextStatus = codevaldwork.TaskStatusFailed
	}

	task, err := d.mgr.GetTask(ctx, d.agencyID, p.TaskID)
	if err != nil {
		if errors.Is(err, codevaldwork.ErrTaskNotFound) {
			// May be a TodoID — map ai.task.* to the TodoStatus lifecycle.
			d.updateTodoStatus(ctx, p.TaskID, topic)
			return
		}
		log.Printf("codevaldwork: TaskEventDispatcher: GetTask %s: %v", p.TaskID, err)
		return
	}

	// When the run produced sub-todos, do not mark the parent task completed yet.
	// work.task.completed will be published by maybeCompleteParentTask once all
	// todos reach a terminal state.
	if topic == topicTaskCompleted && p.HasSubtasks {
		log.Printf("codevaldwork: TaskEventDispatcher: task %s has subtasks — deferring completion", p.TaskID)
		return
	}

	task.Status = nextStatus
	if _, err := d.mgr.UpdateTask(ctx, d.agencyID, task); err != nil {
		log.Printf("codevaldwork: TaskEventDispatcher: UpdateTask %s → %s: %v", p.TaskID, nextStatus, err)
		// Fall through: for ai.task.failed we still bridge to work.task.failed even if
		// the status transition is blocked (e.g. task is already in a terminal state).
	} else {
		log.Printf("codevaldwork: TaskEventDispatcher: task %s → %s", p.TaskID, nextStatus)
	}

	// Bridge ai.task.failed → work.task.failed so CodeValdAI's task-failed-operations-handler fires.
	// Published regardless of whether the status update succeeded — a failed run always
	// warrants the operations-officer review even if the task was already in a terminal state.
	if topic == topicTaskFailed {
		runID := p.WorkflowRunID
		if runID == "" {
			runID = task.WorkflowRunID
		}
		eventbus.SafePublish(ctx, d.pub, eventbus.Event{
			Topic:    codevaldwork.TopicTaskFailed,
			AgencyID: d.agencyID,
			Payload: codevaldwork.TaskFailedPayload{
				TaskID:        p.TaskID,
				RunID:         p.RunID,
				Reason:        p.Reason,
				WorkflowRunID: runID,
			},
		})
		log.Printf("codevaldwork: TaskEventDispatcher: bridged work.task.failed for task=%s run=%s", p.TaskID, p.RunID)
	}
}

// FailTodoWithCascade marks todoID as failed and runs the full cascade:
// blockDependentTodos → maybeCompleteParentTask. It is the public entry point
// for the FailTodo gRPC method (and QA test tooling) so the cascade logic is
// not duplicated outside the dispatcher.
func (d *TaskEventDispatcher) FailTodoWithCascade(ctx context.Context, todoID string) error {
	updated, err := d.mgr.UpdateTaskTodoStatus(ctx, d.agencyID, todoID, codevaldwork.TodoStatusFailed)
	if err != nil {
		return err
	}
	log.Printf("codevaldwork: FailTodoWithCascade: todo %s → failed", todoID)
	if updated.ParentTaskID != "" {
		d.blockDependentTodos(ctx, updated.ParentTaskID, updated.Ordinality)
		d.maybeCompleteParentTask(ctx, updated.ParentTaskID)
	}
	return nil
}

// updateTodoStatus transitions a TaskTodo when an ai.task.* event arrives with a TodoID.
func (d *TaskEventDispatcher) updateTodoStatus(ctx context.Context, todoID, topic string) {
	var status codevaldwork.TodoStatus
	switch topic {
	case topicTaskStarted:
		status = codevaldwork.TodoStatusDispatched
	case topicTaskCompleted:
		status = codevaldwork.TodoStatusCompleted
	case topicTaskFailed:
		status = codevaldwork.TodoStatusFailed
	default:
		return
	}
	updated, err := d.mgr.UpdateTaskTodoStatus(ctx, d.agencyID, todoID, status)
	if err != nil {
		log.Printf("codevaldwork: TaskEventDispatcher: UpdateTaskTodoStatus %s → %s: %v", todoID, status, err)
		return
	}
	log.Printf("codevaldwork: TaskEventDispatcher: todo %s → %s", todoID, status)

	if updated.ParentTaskID == "" {
		return
	}

	if status == codevaldwork.TodoStatusCompleted || status == codevaldwork.TodoStatusFailed {
		eventbus.SafePublish(ctx, d.pub, eventbus.Event{
			Topic:    codevaldwork.TopicTodoCompleted,
			AgencyID: d.agencyID,
			Payload: codevaldwork.TodoCompletedPayload{
				TodoID:        updated.ID,
				ParentTaskID:  updated.ParentTaskID,
				Title:         updated.Title,
				Status:        string(status),
				WorkflowRunID: updated.WorkflowRunID,
			},
		})
		log.Printf("codevaldwork: TaskEventDispatcher: published %s todo=%s task=%s", codevaldwork.TopicTodoCompleted, updated.ID, updated.ParentTaskID)
	}

	switch status {
	case codevaldwork.TodoStatusCompleted:
		// Unblock any todos that were waiting on this one.
		d.unblockDependentTodos(ctx, updated.ParentTaskID, updated.Ordinality)
		d.maybeCompleteParentTask(ctx, updated.ParentTaskID)
	case codevaldwork.TodoStatusFailed:
		// Cascade failure to every blocked todo that (transitively) depended on this one.
		d.blockDependentTodos(ctx, updated.ParentTaskID, updated.Ordinality)
		d.maybeCompleteParentTask(ctx, updated.ParentTaskID)
	}
}

// maybeCompleteParentTask marks a Task completed or failed once all child todos
// are terminal. Terminal states are: completed, failed, blocked (cascade-failed).
func (d *TaskEventDispatcher) maybeCompleteParentTask(ctx context.Context, taskID string) {
	edges, err := d.mgr.TraverseRelationships(ctx, d.agencyID, taskID, codevaldwork.RelLabelHasTodo, codevaldwork.DirectionOutbound)
	if err != nil {
		log.Printf("codevaldwork: maybeCompleteParentTask: TraverseRelationships task=%s: %v", taskID, err)
		return
	}
	anyFailed := false
	for _, edge := range edges {
		todo, err := d.mgr.GetTaskTodo(ctx, d.agencyID, edge.ToID)
		if err != nil {
			return
		}
		switch todo.Status {
		case codevaldwork.TodoStatusCompleted:
			// fine
		case codevaldwork.TodoStatusFailed, codevaldwork.TodoStatusBlocked:
			anyFailed = true
		default:
			return // pending, dispatched — not yet terminal
		}
	}

	task, err := d.mgr.GetTask(ctx, d.agencyID, taskID)
	if err != nil {
		log.Printf("codevaldwork: maybeCompleteParentTask: GetTask %s: %v", taskID, err)
		return
	}
	if task.Status == codevaldwork.TaskStatusCompleted || task.Status == codevaldwork.TaskStatusFailed {
		return
	}
	if anyFailed {
		task.Status = codevaldwork.TaskStatusFailed
	} else {
		task.Status = codevaldwork.TaskStatusCompleted
	}
	if _, err := d.mgr.UpdateTask(ctx, d.agencyID, task); err != nil {
		log.Printf("codevaldwork: maybeCompleteParentTask: UpdateTask %s: %v", taskID, err)
		return
	}
	log.Printf("codevaldwork: maybeCompleteParentTask: task %s → %s (all todos terminal)", taskID, task.Status)
}

// unblockDependentTodos dispatches any blocked todos whose entire depends_on
// list is now satisfied after completedOrdinality just completed.
func (d *TaskEventDispatcher) unblockDependentTodos(ctx context.Context, taskID string, completedOrdinality int) {
	edges, err := d.mgr.TraverseRelationships(ctx, d.agencyID, taskID, codevaldwork.RelLabelHasTodo, codevaldwork.DirectionOutbound)
	if err != nil {
		log.Printf("codevaldwork: unblockDependentTodos: TraverseRelationships task=%s: %v", taskID, err)
		return
	}

	// Build ordinality → status map for the entire sibling set.
	ordStatus := make(map[int]codevaldwork.TodoStatus, len(edges))
	todos := make(map[string]codevaldwork.TaskTodo, len(edges))
	for _, edge := range edges {
		todo, err := d.mgr.GetTaskTodo(ctx, d.agencyID, edge.ToID)
		if err != nil {
			continue
		}
		ordStatus[todo.Ordinality] = todo.Status
		todos[edge.ToID] = todo
	}

	for todoID, todo := range todos {
		if todo.Status != codevaldwork.TodoStatusBlocked {
			continue
		}
		dependsOnThis := false
		for _, dep := range todo.DependsOn {
			if dep == completedOrdinality {
				dependsOnThis = true
				break
			}
		}
		if !dependsOnThis {
			continue
		}
		// Only dispatch if every dependency is now completed.
		allSatisfied := true
		for _, dep := range todo.DependsOn {
			if ordStatus[dep] != codevaldwork.TodoStatusCompleted {
				allSatisfied = false
				break
			}
		}
		if allSatisfied {
			if err := d.mgr.DispatchTaskTodo(ctx, d.agencyID, todoID); err != nil {
				log.Printf("codevaldwork: unblockDependentTodos: DispatchTaskTodo %s: %v", todoID, err)
			} else {
				log.Printf("codevaldwork: unblockDependentTodos: dispatched todo %s (ordinality=%d)", todoID, todo.Ordinality)
			}
		}
	}
}

// blockDependentTodos cascade-fails every blocked todo that directly or
// transitively depended on failedOrdinality.
func (d *TaskEventDispatcher) blockDependentTodos(ctx context.Context, taskID string, failedOrdinality int) {
	edges, err := d.mgr.TraverseRelationships(ctx, d.agencyID, taskID, codevaldwork.RelLabelHasTodo, codevaldwork.DirectionOutbound)
	if err != nil {
		log.Printf("codevaldwork: blockDependentTodos: TraverseRelationships task=%s: %v", taskID, err)
		return
	}
	for _, edge := range edges {
		todo, err := d.mgr.GetTaskTodo(ctx, d.agencyID, edge.ToID)
		if err != nil || todo.Status != codevaldwork.TodoStatusBlocked {
			continue
		}
		for _, dep := range todo.DependsOn {
			if dep == failedOrdinality {
				if _, err := d.mgr.UpdateTaskTodoStatus(ctx, d.agencyID, edge.ToID, codevaldwork.TodoStatusFailed); err != nil {
					log.Printf("codevaldwork: blockDependentTodos: UpdateTaskTodoStatus %s: %v", edge.ToID, err)
				} else {
					log.Printf("codevaldwork: blockDependentTodos: cascade-failed todo %s (ordinality=%d)", edge.ToID, todo.Ordinality)
					// Recurse: this todo is now failed; cascade to its own dependents.
					d.blockDependentTodos(ctx, taskID, todo.Ordinality)
				}
				break
			}
		}
	}
}

// handleAITodoCreated consumes an ai.todo.created decomposition payload:
// creates one TaskTodo entity per item, wires graph edges, then dispatches
// only root todos (those with empty depends_on). Todos with dependencies start
// as blocked and are dispatched by unblockDependentTodos once their
// predecessors complete.
func (d *TaskEventDispatcher) handleAITodoCreated(ctx context.Context, payloadStr string) {
	var p aiTodoCreatedPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.ParentTaskID == "" {
		log.Printf("codevaldwork: TaskEventDispatcher: ai.todo.created: bad payload: %v", err)
		return
	}
	log.Printf("codevaldwork: TaskEventDispatcher: ai.todo.created: parent=%s items=%d", p.ParentTaskID, len(p.Todos))

	type createdTodo struct {
		id       string
		isRoot   bool
	}
	created := make([]createdTodo, 0, len(p.Todos))

	agentEntityID, externalAgentID := d.resolveAgentForTask(ctx, p.ParentTaskID, p.AgentID)

	// Inherit workflow_run_id from the parent Task per FEAT-20260602-002
	// chain-through rule. Prefer the event payload's explicit value when
	// present; otherwise read the parent.
	inheritedRunID := p.WorkflowRunID
	if inheritedRunID == "" {
		if parent, err := d.mgr.GetTask(ctx, d.agencyID, p.ParentTaskID); err == nil {
			inheritedRunID = parent.WorkflowRunID
		}
	}

	for _, item := range p.Todos {
		todo := codevaldwork.TaskTodo{
			Title:          item.Title,
			Description:    item.Description,
			Instructions:   item.Instructions,
			Ordinality:     item.Ordinality,
			CanRunParallel: item.CanRunParallel,
			DependsOn:      item.DependsOn,
			ParentTaskID:   p.ParentTaskID,
			DecompRunID:    p.RunID,
			AgentID:        externalAgentID,
			Precalls:       item.Precalls,
			WorkflowRunID:  inheritedRunID,
		}

		ct, err := d.mgr.CreateTaskTodo(ctx, d.agencyID, todo)
		if err != nil {
			log.Printf("codevaldwork: TaskEventDispatcher: CreateTaskTodo ordinality=%d: %v", item.Ordinality, err)
			continue
		}

		// has_todo edge: Task → TaskTodo
		if _, err := d.mgr.CreateRelationship(ctx, d.agencyID, codevaldwork.Relationship{
			Label:  codevaldwork.RelLabelHasTodo,
			FromID: p.ParentTaskID,
			ToID:   ct.ID,
		}); err != nil {
			log.Printf("codevaldwork: TaskEventDispatcher: has_todo edge todo=%s: %v", ct.ID, err)
		}

		// todo_assigned_to edge: TaskTodo → Agent
		if agentEntityID != "" {
			if _, err := d.mgr.CreateRelationship(ctx, d.agencyID, codevaldwork.Relationship{
				Label:  codevaldwork.RelLabelTodoAssignedTo,
				FromID: ct.ID,
				ToID:   agentEntityID,
			}); err != nil {
				log.Printf("codevaldwork: TaskEventDispatcher: todo_assigned_to edge todo=%s agent=%s: %v",
					ct.ID, agentEntityID, err)
			}
		}

		log.Printf("codevaldwork: TaskEventDispatcher: created TaskTodo id=%s ordinality=%d status=%s agent=%s",
			ct.ID, item.Ordinality, ct.Status, externalAgentID)
		created = append(created, createdTodo{id: ct.ID, isRoot: len(item.DependsOn) == 0})
	}

	// Dispatch only root todos — those with no depends_on. Dependent todos are
	// blocked and will be dispatched by unblockDependentTodos as predecessors complete.
	for _, ct := range created {
		if !ct.isRoot {
			continue
		}
		if err := d.mgr.DispatchTaskTodo(ctx, d.agencyID, ct.id); err != nil {
			log.Printf("codevaldwork: TaskEventDispatcher: DispatchTaskTodo root todo=%s: %v", ct.id, err)
		} else {
			log.Printf("codevaldwork: TaskEventDispatcher: dispatched root todo=%s", ct.id)
		}
	}
}

// handleTaskUpdate processes a work.task.update event published by CodeValdAI
// when the LLM chooses a branch name. It patches only the non-nil fields in
// the payload (currently branch_name) onto the Task entity.
func (d *TaskEventDispatcher) handleTaskUpdate(ctx context.Context, payloadStr string) {
	var p codevaldwork.TaskUpdatePayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.TaskID == "" {
		log.Printf("codevaldwork: handleTaskUpdate: bad payload: %v", err)
		return
	}
	task, err := d.mgr.GetTask(ctx, d.agencyID, p.TaskID)
	if err != nil {
		log.Printf("codevaldwork: handleTaskUpdate: GetTask %s: %v", p.TaskID, err)
		return
	}
	if p.BranchName != "" {
		task.BranchName = p.BranchName
	}
	if _, err := d.mgr.UpdateTask(ctx, d.agencyID, task); err != nil {
		log.Printf("codevaldwork: handleTaskUpdate: UpdateTask %s: %v", p.TaskID, err)
		return
	}
	log.Printf("codevaldwork: handleTaskUpdate: task=%s branch_name=%q", p.TaskID, p.BranchName)
}

// resolveAgentForTask returns the (entity ID, external agent ID) for the agent
// assigned to taskID. If the task has no assignee, falls back to fallbackAgentID
// by upserting an Agent vertex.
func (d *TaskEventDispatcher) resolveAgentForTask(ctx context.Context, taskID, fallbackAgentID string) (entityID, externalID string) {
	edges, err := d.mgr.TraverseRelationships(ctx, d.agencyID, taskID, codevaldwork.RelLabelAssignedTo, codevaldwork.DirectionOutbound)
	if err == nil && len(edges) > 0 {
		agent, err := d.mgr.GetAgent(ctx, d.agencyID, edges[0].ToID)
		if err == nil {
			return edges[0].ToID, agent.AgentID
		}
	}
	if fallbackAgentID == "" {
		return "", ""
	}
	agent, err := d.mgr.UpsertAgent(ctx, d.agencyID, codevaldwork.Agent{AgentID: fallbackAgentID})
	if err != nil {
		log.Printf("codevaldwork: TaskEventDispatcher: UpsertAgent %s: %v", fallbackAgentID, err)
		return "", fallbackAgentID
	}
	return agent.ID, fallbackAgentID
}
