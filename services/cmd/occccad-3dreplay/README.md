# occccad-3dreplay

Replay an immutable numerical snapshot using the production Go adapter and C++ solver. No database, S3, B-Rep, Part evaluator or online topology resolution is needed. From `services/`:

```sh
go run ./cmd/occccad-3dreplay \
  -worker-binary ../build/cmake/debug/workers/geometry/occccad_geometry_worker \
  -mode original -target <NotUpdated-constraint-id> \
  -out /tmp/original-result.3dreplay ../tests/test.data/assembly-notupdated-product.3dreplay
```

`-worker address` uses an existing Worker or Router instead. A local Worker uses its own temporary directory and is closed on success and errors. `-out` requires a new path and never overwrites the source or an existing result. Numerical failure is structured output; malformed input, invalid table references and digests fail explicitly.

## Input and output

The outer JSON schema is `occccad.3dreplay.v1`. Standalone numerical files have one Proto JSON `request` and optional `result`, `budget`, `missing` and `transportError`. Complete Product files instead have **only** `snapshot` (`occccad.assembly-diagnostic.v2`) as their numerical source:

| Field | Contents |
|---|---|
| `definitions`, `sources` | Every saved constraint, original evaluation/suppression/mode, Quantity and branch intent; endpoint references share a source table. |
| `geometry`, `primitives` | Deduplicated exact body-local numerical records, referenced by frames. Identity includes owning occurrence; equal coordinates do not merge objects. |
| `current` | Current poses/guesses, effective profile, intent and numerical table references. `participantIds` lists driving rows separately from saved definitions; suppressed and NotUpdated rows remain available. |
| `groupStages`, `documents` | Group compilation context and source document/revision/Part Body identities. No full Part modeling history or B-Rep bytes. |
| `compileFailures` | Constraint, endpoint, phase and actual resolution/parameter error. Failed endpoints have no fabricated geometry. Other resolvable data is still exported. |
| `attempts` | NotUpdated diagnostic ID, affected definitions, original base revision/manifest identity and the actual ordered numerical RPC sequence, including group stages. Each frame retains its initial poses, guesses, participants, profile, intent, branches, budgets and result/error. |
| `missing` | Uncaptured, pruned or budget-limited data. Missing original attempts are never replaced with the current input. |

`GET /api/documents/{id}/assembly-diagnostic` exports the current Product plus available failure archives with read permission. `?naming=true` adds table-referenced detailed Naming evidence. Export performs no solve or model/history/warm-start write. The DEBUG toolbar keeps separate Part and assembly commands.

Production resolution uses SHA-256 semantic reference cache keys; the shared Go numerical compiler assigns short `g000000` IDs deterministically by `(body, source identity)`. Numerical RPCs and replay use that same compiler. C++ binds endpoints to geometry/body/cluster indices once, after nominal branch freezing, outside residual/Jacobian loops. Stable business references remain in definitions/sources, and stable constraint IDs remain in results. SHA-256 manifest/content validation and commit/CAS identity remain active. Branch winding and sequence counters retain 64-bit values; Proto JSON preserves optional zero values and numerical round trips.

Derived output retains the original `snapshot` unchanged, records options in `derivation`, and adds actual `stages`, the last numerical `result`, outcome and domain `assemblyResult` group/preference evidence. Numerical poses/residuals/ranks belong to RPC results. Effective profiles are recorded after Worker default resolution. Results contain a solver implementation fingerprint of solver/contact sources, public solver header, Eigen version, compiler, build flags/type and architecture; identical policy names alone do not imply identical implementations. Recorded requests reproduce inputs, not a promise of identical future-build outputs.

## Selection and experiments

| `-mode` | Selection |
|---|---|
| `accepted` | Verified, unsuppressed constraints, including measurements. Default current-state retry. |
| `accepted-pending -constraints id1,id2` | Accepted set plus named unsuppressed NotUpdated definitions. Without IDs, explicitly retries all such definitions. |
| `all-driving` | All unsuppressed driving definitions, including unresolved/Broken rows; unavailable inputs produce `INPUT_ERROR`. |
| `subsystem -target id` | Entire connected driving component, retaining explicit Fix, boundary constraints, independent third axes and recursive group membership. No synthetic Fix. |
| `original -target id` or `-diagnostic id` | Exactly one archived failure's actual ordered requests, with its recorded per-stage budgets. Incomplete originals return `INPUT_ERROR` and missing markers. |

