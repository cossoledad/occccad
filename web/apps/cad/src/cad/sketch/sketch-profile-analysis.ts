import type { SketchProfileAnalysis, Vec2 } from "../../types";

// Presentation only. The server's production Profile Builder owns topology;
// render samples and solver DOF never become a separate closure test.
export function sketchProfileFeedback(analysis?: SketchProfileAnalysis): {
  summary: string;
  issues: NonNullable<SketchProfileAnalysis["issues"]>;
  locations: Vec2[];
} {
  if (!analysis) return { summary: "轮廓待检查", issues: [], locations: [] };
  const status = { EMPTY: "无实体轮廓", OPEN: "开放轮廓，可继续编辑", INVALID: "轮廓需要修复", CLOSED: analysis.geometryVerified ? "轮廓已闭合" : "闭环待检查" }[analysis.status];
  return {
    summary: `${status}${analysis.status === "CLOSED" && analysis.geometryVerified ? ` · ${analysis.regionCount} 个区域 / ${analysis.loopCount} 个闭环` : ""}`,
    issues: analysis.issues,
    locations: analysis.issues.flatMap(issue => issue.position ? [[issue.position.x, issue.position.y] as Vec2] : []),
  };
}

export function profileIssueText(code:string):string {
 const messages:Record<string,string>={OPEN_ENDPOINT:'轮廓端点未连接，请补齐或添加重合约束。',DUPLICATE_EDGE:'存在重复轮廓元素，请删除重叠的线。',DEGENERATE_EDGE:'轮廓线段过短，请调整或删除。',UNSOLVED_CONNECTION:'连接端点尚未重合，请检查约束。',BRANCH_POINT:'轮廓存在分叉，请检查连接处。',INVALID_REGION:'轮廓无法形成有效区域，请检查交叉与闭合。'};
 return messages[code]??'轮廓需要检查，请定位相关几何。';
}
