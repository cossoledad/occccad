import assert from "node:assert/strict";
import { assemblyGeometryRef } from "../../../cad/assembly/assembly-reference.ts";

const publication = {
  id: "publication-mount-plane", name: "Mount Plane", type: "PLANE", compatibilityVersion: "1.0.0",
  target: { kind: "DATUM", datumId: "datum-replacement" },
  contract: { geometryKind: "PLANE", symmetry: "NORMAL_UNORIENTED" },
  resolution: { status: "CONNECTED", resolvedVersionId: "part-r2" },
};
const reference = assemblyGeometryRef({ kind: "plane", id: "occurrence:datum-replacement", instanceId: "component-a",
  entityId: "datum-replacement", plane: "CUSTOM", publicationId: publication.id, publication });
assert.deepEqual(reference, { instanceId: "component-a", kind: "PLANE", geometryId: "datum-replacement",
  publicationRef: { publicationId: publication.id, expectedType: "PLANE", compatibilityVersion: "1.0.0",
    persistentSelection: undefined } });

console.log("assembly Publication scenario passed");
