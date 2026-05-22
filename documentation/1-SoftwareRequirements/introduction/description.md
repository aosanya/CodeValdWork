**CodeValdWork** is a dedicated task management microservice for AI agent orchestration. It serves as the authoritative source of truth for task state across the CodeVald platform — tracking every work item from `pending` through `in_progress` to `completed`, `failed`, or `cancelled`, with enforced lifecycle transitions that prevent invalid state jumps.

Built in Go with a gRPC API, ArangoDB persistence, and first-class integration with the CodeVald service mesh. AI agents and orchestrators query it to claim work, update progress, and filter tasks by status, priority, or assignment — all without stepping on each other.

Part of the **CodeVald** platform — infrastructure for coordinating AI agents at scale.

GitHub: https://github.com/aosanya/CodeValdWork
