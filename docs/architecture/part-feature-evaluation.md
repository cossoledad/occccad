# Part Feature evaluation and Sketch context

This page is the focused contract for Sketch-in-context display and the exact
Part Feature evaluation path. Current delivery status remains in
`docs/CURRENT_ARCHITECTURE.md`; long-term Feature semantics remain in
`docs/TARGET_ARCHITECTURE.md` section 5.4.

## Sketch editing is an in-context view

Entering Sketcher changes interaction, not the authoritative Part state. The
viewport composes three independent layers:

1. the latest evaluated Body artifact, visible as read-only design context;
2. the active Sketch overlay, grid, constraints and transient previews;
3. reference/environment helpers, including the world ground grid retained
   alongside the active support plane grid.

Visibility and selectability are separate. The Body remains rendered and may
participate in navigation/occlusion, while normal Sketch tools only select the
active Sketch entities and constraints. A later ExternalGeometry tool may
explicitly opt into upstream Face/Edge/Vertex capture; ordinary drawing tools
must not accidentally edit or persist a raw topology pick.

The context Body reuses the immutable artifact already attached to the opened
Revision. Sketcher does not create a second B-Rep, and display state never
becomes modeling truth. Sketch curves, points, dimensions and previews render
above coincident support faces so the visible Body does not hide editable
geometry.

## Feature evaluation pipeline

Every generator or modifying Feature follows one pipeline:

```text
typed Feature definition
  -> resolve parameters and stable references
  -> build Profile/ToolShape
  -> apply BodyOperation and collect generator/Boolean history
  -> same-domain normalization and compose its history
  -> B-Rep/body-policy validation
  -> semantic adjacency closure
  -> complete-shape naming gate
  -> immutable B-Rep/mesh/topology manifest
```

The Boolean result is not accepted merely because `IsDone()` is true. It must
be valid, non-empty, satisfy the single-Solid Body policy, and provide exactly
one semantic output and lineage result for every live Face, Edge and Vertex.

## Two-stage topology history

Primary history preserves the strongest evidence: same-type
`Modified/Generated/Deleted` mappings from the generator, Boolean builder and
`ShapeUpgrade_UnifySameDomain`. These mappings retain existing semantic refs
and produce explicit split, merge and tombstone records.

OCCT may legally create a final intersection Edge or Vertex only as the
boundary of modified higher-dimensional shapes, without returning a usable
same-type history entry. Rejecting every such result makes valid pockets and
future Features unusable. The second stage therefore closes only uncovered
final topology using already named semantic adjacency:

- an uncovered Face prefers its named boundary Edges;
- an uncovered Edge derives from its named incident Faces;
- an uncovered Vertex derives from its named incident Edges;
- a deterministic evidence order is used only to distinguish multiple results
  with the same stable source set; equal evidence remains an error.

Derived refs are owned by the current Feature and record the complete sorted
source-ref set in lineage. They never use an OCCT pointer, traversal/local ID,
mesh index, or coordinate alone as identity. The closure is versioned by the
topology evaluator/policy digest. If no stable adjacent sources exist, or two
candidates cannot be distinguished, evaluation still fails before artifact
adoption and Workspace CAS.

## Diagnostics and provenance

Successful closure adds `TOPOLOGY_HISTORY_DERIVED_CLOSURE:<count>` to the
Feature diagnostics. The complete-shape gate continues to reject missing,
duplicate, dangling, type-less and live/tombstone-conflicting outputs. Worker
errors must retain the Feature evaluation phase and stable code; callers must
not retry by dropping history or adopting unnamed geometry.

## Required corpus

The baseline covers rectangle, circle, arc, spline and multi-region profiles;
NEW_BODY, ADD and REMOVE; face-based boss, pocket, side-opening, through cut
and annular cut; same-domain merge/split/delete; Undo/Redo and cold evaluator
rebuild. Every kernel case compares semantic outputs with the final
Face/Edge/Vertex counts; representative chains repeat evaluation to verify
stable refs and history digest.

Viewport scenarios separately verify that direct-Part and occurrence-context
Sketcher keep the Body visible, keep non-active Sketch geometry filtered, and
keep Body topology non-selectable until an explicit reference-capture tool is
active. Browser/WebGL acceptance remains required for depth, occlusion and
camera behavior.
