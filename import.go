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
type importDoc struct {
	Project    string       `json:"project"`
	TaskPrefix string       `json:"task_prefix"`
	Tasks      []importTask `json:"tasks"`
}

type importTask struct {
	Name        string   `json:"name"`
	Priority    string   `json:"priority"`
	DependsOn   []string `json:"depends_on"`
	Description string   `json:"description"`
}

// ImportProject parses a JSON document describing a project and creates:
//   - one Project vertex
//   - one Task vertex per entry in "tasks"
//   - one member_of edge per Task pointing to the Project
//   - depends_on edges for each entry in a task's "depends_on" array
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
