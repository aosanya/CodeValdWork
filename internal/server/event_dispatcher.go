package server

import (
	"context"
	"encoding/json"
	"log"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

const (
	topicTaskInProgress = "ai.task.in_progress"
	topicTaskCompleted  = "ai.task.completed"
	topicTaskFailed     = "ai.task.failed"
)

// aiTaskPayload is the common shape of ai.task.* event payloads.
type aiTaskPayload struct {
	TaskID string `json:"TaskID"`
}

// TaskEventDispatcher bridges ai.task.* events into work task status transitions.
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
	var nextStatus codevaldwork.TaskStatus
	switch topic {
	case topicTaskInProgress:
		nextStatus = codevaldwork.TaskStatusInProgress
	case topicTaskCompleted:
		nextStatus = codevaldwork.TaskStatusCompleted
	case topicTaskFailed:
		nextStatus = codevaldwork.TaskStatusFailed
	default:
		return
	}

	var p aiTaskPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil || p.TaskID == "" {
		log.Printf("codevaldwork: TaskEventDispatcher: bad payload for topic=%s: %v", topic, err)
		return
	}

	task, err := d.mgr.GetTask(ctx, d.agencyID, p.TaskID)
	if err != nil {
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
