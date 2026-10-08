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
[performance report](../../tests/test.data/assembly-input-performance.md) records
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

| Constraint | Supported geometry |
|---|---|
| Fix | Body pose |
| Coincident | Point-Point, Point-Axis/Cylinder, Point-Plane, Axis/Cylinder pairs, Plane-Plane |
| Exact Coincident | Point-Circle, Point-Sphere, Point-selected-leaf Cone; full Frame-Frame pose |
| SurfaceIncidence | Explicit Point-Cylinder surface, rank 1; legacy Coincident Point-Cylinder still means axis incidence, rank 2 |
| Concentric | Any Axis/Cylinder pair |
| Angle | Any pair of Plane, Axis or Cylinder directions |
| Distance | Point-Point, Point-Axis/Cylinder, Point-Plane, Axis/Cylinder pairs, Axis/Cylinder-Plane, Plane-Plane |
| Parallel | Plane/Axis/Cylinder direction pairs; generic rank 2 |
| Perpendicular | Plane/Axis/Cylinder direction pairs; generic rank 1 |
| Contact | Plane-Plane face; Plane-Cylinder line; Plane-Sphere point; Cylinder-Cylinder line/face; Sphere-Sphere face; Sphere-Cone/Sphere-Circle ring; Cone-Cone line/face; Cone-Circle ring |

Zero driving Point-Point/Point-Axis distances compile to coincidence equations with rank 3/2; a scalar norm at zero cannot represent that manifold with a regular Jacobian. The composition corpus checks ranks zero through six, the position/direction/clocking construction and suppression.

`DistanceRelation::SelectedPlaneNormal` evaluates `n·(p_first-p_second)`, choosing the first plane normal for two planes or the unique plane otherwise. Direction relation is independent of the signed target. Legacy second-normal modes retain their equations. `AssemblyOffset.*` scenarios assert final rotated geometry, exchange/normal transformations, actual movement, rank and analytic Jacobians; see the shared [contract runner](../../tests/assembly-contract/README.md) for layered evidence.

Cylinder-Cylinder `Coincident` includes equal radius; `Concentric` deliberately does
not. Plane distance also imposes parallelism, which makes it a stable assembly mate
rather than a closest-point measurement between arbitrary planes.

