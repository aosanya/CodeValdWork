package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdWork/internal/reviewer"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

const (
	topicTaskStarted   = "task.started"
	topicTaskCompleted = "task.completed"
	topicTaskFailed    = "task.failed"
	topicTodoCreated   = "todo.created"
	topicTaskUpdate    = codevaldwork.TopicTaskUpdate
	// topicFileWritten is consumed from CodeValdGit to close the BUG-09-020
	// flush-race gate on work.todo.completed. String literal (not imported)
	// to keep CodeValdWork's no-cross-service-import rule intact.
	topicFileWritten = "git.file.written"
	// topicTaskDirection is the inbound direction response from the AI failure
	// reviewer or a human submitting the direction form.
	topicTaskDirection = codevaldwork.TopicTaskDirection
	// topicTaskFailureClassified is the AI classification response for the
	// retry-ladder escalation path (FEAT-20260603-003).
	topicTaskFailureClassified = codevaldwork.TopicTaskFailureClassified

	// topicTaskPlanSplit is emitted by the planner agent when it decides to
	// break a task into child Task entities (FEAT-20260604-001).
	topicTaskPlanSplit = codevaldwork.TopicTaskPlanSplit

	// defaultMaxRecoveryRuns is the platform-default retry budget per task.
	// Configurable per WorkPlan via max_recovery_runs; this value is used
	// when the WorkPlan property is absent (FEAT-20260603-003).
	defaultMaxRecoveryRuns = 3
)

// aiTaskPayload is the common shape of task.started/completed/failed payloads from CodeValdAI.
type aiTaskPayload struct {
	TaskID        string   `json:"TaskID"`
	RunID         string   `json:"RunID"`
	Reason        string   `json:"Reason"`
	HasSubtasks   bool     `json:"has_subtasks,omitempty"`
	EmittedWrites []string `json:"emitted_writes,omitempty"`
	WorkflowRunID string   `json:"workflow_run_id,omitempty"`
}

// aiTodoCreatedPayload mirrors the CodeValdAI TodoCreatedPayload — the todo.created event body.
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

