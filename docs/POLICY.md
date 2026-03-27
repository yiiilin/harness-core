# POLICY.md

## Goal

Define the permission, approval, and audit model for `harness-core`.

This document is intentionally runtime-core oriented, not product-specific.

---

## Core principle

Block unregistered or disabled capabilities by default.
For registered capabilities, allow / ask / deny should come from explicit policy data and evaluator composition.

The runtime should never assume that because a model requested an action, the action should run.

---

## Policy layers

### 1. Task-level policy
What a task type is allowed to do.

Examples:
- `knowledge_query` → read/search only
- `desktop_control` → UI interaction allowed, system settings denied
- `code_task` → workspace file edits allowed, system paths denied

### 2. Tool-level policy
Whether a tool can be used at all in the current environment.

Examples:
- allow `shell.exec`
- deny `system.shutdown`
- ask for `windows.run_powershell`

### 3. Parameter-level policy
Same tool, different args, different risk.

Examples:
- allow reading `/workspace/**`
- deny reading `~/.ssh/**`
- allow `Get-Process`
- deny destructive PowerShell commands

### 4. Output-level policy
Even successful execution may produce sensitive output that requires redaction or suppression.

---

## Risk model

Recommended initial levels:

- `L1` low risk: read-only, observational
- `L2` medium risk: reversible state changes
- `L3` high risk: destructive, external, or privileged operations

Examples:
- `knowledge.search` → L1
- `windows.list_windows` → L1
- `shell.exec` read-only query → L1
- `windows.type_text` → L2
- file write in workspace → L2
- external POST / send message → L3
- unrestricted shell / registry / system settings → L3

---

## Approval actions

Recommended policy actions:
- `allow`
- `ask`
- `deny`

Recommended approval replies:
- `once`
- `always`
- `reject`

The runtime should support these as generic concepts even if a host app presents them in a custom UI.
Second confirmation stays outside `approval.Record`: model it as a generic `BlockedRuntimeConfirmation`
through `RequestConfirmation(...)` / `RespondBlockedRuntime(...)` / `ResumeBlockedRuntime(...)`, not
as a new approval reply value.

## Approval lifecycle

When policy returns `ask`, the runtime should:
- create a durable pending approval record
- mark the step `blocked`
- move session execution state to `awaiting_approval`
- avoid invoking the tool until a reply is recorded

Recommended resume path:
- host records a reply through `RespondApproval(...)` or transport-equivalent APIs
- `once` allows exactly one resumed execution
- `always` allows reuse for future matching steps
- `reject` fails the blocked step without executing the tool
- `ResumePendingApproval(...)` re-enters the step loop explicitly

---

## What must be explicit in v1

### Must be explicit
- tool risk level
- allowed/denied tools
- path and target restrictions
- approval-required actions
- audit event generation

### Can wait until later
- complex RBAC
- enterprise tenant policy UI
- organization-level inheritance rules
- delegated approvals across teams

---

## Audit requirements

The runtime should emit structured audit events for:
- task creation
- permission request
- permission granted/rejected
- tool invocation
- tool denial
- verification success/failure
- task completion/failure

At minimum, each audit event should include:
- event type
- session id
- task id when available
- step id if relevant
- attempt id / action id when relevant
- trace / causation ids for replay and debugging
- tool name if relevant
- timestamp
- structured payload

---

## Suggested v1 defaults

### Shared-token deployment
- service protected by a single shared token
- tool set starts small
- dangerous tools disabled by default
- approvals required for L3 actions

### Allowed by default (example)
- session management
- list tools
- low-risk read/search tools

### Ask by default (example)
- shell exec
- file write
- desktop interaction that mutates state

### Deny by default (example)
- unrestricted PowerShell
- destructive file deletion outside workspace
- registry edits
- system shutdown / reboot
- external network posting if not explicitly enabled

---

## Policy outcome contract

Every policy decision should be representable as data.

```json
{
  "action": "ask",
  "reason": "tool is medium-risk and requires user confirmation",
  "matched_rule": "desktop_control/windows.type_text"
}
```

This keeps the runtime explainable and testable.

---

## Summary

`harness-core` should not embed ad-hoc safety checks all over the codebase.
It should centralize:
- risk classification
- allow/ask/deny decisions
- approval / resume mechanics
- audit emission
