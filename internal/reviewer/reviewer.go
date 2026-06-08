// Package reviewer evaluates AcceptanceCriteria for completed tasks and emits
// work.review.passed or work.review.failed (FEAT-20260605-003).
package reviewer

import (
	"context"
	"log"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

// CriteriaEvaluator evaluates a single AcceptanceCriteria against the
// deliverables produced by its owning task.
//
// The production implementation publishes an evaluation request via Cross and
// awaits the AI response; the test implementation is an in-memory mock.
type CriteriaEvaluator interface {
	// Evaluate returns the result ("passed" | "failed" | "skipped" | "blocked")
	// and an explanatory note. A non-nil error means the evaluation could not
	// be attempted at all; the caller writes "blocked" in that case.
	Evaluate(ctx context.Context, criteria codevaldwork.AcceptanceCriteria, deliverables []codevaldwork.Deliverable) (result, notes string, err error)
}

// ReviewManager is the subset of codevaldwork.TaskManager the reviewer needs.
type ReviewManager interface {
	ListDeliverablesForTask(ctx context.Context, agencyID, taskID string) ([]codevaldwork.Deliverable, error)
	ListAcceptanceCriteriaForTask(ctx context.Context, agencyID, taskID string) ([]codevaldwork.AcceptanceCriteria, error)
	WriteAcceptanceCriteriaResult(ctx context.Context, agencyID, criteriaID, result, notes string) error
}

// passThroughEvaluator is the no-LLM default: marks every criterion "skipped"
// so the pipeline does not stall when no evaluator is wired up.
type passThroughEvaluator struct{}

// NewPassThroughEvaluator returns a CriteriaEvaluator that marks every
// criterion as "skipped" (no LLM available). Used in production when the
// review gate is enabled but no AI evaluator has been configured yet.
func NewPassThroughEvaluator() CriteriaEvaluator { return passThroughEvaluator{} }

func (passThroughEvaluator) Evaluate(_ context.Context, _ codevaldwork.AcceptanceCriteria, _ []codevaldwork.Deliverable) (string, string, error) {
	return "skipped", "no evaluator configured", nil
}

// Reviewer handles the review gate for completed tasks.
type Reviewer struct {
	mgr       ReviewManager
	pub       eventbus.Publisher
	evaluator CriteriaEvaluator
	agencyID  string
}

// New constructs a Reviewer. pub may be nil in tests (events are skipped).
func New(mgr ReviewManager, pub eventbus.Publisher, evaluator CriteriaEvaluator, agencyID string) *Reviewer {
	return &Reviewer{mgr: mgr, pub: pub, evaluator: evaluator, agencyID: agencyID}
}

// Review fetches all AcceptanceCriteria and Deliverables for taskID, evaluates
// each criterion, writes the results back, and publishes the review outcome.
//
// If there are no AcceptanceCriteria the review vacuously passes.
func (r *Reviewer) Review(ctx context.Context, taskID, workflowRunID string) {
	criteria, err := r.mgr.ListAcceptanceCriteriaForTask(ctx, r.agencyID, taskID)
	if err != nil {
		log.Printf("reviewer: ListAcceptanceCriteriaForTask task=%s: %v", taskID, err)
		return
	}

	// Vacuous pass — no criteria means nothing to fail.
	if len(criteria) == 0 {
		r.publishOutcome(ctx, taskID, workflowRunID, nil)
		return
	}

	deliverables, err := r.mgr.ListDeliverablesForTask(ctx, r.agencyID, taskID)
	if err != nil {
		log.Printf("reviewer: ListDeliverablesForTask task=%s: %v", taskID, err)
		return
	}

	var failed []codevaldwork.FailedCriterionSummary

	for _, c := range criteria {
		result, notes, evalErr := r.evaluator.Evaluate(ctx, c, deliverables)
		if evalErr != nil {
			log.Printf("reviewer: Evaluate criteria=%s: %v — marking blocked", c.ID, evalErr)
			result = "blocked"
			notes = evalErr.Error()
		}

		if err := r.mgr.WriteAcceptanceCriteriaResult(ctx, r.agencyID, c.ID, result, notes); err != nil {
			log.Printf("reviewer: WriteAcceptanceCriteriaResult criteria=%s: %v", c.ID, err)
		}

		if result != "passed" && result != "skipped" {
			failed = append(failed, codevaldwork.FailedCriterionSummary{
				CriterionID: c.ID,
				Title:       c.Title,
				Result:      result,
				ResultNotes: notes,
			})
		}
	}

	r.publishOutcome(ctx, taskID, workflowRunID, failed)
}

func (r *Reviewer) publishOutcome(ctx context.Context, taskID, workflowRunID string, failed []codevaldwork.FailedCriterionSummary) {
	if r.pub == nil {
		return
	}

	topic := codevaldwork.TopicReviewPassed
	if len(failed) > 0 {
		topic = codevaldwork.TopicReviewFailed
	}

	eventbus.SafePublish(ctx, r.pub, eventbus.Event{
		Topic:    topic,
		AgencyID: r.agencyID,
		Payload: codevaldwork.ReviewOutcomePayload{
			TaskID:         taskID,
			WorkflowRunID:  workflowRunID,
			AgencyID:       r.agencyID,
			FailedCriteria: failed,
		},
	})
}
