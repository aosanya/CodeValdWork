package codevaldwork

// FormOption is one selectable recovery action presented to the direction
// resolver (human or AI). It is embedded in [DirectionForm] and serialised
// into the work.task.needs-direction event payload so any platform can render
// the form from JSON without additional metadata.
type FormOption struct {
	// ID is the machine identifier sent back in work.task.direction.selected_option.
	// Values: "retry-with-instructions", "skip", "mark-blocked", "cancel".
	ID string `json:"id"`

	// Label is the short display text.
	Label string `json:"label"`

	// Description explains what choosing this option will do.
	Description string `json:"description"`

	// RequiresInput indicates whether a free-text input field must be shown.
	RequiresInput bool `json:"requires_input"`

	// InputLabel is the label for the input field when RequiresInput is true.
	InputLabel string `json:"input_label,omitempty"`

	// InputPlaceholder is the hint text for the input field.
	InputPlaceholder string `json:"input_placeholder,omitempty"`

	// Suggestions are AI-generated quick-fill strings the resolver can accept
	// without typing. Only meaningful when RequiresInput is true.
	Suggestions []string `json:"suggestions,omitempty"`
}

// DirectionForm is the JSON-serialisable form description carried by the
// work.task.needs-direction event. Any frontend (web, mobile, CLI, notification
// widget) can render the form solely from this payload — no CodeVald-specific
// knowledge required.
//
// The AI failure reviewer (failure-direction-handler work plan) also reads this
// struct when deciding which option to select autonomously.
type DirectionForm struct {
	// Title is the modal/card heading shown to the resolver.
	Title string `json:"title"`

	// Description provides context about why direction is needed.
	Description string `json:"description"`

	// FailureOutput is the raw error message or reason from the last failure.
	// Always shown so the resolver can make an informed choice.
	FailureOutput string `json:"failure_output"`

	// Question is the prompt presented above the option list.
	Question string `json:"question"`

	// Options are the selectable recovery actions. Populated by the AI
	// classification step; not hardcoded — the set may vary per failure type.
	Options []FormOption `json:"options"`

	// AllowFreetext enables a free-text override input below the options list.
	// Always true — resolvers can always type a custom instruction.
	AllowFreetext bool `json:"allow_freetext"`

	// FreetextLabel is the label for the free-text override field.
	FreetextLabel string `json:"freetext_label,omitempty"`
}

// TaskNeedsDirectionPayload is the Event.Payload for [TopicTaskNeedsDirection].
type TaskNeedsDirectionPayload struct {
	TaskID            string        `json:"task_id"`
	WorkflowRunID     string        `json:"workflow_run_id,omitempty"`
	AgencyID          string        `json:"agency_id"`
	FailureCount      int           `json:"failure_count"`
	LastFailureReason string        `json:"last_failure_reason"`
	Form              DirectionForm `json:"form"`
}

// TaskDirectionPayload is the Event.Payload for [TopicTaskDirection].
// Emitted by CodeValdAI (failure-direction-handler) or a human via the frontend.
type TaskDirectionPayload struct {
	TaskID         string              `json:"task_id"`
	WorkflowRunID  string              `json:"workflow_run_id,omitempty"`
	SelectedOption TaskDirectionOption `json:"selected_option"`
	// Instructions holds corrective guidance — required for DirectionRetry and
	// DirectionMarkBlocked; empty string for DirectionSkip and DirectionCancel.
	Instructions string `json:"instructions,omitempty"`
	// DirectedBy is the agent ID or human user identifier that submitted this direction.
	DirectedBy string `json:"directed_by,omitempty"`
}

// RunPausedPayload is the Event.Payload for [TopicRunPaused].
type RunPausedPayload struct {
	WorkflowRunID     string `json:"workflow_run_id"`
	PausedByTaskID    string `json:"paused_by_task_id"`
	FailureCount      int    `json:"failure_count"`
	LastFailureReason string `json:"last_failure_reason"`
}

// RunResumedPayload is the Event.Payload for [TopicRunResumed].
type RunResumedPayload struct {
	WorkflowRunID    string `json:"workflow_run_id"`
	ResumedByTaskID  string `json:"resumed_by_task_id"`
	SelectedOption   string `json:"selected_option"`
}

// TaskDirectionOption enumerates the valid selected_option values in a
// work.task.direction payload. Using typed constants prevents typos in
// the direction handler routing switch.
type TaskDirectionOption string

const (
	// DirectionRetry re-dispatches the task with Instructions appended to
	// its description. The AI agent will re-plan with the extra context.
	DirectionRetry TaskDirectionOption = "retry-with-instructions"

	// DirectionSkip marks the specific failed TaskTodo as skipped (non-failing
	// terminal) and re-evaluates parent task completion.
	DirectionSkip TaskDirectionOption = "skip"

	// DirectionMarkBlocked sets the task to blocked status with a blocker note.
	// The WorkflowRun stays paused until an unblock event arrives.
	DirectionMarkBlocked TaskDirectionOption = "mark-blocked"

	// DirectionCancel terminates the task and fails the WorkflowRun.
	DirectionCancel TaskDirectionOption = "cancel"
)
