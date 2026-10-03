import type { SketchConstraint, SketchEntity, SketchGeometryRef, SketchOperation, SketchPoint2, Vec2 } from "../../types";

// Display candidates only. These curves neither solve constraints nor establish
// topology, and are never used as the commit payload or authoritative intersections.
export type SketchEditPreviewInput = {
  entities: readonly SketchEntity[];
  constraints?: readonly SketchConstraint[];
  operation: SketchOperation;
  axisPoints?: { start: Vec2; end: Vec2 };
  /** Optional normalized production intersection parameters, including seam hits. */
  intersectionParameters?: readonly number[];
};
export type SketchEditCandidatePreview = {
  status: "APPROXIMATE" | "UNAVAILABLE";
  operation: SketchOperation;
  entities: SketchEntity[];
  hitEntities: SketchEntity[];
  replacedEntityIds: string[];
  retainedIntervals: Array<[number, number]>;
  hitIntervals: Array<[number, number]>;
  candidateCount: number;
  diagnostic?: string;
};
const tau = 2 * Math.PI;
const displayModelTolerance = 1e-7;
const add = (a: SketchPoint2, b: SketchPoint2): SketchPoint2 => ({ x: a.x + b.x, y: a.y + b.y });
const sub = (a: SketchPoint2, b: SketchPoint2): SketchPoint2 => ({ x: a.x - b.x, y: a.y - b.y });
const mul = (p: SketchPoint2, k: number): SketchPoint2 => ({ x: p.x * k, y: p.y * k });
const dot = (a: SketchPoint2, b: SketchPoint2) => a.x * b.x + a.y * b.y;
const cross = (a: SketchPoint2, b: SketchPoint2) => a.x * b.y - a.y * b.x;
const length = (p: SketchPoint2) => Math.hypot(p.x, p.y);
const distance = (a: SketchPoint2, b: SketchPoint2) => length(sub(a, b));
const unit = (p: SketchPoint2) => mul(p, 1 / length(p));
// Match Go math.Remainder's ties-to-even rule for the same angular branch.
const remainder = (a: number, period: number) => {
  const quotient = a / period, floor = Math.floor(quotient), fraction = quotient - floor;
  const rounded = fraction < .5 ? floor : fraction > .5 ? floor + 1 : floor % 2 === 0 ? floor : floor + 1;
  return a - rounded * period;
};
const positive = (a: number) => ((a % tau) + tau) % tau;
const finitePoint = (p: SketchPoint2 | undefined): p is SketchPoint2 => !!p && Number.isFinite(p.x) && Number.isFinite(p.y);
const clone = (entity: SketchEntity) => structuredClone(entity);
function endpoint(entity: SketchEntity, end: boolean): SketchPoint2 | undefined {
  if (entity.kind === "LINE") return end ? entity.end : entity.start;
  if (!finitePoint(entity.center) || !Number.isFinite(entity.radius)) return undefined;
  const angle = (end ? entity.endAngle : entity.startAngle) ?? 0;
  return add(entity.center, { x: entity.radius! * Math.cos(angle), y: entity.radius! * Math.sin(angle) });
}
function lineIntersection(a: SketchPoint2, ad: SketchPoint2, b: SketchPoint2, bd: SketchPoint2): SketchPoint2 | undefined {
  const determinant = cross(ad, bd);
  return determinant === 0 ? undefined : add(a, mul(ad, cross(sub(b, a), bd) / determinant));
}
function lineCircle(a: SketchPoint2, direction: SketchPoint2, center: SketchPoint2, radius: number): SketchPoint2[] {
  const u = unit(direction), foot = add(a, mul(u, dot(sub(center, a), u)));
  const heightSquared = radius * radius - dot(sub(foot, center), sub(foot, center));
  if (heightSquared < 0) return [];
  if (heightSquared === 0) return [foot];
  const h = Math.sqrt(heightSquared);
  return [add(foot, mul(u, h)), add(foot, mul(u, -h))];
}
function circleCircle(a: SketchPoint2, ar: number, b: SketchPoint2, br: number): SketchPoint2[] {
  const d = distance(a, b);
  if (d === 0 || d > ar + br || d < Math.abs(ar - br)) return [];
  const u = unit(sub(b, a)), x = (ar * ar - br * br + d * d) / (2 * d), h2 = ar * ar - x * x;
  if (h2 < 0) return [];
  const foot = add(a, mul(u, x));
  if (h2 === 0) return [foot];
  const n = { x: -u.y, y: u.x }, h = Math.sqrt(h2);
  return [add(foot, mul(n, h)), add(foot, mul(n, -h))];
}
function curveParameter(entity: SketchEntity, point: SketchPoint2): number | undefined {
  if (entity.kind === "LINE" && entity.start && entity.end) {
    const direction = sub(entity.end, entity.start), denominator = dot(direction, direction);
    return denominator === 0 ? undefined : dot(sub(point, entity.start), direction) / denominator;
  }
  if ((entity.kind === "CIRCLE" || entity.kind === "ARC") && entity.center) {
    const from = entity.kind === "CIRCLE" ? 0 : entity.startAngle!, to = entity.kind === "CIRCLE" ? tau : entity.endAngle!;
    let angle = Math.atan2(point.y - entity.center.y, point.x - entity.center.x);
    if (entity.kind === "CIRCLE") return positive(angle) / tau;
    angle += Math.round(((from + to) / 2 - angle) / tau) * tau;
    return (angle - from) / (to - from);
  }
  return undefined;
}
function contains(entity: SketchEntity, p: SketchPoint2): boolean {
  const parameter = curveParameter(entity, p);
  if (parameter === undefined || parameter < -1e-10 || parameter > 1 + 1e-10) return false;
  if (entity.kind === "LINE") {
    const direction = sub(entity.end!, entity.start!);
    return Math.abs(cross(sub(p, entity.start!), direction)) / length(direction) <= displayModelTolerance;
  }
  return Math.abs(distance(p, entity.center!) - entity.radius!) <= displayModelTolerance;
}
function cutAllowed(entity: SketchEntity, cutEnd: boolean, p: SketchPoint2): boolean {
  const other = endpoint(entity, !cutEnd);
  return !!other && contains(entity, p) && distance(other, p) > displayModelTolerance;
}
function curveInterval(source: SketchEntity, interval: [number, number], id: string): SketchEntity | undefined {
  const result = clone(source), [a, b] = interval;
  result.id = id;
  if (source.kind === "LINE" && source.start && source.end) {
    const delta = sub(source.end, source.start);
    result.start = add(source.start, mul(delta, a)); result.end = add(source.start, mul(delta, b));
  } else if (["CIRCLE", "ARC", "ELLIPSE", "ELLIPTICAL_ARC"].includes(source.kind)) {
    const from = source.startAngle ?? 0, to = source.endAngle ?? tau;
    result.startAngle = from + (to - from) * a; result.endAngle = from + (to - from) * b;
    if (source.kind === "CIRCLE") result.kind = "ARC";
    if (source.kind === "ELLIPSE") result.kind = "ELLIPTICAL_ARC";
  } else if (source.kind === "SPLINE") {
    const from = source.parameterStart ?? source.knots?.[0], to = source.parameterEnd ?? source.knots?.at(-1);
    if (!source.poles?.length || from === undefined || to === undefined) return undefined;
    result.parameterStart = from + (to - from) * a; result.parameterEnd = from + (to - from) * b; result.closed = false;
  } else return undefined;
  return result;
}
function reflectPoint(p: SketchPoint2, axis: { start: Vec2; end: Vec2 }): SketchPoint2 {
  const a = { x: axis.start[0], y: axis.start[1] }, d = { x: axis.end[0] - a.x, y: axis.end[1] - a.y };
  return sub(mul(add(a, mul(d, dot(sub(p, a), d) / dot(d, d))), 2), p);
}
export function mirrorSketchPreviewEntity(source: SketchEntity, axis: { start: Vec2; end: Vec2 }): SketchEntity {
  if (![...axis.start, ...axis.end].every(Number.isFinite) || axis.start[0] === axis.end[0] && axis.start[1] === axis.end[1]) throw new Error("镜像轴必须是非退化直线");
  const entity = clone(source), reflectionAngle = 2 * Math.atan2(axis.end[1] - axis.start[1], axis.end[0] - axis.start[0]);
  for (const key of ["point", "start", "end", "center"] as const) if (entity[key]) entity[key] = reflectPoint(entity[key]!, axis);
  if (entity.controlPoints) entity.controlPoints = entity.controlPoints.map(p => reflectPoint(p, axis));
  if (entity.poles) entity.poles = entity.poles.map(p => reflectPoint(p, axis));
  if (entity.kind === "ELLIPSE" || entity.kind === "ELLIPTICAL_ARC") entity.rotation = reflectionAngle - (entity.rotation ?? 0);
  if (entity.startAngle !== undefined && entity.endAngle !== undefined) {
    if (entity.kind === "ELLIPTICAL_ARC") { entity.startAngle = -entity.startAngle; entity.endAngle = -entity.endAngle; }
    else { entity.startAngle = reflectionAngle - entity.startAngle; entity.endAngle = reflectionAngle - entity.endAngle; }
  }
  return entity;
}
function axisFor(input: SketchEditPreviewInput, reference: SketchGeometryRef): { start: Vec2; end: Vec2 } | undefined {
  if (input.axisPoints) return input.axisPoints;
  if (reference.target === "SKETCH_X_AXIS") return { start: [0, 0], end: [1, 0] };
  if (reference.target === "SKETCH_Y_AXIS") return { start: [0, 0], end: [0, 1] };
  const entity = input.entities.find(e => e.id === reference.entityId);
  return entity?.kind === "LINE" && entity.start && entity.end ? { start: [entity.start.x, entity.start.y], end: [entity.end.x, entity.end.y] } : undefined;
}
type FilletCandidate = { center: SketchPoint2; first: SketchPoint2; second: SketchPoint2; start: number; end: number; score: number };
function arcDistance(click: SketchPoint2, center: SketchPoint2, radius: number, start: number, end: number): number {
  let angle = Math.atan2(click.y - center.y, click.x - center.x);
  const lo = Math.min(start, end), hi = Math.max(start, end);
  angle += Math.round(((lo + hi) / 2 - angle) / tau) * tau;
  return angle >= lo && angle <= hi ? Math.abs(distance(click, center) - radius) : Math.min(distance(click, add(center, { x: radius * Math.cos(start), y: radius * Math.sin(start) })), distance(click, add(center, { x: radius * Math.cos(end), y: radius * Math.sin(end) })));
}
function filletCandidates(first: SketchEntity, second: SketchEntity, active: [SketchEntity, SketchEntity], ends: [boolean, boolean], radius: number, click: SketchPoint2): FilletCandidate[] {
  const candidates: FilletCandidate[] = [];
  const append = (center: SketchPoint2, a: SketchPoint2, b: SketchPoint2) => {
    if (!cutAllowed(active[0], ends[0], a) || !cutAllowed(active[1], ends[1], b) || distance(a, b) <= displayModelTolerance) return;
    const start = Math.atan2(a.y - center.y, a.x - center.x), sweep = remainder(Math.atan2(b.y - center.y, b.x - center.x) - start, tau);
    // The click selects a support/side candidate, never the complementary major arc.
    candidates.push({ center, first: a, second: b, start, end: start + sweep, score: arcDistance(click, center, radius, start, start + sweep) });
  };
  if (first.kind === "LINE" && second.kind === "LINE") {
    const au = unit(sub(first.end!, first.start!)), bu = unit(sub(second.end!, second.start!)), an = { x: -au.y, y: au.x }, bn = { x: -bu.y, y: bu.x };
    for (const as of [-1, 1]) for (const bs of [-1, 1]) {
      const center = lineIntersection(add(first.start!, mul(an, as * radius)), au, add(second.start!, mul(bn, bs * radius)), bu);
      if (center) append(center, sub(center, mul(an, as * radius)), sub(center, mul(bn, bs * radius)));
    }
  } else if (first.kind === "LINE" || second.kind === "LINE") {
    const flipped = first.kind !== "LINE", line = flipped ? second : first, arc = flipped ? first : second;
    const u = unit(sub(line.end!, line.start!)), n = { x: -u.y, y: u.x };
    for (const side of [-1, 1]) for (const offset of [arc.radius! + radius, arc.radius! - radius]) {
      if (offset === 0) continue;
      for (const center of lineCircle(add(line.start!, mul(n, side * radius)), u, arc.center!, Math.abs(offset))) {
        const a = sub(center, mul(n, side * radius)), b = add(arc.center!, mul(sub(center, arc.center!), arc.radius! / offset));
        if (flipped) append(center, b, a); else append(center, a, b);
      }
    }
  } else {
    for (const aoffset of [first.radius! + radius, first.radius! - radius]) for (const boffset of [second.radius! + radius, second.radius! - radius]) {
      if (aoffset === 0 || boffset === 0) continue;
      for (const center of circleCircle(first.center!, Math.abs(aoffset), second.center!, Math.abs(boffset))) append(center, add(first.center!, mul(sub(center, first.center!), first.radius! / aoffset)), add(second.center!, mul(sub(center, second.center!), second.radius! / boffset)));
    }
  }
  return candidates.sort((a, b) => a.score - b.score || a.center.x - b.center.x || a.center.y - b.center.y || (a.end - a.start) - (b.end - b.start));
}
function localIntersections(source: SketchEntity, boundaries: readonly SketchEntity[]): number[] {
  const supported = (e: SketchEntity) => ["LINE", "CIRCLE", "ARC"].includes(e.kind);
  if (!supported(source) || boundaries.some(e => !supported(e))) throw new Error("该组合等待精确曲线候选；本地显示不使用采样交点");
  const parameters: number[] = [];
  for (const boundary of boundaries) {
    let points: SketchPoint2[];
    if (source.kind === "LINE" && boundary.kind === "LINE") {
      const a = sub(source.end!, source.start!), b = sub(boundary.end!, boundary.start!);
      if (cross(a, b) === 0 && Math.abs(cross(sub(boundary.start!, source.start!), a)) / length(a) <= displayModelTolerance) throw new Error("重叠支撑需精确曲线预览，不能当作孤立交点");
      const p = lineIntersection(source.start!, a, boundary.start!, b); points = p ? [p] : [];
    } else if (source.kind === "LINE") points = lineCircle(source.start!, sub(source.end!, source.start!), boundary.center!, boundary.radius!);
    else if (boundary.kind === "LINE") points = lineCircle(boundary.start!, sub(boundary.end!, boundary.start!), source.center!, source.radius!);
    else {
      if (distance(source.center!, boundary.center!) <= displayModelTolerance && Math.abs(source.radius! - boundary.radius!) <= displayModelTolerance) {
        if(source.kind!=="ARC"||boundary.kind!=="ARC")throw new Error("重叠圆支撑需精确曲线预览");
        const midpoint=(e:SketchEntity)=>add(e.center!,{x:e.radius!*Math.cos((e.startAngle!+e.endAngle!)/2),y:e.radius!*Math.sin((e.startAngle!+e.endAngle!)/2)});
        if(contains(source,midpoint(boundary))||contains(boundary,midpoint(source)))throw new Error("重叠圆弧区间需精确曲线预览");
        points=[endpoint(boundary,false)!,endpoint(boundary,true)!];
      } else points = circleCircle(source.center!, source.radius!, boundary.center!, boundary.radius!);
    }
    for (const p of points) if (contains(source, p) && contains(boundary, p)) {
      const parameter = curveParameter(source, p);
      if (parameter !== undefined) parameters.push(Math.max(0, Math.min(1, parameter)));
    }
  }
  return parameters;
}
function previewResult(operation: SketchOperation): SketchEditCandidatePreview {
  return { status: "APPROXIMATE", operation: structuredClone(operation), entities: [], hitEntities: [], replacedEntityIds: [], retainedIntervals: [], hitIntervals: [], candidateCount: 0 };
}
function supportsCorner(source: SketchEntity): boolean {
  if (source.suppressed) return false;
  if (source.kind === "LINE") return finitePoint(source.start) && finitePoint(source.end) && distance(source.start, source.end) > displayModelTolerance;
  return source.kind === "ARC" && finitePoint(source.center) && Number.isFinite(source.radius) && source.radius! > 0 && Number.isFinite(source.startAngle) && Number.isFinite(source.endAngle) && source.startAngle !== source.endAngle;
}
function hasSupport(input: SketchEditPreviewInput, child: SketchEntity, source: SketchEntity): boolean {
  return !!input.constraints?.some(c => !c.suppressed && c.kind === "SAME_SUPPORT" && c.references.some(r => r.entityId === child.id) && c.references.some(r => r.entityId === source.id));
}
function cornerPair(input: SketchEditPreviewInput, operation: Extract<SketchOperation, { type: "FILLET_ENTITIES" | "CHAMFER_ENTITIES" }>): { sources: [SketchEntity, SketchEntity]; active: [SketchEntity, SketchEntity]; ends: [boolean, boolean] } {
  const sources: SketchEntity[] = [], active: SketchEntity[] = [], ends: boolean[] = [];
  for (const [index, id] of operation.entityIds.entries()) {
    let source = input.entities.find(e => e.id === id);
    if (!source) throw new Error("角点必须选择本地线段或圆弧");
    const ancestor = input.entities.find(e => e.id === source!.sourceEntityId);
    if (ancestor && hasSupport(input, source, ancestor)) source = ancestor;
    if (!supportsCorner(source)) throw new Error("角点支撑必须是非退化线段或圆弧");
    const reference = index === 0 ? operation.firstReference : operation.secondReference;
    if (reference.target !== "ENTITY" || ![id, source.id].includes(reference.entityId!) || !["START", "END"].includes(reference.subElement)) throw new Error("请选择每条曲线要切除的稳定起点或终点");
    const descendants = input.entities.filter(e => e.role === "PROFILE" && e.sourceEntityId === source!.id && hasSupport(input, e, source!));
    if (descendants.length > 1) throw new Error("角点子段有歧义，请选择明确的曲线");
    const child = descendants[0] ?? source, end = reference.subElement === "END";
    if (operation.trimMode === "TRIM" && child !== source && distance(endpoint(source, end)!, endpoint(child, end)!) > displayModelTolerance) throw new Error("该端点已有角点，请编辑其尺寸");
    sources.push(source); active.push(child); ends.push(end);
  }
  if (sources[0].id === sources[1].id) throw new Error("两条曲线不能来自同一支撑");
  return { sources: sources as [SketchEntity, SketchEntity], active: active as [SketchEntity, SketchEntity], ends: ends as [boolean, boolean] };
}
function clippedCornerEntity(source: SketchEntity, cutEnd: boolean, point: SketchPoint2, id: string, role: SketchEntity["role"]): SketchEntity {
  const result = clone(source); result.id = id; result.role = role;
  if (source.kind === "LINE") { if (cutEnd) result.end = point; else result.start = point; }
  else {
    let angle = Math.atan2(point.y - source.center!.y, point.x - source.center!.x);
    angle += Math.round(((source.startAngle! + source.endAngle!) / 2 - angle) / tau) * tau;
    if (cutEnd) result.endAngle = angle; else result.startAngle = angle;
  }
  return result;
}
/** Build a frozen display candidate from the same operation sent on confirmation. */
export function buildSketchEditPreview(input: SketchEditPreviewInput): SketchEditCandidatePreview {
  const operation = input.operation, result = previewResult(operation);
  const id = (slot: string) => `preview:${"operationId" in operation ? operation.operationId : "edit"}:${slot}`;
  try {
    if (operation.type === "MIRROR_ENTITIES") {
      const axis = axisFor(input, operation.axis);
      if (!axis) throw new Error("请选择明确的镜像直线或内置轴");
      for (const sourceID of operation.entityIds) {
        const source = input.entities.find(e => e.id === sourceID);
        if (!source || source.suppressed) throw new Error("镜像源不是可编辑的本地几何");
        const reflected = mirrorSketchPreviewEntity(source, axis); reflected.id = id(sourceID); result.entities.push(reflected);
      }
      result.candidateCount = result.entities.length;
      result.diagnostic = operation.mirrorMode === "INDEPENDENT" ? "独立镜像候选；确认后由模型验证" : "关联镜像候选；连接与对称关系将在确认后求解";
    } else if (operation.type === "FILLET_ENTITIES" || operation.type === "CHAMFER_ENTITIES") {
      if (!finitePoint(operation.point)) throw new Error("请选择明确的角点分支位置");
      const { sources, active, ends } = cornerPair(input, operation);
      let first: SketchPoint2, second: SketchPoint2, bridge: SketchEntity;
      if (operation.type === "FILLET_ENTITIES") {
        if (!Number.isFinite(operation.value) || operation.value <= 0) throw new Error("圆角半径必须为正值");
        const candidates = filletCandidates(sources[0], sources[1], active, ends, operation.value, operation.point), chosen = candidates[0];
        if (!chosen) throw new Error("没有有限圆角分支：半径过大、支撑平行或接点超出曲线范围");
        first = chosen.first; second = chosen.second; result.candidateCount = candidates.length;
        bridge = { id: id("fillet"), kind: "ARC", role: "PROFILE", center: chosen.center, radius: operation.value, startAngle: chosen.start, endAngle: chosen.end };
      } else {
        if (sources.some(e => e.kind !== "LINE")) throw new Error("倒角只支持两条直线");
        if (!["EQUAL", "TWO_LENGTHS", "LENGTH_ANGLE"].includes(operation.chamferMode)) throw new Error("请选择明确的倒角尺寸模式");
        const virtual = lineIntersection(sources[0].start!, sub(sources[0].end!, sources[0].start!), sources[1].start!, sub(sources[1].end!, sources[1].start!));
        if (!virtual) throw new Error("平行线没有有限角点");
        const rays = active.map((entity, i) => unit(sub(endpoint(entity, !ends[i])!, virtual)));
        const a = operation.chamferFirst;
        if (!Number.isFinite(a) || a <= 0) throw new Error("倒角第一长度必须为正值");
        let b = a;
        if (operation.chamferMode === "TWO_LENGTHS") b = operation.chamferSecond!;
        else if (operation.chamferMode === "LENGTH_ANGLE") {
          const theta = operation.chamferAngle! * Math.PI / 180, phi = Math.acos(Math.max(-1, Math.min(1, dot(rays[0], rays[1]))));
          if (!Number.isFinite(theta) || theta <= 0 || theta >= Math.PI || phi === 0 || phi + theta >= Math.PI) throw new Error("倒角角度没有正长度分支");
          b = a * Math.sin(theta) / Math.sin(phi + theta);
        }
        if (!Number.isFinite(b) || b <= 0) throw new Error("倒角第二长度必须为正值");
        first = add(virtual, mul(rays[0], a)); second = add(virtual, mul(rays[1], b));
        if (!cutAllowed(active[0], ends[0], first) || !cutAllowed(active[1], ends[1], second)) throw new Error("倒角超过有限边长");
        let reverseBridge = false;
        if (operation.chamferMode === "LENGTH_ANGLE") {
          const direction = sub(second, first), sourceDirection = sub(sources[0].end!, sources[0].start!);
          const delta = Math.atan2(cross(sourceDirection, direction), dot(sourceDirection, direction)) * 180 / Math.PI;
          let found = false;
          for (const flip of [false, true]) {
            for (const swap of [false, true]) {
              let angle = flip ? remainder(delta + 180, 360) : delta;
              if (swap) angle = -angle;
              if (Math.abs(angle - operation.chamferAngle!) <= 1e-8) { reverseBridge = flip; found = true; break; }
            }
            if (found) break;
          }
          if (!found) throw new Error("倒角角度分支无法一致表达");
        }
        bridge = { id: id("chamfer"), kind: "LINE", role: "PROFILE", start: reverseBridge ? second : first, end: reverseBridge ? first : second }; result.candidateCount = 1;
      }
      const role = operation.trimMode === "KEEP" ? "CONSTRUCTION" : "PROFILE";
      bridge.role = role;
      result.entities = [clippedCornerEntity(active[0], ends[0], first, id("first-cut"), role), clippedCornerEntity(active[1], ends[1], second, id("second-cut"), role), bridge];
      result.replacedEntityIds = operation.trimMode === "KEEP" ? [] : active.map(e => e.id);
      result.diagnostic = operation.trimMode === "KEEP" ? "保留原元素，新增辅助几何候选" : "真实曲线候选；约束、连接和引用影响由确认时验证";
    } else if (operation.type === "QUICK_TRIM" || operation.type === "TRIM_ENTITY" || operation.type === "SPLIT_ENTITY") {
      if (operation.entityIds.length !== 1) throw new Error("请选择单条曲线");
      const source = input.entities.find(e => e.id === operation.entityIds[0]);
      if (!source) throw new Error("修剪源不是本地几何");
      const intervals: Array<[number, number]> = [], hits: Array<[number, number]> = [];
      if (operation.type === "TRIM_ENTITY") {
        const [a, b] = operation.parameters;
        if (!Number.isFinite(a) || !Number.isFinite(b) || a < 0 || b > 1 || a >= b) throw new Error("保留范围必须满足 0 ≤ 起点 < 终点 ≤ 1");
        intervals.push([a, b]); if (a > 0) hits.push([0, a]); if (b < 1) hits.push([b, 1]);
      } else {
        let cuts: readonly number[];
        if (operation.type === "SPLIT_ENTITY") cuts = operation.parameters;
        else {
          if (!Number.isFinite(operation.hitParameter) || operation.hitParameter < 0 || operation.hitParameter > 1) throw new Error("命中参数必须位于当前曲线范围");
          if (!["DELETE_HIT", "KEEP_HIT", "BREAK"].includes(operation.trimMode)) throw new Error("请选择删除、保留或打断命中区间");
          const boundaries = operation.boundaryIds.map(boundary => input.entities.find(e => e.id === boundary));
          if (boundaries.some(e => !e) && !input.intersectionParameters) throw new Error("外部边界等待精确候选");
          cuts = input.intersectionParameters ?? localIntersections(source, boundaries as SketchEntity[]);
        }
        if (cuts.some(value => !Number.isFinite(value) || value < 0 || value > 1)) throw new Error("无效的曲线分割参数");
        const sorted = [...new Set([0, ...cuts, 1])].sort((a, b) => a - b);
        if (sorted.length < 3 && !(operation.type==="QUICK_TRIM"&&operation.trimMode==="DELETE_HIT")) throw new Error("没有内部交点或分割区间");
        const cyclic = operation.type === "QUICK_TRIM" && !cuts.some(value => Math.abs(value) <= 1e-10 || Math.abs(value - 1) <= 1e-10) && (source.kind === "CIRCLE" || source.kind === "ELLIPSE" || source.closed) && (operation.hitParameter < sorted[1] || operation.hitParameter >= sorted[sorted.length - 2]);
        if(operation.type==="SPLIT_ENTITY"&&(source.kind==="CIRCLE"||source.kind==="ELLIPSE")) {
          const periodicCuts=[...new Set(cuts.map(value=>value===1?0:value))].sort((a,b)=>a-b);
          if(periodicCuts.length<2)throw new Error("闭合曲线需要两个不同分割位置");
          for(let i=0;i<periodicCuts.length;i++)intervals.push([periodicCuts[i],i+1<periodicCuts.length?periodicCuts[i+1]:periodicCuts[0]+1]);
        } else for (let i = 1; i < sorted.length; ++i) {
          const interval: [number, number] = [sorted[i - 1], sorted[i]], hit = operation.type === "QUICK_TRIM" && ((operation.hitParameter >= interval[0] && (operation.hitParameter < interval[1] || i === sorted.length - 1 && operation.hitParameter === 1)) || cyclic && (i === 1 || i === sorted.length - 1));
          if (hit) hits.push(interval);
          if (operation.type === "SPLIT_ENTITY" || operation.trimMode === "BREAK" || operation.trimMode === "DELETE_HIT" && !hit || operation.trimMode === "KEEP_HIT" && hit) intervals.push(interval);
        }
      }
      if (intervals.length > 64) throw new Error("曲线候选超过 64 段限制");
      result.retainedIntervals = intervals; result.hitIntervals = hits;
      for (const [index, interval] of intervals.entries()) { const curve = curveInterval(source, interval, id(`retained-${index}`)); if (!curve) throw new Error("样条候选需要真实规范曲线数据"); result.entities.push(curve); }
      for (const [index, interval] of hits.entries()) { const curve = curveInterval(source, interval, id(`hit-${index}`)); if (!curve) throw new Error("命中区间需要真实规范曲线数据"); result.hitEntities.push(curve); }
      result.replacedEntityIds = [source.id]; result.candidateCount = intervals.length;
      result.diagnostic = operation.type === "QUICK_TRIM" ? ({ DELETE_HIT: "删除高亮命中区间", KEEP_HIT: "保留高亮命中区间", BREAK: "打断并保留全部区间" }[operation.trimMode]) : "分割范围候选；确认后由精确内核验证";
    } else throw new Error("此编辑使用已有工具预览");
    return result;
  } catch (error) {
    return { ...previewResult(operation), status: "UNAVAILABLE", diagnostic: error instanceof TypeError ? "候选参数或几何数据不完整" : error instanceof Error ? error.message : "无法构造编辑候选" };
  }
}