The original sequence replays each recorded request in order, including each recorded stage's actual initial guess; it does not substitute newly computed stage results into subsequent original requests. Current-state modes execute the production staged orchestration and can select different participants. Saved document states never change.

`-overrides patch.json` records changes to `initialGuesses`, `angleBranches`, `maxIterations`, `maxPreferenceIterations` or `wallClockMs` in the derived output. Example:

```json
{"maxIterations":250,"maxPreferenceIterations":150,"wallClockMs":20000}
```

`-timeout` is the overall execution deadline (default 30 s); original per-stage budgets remain active unless explicitly overridden. `-trace` captures bounded evaluation states/residuals, and `-matrices` adds final null-space data and traced row-major Jacobians. Trace body poses follow request body order. Default summaries include evaluation counts, aggregate/per-constraint residuals, rank/degeneracy diagnostics, best returned candidate, implementation identity and compile/residual/Jacobian/solve durations. Hard-feasibility, retraction and preference phase durations are inclusive; nested timings cannot be summed as a partition.

`FEASIBLE`, `FEASIBLE_PREFERENCE_NOT_CONVERGED`, `INPUT_ERROR`, `NUMERICAL_NONCONVERGENCE` and `EXECUTION_FAILURE` are distinct. `PROVEN_CONTRADICTION` requires explicit `GROUNDED_CONTRADICTION` evidence: an inconsistent component with no movable parameters under its compiled branches and fixed poses. Timeout, local failure, suspected conflicts or rank loss are not global infeasibility proofs. An archived observed failure is not automatically a valid expected test conclusion.

## Budgets and retained evidence

- Numerical capture and complete export: 8 MiB each. Export drops optional historical sequences/Naming/document metadata with missing markers first; if core definitions/current input still exceed the bound, it fails explicitly.
- One operation/derived sequence: 4 MiB and 32 frames; truncation is explicit. Continuous interaction records are request-scoped, not an unbounded session movie.
- Trace: at most 4 MiB conservative charged storage and 1024 evaluation records. The effective cap and truncation are recorded. Matrices/trajectories are opt-in.
- Async runtime archive: 32 MiB queued bytes and 64 entries; existing local debug repository retention applies (application configuration: 50 files per document/7 days). Operation archives also have an 8 MiB write bound and mark omitted metadata.
- Derived file reader/writer: 24 MiB, allowing the immutable source, bounded new sequence and final result. Archive/capture failures never authorize failed poses or invalidate a formal operation.

Standalone numerical v1 fixtures remain readable. Earlier complete `assembly-diagnostic.v1` exports must be re-exported; no second format adapter is maintained. Original attempts created before capture, removed by retention or dropped by budget are explicitly unavailable. Unresolved topology can be inspected but cannot be numerically solved without its real geometry. Sparse solving and a general nonlinear infeasibility certificate are not implemented.

## Focused tests

Unit tests can load a file and call `workspace.ReplayAssemblyDiagnosticWithOptions(ctx, client, bytes, options)`. `geometry.OpenReplayWorker` supplies a production local solver. The checked-in [Product fixture](../../../tests/test.data/assembly-notupdated-product.3dreplay) contains saved NotUpdated state and its real failed request; the offline test checks preservation and evidence rather than claiming its observed failure is globally proven.

```sh
# services/, matching built Worker; offline tests do not use PostgreSQL
OCCCCAD_TEST_GEOMETRY_WORKER="$PWD/../build/cmake/debug/workers/geometry/occccad_geometry_worker" \
  go test ./internal/workspace -run 'TestAssemblyOfflineReplay|TestAssemblyDiagnostic|TestAssemblyOriginal' -count=1

go test ./internal/geometry ./internal/valuecopy -run 'TestAssembly|TestClone' -count=1
```

Router integration uses `internal/testsupport.OpenPostgres` and a dedicated `OCCCCAD_TEST_DATABASE_URL` (`occccad_*_test`), the matching Worker and exact analytic fixture directory. It validates actual database identity and migrations, allocates fresh documents and its own ArtifactStore, never resets data or falls back to SQLite. See [development environment](../../../docs/development-environment.md). Performance measurements and their limits are in [the focused report](../../../tests/test.data/assembly-input-performance.md).
