// import.go implements [TaskManager.ImportProject], [TaskManager.StartImportProject],
// [TaskManager.GetImportProjectStatus], and [TaskManager.CancelImportProject].
//
// ImportProject is a synchronous convenience wrapper over the async
// StartImportProject → poll pattern, kept for backward compatibility with the
// existing gRPC surface. For large documents the async path is preferred.
//
// The async flow mirrors CodeValdGit's ImportRepo / GetImportStatus pattern:
//  1. StartImportProject creates an ImportProjectJob entity (status=pending)
//     and returns immediately.
//  2. A background goroutine parses the document, creates all entities, and
//     transitions the job through pending → running → completed | failed |
//     cancelled.
//  3. GetImportProjectStatus polls the stored entity and augments the result
//     with in-memory progress steps while the goroutine is alive.
//  4. CancelImportProject signals the goroutine via context cancellation.
package codevaldwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// ── Status constants ──────────────────────────────────────────────────────────

const (
	importJobStatusPending   = "pending"
	importJobStatusRunning   = "running"
	importJobStatusCompleted = "completed"
	importJobStatusFailed    = "failed"
	importJobStatusCancelled = "cancelled"
)

// importJobTypeID is the TypeDefinition.Name for ImportProjectJob entities.
const importJobTypeID = "ImportProjectJob"

// ── In-process job tracking ───────────────────────────────────────────────────

// importJobEntry holds the cancel function and in-memory progress log for an
// in-flight import goroutine.
type importJobEntry struct {
	cancel context.CancelFunc
	mu     sync.Mutex
	steps  []string
}

func (e *importJobEntry) appendStep(msg string) {
	e.mu.Lock()
	e.steps = append(e.steps, msg)
	e.mu.Unlock()
}

func (e *importJobEntry) getSteps() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.steps))
	copy(out, e.steps)
	return out
}

var (
	importJobsMu sync.Mutex
	importJobs   = make(map[string]*importJobEntry)
)

// ── JSON document schema ──────────────────────────────────────────────────────

// importDoc is the JSON schema for a project import document.
type importDoc struct {
	Project    string       `json:"project"`
	TaskPrefix string       `json:"task_prefix"`
	Tasks      []importTask `json:"tasks"`
}

type importTask struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Priority    string   `json:"priority"`
	DependsOn   []string `json:"depends_on"`
	Description string   `json:"description"`
}

// ── Public interface methods ──────────────────────────────────────────────────

