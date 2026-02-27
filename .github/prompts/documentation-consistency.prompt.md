````prompt
---
agent: agent
---

# Documentation Consistency & Organization Checker

## Purpose
Perform systematic documentation consistency checks through **one question at a time**, identifying outdated references, consolidating related files, and organizing documentation structure for maintainability.

---

## Instructions for AI Assistant

Conduct a comprehensive documentation consistency analysis through **iterative single-question exploration**. Ask ONE question at a time, wait for the response, then decide whether to:
- **🔍 DEEPER**: Go deeper into the same topic with follow-up questions
- **📝 NOTE**: Record an issue/gap for later action
- **➡️ NEXT**: Move to the next consistency check area
- **📊 REVIEW**: Summarize findings and determine next steps

---

## Current Technology Stack (Reference)

**Update this section when stack changes:**

```yaml
Service:
  Language: Go 1.21+
  Module: github.com/aosanya/CodeValdWork
  Transport: gRPC + protobuf
  Storage: ArangoDB via custom Backend interface

Key interface:
  - TaskManager: CreateTask, GetTask, UpdateTask, DeleteTask, ListTasks

Consumer:
  Project: CodeValdCortex
  Integration: called via gRPC; also registered with CodeValdCross

Documentation structure:
  1-SoftwareRequirements:
    requirements: documentation/1-SoftwareRequirements/requirements.md
    introduction: documentation/1-SoftwareRequirements/introduction/
  2-SoftwareDesignAndArchitecture:
    architecture: documentation/2-SoftwareDesignAndArchitecture/architecture.md
  3-SofwareDevelopment:
    mvp: documentation/3-SofwareDevelopment/mvp.md
    mvp-details: documentation/3-SofwareDevelopment/mvp-details/
  4-QA:
    qa: documentation/4-QA/README.md
```

---

## Consistency Check Areas

1. **Requirements vs. Implementation** — are all FRs reflected in code?
2. **Architecture vs. Code** — does the architecture.md match actual file structure?
3. **MVP task status** — are completed tasks marked ✅ in mvp.md?
4. **Proto vs. Go types** — do proto messages align with Go `types.go` structs?
5. **Env vars** — are all env vars in `.env.example` documented in `cmd/server/main.go`?
6. **Cross-references** — do internal links in docs point to existing files?
````
