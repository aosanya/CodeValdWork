// Package server — WorkflowRun status transition handler (FEAT-20260602-003).
//
// RunStatusHandler consumes inbound events that carry workflow_run_id and
// drives WorkflowRun lifecycle transitions:
//
//	work.task.assigned          → pending      → in_progress
//	work.task.failed            → in_progress  → failed
//	functions.job.failed        → in_progress  → failed
//	ai.run.failed               → in_progress  → failed
//	git.merge.failed            → in_progress  → failed
//	<terminal_event condition>  → in_progress  → completed
//
// Wired into [TaskEventDispatcher.Dispatch] so every ACKed event is checked.
package server

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

// failureTopics is the closed set of event topics that force in_progress → failed.
var failureTopics = map[string]bool{
	codevaldwork.TopicTaskFailed: true,
	"functions.job.failed":       true,
	"ai.run.failed":              true,
	"git.merge.failed":           true,
}

// RunStatusHandler drives WorkflowRun status transitions from inbound events.
type RunStatusHandler struct {
	mgr      codevaldwork.TaskManager
	agencyID string
}

// NewRunStatusHandler constructs a RunStatusHandler.
func NewRunStatusHandler(mgr codevaldwork.TaskManager, agencyID string) *RunStatusHandler {
	return &RunStatusHandler{mgr: mgr, agencyID: agencyID}
}

// HandleEvent processes a single inbound event for WorkflowRun transitions.
// Ignores events that carry no workflow_run_id.
func (h *RunStatusHandler) HandleEvent(ctx context.Context, topic, payloadStr string) {
	runID, payloadMap := extractRunContext(payloadStr)
	if runID == "" {
		return
	}

	run, err := h.mgr.GetWorkflowRun(ctx, h.agencyID, runID)
	if err != nil {
		// Run not found — ignore; the event may belong to a different agency or
		// a run that has already been cleaned up.
		return
	}

	var nextStatus codevaldwork.WorkflowRunStatus
	var reason string

	switch {
	case run.Status == codevaldwork.WorkflowRunStatusPending &&
		topic == codevaldwork.TopicTaskAssigned:
		nextStatus = codevaldwork.WorkflowRunStatusInProgress

	case run.Status == codevaldwork.WorkflowRunStatusInProgress && failureTopics[topic]:
		nextStatus = codevaldwork.WorkflowRunStatusFailed
		reason = extractReason(payloadMap)

	case run.Status == codevaldwork.WorkflowRunStatusInProgress &&
		run.TerminalEvent != "" &&
		matchesTerminalEvent(run.TerminalEvent, topic, payloadMap):
		nextStatus = codevaldwork.WorkflowRunStatusCompleted

	case run.Status == codevaldwork.WorkflowRunStatusInProgress &&
		topic == codevaldwork.TopicTaskCompleted:
		// All tasks in the run have reached a terminal state → close the run.
		nextStatus, reason = h.allTasksTerminalStatus(ctx, runID)
		if nextStatus == "" {
			return // non-terminal tasks still pending
		}

	default:
		return // no applicable transition
	}

	updated, err := h.mgr.UpdateWorkflowRunStatus(ctx, h.agencyID, runID, nextStatus, reason)
	if err != nil {
		log.Printf("codevaldwork: RunStatusHandler: UpdateWorkflowRunStatus run=%s → %s: %v", runID, nextStatus, err)
		return
	}
	log.Printf("codevaldwork: RunStatusHandler: run=%s %s → %s", runID, run.Status, updated.Status)
}

// extractRunContext parses the JSON payload string and returns the
// workflow_run_id value plus the full decoded map for field matching.
// Returns ("", nil) when the payload is not valid JSON or carries no run ID.
func extractRunContext(payloadStr string) (runID string, m map[string]any) {
	if payloadStr == "" {
		return "", nil
	}
	if err := json.Unmarshal([]byte(payloadStr), &m); err != nil {
		return "", nil
	}
	if s, ok := m["workflow_run_id"].(string); ok {
		runID = s
	}
	if runID == "" {
		// Camel-case fallback used by some legacy event shapes.
		runID, _ = m["WorkflowRunID"].(string)
	}
	return runID, m
}

// extractReason pulls a human-readable failure description from a payload map.
// Checks "reason", "Reason", "error", and "message" keys, in that order.
func extractReason(m map[string]any) string {
	for _, key := range []string{"reason", "Reason", "error", "message"} {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// allTasksTerminalStatus checks every Task in the run's closure. If all are
// terminal it returns the appropriate target WorkflowRun status and a reason
// string. It returns ("", "") when at least one task is still active.
func (h *RunStatusHandler) allTasksTerminalStatus(ctx context.Context, runID string) (codevaldwork.WorkflowRunStatus, string) {
	closure, err := h.mgr.GetWorkflowRunClosure(ctx, h.agencyID, runID)
	if err != nil {
		log.Printf("codevaldwork: RunStatusHandler: GetWorkflowRunClosure run=%s: %v", runID, err)
		return "", ""
	}
	if len(closure.Tasks) == 0 {
		return "", "" // no tasks linked yet — don't close prematurely
	}
	anyFailed := false
	for _, task := range closure.Tasks {
		switch task.Status {
		case codevaldwork.TaskStatusCompleted, codevaldwork.TaskStatusCancelled:
			// terminal — fine
		case codevaldwork.TaskStatusFailed:
			anyFailed = true
		default:
			return "", "" // still active
		}
	}
	if anyFailed {
		return codevaldwork.WorkflowRunStatusFailed, "one or more tasks failed"
	}
	return codevaldwork.WorkflowRunStatusCompleted, ""
}

// matchesTerminalEvent reports whether topic + payloadMap satisfy the
// terminal_event condition stored on the run.
//
// Format: "topic:field=value:field=value..."
// Example: "functions.job.completed:function_name=merge-flutter-branch:status=ok"
//
// The topic segment must match exactly. Each "field=value" qualifier must
// appear in the payload map as an exact string match. Extra payload fields
// are ignored.
func matchesTerminalEvent(terminalEvent, topic string, payloadMap map[string]any) bool {
	if terminalEvent == "" {
		return false
	}
	parts := strings.SplitN(terminalEvent, ":", 2)
	if parts[0] != topic {
		return false
	}
	if len(parts) == 1 {
		return true // topic-only condition
	}
	for _, qualifier := range strings.Split(parts[1], ":") {
		kv := strings.SplitN(qualifier, "=", 2)
		if len(kv) != 2 {
			continue
		}
		field, want := kv[0], kv[1]
		got, _ := payloadMap[field].(string)
		if got != want {
			return false
		}
	}
	return true
}
