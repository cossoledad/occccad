# Three-dimensional assembly constraint solver evolution

## 1. Purpose and scope

This document defines the next evolution of `kernel/assembly` from the current
standalone numerical slice into a geometry-based three-dimensional assembly
constraint manager. CATIA Assembly Design is the reference for user-visible
engineering semantics and D-Cubed 3D DCM is a reference for constraint-manager
capabilities such as rigid sets, freedoms, diagnostics and partitioning. This does
not imply copying either product's UI, data format or private implementation.

The target is not merely a nonlinear optimizer with more residual kinds. A usable
CAD assembly solver must preserve design intent across edits, distinguish valid
under-constrained models from failures, explain conflicts and redundant
constraints, keep discrete solution branches stable, and solve only the affected
part of a large occurrence graph.

This document is subordinate to the platform invariants and Product/Assembly
domain model in [`docs/TARGET_ARCHITECTURE.md`](../../docs/TARGET_ARCHITECTURE.md).
It focuses on the internal solver architecture, implementation order and
verification gates.

## 2. Current verified baseline

The current module already provides a useful A1 numerical baseline:

- immutable value types for rigid bodies and body-local `Point`, `Axis`, `Plane`
  and `Cylinder` descriptors;
- `Fix`, `Rigid`, `Coincident`, `Concentric`, `Angle` and `Distance` constraints;
- explicit direction and signed-distance relation fields;
- right-handed `SE(3)` poses with local six-dimensional updates rather than
  unconstrained addition to quaternion coefficients;
- damped least-squares iteration and per-constraint normalized residuals;
- no dependency on OCCT, Product documents, persistence or RPC;
- a control-plane adapter that resolves direct Part datums and selected planar or
  cylindrical faces, and a coarse-grained `SolveAssembly` RPC routed through the
  formal Geometry Router path.

The existing implementation is nevertheless a single dense numerical problem:

- every body contributes six variables, including fixed bodies;
- `Fix` and `Rigid` are residuals rather than elimination and rigid clustering;
- the Jacobian is recomputed with forward finite differences over all variables;
- the dense normal equation `J^T J` is formed and solved as one system;
- unrelated constraint components are not separated;
- residual row count is not distinguished from independent equation rank;
- results contain convergence and residual information, but no component DOF,
  gauge freedom, redundancy, conflict set, degeneracy or branch candidates;
- constrained dragging is represented by an additional temporary `Fix`, making a
  user target a six-DOF hard constraint;
- Product input still uses direct `InstanceId` and revision-local topology IDs
  rather than typed `InstancePath`, immutable resolution snapshots and
  Publication/PersistentSelection contracts.

These limitations are expected for the first slice. They define the migration
boundary; they must not be hidden by adding more constraint enum values to the
current monolithic solver.

## 3. Lessons from CATIA and D-Cubed

### 3.1 Engineering connections are above scalar equations

CATIA represents an Engineering Connection as one or more assembly constraints
between components and guides the user toward compatible functional geometry
combinations. Connection types such as rigid, revolute or prismatic express
mechanical intent and remaining freedom; they are not aliases for a single
residual. CATIA also exposes constrained manipulation, constraint-network
diagnosis and flexible subassemblies.

occcad should therefore use this hierarchy:

```text
EngineeringConnection
  -> AssemblyConstraint
       -> compiled semantic equations
            -> backend residual and Jacobian blocks
```

For example, a declared revolute connection can compile to axis coincidence and
axial positioning constraints, but is valid only if the solved connection really
retains one rotational DOF. The declared type must be verified against the
Jacobian; it cannot be trusted as a UI label.

Public CATIA references:

