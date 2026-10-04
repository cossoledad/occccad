import type { PatternDefinition, SelectionItem, SketchEntity, SketchFeature } from "../../types";

export function patternEntityStatus(sketch:SketchFeature,entity:SketchEntity):string {
  const source=entity.sourceEntityId??entity.id;
  const component=sketch.solve.components?.find(component=>component.entityIds.includes(entity.id))??sketch.solve.components?.find(component=>component.entityIds.includes(source));
  return component?.definitionStatus??component?.status??sketch.solve.definitionStatus??sketch.solve.status;
}

export function pickedPatternCenter(selection:SelectionItem):PatternDefinition["centerReference"] {
  if(selection.kind==="visual"&&selection.sketchReference&&["POINT","START","END","CENTER"].includes(selection.sketchReference.subElement)) {
    return {sketchId:selection.featureId,reference:selection.sketchReference};
  }
  if(selection.kind==="axis-system"&&selection.entityId)return {axisEntityId:`AXIS_SYSTEM:${selection.entityId}:Z`,reference:{target:"SKETCH_ORIGIN",subElement:"POINT"}};
  return undefined;
}

export function sketchPatternDefaultReferences(sketchId:string,kind:PatternDefinition["kind"]):Partial<PatternDefinition> {
 return kind==="LINEAR"?{directionReference:{target:"SKETCH_X_AXIS",subElement:"DIRECTION"}}:{centerReference:{sketchId,reference:{target:"SKETCH_ORIGIN",subElement:"POINT"}}};
}
