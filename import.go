package codevaldwork

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ImportResult is returned by [TaskManager.ImportProject].
type ImportResult struct {
	// Project is the newly created Project vertex.
	Project Project

	// Tasks are the Task vertices created in document order.
	Tasks []Task

	// DepsCreated is the number of depends_on edges written between tasks.
	DepsCreated int
}

// importDoc is the JSON schema for a project import document.
// DueAt is intentionally absent — imported tasks start with no deadline.
type importDoc struct {
	Project    string       `json:"project"`
	TaskPrefix string       `json:"task_prefix"` // e.g. "MVP-SF"; prefixed onto each task id as the stored tag
	Tasks      []importTask `json:"tasks"`
}

type importTask struct {
	Name        string   `json:"name"`        // full prefixed name, e.g. "MVP-SF-001"
	Priority    string   `json:"priority"`    // "low"|"medium"|"high"|"critical"; default medium
	DependsOn   []string `json:"depends_on"`  // short IDs of prerequisite tasks, e.g. "001"
	Description string   `json:"description"`
}

// ImportProject parses a JSON document describing a project and creates:
//   - one Project vertex (name from the "project" field)
//   - one Task vertex per entry in "tasks" (always starts as pending)
//   - one member_of edge per Task pointing to the Project
//   - depends_on edges for each entry in a task's "depends_on" array whose
//     referenced ID is present in the same document
//
// The task "name" field carries the full prefixed name (e.g. "MVP-SF-001").
// The "task_prefix" is stripped from each name to derive the short key used
// by "depends_on" entries (e.g. "001").
//
// Returns [ErrInvalidImport] when the document is malformed or empty.
func (m *taskManager) ImportProject(ctx context.Context, agencyID, document string) (ImportResult, error) {
	var doc importDoc
	if err := json.Unmarshal([]byte(document), &doc); err != nil {
		return ImportResult{}, fmt.Errorf("%w: %s", ErrInvalidImport, err.Error())
	}
	if doc.Project == "" {
		return ImportResult{}, fmt.Errorf("%w: \"project\" field is required", ErrInvalidImport)
	}
	if len(doc.Tasks) == 0 {
		return ImportResult{}, fmt.Errorf("%w: \"tasks\" array must not be empty", ErrInvalidImport)
	}
	for i, t := range doc.Tasks {
		if t.Name == "" {
			return ImportResult{}, fmt.Errorf("%w: task[%d] missing \"name\"", ErrInvalidImport, i)
		}
	}

	proj, err := m.CreateProject(ctx, agencyID, Project{Name: doc.Project, TaskPrefix: doc.TaskPrefix})
	if err != nil {
		return ImportResult{}, fmt.Errorf("ImportProject: create project: %w", err)
	}

	// shortKey (e.g. "001") → entity ID assigned by the graph
	idMap := make(map[string]string, len(doc.Tasks))
	tasks := make([]Task, 0, len(doc.Tasks))

	for _, it := range doc.Tasks {
		t, err := m.CreateTask(ctx, agencyID, Task{
			Priority:    parsePriority(it.Priority),
			Description: it.Description,
			Tags:        []string{it.Name},
		})
		if err != nil {
			return ImportResult{}, fmt.Errorf("ImportProject: create task %s: %w", it.Name, err)
		}
		shortKey := strings.TrimPrefix(it.Name, doc.TaskPrefix)
		idMap[shortKey] = t.ID
		tasks = append(tasks, t)

		if err := m.AddTaskToProject(ctx, agencyID, t.ID, proj.ID); err != nil {
			return ImportResult{}, fmt.Errorf("ImportProject: add task %s to project: %w", it.Name, err)
		}
	}

	depsCreated := 0
	for _, it := range doc.Tasks {
		shortKey := strings.TrimPrefix(it.Name, doc.TaskPrefix)
		fromID := idMap[shortKey]
		for _, depShortID := range it.DependsOn {
			toID, ok := idMap[depShortID]
			if !ok {
				continue // dependency not in this document — skip
			}
			_, err := m.CreateRelationship(ctx, agencyID, Relationship{
				Label:  RelLabelDependsOn,
				FromID: fromID,
				ToID:   toID,
				Properties: map[string]any{
					"createdAt": time.Now().UTC().Format(time.RFC3339Nano),
				},
			})
			if err != nil {
				return ImportResult{}, fmt.Errorf("ImportProject: depends_on %s→%s: %w", it.Name, depShortID, err)
			}
			depsCreated++
		}
	}

	return ImportResult{Project: proj, Tasks: tasks, DepsCreated: depsCreated}, nil
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
