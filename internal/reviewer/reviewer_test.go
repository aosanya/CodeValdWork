package reviewer_test

import (
	"context"
	"errors"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdWork/internal/reviewer"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeManager struct {
	deliverables []codevaldwork.Deliverable
	criteria     []codevaldwork.AcceptanceCriteria
	writeResults map[string]struct{ result, notes string }
	listDelErr   error
	listCritErr  error
	writeErr     error
}

func (m *fakeManager) ListDeliverablesForTask(_ context.Context, _, _ string) ([]codevaldwork.Deliverable, error) {
	return m.deliverables, m.listDelErr
}

func (m *fakeManager) ListAcceptanceCriteriaForTask(_ context.Context, _, _ string) ([]codevaldwork.AcceptanceCriteria, error) {
	return m.criteria, m.listCritErr
}

func (m *fakeManager) WriteAcceptanceCriteriaResult(_ context.Context, _, id, result, notes string) error {
	if m.writeResults == nil {
		m.writeResults = map[string]struct{ result, notes string }{}
	}
	m.writeResults[id] = struct{ result, notes string }{result, notes}
	return m.writeErr
}

type fakeEvaluator struct {
	outcomes map[string]struct {
		result string
		notes  string
		err    error
	}
}

func (e *fakeEvaluator) Evaluate(_ context.Context, c codevaldwork.AcceptanceCriteria, _ []codevaldwork.Deliverable) (string, string, error) {
	if o, ok := e.outcomes[c.ID]; ok {
		return o.result, o.notes, o.err
	}
	return "passed", "", nil
}

type fakePublisher struct {
	published []eventbus.Event
}

