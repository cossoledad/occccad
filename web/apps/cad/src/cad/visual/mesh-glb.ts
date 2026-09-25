import type { MeshData, VisualizationManifest, Vec3 } from "../../types";
export type DecodedVisual = {
    mesh: MeshData;
    visualization?: VisualizationManifest;
};
const emptyMesh = (): MeshData => ({ vertices: [], triangles: [], faceIds: [], edges: [], topologyVertices: [], stableIds: {} });
export function decodeMeshGLB(buffer: ArrayBuffer): DecodedVisual {
    const data = new DataView(buffer);
    if (buffer.byteLength < 20 || data.getUint32(0, true) !== 0x46546c67 || data.getUint32(4, true) !== 2 || data.getUint32(8, true) !== buffer.byteLength)
        throw new Error("Invalid GLB header");
    let document: any, binary: DataView | undefined;
    for (let offset = 12; offset < buffer.byteLength;) {
        if (offset + 8 > buffer.byteLength)
            throw new Error("Invalid GLB chunk header");
        const length = data.getUint32(offset, true), kind = data.getUint32(offset + 4, true);
        offset += 8;
        if (length % 4 || offset + length > buffer.byteLength)
            throw new Error("Invalid GLB chunk length");
        if (kind === 0x4e4f534a)
            document = JSON.parse(new TextDecoder().decode(new Uint8Array(buffer, offset, length)));
        if (kind === 0x004e4942)
            binary = new DataView(buffer, offset, length);
        offset += length;
    }
    if (document?.asset?.version !== "2.0")
        throw new Error("Unsupported GLB asset");
    const mesh = emptyMesh(), visualization = document.extensions?.OCCCCAD_visualization;
    if (!document.meshes?.length)
        return { mesh, visualization };
    const cad = document.extensions?.OCCCCAD_cad;
    if (cad?.schemaVersion !== 1 || cad.units !== "mm" || cad.coordinateSpace !== "PART_LOCAL" || !binary)
        throw new Error("Missing CAD GLB mapping");
    const read = (index: number, components: number, type: number): number[] => {
        const a = document.accessors?.[index], v = document.bufferViews?.[a?.bufferView];
        if (!a || !v || a.componentType !== type || a.type !== (components === 3 ? "VEC3" : "SCALAR") || v.buffer !== 0 || v.byteStride)
            throw new Error("Unsupported GLB accessor");
        const start = (v.byteOffset ?? 0) + (a.byteOffset ?? 0), length = a.count * components * 4;
        if (!Number.isSafeInteger(start) || !Number.isSafeInteger(v.byteLength) || v.byteLength < 0 || !Number.isSafeInteger(a.byteOffset ?? 0) || (a.byteOffset ?? 0) < 0 || !Number.isSafeInteger(a.count) || a.count < 0 || !Number.isSafeInteger(length) || start < 0 || length > (v.byteLength - (a.byteOffset ?? 0)) || start + length > binary!.byteLength)
            throw new Error("Invalid GLB accessor bounds");
        return Array.from({ length: a.count * components }, (_, i) => {
            const n = type === 5126 ? binary!.getFloat32(start + i * 4, true) : binary!.getUint32(start + i * 4, true);
            if (!Number.isFinite(n))
                throw new Error("Nonfinite GLB coordinate");
            return n;
        });
    };
    const vectors = (values: number[]): Vec3[] => Array.from({ length: values.length / 3 }, (_, i) => [values[i * 3], values[i * 3 + 1], values[i * 3 + 2]]);
    const primitives = document.meshes[0].primitives;
    if (primitives?.length !== 1)
        throw new Error("Invalid solid primitive count");
    mesh.vertices = vectors(read(primitives[0].attributes.POSITION, 3, 5126));
    const indices = read(primitives[0].indices, 1, 5125);
    if (indices.length % 3 || indices.some(id => id >= mesh.vertices.length))
        throw new Error("Invalid triangle index");
    mesh.triangles = vectors(indices);
    mesh.faceIds = read(cad.faceIds, 1, 5125);
    if (mesh.faceIds.length !== mesh.triangles.length)
        throw new Error("Invalid face mapping count");
    if (!Array.isArray(cad.edges) || !Array.isArray(cad.vertices))
        throw new Error("Invalid CAD topology mapping");
    const validId = (id: unknown) => typeof id === "number" && Number.isSafeInteger(id) && id > 0;
    mesh.edges = cad.edges.map((e: any) => { if (!validId(e.localId))
        throw new Error("Invalid CAD edge ID"); return { localId: e.localId, points: vectors(read(e.positions, 3, 5126)) }; });
    if (cad.vertices.some((v: any) => !validId(v.localId) || !Array.isArray(v.point) || v.point.length !== 3 || v.point.some((n: unknown) => typeof n !== "number" || !Number.isFinite(n))))
        throw new Error("Invalid CAD vertex mapping");
    mesh.topologyVertices = cad.vertices;
    mesh.stableIds = cad.stableIds ?? {};
    return { mesh, visualization };
}
// Mock/corpus authoring only. Production GLBs are emitted by the Geometry Worker.
export function encodeMeshGLB(mesh: MeshData, visualization?: VisualizationManifest): ArrayBuffer {
    const chunks: Uint8Array[] = [], views: any[] = [], accessors: any[] = [];
    let offset = 0;
    const add = (values: number[], components: number, float: boolean) => {
        const buffer = new ArrayBuffer(values.length * 4), view = new DataView(buffer);
        values.forEach((n, i) => float ? view.setFloat32(i * 4, n, true) : view.setUint32(i * 4, n, true));
        const index = accessors.length;
        views.push({ buffer: 0, byteOffset: offset, byteLength: buffer.byteLength });
        offset += buffer.byteLength;
        chunks.push(new Uint8Array(buffer));
        const bounds = float && components === 3 && values.length ? { min: [0, 1, 2].map(axis => Math.min(...values.filter((_, i) => i % 3 === axis))), max: [0, 1, 2].map(axis => Math.max(...values.filter((_, i) => i % 3 === axis))) } : {};
        accessors.push({ bufferView: index, componentType: float ? 5126 : 5125, count: values.length / components, type: components === 3 ? "VEC3" : "SCALAR", ...bounds });
        return index;
    };
    const position = add(mesh.vertices.flat(), 3, true), indices = add(mesh.triangles.flat(), 1, false), faceIds = add(mesh.faceIds, 1, false);
    const cad = { schemaVersion: 1, units: "mm", coordinateSpace: "PART_LOCAL", faceIds, edges: (mesh.edges ?? []).map(e => ({ localId: e.localId, positions: add(e.points.flat(), 3, true) })), vertices: mesh.topologyVertices ?? [], stableIds: mesh.stableIds ?? {} };
    const document = { asset: { version: "2.0" }, buffers: [{ byteLength: offset }], bufferViews: views, accessors, meshes: mesh.triangles.length ? [{ primitives: [{ attributes: { POSITION: position }, indices }] }] : [], nodes: [{ mesh: 0 }], scenes: [{ nodes: [0] }], scene: 0, extensionsUsed: ["OCCCCAD_cad", "OCCCCAD_visualization"], extensions: { OCCCCAD_cad: cad, OCCCCAD_visualization: visualization } };
    let json = new TextEncoder().encode(JSON.stringify(document));
    const padded = new Uint8Array(Math.ceil(json.length / 4) * 4).fill(32);
    padded.set(json);
    json = padded;
    const output = new ArrayBuffer(28 + json.length + offset), header = new DataView(output), bytes = new Uint8Array(output);
    header.setUint32(0, 0x46546c67, true);
    header.setUint32(4, 2, true);
    header.setUint32(8, output.byteLength, true);
    header.setUint32(12, json.length, true);
    header.setUint32(16, 0x4e4f534a, true);
    bytes.set(json, 20);
    header.setUint32(20 + json.length, offset, true);
    header.setUint32(24 + json.length, 0x004e4942, true);
    let cursor = 28 + json.length;
    for (const chunk of chunks) {
        bytes.set(chunk, cursor);
        cursor += chunk.length;
    }
    return output;
}
