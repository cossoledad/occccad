import { useEffect, useRef, useState } from "react";
import { Button, Checkbox, Input, Select, Space, Typography } from "antd";
import { formatDimensionValue, selectInitialDimensionValue } from "./dimension-value-format";
import { SketchReferenceSelectionSession, type SketchReferenceSelectionBindings } from "./sketch-reference-selection-session";
import { sketchCommitResultUnknown, type SketchCommitIntent } from "../tool/sketch-command-session";
import { millimetersToDisplayLength, type DisplayLengthUnit } from "../../state/ui-preferences";
import { measureSketchDimension } from "./sketch-constraint-layout";
export type { SketchReferenceSelectionRequest, SketchReferenceSelectionBindings } from "./sketch-reference-selection-session";
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
export function dimensionDefinitionOperation(constraint:SketchConstraint, original:SketchConstraint|undefined, source:string, initialSource:string, name:string, readonly:boolean, restore:"ORIGINAL"|"MEASUREMENT",inputUnit:string=constraint.unit??"mm"):SketchOperation {
 if(name&&!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name))throw new Error("名称须使用字母、数字和下划线，并以字母或下划线开头");
 const parameterSource=!constraint.reference&&!readonly&&(!original||source!==initialSource)?dimensionSourceInput(source,inputUnit):undefined;
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
export function dimensionDisplayValue(value:number,unit:DisplayLengthUnit|"deg"):number {
 return unit==="deg"?value:millimetersToDisplayLength(value,unit);
}
export function referenceMatches(a:SketchGeometryRef,b:SketchGeometryRef):boolean {
 if(a.target!==b.target||a.entityId!==b.entityId||a.subElement!==b.subElement)return false;
 if(a.pointId||b.pointId)return a.pointId===b.pointId;
 if(a.controlPointId||b.controlPointId)return a.controlPointId===b.controlPointId;
 return a.controlPointIndex===b.controlPointIndex;
}
export function validReferenceReplacement(constraint:SketchConstraint,slot:number,candidate:SketchGeometryRef,entities:SketchEntity[]):boolean {
 return constraintReferenceChoices(constraint,slot,entities).some(choice=>referenceMatches(choice,candidate));
}
const entityLabels:Record<SketchEntity["kind"],string>={POINT:"点",LINE:"直线",CIRCLE:"圆",ARC:"圆弧",ELLIPSE:"椭圆",ELLIPTICAL_ARC:"椭圆弧",SPLINE:"样条"};
const roleLabels:Record<SketchGeometryRef["subElement"],string>={POINT:"点",START:"起点",END:"终点",CENTER:"中心",CONTROL:"编辑点",WHOLE:"整体",DIRECTION:"方向"};
export function sketchReferenceLabel(reference:SketchGeometryRef,entities:readonly SketchEntity[]):string {
 if(reference.target!=="ENTITY")return reference.target==="SKETCH_ORIGIN"?"草图原点":reference.target==="SKETCH_X_AXIS"?"草图 X 轴":"草图 Y 轴";
 const entity=entities.find(entity=>entity.id===reference.entityId);
 if(!entity)return "引用元素已不可用";
 const role=reference.subElement==="CONTROL"?(entity.kind==="SPLINE"&&entity.mode==="CONTROL"?"控制极点":"拟合点"):roleLabels[reference.subElement];
 return `${entityLabels[entity.kind]} · ${role}`;
}
/** The exact atomic definition and receipt identity survive transport ambiguity. */
export class SketchDimensionCommitSession {
 private retained?:{operations:SketchOperation[];intent:SketchCommitIntent};
 unknown=false;
 async submit(operations:SketchOperation[],baseVersionId:string,onSubmit:(operations:SketchOperation[],intent?:SketchCommitIntent)=>Promise<unknown>){
  const attempt=this.retained??{operations:structuredClone(operations),intent:{requestId:randomUUID(),baseVersionId}};
  this.retained=attempt;
  try{const result=await onSubmit(structuredClone(attempt.operations),{...attempt.intent,retryReceipt:this.unknown||undefined});this.retained=undefined;this.unknown=false;return result;}
  catch(error){this.unknown=sketchCommitResultUnknown(error);if(!this.unknown)this.retained=undefined;throw error;}
 }
}
export function SketchDimensionEditor({request,view,onClose,onSubmit,onSelectReference,onHighlightReference,onLocateReference,preferredLengthUnit,onPreview}: {
 request:DimensionRequest;view:DocumentView;preferredLengthUnit?:DisplayLengthUnit;onClose:()=>void;onSubmit:(operations:SketchOperation[],intent?:SketchCommitIntent)=>Promise<unknown>;onPreview?:(operations:SketchOperation[],signal:AbortSignal)=>Promise<void>
}&SketchReferenceSelectionBindings) {
 const sketch=view.part?.features.find(feature=>feature.id===request.featureId)?.sketch;
 const original=request.mode==="edit"?sketch?.constraints.find(c=>c.id===request.constraintId):undefined;
 const logical=!!original&&!isDimensionConstraintKind(original.kind);
 const kind=original?.kind??(request.mode==="create"?request.kind:"DISTANCE");
 const definition=constraintDefinition(kind);
 const parameter=view.part?.parameters?.find(p=>p.parameterId===original?.parameterId);
 const [inputUnit]=useState<DisplayLengthUnit|"deg">(()=>request.unit==="deg"?"deg":preferredLengthUnit??"mm");
 const parameterText=parameter?parameterSourceText(parameter,inputUnit):undefined;
 const initialSource=parameter?.source.literal?formatDimensionValue(Number(parameterText)):parameterText??(request.value===undefined?"":formatDimensionValue(dimensionDisplayValue(request.value,inputUnit)));
 const valueFocused=useRef<HTMLInputElement|null>(null);
 const [source,setSource]=useState(initialSource),[name,setName]=useState(parameter?.key??"");
 const [reference,setReference]=useState(original?.reference??false);
 const hasOriginal=!!parameter?.source.literal||!!parameter?.source.expression||!!parameter?.source.external;
 const [restore,setRestore]=useState<"ORIGINAL"|"MEASUREMENT">(hasOriginal?"ORIGINAL":"MEASUREMENT");
 const [references,setReferences]=useState(original?.references??(request.mode==="create"?request.references:[]));
 const [suppressed,setSuppressed]=useState(original?.suppressed??false);
 const [previewConstraintID]=useState(()=>request.mode==="edit"?request.constraintId:randomUUID());
 const commitSession=useRef(new SketchDimensionCommitSession());
 const [unknown,setUnknown]=useState(false);
 const [deleting,setDeleting]=useState(false),[selecting,setSelecting]=useState<number>(),[pending,setPending]=useState(false);
 const readonly=!!parameter?.source.external;
 const draft={...(original??{id:"candidate",kind}),references};
 const measured=measureSketchDimension(kind,references,sketch?.entities??[]);
 const measurement=measured!==undefined&&Number.isFinite(measured)?measured:undefined;
 const current=useRef({view,references,draft,sketch,onHighlightReference});
 current.current={view,references,draft,sketch,onHighlightReference};
 const activeSlot=useRef<number|undefined>(undefined);
 const release=useRef<(()=>void)|undefined>(undefined);
 const session=useRef<SketchReferenceSelectionSession|undefined>(undefined);
 if(!session.current)session.current=new SketchReferenceSelectionSession({
  version:()=>`${current.current.view.document.id}:${current.current.view.document.versionId}:${request.featureId}`,
  validate:(candidate,slot)=>validReferenceReplacement(current.current.draft,slot,candidate,current.current.sketch?.entities??[]),
  onAccept:(candidate,slot)=>{setReferences(refs=>refs.map((ref,index)=>index===slot?candidate:ref));setSelecting(undefined);const cleanup=release.current;release.current=undefined;cleanup?.();},
  onHighlight:candidate=>current.current.onHighlightReference?.(request.featureId,candidate,activeSlot.current),
  onCancel:()=>{setSelecting(undefined);const cleanup=release.current;release.current=undefined;cleanup?.();}
 });
 const cancelSelection=()=>session.current!.cancel();
 useEffect(()=>()=>{session.current!.cancel();onHighlightReference?.(request.featureId,undefined);},[]);
 useEffect(()=>{cancelSelection();},[view.document.id,view.document.versionId]);
 const replace=(slot:number)=>{
  if(!onSelectReference||pending||unknown)return;
  const token=session.current!.begin(slot);activeSlot.current=slot;setSelecting(slot);
  const cleanup=onSelectReference({featureId:request.featureId,slot,pick:definition.picks[slot],
   retained:sketch?.entities.find(entity=>entity.id===references[slot===0?1:0]?.entityId),references,
   allowed:candidate=>validReferenceReplacement(current.current.draft,slot,candidate,current.current.sketch?.entities??[]),
   onCandidate:candidate=>session.current!.accept(token,candidate),
   onPreview:candidate=>session.current!.preview(token,candidate),onCancel:cancelSelection});
  if(session.current!.active)release.current=cleanup;else cleanup();
 };
 const [previewError,setPreviewError]=useState<string>();
 const previewPort=useRef(onPreview);previewPort.current=onPreview;
 useEffect(()=>{
  if(!previewPort.current)return;
  const abort=new AbortController();
  // Clear a superseded candidate immediately. Partial text remains a draft;
  // parse/validation errors appear locally, never as repeated notifications.
  void previewPort.current([],abort.signal);
  if(pending||unknown||deleting||selecting!==undefined)return ()=>abort.abort();
  const timer=setTimeout(()=>{
   try {
    const constraint:SketchConstraint=original?{...original,references,reference,suppressed}:{id:previewConstraintID,kind,references,reference,unit:request.unit,value:request.value,labelPosition:request.mode==="create"?{x:request.labelPosition[0],y:request.labelPosition[1]}:undefined};
    const operation=logical&&original?logicalDefinitionOperation(original,references,suppressed):dimensionDefinitionOperation(constraint,original,source,initialSource,name,readonly,restore,inputUnit);
    void previewPort.current!([operation],abort.signal).then(()=>{if(!abort.signal.aborted)setPreviewError(undefined);},error=>{if(!abort.signal.aborted)setPreviewError(error instanceof Error?error.message:String(error));});
   } catch {setPreviewError(undefined);}
  },180);
  return ()=>{clearTimeout(timer);abort.abort();};
 },[source,name,reference,references,suppressed,restore,pending,unknown,deleting,selecting]);
 const close=()=>{if(session.current!.active){cancelSelection();return;}if(!pending)onClose();};
 const submit=async()=>{
  if(session.current!.active||pending)return;
  setPending(true);
  const commit=(operations:SketchOperation[])=>commitSession.current.submit(operations,view.document.versionId,onSubmit);
  try{
   if(commitSession.current.unknown){await commit([]);setUnknown(false);onClose();return;}
   if(deleting&&original){await commit([{type:"DELETE_CONSTRAINT",constraintId:original.id}]);onClose();return;}
   if(logical&&original){await commit([logicalDefinitionOperation(original,references,suppressed)]);onClose();return;}
   if(name&&!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name))throw new Error("名称须使用字母、数字和下划线，并以字母或下划线开头");
   const constraint:SketchConstraint=original?{...original,references,reference,suppressed}:{id:randomUUID(),kind,references,reference,unit:request.unit,value:request.value,labelPosition:request.mode==="create"?{x:request.labelPosition[0],y:request.labelPosition[1]}:undefined};
   await commit([dimensionDefinitionOperation(constraint,original,source,initialSource,name,readonly,restore,inputUnit)]);onClose();
  }finally{setUnknown(commitSession.current.unknown);setPending(false);}
 };
 return <CommandDialog id="sketch-dimension" open title={logical?`编辑${definition.label}约束`:`编辑${definition.label}尺寸`} onClose={close} onConfirm={submit} confirmDisabled={selecting!==undefined} confirmText={unknown?"重试确认":deleting?(logical?"删除约束":"删除尺寸"):"确定"} width={380}>
  <fieldset disabled={pending||unknown} style={{border:0,padding:0,margin:0,minWidth:0}}>
  <div className="sketch-dimension-form">
  {previewError&&<Typography.Text type="secondary" aria-live="polite">预览不可用：{previewError}</Typography.Text>}
  {unknown&&<Typography.Paragraph aria-live="polite">请求已发出；结果待确认。重试使用同一请求查询结果，草稿暂不可修改。关闭面板不撤销已发请求。</Typography.Paragraph>}
  {deleting?<Typography.Paragraph>{logical?"确认后正式删除此关系。":"确认后正式删除关系及其尺寸参数；存在表达式依赖时操作将原子拒绝。"}</Typography.Paragraph>:<>
   {!logical&&<label className="sketch-dimension-value">值或表达式（{inputUnit}）
    <Input autoFocus aria-label="尺寸值或表达式" value={source} disabled={pending||unknown||reference||readonly}
     onFocus={e=>selectInitialDimensionValue(e.currentTarget,valueFocused)} onChange={e=>setSource(e.target.value)}/>
   </label>}
   {!logical&&<label className="sketch-dimension-name">名称
    <Input disabled={pending||unknown} aria-label="尺寸名称" value={name} placeholder="可选参数名称" onChange={e=>setName(e.target.value)}/>
   </label>}
   <div className="sketch-dimension-references" aria-label="作用对象">
    {references.map((ref,index)=><div className="sketch-dimension-reference" key={index}
      onMouseEnter={()=>{if(selecting===undefined)onHighlightReference?.(request.featureId,ref,index);}}
      onMouseLeave={()=>{if(selecting===undefined)onHighlightReference?.(request.featureId,undefined,index);}}>
     <div><Typography.Text type="secondary">{definition.pickLabels[index]??"引用对象"}</Typography.Text>
      <div><Typography.Text>{sketchReferenceLabel(ref,sketch?.entities??[])}</Typography.Text></div></div>
     <Space size="small"><Button size="small" aria-label={`定位${definition.pickLabels[index]}`} disabled={pending||unknown||!onLocateReference} onClick={()=>onLocateReference?.(request.featureId,ref,index)}>定位</Button>
      {logical&&<Button size="small" aria-label={`${selecting===index?"取消更换":"更换"}${definition.pickLabels[index]}`} type={selecting===index?"primary":"default"} disabled={pending||unknown||!onSelectReference} onClick={()=>selecting===index?cancelSelection():replace(index)}>{selecting===index?"取消更换":"更换"}</Button>}
     </Space>
    </div>)}
   </div>
   {selecting!==undefined&&<Typography.Paragraph aria-live="polite">请在草图中选择{definition.pickLabels[selecting]}；Esc 取消此次更换，保留草稿。</Typography.Paragraph>}
   <div className="sketch-dimension-options">
    {!logical&&<Checkbox disabled={pending||unknown} checked={reference} onChange={e=>setReference(e.target.checked)}>参考尺寸</Checkbox>}
    {original&&<Checkbox disabled={pending||unknown} checked={suppressed} onChange={e=>setSuppressed(e.target.checked)}>停用</Checkbox>}
   </div>
   {!logical&&reference&&<Typography.Text type="secondary" aria-live="polite">草稿测量：{measurement===undefined?"不可测":`${formatDimensionValue(dimensionDisplayValue(measurement,inputUnit))} ${inputUnit}`} · 不驱动几何</Typography.Text>}
   {readonly&&<Typography.Text type="secondary">外部来源只读。</Typography.Text>}
   {!logical&&original?.reference&&!reference&&<label>恢复驱动<Select disabled={pending||unknown} aria-label="恢复驱动规则" style={{width:"100%"}} value={restore} onChange={setRestore} options={[{value:"ORIGINAL",label:"恢复原驱动定义",disabled:!hasOriginal},{value:"MEASUREMENT",label:"显式使用当前测量",disabled:readonly||measurement===undefined}]}/></label>}
   {!logical&&original?.reference&&!hasOriginal&&<Typography.Text type="secondary">此参考尺寸没有原驱动定义；恢复时需使用合法的当前测量。</Typography.Text>}
  </>}
  {original&&<div className="sketch-dimension-danger"><Button danger type="text" disabled={pending||unknown||selecting!==undefined} onClick={()=>setDeleting(!deleting)}>{deleting?"取消删除":logical?"删除约束":"删除尺寸"}</Button></div>}
  </div></fieldset>
 </CommandDialog>;
}