func (p *fakePublisher) Publish(_ context.Context, ev eventbus.Event) error {
	p.published = append(p.published, ev)
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func makeCriterion(id, title string) codevaldwork.AcceptanceCriteria {
	return codevaldwork.AcceptanceCriteria{ID: id, AgencyID: "a1", Title: title}
}

func makeDeliverable(id string) codevaldwork.Deliverable {
	return codevaldwork.Deliverable{ID: id, AgencyID: "a1", Title: "output"}
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestReviewer_NoCriteria_VacuousPass(t *testing.T) {
	t.Parallel()
	mgr := &fakeManager{}
	pub := &fakePublisher{}
	eval := &fakeEvaluator{}

	rev := reviewer.New(mgr, pub, eval, "a1")
	rev.Review(context.Background(), "task-1", "run-1")

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	if pub.published[0].Topic != codevaldwork.TopicReviewPassed {
		t.Errorf("topic: want %q, got %q", codevaldwork.TopicReviewPassed, pub.published[0].Topic)
	}
}

func TestReviewer_AllCriteriaPassed_EmitsReviewPassed(t *testing.T) {
	t.Parallel()
	mgr := &fakeManager{
		criteria:     []codevaldwork.AcceptanceCriteria{makeCriterion("c1", "C1"), makeCriterion("c2", "C2")},
		deliverables: []codevaldwork.Deliverable{makeDeliverable("d1")},
	}
	pub := &fakePublisher{}
	eval := &fakeEvaluator{outcomes: map[string]struct {
		result string
		notes  string
		err    error
	}{
		"c1": {"passed", "looks good", nil},
		"c2": {"passed", "verified", nil},
	}}

	rev := reviewer.New(mgr, pub, eval, "a1")
	rev.Review(context.Background(), "task-1", "run-1")

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	if pub.published[0].Topic != codevaldwork.TopicReviewPassed {
		t.Errorf("topic: want %q, got %q", codevaldwork.TopicReviewPassed, pub.published[0].Topic)
	}
	// Both results written back.
	if mgr.writeResults["c1"].result != "passed" {
		t.Errorf("c1 result: want %q, got %q", "passed", mgr.writeResults["c1"].result)
	}
	if mgr.writeResults["c2"].result != "passed" {
		t.Errorf("c2 result: want %q, got %q", "passed", mgr.writeResults["c2"].result)
	}
}

func TestReviewer_OneCriteriaFailed_EmitsReviewFailed(t *testing.T) {
	t.Parallel()
	mgr := &fakeManager{
		criteria:     []codevaldwork.AcceptanceCriteria{makeCriterion("c1", "C1"), makeCriterion("c2", "C2"), makeCriterion("c3", "C3")},
		deliverables: []codevaldwork.Deliverable{makeDeliverable("d1")},
	}
	pub := &fakePublisher{}
	eval := &fakeEvaluator{outcomes: map[string]struct {
		result string
		notes  string
		err    error
	}{
		"c1": {"passed", "ok", nil},
		"c2": {"failed", "test output missing", nil},
		"c3": {"passed", "ok", nil},
	}}

	rev := reviewer.New(mgr, pub, eval, "a1")
	rev.Review(context.Background(), "task-1", "run-1")

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	if pub.published[0].Topic != codevaldwork.TopicReviewFailed {
		t.Errorf("topic: want %q, got %q", codevaldwork.TopicReviewFailed, pub.published[0].Topic)
	}
	payload, ok := pub.published[0].Payload.(codevaldwork.ReviewOutcomePayload)
	if !ok {
		t.Fatalf("unexpected payload type: %T", pub.published[0].Payload)
	}
	if len(payload.FailedCriteria) != 1 {
		t.Errorf("FailedCriteria: want 1, got %d", len(payload.FailedCriteria))
	}
	if payload.FailedCriteria[0].CriterionID != "c2" {
		t.Errorf("failed criterion ID: want %q, got %q", "c2", payload.FailedCriteria[0].CriterionID)
	}
}

func TestReviewer_BlockedResult_CountsAsFailed(t *testing.T) {
	t.Parallel()
	mgr := &fakeManager{
		criteria:     []codevaldwork.AcceptanceCriteria{makeCriterion("c1", "C1")},
		deliverables: []codevaldwork.Deliverable{},
	}
	pub := &fakePublisher{}
	eval := &fakeEvaluator{outcomes: map[string]struct {
		result string
		notes  string
		err    error
	}{
		"c1": {"blocked", "LLM error", errors.New("evaluation failed")},
	}}

	rev := reviewer.New(mgr, pub, eval, "a1")
	rev.Review(context.Background(), "task-1", "run-1")

	if pub.published[0].Topic != codevaldwork.TopicReviewFailed {
		t.Errorf("topic: want %q, got %q", codevaldwork.TopicReviewFailed, pub.published[0].Topic)
	}
	if mgr.writeResults["c1"].result != "blocked" {
		t.Errorf("c1 result: want %q, got %q", "blocked", mgr.writeResults["c1"].result)
	}
}

func TestReviewer_SkippedCriteria_DoesNotCountAsFailed(t *testing.T) {
	t.Parallel()
	mgr := &fakeManager{
		criteria:     []codevaldwork.AcceptanceCriteria{makeCriterion("c1", "C1"), makeCriterion("c2", "C2")},
		deliverables: []codevaldwork.Deliverable{},
	}
	pub := &fakePublisher{}
	eval := &fakeEvaluator{outcomes: map[string]struct {
		result string
		notes  string
		err    error
	}{
		"c1": {"passed", "ok", nil},
		"c2": {"skipped", "not applicable", nil},
	}}

	rev := reviewer.New(mgr, pub, eval, "a1")
	rev.Review(context.Background(), "task-1", "run-1")

	if pub.published[0].Topic != codevaldwork.TopicReviewPassed {
		t.Errorf("topic: want %q, got %q", codevaldwork.TopicReviewPassed, pub.published[0].Topic)
	}
}

func TestReviewer_PayloadCarriesTaskAndRunID(t *testing.T) {
	t.Parallel()
	mgr := &fakeManager{}
	pub := &fakePublisher{}
	eval := &fakeEvaluator{}

	rev := reviewer.New(mgr, pub, eval, "agency-42")
	rev.Review(context.Background(), "task-xyz", "run-abc")

	payload, ok := pub.published[0].Payload.(codevaldwork.ReviewOutcomePayload)
	if !ok {
		t.Fatalf("unexpected payload type: %T", pub.published[0].Payload)
	}
	if payload.TaskID != "task-xyz" {
		t.Errorf("TaskID: want %q, got %q", "task-xyz", payload.TaskID)
	}
	if payload.WorkflowRunID != "run-abc" {
		t.Errorf("WorkflowRunID: want %q, got %q", "run-abc", payload.WorkflowRunID)
	}
	if payload.AgencyID != "agency-42" {
		t.Errorf("AgencyID: want %q, got %q", "agency-42", payload.AgencyID)
	}
}
