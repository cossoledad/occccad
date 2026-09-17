# Persistent naming quick reference

> Canonical semantics: `TARGET_ARCHITECTURE.md` §5.7 and relevant §5.4/§5.6 contracts. Current facts: `CURRENT_ARCHITECTURE.md` §5.

Use this page for topology history, PersistentSelection, Product supporting elements, Reconnect and exact topology properties.

## Identity chain

```text
Feature / Body / profile entity stable IDs
  -> evaluator naming policy + digest
  -> OCCT Generated/Modified/Deleted evidence
  -> semantic topology outputs + TopologyHistory
  -> immutable topology manifest artifact + digest
  -> revision-local raw pick as creation evidence
  -> server-side PersistentSelection binding
  -> target Revision resolution
  -> Connected / NotConnected + ambiguity/tombstone evidence
```

- Persistent identity never uses OCCT pointer, traversal index, mesh primitive, coordinates alone or display name.
- A raw Face/Edge/Vertex local ID identifies only one geometry artifact and is accepted only as verified creation/reconnect evidence.
- Semantic outputs derive from stable Feature/Profile identities and actual OCCT history. Boolean/unify first preserves Generated/Modified/Unchanged/Split/Merged/Deleted evidence; a policy-versioned closure may name only otherwise-uncovered final intersection topology from already named semantic adjacency, without rewriting primary ancestry.
- Naming policy contains explicit length/angle tolerance and version digest. It participates in request, manifest and cache identity.
- Complete history covers every live final topology element exactly once and rejects conflicting live/tombstone or duplicate semantic/local identities.
- Ambiguity is a result, not permission to select the first candidate. Deleted topology remains a tombstone; repair is explicit Reconnect.

## Current boundary

Linear Extrude/Boolean has the first Face/Edge/Vertex end-to-end contract. Revolve, Import and other incomplete evaluators report `topology_history_complete=false`; downstream Product constraints must not treat them as safely persistent. Consult Current Architecture for the exact delivered matrix rather than copying it here.

## Source route

| Concern | Start here |
|---|---|
| OCCT-free topology contract | `kernel/api/include/occccad/kernel/topology_naming.hpp` |
| OCCT history generation/gates | search operation or semantic output in `kernel/occt/src/occt_kernel.cpp` |
| worker Proto manifest | `proto/occcad/worker/v1/geometry_worker.proto` |
| artifact adoption/digest checks | `services/internal/workspace/service.go` and geometry adapter symbols |
| binding and cross-Revision resolution | `services/internal/workspace/topology_selection.go` |
| Product supporting-element state | workspace assembly solve/structure projection and Web constraint UX |
| conformance | `kernel/occt/tests/geometry_exchange_scenarios.cpp`, workspace topology tests, control integration tests |

Do not start from generated protobuf or STEP corpus bytes. Begin with the stable semantic output or failing selection and expand to the exact OCCT/test range.

## Failure gates

- Geometry may be numerically identical while Feature identity differs; cache/naming digest must keep them distinct.
- Adopting an external manifest requires agreement between inline response, referenced immutable object and adopted digest.
- Resolver input freezes source/target Revision and policy evidence; resolving against “latest” silently is invalid.
- Reconnect stores a new stable binding through one Domain Command; it does not persist the raw local ID as long-term identity.
- Product/UI presents supporting-element connectivity separately from constraint solve status.

## Validation

Use an exact geometry/workspace test first, then:

```bash
invoke check --scope geometry
invoke check --scope workspace
```

Changes spanning Proto/Worker/Router/Product require `invoke check --scope all` and a test through the formal Router. Only read `models/` when the regression explicitly requires real STEP/BREP input.