- [Assembly Design capabilities](https://3dswym.3dexperience.3ds.com/wiki/catia-user-community/catia-assembly-design-2-asd_Lx2jW-oPR9qcb3gZNgrKhw)
- [Creating Engineering Connections](https://3dswym.3dexperience.3ds.com/post/3dexperience-edu-students/creating-assemblies-with-catia-3dexperience-r2022x_3AhyqEsmTOueqlaoNXes2A)
- [Constraints in a mechanism](https://help-3dexperience.aesvietnam.com/English/KimUserMap/kim-c-EngConnections-Constraints.htm)

### 3.2 Rigid sets, freedoms and partitions are first-class

D-Cubed 3D DCM publicly describes rigid sets representing parts, reporting the
remaining translational or rotational freedoms of an under-defined set, and
partitioning regions whose changes cannot affect each other. Its published design
discussion also emphasizes explicit support for known geometry types: geometric
special cases enable more predictable behavior, more applicable constraint
combinations and analytic methods that can be faster and more accurate than a
single general formulation.

Consequences for occccad are:

- rigid clustering and graph partitioning precede numeric optimization;
- under-constrained is a successful model state, not a convergence error;
- DOF must include directions and axes, not just a count;
- analytic geometry equations remain explicit rather than being reduced early to
  generic sampled surfaces or closest-distance functions;
- diagnostics must map back to application geometry, connections, constraints and
  individual semantic equations.

Public D-Cubed references:

- [3D DCM version 55: rigid-set freedoms, diagnostics and partitioning](https://blogs.sw.siemens.com/plm-components/d-cubed-3d-dcm-version-55-0/)
- [3D DCM version 56: enhanced partitioning](https://blogs.sw.siemens.com/plm-components/d-cubed-3d-dcm-version-56-0/)
- [3D DCM version 60: sets with transforms and under-defined DOF](https://blogs.sw.siemens.com/plm-components/d-cubed-3d-components-release-highlights/)
- [Why geometry-specific solving matters](https://blogs.sw.siemens.com/news/geometric-constraint-solving-its-all-about-special-cases/)
- [Bounded dimensions and inequality relationships](https://blogs.sw.siemens.com/plm-components/d-cubed-3d-dcm-v50-a-closer-look-at-bounded-constraints/)

## 4. Architectural principles

### 4.1 Separate domain semantics from the numerical backend

The public kernel API owns:

- stable connection, constraint, endpoint and equation identities;
- typed geometry descriptors and their symmetry;
- constraint mode, units, branch intent and limits;
- component DOF, conflict/redundancy and diagnostic contracts;
- solver profile and provenance.

A numeric backend owns only local variable blocks, residual/Jacobian evaluation,
linearization and nonlinear iteration. Ceres, Eigen or a future custom backend must
not leak types into Product models, Proto or persisted commands.

### 4.2 Use three distinct graphs

The architecture contains three related but non-interchangeable graphs:

| Graph | Nodes and edges | Responsibility |
|---|---|---|
| Product occurrence graph | Product references and typed instance paths | hierarchy, configuration, permissions and expansion |
| Constraint incidence graph | rigid clusters, constraints and semantic equations | components, grounding, DOF, conflicts and solve scope |
| Revision dependency graph | revisions, publications and geometry inputs | dirty closure, cache invalidation and replay |

Within one solve component, the incidence graph becomes a factor graph between
`SE(3)` pose variable blocks and equation blocks. Graph optimization here means
preserving and exploiting this block sparsity; it does not mean treating Product
structure as a pose graph or storing numerical factors as business state.

### 4.3 Residual rows are not independent DOF restrictions

An implementation may use a three-component cross product to enforce two
independent directional equations. Similarly, unit-vector difference residuals
contain structural dependence. Each compiled constraint must therefore declare:

- its semantic equation kinds;
- expected generic rank;
- geometry-dependent degeneracy conditions;
- physical units and normalization;
- branch variables;
- the mapping from every Jacobian row to
  `(connection_id, constraint_id, equation_id)`.

DOF and redundancy must use rank, not `variable_count - residual_count`.

### 4.4 Hard constraints and interaction objectives are different

Driving constraints define the feasible manifold. Measured constraints only
evaluate values. A drag target and preference for nominal poses are secondary
objectives and must not be encoded as infinite-weight or temporary hard
constraints.

Conceptually, constrained drag solves:

```text
minimize  drag_error(selected_pose, target)
          + nominal_change(other_poses)
subject to all active driving constraints
```

When the target is unreachable, the result should be the nearest feasible pose
plus the blocked directions, without weakening assembly constraints.

## 5. Recommended module boundaries

The existing library should evolve incrementally toward:

```text
kernel/assembly/
  api/                 descriptors, connections, constraints, result, diagnostics
  compile/             validation, capability matrix, equation compilation
  graph/               rigid clusters, components, grounding, affected subgraph
  equations/           geometry-specific residuals and Jacobians
  diagnostics/         rank, DOF basis, redundancy, conflicts, degeneracy
  solve/               initialization, candidates, continuation, orchestration
  backends/
    eigen-reference/   small deterministic reference solver
    ceres/             optional nonlinear backend after evaluation
```

This is a dependency direction, not a requirement to create every directory
immediately. New files should be extracted only when the corresponding boundary
has behavior and conformance tests.

## 6. Solver pipeline

### 6.1 Freeze an immutable solve manifest

Before entering the solver, the control plane resolves:

- root Product Revision and configuration/resolution snapshot;
- typed occurrence paths and occurrence-local nominal poses;
- Publication or PersistentSelection targets;
- immutable local geometry descriptors and symmetry;
- Engineering Connections and atomic constraints;
- tolerance profile, solver build, interaction target and deadline.

The Assembly Solver Worker receives this immutable manifest and never queries the
business database, follows a moving workspace head or reads a B-Rep directly.

### 6.2 Compile constraints into semantic equations

Compilation performs:

- endpoint and geometry-kind compatibility checks;
- units, finite-value, range and degeneracy validation;
- branch completeness validation;
- conversion to normalized equation blocks;
- analytic or automatic Jacobian binding;
- provenance assignment for every equation row.

Compatibility belongs in a server-authoritative capability registry shared by
command validation and solver compilation. The UI may use the registry for
guidance but is never the authority.

### 6.3 Collapse rigid clusters

Use rigid connections to union occurrences into clusters. Keep each member's
fixed transform relative to the cluster frame. A cluster contributes one six-DOF
pose; a grounded cluster contributes no free variable.

Constraints entirely inside one rigid cluster are still evaluated. A violated
internal constraint is an explainable `INTRA_RIGID_CONFLICT`, not an ignored
factor.

### 6.4 Build and partition the incidence graph

Build the bipartite cluster/constraint graph, remove suppressed and measured
constraints from the driving equation set, and find connected components. Only
components affected by an edit, drag target or explicit request are solved.
Independent components may run concurrently and can reuse their compiled graph
signature, sparsity pattern and warm start.

This partitioning must be implemented before attempting to solve the entire
assembly with a more sophisticated sparse library.

### 6.5 Grounding, gauge and structural analysis

For each component:

- eliminate grounded clusters from the variable vector;
- if there is no ground, report the six global rigid-body gauge freedoms
  separately;
- use structural matching to identify candidate under-, well- and
  over-constrained regions;
- use rank-revealing QR, with SVD for small suspect blocks, for numeric
  classification;
- use a versioned rank tolerance from `ToleranceProfile`.

The reported DOF is:

```text
remaining_dof = dimension(free tangent variables) - rank(active Jacobian)
```

The result also contains a null-space basis interpreted as translational
directions, rotational axes and coupled screw-like instantaneous motions.

### 6.6 Generate and preserve branch candidates

Known geometry combinations should provide analytic initialization candidates for
orientation, angle sector, contact side and axial direction. Candidate selection
uses this priority:

1. satisfy persisted branch intent;
2. preserve continuity with the preceding accepted solution;
3. minimize change from nominal poses;
4. return `AMBIGUOUS_BRANCH` if distinct candidates remain equivalent.

The solver must never silently select the first topology or geometric solution.

### 6.7 Refine on `SE(3)`

The first backend evolution can remain Eigen-based, but it must consume one
compiled component at a time and preserve block sparsity. Required improvements
are:

- ground and rigid-cluster elimination;
- analytic Jacobians for Point/Axis/Plane/Cylinder equations;
- finite differences retained as a Jacobian conformance oracle, not production
  evaluation;
- trust-region damping with physical residual checks;
- rank-revealing linear solves for small and rank-deficient components;
- sparse QR/Cholesky evaluation only after representative benchmarks;
- deadline, cancellation, finite-pose and resource checks.

Ceres is a reasonable optional backend experiment because it provides manifolds,
automatic differentiation and sparse nonlinear least-squares solvers. It is not a
constraint manager: branch selection, rigid clustering, CAD geometry semantics,
DOF and conflict explanations remain occccad responsibilities. Ceres also warns
that forming normal equations squares the condition number and recommends
analytic or automatic differentiation over numeric differentiation:

- [Ceres modeling and manifolds](https://ceres-solver.readthedocs.io/latest/nnls_modeling.html)
- [Ceres derivative guidance](https://ceres-solver.readthedocs.io/latest/modeling_faqs.html)
- [Ceres sparse and dense linear solvers](https://ceres-solver.readthedocs.io/latest/nnls_solving.html)

### 6.8 Validate and diagnose the result

Numerical convergence is necessary but insufficient. Post-solve validation checks:

- each constraint's physical length/angular error against its tolerance;
- connection limits and branch intent;
- finite, canonical rigid poses;
- declared Connection type against actual remaining DOF;
- rank changes caused by geometric degeneracy;
- conflict and redundant equation explanations.

The public status set must distinguish at least:

- `SOLVED_FULLY`;
- `SOLVED_UNDER_CONSTRAINED`;
- `REDUNDANT`;
- `CONFLICTING`;
- `BROKEN_REFERENCE`;
- `AMBIGUOUS_BRANCH`;
- `NON_CONVERGENT`;
- `LIMIT_VIOLATION`;
- `RESOURCE_LIMIT`.

Under-constrained is normally committable. Conflicting, ambiguous or broken input
must not modify the Workspace.

## 7. Diagnostics strategy

Initial diagnostics should be useful and deterministic before attempting globally
minimal explanations:

1. verify that the previous model solves;
2. when a new constraint fails, start from its connected graph neighborhood;
3. use incremental rank contribution to identify redundant equation blocks;
4. apply deletion filtering or a QuickXplain-style search to the small conflict
   neighborhood;
5. return a small explanatory set containing connection, constraint and equation
   IDs plus geometry evidence;
6. label the result as minimal only if minimality was actually verified.

Diagnostics are domain data, not an English string from the numeric backend. Each
diagnostic needs a stable code, severity, affected identities, numeric evidence,
tolerance/profile information and a localizable message key.

## 8. Delivery plan

### M0: corpus and baseline observability (implemented)

Create `tests/assembly-corpus/` before structural rewrites. Cover at least:

- one free body: 6 DOF;
- one fixed body: 0 DOF;
- plane coincidence: expected 3 remaining relative DOF;
- concentric axes: expected 2 remaining relative DOF;
- revolute: 1R;
- prismatic: 1T;
- cylindrical: 1R + 1T;
- two ungrounded bodies: relative freedoms plus six gauge freedoms;
- duplicate constraints;
- conflicting offsets and angles;
- exact and near-degenerate axes/planes;
- branch preservation after dimension edits;
- permutation invariance of body and constraint order;
- semantic equivalence of cold solve and valid warm start.

Golden checks compare satisfied invariants, DOF, branch and diagnostics within a
tolerance profile, not byte-identical poses.

The executable baseline is maintained in
[`tests/assembly-corpus`](../../tests/assembly-corpus). Freedom cases retain
canonical feasible and blocked rigid motions as black-box conformance checks and
also assert the M1 numeric rank, relative DOF, gauge and structured classification.

### M1: graph kernel and result contract (implemented baseline)

The baseline now provides:

- introduce stable connection/equation IDs and constraint modes;
- build rigid clusters and connected components;
- eliminate grounded variables and report gauge freedoms;
- solve only affected components;
- extend results with component DOF, redundancy/conflict fields and structured
  diagnostics;
- keep the present numerical method as a reference backend.

The current rank and whole-constraint redundancy analysis still use the
finite-difference reference Jacobian. Analytic equations, interpreted null-space
bases and localized/minimal conflict explanations remain M2/M3 work; complex
surface constraints were deliberately not added in M1.

### M1.5: request-scoped moving/reference intent (implemented baseline)

Selection order is an interaction intent, not an intrinsic property of a geometric
constraint. For a binary constraint the Product adapter sends the first selected
occurrence as `moving` and the second as `reference`, using the request-scoped
`MoveFirstMinimizeReference` policy. The persisted constraint, equation identities
and residuals remain symmetric; replay does not permanently turn one occurrence
into a driver and the other into a dependent object.

For an ungrounded connected component the six global rigid motions are a gauge, so
M1.5 deterministically removes the selected reference cluster from the numerical
variables and holds its nominal pose exactly. The first selection therefore moves
to satisfy the constraint while the second stays unchanged, without synthesizing a
physical `Fix`. Result reporting still uses the logical pre-gauge variable count
and reports the component's six gauge DOF.

This is deliberately narrower than a general priority solver. If a component is
already physically grounded but retains several feasible internal motions, the
finite-difference backend still chooses its ordinary minimum-step update. Strict
lexicographic minimization of reference motion, weighted body groups and drag
projection require null-space/hierarchical optimization and remain M3 work. The
`Driving`/`Measured`/`Controlled`/`Suppressed` mode remains orthogonal: mode decides
whether an equation drives the solve, while solve intent decides how equivalent
pose solutions are distributed.

### M2: equation registry and Jacobians

- replace stringly typed worker kinds with typed definitions;
- implement a geometry-pair capability table;
- define independent semantic equations and generic rank;
- add analytic Jacobians for the existing descriptor matrix;
- verify them against central finite differences;
- add rank-revealing QR/SVD diagnostics.

### M3: branch continuity and constrained interaction

- persist discrete branch intent;
- generate multiple analytic initialization candidates;
- use nominal pose as a secondary objective;
- expose DOF directions and axes;
- project drag targets onto the feasible manifold;
- add warm starts and incremental graph recompilation;
- return small conflict and redundancy explanations.

### M4: stable Product input adapter

- typed `InstancePath` and immutable `ResolutionSnapshot`;
- Publication and PersistentSelection resolution;
- descriptor symmetry and provenance;
- nested Product occurrences;
- rigid and flexible subassembly expansion policies.

This is required for replayable real-world assemblies even if the numeric kernel
is already strong.

### M5: constraint and connection coverage

Add capabilities in this order, gated by equation, DOF and diagnostic corpus:

1. `Offset`, `Parallel`, `Perpendicular`;
2. analytic `Contact` subsets and Frame/Connector geometry;
3. rigid, revolute, prismatic, cylindrical and planar Connections;
4. Sphere, Cone and Circle descriptors;
5. bounded distance/angle and joint limits;
6. spherical, universal and screw Connections;
7. only then evaluate general curve/surface contact, gear and rack relationships.

Arbitrary NURBS-to-NURBS contact is not a near-term positioning primitive. Datum
and Connector Publications should express stable engineering intent first.

## 9. First vertical slice and acceptance criteria

The first mergeable slice should retain the current UI constraint set while adding
rigid clustering, graph components, ground elimination and component-level DOF.

It is complete only when:

- unrelated constraint components compile and solve independently;
- a fixed cluster contributes no numeric variable;
- rigid-connected bodies contribute one cluster pose;
- an ungrounded component reports six gauge freedoms without treating them as a
  conflict;
- the corpus reports revolute as `1R`, prismatic as `1T` and cylindrical as
  `1R + 1T` when expressed by supported primitive constraints;
- duplicate equations are reported as redundant with stable IDs;
- order permutations produce semantically equivalent results;
- Product create/edit/delete, atomic pose submission, Undo/Redo and formal Router
  integration continue to pass;
- solver provenance includes build and tolerance-profile identities.

## 10. Explicit non-goals for the next milestone

- copying CATIA UI or proprietary formats;
- treating Ceres or another optimizer as the domain architecture;
- arbitrary freeform surface contact;
- mechanism time simulation, collision dynamics or force propagation;
- distributing one connected constraint component across RPC calls;
- persisting OCCT local topology IDs as stable assembly identity;
- claiming CATIA-level capability from visual placement alone.

The immediate objective is a small, explainable and well-tested constraint-manager
core. Broader geometry and mechanism coverage should be built on that foundation.
