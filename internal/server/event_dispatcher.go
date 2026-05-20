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
)

// aiTaskPayload is the common shape of ai.task.started/completed/failed payloads.
type aiTaskPayload struct {
	TaskID      string `json:"TaskID"`
	RunID       string `json:"RunID"`
	Reason      string `json:"Reason"`
	HasSubtasks bool   `json:"has_subtasks,omitempty"`
}

// aiTodoCreatedPayload mirrors the CodeValdAI TodoCreatedPayload — the ai.todo.created event body.
type aiTodoCreatedPayload struct {
	ParentTaskID string        `json:"parent_task_id"`
	RunID        string        `json:"run_id"`
	AgentID      string        `json:"agent_id"`
	Todos        []aiTodoItem  `json:"todos"`
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
	mgr      codevaldwork.TaskManager
	agencyID string
	pub      eventbus.Publisher // optional; nil = skip work.task.failed bridging
}

// NewTaskEventDispatcher constructs a TaskEventDispatcher.
// pub may be nil — work.task.failed bridging is skipped when no publisher is set.
func NewTaskEventDispatcher(mgr codevaldwork.TaskManager, agencyID string, pub eventbus.Publisher) *TaskEventDispatcher {
	return &TaskEventDispatcher{mgr: mgr, agencyID: agencyID, pub: pub}
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
	}
}

// handleAITaskStatus routes ai.task.started/completed/failed to a Task or
// TaskTodo status update depending on which entity the TaskID refers to.
func (d *TaskEventDispatcher) handleAITaskStatus(ctx context.Context, topic, payloadStr string) {
	var p aiTaskPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.TaskID == "" {
		log.Printf("codevaldwork: TaskEventDispatcher: bad payload for topic=%s: %v", topic, err)
		return
	}

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
		eventbus.SafePublish(ctx, d.pub, eventbus.Event{
			Topic:    codevaldwork.TopicTaskFailed,
			AgencyID: d.agencyID,
			Payload: codevaldwork.TaskFailedPayload{
				TaskID: p.TaskID,
				RunID:  p.RunID,
				Reason: p.Reason,
			},
		})
		log.Printf("codevaldwork: TaskEventDispatcher: bridged work.task.failed for task=%s run=%s", p.TaskID, p.RunID)
	}
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

	// When a todo reaches a terminal state, check whether the parent task can
	// now be marked completed (all sibling todos done).
	if (status == codevaldwork.TodoStatusCompleted || status == codevaldwork.TodoStatusFailed) &&
		updated.ParentTaskID != "" {
		d.maybeCompleteParentTask(ctx, updated.ParentTaskID)
	}
}

// maybeCompleteParentTask marks a Task as completed when all of its child todos
// have reached a terminal state. Called after each todo terminal transition.
func (d *TaskEventDispatcher) maybeCompleteParentTask(ctx context.Context, taskID string) {
	edges, err := d.mgr.TraverseRelationships(ctx, d.agencyID, taskID, codevaldwork.RelLabelHasTodo, codevaldwork.DirectionOutbound)
	if err != nil {
		log.Printf("codevaldwork: maybeCompleteParentTask: TraverseRelationships task=%s: %v", taskID, err)
		return
	}
	for _, edge := range edges {
		todo, err := d.mgr.GetTaskTodo(ctx, d.agencyID, edge.ToID)
		if err != nil || (todo.Status != codevaldwork.TodoStatusCompleted && todo.Status != codevaldwork.TodoStatusFailed) {
			return // at least one todo is still running
		}
	}

	// All todos terminal — complete the parent task.
	task, err := d.mgr.GetTask(ctx, d.agencyID, taskID)
	if err != nil {
		log.Printf("codevaldwork: maybeCompleteParentTask: GetTask %s: %v", taskID, err)
		return
	}
	if task.Status == codevaldwork.TaskStatusCompleted || task.Status == codevaldwork.TaskStatusFailed {
		return // already in terminal state
	}
	task.Status = codevaldwork.TaskStatusCompleted
	if _, err := d.mgr.UpdateTask(ctx, d.agencyID, task); err != nil {
		log.Printf("codevaldwork: maybeCompleteParentTask: UpdateTask %s: %v", taskID, err)
		return
	}
	log.Printf("codevaldwork: maybeCompleteParentTask: task %s → completed (all todos done)", taskID)
}

// handleAITodoCreated consumes an ai.todo.created decomposition payload:
// creates one TaskTodo entity per item, wires the graph edges, and
// publishes work.todo.dispatched (via CreateTaskTodo) so CodeValdAI agents can
// pick up each todo through their work plans.
func (d *TaskEventDispatcher) handleAITodoCreated(ctx context.Context, payloadStr string) {
	var p aiTodoCreatedPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.ParentTaskID == "" {
		log.Printf("codevaldwork: TaskEventDispatcher: ai.todo.created: bad payload: %v", err)
		return
	}
	log.Printf("codevaldwork: TaskEventDispatcher: ai.todo.created: parent=%s items=%d", p.ParentTaskID, len(p.Todos))

	for _, item := range p.Todos {
		// Determine which agent will execute this todo.
		agentEntityID, externalAgentID := d.resolveAgentForTask(ctx, p.ParentTaskID, p.AgentID)

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
		}

		created, err := d.mgr.CreateTaskTodo(ctx, d.agencyID, todo)
		if err != nil {
			log.Printf("codevaldwork: TaskEventDispatcher: CreateTaskTodo ordinality=%d: %v", item.Ordinality, err)
			continue
		}

		// has_todo edge: Task → TaskTodo
		if _, err := d.mgr.CreateRelationship(ctx, d.agencyID, codevaldwork.Relationship{
			Label:  codevaldwork.RelLabelHasTodo,
			FromID: p.ParentTaskID,
			ToID:   created.ID,
		}); err != nil {
			log.Printf("codevaldwork: TaskEventDispatcher: has_todo edge todo=%s: %v", created.ID, err)
		}

		// todo_assigned_to edge: TaskTodo → Agent
		if agentEntityID != "" {
			if _, err := d.mgr.CreateRelationship(ctx, d.agencyID, codevaldwork.Relationship{
				Label:  codevaldwork.RelLabelTodoAssignedTo,
				FromID: created.ID,
				ToID:   agentEntityID,
			}); err != nil {
				log.Printf("codevaldwork: TaskEventDispatcher: todo_assigned_to edge todo=%s agent=%s: %v",
					created.ID, agentEntityID, err)
			}
		}

		log.Printf("codevaldwork: TaskEventDispatcher: created TaskTodo id=%s ordinality=%d agent=%s",
			created.ID, item.Ordinality, externalAgentID)
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