// ImportProject is a synchronous wrapper: it calls StartImportProject, then
// blocks until the background goroutine finishes, and returns the result.
// This preserves the existing gRPC surface while the implementation goes async.
func (m *taskManager) ImportProject(ctx context.Context, agencyID, document string) (ImportResult, error) {
	job, err := m.StartImportProject(ctx, agencyID, document)
	if err != nil {
		return ImportResult{}, err
	}

	for {
		select {
		case <-ctx.Done():
			_ = m.CancelImportProject(ctx, agencyID, job.ID)
			return ImportResult{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}

		current, err := m.GetImportProjectStatus(ctx, agencyID, job.ID)
		if err != nil {
			return ImportResult{}, fmt.Errorf("ImportProject: poll: %w", err)
		}
		switch current.Status {
		case importJobStatusCompleted:
			proj, projErr := m.GetProjectByName(ctx, agencyID, current.ProjectName)
			if projErr != nil {
				return ImportResult{TasksCreated: current.TasksCreated, DepsCreated: current.DepsCreated}, nil
			}
			tasks, _ := m.ListTasksInProject(ctx, agencyID, proj.ID)
			return ImportResult{
				Project:      proj,
				Tasks:        tasks,
				TasksCreated: current.TasksCreated,
				DepsCreated:  current.DepsCreated,
			}, nil
		case importJobStatusFailed:
			return ImportResult{}, fmt.Errorf("%w: %s", ErrInvalidImport, current.ErrorMessage)
		case importJobStatusCancelled:
			return ImportResult{}, fmt.Errorf("import cancelled")
		}
	}
}

// StartImportProject validates the document, creates an ImportProjectJob entity,
// starts the background goroutine, and returns immediately.
func (m *taskManager) StartImportProject(ctx context.Context, agencyID, document string) (ImportProjectJob, error) {
	// Validate the document before creating any entities.
	if err := validateImportDoc(document); err != nil {
		return ImportProjectJob{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	jobEntity, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID: agencyID,
		TypeID:   importJobTypeID,
		Properties: map[string]any{
			"status":        importJobStatusPending,
			"error_message": "",
			"tasks_created": 0,
			"deps_created":  0,
			"created_at":    now,
			"updated_at":    now,
		},
	})
	if err != nil {
		return ImportProjectJob{}, fmt.Errorf("StartImportProject: create job: %w", err)
	}
	job := importProjectJobFromEntity(jobEntity)

	jobCtx, cancel := context.WithCancel(context.Background())
	entry := &importJobEntry{cancel: cancel}
	importJobsMu.Lock()
	importJobs[job.ID] = entry
	importJobsMu.Unlock()

	go m.runImport(jobCtx, agencyID, job.ID, document, entry)

	return job, nil
}

// GetImportProjectStatus returns the current state of an async import job.
func (m *taskManager) GetImportProjectStatus(ctx context.Context, agencyID, jobID string) (ImportProjectJob, error) {
	entity, err := m.dm.GetEntity(ctx, agencyID, jobID)
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return ImportProjectJob{}, ErrImportJobNotFound
		}
		return ImportProjectJob{}, fmt.Errorf("GetImportProjectStatus: %w", err)
	}
	if entity.TypeID != importJobTypeID {
		return ImportProjectJob{}, ErrImportJobNotFound
	}
	job := importProjectJobFromEntity(entity)

	importJobsMu.Lock()
	entry, ok := importJobs[jobID]
	importJobsMu.Unlock()
	if ok {
		job.ProgressSteps = entry.getSteps()
	}
	return job, nil
}

// CancelImportProject signals the background goroutine to stop.
func (m *taskManager) CancelImportProject(ctx context.Context, agencyID, jobID string) error {
	job, err := m.GetImportProjectStatus(ctx, agencyID, jobID)
	if err != nil {
		return err
	}
	switch job.Status {
	case importJobStatusCompleted, importJobStatusFailed, importJobStatusCancelled:
		return ErrImportJobNotCancellable
	}

	importJobsMu.Lock()
	entry, ok := importJobs[jobID]
	importJobsMu.Unlock()
	if ok {
		entry.cancel()
	}
	return m.updateImportJobStatus(context.Background(), agencyID, jobID, importJobStatusCancelled, "")
}

// ── Background goroutine ──────────────────────────────────────────────────────

