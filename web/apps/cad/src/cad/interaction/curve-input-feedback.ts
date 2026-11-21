import type {GeometryKind, SelectionItem, Vec3} from '../../types';
import type {DisplayDocumentView} from '../visual/visual-repository';

export type FillBoundaryCheck = {
    kind: 'empty'|'unknown'|'open'|'closed'|'branch'|'multiple'; markers: Vec3[]
};

type EdgeEnds = [Vec3, Vec3];
function distance(a: Vec3, b: Vec3) {
    return Math.hypot(a[0] - b[0], a[1] - b[1], a[2] - b[2]);
}
function touching(a: Vec3, b: Vec3, tolerance: number) {
    return distance(a, b) <= tolerance ||
        a.every((x, k) => Math.abs(x - b[k]) <= Math.max(Math.abs(x), Math.abs(b[k])) * 2 ** -23);
}
export function inspectFillEndpoints(edges: readonly EdgeEnds[], tolerance = 1e-7): FillBoundaryCheck {
    if (!edges.length) return {kind: 'empty', markers: []};
    if (edges.length > 256) return {kind: 'unknown', markers: []};
    // Display endpoints are float32 in Part-local millimetres. This check
    // never changes the selected curves or the tolerance of the exact B-Rep.
    const ends = edges.flat();
    const mates = ends.map(() => [] as number[]);
    for (let i = 0; i < ends.length; i++) {
        for (let j = i + 1; j < ends.length; j++) {
            const a = ends[i], b = ends[j];
            if (touching(a, b, tolerance)) {
                mates[i].push(j);
                mates[j].push(i);
            }
        }
    }
    const branches = ends.filter((_, i) => mates[i].length > 1);
    if (branches.length) return {kind: 'branch', markers: branches};
    const free = ends.filter((_, i) => !mates[i].length);
    // Each connected component is now a chain or a cycle. A closed island
    // alongside an open chain must not be reported as a connected wire.
    const seen = new Set<number>();
    const stack = [0];
    while (stack.length) {
        const i = stack.pop()!;
        if (seen.has(i)) continue;
        seen.add(i);
        stack.push(i ^ 1, ...mates[i]);
    }
    if (seen.size !== ends.length) return {kind: 'multiple', markers: free};
    return {kind: free.length ? 'open' : 'closed', markers: free};
}

const inputKinds = [
    'SURFACE_FILL', 'SUPPORTED_FILL', 'SURFACE_BOUNDARY', 'COMPOSITE', 'SURFACE_BLEND',
    'G1_CONNECT', 'SURFACE_LOFT', 'SURFACE_SWEEP', 'SURFACE_EXTRUDE', 'SURFACE_REVOLVE',
] as const;
export type CurveInputKind = typeof inputKinds[number];
export function hasCurveInputFeedback(kind: GeometryKind): kind is Extract<GeometryKind, CurveInputKind> {
    return inputKinds.some(inputKind => inputKind === kind);
}
export type CurveInputMember = {
    selections: SelectionItem[]; reversed: boolean
};
export type CurveInputSession = {
    kind: CurveInputKind; documentId: string; versionId: string; members: (CurveInputMember|null)[];
    tolerance?: number; report: (pause: boolean) => void;
};
export type CurveMarkerKind = 'endpoint' | 'closed' | 'warning';
type CurveFeedback = { kind: CurveMarkerKind; markers: Vec3[]; pause: boolean };
function feedback(kind: CurveMarkerKind, markers: Vec3[] = [], pause = false): CurveFeedback {
    return {kind, markers, pause};
}
type Member = {
    edges: EdgeEnds[]; oriented: boolean
};
function displayMember(view: DisplayDocumentView, member: CurveInputMember): Member|undefined {
    const edges: EdgeEnds[] = [];
    let oriented = true;
    for (const s of member.selections) {
        if ((s.ownerDocumentId ?? s.documentId) !== view.document.id ||
            s.versionId !== view.document.versionId)
            return;
        let curves: Vec3[][]|undefined;
        if (s.kind === 'visual') {
            curves =
                view.sketchVisualization?.primitives
                    ?.filter(
                        p => p.featureId === s.featureId && (p.displayEntityId ?? p.id) === s.entityId &&
                            p.semantic === 'SKETCH_CURVE')
                    .map(p => p.positions);
        } else {
            const artifact = s.geometryKey ? view.artifacts?.[s.geometryKey] : undefined;
            if (s.kind === 'edge')
                curves = artifact?.mesh.edges?.filter(e => e.localId === s.topologyId).map(e => e.points);
            else if (s.result && artifact?.geometryDomain === 'WIRE_SET')
                curves = artifact.mesh.edges?.map(e => e.points);
            // A tessellated B-Rep edge has no oriented-use contract.
            oriented = false;
        }
        if (!curves?.length || curves.some(c => c.length < 2) || edges.length + curves.length > 256) return;
        for (const c of curves)
            edges.push(member.reversed ? [c[c.length - 1], c[0]] : [c[0], c[c.length - 1]]);
    }
    return edges.length ? {edges, oriented} : undefined;
}

