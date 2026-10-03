import { useState } from "react";
import type { DocumentView, SketchConstraint, SketchGeometryRef, SketchOperation, SketchEntity, Vec2 } from "../../types";
import { CommandDialog } from "../overlay/floating-panel";
import { parameterSourceText, parseParameterSource } from "../../features/workbench/parameter-editor";
import {constraintDefinition,isDimensionConstraintKind} from "./sketch-constraint-definition";
import {preselectedSketchReference} from "../interaction/sketch-reference-pick";
import {sketchEntityPoint,splineEditablePointIDs,splineEditablePoints} from "./sketch-geometry";
import { randomUUID } from "../../utils/random-uuid";

export type DimensionRequest = { mode:"edit"; featureId:string; constraintId:string; value?:number; unit:"mm"|"deg"; x:number;y:number }
 | { mode:"create"; featureId:string; kind:SketchConstraint["kind"]; references:SketchGeometryRef[]; labelPosition:Vec2; value:number; unit:"mm"|"deg";x:number;y:number };
export function dimensionSourceInput(source:string,unit:string):string {
 const parsed=parseParameterSource(source,unit);
 if(parsed.kind==="EXPRESSION") { if(!parsed.expression)throw new Error("请输入尺寸值或表达式");return parsed.expression; }
 return `${parsed.value} ${parsed.unit}`;
}
export function dimensionDefinitionOperation(constraint:SketchConstraint, original:SketchConstraint|undefined, source:string, initialSource:string, name:string, readonly:boolean, restore:"ORIGINAL"|"MEASUREMENT"):SketchOperation {
 if(name&&!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name))throw new Error("名称须使用字母、数字和下划线，并以字母或下划线开头");
 const parameterSource=!constraint.reference&&!readonly&&(!original||source!==initialSource)?dimensionSourceInput(source,constraint.unit??"mm"):undefined;
 return original?{type:"UPDATE_CONSTRAINT",constraintId:original.id,constraint,parameterSource,parameterKey:name||undefined,restoreMode:original.reference&&!constraint.reference?restore:undefined}:{type:"ADD_CONSTRAINT",constraint,parameterSource,parameterKey:name||undefined};
}
export function constraintReferenceChoices(constraint:SketchConstraint,index:number,entities:SketchEntity[]):SketchGeometryRef[]{
 const pick=constraintDefinition(constraint.kind).picks[index],retained=entities.find(entity=>entity.id===constraint.references[index===0?1:0]?.entityId);
 const choices:SketchGeometryRef[]=[];
 for(const entity of entities){if(entity.suppressed)continue;
  const whole=preselectedSketchReference(entity,pick,retained);if(whole)choices.push(whole);
  if(["POINT","LINEAR_DIMENSION","SYMMETRY_CENTER"].includes(pick)){
   for(const subElement of ["POINT","START","END","CENTER"] as const){if(!sketchEntityPoint(entity,subElement))continue;const pointId=subElement==="START"?entity.startPointId:subElement==="END"?entity.endPointId:undefined;
    choices.push({target:"ENTITY",entityId:entity.id,subElement,...(pointId?{pointId}:{})});}
   if(entity.kind==="SPLINE")splineEditablePoints(entity).forEach((_,controlPointIndex)=>choices.push({target:"ENTITY",entityId:entity.id,subElement:"CONTROL",controlPointIndex,...(splineEditablePointIDs(entity)[controlPointIndex]?{controlPointId:splineEditablePointIDs(entity)[controlPointIndex]}:{})}));
  }
  if(pick==="TANGENT_CURVE"&&((!retained||retained.kind==="LINE")&&(["ARC","ELLIPTICAL_ARC"].includes(entity.kind)||entity.kind==="SPLINE"&&entity.mode==="CONTROL"&&!entity.closed)||entity.kind==="ARC"&&!!retained&&["ARC","CIRCLE"].includes(retained.kind)))
   for(const subElement of ["START","END"] as const){const pointId=subElement==="START"?entity.startPointId:entity.endPointId;choices.push({target:"ENTITY",entityId:entity.id,subElement,...(pointId?{pointId}:{})});}
 }
 if(["POINT","LINEAR_DIMENSION","SYMMETRY_CENTER"].includes(pick))choices.push({target:"SKETCH_ORIGIN",subElement:"POINT"});
 if(["LINE","LINEAR_DIMENSION","SOLVER_CURVE","SYMMETRY_CENTER"].includes(pick)||pick==="TANGENT_CURVE"&&retained?.kind!=="LINE")choices.push({target:"SKETCH_X_AXIS",subElement:"DIRECTION"},{target:"SKETCH_Y_AXIS",subElement:"DIRECTION"});
 return [...new Map(choices.map(ref=>[JSON.stringify(ref),ref])).values()];
}
export function logicalDefinitionOperation(original:SketchConstraint,references:SketchGeometryRef[],suppressed:boolean):SketchOperation{
 if(isDimensionConstraintKind(original.kind)||original.internal||["MIRROR","SAME_SUPPORT"].includes(original.kind))throw new Error("派生/内部关系须通过对应几何编辑显式解除，不能直接改写");
 return {type:"UPDATE_CONSTRAINT",constraintId:original.id,constraint:{...original,references,suppressed}};
}
export function SketchDimensionEditor({request,view,onClose,onSubmit}:{request:DimensionRequest;view:DocumentView;onClose:()=>void;onSubmit:(operations:SketchOperation[])=>Promise<unknown>}) {
 const sketch=view.part?.features.find(feature=>feature.id===request.featureId)?.sketch;
 const original=request.mode==="edit"?sketch?.constraints.find(c=>c.id===request.constraintId):undefined;
 const logical=!!original&&!isDimensionConstraintKind(original.kind);
 const parameter=view.part?.parameters?.find(p=>p.parameterId===original?.parameterId);
 const initialSource=parameter?parameterSourceText(parameter):`${request.value??""} ${request.unit}`;
 const [source,setSource]=useState(initialSource),[name,setName]=useState(parameter?.key??"");
 const [reference,setReference]=useState(original?.reference??false);
 const hasOriginal=!!parameter?.source.literal||!!parameter?.source.expression||!!parameter?.source.external;
 const [restore,setRestore]=useState<"ORIGINAL"|"MEASUREMENT">(hasOriginal?"ORIGINAL":"MEASUREMENT");
 const [references,setReferences]=useState(original?.references??(request.mode==="create"?request.references:[]));
 const [suppressed,setSuppressed]=useState(original?.suppressed??false);
 const [deleting,setDeleting]=useState(false);
 const readonly=!!parameter?.source.external;
 const submit=async()=>{
  if(deleting&&original){await onSubmit([{type:"DELETE_CONSTRAINT",constraintId:original.id}]);onClose();return;}
  if(logical&&original){await onSubmit([logicalDefinitionOperation(original,references,suppressed)]);onClose();return;}
  if(name&&!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name))throw new Error("名称须使用字母、数字和下划线，并以字母或下划线开头");
  const constraint:SketchConstraint=original?{...original,references,reference,suppressed}:{id:randomUUID(),kind:request.mode==="create"?request.kind:"DISTANCE",references,reference,unit:request.unit,value:request.value,labelPosition:request.mode==="create"?{x:request.labelPosition[0],y:request.labelPosition[1]}:undefined};
  const operation=dimensionDefinitionOperation(constraint,original,source,initialSource,name,readonly,restore);
  await onSubmit([operation]);onClose();
 };
 return <CommandDialog id="sketch-dimension" open title={logical?`编辑${constraintDefinition(original!.kind).label}约束`:"编辑草图尺寸"} onClose={onClose} onConfirm={submit} confirmText={deleting?(logical?"删除约束":"删除尺寸"):"确定"} width={380}>
  {original&&<button type="button" onClick={()=>setDeleting(!deleting)}>{deleting?"取消删除":logical?"删除此约束…":"删除此尺寸…"}</button>}
  {deleting&&<p>{logical?"按“删除约束”正式删除此关系。":"按“删除尺寸”正式删除关系及其尺寸参数；存在表达式依赖时操作将原子拒绝。"}</p>}
  {!logical&&<><label>名称<input aria-label="尺寸名称" value={name} placeholder="可选参数别名" onChange={e=>setName(e.target.value)}/></label>
  <label><input type="checkbox" checked={reference} onChange={e=>setReference(e.target.checked)}/>参考尺寸（测量，不驱动）</label>
  <label>值或表达式<input aria-label="尺寸值或表达式" value={source} disabled={reference||readonly} onChange={e=>setSource(e.target.value)}/></label>
  {reference&&<p>测量：{original?.value===undefined?"不可测":`${original.value} ${request.unit}`}。保留原驱动定义。</p>}
  {readonly&&<p>外部来源只读。</p>}
  {original?.reference&&!reference&&<label>恢复驱动<select aria-label="恢复驱动规则" value={restore} onChange={e=>setRestore(e.target.value as typeof restore)}><option value="ORIGINAL" disabled={!hasOriginal}>恢复原驱动定义</option><option value="MEASUREMENT" disabled={readonly||original.value===undefined}>显式使用当前测量</option></select></label>}
  {original?.reference&&!hasOriginal&&<p>此参考尺寸没有原驱动定义；恢复时需使用合法的当前测量。</p>}
  </>}
  {references.map((ref,index)=><label key={index}>引用 {index+1}<select aria-label={`${logical?"约束":"尺寸"}引用 ${index+1}`} value={JSON.stringify(ref)} onChange={e=>setReferences(references.map((r,i)=>i===index?JSON.parse(e.target.value) as SketchGeometryRef:r))}>
   <option value={JSON.stringify(ref)}>当前：{ref.subElement}</option>
   {constraintReferenceChoices(original?{...original,references}:{id:"candidate",kind:request.mode==="create"?request.kind:"DISTANCE",references},index,sketch?.entities??[]).map(choice=>{
    const entity=sketch?.entities.find(entity=>entity.id===choice.entityId),ordinal=sketch?.entities.indexOf(entity!)??-1;
    return <option key={JSON.stringify(choice)} value={JSON.stringify(choice)}>{entity?`${entity.kind} ${ordinal+1}`:choice.target} · {choice.subElement}{choice.controlPointIndex!==undefined?` ${choice.controlPointIndex+1}`:""}</option>;
   })}
  </select></label>)}
  {original&&<label><input type="checkbox" checked={suppressed} onChange={e=>setSuppressed(e.target.checked)}/>停用</label>}
 </CommandDialog>;
}
