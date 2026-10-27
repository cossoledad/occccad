# Assembly geometric solver

The current implementation and its focused build instructions are described here.
The module boundaries are documented in
[`SOLVER_ARCHITECTURE.md`](SOLVER_ARCHITECTURE.md).
The equations, graph compilation, numerical iteration and diagnostic algorithms
actually used by the current implementation are recorded separately in
[`SOLVER_ALGORITHMS.md`](SOLVER_ALGORITHMS.md).

`occccad_assembly_solver` is the standalone algorithm module for 3D assembly
constraints. It consumes rigid bodies, body-local geometric elements and geometric
constraints, compiles rigid clusters and connected components, then returns solved
body poses, equation provenance, component DOF and diagnostics. The module does not
depend on OCCT, Product documents, topology naming, RPC or persistence.

## Model and conventions

- A body pose is an `SE(3)` transform mapping local coordinates to world coordinates
  as `R * p + t`. Solver updates are six-dimensional local increments.
- Geometry is an immutable value descriptor owned by one body: `Point`, `Axis`,
  `Plane`, `Cylinder`, `Circle`, `Sphere`, `Cone` or `Frame`. The Product adapter
  resolves frozen stable `InstancePath`/Publication/persistent selections and
  explicit derived roles before solving. One solver body is an occurrence motion
  unit, not each CAD Body inside a multi-Body Part.
- Constraints refer to geometry by `(body_id, geometry_id)`. `Fix` refers to a body
  and preserves its initial pose unless an explicit target pose is supplied.
- Constraints have stable connection identity and `Driving`, `Measured`,
  `Controlled` or `Suppressed` mode. Driving and Controlled constraints enter the
  equation system; Measured constraints are evaluated without moving bodies.
- Angles use radians; descriptor coordinates, lengths and radii use millimeters.
  Quantity source values use SI and convert once in the Domain compilation
  boundary, not independently inside each equation. Length and angular residuals
  are normalized independently through `SolverOptions`.
- Direction and signed-distance branches are explicit. `Unoriented` is convenient
  for symmetric geometric entities, while `Same`/`Opposite` and plane-side options
  preserve user intent when a result has multiple branches.

## Compiled input and diagnostics

The production Go adapter allocates solve-private short geometry IDs by owning
body and endpoint identity. Here, endpoint geometry/body/cluster indices are bound
once after branch freezing. Residual and analytic Jacobian evaluation reuse those
bindings without concatenating or hashing business-reference strings; stable
constraint identities still label output rows and diagnostics. Inputs are immutable.

`SolveResult.metrics` reports compile, residual/Jacobian and solve durations,
evaluation counts and hot endpoint string lookup bytes/counts. Inclusive hard
feasibility/retraction/preference timings expose phase costs. Optional
`SolverOptions.record_evaluation` receives bounded-by-caller numerical evaluation
states; `record_matrices` also records Jacobians. Default solves keep summary
evidence. Scalar `factorization_ms` (including equilibration/orthonormalization),
`bfgs_update_ms`, `pose_build_ms`, `dof_analysis_ms` and `redundancy_ms` add hotspot
evidence without retaining traces. Factorization counters count helper invocations,
including empty-matrix guards. These are inclusive subcosts, not an additive
partition: residual/Jacobian/pose/factorization costs occur inside hard recovery,
preference or diagnostics; retraction occurs inside preference. `Solver::implementation_id()` fingerprints solver/contact sources,
header, Eigen/compiler/build configuration separately from the public policy name.
A grounded inconsistent component with no movable parameters emits explicit
`GROUNDED_CONTRADICTION`; other numerical failures do not prove global infeasibility.

The offline [replay tool](../../services/cmd/occccad-3dreplay/README.md) uses this
same solver and production input adapter. Its tables preserve all saved constraint
states separately from participation and original failure attempts. The
[performance report](../../docs/validation/performance-baseline.md) records
measured payload, allocation, phase and RSS tradeoffs.

## Native primitives and public compilation

The unique production capability source is
[`services/internal/assemblycontract/catalog.json`](../../services/internal/assemblycontract/catalog.json)
(`schemaVersion=1`, `assembly-six-families-v2`). The contract test directory uses
a symlink to it; no independent C++/Go/Web product matrix is maintained. Public
definition v2 uses Coincidence, Contact, Offset, Angle, Fix and Fix Together;
Concentric/Distance/Parallel/Perpendicular/Rigid are numeric primitives or explicit
shortcuts. Product availability comes from that semantic contract and exact server
validation, never a test-report PASS count. The native module receives compiled
values rather than interpreting the public JSON catalog or persistent commands.

