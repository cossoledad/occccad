# Assembly geometric solver

`occccad_assembly_solver` is the first standalone algorithm slice for 3D assembly
constraints. It consumes rigid bodies, body-local geometric elements and geometric
constraints, then returns solved body poses and per-constraint residuals. The module
does not depend on OCCT, Product documents, topology naming, RPC or persistence.

## Model and conventions

- A body pose is an `SE(3)` transform mapping local coordinates to world coordinates
  as `R * p + t`. Solver updates are six-dimensional local increments.
- Geometry is an immutable value descriptor owned by one body: `Point`, `Axis`,
  `Plane` or `Cylinder`. An eventual Product adapter must resolve stable
  `InstancePath`/persistent selections into these descriptors before solving.
- Constraints refer to geometry by `(body_id, geometry_id)`. `Fix` refers to a body
  and preserves its initial pose unless an explicit target pose is supplied.
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

## Numerical implementation and extension boundary

The current implementation is a deterministic damped least-squares solve with a
finite-difference Jacobian. Each constraint owns a residual block, so adding a
geometry pair or constraint does not change the optimizer or the public result
contract. Validation errors and numerical failures are returned as statuses rather
than exceptions crossing the API.

This first slice intentionally does not classify degrees of freedom, redundancy or
conflicts; select discrete solution branches globally; expose drag goals; or connect
to the Product evaluator. The next numerical evolution should add analytic or
automatic Jacobians behind the residual-block boundary, graph decomposition and
rank diagnostics before assembly persistence is introduced. OCCT extraction and
persistent topology references belong in an adapter, never in this solver library.

Build and run the focused scenarios with:

```sh
cmake --build build/cmake/debug --target occcad_assembly_solver_scenarios
ctest --test-dir build/cmake/debug -R '^assembly/' --output-on-failure
```
