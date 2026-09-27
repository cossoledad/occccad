import type { MeshData, VisualizationManifest, Vec3 } from "../../types";
export type DecodedVisual = {
    mesh: MeshData;
    association?: { geometryId?: string; namingDigest?: string };
    visualization?: VisualizationManifest;
};
const emptyMesh = (): MeshData => ({ vertices: [], triangles: [], faceIds: [], edges: [], topologyVertices: [] });
export function decodeMeshGLB(buffer: ArrayBuffer): DecodedVisual {
    const data = new DataView(buffer);
    if (buffer.byteLength < 20 || data.getUint32(0, true) !== 0x46546c67 || data.getUint32(4, true) !== 2 || data.getUint32(8, true) !== buffer.byteLength)
        throw new Error("Invalid GLB header");
    let document: any, binary: DataView | undefined;
    for (let offset = 12;offset < buffer.byteLength;) {
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
    if ((document.extensionsRequired ?? []).some((name: string) => !["OCCCCAD_cad", "OCCCCAD_visualization"].includes(name))) throw new Error("Unsupported required GLB extension");
    const mesh = emptyMesh(), visualization = document.extensions?.OCCCCAD_visualization;
    const cad = document.extensions?.OCCCCAD_cad;
    if (cad?.schemaVersion !== 2 || cad.units !== "mm" || cad.coordinateSpace !== "PART_LOCAL")
        throw new Error("Missing CAD GLB mapping");
    const read = (index: number, components: number, type: number): number[] => {
        if (!binary) throw new Error("Missing GLB binary");
        const a = document.accessors?.[index], v = document.bufferViews?.[a?.bufferView];
        if (!a || !v || a.componentType !== type || a.type !== (components === 3 ? "VEC3" : "SCALAR") || v.buffer !== 0 || v.byteStride)
            throw new Error("Unsupported GLB accessor");
        const start = (v.byteOffset ?? 0) + (a.byteOffset ?? 0), length = a.count * components * 4;
        if (!Number.isSafeInteger(start) || !Number.isSafeInteger(v.byteLength) || v.byteLength < 0 || !Number.isSafeInteger(a.byteOffset ?? 0) || (a.byteOffset ?? 0) < 0 || !Number.isSafeInteger(a.count) || a.count < 0 || !Number.isSafeInteger(length) || start < 0 || length > (v.byteLength - (a.byteOffset ?? 0)) || (v.byteOffset ?? 0) < 0 || (v.byteOffset ?? 0) + v.byteLength > binary!.byteLength || start + length > binary!.byteLength)
            throw new Error("Invalid GLB accessor bounds");
        return Array.from({ length: a.count * components }, (_, i) => {
            const n = type === 5126 ? binary!.getFloat32(start + i * 4, true) : binary!.getUint32(start + i * 4, true);
            if (!Number.isFinite(n))
                throw new Error("Nonfinite GLB coordinate");
            return n;
        });
    };
    const vectors = (values: number[]): Vec3[] => Array.from({ length: values.length / 3 }, (_, i) => [values[i * 3], values[i * 3 + 1], values[i * 3 + 2]]);
    let completeNormals = true;
    const normals: Vec3[] = [];
    for (const source of document.meshes ?? []) for (const p of source.primitives ?? []) {
        const mapping = p.extensions?.OCCCCAD_cad;
        if (!mapping) continue;
        const positions = vectors(read(p.attributes.POSITION, 3, 5126));
        const indices = p.indices === undefined ? positions.map((_, i) => i) : read(p.indices, 1, 5125);
        const ids = read(mapping.localIds, 1, 5125), mode = p.mode ?? 4;
        if (indices.some(i => i >= positions.length) || ids.some(id => !id)) throw new Error("Invalid primitive index or locator");
        if (mapping.kind === "FACE") {
            if (mode !== 4 || indices.length % 3 || ids.length !== indices.length / 3) throw new Error("Invalid face mapping count");
            if (p.attributes.NORMAL !== undefined) {
                const values = vectors(read(p.attributes.NORMAL, 3, 5126));
                if (values.length !== positions.length) throw new Error("Invalid normal count");
                for (const n of values) normals.push(n);
            } else completeNormals = false;
            const base = mesh.vertices.length;
            for (const v of positions) mesh.vertices.push(v);
            for (const t of vectors(indices.map(i => i + base))) mesh.triangles.push(t);
            for (const id of ids) mesh.faceIds.push(id);
        } else if (mapping.kind === "EDGE") {
            if (![1, 3].includes(mode) || (mode === 1 && indices.length % 2) || ids.length !== (mode === 3 ? indices.length - 1 : indices.length / 2)) throw new Error("Invalid edge mapping");
            for (let i = 0;i < ids.length;i++) {
                const j = mode === 3 ? i : i * 2;
                const previous = mesh.edges!.at(-1);
                if (i > 0 && previous?.localId === ids[i] && previous.points.at(-1)!.every((v, axis) => v === positions[indices[j]][axis])) previous.points.push(positions[indices[j + 1]]);
                else mesh.edges!.push({ localId: ids[i], points: [positions[indices[j]], positions[indices[j + 1]]] });
            }
        } else if (mapping.kind === "VERTEX") {
            if (mode !== 0 || ids.length !== indices.length) throw new Error("Invalid vertex mapping");
            ids.forEach((id, i) => mesh.topologyVertices!.push({ localId: id, point: positions[indices[i]] }));
        } else throw new Error("Unknown topology kind");
    }
    if (completeNormals && normals.length) mesh.normals = normals;
    if (visualization) {
        if (visualization.schemaVersion !== 2) throw new Error("Unsupported visualization schema");
        for (const p of visualization.primitives ?? []) {
            p.positions = p.positions === undefined ? [] : vectors(read(p.positions, 3, 5126));
            if (p.indices !== undefined) p.indices = read(p.indices, 1, 5125);
        }
    }
    return { mesh, visualization, association: cad.association };
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
        const bounds: { min?: number[]; max?: number[] } = {};
        if (float && components === 3 && values.length) { bounds.min = values.slice(0, 3); bounds.max = values.slice(0, 3); values.forEach((v, i) => { bounds.min![i % 3] = Math.min(bounds.min![i % 3], v); bounds.max![i % 3] = Math.max(bounds.max![i % 3], v) }); }
        accessors.push({ bufferView: index, componentType: float ? 5126 : 5125, count: values.length / components, type: components === 3 ? "VEC3" : "SCALAR", ...bounds });
        return index;
    };
    const primitives: any[] = [];
    const primitive = (positions: Vec3[], mode: number, kind: string, ids: number[], indices?: number[]) => {
        const p: any = { attributes: { POSITION: add(positions.flat(), 3, true) }, mode, extensions: { OCCCCAD_cad: { kind, localIds: add(ids, 1, false) } } };
        if (indices) p.indices = add(indices, 1, false);
        if (kind === "FACE" && mesh.normals) p.attributes.NORMAL = add(mesh.normals.flat(), 3, true);
        primitives.push(p);
    };
    if (mesh.triangles.length) primitive(mesh.vertices, 4, "FACE", mesh.faceIds, mesh.triangles.flat());
    for (const e of mesh.edges ?? []) if (e.points.length > 1) primitive(e.points, 3, "EDGE", e.points.slice(1).map(() => e.localId));
    if (mesh.topologyVertices?.length) primitive(mesh.topologyVertices.map(v => v.point), 0, "VERTEX", mesh.topologyVertices.map(v => v.localId));
    const display = visualization ? { ...visualization, schemaVersion: 2, primitives: (visualization.primitives ?? []).map(p => ({ ...p, positions: add(p.positions.flat(), 3, true), indices: p.indices ? add(p.indices, 1, false) : undefined })) } : undefined;
    const cad = { schemaVersion: 2, units: "mm", coordinateSpace: "PART_LOCAL" };
    const document: any = { asset: { version: "2.0" }, buffers: [{ byteLength: offset }], bufferViews: views, accessors, meshes: primitives.length ? [{ primitives }] : [], nodes: primitives.length ? [{ mesh: 0 }] : [], scenes: [{ nodes: primitives.length ? [0] : [] }], scene: 0, extensionsUsed: ["OCCCCAD_cad", "OCCCCAD_visualization"], extensions: { OCCCCAD_cad: cad, OCCCCAD_visualization: display } };
    if (!offset) delete document.buffers;
    for (const key of ["meshes", "nodes", "accessors", "bufferViews"]) if (!document[key].length) delete document[key];
    if (!document.nodes) document.scenes = [{}];
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
