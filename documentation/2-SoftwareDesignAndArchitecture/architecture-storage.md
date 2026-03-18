# CodeValdWork — Architecture: Storage

> Part of [architecture.md](architecture.md)

## 1. Database

CodeValdWork stores all data in ArangoDB. The default database name is
`codevaldwork` (configurable via `ARANGO_DATABASE`). All collection names
carry the `work_` prefix.

---

## 2. Collection Inventory

| Collection | Type | Holds |
|---|---|---|
| `work_schemas` | Document | Pre-delivered Schema documents per agency |
| `work_tasks` | Document | Task entities |
| `work_groups` | Document | TaskGroup entities |
| `work_agents` | Document | Agent vertex entities (work-domain agent projection) |
| `work_relationships` | Edge | All relationship edges (`assigned_to`, `blocks`, `subtask_of`, `depends_on`, `member_of`) |

---

## 3. Named Graph

```
Graph name:  work_graph
Edge collection:     work_relationships
Vertex collections:  work_tasks
                     work_groups
                     work_agents
```

All `TraverseGraph` calls use `work_graph` as the named graph. The `_from`
and `_to` fields in `work_relationships` reference keys within these three
vertex collections.

---

## 4. Document Shapes

### work_schemas

```json
{
  "_key":     "<agencyID>",
  "agencyID": "agency-001",
  "version":  1,
  "types": [
    {
      "name":              "Task",
      "displayName":       "Task",
      "storageCollection": "work_tasks",
      "immutable":         false,
      "properties": [...]
    },
    {
      "name":              "TaskGroup",
      "displayName":       "Task Group",
      "storageCollection": "work_groups",
      "immutable":         false,
      "properties": [...]
    },
    {
      "name":              "Agent",
      "displayName":       "Agent",
      "storageCollection": "work_agents",
      "immutable":         false,
      "properties": [...]
    }
  ],
  "seededAt": "2025-01-01T00:00:00Z"
}
```

One document per agency, keyed by `agencyID`. Seeding is idempotent.

---

### work_tasks (Task)

```json
{
  "_key":            "<uuid>",
  "agencyID":        "agency-001",
  "typeID":          "Task",
  "title":           "Research quantum entanglement",
  "description":     "Summarise recent papers into a 2-page brief.",
  "status":          "pending",
  "priority":        "high",
  "dueAt":           "2026-04-01T09:00:00Z",
  "tags":            ["research", "quantum"],
  "estimatedHours":  4.5,
  "context":         "...",
  "completedAt":     null,
  "createdAt":       "2026-03-18T10:00:00Z",
  "updatedAt":       "2026-03-18T10:00:00Z"
}
```

`status` and `priority` are stored as plain string fields on the document.
Transitions are enforced by `TaskManager` before the storage write, not by
ArangoDB constraints.

---

### work_groups (TaskGroup)

```json
{
  "_key":        "<uuid>",
  "agencyID":    "agency-001",
  "typeID":      "TaskGroup",
  "name":        "Sprint 12",
  "description": "Q2 deliverables",
  "dueAt":       "2026-04-30T00:00:00Z",
  "createdAt":   "2026-03-18T08:00:00Z",
  "updatedAt":   "2026-03-18T08:00:00Z"
}
```

---

### work_agents (Agent)

```json
{
  "_key":        "<uuid>",
  "agencyID":    "agency-001",
  "typeID":      "Agent",
  "agentID":     "external-agent-id",
  "displayName": "ResearchBot",
  "capability":  "research",
  "createdAt":   "2026-03-18T09:00:00Z",
  "updatedAt":   "2026-03-18T09:00:00Z"
}
```

At most one Agent per `(agencyID, agentID)` pair — enforced by the unique
index on `agencyID + agentID`.

---

### work_relationships (all edges)

```json
{
  "_key":    "<uuid>",
  "_from":   "work_tasks/<taskID>",
  "_to":     "work_agents/<agentID>",
  "label":   "assigned_to",
  "agencyID": "agency-001",

  "assignedAt": "2026-03-18T10:05:00Z",
  "assignedBy": "<operatorID>"
}
```

Label-specific shapes:

| Label | Extra properties |
|---|---|
| `assigned_to` | `assignedAt`, `assignedBy` |
| `blocks` | `createdAt`, `reason` |
| `subtask_of` | `createdAt` |
| `depends_on` | `createdAt`, `reason` |
| `member_of` | `addedAt` |

---

## 5. Indexes

### work_tasks
| Fields | Type | Purpose |
|---|---|---|
| `agencyID` | persistent | All task queries scoped by agency |
| `agencyID, status` | persistent | Filter by status (most common query) |
| `agencyID, priority` | persistent | Filter by priority |
| `agencyID, status, priority` | persistent | Combined filter |
| `dueAt` | persistent, ascending | Upcoming due date ordering |

### work_groups
| Fields | Type | Purpose |
|---|---|---|
| `agencyID` | persistent | Scope group queries |
| `dueAt` | persistent, ascending | Upcoming deadline ordering |

### work_agents
| Fields | Type | Purpose |
|---|---|---|
| `agencyID` | persistent | Scope agent queries |
| `agencyID, agentID` | persistent, unique | Enforce one Agent per external agent per agency |
| `capability` | persistent | Filter agents by capability |

### work_relationships
| Fields | Type | Purpose |
|---|---|---|
| `_from, label` | persistent | Outbound edge traversal by label |
| `_to, label` | persistent | Inbound edge traversal by label |
| `agencyID` | persistent | Scope all edge queries |
| `agencyID, label` | persistent | Cross-agency edge isolation |