export function inspectCurveInputs(
    kind: CurveInputKind, slots: (Member|null|undefined)[], tolerance?: number): CurveFeedback {
    const members = slots.filter((m): m is Member => !!m);
    if (!members.length || slots.includes(undefined)) return feedback('endpoint');
    const edges = members.flatMap(m => m.edges);
    const anchors = members.map(m => m.edges[0][0]);
    if (kind === 'SURFACE_BOUNDARY' || kind === 'SUPPORTED_FILL') {
        if (members.length > 4) return feedback('warning', anchors);
        for (let i = 0; i < 4; i++) {
            const m = slots[i];
            if (!m) continue;
            if (m.edges.length !== 1 || touching(...m.edges[0], 1e-7))
                return feedback('warning', [m.edges[0][0]]);
            const n = slots[(i + 1) % 4];
            if (n?.edges.length !== 1) continue;
            const a = m.edges[0], b = n.edges[0];
            const pairs: EdgeEnds[] = m.oriented && n.oriented ? [[a[1], b[0]]] :
                a.flatMap(x => b.map(y => [x, y] as EdgeEnds));
            const pair = pairs.reduce((p, q) => distance(...p) <= distance(...q) ? p : q);
            if (tolerance !== undefined && !touching(...pair, tolerance)) return feedback('warning', pair);
        }
        if (members.length < 4) return feedback('endpoint', members.at(-1)!.edges[0]);
        if (tolerance === undefined) return feedback('endpoint');
        const loop = inspectFillEndpoints(edges, tolerance);
        return loop.kind === 'closed' ? feedback('closed', [anchors[0]]) :
            feedback('warning', loop.markers.length ? loop.markers : anchors);
    }
    if (kind === 'SURFACE_FILL' || kind === 'COMPOSITE' || kind === 'SURFACE_EXTRUDE' || kind === 'SURFACE_REVOLVE') {
        const check = inspectFillEndpoints(edges);
        if (kind === 'COMPOSITE' && ['closed', 'open'].includes(check.kind) && members.every(m => m.edges.length === 1)) {
            const previous: Vec3[] = [];
            for (const edge of edges) {
                if (previous.length && !edge.some(p => previous.some(q => touching(p, q, 1e-7))))
                    return feedback('warning', edge);
                previous.push(...edge);
            }
        }
        if (check.kind === 'closed') return feedback('closed', [anchors[0]]);
        if (check.kind === 'open') return feedback('endpoint', check.markers, kind === 'SURFACE_FILL');
        if (check.kind === 'branch' || check.kind === 'multiple') {
            if (kind === 'SURFACE_EXTRUDE' || kind === 'SURFACE_REVOLVE') return feedback('endpoint', check.markers);
            return feedback('warning', check.markers.length ? check.markers : anchors, kind === 'SURFACE_FILL');
        }
        return feedback('endpoint');
    }
    if (kind === 'SURFACE_BLEND' || kind === 'G1_CONNECT') {
        for (const m of members) {
            if (m.edges.length !== 1 || (kind === 'SURFACE_BLEND' && touching(...m.edges[0], 1e-7)))
                return feedback('warning', [m.edges[0][0]]);
        }
        if (kind === 'G1_CONNECT' && members.length === 2 && members.every(m => m.oriented)) {
            const pair: EdgeEnds = [members[0].edges[0][1], members[1].edges[0][0]];
            return feedback(touching(...pair, 1e-7) ? 'warning' : 'endpoint', pair);
        }
        return feedback('endpoint', edges.flat());
    }
    const checks = members.map(m => inspectFillEndpoints(m.edges));
    if (checks.some(c => c.kind === 'unknown')) return feedback('endpoint');
    const bad = checks.findIndex(c => !['open', 'closed'].includes(c.kind));
    if (bad >= 0) return feedback('warning', checks[bad].markers.length ? checks[bad].markers : [anchors[bad]]);
    if (kind === 'SURFACE_LOFT') {
        const mismatch = checks.findIndex((c, i) => c.kind !== checks[0].kind || members[i].edges.length !== members[0].edges.length);
        if (mismatch >= 0) return feedback('warning', [anchors[mismatch]]);
    }
    const last = checks.at(-1)!;
    return last.kind === 'closed' ? feedback('closed', [anchors.at(-1)!]) : feedback('endpoint', last.markers);
}

export function displayCurveInputs(view: DisplayDocumentView, session: CurveInputSession): CurveFeedback {
    return inspectCurveInputs(
        session.kind, Array.from(session.members, m => m ? displayMember(view, m) : null),
        session.tolerance);
}
