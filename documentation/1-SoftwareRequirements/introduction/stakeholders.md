# Stakeholders

## Primary Consumer

| Stakeholder | Role |
|---|---|
| **AI Agents** | Receive task assignments; transition tasks to `in_progress`, then `completed` or `failed` |
| **CodeValdCross** | Creates tasks when agencies receive new work; queries task status for routing decisions; marks tasks complete when agent output is accepted; routes `cross.task.requested` and `cross.agency.created` events to CodeValdWork |

## Secondary Consumers

| Stakeholder | Role |
|---|---|
| **Operators** | Monitor task backlogs; cancel stuck tasks; query task history |
| **CodeValdFortex** | (Future) UI display of task lists, status badges, and task detail views |
