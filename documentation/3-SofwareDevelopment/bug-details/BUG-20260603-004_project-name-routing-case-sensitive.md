# BUG-20260603-004 — Project-name URL routing is case-sensitive; display name casing returns 404

**Status:** 📋 Open
**Severity:** Medium — any caller that uses the project display name (`SharedFarms`) instead of the stored slug (`sharedfarms`) gets 404; CodeValdWorkFrontend renders "Failed to load project." when the QA doc URL uses display-name casing
**Owner:** CodeValdWork (backend lookup; fix here covers all callers)
**Estimated effort:** ~0.5 day (one-line normalize + integration test update)
**Source finding:** QA 03 run 2026-06-03 — `GET /work/utility-app-builder/projects/SharedFarms/tasks` → 404; `GET /work/utility-app-builder/projects/sharedfarms/tasks` → 200 with 11 tasks

## Problem

`GET /projects/{projectName}/tasks` (and all other project-scoped routes) performs an exact-match lookup on `projectName`. The DB stores slugs in lowercase (`sharedfarms`). If the caller passes the display-name casing (`SharedFarms`) — as the QA doc did — Cross returns:

```json
{"code":"NOT_FOUND","message":"project not found"}
```

The CodeValdWorkFrontend SSR loader treats this as a fatal error and renders the `ErrorBoundary` "Failed to load project." page instead of an empty task list.

| URL form | Result |
|---|---|
| `/projects/sharedfarms/tasks` (lowercase slug) | 200 + 11 tasks |
| `/projects/SharedFarms/tasks` (display name) | 404 NOT_FOUND |

## Evidence

```bash
# 404 path
curl -s http://codevaldcross:8081/work/utility-app-builder/projects/SharedFarms/tasks
# {"code":"NOT_FOUND","message":"project not found"}

# 200 path (correct slug)
curl -s http://codevaldcross:8081/work/utility-app-builder/projects/sharedfarms/tasks
# {"tasks":[...11 items...]}

# Frontend: http://codevaldworkfrontend:50064/agencies/utility-app-builder/projects/SharedFarms/tasks
# HTTP 500, HTML contains: <p ...>Failed to load project.</p>

# Frontend: http://codevaldworkfrontend:50064/agencies/utility-app-builder/projects/sharedfarms/tasks
# HTTP 200, 11 MVP-SF- task rows rendered, no ErrorBoundary
```

## Root cause

`GetProject` (and the tasks-in-project lookup it depends on) queries ArangoDB for an exact match on the `projectName` field. Project slugs are created lowercase at import time (`sharedfarms` for display name `SharedFarms`), but callers that derive the URL from the display name produce mixed-case paths.

No normalization is applied either at the HTTP handler (in Cross's proxy or CodeValdWork's server) or at the query layer before the ArangoDB lookup.

## Fix plan

### Option A — Normalize at query layer (recommended)

In `CodeValdWork`, wherever `projectName` is extracted from the request path and used in an ArangoDB query, apply `strings.ToLower()`:

```go
projectName = strings.ToLower(projectName)
```

This covers all project-scoped routes in one change and is consistent with how the slugs are stored.

### Option B — Normalize at import / creation time with a case-insensitive index

Add an ArangoDB persistent index on `LOWER(doc.projectName)` and normalise the path parameter to lowercase in the Go handler. Functionally equivalent to Option A but adds an index.

### Option C — Redirect at Cross (not recommended)

Cross could redirect mixed-case project paths to their lowercase equivalent. Deferred — better to fix at the source.

**Recommended:** Option A, applied in `internal/server/project_server.go` (and any other handler that takes `projectName` from the path).

## Workaround (already applied)

QA doc `03-codevaldwork-import-and-tasks.md` updated to use lowercase `sharedfarms` in all URL examples and to add a "URL case" note:

> Use the lowercase `projectName` slug (`sharedfarms`). The display name `SharedFarms` triggers a 404 from the API and the frontend renders "Failed to load project."

## Verification

After fix:

```bash
# Both should return 200 with 11 tasks
curl -s http://codevaldcross:8081/work/utility-app-builder/projects/SharedFarms/tasks | jq '.tasks | length'
curl -s http://codevaldcross:8081/work/utility-app-builder/projects/sharedfarms/tasks | jq '.tasks | length'

# Frontend URL with display-name casing should also render correctly
# http://codevaldworkfrontend:50064/agencies/utility-app-builder/projects/SharedFarms/tasks → 200, 11 rows
```

## Dependencies

None. Isolated to CodeValdWork project-lookup handlers.
Secondary: CodeValdWorkFrontend URLs use `projectName` from the API response (already lowercase), so the frontend fix is implicit once the backend normalises on lookup.
