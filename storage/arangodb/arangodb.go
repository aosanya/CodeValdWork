// Package arangodb implements the codevaldwork.Backend interface backed by
// ArangoDB. Task documents are stored in a single collection per database.
//
// Use [NewArangoBackend] to construct; pass the result to
// codevaldwork.NewTaskManager.
package arangodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	driver "github.com/arangodb/go-driver"
	"github.com/arangodb/go-driver/http"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

const collectionName = "work_tasks"

// Config holds the connection parameters for the ArangoDB backend.
type Config struct {
	// Endpoint is the ArangoDB HTTP endpoint (e.g. "http://localhost:8529").
	Endpoint string

	// Username is the ArangoDB username (default "root").
	Username string

	// Password is the ArangoDB password.
	Password string

	// Database is the ArangoDB database name (default "codevaldwork").
	Database string
}

// ArangoBackend is the ArangoDB implementation of [codevaldwork.Backend].
type ArangoBackend struct {
	db  driver.Database
	col driver.Collection
}

// NewArangoBackend connects to ArangoDB, ensures the tasks collection exists,
// and returns a ready-to-use [ArangoBackend].
func NewArangoBackend(cfg Config) (*ArangoBackend, error) {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://localhost:8529"
	}
	if cfg.Username == "" {
		cfg.Username = "root"
	}
	if cfg.Database == "" {
		cfg.Database = "codevaldwork"
	}

	conn, err := http.NewConnection(http.ConnectionConfig{
		Endpoints: []string{cfg.Endpoint},
	})
	if err != nil {
		return nil, fmt.Errorf("arangodb: connection: %w", err)
	}

	client, err := driver.NewClient(driver.ClientConfig{
		Connection:     conn,
		Authentication: driver.BasicAuthentication(cfg.Username, cfg.Password),
	})
	if err != nil {
		return nil, fmt.Errorf("arangodb: client: %w", err)
	}

	ctx := context.Background()

	db, err := ensureDatabase(ctx, client, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("arangodb: ensure database: %w", err)
	}

	col, err := ensureCollection(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("arangodb: ensure collection: %w", err)
	}

	return &ArangoBackend{db: db, col: col}, nil
}

// NewArangoBackendFromDB constructs an [ArangoBackend] from an already-open
// [driver.Database]. It ensures the tasks collection exists and returns a
// ready-to-use backend. This constructor is intended for tests that manage
// their own database lifecycle.
func NewArangoBackendFromDB(db driver.Database) (*ArangoBackend, error) {
	if db == nil {
		return nil, fmt.Errorf("arangodb: NewArangoBackendFromDB: database must not be nil")
	}
	col, err := ensureCollection(context.Background(), db)
	if err != nil {
		return nil, fmt.Errorf("arangodb: ensure collection: %w", err)
	}
	return &ArangoBackend{db: db, col: col}, nil
}

func ensureDatabase(ctx context.Context, client driver.Client, name string) (driver.Database, error) {
	exists, err := client.DatabaseExists(ctx, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return client.Database(ctx, name)
	}
	return client.CreateDatabase(ctx, name, nil)
}

func ensureCollection(ctx context.Context, db driver.Database) (driver.Collection, error) {
	exists, err := db.CollectionExists(ctx, collectionName)
	if err != nil {
		return nil, err
	}
	if exists {
		return db.Collection(ctx, collectionName)
	}
	return db.CreateCollection(ctx, collectionName, nil)
}

// ── taskDocument is the ArangoDB document representation ─────────────────────

