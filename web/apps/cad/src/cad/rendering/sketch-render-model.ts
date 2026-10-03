import type { Feature, Vec2 } from "../../types";
import { sketchEntityPoint } from "../sketch/sketch-geometry";

export type SketchRenderModel = {
  profileLines: Vec2[];
  constructionLines: Vec2[];
  profilePoints: Vec2[];
  constructionPoints: Vec2[];
  endpoints: Vec2[];
};

export function buildSketchRenderModel(feature: Feature): SketchRenderModel {
  const result: SketchRenderModel = {
    profileLines: [], constructionLines: [], profilePoints: [], constructionPoints: [], endpoints: [],
  };
  for (const entity of feature.sketch?.entities ?? []) {
    const construction = entity.role === "CONSTRUCTION";
    if (entity.kind === "POINT" && entity.point) {
      (construction ? result.constructionPoints : result.profilePoints).push([entity.point.x, entity.point.y]);
    }
    if (entity.kind === "LINE" && entity.start && entity.end) {
      const start: Vec2 = [entity.start.x, entity.start.y];
      const end: Vec2 = [entity.end.x, entity.end.y];
      (construction ? result.constructionLines : result.profileLines).push(start, end);
      result.endpoints.push(start, end);
    }
    if (["CIRCLE", "ARC", "ELLIPSE", "ELLIPTICAL_ARC"].includes(entity.kind) && entity.center) {
      (construction ? result.constructionPoints : result.profilePoints).push([entity.center.x, entity.center.y]);
    }
    if (entity.kind === "ELLIPTICAL_ARC") {
      const start = sketchEntityPoint(entity, "START"), end = sketchEntityPoint(entity, "END");
      if (start && end) result.endpoints.push(start, end);
    }
    if (entity.kind === "ARC" && entity.center && entity.radius && entity.startAngle !== undefined && entity.endAngle !== undefined) {
      result.endpoints.push(
        [entity.center.x + entity.radius * Math.cos(entity.startAngle), entity.center.y + entity.radius * Math.sin(entity.startAngle)],
        [entity.center.x + entity.radius * Math.cos(entity.endAngle), entity.center.y + entity.radius * Math.sin(entity.endAngle)],
      );
    }
    if (entity.kind === "SPLINE" && !entity.closed) {
      const start = sketchEntityPoint(entity, "START"), end = sketchEntityPoint(entity, "END");
      if (start && end) result.endpoints.push(start, end);
    }
  }
  return result;
}
