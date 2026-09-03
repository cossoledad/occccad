# Assembly geometric solver

The current implementation and its focused build instructions are described here.
The layered constraint-manager target, external design references and phased
implementation plan are documented in
[`SOLVER_ARCHITECTURE.md`](SOLVER_ARCHITECTURE.md).
The equations, graph compilation, numerical iteration and diagnostic algorithms
actually used by the current M1.7 implementation are recorded separately in
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
  `Plane` or `Cylinder`. An eventual Product adapter must resolve stable
  `InstancePath`/persistent selections into these descriptors before solving.
- Constraints refer to geometry by `(body_id, geometry_id)`. `Fix` refers to a body
  and preserves its initial pose unless an explicit target pose is supplied.
- Constraints have stable connection identity and `Driving`, `Measured`,
  `Controlled` or `Suppressed` mode. Driving and Controlled constraints enter the
  equation system; Measured constraints are evaluated without moving bodies.
- Angles use radians and distances use the caller's model-length unit. Length and
  angular residuals are normalized independently through `SolverOptions`.
- Direction and signed-distance branches are explicit. `Unoriented` is convenient
  for symmetric geometric entities, while `Same`/`Opposite` and plane-side options
  preserve user intent when a result has multiple branches.

## First supported constraint matrix

| Constraint | Supported geometry |
|---|---|
| Fix | Body pose |
| Coincident | Point-Point, Point-Axis/Cylinder, Point-Plane, Axis/Cylinder pairs, Plane-Plane |
| Concentric | Any Axis/Cylinder pair |
| Angle | Any pair of Plane, Axis or Cylinder directions |
| Distance | Point-Point, Point-Plane, Axis/Cylinder pairs, Plane-Plane |

Cylinder-Cylinder `Coincident` includes equal radius; `Concentric` deliberately does
not. Plane distance also imposes parallelism, which makes it a stable assembly mate
rather than a closest-point measurement between arbitrary planes.

## Graph compilation and numerical implementation

Active `Rigid` constraints are collapsed into rigid clusters. Consistent `Fix`
targets ground a whole cluster and remove it from the tangent variable vector. The
cluster/constraint incidence graph is split into deterministic connected
components; `SolverOptions.affected_body_ids` can restrict numeric updates to the
components touched by an interaction or edit.

`SolverOptions.solve_intent` is request-scoped interaction policy, not persisted
constraint asymmetry. For a binary constraint the Product adapter treats the first
selection as moving and the second as reference. `MoveFirstMinimizeReference` keeps
a reference cluster exactly at its nominal pose when an otherwise ungrounded
component needs a six-DOF gauge anchor; it does not create a physical `Fix`. M1.7
also assigns weak cluster-level motion weights in grounded or otherwise non-unique
solutions. Strict hierarchical minimum-reference motion still requires null-space
optimization.

Each selected component uses deterministic damped least squares with separate
translation/rotation central-difference steps. Column-normalized SVD reports
relative and six-dimensional gauge freedom separately. Incremental block-rank
analysis reports equation count, effective rank and chosen-basis incremental rank;
failed components may run bounded single-constraint removal probes. Stable equation
IDs map every residual row back to its Connection and Constraint. Every component
uses bounded deterministic backtracking: rotating a body also moves an off-origin
support point, so even plane coincidence can require a smaller step than the raw LM
candidate.

The M1.7 baseline projects Directed Angle endpoints onto the reference-axis normal
plane, uses periodic scalar angle errors at every target including zero/pi, and
exposes wrapped/unwrapped/winding branch state. Direction, distance-side and angle
branches are frozen outside residual evaluation. The versioned SolverProfile and
rank/branch/suspected-conflict diagnostics cross the Proto, Worker and Go boundary.

M1.7 intentionally does not yet provide analytic Jacobians, interpreted null-space
directions, minimal conflict sets, global discrete branch candidates or a drag
objective. Those are M2/M3 concerns. OCCT extraction and persistent topology
references remain in adapters, never in this solver library.

Build and run the focused scenarios with:

```sh
cmake --build build/cmake/debug --target occcad_assembly_solver_scenarios
ctest --test-dir build/cmake/debug -R '^assembly/' --output-on-failure
```

The executable M0 conformance corpus lives in
[`tests/assembly-corpus`](../../tests/assembly-corpus). It records canonical
freedoms, conflict/degeneracy baselines, permutation invariance, branch continuity
and cold/warm-start equivalence. Run it independently with:

```sh
cmake --build build/cmake/debug --target occcad_assembly_solver_corpus
ctest --test-dir build/cmake/debug -R '^assembly-corpus/' --output-on-failure
```