Curve support here means an exact infinite Line or explicit underlying Circle;
surface incidence is Plane/Cylinder/Sphere/selected-leaf Cone, not arbitrary NURBS
or browser polylines. Frame relationships outside full Frame-Frame use explicit
origin/axis/plane roles. Contact uses its analytic support/material equations, not
zero Distance, finite-face overlap or mesh collision. Its 11 branches, ranks,
degeneracies and side semantics are documented in
[Solver Algorithms](SOLVER_ALGORITHMS.md#解析-contact).

Fix Together retains Domain group identity and membership. Frozen manifest
`groupStages` solves member-internal constraints first, captures successful
relative relationships, then supplies compiled Rigid links to the outer solve.
Native Rigid alone is not a multi-member group implementation. Failed internal or
outer results cannot promote captured relationships or candidate poses.

## Graph compilation and numerical implementation

Active `Rigid` constraints are collapsed into rigid clusters. Consistent `Fix`
targets ground a whole cluster and remove it from the tangent variable vector. The
cluster/constraint incidence graph is split into deterministic connected
components; `SolverOptions.affected_body_ids` can restrict numeric updates to the
components touched by an interaction or edit.

`SolverOptions.solve_intent` is request-scoped placement policy. Binary constraint
creation and editing mark the first occurrence moving and the second reference.
Static solving first restores all physical constraints, minimizes reference motion on that
feasible manifold, then minimizes total occurrence motion in the reference-optimal
subspace. Physical Fix/Rigid constraints remain authoritative: a fixed first body
can require the unfixed reference to move. There is no weak role weighting and no
"fix reference, fail, release and retry" path. A single consistent reference cluster
can still eliminate the six global gauge coordinates of an ungrounded component.

`Body.initial_pose` freezes nominal motion and branch intent; optional
`initial_guess` is only a numerical seed. Motion uses occurrence origins and
rotation Log with independent length/angle scales, including every rigid member.
Geometric convergence and `MotionPreference.status` are separate. Product rejects
feasible results whose preference has not converged; raw RPC callers receive both
states and may inspect the evidence. Local stationarity is not a global nonconvex
optimality guarantee. The algorithm and acceptance corpus are in
`SOLVER_ALGORITHMS.md` and `tests/motion_scenarios.cpp`.

For an isolated positive unsigned distance between parallel infinite lines,
initial recovery uses a radial translation. With one free cluster, preference
optimization stays in the local parallel chart instead of differentiating
through the nonsmooth skew-line limit. Chart rows do not affect physical
equations, rank or DOF; this is not an added Parallel constraint or a global
minimum-motion guarantee. Zero-distance intersection keeps its physical crossing
manifold; the parallel local chart and explicit Parallel coupling have dedicated
rank, finite-motion and physical-Fix-conflict regressions. The chart never adds a
hidden persistent Parallel/Fix. Regression:
`AssemblyOffset.ParallelOffOriginEdgesMoveToRequestedDistance`.

The feasibility LM translation damping floor scales as `1/length_scale^2`, avoiding
over-penalized translation of off-origin supports. Preference energy bounds are
frozen after the same strict feasibility retraction used for candidates. Legal
stationary initial directions use trial-only seeds: 90 degrees for Plane-Cylinder
Contact and Axis-Plane Coincidence/Offset, 180 degrees for antipodal Plane-Plane
Contact. These preserve nominal/Fix/captured-group data, physical equations, rank,
motion priorities and tolerances.

Each selected component uses deterministic damped least squares whose linearized
step is solved as an augmented QR problem without forming normal equations. Typed
compiled equations provide forward analytic derivatives in the stable cluster
tangent ordering; optional central differences are a conformance oracle. Column-
normalized SVD reports relative and six-dimensional gauge freedom separately and
returns the numeric null-space basis, singular values and applied threshold.
Incremental block-rank
analysis reports equation count, effective rank and chosen-basis incremental rank;
failed components may run bounded single-constraint removal probes. Stable equation
IDs map every residual row back to its Connection and Constraint. Every component
uses bounded deterministic backtracking: rotating a body also moves an off-origin
support point, so even plane coincidence can require a smaller step than the raw LM
candidate.

Directed Angle projects Directed Angle endpoints onto the reference-axis normal
plane, uses periodic scalar angle errors at every target including zero/pi, and
exposes wrapped/unwrapped/winding branch state. Spatial angles without an axis
use true separation in [0, 2pi] with a transported sector selector, distinguishing
90 from 270 without projecting onto a fixed rotation axis.
Spatial endpoints 0/pi/2pi use rank-two alignment; regular angles have rank one.
Explicit direction, distance-side and directed-angle branches preserve intent.
Unoriented alignment admits both directions; differential checks stay in the
local branch selected at the base point. The versioned SolverProfile and
rank/branch/suspected-conflict diagnostics cross the Proto, Worker and Go boundary.

Modern DIRECTED Angle resolves `angle_reference_geometry` in its own body, which
may be an independent third occurrence and participates in the component/Jacobian.
`reverse_angle_reference` is explicit; selection exchange does not change that
owner. Legacy `angle_reference_direction` retains its second-body-local frozen
interpretation. New inputs use `assembly-six-families-composition-v10`; the
control-plane manifest schema remains 1 and SolverProfile schema remains 2.

The solver returns per-body instantaneous translation, rotation/screw and allowed/blocked
subspaces with linearization poses, metric scales, reference frame and rank
threshold. Canonical revolute, prismatic, cylindrical, planar and spherical families
are inferred from subspaces; ambiguous combinations remain `Coupled`. These are
local differential freedoms, not persisted Engineering Connections or guarantees of
finite travel. Product previews expose this evidence in the constraint dialog.
Persistent topology and durable solve manifests are provided by the current
Product/Worker boundary, not queried by this module. Continuous dragging uses a distinct
pure-value objective (below); a general minimum-cardinality conflict prover and
large-scale sparse backend are not implemented by this mathematical module.

Build and run the focused scenarios with:

```sh
cmake --build build/cmake/debug --target occcad_assembly_solver_scenarios
ctest --test-dir build/cmake/debug -R '^assembly/' --output-on-failure
```

For the stable, quiet domain entry (including relevant Go integration and Web assembly scenarios), use `invoke check --scope assembly`. It prints summaries on success and expands subprocess diagnostics on failure.

The executable conformance corpus lives in
[`tests/assembly-corpus`](../../tests/assembly-corpus). It records canonical
freedoms, conflict/degeneracy baselines, permutation invariance, branch continuity
and cold/warm-start equivalence. Run it independently with:

```sh
cmake --build build/cmake/debug --target occcad_assembly_solver_corpus
ctest --test-dir build/cmake/debug -R '^assembly-corpus/' --output-on-failure
```

Run the deterministic dense-backend baseline with
`invoke performance-baseline`; the assembly result is written to
`build/performance/assembly-solver.txt`.

Cross-layer evidence and safe execution requirements belong to the shared
[contract runner](../../tests/assembly-contract/README.md). Native success alone
does not certify Web, Domain, history or Release. Maintainer feedback is scoped
to current usage, not industrial or performance acceptance.

The focused, serial optimized kernel/Router/Session runner is
`python kernel/assembly/tests/run_performance.py --label before --samples 5`
(and `--label after` after rebuilding the same configuration). It requires an
optimized configured build, the existing dedicated PostgreSQL test database and
analytic fixtures, and records actual flags/hardware, cold/warm samples, validity,
iterations, components, phase timings and allocation probes. See the
[first-round report](../../tests/test.data/assembly-solver-performance.md) for
build/reproduction commands and measurement boundaries. It never resets data.

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

When the BFGS preference direction stalls, a positive Lagrangian-curvature
fallback retries the line search without relaxing geometry, motion priorities or
convergence tolerances. The total-motion fallback applies at a zero reference
minimum; nonzero reference minima keep their existing hierarchical manifold.
