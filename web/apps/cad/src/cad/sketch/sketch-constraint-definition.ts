import type { SketchConstraint } from "../../types";
import type { SketchReferencePickKind } from "../interaction/sketch-reference-pick";

export type ConstraintKind = SketchConstraint["kind"];
export type ToolbarConstraintKind = Exclude<ConstraintKind, "FIXED_POINT">;
export type DimensionConstraintKind = Extract<ConstraintKind, "DISTANCE" | "LENGTH" | "RADIUS" | "DIAMETER" | "ANGLE">;
export type ConstraintSymbol =
  | "coincident" | "parallel" | "fixed" | "horizontal" | "vertical" | "perpendicular"
  | "tangent" | "equal" | "distance" | "length" | "radius" | "diameter" | "angle"
  | "concentric" | "point_on_object" | "midpoint" | "symmetry";

export type SketchConstraintDefinition = {
  kind: ConstraintKind;
  label: string;
  symbol: ConstraintSymbol;
  picks: readonly SketchReferencePickKind[];
  pickLabels: readonly string[];
  unit?: "mm" | "deg";
  dimension: "none" | "linear" | "radial" | "diametric" | "angular";
};

const define = <T extends SketchConstraintDefinition>(definition: T): T => definition;

export const SKETCH_CONSTRAINT_DEFINITIONS: Record<ConstraintKind, SketchConstraintDefinition> = {
  COINCIDENT: define({ kind: "COINCIDENT", label: "重合", symbol: "coincident", picks: ["POINT", "POINT"],
    pickLabels: ["第一个点", "第二个点"], dimension: "none" }),
  PARALLEL: define({ kind: "PARALLEL", label: "平行", symbol: "parallel", picks: ["LINE", "LINE"],
    pickLabels: ["第一条直线", "第二条直线"], dimension: "none" }),
  FIXED: define({ kind: "FIXED", label: "固定", symbol: "fixed", picks: ["ENTITY"],
    pickLabels: ["要固定的元素"], dimension: "none" }),
  FIXED_POINT: define({ kind: "FIXED_POINT", label: "固定点", symbol: "fixed", picks: ["POINT"],
    pickLabels: ["要固定的点"], dimension: "none" }),
  HORIZONTAL: define({ kind: "HORIZONTAL", label: "水平", symbol: "horizontal", picks: ["LINE"],
    pickLabels: ["直线"], dimension: "none" }),
  VERTICAL: define({ kind: "VERTICAL", label: "竖直", symbol: "vertical", picks: ["LINE"],
    pickLabels: ["直线"], dimension: "none" }),
  PERPENDICULAR: define({ kind: "PERPENDICULAR", label: "垂直", symbol: "perpendicular", picks: ["LINE", "LINE"],
    pickLabels: ["第一条直线", "第二条直线"], dimension: "none" }),
  TANGENT: define({ kind: "TANGENT", label: "相切", symbol: "tangent", picks: ["TANGENT_CURVE", "TANGENT_CURVE"],
    pickLabels: ["第一条曲线", "第二条曲线"], dimension: "none" }),
  EQUAL: define({ kind: "EQUAL", label: "相等", symbol: "equal", picks: ["EQUAL_CURVE", "EQUAL_CURVE"],
    pickLabels: ["第一个等长/等半径元素", "兼容的第二个元素"], dimension: "none" }),
  DISTANCE: define({ kind: "DISTANCE", label: "距离", symbol: "distance", picks: ["POINT", "POINT"],
    pickLabels: ["第一个点", "第二个点"], unit: "mm", dimension: "linear" }),
  LENGTH: define({ kind: "LENGTH", label: "长度", symbol: "length", picks: ["LINE"],
    pickLabels: ["直线"], unit: "mm", dimension: "linear" }),
  RADIUS: define({ kind: "RADIUS", label: "半径", symbol: "radius", picks: ["CIRCULAR"],
    pickLabels: ["圆或圆弧"], unit: "mm", dimension: "radial" }),
  DIAMETER: define({ kind: "DIAMETER", label: "直径", symbol: "diameter", picks: ["CIRCULAR"],
    pickLabels: ["圆或圆弧"], unit: "mm", dimension: "diametric" }),
  ANGLE: define({ kind: "ANGLE", label: "角度", symbol: "angle", picks: ["LINE", "LINE"],
    pickLabels: ["第一条直线", "第二条直线"], unit: "deg", dimension: "angular" }),
  CONCENTRIC: define({ kind: "CONCENTRIC", label: "同心", symbol: "concentric", picks: ["CIRCULAR", "CIRCULAR"],
    pickLabels: ["第一个圆或圆弧", "第二个圆或圆弧"], dimension: "none" }),
  POINT_ON_OBJECT: define({ kind: "POINT_ON_OBJECT", label: "点在对象上", symbol: "point_on_object",
    picks: ["POINT", "SOLVER_CURVE"], pickLabels: ["点", "直线、圆或圆弧"], dimension: "none" }),
  MIDPOINT: define({ kind: "MIDPOINT", label: "中点", symbol: "midpoint", picks: ["POINT", "LINE"],
    pickLabels: ["点", "直线"], dimension: "none" }),
  SYMMETRY: define({ kind: "SYMMETRY", label: "对称", symbol: "symmetry", picks: ["POINT", "SYMMETRY_CENTER", "POINT"],
    pickLabels: ["第一个点", "对称轴或中心点", "第二个点"], dimension: "none" }),
};

export const TOOLBAR_CONSTRAINT_KINDS: readonly ToolbarConstraintKind[] = [
  "COINCIDENT", "PARALLEL", "FIXED", "HORIZONTAL", "VERTICAL", "PERPENDICULAR", "TANGENT", "EQUAL",
  "DISTANCE", "LENGTH", "RADIUS", "DIAMETER", "ANGLE", "CONCENTRIC", "POINT_ON_OBJECT", "MIDPOINT", "SYMMETRY",
];

export const LOGICAL_CONSTRAINT_KINDS: readonly ToolbarConstraintKind[] = [
  "COINCIDENT", "PARALLEL", "FIXED", "HORIZONTAL", "VERTICAL", "PERPENDICULAR", "TANGENT", "EQUAL",
  "CONCENTRIC", "POINT_ON_OBJECT", "MIDPOINT", "SYMMETRY",
];

export const OTHER_DIMENSION_CONSTRAINT_KINDS: readonly ToolbarConstraintKind[] = ["RADIUS", "DIAMETER", "ANGLE"];
export const DIMENSION_CONSTRAINT_KINDS: readonly DimensionConstraintKind[] = ["DISTANCE", "LENGTH", "RADIUS", "DIAMETER", "ANGLE"];

export function isDimensionConstraintKind(kind: string): kind is DimensionConstraintKind {
  return (DIMENSION_CONSTRAINT_KINDS as readonly string[]).includes(kind);
}

export function constraintDefinition(kind: ConstraintKind): SketchConstraintDefinition {
  return SKETCH_CONSTRAINT_DEFINITIONS[kind];
}
