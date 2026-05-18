package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

const (
	topicTaskInProgress = "ai.task.in_progress"
	topicTaskCompleted  = "ai.task.completed"
	topicTaskFailed     = "ai.task.failed"
	topicTaskTodo       = "ai.task.todo"
)

// aiTaskPayload is the common shape of ai.task.in_progress/completed/failed payloads.
type aiTaskPayload struct {
	TaskID string `json:"TaskID"`
}

// aiTaskTodoPayload mirrors the CodeValdAI TaskTodoPayload — the ai.task.todo event body.
type aiTaskTodoPayload struct {
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
}

// TaskEventDispatcher bridges ai.task.* events into work task / todo status transitions
// and materialises ai.task.todo decompositions into TaskTodo entities.
type TaskEventDispatcher struct {
	mgr      codevaldwork.TaskManager
	agencyID string
}

// NewTaskEventDispatcher constructs a TaskEventDispatcher.
func NewTaskEventDispatcher(mgr codevaldwork.TaskManager, agencyID string) *TaskEventDispatcher {
	return &TaskEventDispatcher{mgr: mgr, agencyID: agencyID}
}

// Dispatch handles an incoming event by topic.
func (d *TaskEventDispatcher) Dispatch(ctx context.Context, topic, payload string) {
	switch topic {
	case topicTaskTodo:
		d.handleAITaskTodo(ctx, payload)
		return
	case topicTaskInProgress, topicTaskCompleted, topicTaskFailed:
		d.handleAITaskStatus(ctx, topic, payload)
	}
}

// handleAITaskStatus routes ai.task.in_progress/completed/failed to a Task or
// TaskTodo status update depending on which entity the TaskID refers to.
func (d *TaskEventDispatcher) handleAITaskStatus(ctx context.Context, topic, payloadStr string) {
	var p aiTaskPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.TaskID == "" {
		log.Printf("codevaldwork: TaskEventDispatcher: bad payload for topic=%s: %v", topic, err)
		return
	}

	var nextStatus codevaldwork.TaskStatus
	switch topic {
	case topicTaskInProgress:
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

	task.Status = nextStatus
	if _, err := d.mgr.UpdateTask(ctx, d.agencyID, task); err != nil {
		log.Printf("codevaldwork: TaskEventDispatcher: UpdateTask %s → %s: %v", p.TaskID, nextStatus, err)
		return
	}
	log.Printf("codevaldwork: TaskEventDispatcher: task %s → %s", p.TaskID, nextStatus)
}

// updateTodoStatus transitions a TaskTodo when an ai.task.* event arrives with a TodoID.
func (d *TaskEventDispatcher) updateTodoStatus(ctx context.Context, todoID, topic string) {
	var status codevaldwork.TodoStatus
	switch topic {
	case topicTaskInProgress:
		status = codevaldwork.TodoStatusDispatched
	case topicTaskCompleted:
		status = codevaldwork.TodoStatusCompleted
	case topicTaskFailed:
		status = codevaldwork.TodoStatusFailed
	default:
		return
	}
	if _, err := d.mgr.UpdateTaskTodoStatus(ctx, d.agencyID, todoID, status); err != nil {
		log.Printf("codevaldwork: TaskEventDispatcher: UpdateTaskTodoStatus %s → %s: %v", todoID, status, err)
		return
	}
	log.Printf("codevaldwork: TaskEventDispatcher: todo %s → %s", todoID, status)
}

// handleAITaskTodo consumes an ai.task.todo decomposition payload:
// creates one TaskTodo entity per item, wires the graph edges, and
// publishes work.task.todo (via CreateTaskTodo) so CodeValdAI agents can
// pick up each todo through their work plans.
func (d *TaskEventDispatcher) handleAITaskTodo(ctx context.Context, payloadStr string) {
	var p aiTaskTodoPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil || p.ParentTaskID == "" {
		log.Printf("codevaldwork: TaskEventDispatcher: ai.task.todo: bad payload: %v", err)
		return
	}
	log.Printf("codevaldwork: TaskEventDispatcher: ai.task.todo: parent=%s items=%d", p.ParentTaskID, len(p.Todos))

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