Exact native primitives and their supported geometry, ranks, branches and degeneracies are owned by [Solver Algorithms](SOLVER_ALGORITHMS.md). Public composition, support types and lifecycle are owned by the [six-family contract](../../docs/architecture/current/assembly-constraints.md). Analytic contact is not finite-face overlap or mesh collision; native Rigid alone is not the Product multi-member group implementation.

## Graph compilation and numerical implementation

The solver collapses rigid clusters, resolves physical grounding, and separates connected components before local dense augmented-QR feasibility and hierarchical motion-preference solves. Static reference/nominal and interaction driven/held objectives are distinct. `initial_pose` freezes nominal/branch intent; `initial_guess` is only a numerical seed.

Geometric feasibility, preference convergence, physical rank/gauge/DOF and contradiction evidence are reported separately. A raw feasible result need not qualify for Product adoption; the caller's current command contract controls acceptance. Stable equation IDs retain provenance through analytic Jacobians and diagnostics. The actual equations, seed/retraction rules, singular-angle branches, rank thresholds and numerical failure classification are maintained once in [Solver Algorithms](SOLVER_ALGORITHMS.md).

Solves are local, with no global closest-solution or finite-travel guarantee. Instantaneous freedoms are not persistent joints. Dense factorization cannot be interrupted mid-operation; sparse solving and a general minimum-cardinality conflict prover are not implemented. Product sessions, group orchestration, candidate qualification, immutable manifests and CAS remain outside this library.

## Build and validation

Build and run the focused scenarios with:

```sh
cmake --build build/cmake/linux-gcc15-x86_64/release --target occcad_assembly_solver_scenarios
ctest --test-dir build/cmake/linux-gcc15-x86_64/release -R '^assembly/' --output-on-failure
```

For the stable, quiet domain entry (including relevant Go integration and Web assembly scenarios), use `mise run check --scope assembly`. It prints summaries on success and expands subprocess diagnostics on failure.

The executable conformance corpus lives in
[`tests/assembly-corpus`](../../tests/assembly-corpus). It records canonical
freedoms, conflict/degeneracy baselines, permutation invariance, branch continuity
and cold/warm-start equivalence. Run it independently with:

```sh
cmake --build build/cmake/linux-gcc15-x86_64/release --target occcad_assembly_solver_corpus
ctest --test-dir build/cmake/linux-gcc15-x86_64/release -R '^assembly-corpus/' --output-on-failure
```

## Performance

Dated workstation measurements and raw-sample snapshot: [performance baseline](../../docs/validation/performance-baseline.md).

Run the deterministic dense-backend baseline with
`mise run benchmark:baseline`; the assembly result is written to
`build/performance/assembly-solver.txt`.

Cross-layer evidence and safe execution requirements belong to the shared
[contract runner](../../tests/assembly-contract/README.md). Native success alone
does not certify Web, Domain, history or Release. Maintainer feedback is scoped
to current usage, not industrial or performance acceptance.

Run the focused solver benchmark with one command:

```sh
mise run benchmark
# Optional: more samples or all kernel/Router/Session/allocation stages.
mise run benchmark --samples=20
mise run benchmark --stages=static,interaction,service,allocations
```

The command uses existing optimized artifacts selected by `--profile` (default release) (Release by default); it does not configure or rebuild
native targets. Optional service/allocation stages use `go test` and may compile
the Go test harness.
Default stages are native static chains/BFGS and interaction/failure paths,
with five samples per scene, run serially. Sanitized/unoptimized builds are
rejected. Each run creates a new timestamped directory under
`build/performance/assembly/`, containing `environment.json`, raw stage logs
and `summary.json` (native means/medians and full interaction samples).
Explicit `service` stages use the same configured SQLite/PostgreSQL development database, ArtifactStore and
analytic fixtures as normal application startup; an environment-blocked test fails the command instead of
reporting performance success. No stage resets data. The underlying runner is
`mise run benchmark`; `--label` accepts a unique
run name and never overwrites an existing run. See the
[performance report](../../docs/validation/performance-baseline.md) for
measured inputs, environment and evidence boundaries.

`occcad_assembly_interaction_benchmark [samples] [scene]` measures kernel-only
latency for `single`, `connected50-200`, `independent50`, and `group-contact`,
plus grounded contradiction, budget, cancellation and invalid-input paths.
`occcad_assembly_solver_benchmark [samples] [long-ids]` covers static plane chains
and the warm rotation objective regression (which actually exercises BFGS).
The 50-body case is one connected component with 200 active definitions,
including explicit dependent loop constraints; output reports physical rank,
actual components and sample count. It does not measure transport or rendering,
and therefore cannot certify input-to-display P95 or 60 Hz.

The translated/rotated FACE 4 to fixed FACE 6 regression is also available as a
[minimal 3dreplay fixture](../../tests/test.data/face4-face6.3dreplay), replayable
through the [standalone CLI](../../services/cmd/occccad-3dreplay/README.md).
