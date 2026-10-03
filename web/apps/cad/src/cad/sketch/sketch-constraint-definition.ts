import type { SketchConstraint } from "../../types";
import type { SketchReferencePickKind } from "../interaction/sketch-reference-pick";

export type ConstraintKind = SketchConstraint["kind"];
export type ToolbarConstraintKind = Exclude<ConstraintKind, "FIXED_POINT" | "MIRROR" | "SAME_SUPPORT">;
export type DimensionConstraintKind = Extract<ConstraintKind, "DISTANCE" | "HORIZONTAL_DISTANCE" | "VERTICAL_DISTANCE" | "LENGTH" | "RADIUS" | "DIAMETER" | "MAJOR_RADIUS" | "MINOR_RADIUS" | "ANGLE">;
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
  COLLINEAR: define({ kind: "COLLINEAR", label: "共线", symbol: "parallel", picks: ["LINE", "LINE"],
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
  DISTANCE: define({ kind: "DISTANCE", label: "距离", symbol: "distance", picks: ["LINEAR_DIMENSION", "LINEAR_DIMENSION"],
    pickLabels: ["第一个点或直线", "第二个点或直线（两线采用平行间距）"], unit: "mm", dimension: "linear" }),
  HORIZONTAL_DISTANCE: define({ kind: "HORIZONTAL_DISTANCE", label: "水平距离", symbol: "distance", picks: ["POINT", "POINT"],
    pickLabels: ["基准点", "目标点（目标 X − 基准 X）"], unit: "mm", dimension: "linear" }),
  VERTICAL_DISTANCE: define({ kind: "VERTICAL_DISTANCE", label: "竖直距离", symbol: "distance", picks: ["POINT", "POINT"],
    pickLabels: ["基准点", "目标点（目标 Y − 基准 Y）"], unit: "mm", dimension: "linear" }),
  LENGTH: define({ kind: "LENGTH", label: "长度", symbol: "length", picks: ["LINE"],
    pickLabels: ["直线"], unit: "mm", dimension: "linear" }),
  RADIUS: define({ kind: "RADIUS", label: "半径", symbol: "radius", picks: ["CIRCULAR"],
    pickLabels: ["圆或圆弧"], unit: "mm", dimension: "radial" }),
  DIAMETER: define({ kind: "DIAMETER", label: "直径", symbol: "diameter", picks: ["CIRCULAR"],
    pickLabels: ["圆或圆弧"], unit: "mm", dimension: "diametric" }),
  MAJOR_RADIUS: define({ kind: "MAJOR_RADIUS", label: "椭圆长半轴", symbol: "radius", picks: ["ELLIPTICAL"],
    pickLabels: ["椭圆或椭圆弧"], unit: "mm", dimension: "linear" }),
  MINOR_RADIUS: define({ kind: "MINOR_RADIUS", label: "椭圆短半轴", symbol: "radius", picks: ["ELLIPTICAL"],
    pickLabels: ["椭圆或椭圆弧"], unit: "mm", dimension: "linear" }),
  ANGLE: define({ kind: "ANGLE", label: "角度", symbol: "angle", picks: ["LINE", "LINE"],
    pickLabels: ["第一条直线", "第二条直线"], unit: "deg", dimension: "angular" }),
  CONCENTRIC: define({ kind: "CONCENTRIC", label: "同心", symbol: "concentric", picks: ["CENTER_CURVE", "CENTER_CURVE"],
    pickLabels: ["第一个圆、圆弧或椭圆", "第二个圆、圆弧或椭圆"], dimension: "none" }),
  POINT_ON_OBJECT: define({ kind: "POINT_ON_OBJECT", label: "点在对象上", symbol: "point_on_object",
    picks: ["POINT", "SOLVER_CURVE"], pickLabels: ["点", "直线、圆、圆弧或椭圆"], dimension: "none" }),
  MIDPOINT: define({ kind: "MIDPOINT", label: "中点", symbol: "midpoint", picks: ["POINT", "LINE"],
    pickLabels: ["点", "直线"], dimension: "none" }),
  SAME_SUPPORT: define({kind:"SAME_SUPPORT",label:"保持原支撑曲线",symbol:"equal",picks:["CURVE","CURVE"],pickLabels:["原曲线","子曲线"],dimension:"none"}),
  MIRROR: define({ kind: "MIRROR", label: "关联镜像", symbol: "symmetry", picks: ["ENTITY", "LINE", "ENTITY"],
    pickLabels: ["源元素", "镜像轴", "镜像结果"], dimension: "none" }),
  SYMMETRY: define({ kind: "SYMMETRY", label: "对称", symbol: "symmetry", picks: ["POINT", "SYMMETRY_CENTER", "POINT"],
    pickLabels: ["第一个点", "对称轴或中心点", "第二个点"], dimension: "none" }),
};

export const TOOLBAR_CONSTRAINT_KINDS: readonly ToolbarConstraintKind[] = [
  "COINCIDENT", "PARALLEL", "COLLINEAR", "FIXED", "HORIZONTAL", "VERTICAL", "PERPENDICULAR", "TANGENT", "EQUAL",
  "DISTANCE", "HORIZONTAL_DISTANCE", "VERTICAL_DISTANCE", "LENGTH", "RADIUS", "DIAMETER", "MAJOR_RADIUS", "MINOR_RADIUS", "ANGLE", "CONCENTRIC", "POINT_ON_OBJECT", "MIDPOINT", "SYMMETRY",
];

export const LOGICAL_CONSTRAINT_KINDS: readonly ToolbarConstraintKind[] = [
  "COINCIDENT", "PARALLEL", "COLLINEAR", "FIXED", "HORIZONTAL", "VERTICAL", "PERPENDICULAR", "TANGENT", "EQUAL",
  "CONCENTRIC", "POINT_ON_OBJECT", "MIDPOINT", "SYMMETRY",
];

export const OTHER_DIMENSION_CONSTRAINT_KINDS: readonly ToolbarConstraintKind[] = ["HORIZONTAL_DISTANCE", "VERTICAL_DISTANCE", "RADIUS", "MAJOR_RADIUS", "MINOR_RADIUS", "ANGLE"];
export const DIMENSION_CONSTRAINT_KINDS: readonly DimensionConstraintKind[] = ["DISTANCE", "HORIZONTAL_DISTANCE", "VERTICAL_DISTANCE", "LENGTH", "RADIUS", "DIAMETER", "MAJOR_RADIUS", "MINOR_RADIUS", "ANGLE"];

export function isDimensionConstraintKind(kind: string): kind is DimensionConstraintKind {
  return (DIMENSION_CONSTRAINT_KINDS as readonly string[]).includes(kind);
}

export function constraintDefinition(kind: ConstraintKind): SketchConstraintDefinition {
  return SKETCH_CONSTRAINT_DEFINITIONS[kind];
}
