# Model, Command and History quick reference

> Canonical semantics: `TARGET_ARCHITECTURE.md` §4.3. Current facts: `CURRENT_ARCHITECTURE.md` §4.1–4.4.

Use this page when changing Domain Command, Workspace, Revision, ChangeSet, dependency projection, Undo/Redo or evaluator normalization. It is a navigation projection, not a second schema specification.

## Authority and lifecycle

```text
UI Command
  -> optional Interaction/Preview
  -> versioned Domain Command / Transaction
  -> pure candidate model transform
  -> evaluator/solver normalization outside DB transaction
  -> rebuild ChangeSet from final model
  -> short Workspace CAS commit
  -> immutable Revision + projections + Outbox
```

- Workspace owns mutable Head/sequence/base; Revision snapshots are immutable.
- One understandable user action forms one idempotent Transaction. Pointer samples and preview poses never become Revision history.
- Handler output is only a candidate. If evaluation changes coordinates, diagnostics, parameters or derived fields, the authoritative ChangeSet is rebuilt after evaluation.
- Expensive geometry/network work is outside the database transaction. Final commit verifies the expected Head so late results cannot overwrite a newer Revision.
- Undo/Redo append compensating/replay actions and preserve history. Restore, Branch and Feature Rollback are different operations.

## Property and dependency completion rule

A new writable property is complete only when the same stable PropertySlot participates in:

1. handler ChangeSet generation;
2. current-value lookup;
3. compensation/replay writeback;
4. canonical digest;
5. dependency seeds/dirty closure;
6. two-step Undo/Redo tests.

Every final dependency graph must contain both endpoints of every edge. Incremental evaluation must remain semantically equivalent to a cold rebuild.

## Source route

| Concern | Start here |
|---|---|
| public model/view types | `services/internal/workspace/model.go` |
| typed command registry and handlers | `services/internal/workspace/model_core.go` |
| legacy REST/UI command adaptation | `services/internal/workspace/legacy_commands.go` |
| parameter/dependency evaluation | `services/internal/workspace/evaluation_projection.go` |
| evaluation and initial transaction persistence | `services/internal/workspace/evaluation_persistence.go` |
| generic command/change/graph primitives | `services/internal/modelcore/` |
| workspace service/database orchestration | `services/internal/workspace/service.go` |
| authoritative tests | neighboring `*_test.go`, especially history/model core scenarios |

Search the command type URI, PropertySlot or test name before reading these files. `service.go` and `model_core.go` remain large context objects.

## Failure gates

| Signal | Check |
|---|---|
| digest mismatch / original after value missing | ChangeSet was built before final evaluation, or slot read/write/canonicalization is incomplete |
| second Undo/Redo disappears | capability and execution use different action-log folding, or redo is modeled as one value |
| unknown dependency node | graph edges were not rebuilt atomically from the final model |
| late preview/compute changes Head | expected revision/sequence was not checked at CAS promotion |
| retry duplicates history | request/transaction idempotency is incomplete |

## Validation

```bash
invoke check --scope workspace --match '<test regex>'
invoke check --scope workspace
```

Escalate to services/web when a public view or API changes, and to `all` for migrations, shared protocol, cross-language evaluator contracts or broad history semantics. Development data reset does not replace state-transition, retry and Undo/Redo tests.