// TaskEventDispatcher bridges task.* events from CodeValdAI into work task / todo status transitions
// and materialises todo.created decompositions into TaskTodo entities.
type TaskEventDispatcher struct {
	mgr       codevaldwork.TaskManager
	agencyID  string
	pub       eventbus.Publisher // optional; nil = skip work.task.failed bridging
	writes    *writeTracker      // gates task.completed on git.file.written (BUG-09-020 Phase 2)
	runStatus *RunStatusHandler  // drives WorkflowRun status transitions
	rev       *reviewer.Reviewer // optional; nil = skip review gate (FEAT-20260605-003)
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

// WithReviewer attaches a Reviewer to the dispatcher so the review gate fires
// after every work.task.completed event (FEAT-20260605-003).
func (d *TaskEventDispatcher) WithReviewer(r *reviewer.Reviewer) {
	d.rev = r
}

// Dispatch handles an incoming event by topic.
//
// After BUG-20260609-001 (AI) the ai.* prefix is gone, so the dispatcher
// sees the same topic name "task.completed" from two producers: CodeValdAI
// (lifecycle event from an AgentRun) and CodeValdWork itself (terminal-state
// roll-up). Both handlers run on that single case; handleAITaskStatus
// distinguishes the two by RunID (only present in AI's payload).
func (d *TaskEventDispatcher) Dispatch(ctx context.Context, topic, payload string) {
	switch topic {
	case topicTodoCreated:
		d.handleAITodoCreated(ctx, payload)
	case topicTaskUpdate:
		d.handleTaskUpdate(ctx, payload)
	case topicTaskCompleted: // = codevaldwork.TopicTaskCompleted after the rename
		d.handleAITaskStatus(ctx, topic, payload)
		d.handleWorkTaskCompleted(ctx, payload)
	case topicTaskStarted, topicTaskFailed:
		d.handleAITaskStatus(ctx, topic, payload)
	case topicFileWritten:
		d.handleFileWritten(ctx, payload)
	case codevaldwork.TopicRunTimeout:
		d.handleRunTimeout(ctx, payload)
	case codevaldwork.TopicTaskTimeout:
		d.handleTaskTimeout(ctx, payload)
	case topicTaskDirection:
		d.handleTaskDirection(ctx, payload)
	case topicTaskFailureClassified:
		d.handleTaskFailureClassified(ctx, payload)
	case topicTaskPlanSplit:
		d.handleTaskPlanSplit(ctx, payload)
	}
	// Always check for WorkflowRun status transitions on every event that
	// carries a workflow_run_id — handles both the hardcoded failure topics
	// and any terminal_event topic configured on the run.
	if d.runStatus != nil {
		d.runStatus.HandleEvent(ctx, topic, payload)
	}
}

// handleRunTimeout processes a work.run.timeout event by delegating to
// TaskManager.HandleRunTimeout (FEAT-20260602-006).
func (d *TaskEventDispatcher) handleRunTimeout(ctx context.Context, payloadStr string) {
	var p codevaldwork.WorkflowRunTimeoutPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.WorkflowRunID == "" {
		log.Printf("codevaldwork: handleRunTimeout: bad payload: %v", err)
		return
	}
	if err := d.mgr.HandleRunTimeout(ctx, d.agencyID, p.WorkflowRunID); err != nil {
		log.Printf("codevaldwork: handleRunTimeout: run=%s: %v", p.WorkflowRunID, err)
	}
}

// handleTaskTimeout processes a work.task.timeout event by delegating to
// TaskManager.HandleTaskTimeout (FEAT-20260602-006).
func (d *TaskEventDispatcher) handleTaskTimeout(ctx context.Context, payloadStr string) {
	var p codevaldwork.WorkflowRunTaskTimeoutPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.WorkflowRunID == "" {
		log.Printf("codevaldwork: handleTaskTimeout: bad payload: %v", err)
		return
	}
	if err := d.mgr.HandleTaskTimeout(ctx, d.agencyID, p.StepID, p.WorkflowRunID); err != nil {
		log.Printf("codevaldwork: handleTaskTimeout: step=%s run=%s: %v", p.StepID, p.WorkflowRunID, err)
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

	if d.rev != nil {
		go d.rev.Review(context.Background(), p.TaskID, p.WorkflowRunID)
	}
}

// handleAITaskStatus routes task.started/completed/failed to a Task or
// TaskTodo status update depending on which entity the TaskID refers to.
//
// For task.completed payloads that carry EmittedWrites, the actual status
// transition is deferred until every emitted path has been confirmed by a
// matching git.file.written event (BUG-09-020 Phase 2). A timeout falls back
// to firing anyway with a warning so the pipeline cannot stall.
func (d *TaskEventDispatcher) handleAITaskStatus(ctx context.Context, topic, payloadStr string) {
	var p aiTaskPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.TaskID == "" {
		log.Printf("codevaldwork: TaskEventDispatcher: bad payload for topic=%s: %v", topic, err)
		return
	}
	// After the ai. prefix rename, Work itself also publishes task.completed
	// (and task.failed) when its UpdateTask hook fires. Those emissions carry
	// no RunID. Skip them here — handleWorkTaskCompleted handles the Work
	// side. AI-originated payloads always carry RunID (the AgentRun ID).
	if p.RunID == "" {
		return
	}

	if topic == topicTaskCompleted && len(p.EmittedWrites) > 0 && p.RunID != "" && d.writes != nil {
		log.Printf("codevaldwork: TaskEventDispatcher: gating task.completed task=%s run=%s on %d emitted write(s)",
			p.TaskID, p.RunID, len(p.EmittedWrites))
		d.writes.WaitForWrites(p.RunID, p.EmittedWrites, func() {
			d.applyAITaskStatus(context.Background(), topic, p)
		})
		return
	}

	d.applyAITaskStatus(ctx, topic, p)
}

// applyAITaskStatus performs the actual Task/Todo status transition described
// by a task.* event from CodeValdAI. It is called either directly from handleAITaskStatus
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
			// May be a TodoID — map task.* to the TodoStatus lifecycle.
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

	// Retry ladder (FEAT-20260603-003): on failure, try automatic retries before
	// escalating to AI classification and then human direction.
	if topic == topicTaskFailed {
		runID := p.WorkflowRunID
		if runID == "" {
			runID = task.WorkflowRunID
		}
		if d.handleTaskFailureWithRetry(ctx, task, p.RunID, p.Reason, runID) {
			// Retry or escalation path handled — do not fall through to the
			// standard failed-status update or work.task.failed bridge.
			return
		}
		// Budget exhausted and classification emitted; fall through to mark
		// the task failed so the run status handler sees the transition.
	}

	task.Status = nextStatus
	if _, err := d.mgr.UpdateTask(ctx, d.agencyID, task); err != nil {
		log.Printf("codevaldwork: TaskEventDispatcher: UpdateTask %s → %s: %v", p.TaskID, nextStatus, err)
	} else {
		log.Printf("codevaldwork: TaskEventDispatcher: task %s → %s", p.TaskID, nextStatus)
	}

	// (Prior to BUG-20260609-001 this handler re-published a work.task.failed
	// bridge so the AI task-failed-operations-handler WorkPlan would fire.
	// After the rename `ai.task.failed` and `work.task.failed` collapsed into
	// a single `task.failed` topic — the WorkPlan now matches CodeValdAI's
	// original emission directly, so the bridge is redundant and is gone.)

	// If this task is a child of a split-plan parent, roll up completion once
	// all siblings reach a terminal state.
	if (nextStatus == codevaldwork.TaskStatusCompleted || nextStatus == codevaldwork.TaskStatusFailed) && task.ParentTaskID != "" {
		d.maybeCompleteSplitParent(ctx, task.ParentTaskID)
	}
}

// handleTaskFailureWithRetry implements the three-phase recovery ladder
// (FEAT-20260603-003). Returns true when the failure was intercepted by the
// ladder (retry dispatched or classification emitted) so the caller should
// not proceed with the normal failed-status path.
//
// Phase 1 — budget not exhausted: increment recovery_runs_used, re-dispatch.
// Phase 2 — budget exhausted: emit work.task.classify-failure for AI to classify.
// The classification response is handled by handleTaskFailureClassified.
func (d *TaskEventDispatcher) handleTaskFailureWithRetry(ctx context.Context, task codevaldwork.Task, runID, reason, workflowRunID string) bool {
	task.RecoveryRunsUsed++
	task.UpdatedAt = nowRFC3339()
	if _, err := d.mgr.UpdateTask(ctx, d.agencyID, task); err != nil {
		log.Printf("codevaldwork: handleTaskFailureWithRetry: UpdateTask %s: %v", task.ID, err)
		// Fall through to normal failure path on storage error.
		return false
	}

	if task.RecoveryRunsUsed < defaultMaxRecoveryRuns {
		// Phase 1: automatic retry. Re-publish work.task.assigned so the
		// developer-assigned-handler fires another AI run.
		agentEntityID, _ := d.resolveAgentForTask(ctx, task.ID, task.AssignedTo)
		roleName := ""
		if agentEntityID != "" {
			if ag, err := d.mgr.GetAgent(ctx, d.agencyID, agentEntityID); err == nil {
				roleName = ag.RoleName
			}
		}
		eventbus.SafePublish(ctx, d.pub, eventbus.Event{
			Topic:    codevaldwork.TopicTaskAssigned,
			AgencyID: d.agencyID,
			Payload: codevaldwork.TaskAssignedPayload{
				TaskID:        task.ID,
				AgentID:       task.AssignedTo,
				RoleName:      roleName,
				TaskCode:      task.TaskName,
				Title:         task.Title,
				Description:   task.Description,
				WorkflowRunID: workflowRunID,
			},
		})
		log.Printf("codevaldwork: handleTaskFailureWithRetry: retry %d/%d task=%s",
			task.RecoveryRunsUsed, defaultMaxRecoveryRuns, task.ID)
		return true
	}

	// Phase 2: budget exhausted — request AI classification.
	// Find the most-recently-failed todo for context.
	var failedTodoTitle, failedTodoInstructions string
	edges, _ := d.mgr.TraverseRelationships(ctx, d.agencyID, task.ID, codevaldwork.RelLabelHasTodo, codevaldwork.DirectionOutbound)
	for _, edge := range edges {
		if todo, err := d.mgr.GetTaskTodo(ctx, d.agencyID, edge.ToID); err == nil && todo.Status == codevaldwork.TodoStatusFailed {
			failedTodoTitle = todo.Title
			failedTodoInstructions = todo.Instructions
		}
	}
	eventbus.SafePublish(ctx, d.pub, eventbus.Event{
		Topic:    codevaldwork.TopicTaskClassifyFailure,
		AgencyID: d.agencyID,
		Payload: codevaldwork.TaskClassifyFailurePayload{
			TaskID:                 task.ID,
			WorkflowRunID:          workflowRunID,
			FailureCount:           task.RecoveryRunsUsed,
			LastFailureReason:      reason,
			TaskDescription:        task.Description,
			FailedTodoTitle:        failedTodoTitle,
			FailedTodoInstructions: failedTodoInstructions,
		},
	})
	log.Printf("codevaldwork: handleTaskFailureWithRetry: budget exhausted — emitted classify-failure task=%s", task.ID)
	// Return true: caller should NOT immediately mark the task as failed.
	// The classification response will determine the next transition.
	return true
}

// handleTaskFailureClassified processes a work.task.failure-classified event.
// transient → grant one extra retry (re-dispatch work.task.assigned).
// requires-human (or classification failure) → transition task → awaiting-direction,
// emit work.task.needs-direction, pause the WorkflowRun.
func (d *TaskEventDispatcher) handleTaskFailureClassified(ctx context.Context, payloadStr string) {
	var p codevaldwork.TaskFailureClassifiedPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.TaskID == "" {
		log.Printf("codevaldwork: handleTaskFailureClassified: bad payload: %v", err)
		return
	}
	log.Printf("codevaldwork: handleTaskFailureClassified: task=%s type=%s", p.TaskID, p.FailureType)

	task, err := d.mgr.GetTask(ctx, d.agencyID, p.TaskID)
	if err != nil {
		log.Printf("codevaldwork: handleTaskFailureClassified: GetTask %s: %v", p.TaskID, err)
		return
	}

	if p.FailureType == "transient" {
		// Grant one extra retry beyond the budget.
		agentEntityID, _ := d.resolveAgentForTask(ctx, task.ID, task.AssignedTo)
		roleName := ""
		if agentEntityID != "" {
			if ag, err := d.mgr.GetAgent(ctx, d.agencyID, agentEntityID); err == nil {
				roleName = ag.RoleName
			}
		}
		eventbus.SafePublish(ctx, d.pub, eventbus.Event{
			Topic:    codevaldwork.TopicTaskAssigned,
			AgencyID: d.agencyID,
			Payload: codevaldwork.TaskAssignedPayload{
				TaskID:        task.ID,
				AgentID:       task.AssignedTo,
				RoleName:      roleName,
				TaskCode:      task.TaskName,
				Title:         task.Title,
				Description:   task.Description,
				WorkflowRunID: task.WorkflowRunID,
			},
		})
		log.Printf("codevaldwork: handleTaskFailureClassified: transient — retrying task=%s", task.ID)
		return
	}

	// requires-human (or unknown type — default safe): escalate.
	d.EscalateToHumanDirection(ctx, task, p.TaskID)
}

// EscalateToHumanDirection transitions a task to awaiting-direction, emits
// work.task.needs-direction (carrying a default direction form), and pauses
// the associated WorkflowRun. Called when AI classification returns
// "requires-human" or times out.
func (d *TaskEventDispatcher) EscalateToHumanDirection(ctx context.Context, task codevaldwork.Task, taskID string) {
	task.Status = codevaldwork.TaskStatusAwaitingDirection
	task.UpdatedAt = nowRFC3339()
	if _, err := d.mgr.UpdateTask(ctx, d.agencyID, task); err != nil {
		log.Printf("codevaldwork: EscalateToHumanDirection: UpdateTask %s: %v", taskID, err)
		return
	}
	log.Printf("codevaldwork: EscalateToHumanDirection: task=%s → awaiting-direction", taskID)

	// Find the most-recently-failed todo for context in the direction form.
	var failedTodoTitle, lastFailureReason string
	edges, _ := d.mgr.TraverseRelationships(ctx, d.agencyID, taskID, codevaldwork.RelLabelHasTodo, codevaldwork.DirectionOutbound)
	for _, edge := range edges {
		if todo, err := d.mgr.GetTaskTodo(ctx, d.agencyID, edge.ToID); err == nil && todo.Status == codevaldwork.TodoStatusFailed {
			failedTodoTitle = todo.Title
		}
	}

	form := codevaldwork.DirectionForm{
		Title:         "Task needs your direction",
		Description:   "The task has exhausted its automatic retry budget.",
		FailureOutput: lastFailureReason,
		Question:      "How should the agent proceed?",
		Options: []codevaldwork.FormOption{
			{
				ID:               string(codevaldwork.DirectionRetry),
				Label:            "Retry with new instructions",
				Description:      "Tell the agent what to do differently.",
				RequiresInput:    true,
				InputLabel:       "Instructions",
				InputPlaceholder: "e.g. The compile step is a stub — skip it and mark as completed.",
			},
			{
				ID:            string(codevaldwork.DirectionSkip),
				Label:         "Skip this step",
				Description:   "Mark the failed todo as skipped and continue.",
				RequiresInput: false,
			},
			{
				ID:               string(codevaldwork.DirectionMarkBlocked),
				Label:            "Mark task as blocked",
				Description:      "Pause indefinitely — note what is blocking.",
				RequiresInput:    true,
				InputLabel:       "Blocker",
				InputPlaceholder: "e.g. Flutter toolchain not installed in CI.",
			},
			{
				ID:            string(codevaldwork.DirectionCancel),
				Label:         "Cancel task",
				Description:   "Terminate this task and fail the workflow run cleanly.",
				RequiresInput: false,
			},
		},
		AllowFreetext: true,
		FreetextLabel: "Other — write your own direction",
	}
	_ = failedTodoTitle // available for richer form descriptions when needed

	eventbus.SafePublish(ctx, d.pub, eventbus.Event{
		Topic:    codevaldwork.TopicTaskNeedsDirection,
		AgencyID: d.agencyID,
		Payload: codevaldwork.TaskNeedsDirectionPayload{
			TaskID:            taskID,
			WorkflowRunID:     task.WorkflowRunID,
			AgencyID:          d.agencyID,
			FailureCount:      task.RecoveryRunsUsed,
			LastFailureReason: lastFailureReason,
			Form:              form,
		},
	})

	// Pause the WorkflowRun if one is associated.
	if task.WorkflowRunID != "" {
		run, err := d.mgr.GetWorkflowRun(ctx, d.agencyID, task.WorkflowRunID)
		if err != nil {
			log.Printf("codevaldwork: EscalateToHumanDirection: GetWorkflowRun %s: %v", task.WorkflowRunID, err)
			return
		}
		if run.Status.CanTransitionTo(codevaldwork.WorkflowRunStatusPaused) {
			if _, err := d.mgr.UpdateWorkflowRunStatus(ctx, d.agencyID, task.WorkflowRunID, codevaldwork.WorkflowRunStatusPaused, ""); err != nil {
				log.Printf("codevaldwork: EscalateToHumanDirection: pause run %s: %v", task.WorkflowRunID, err)
			} else {
				eventbus.SafePublish(ctx, d.pub, eventbus.Event{
					Topic:    codevaldwork.TopicRunPaused,
					AgencyID: d.agencyID,
					Payload: codevaldwork.RunPausedPayload{
						WorkflowRunID:  task.WorkflowRunID,
						PausedByTaskID: taskID,
						FailureCount:   task.RecoveryRunsUsed,
					},
				})
				log.Printf("codevaldwork: EscalateToHumanDirection: run=%s → paused", task.WorkflowRunID)
			}
		}
	}
}

// nowRFC3339 returns the current UTC time formatted as RFC3339.
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// latestDecomposition returns only the todos belonging to the most-recent
// AI decomposition for a task. Todos are grouped by DecompRunID; the group
// containing the latest CreatedAt timestamp wins. A retry-with-instructions
// direction creates a fresh decomposition with a new DecompRunID — older
// (failed) decompositions are superseded and must not count toward parent-task
// completion. Todos with an empty DecompRunID are treated as their own group
// keyed on "" so non-AI-decomposed work continues to behave as before.
func latestDecomposition(todos []codevaldwork.TaskTodo) []codevaldwork.TaskTodo {
	if len(todos) == 0 {
		return todos
	}
	type groupInfo struct {
		latestCreatedAt string
		members         []codevaldwork.TaskTodo
	}
	groups := make(map[string]*groupInfo)
	for _, t := range todos {
		g := groups[t.DecompRunID]
		if g == nil {
			g = &groupInfo{}
			groups[t.DecompRunID] = g
		}
		g.members = append(g.members, t)
		if t.CreatedAt > g.latestCreatedAt {
			g.latestCreatedAt = t.CreatedAt
		}
	}
	var winnerKey string
	var winnerCreatedAt string
	for key, g := range groups {
		if g.latestCreatedAt > winnerCreatedAt {
			winnerCreatedAt = g.latestCreatedAt
			winnerKey = key
		}
	}
	return groups[winnerKey].members
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

// updateTodoStatus transitions a TaskTodo when a task.* event arrives with a TodoID.
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

	if status == codevaldwork.TodoStatusCompleted ||
		status == codevaldwork.TodoStatusFailed ||
		status == codevaldwork.TodoStatusSkipped {
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
		log.Printf("codevaldwork: TaskEventDispatcher: published %s todo=%s task=%s status=%s", codevaldwork.TopicTodoCompleted, updated.ID, updated.ParentTaskID, status)
	}

	switch status {
	case codevaldwork.TodoStatusCompleted, codevaldwork.TodoStatusSkipped:
		// skipped is non-failing terminal — unblock dependents same as completed.
		d.unblockDependentTodos(ctx, updated.ParentTaskID, updated.Ordinality)
		d.maybeCompleteParentTask(ctx, updated.ParentTaskID)
	case codevaldwork.TodoStatusFailed:
		// Cascade failure to every blocked todo that (transitively) depended on this one.
		d.blockDependentTodos(ctx, updated.ParentTaskID, updated.Ordinality)
		d.maybeCompleteParentTask(ctx, updated.ParentTaskID)
	}
}

// maybeCompleteParentTask marks a Task completed or failed once all child todos
// are terminal. Terminal states: completed, skipped (non-failing), failed, blocked (cascade-failed).
//
// Only the latest decomposition is considered. A `retry-with-instructions`
// direction creates a fresh decomposition (new DecompRunID) on the same parent
// task; the prior decomposition's FAILED todos must be ignored so the retry can
// succeed. Todos are grouped by DecompRunID and the group with the most-recent
// CreatedAt wins. Todos with no DecompRunID (legacy or non-AI-decomposed) are
// treated as their own group.
func (d *TaskEventDispatcher) maybeCompleteParentTask(ctx context.Context, taskID string) {
	edges, err := d.mgr.TraverseRelationships(ctx, d.agencyID, taskID, codevaldwork.RelLabelHasTodo, codevaldwork.DirectionOutbound)
	if err != nil {
		log.Printf("codevaldwork: maybeCompleteParentTask: TraverseRelationships task=%s: %v", taskID, err)
		return
	}
	allTodos := make([]codevaldwork.TaskTodo, 0, len(edges))
	for _, edge := range edges {
		todo, err := d.mgr.GetTaskTodo(ctx, d.agencyID, edge.ToID)
		if err != nil {
			return
		}
		allTodos = append(allTodos, todo)
	}
	todos := latestDecomposition(allTodos)
	anyFailed := false
	for _, todo := range todos {
		switch todo.Status {
		case codevaldwork.TodoStatusCompleted, codevaldwork.TodoStatusSkipped:
			// skipped is non-failing terminal — treat same as completed
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
		// Only dispatch if every dependency is now completed or skipped.
		allSatisfied := true
		for _, dep := range todo.DependsOn {
			s := ordStatus[dep]
			if s != codevaldwork.TodoStatusCompleted && s != codevaldwork.TodoStatusSkipped {
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

// SkipTodo marks todoID as skipped (non-failing terminal) and runs the
// unblock-and-complete cascade. Dependents of a skipped todo are unblocked
// the same as if it had completed — skipping a step does not cascade-fail.
func (d *TaskEventDispatcher) SkipTodo(ctx context.Context, todoID string) error {
	updated, err := d.mgr.UpdateTaskTodoStatus(ctx, d.agencyID, todoID, codevaldwork.TodoStatusSkipped)
	if err != nil {
		return err
	}
	log.Printf("codevaldwork: SkipTodo: todo %s → skipped", todoID)
	if updated.ParentTaskID != "" {
		eventbus.SafePublish(ctx, d.pub, eventbus.Event{
			Topic:    codevaldwork.TopicTodoCompleted,
			AgencyID: d.agencyID,
			Payload: codevaldwork.TodoCompletedPayload{
				TodoID:        updated.ID,
				ParentTaskID:  updated.ParentTaskID,
				Title:         updated.Title,
				Status:        string(codevaldwork.TodoStatusSkipped),
				WorkflowRunID: updated.WorkflowRunID,
			},
		})
		d.unblockDependentTodos(ctx, updated.ParentTaskID, updated.Ordinality)
		d.maybeCompleteParentTask(ctx, updated.ParentTaskID)
	}
	return nil
}

// handleTaskDirection processes a work.task.direction event.
// Routes by selected_option:
//   - retry-with-instructions: append instructions to task description, move to in_progress, re-dispatch
//   - skip: mark the most-recently-failed todo as skipped, resume cascade
//   - mark-blocked: transition task to blocked, store blocker note, run stays paused
//   - cancel: transition task to cancelled, maybeCompleteWorkflowRun evaluates to failed
func (d *TaskEventDispatcher) handleTaskDirection(ctx context.Context, payloadStr string) {
	var p codevaldwork.TaskDirectionPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.TaskID == "" {
		log.Printf("codevaldwork: handleTaskDirection: bad payload: %v", err)
		return
	}
	log.Printf("codevaldwork: handleTaskDirection: task=%s option=%s directed_by=%s", p.TaskID, p.SelectedOption, p.DirectedBy)

	task, err := d.mgr.GetTask(ctx, d.agencyID, p.TaskID)
	if err != nil {
		log.Printf("codevaldwork: handleTaskDirection: GetTask %s: %v", p.TaskID, err)
		return
	}

	// Append to direction_history before routing.
	task.DirectionHistory = appendDirectionHistory(task.DirectionHistory, string(p.SelectedOption))

	switch p.SelectedOption {
	case codevaldwork.DirectionRetry:
		// Append human/AI instructions to the task description so the next
		// agent run has the extra context, then re-dispatch.
		if p.Instructions != "" {
			task.Description = task.Description + "\n\nDirection: " + p.Instructions
		}
		task.Status = codevaldwork.TaskStatusInProgress
		if _, err := d.mgr.UpdateTask(ctx, d.agencyID, task); err != nil {
			log.Printf("codevaldwork: handleTaskDirection: retry: UpdateTask %s: %v", p.TaskID, err)
			return
		}
		// Resolve agent role name so the re-emitted work.task.assigned carries
		// RoleName — required by developer-assigned-handler payload_condition.
		agentEntityID, _ := d.resolveAgentForTask(ctx, task.ID, task.AssignedTo)
		roleName := ""
		if agentEntityID != "" {
			if ag, err := d.mgr.GetAgent(ctx, d.agencyID, agentEntityID); err == nil {
				roleName = ag.RoleName
			}
		}
		// Re-publish work.task.assigned so the developer-assigned-handler fires again.
		eventbus.SafePublish(ctx, d.pub, eventbus.Event{
			Topic:    codevaldwork.TopicTaskAssigned,
			AgencyID: d.agencyID,
			Payload: codevaldwork.TaskAssignedPayload{
				TaskID:        task.ID,
				AgentID:       task.AssignedTo,
				RoleName:      roleName,
				TaskCode:      task.TaskName,
				Title:         task.Title,
				Description:   task.Description,
				WorkflowRunID: task.WorkflowRunID,
			},
		})
		log.Printf("codevaldwork: handleTaskDirection: retry: re-dispatched task=%s role=%s", p.TaskID, roleName)

	case codevaldwork.DirectionSkip:
		// Find the most-recently-failed todo for this task and skip it.
		edges, err := d.mgr.TraverseRelationships(ctx, d.agencyID, p.TaskID, codevaldwork.RelLabelHasTodo, codevaldwork.DirectionOutbound)
		if err != nil {
			log.Printf("codevaldwork: handleTaskDirection: skip: TraverseRelationships %s: %v", p.TaskID, err)
			return
		}
		for _, edge := range edges {
			todo, err := d.mgr.GetTaskTodo(ctx, d.agencyID, edge.ToID)
			if err != nil {
				continue
			}
			if todo.Status == codevaldwork.TodoStatusFailed {
				if err := d.SkipTodo(ctx, todo.ID); err != nil {
					log.Printf("codevaldwork: handleTaskDirection: SkipTodo %s: %v", todo.ID, err)
				}
				// Move task back to in_progress so maybeCompleteParentTask can
				// evaluate whether it's done.
				task.Status = codevaldwork.TaskStatusInProgress
				if _, err := d.mgr.UpdateTask(ctx, d.agencyID, task); err != nil {
					log.Printf("codevaldwork: handleTaskDirection: skip: UpdateTask %s: %v", p.TaskID, err)
				}
				return
			}
		}
		log.Printf("codevaldwork: handleTaskDirection: skip: no failed todo found for task=%s", p.TaskID)

	case codevaldwork.DirectionMarkBlocked:
		task.BlockerNote = p.Instructions
		task.Status = codevaldwork.TaskStatusBlocked
		if _, err := d.mgr.UpdateTask(ctx, d.agencyID, task); err != nil {
			log.Printf("codevaldwork: handleTaskDirection: mark-blocked: UpdateTask %s: %v", p.TaskID, err)
		}
		log.Printf("codevaldwork: handleTaskDirection: mark-blocked: task=%s", p.TaskID)
		// Run stays paused — no resume check needed.
		return

	case codevaldwork.DirectionCancel:
		task.Status = codevaldwork.TaskStatusCancelled
		if _, err := d.mgr.UpdateTask(ctx, d.agencyID, task); err != nil {
			log.Printf("codevaldwork: handleTaskDirection: cancel: UpdateTask %s: %v", p.TaskID, err)
			return
		}
		eventbus.SafePublish(ctx, d.pub, eventbus.Event{
			Topic:    codevaldwork.TopicTaskCancelled,
			AgencyID: d.agencyID,
			Payload: codevaldwork.TaskCancelledPayload{
				TaskID:        task.ID,
				WorkflowRunID: task.WorkflowRunID,
			},
		})
		log.Printf("codevaldwork: handleTaskDirection: cancel: task=%s", p.TaskID)
		if task.ParentTaskID != "" {
			d.maybeCompleteSplitParent(ctx, task.ParentTaskID)
		}
		// Fall through to run-resume check — cancellation may complete the run.

	default:
		log.Printf("codevaldwork: handleTaskDirection: unknown option %q for task=%s", p.SelectedOption, p.TaskID)
		return
	}

	// After retry or cancel direction: check whether the WorkflowRun can resume.
	if task.WorkflowRunID != "" {
		d.maybeResumeWorkflowRun(ctx, task.WorkflowRunID, p.TaskID, string(p.SelectedOption))
	}
}

// maybeResumeWorkflowRun transitions a paused WorkflowRun back to in_progress
// when no tasks in the run remain in awaiting-direction or blocked status.
// Called after a direction event resolves a task.
func (d *TaskEventDispatcher) maybeResumeWorkflowRun(ctx context.Context, runID, resolvedTaskID, selectedOption string) {
	run, err := d.mgr.GetWorkflowRun(ctx, d.agencyID, runID)
	if err != nil || run.Status != codevaldwork.WorkflowRunStatusPaused {
		return
	}
	closure, err := d.mgr.GetWorkflowRunClosure(ctx, d.agencyID, runID)
	if err != nil {
		log.Printf("codevaldwork: maybeResumeWorkflowRun: GetWorkflowRunClosure run=%s: %v", runID, err)
		return
	}
	for _, task := range closure.Tasks {
		if task.Status == codevaldwork.TaskStatusAwaitingDirection {
			return // at least one task still waiting — stay paused
		}
	}
	// All tasks resolved — transition run based on the direction taken.
	nextStatus := codevaldwork.WorkflowRunStatusInProgress
	reason := ""
	if selectedOption == string(codevaldwork.DirectionCancel) {
		nextStatus = codevaldwork.WorkflowRunStatusFailed
		reason = "task cancelled by direction"
	}
	if !run.Status.CanTransitionTo(nextStatus) {
		return
	}
	if _, err := d.mgr.UpdateWorkflowRunStatus(ctx, d.agencyID, runID, nextStatus, reason); err != nil {
		log.Printf("codevaldwork: maybeResumeWorkflowRun: UpdateWorkflowRunStatus run=%s → %s: %v", runID, nextStatus, err)
		return
	}
	if nextStatus == codevaldwork.WorkflowRunStatusInProgress {
		eventbus.SafePublish(ctx, d.pub, eventbus.Event{
			Topic:    codevaldwork.TopicRunResumed,
			AgencyID: d.agencyID,
			Payload: codevaldwork.RunResumedPayload{
				WorkflowRunID:   runID,
				ResumedByTaskID: resolvedTaskID,
				SelectedOption:  selectedOption,
			},
		})
		log.Printf("codevaldwork: maybeResumeWorkflowRun: run=%s → in_progress (resumed)", runID)
	} else {
		log.Printf("codevaldwork: maybeResumeWorkflowRun: run=%s → %s", runID, nextStatus)
	}
}

// appendDirectionHistory appends selectedOption to the JSON-encoded direction
// history string. If history is empty or invalid JSON, starts fresh.
func appendDirectionHistory(history, selectedOption string) string {
	var past []string
	if history != "" {
		_ = json.Unmarshal([]byte(history), &past)
	}
	past = append(past, selectedOption)
	b, _ := json.Marshal(past)
	return string(b)
}

// handleAITodoCreated consumes a todo.created decomposition payload:
// creates one TaskTodo entity per item, wires graph edges, then dispatches
// only root todos (those with empty depends_on). Todos with dependencies start
// as blocked and are dispatched by unblockDependentTodos once their
// predecessors complete.
func (d *TaskEventDispatcher) handleAITodoCreated(ctx context.Context, payloadStr string) {
	var p aiTodoCreatedPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.ParentTaskID == "" {
		log.Printf("codevaldwork: TaskEventDispatcher: todo.created: bad payload: %v", err)
		return
	}
	log.Printf("codevaldwork: TaskEventDispatcher: todo.created: parent=%s items=%d", p.ParentTaskID, len(p.Todos))

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
