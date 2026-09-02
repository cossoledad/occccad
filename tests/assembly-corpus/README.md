# Assembly conformance corpus

This directory contains the executable M0 baseline for the three-dimensional
assembly solver. `assembly_corpus.cpp` owns canonical input models and their
semantic expectations; `assembly_corpus_test.cpp` runs the same cases against the
current backend.

The initial corpus intentionally tests the public solver as a black box. For a
canonical solved configuration it applies small rigid motions that should either
remain on the feasible manifold or be rejected and corrected. This verifies the
expected freedoms without adding a test-only Jacobian or prematurely exposing the
M1 DOF result contract.

The corpus currently covers:

- free and fixed rigid bodies;
- plane coincidence, cylindrical, revolute and prismatic motion families;
- an ungrounded component and its global rigid-body gauge;
- duplicate, conflicting and degenerate inputs;
- body/geometry/constraint order permutation;
- cold solve versus a previous accepted pose used as a warm start;
- explicit plane-side preservation after a dimension edit.

Freedom cases now assert both black-box feasible motions and the component's M1
Jacobian rank, relative DOF and gauge DOF. Classification cases assert structured
`REDUNDANT`, `INCONSISTENT` and invalid-model outcomes while retaining the lower
level numerical status. Focused M1 scenarios additionally cover rigid-cluster
ground elimination, affected-component solving, constraint modes and stable
equation provenance. M1.5 scenarios verify that first-selection moving and
second-selection reference roles remain disjoint and that an ungrounded reference
cluster is used as a gauge anchor without losing the component's six reported
gauge freedoms. M1.6 scenarios additionally distinguish a proven stationary or
grounded `UNSATISFIED` result from iteration-budget `NON_CONVERGENT`, and cover
frozen direction/side branches, independent physical tolerances, exact zero/pi
angles and the near-parallel axis-distance limit.

Run only this corpus with:

```sh
cmake --build build/cmake/debug --target occcad_assembly_solver_corpus
ctest --test-dir build/cmake/debug -R '^assembly-corpus/' --output-on-failure
```
