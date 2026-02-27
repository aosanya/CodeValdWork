# Problem Definition

## Problem

CodeValdCortex is an enterprise multi-agent AI orchestration platform where agencies coordinate AI agents to perform complex tasks. Currently, there is no dedicated, authoritative service for managing the lifecycle of these tasks. Task state is managed ad-hoc within CodeValdCortex, making it difficult to:

1. Track task status reliably across service restarts
2. Route work to agents deterministically
3. Audit task history (who was assigned, when did it transition, what failed)
4. Scale task management independently from the orchestration layer

## Solution

CodeValdWork is a dedicated **task management microservice** that:

- Stores all tasks persistently in ArangoDB (survives restarts)
- Enforces valid lifecycle transitions (no invalid state jumps)
- Exposes a clean gRPC API consumed by CodeValdCortex and AI agents
- Registers itself with CodeValdCross for service discovery and pub/sub routing

## Boundaries

**In scope:**
- Task CRUD operations
- Status lifecycle enforcement
- CodeValdCross registration and heartbeating
- ArangoDB-backed persistence

**Out of scope:**
- Agent assignment logic (CodeValdCortex decides who gets which task)
- Task scheduling / prioritization (CodeValdCortex consumes tasks via its own ordering)
- Task result storage (artifacts belong to CodeValdGit)