func (m *taskManager) runImport(ctx context.Context, agencyID, jobID, document string, entry *importJobEntry) {
	defer func() {
		importJobsMu.Lock()
		delete(importJobs, jobID)
		importJobsMu.Unlock()
	}()

	if err := m.updateImportJobStatus(ctx, agencyID, jobID, importJobStatusRunning, ""); err != nil {
		return
	}
	entry.appendStep("Parsing import document…")

	var doc importDoc
	if err := json.Unmarshal([]byte(document), &doc); err != nil {
		m.failImportJob(ctx, agencyID, jobID, err.Error())
		return
	}

	entry.appendStep(fmt.Sprintf("Creating project %q…", doc.Project))
	proj, err := m.CreateProject(ctx, agencyID, Project{Name: doc.Project, TaskPrefix: doc.TaskPrefix})
	if err != nil {
		m.failImportJob(ctx, agencyID, jobID, fmt.Sprintf("create project: %v", err))
		return
	}

	idMap := make(map[string]string, len(doc.Tasks))
	tasksCreated := 0

	for _, it := range doc.Tasks {
		select {
		case <-ctx.Done():
			_ = m.updateImportJobStatus(context.Background(), agencyID, jobID, importJobStatusCancelled, "")
			return
		default:
		}
		entry.appendStep(fmt.Sprintf("Creating task %q…", it.Name))
		t, err := m.CreateTask(ctx, agencyID, Task{
			Title:       it.Title,
			Priority:    parsePriority(it.Priority),
			Description: it.Description,
			TaskName:    it.Name,
			ProjectName: proj.ProjectName,
		})
		if err != nil {
			m.failImportJob(ctx, agencyID, jobID, fmt.Sprintf("create task %s: %v", it.Name, err))
			return
		}
		shortKey := strings.TrimPrefix(it.Name, doc.TaskPrefix)
		idMap[shortKey] = t.ID
		tasksCreated++

		if err := m.AddTaskToProject(ctx, agencyID, t.ID, proj.ID); err != nil {
			m.failImportJob(ctx, agencyID, jobID, fmt.Sprintf("add task %s to project: %v", it.Name, err))
			return
		}
	}

	depsCreated := 0
	entry.appendStep("Writing dependency edges…")
	for _, it := range doc.Tasks {
		shortKey := strings.TrimPrefix(it.Name, doc.TaskPrefix)
		fromID := idMap[shortKey]
		for _, depShortID := range it.DependsOn {
			toID, ok := idMap[depShortID]
			if !ok {
				continue
			}
			_, err := m.CreateRelationship(ctx, agencyID, Relationship{
				Label:  RelLabelDependsOn,
				FromID: fromID,
				ToID:   toID,
				Properties: map[string]any{
					"created_at": time.Now().UTC().Format(time.RFC3339),
				},
			})
			if err != nil {
				m.failImportJob(ctx, agencyID, jobID, fmt.Sprintf("depends_on %s→%s: %v", it.Name, depShortID, err))
				return
			}
			depsCreated++
		}
	}

	entry.appendStep(fmt.Sprintf("Done: %d tasks, %d deps.", tasksCreated, depsCreated))
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = m.dm.UpdateEntity(context.Background(), agencyID, jobID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{
			"status":        importJobStatusCompleted,
			"tasks_created": tasksCreated,
			"deps_created":  depsCreated,
			"project_name":  proj.ProjectName,
			"updated_at":    now,
		},
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m *taskManager) updateImportJobStatus(ctx context.Context, agencyID, jobID, status, errMsg string) error {
	_, err := m.dm.UpdateEntity(ctx, agencyID, jobID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{
			"status":        status,
			"error_message": errMsg,
			"updated_at":    time.Now().UTC().Format(time.RFC3339),
		},
	})
	return err
}

func (m *taskManager) failImportJob(ctx context.Context, agencyID, jobID, errMsg string) {
	_ = m.updateImportJobStatus(context.Background(), agencyID, jobID, importJobStatusFailed, errMsg)
}

// importProjectJobFromEntity converts an entity to an ImportProjectJob.
func importProjectJobFromEntity(e entitygraph.Entity) ImportProjectJob {
	return ImportProjectJob{
		ID:           e.ID,
		AgencyID:     e.AgencyID,
		Status:       entitygraph.StringProp(e.Properties, "status"),
		ErrorMessage: entitygraph.StringProp(e.Properties, "error_message"),
		TasksCreated: int(entitygraph.Int64Prop(e.Properties, "tasks_created")),
		DepsCreated:  int(entitygraph.Int64Prop(e.Properties, "deps_created")),
		ProjectName:  entitygraph.StringProp(e.Properties, "project_name"),
		CreatedAt:    entitygraph.StringProp(e.Properties, "created_at"),
		UpdatedAt:    entitygraph.StringProp(e.Properties, "updated_at"),
	}
}

// validateImportDoc parses and validates the document structure without side effects.
func validateImportDoc(document string) error {
	var doc importDoc
	if err := json.Unmarshal([]byte(document), &doc); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidImport, err.Error())
	}
	if doc.Project == "" {
		return fmt.Errorf("%w: \"project\" field is required", ErrInvalidImport)
	}
	if len(doc.Tasks) == 0 {
		return fmt.Errorf("%w: \"tasks\" array must not be empty", ErrInvalidImport)
	}
	for i, t := range doc.Tasks {
		if t.Name == "" {
			return fmt.Errorf("%w: task[%d] missing \"name\"", ErrInvalidImport, i)
		}
	}
	return nil
}

func parsePriority(s string) TaskPriority {
	switch s {
	case "low":
		return TaskPriorityLow
	case "high":
		return TaskPriorityHigh
	case "critical":
		return TaskPriorityCritical
	default:
		return TaskPriorityMedium
	}
}
