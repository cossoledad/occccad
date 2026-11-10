import { packedMesh, type MeshBuffers } from "./packed-mesh";
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
    const read = (index: number, components: number, type: number): Float32Array | Uint32Array => {
        if (!binary) throw new Error("Missing GLB binary");
        const a = document.accessors?.[index], v = document.bufferViews?.[a?.bufferView];
        if (!a || !v || a.componentType !== type || a.type !== (components === 3 ? "VEC3" : "SCALAR") || v.buffer !== 0 || v.byteStride)
            throw new Error("Unsupported GLB accessor");
        const start = (v.byteOffset ?? 0) + (a.byteOffset ?? 0), length = a.count * components * 4;
        if (!Number.isSafeInteger(start) || !Number.isSafeInteger(v.byteLength) || v.byteLength < 0 || !Number.isSafeInteger(a.byteOffset ?? 0) || (a.byteOffset ?? 0) < 0 || !Number.isSafeInteger(a.count) || a.count < 0 || !Number.isSafeInteger(length) || start < 0 || length > (v.byteLength - (a.byteOffset ?? 0)) || (v.byteOffset ?? 0) < 0 || (v.byteOffset ?? 0) + v.byteLength > binary!.byteLength || start + length > binary!.byteLength)
            throw new Error("Invalid GLB accessor bounds");
        if ((binary.byteOffset + start) % 4) throw new Error("Unaligned GLB accessor");
        const values = type === 5126 ? new Float32Array(buffer, binary.byteOffset + start, a.count * components) : new Uint32Array(buffer, binary.byteOffset + start, a.count * components);
        if (type === 5126 && values.some(n => !Number.isFinite(n))) throw new Error("Nonfinite GLB coordinate");
        return values;
    };
    const vectors = (values: ArrayLike<number>): Vec3[] => Array.from({ length: values.length / 3 }, (_, i) => [values[i * 3], values[i * 3 + 1], values[i * 3 + 2]]);
    let completeNormals = true;
    const faces: MeshBuffers[] = [];
    for (const source of document.meshes ?? []) for (const p of source.primitives ?? []) {
        const mapping = p.extensions?.OCCCCAD_cad;
        if (!mapping) continue;
        const positions = read(p.attributes.POSITION, 3, 5126) as Float32Array;
        const vertexCount = positions.length / 3;
        const indices = p.indices === undefined ? Uint32Array.from({length: vertexCount}, (_, i) => i) : read(p.indices, 1, 5125) as Uint32Array;
        const ids = read(mapping.localIds, 1, 5125), mode = p.mode ?? 4;
        if (indices.some(i => i >= vertexCount) || ids.some(id => !id)) throw new Error("Invalid primitive index or locator");
        if (mapping.kind === "FACE") {
            if (mode !== 4 || indices.length % 3 || ids.length !== indices.length / 3) throw new Error("Invalid face mapping count");
            let normals: Float32Array | undefined;
            if (p.attributes.NORMAL !== undefined) {
                normals = read(p.attributes.NORMAL, 3, 5126) as Float32Array;
                if (normals.length !== positions.length) throw new Error("Invalid normal count");
            } else completeNormals = false;
            faces.push({positions, indices, normals, faceIds: ids as Uint32Array});
        } else if (mapping.kind === "EDGE") {
            if (![1, 3].includes(mode) || (mode === 1 && indices.length % 2) || ids.length !== (mode === 3 ? indices.length - 1 : indices.length / 2)) throw new Error("Invalid edge mapping");
            const points = vectors(positions);
            for (let i = 0;i < ids.length;i++) {
                const j = mode === 3 ? i : i * 2;
                const previous = mesh.edges!.at(-1);
                if (i > 0 && previous?.localId === ids[i] && previous.points.at(-1)!.every((v, axis) => v === points[indices[j]][axis])) previous.points.push(points[indices[j + 1]]);
                else mesh.edges!.push({ localId: ids[i], points: [points[indices[j]], points[indices[j + 1]]] });
            }
        } else if (mapping.kind === "VERTEX") {
            if (mode !== 0 || ids.length !== indices.length) throw new Error("Invalid vertex mapping");
            const points = vectors(positions);
            ids.forEach((id, i) => mesh.topologyVertices!.push({ localId: id, point: points[indices[i]] }));
        } else throw new Error("Unknown topology kind");
    }
    let buffers: MeshBuffers;
    if (faces.length === 1) buffers = {...faces[0], normals: completeNormals ? faces[0].normals : undefined};
    else {
        const vertices = faces.reduce((n, f) => n + f.positions.length, 0), indices = faces.reduce((n, f) => n + f.indices.length, 0);
        buffers = {positions: new Float32Array(vertices), indices: new Uint32Array(indices), faceIds: new Uint32Array(indices / 3), normals: completeNormals && vertices ? new Float32Array(vertices) : undefined};
        let vp = 0, ip = 0;
        for (const face of faces) {
            buffers.positions.set(face.positions, vp);
            buffers.indices.set(face.indices, ip);
            if (vp) for (let i = ip; i < ip + face.indices.length; i++) buffers.indices[i] += vp / 3;
            buffers.faceIds.set(face.faceIds, ip / 3);
            if (buffers.normals) buffers.normals.set(face.normals!, vp);
            vp += face.positions.length; ip += face.indices.length;
        }
    }
    if (visualization) {
        if (visualization.schemaVersion !== 2) throw new Error("Unsupported visualization schema");
        for (const p of visualization.primitives ?? []) {
            p.positions = p.positions === undefined ? [] : vectors(read(p.positions, 3, 5126));
            if (p.indices !== undefined) p.indices = Array.from(read(p.indices, 1, 5125));
        }
    }
    return { mesh: packedMesh(buffers, mesh.edges, mesh.topologyVertices), visualization, association: cad.association };
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
