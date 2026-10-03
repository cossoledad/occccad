import type { SketchProfileAnalysis, Vec2 } from "../../types";

// Presentation only. The server's production Profile Builder owns topology;
// render samples and solver DOF never become a separate closure test.
export function sketchProfileFeedback(analysis?: SketchProfileAnalysis): {
  summary: string;
  issues: NonNullable<SketchProfileAnalysis["issues"]>;
  locations: Vec2[];
} {
  if (!analysis) return { summary: "等待服务器轮廓分析", issues: [], locations: [] };
  const status = { EMPTY: "无实体轮廓", OPEN: "开放轮廓，可继续编辑", INVALID: "轮廓需要修复", CLOSED: analysis.geometryVerified ? "Profile 闭合检查通过" : "连通闭环，等待精确几何验证" }[analysis.status];
  return {
    summary: `${status}${analysis.status === "CLOSED" && analysis.geometryVerified ? ` · ${analysis.regionCount} 个区域 / ${analysis.loopCount} 个闭环` : ""}`,
    issues: analysis.issues,
    locations: analysis.issues.flatMap(issue => issue.position ? [[issue.position.x, issue.position.y] as Vec2] : []),
  };
}
