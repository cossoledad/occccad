import type {SelectionItem, Vec3} from "../../types";
import type {DisplayDocumentView} from "../visual/visual-repository";

export type FillBoundaryCheck = {kind: "empty"|"unknown"|"open"|"closed"|"branch"|"multiple"; markers: Vec3[]};

export function inspectFillEndpoints(edges: readonly [Vec3, Vec3][]): FillBoundaryCheck {
    if (!edges.length) return {kind: "empty", markers: []};
    if (edges.length > 256) return {kind: "unknown", markers: []};
    // Display endpoints are float32 in Part-local millimetres. This check
    // never changes the selected curves or the tolerance of the exact B-Rep.
    const ends = edges.flat();
    const mates = ends.map(() => [] as number[]);
    for (let i = 0; i < ends.length; i++) {
        for (let j = i + 1; j < ends.length; j++) {
            const a = ends[i], b = ends[j];
            if (a.every((x, k) => Math.abs(x-b[k]) <= Math.max(1e-7, Math.max(Math.abs(x),Math.abs(b[k])) * 2**-23))) {
                mates[i].push(j);
                mates[j].push(i);
            }
        }
    }
    const branches = ends.filter((_,i) => mates[i].length > 1);
    if (branches.length) return {kind: "branch", markers: branches};
    const free = ends.filter((_,i) => !mates[i].length);
    if (free.length) return {kind: "open", markers: free};
    let start = 0, count = 0;
    do {
        start = mates[start ^ 1][0];
        count++;
    } while (start !== 0);
    return {kind: count === edges.length ? "closed" : "multiple", markers: []};
}

export function displayFillBoundary(view: DisplayDocumentView, members: SelectionItem[][]): FillBoundaryCheck {
    const edges: [Vec3, Vec3][] = [];
    for (const selections of members) {
        if (!selections.length) return {kind: "unknown", markers: []};
        for (const s of selections) {
            if (s.documentId !== view.document.id || s.versionId !== view.document.versionId)
                return {kind: "unknown", markers: []};
            let curves: Vec3[][] | undefined;
            if (s.kind === "visual") {
                curves = view.sketchVisualization?.primitives?.filter(p => p.featureId === s.featureId &&
                    (p.displayEntityId ?? p.id) === s.entityId && p.semantic === "SKETCH_CURVE").map(p => p.positions);
            } else {
                const artifact = s.geometryKey ? view.artifacts?.[s.geometryKey] : undefined;
                if (s.kind === "edge") curves = artifact?.mesh.edges?.filter(e => e.localId === s.topologyId).map(e => e.points);
                else if (s.result && artifact?.geometryDomain === "WIRE_SET") curves = artifact.mesh.edges?.map(e => e.points);
            }
            if (!curves?.length || curves.some(c => c.length < 2)) return {kind: "unknown", markers: []};
            for (const curve of curves) edges.push([curve[0],curve[curve.length-1]]);
        }
    }
    return inspectFillEndpoints(edges);
}

export function fillBoundaryText(check: FillBoundaryCheck): string {
    switch (check.kind) {
        case "closed": return "填充边界已闭合（显示预判）";
        case "open": return `填充边界未闭合 · ${check.markers.length} 个自由端点`;
        case "empty": return "请选择填充边界";
        case "branch": return "填充边界有分叉或重复边";
        case "multiple": return "选中了多个闭环，请保留一个外边界";
        case "unknown": return "边界显示数据不足，等待精确预览";
    }
}
