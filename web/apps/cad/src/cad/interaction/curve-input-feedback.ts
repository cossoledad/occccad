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

const inputNames = {
    SURFACE_FILL: '填充边界',
    SUPPORTED_FILL: '带支撑填充',
    SURFACE_BOUNDARY: '边界曲面',
    COMPOSITE: '复合曲线',
    SURFACE_BLEND: '桥接边界',
    G1_CONNECT: 'G1 连接',
    SURFACE_LOFT: '放样截面',
    SURFACE_SWEEP: '扫掠',
    SURFACE_EXTRUDE: '拉伸轮廓',
    SURFACE_REVOLVE: '旋转轮廓',
};
export type CurveInputKind = keyof typeof inputNames;
export function hasCurveInputFeedback(kind: GeometryKind): kind is Extract<GeometryKind, CurveInputKind> {
    return Object.hasOwn(inputNames, kind);
}
export type CurveInputMember = {
    selections: SelectionItem[]; reversed: boolean
};
export type CurveInputSession = {
    kind: CurveInputKind; documentId: string; versionId: string; members: (CurveInputMember|null)[];
    tolerance?: number; report: (pause: boolean) => void;
};
export type CurveFeedback = {
    title: string; text: string; tone: 'info' | 'warning' | 'success'; markers: Vec3[];
    gap?: EdgeEnds; pause: boolean;
};
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
    const title = inputNames[kind], members = slots.filter((m): m is Member => !!m);
    const notice =
        (text: string, tone: CurveFeedback['tone'] = 'info', markers: Vec3[] = [], gap?: EdgeEnds,
         pause = false): CurveFeedback => ({title, text, tone, markers, gap, pause});
    if (slots.includes(undefined)) return notice('显示数据不足，由精确预览检查');
    const fourSides = kind === 'SURFACE_BOUNDARY' || kind === 'SUPPORTED_FILL';
    if (!members.length) {
        if (fourSides) return notice('请选择第一边 · 共 4 边');
        if (kind === 'SURFACE_FILL') return notice('请选择填充边界');
        if (kind === 'SURFACE_SWEEP') return notice('请选择轮廓和脊线');
        if (kind === 'SURFACE_LOFT') return notice('请依次选择放样截面');
        return notice('请选择曲线或边');
    }
    if (fourSides) {
        if (members.length > 4) return notice('带支撑的填充需要四条有序单边', 'warning');
        for (let i = 0; i < 4; i++) {
            const m = slots[i];
            if (!m) continue;
            if (m.edges.length !== 1 || touching(...m.edges[0], 1e-7))
                return notice(`第 ${i + 1} 边需要单条开放曲线`, 'warning', m.edges[0]);
            const next = (i + 1) % 4, n = slots[next];
            if (n?.edges.length !== 1) continue;
            const a = m.edges[0], b = n.edges[0];
            const pairs: EdgeEnds[] =
                m.oriented && n.oriented ? [[a[1], b[0]]] : a.flatMap(x => b.map(y => [x, y] as EdgeEnds));
            const pair = pairs.reduce((p, q) => distance(...p) <= distance(...q) ? p : q);
            if (tolerance !== undefined && !touching(...pair, tolerance))
                return notice(`第 ${i + 1}、${next + 1} 边未相接，请检查顺序或方向`, 'warning', pair, pair);
        }
        if (members.length < 4) {
            const next = [0, 1, 2, 3].find(i => !slots[i])!;
            return notice(
                `已选 ${members.length}/4 边 · 请选择第 ${next + 1} 边`, 'info', members.at(-1)!.edges[0]);
        }
        if (tolerance === undefined) return notice('四边已选 · 由精确预览检查容差和方向');
        const loop = inspectFillEndpoints(members.flatMap(m => m.edges), tolerance);
        if (loop.kind !== 'closed') return notice('四边未形成单一闭环', 'warning', loop.markers);
        return notice('四边已围合 · 方向由精确预览确认', 'success');
    }
    if (kind === 'SURFACE_FILL' || kind === 'COMPOSITE') {
        const check = inspectFillEndpoints(members.flatMap(m => m.edges)), free = check.markers.length,
              fill = kind === 'SURFACE_FILL';
        // OCCT builds a composite by adding edges in input order. Only single
        // edge members have an unambiguous display order within each input.
        if (!fill && ['closed', 'open'].includes(check.kind) && members.every(m => m.edges.length === 1)) {
            const previous: Vec3[] = [];
            for (const [index, member] of members.entries()) {
                const edge = member.edges[0];
                if (index && !edge.some(p => previous.some(q => touching(p, q, 1e-7))))
                    return notice(`第 ${index + 1} 条曲线未接上此前的线链，请调整顺序`, 'warning', edge);
                previous.push(...edge);
            }
        }
        switch (check.kind) {
            case 'closed':
                return notice(fill ? '已闭合' : '已组成闭合线链', 'success');
            case 'open':
                return notice(
                    fill ? `未闭合 · ${free} 个自由端点` : '已组成开放线链 · 2 个自由端点',
                    fill ? 'info' : 'success', check.markers, undefined, fill);
            case 'branch':
                return notice('存在分叉或重复边，请检查标记处', 'warning', check.markers, undefined, fill);
            case 'multiple':
                return notice(
                    free ? `曲线分离 · ${free} 个自由端点` : '存在多个独立闭环', 'warning', check.markers,
                    undefined, fill);
            default:
                return notice('等待精确预览');
        }
    }
    if (kind === 'SURFACE_BLEND' || kind === 'G1_CONNECT') {
        for (const m of members)
            if (m.edges.length !== 1 || kind === 'SURFACE_BLEND' && touching(...m.edges[0], 1e-7))
                return notice(
                    kind === 'SURFACE_BLEND' ? '每侧需要一条开放边界' : '每端需要一条曲线', 'warning');
        if (members.length < 2)
            return notice(
                `已选 1/2 · 请选择${slots[0] ? '第二' : '第一'}条曲线`, 'info', members[0].edges[0]);
        if (kind === 'G1_CONNECT' && members.every(m => m.oriented)) {
            const pair: EdgeEnds = [members[0].edges[0][1], members[1].edges[0][0]];
            return touching(...pair, 1e-7) ?
                notice('两个连接端点重合，请检查方向', 'warning', pair) :
                notice('连接第一条的末端与第二条的起端', 'success', pair, pair);
        }
        return notice(
            kind === 'SURFACE_BLEND' ? '两侧边界已选 · 检查对应方向和支撑' :
                                       '两条曲线已选 · 连接方向由精确预览确认',
            'info', members.flatMap(m => m.edges[0]));
    }
    if (kind === 'SURFACE_EXTRUDE' || kind === 'SURFACE_REVOLVE') {
        const edges = members.flatMap(m => m.edges), check = inspectFillEndpoints(edges);
        const shape = check.kind === 'closed' ? '闭合轮廓' : check.kind === 'open' ? '开放轮廓' : '轮廓';
        return notice(`${shape} · ${edges.length} 段`, 'info', check.markers);
    }
    const checks = members.map(m => inspectFillEndpoints(m.edges));
    if (checks.some(c => c.kind === 'unknown')) return notice('边数量超出显示检查范围，由精确预览检查');
    const bad = checks.findIndex(c => !['open', 'closed'].includes(c.kind));
    if (bad >= 0)
        return notice(
            `${kind === 'SURFACE_LOFT' ? '截面' : '输入'} ${bad + 1} 需要单一连续线链`, 'warning',
            checks[bad].markers);
    if (kind === 'SURFACE_LOFT') {
        if (checks.some(
                (c, i) => c.kind !== checks[0].kind || members[i].edges.length !== members[0].edges.length))
            return notice('各截面的开闭状态和边数需一致', 'warning');
        return notice(
            `${members.length} 个${checks[0].kind === 'open' ? '开放' : '闭合'}截面 · 每截面 ${
                members[0].edges.length} 段${members.length === 1 ? ' · 请继续选择' : ''}`,
            members.length > 1 ? 'success' : 'info', checks.at(-1)!.markers);
    }
    return notice(
        members.length === 2 ? '轮廓和脊线已选 · 精确预览检查相切和起始平面' :
            slots[0]         ? '轮廓已选 · 请选择脊线' :
                               '脊线已选 · 请选择轮廓',
        'info', checks.at(-1)!.markers);
}

export function displayCurveInputs(view: DisplayDocumentView, session: CurveInputSession): CurveFeedback {
    return inspectCurveInputs(
        session.kind, Array.from(session.members, m => m ? displayMember(view, m) : null),
        session.tolerance);
}
