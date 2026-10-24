import type { PatternDefinition, SelectionItem, SketchEntity, SketchFeature } from "../../types";

export function patternEntityStatus(sketch:SketchFeature,entity:SketchEntity):string {
  const source=entity.sourceEntityId??entity.id;
  const component=sketch.solve.components?.find(component=>component.entityIds.includes(entity.id))??sketch.solve.components?.find(component=>component.entityIds.includes(source));
  return component?.definitionStatus??component?.status??sketch.solve.definitionStatus??sketch.solve.status;
}

export function pickedPatternCenter(selection:SelectionItem,view?:import('../../types').DocumentView):PatternDefinition["centerReference"] {
  if(selection.kind==="visual"&&selection.sketchReference&&["POINT","START","END","CENTER"].includes(selection.sketchReference.subElement)) {
    return {sketchId:selection.featureId,reference:selection.sketchReference};
  }
  if((selection.kind==="axis-system"||selection.kind==="datum-point"&&view?.part?.axisSystems.some(a=>a.id===selection.entityId))&&selection.entityId)return {axisEntityId:`AXIS_SYSTEM:${selection.entityId}:Z`,reference:{target:"SKETCH_ORIGIN",subElement:"POINT"}};
  return undefined;
}

export function sketchPatternDefaultReferences(sketchId:string,kind:PatternDefinition["kind"]):Partial<PatternDefinition> {
 return kind==="LINEAR"?{directionReference:{target:"SKETCH_X_AXIS",subElement:"DIRECTION"}}:{centerReference:{sketchId,reference:{target:"SKETCH_ORIGIN",subElement:"POINT"}}};
}

// Restore accepted references for display/location; names never become inputs.
export function patternReferenceName(view:import('../../types').DocumentView,id:string,reference?:PatternDefinition['directionReference']):string {
 if(!reference){const [kind,owner,axis]=id.split(':');const object=kind==='DATUM_AXIS'?view.part?.datumAxes?.find(a=>a.id===owner):kind==='AXIS_SYSTEM'?view.part?.axisSystems.find(a=>a.id===owner):view.part?.features.find(f=>f.id===owner);if(!object)return '引用方向已丢失';if(kind==='SKETCH_LINE'){const entity=view.part?.features.find(f=>f.id===owner)?.sketch?.entities.find(e=>e.id===axis);return `${object.name} / ${entity?(entity.name||'直线'):'引用直线已丢失'}`}return `${object.name}${kind==='AXIS_SYSTEM'?` / ${axis} 轴`:''}`;}
 const sketch=view.part?.features.find(f=>f.id===id);if(!sketch?.sketch)return '引用草图已丢失';
 const roles:Record<string,string>={SKETCH_ORIGIN:'原点',SKETCH_X_AXIS:'水平轴 (U)',SKETCH_Y_AXIS:'竖直轴 (V)',POINT:'点',START:'起点',END:'终点',CENTER:'圆心',DIRECTION:'方向',WHOLE:'几何'};
 const entity=reference.target==='EXTERNAL'?sketch.sketch.externalGeometry?.find(e=>e.id===reference.entityId):sketch.sketch.entities.find(e=>e.id===reference.entityId);
 return `${sketch.name} / ${['ENTITY','EXTERNAL'].includes(reference.target)?`${entity?(entity.name||'几何'):'引用几何已丢失'}${reference.subElement!=='WHOLE'?` / ${roles[reference.subElement]??'几何'}`:''}`:roles[reference.target]??'参考几何'}`;
}
type ReferenceContext=Pick<SelectionItem,'contextVariantKey'|'rootDocumentId'|'instancePath'>;
export function patternAxisPick(view:import('../../types').DocumentView,id:string,path?:string,context:ReferenceContext={},origin=false):SelectionItem|undefined {
 const [kind,owner,axis]=id.split(':');const base={...context,documentId:view.document.id,versionId:view.document.versionId,occurrencePath:path};
 if(kind==='SKETCH_LINE')return patternReferencePick(view,owner,{target:'ENTITY',entityId:axis,subElement:'DIRECTION'},path,context);
 if(kind==='DATUM_AXIS'&&view.part?.datumAxes?.some(a=>a.id===owner))return {...base,kind:'axis',id,entityId:owner,axis:'DATUM'};
 if(kind==='AXIS_SYSTEM'&&view.part?.axisSystems.some(a=>a.id===owner)){if(origin)return {...base,kind:'datum-point',id:`${path||'root'}:${owner}:point`,entityId:owner};if(axis==='X'||axis==='Y'||axis==='Z')return {...base,kind:'axis',id,entityId:owner,axis};}
 return undefined;
}
export function patternReferencePick(view:import('../../types').DocumentView,sketchId:string,reference:NonNullable<PatternDefinition['directionReference']>,path?:string,context:ReferenceContext={}):SelectionItem|undefined {
 const feature=view.part?.features.find(f=>f.id===sketchId);if(!feature?.sketch)return undefined;
 if(reference.target==='ENTITY'&&!feature.sketch.entities.some(e=>e.id===reference.entityId))return undefined;
 return {...context,kind:'visual',id:`reference:${sketchId}:${reference.target}:${reference.entityId??''}:${reference.subElement}`,featureId:sketchId,entityId:reference.entityId??reference.target,bodyId:feature.bodyId,visualType:reference.subElement==='DIRECTION'?'CURVE':'POINT',sketchReference:reference,documentId:view.document.id,versionId:view.document.versionId,occurrencePath:path};
}