type taskDocument struct {
	Key         string     `json:"_key,omitempty"`
	AgencyID    string     `json:"agency_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	AssignedTo  string     `json:"assigned_to"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func toDocument(agencyID string, t codevaldwork.Task) taskDocument {
	return taskDocument{
		Key:         t.ID,
		AgencyID:    agencyID,
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		Priority:    string(t.Priority),
		AssignedTo:  t.AssignedTo,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		CompletedAt: t.CompletedAt,
	}
}

func fromDocument(key string, doc taskDocument) codevaldwork.Task {
	return codevaldwork.Task{
		ID:          key,
		AgencyID:    doc.AgencyID,
		Title:       doc.Title,
		Description: doc.Description,
		Status:      codevaldwork.TaskStatus(doc.Status),
		Priority:    codevaldwork.TaskPriority(doc.Priority),
		AssignedTo:  doc.AssignedTo,
		CreatedAt:   doc.CreatedAt,
		UpdatedAt:   doc.UpdatedAt,
		CompletedAt: doc.CompletedAt,
	}
}

// ── Backend interface implementation ─────────────────────────────────────────

// CreateTask implements [codevaldwork.Backend].
func (b *ArangoBackend) CreateTask(ctx context.Context, agencyID string, task codevaldwork.Task) (codevaldwork.Task, error) {
	now := time.Now().UTC()
	task.AgencyID = agencyID
	task.Status = codevaldwork.TaskStatusPending
	task.CreatedAt = now
	task.UpdatedAt = now
	if task.Priority == "" {
		task.Priority = codevaldwork.TaskPriorityMedium
	}

	doc := toDocument(agencyID, task)
	meta, err := b.col.CreateDocument(ctx, doc)
	if err != nil {
		if driver.IsConflict(err) {
			return codevaldwork.Task{}, codevaldwork.ErrTaskAlreadyExists
		}
		return codevaldwork.Task{}, fmt.Errorf("CreateTask: %w", err)
	}

	task.ID = meta.Key
	return task, nil
}

// GetTask implements [codevaldwork.Backend].
func (b *ArangoBackend) GetTask(ctx context.Context, agencyID, taskID string) (codevaldwork.Task, error) {
	var doc taskDocument
	_, err := b.col.ReadDocument(ctx, taskID, &doc)
	if err != nil {
		if driver.IsNotFound(err) {
			return codevaldwork.Task{}, codevaldwork.ErrTaskNotFound
		}
		return codevaldwork.Task{}, fmt.Errorf("GetTask: %w", err)
	}
	if doc.AgencyID != agencyID {
		return codevaldwork.Task{}, codevaldwork.ErrTaskNotFound
	}
	return fromDocument(taskID, doc), nil
}

// UpdateTask implements [codevaldwork.Backend].
func (b *ArangoBackend) UpdateTask(ctx context.Context, agencyID string, task codevaldwork.Task) (codevaldwork.Task, error) {
	task.UpdatedAt = time.Now().UTC()
	doc := toDocument(agencyID, task)
	_, err := b.col.UpdateDocument(ctx, task.ID, doc)
	if err != nil {
		if driver.IsNotFound(err) {
			return codevaldwork.Task{}, codevaldwork.ErrTaskNotFound
		}
		return codevaldwork.Task{}, fmt.Errorf("UpdateTask: %w", err)
	}
	return task, nil
}

// DeleteTask implements [codevaldwork.Backend].
func (b *ArangoBackend) DeleteTask(ctx context.Context, agencyID, taskID string) error {
	// Verify ownership before delete.
	if _, err := b.GetTask(ctx, agencyID, taskID); err != nil {
		return err
	}
	_, err := b.col.RemoveDocument(ctx, taskID)
	if err != nil {
		if driver.IsNotFound(err) {
			return codevaldwork.ErrTaskNotFound
		}
		return fmt.Errorf("DeleteTask: %w", err)
	}
	return nil
}

// ListTasks implements [codevaldwork.Backend].
func (b *ArangoBackend) ListTasks(ctx context.Context, agencyID string, filter codevaldwork.TaskFilter) ([]codevaldwork.Task, error) {
	query, bindVars := buildListQuery(agencyID, filter)

	cursor, err := b.db.Query(ctx, query, bindVars)
	if err != nil {
		return nil, fmt.Errorf("ListTasks: %w", err)
	}
	defer cursor.Close()

	var tasks []codevaldwork.Task
	for cursor.HasMore() {
		var doc taskDocument
		meta, err := cursor.ReadDocument(ctx, &doc)
		if err != nil {
			return nil, fmt.Errorf("ListTasks: read: %w", err)
		}
		tasks = append(tasks, fromDocument(meta.Key, doc))
	}
	if tasks == nil {
		tasks = []codevaldwork.Task{}
	}
	return tasks, nil
}

func buildListQuery(agencyID string, filter codevaldwork.TaskFilter) (string, map[string]interface{}) {
	bindVars := map[string]interface{}{
		"agency": agencyID,
		"@col":   collectionName,
	}
	query := "FOR t IN @@col FILTER t.agency_id == @agency"

	if filter.Status != "" {
		query += " FILTER t.status == @status"
		bindVars["status"] = string(filter.Status)
	}
	if filter.Priority != "" {
		query += " FILTER t.priority == @priority"
		bindVars["priority"] = string(filter.Priority)
	}
	if filter.AssignedTo != "" {
		query += " FILTER t.assigned_to == @assigned_to"
		bindVars["assigned_to"] = filter.AssignedTo
	}
	query += " RETURN t"

	return query, bindVars
}

// isNotFound checks if an ArangoDB error is a 404 not-found response.
// Uses errors.As to handle driver-specific error types.
func isNotFound(err error) bool {
	var ae driver.ArangoError
	return errors.As(err, &ae) && ae.Code == 404
}
