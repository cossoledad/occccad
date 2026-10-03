import { constraintDefinition } from "../sketch/sketch-constraint-definition";
import { buildSketchConstraintLayout } from "../sketch/sketch-constraint-layout";
import { cornerClickIntent, directLengthInput, type DirectEditInput } from "./sketch-direct-input";
import { buildSketchEditPreview, type SketchEditCandidatePreview } from "../sketch/sketch-edit-preview";
import { sketchCommitResultUnknown, type SketchCommandAction, type SketchCommandState, type SketchCommandPhase } from "./sketch-command-session";
import type {SelectionItem,SketchConstraint,SketchEntity,SketchGeometryRef,SketchOperation,SketchPoint2,Vec2} from "../../types";
import {SelectionInputResult,InputResult,type SelectionInputSource,type CadKeyboardEvent,type CadPointerEvent} from "../input/input-types";
import type {CadTool,ToolContext} from "./cad-tool";
import {randomUUID} from "../../utils/random-uuid";
import {splineEditablePoints,splineEditablePointIDs,sampleSketchEntity,evaluateCanonicalSpline} from "../sketch/sketch-geometry";
import {SKETCH_INPUT_POLICY} from "../sketch/sketch-input-policy";

export type SketchEditKind="delete"|"copy"|"move"|"rotate"|"mirror"|"split"|"trim"|"quick_trim"|"fillet"|"chamfer"|"extend"|"complement"|"close"|"offset"|"spline_insert"|"spline_delete"|"spline_close"|"spline_control"|"construction";
type SketchScope=NonNullable<ReturnType<NonNullable<ToolContext["viewport"]["currentSketchIdentity"]>>>;
const scopeKey=(scope:SketchScope|undefined)=>scope&&`${scope.documentId}/${scope.sketchId}/${scope.versionId}/${scope.occurrencePath??""}`;
const point=(value:Vec2):SketchPoint2=>({x:value[0],y:value[1]});
const labels:Record<SketchEditKind,string>={delete:"删除",copy:"复制",move:"移动",rotate:"旋转",mirror:"镜像",split:"分割",trim:"修剪保留范围",quick_trim:"快速修剪",fillet:"草图圆角",chamfer:"草图倒角",extend:"延伸到交点",complement:"圆弧补弧",close:"闭合曲线",offset:"二维独立偏移",spline_insert:"样条插入拟合点/结点",spline_delete:"样条删除拟合点/结点",spline_close:"样条开/闭合",spline_control:"转为控制点样条",construction:"标准/辅助转换"};

export function localSketchSelection(selections:readonly SelectionItem[],context:ToolContext):SketchEntity[]{
  const scope=context.viewport.currentSketchIdentity?.(),entities=context.viewport.currentSketchEntities?.()??[];
  const ids=[...new Set(selections.filter(selection=>selection.kind==="visual"&&(!scope||selection.featureId===scope.sketchId)&&
    (!scope||(selection.ownerDocumentId??selection.documentId)===scope.documentId||(!scope.occurrencePath&&!selection.ownerDocumentId&&!selection.documentId))&&(!scope|| (selection.occurrencePath??"")===(scope.occurrencePath??""))&&(!scope||!selection.versionId||selection.versionId===scope.versionId)).map(selection=>selection.entityId))];
  const byID=new Map(entities.map(entity=>[entity.id,entity]));
  return ids.flatMap(id=>byID.has(id!)?[byID.get(id!)!]:[]);
}

// This is an approximate display transform only. The operation carries original
// stable references to the control plane, which validates and solves the model.
export function sketchEditPreview(entities:readonly SketchEntity[],origin:Vec2,translation:Vec2,angle:number,
  axis?:{start:Vec2;end:Vec2}):SketchEntity[]{
  const c=Math.cos(angle),s=Math.sin(angle);
  const map=(p:SketchPoint2):SketchPoint2=>{
    if(axis){const dx=axis.end[0]-axis.start[0],dy=axis.end[1]-axis.start[1],length=dx*dx+dy*dy,
      projection=((p.x-axis.start[0])*dx+(p.y-axis.start[1])*dy)/length;
      return{x:2*(axis.start[0]+projection*dx)-p.x,y:2*(axis.start[1]+projection*dy)-p.y};}
    const dx=p.x-origin[0],dy=p.y-origin[1];
    return{x:origin[0]+dx*c-dy*s+translation[0],y:origin[1]+dx*s+dy*c+translation[1]};
  };
  const reflectionAngle=axis?2*Math.atan2(axis.end[1]-axis.start[1],axis.end[0]-axis.start[0]):0;
  return entities.map(source=>{
    const entity=structuredClone(source);
    for(const key of ["point","start","end","center"] as const)if(entity[key])entity[key]=map(entity[key]!);
    if(entity.controlPoints)entity.controlPoints=entity.controlPoints.map(map);
    if(entity.poles)entity.poles=entity.poles.map(map);
    if(entity.rotation!==undefined)entity.rotation=axis?reflectionAngle-entity.rotation:entity.rotation+angle;
    if(entity.startAngle!==undefined&&entity.endAngle!==undefined){
      if(entity.kind==="ELLIPTICAL_ARC"){if(axis){entity.startAngle=-entity.startAngle;entity.endAngle=-entity.endAngle;}}
      else{entity.startAngle=axis?reflectionAngle-entity.startAngle:entity.startAngle+angle;entity.endAngle=axis?reflectionAngle-entity.endAngle:entity.endAngle+angle;}
    }
    return entity;
  });
}

function selectedCurveParameter(entity:SketchEntity,target:Vec2):number|undefined{
  if(entity.kind==="LINE"&&entity.start&&entity.end){const dx=entity.end.x-entity.start.x,dy=entity.end.y-entity.start.y,length=dx*dx+dy*dy;
    return length?((target[0]-entity.start.x)*dx+(target[1]-entity.start.y)*dy)/length:undefined;}
  if(["CIRCLE","ARC","ELLIPSE","ELLIPTICAL_ARC"].includes(entity.kind)&&entity.center){
    let dx=target[0]-entity.center.x,dy=target[1]-entity.center.y;
    if(entity.kind==="ELLIPSE"||entity.kind==="ELLIPTICAL_ARC"){const c=Math.cos(entity.rotation??0),s=Math.sin(entity.rotation??0),x=dx*c+dy*s;dy=(-dx*s+dy*c)/entity.minorRadius!;dx=x/entity.majorRadius!;}
    const angle=Math.atan2(dy,dx),start=entity.startAngle??0,end=entity.endAngle??2*Math.PI,sweep=end-start,positive=(value:number)=>((value%(2*Math.PI))+2*Math.PI)%(2*Math.PI);
    return sweep>0?positive(angle-start)/sweep:-positive(start-angle)/sweep;
  }
  if(entity.kind==="SPLINE"){
    const sampled=sampleSketchEntity(entity,128);let best=Infinity,result:number|undefined;
    for(let index=1;index<sampled.length;index++){const a=sampled[index-1],b=sampled[index],dx=b[0]-a[0],dy=b[1]-a[1],length=dx*dx+dy*dy;
      const projection=length?Math.max(0,Math.min(1,((target[0]-a[0])*dx+(target[1]-a[1])*dy)/length)):0;
      const distance=Math.hypot(target[0]-a[0]-projection*dx,target[1]-a[1]-projection*dy);
      if(distance<best){best=distance;result=(index-1+projection)/(sampled.length-1);}}
    return result;
  }
  return undefined;
}
function curvePoint(entity:SketchEntity,parameter:number):Vec2|undefined{
  if(entity.kind==="LINE"&&entity.start&&entity.end)return[entity.start.x+(entity.end.x-entity.start.x)*parameter,entity.start.y+(entity.end.y-entity.start.y)*parameter];
  if(entity.kind==="SPLINE"&&entity.poles?.length){const start=entity.parameterStart??entity.knots?.[0],end=entity.parameterEnd??entity.knots?.at(-1);return start===undefined||end===undefined?undefined:evaluateCanonicalSpline(entity,start+(end-start)*parameter);}
  if(entity.center){const start=entity.startAngle??0,end=entity.endAngle??2*Math.PI,angle=start+(end-start)*parameter;
    if(entity.kind==="ARC"||entity.kind==="CIRCLE")return[entity.center.x+entity.radius!*Math.cos(angle),entity.center.y+entity.radius!*Math.sin(angle)];
    const x=entity.majorRadius!*Math.cos(angle),y=entity.minorRadius!*Math.sin(angle),c=Math.cos(entity.rotation??0),s=Math.sin(entity.rotation??0);return[entity.center.x+x*c-y*s,entity.center.y+x*s+y*c];}
  return undefined;
}

let clipboard:{entities:readonly SketchEntity[];constraints:readonly SketchConstraint[]}|undefined;
export function copySketchSelection(context:ToolContext):boolean{
  const entities=localSketchSelection(context.viewport.currentSelections?.()??[],context);if(!entities.length)return false;
  const ids=new Set(entities.map(entity=>entity.id));
  const constraints=(context.viewport.currentSketchConstraints?.()??[]).filter(constraint=>
    constraint.value===undefined&&!constraint.parameterId&&!['FIXED','FIXED_POINT'].includes(constraint.kind)&&
    constraint.references.every(reference=>reference.target==='ENTITY'&&reference.entityId&&ids.has(reference.entityId)));
  clipboard=structuredClone({entities,constraints});
  context.viewport.setToolPrompt(`已复制 ${entities.length} 个几何及 ${constraints.length} 个内部逻辑关系；不复制驱动尺寸、公式、固定或跨选择集依赖`);return true;
}
export function pasteSketchClipboard(context:ToolContext,offset:Vec2=[10,10]):boolean{
  if(!clipboard?.entities.length||!context.viewport.hasActiveSketch())return false;
  const idMap=new Map(clipboard.entities.map(entity=>[entity.id,randomUUID()]));
  const operationID=randomUUID();
  const pointIDMap=new Map<string,string>();
  for(const entity of clipboard.entities)for(const id of [...entity.controlPointIds??[],...entity.poleIds??[]])pointIDMap.set(id,randomUUID());
  const entities=sketchEditPreview(clipboard.entities,[0,0],offset,0).map(entity=>({...entity,id:idMap.get(entity.id)!,createdByOperationId:operationID,sourceEntityId:entity.id,
    ...(entity.controlPointIds?{controlPointIds:entity.controlPointIds.map(id=>pointIDMap.get(id)!)}:{}),
    ...(entity.poleIds?{poleIds:entity.poleIds.map(id=>pointIDMap.get(id)!)}:{})}));
  const constraints=clipboard.constraints.map(constraint=>({...structuredClone(constraint),id:randomUUID(),
    references:constraint.references.map(reference=>({...reference,entityId:idMap.get(reference.entityId!)!,...(reference.controlPointId?{controlPointId:pointIDMap.get(reference.controlPointId)!}:{})})),
    ...(constraint.labelPosition?{labelPosition:{x:constraint.labelPosition.x+offset[0],y:constraint.labelPosition.y+offset[1]}}:{})}));
  const operations:SketchOperation[]=entities.map(entity=>({type:'ADD_ENTITY',entity}));
  operations.push(...constraints.map(constraint=>({type:'ADD_CONSTRAINT' as const,constraint})));
  context.viewport.commitSketchOperations(operations);
  context.viewport.setToolPrompt(`已提交粘贴请求：${entities.length} 个独立几何及 ${constraints.length} 个内部逻辑关系；驱动尺寸、公式及外部依赖未复制`);return true;
}

export class SketchEditTool implements CadTool{
  readonly id:string;
  private direct?:DirectEditInput;
  private recentCornerSource="5";
  private selected=new Set<string>();
  private baseline:SketchEntity[]=[];
  private scope?:SketchScope;
  private phase:SketchCommandPhase="selection";
  private get armed():boolean { return ["definition","placement","failed","committing","unknown"].includes(this.phase); }
  private error?:string;
  private hover?:SketchEntity;
  private commitGeneration=0;
  private pendingOperations?:SketchOperation[];
  private boundaryMode:"automatic"|"explicit"="automatic";
  private advancedTrim=false;
  private candidate?:SketchEditCandidatePreview;
  private candidateGeneration=0;
  private acceptPendingCandidate=false;
  private candidateAbort?:AbortController;
  private verifiedCandidateGeneration?:number;
  private previewStatus:"idle"|"pending"|"ready"|"failed"="idle";
  private operationID?:string;
  private requestID?:string;
  private origin?:Vec2;
  private ray?:Vec2;
  private translation:Vec2=[0,0];
  private angle=0;
  private axis?:SketchGeometryRef;
  private copy=false;
  private splinePointReference?:SketchGeometryRef;
  private extendReference?:SketchGeometryRef;
  private branchPoint?:Vec2;
  private offsetMode:"MITER"|"ROUND"="MITER";
  private cornerRefs:SketchGeometryRef[]=[];
  private cornerPoint?:Vec2;
  private cornerTrimMode:"TRIM"|"KEEP"="TRIM";
  private chamferMode:"EQUAL"|"TWO_LENGTHS"|"LENGTH_ANGLE"="EQUAL";
  private batch:SketchOperation[]=[];
  private boundaries:SketchGeometryRef[]=[];
  private trimMode:"DELETE_HIT"|"KEEP_HIT"|"BREAK"="DELETE_HIT";
  private detach:string[]=[];
  private mirrorMode:"INDEPENDENT"|"LINKED"="LINKED";
  private policy:"INTERNAL"|"GEOMETRY_ONLY"="GEOMETRY_ONLY";
  private fields:[string,string]=["",""];
  private field=0;
  private captured?:number;
  private extendDragStart?:Vec2;
  private extendHoverTimer?:ReturnType<typeof setTimeout>;
  constructor(readonly kind:SketchEditKind){this.id=`sketch.edit.${kind}`;if(kind==="trim")this.trimMode="KEEP_HIT";}
  private publishDirect(context:ToolContext):void {
    const direct=this.direct!;
    const busy=this.phase==="committing"||this.phase==="unknown";
    let role="",completion:SketchCommandState["completion"],input:SketchCommandState["input"],fields:SketchCommandState["fields"]=[],required:number|undefined;
    if(direct.kind==="mirror"){
      role=direct.stage==="axis"?"选择镜像轴":"选择要镜像的元素";
      if(direct.stage==="sources"&&this.selected.size&&!busy)completion={label:"完成"};
    } else if(direct.kind==="corner"){
      required=2;role=direct.stage==="curves"?this.selected.size?"选择第二条曲线":"选择第一条曲线":direct.stage==="branch"?"选择局部圆角方向":"输入参数，Enter 接受";
      if(direct.stage==="value"){
        const unit=direct.unit??context.viewport.currentLengthUnit?.()??"mm";
        fields=[{label:this.kind==="fillet"?"R":"距离",value:this.fields[0],unit:"length",displayUnit:unit,placeholder:`5 ${unit}`}];
        if(this.kind==="chamfer"&&this.chamferMode!=="EQUAL")fields.push({label:this.chamferMode==="TWO_LENGTHS"?"第二距离":"角度",value:this.fields[1],unit:this.chamferMode==="TWO_LENGTHS"?"length":"angle"});
        const dimension=this.candidate?.dimensions?.[this.field];
        const layout=dimension?buildSketchConstraintLayout(dimension,[...this.baseline,...this.candidate!.entities,...this.candidate!.dimensionEntities??[]]):undefined;
        input={id:direct.inputId,fieldIndex:this.field,anchor:layout?.label?context.viewport.projectSketchPoint?.(layout.label.position):direct.anchor,modelAnchor:layout?.label?.position,dimension:!!layout?.label};
      }
    } else if(direct.kind==="split"){role=direct.first?"在同一圆上选择第二个分割位置":"在曲线上选择分割位置";required=direct.first?2:undefined;}
    else if(direct.kind==="extend")role=direct.stage==="curve"?"选择要延伸的曲线":"移动预览，点击目标位置或边界";
    else role="选择要删除的曲线段";
    const canConfirm=!busy&&(direct.kind==="mirror"?direct.stage==="sources"&&this.selected.size>0:direct.kind==="corner"&&direct.stage==="value");
    context.viewport.setSketchCommandState?.({toolId:this.id,operation:direct.kind==="trim"?"修剪":labels[this.kind],phase:this.phase,role,selectedIds:[...this.selected],references:[...this.cornerRefs,...this.extendReference?[this.extendReference]:[],...this.axis?[this.axis]:[]],fields,options:this.kind==="mirror"?[{name:"mirrorMode",label:"镜像关系",value:this.mirrorMode,choices:[{value:"LINKED",label:"关联镜像"},{value:"INDEPENDENT",label:"独立副本"}]}]:this.kind==="chamfer"?[{name:"chamferMode",label:"倒角定义",value:this.chamferMode,choices:[{value:"EQUAL",label:"等距离"},{value:"TWO_LENGTHS",label:"双距离"},{value:"LENGTH_ANGLE",label:"距离与角度"}]}]:[],canConfirm,next:role,error:this.error,presentation:"inline",input,preview:this.previewStatus==="pending"?"pending":this.previewStatus==="failed"?"unavailable":this.verifiedCandidateGeneration===this.candidateGeneration?"authoritative":"approximate",count:{accepted:this.selected.size,required},completion,recovery:this.recovery(context)});
  }
  private directSelection(selections:readonly SelectionItem[],context:ToolContext,source:SelectionInputSource):SelectionInputResult {
    if(this.phase==="committing"||this.phase==="unknown")return SelectionInputResult.Rejected;
    if(!selections.length){if(source!=="activation"&&this.direct?.kind==="mirror"&&this.direct.stage==="sources"){this.selected.clear();this.refreshDirectSources(context);}return SelectionInputResult.Accepted;}
    const entities=localSketchSelection(selections,context),direct=this.direct!;
    if(entities.length!==new Set(selections.map(s=>s.entityId)).size||entities.length!==selections.length){this.error="选择中包含其它文档、草图或不可编辑对象，请重新选择";this.publishDirect(context);return SelectionInputResult.Rejected;}
    if(direct.kind==="mirror"){
      if(source==="activation"){
        this.selected=new Set(entities.map(e=>e.id));this.baseline=structuredClone(entities);this.scope=context.viewport.currentSketchIdentity?.();this.operationID=randomUUID();this.phase="definition";
        this.direct={kind:"mirror",route:"source-first",stage:"axis"};this.highlight(context);this.publishDirect(context);return SelectionInputResult.Accepted;
      }
      if(direct.stage==="axis"){
        if(entities.length!==1||entities[0].kind!=="LINE"){this.error="镜像轴需要一条直线或已有轴参考";this.publishDirect(context);return SelectionInputResult.Rejected;}
        this.acceptMirrorAxis({target:"ENTITY",entityId:entities[0].id,subElement:"DIRECTION"},context);return SelectionInputResult.Accepted;
      }
      if(entities.some(e=>e.id===this.axis?.entityId)){this.error="镜像轴不计入源对象集合";this.publishDirect(context);return SelectionInputResult.Rejected;}
      this.selected=new Set(entities.map(e=>e.id));this.refreshDirectSources(context);return SelectionInputResult.Accepted;
    }
    if(direct.kind==="corner"){
      if(entities.length>2||entities.some(e=>!this.cornerSelectable(e))){this.error="请选择两条不同的适用曲线，不使用超量或混合预选";this.publishDirect(context);return SelectionInputResult.Rejected;}
      this.selected=new Set(entities.map(e=>e.id));direct.clicks=entities.map(e=>curvePoint(e,.5)!);this.prepareDirectCorner(context);return SelectionInputResult.Accepted;
    }
    // A split/trim activation never invents positions or submits a seed.
    if(entities.length!==1||entities[0].kind==="POINT"){this.error="请选择一条曲线";this.publishDirect(context);return SelectionInputResult.Rejected;}
    this.publishDirect(context);return SelectionInputResult.Accepted;
  }
  private refreshDirectSources(context:ToolContext):void {
    this.baseline=structuredClone((context.viewport.currentSketchEntities?.()??[]).filter(e=>this.selected.has(e.id)));
    this.scope=context.viewport.currentSketchIdentity?.();this.operationID??=randomUUID();this.error=undefined;this.highlight(context);
    if(this.axis&&this.baseline.length)this.preview(context);else context.viewport.clearToolPreview();this.publishDirect(context);
  }
  private acceptMirrorAxis(reference:SketchGeometryRef,context:ToolContext):void {
    const direct=this.direct;if(direct?.kind!=="mirror")return;
    this.axis={...reference};this.scope=context.viewport.currentSketchIdentity?.();this.operationID??=randomUUID();this.phase="definition";this.error=undefined;
    context.viewport.showReferencePreview(reference);
    if(direct.route==="source-first"){this.preview(context);this.confirm(context);}
    else {direct.stage="sources";this.publishDirect(context);}
  }
  private cornerSelectable(entity:SketchEntity):boolean {
    return entity.role!=="CONSTRUCTION"&&(this.kind==="chamfer"?entity.kind==="LINE":["LINE","ARC"].includes(entity.kind));
  }
  private prepareDirectCorner(context:ToolContext):void {
    const direct=this.direct;if(direct?.kind!=="corner")return;
    this.baseline=structuredClone((context.viewport.currentSketchEntities?.()??[]).filter(e=>this.selected.has(e.id)));
    // Preserve the order of accepted clicks independently of model array order.
    this.baseline=[...this.selected].map(id=>this.baseline.find(e=>e.id===id)!);
    this.scope=context.viewport.currentSketchIdentity?.();this.phase=this.baseline.length?"definition":"selection";this.error=undefined;this.highlight(context);
    if(this.baseline.length!==2){direct.stage="curves";this.publishDirect(context);return;}
    this.operationID??=randomUUID();
    const intent=cornerClickIntent(this.baseline,direct.clicks);
    if(intent){this.cornerRefs=intent.references;this.cornerPoint=intent.point;direct.stage="value";direct.unit??=context.viewport.currentLengthUnit?.()??"mm";this.fields[0]||=this.recentCornerSource;this.previewDirectCorner(context);
      const arc=this.candidate?.entities.find(e=>e.kind==="ARC");const location=arc?.center?[arc.center.x,arc.center.y] as Vec2:this.cornerPoint;
      direct.anchor??=location?context.viewport.projectSketchPoint?.(location):undefined;}
    else {direct.stage="branch";this.cornerRefs=this.baseline.map((e,index)=>{const a=curvePoint(e,0)!,b=curvePoint(e,1)!,click=direct.clicks[index];return {target:"ENTITY",entityId:e.id,subElement:Math.hypot(click[0]-a[0],click[1]-a[1])<Math.hypot(click[0]-b[0],click[1]-b[1])?"START":"END"};});}
    this.publishDirect(context);
  }
  private previewDirectCorner(context:ToolContext):void {
    try {const operation=this.cornerOperation(context);if(operation)this.showCandidate(context,operation);else context.viewport.clearToolPreview();}
    catch {this.candidateAbort?.abort();this.candidateGeneration++;this.acceptPendingCandidate=false;this.previewStatus="idle";this.candidate=undefined;context.viewport.clearToolPreview();}
  }
  private directAction(action:SketchCommandAction,context:ToolContext):boolean {
    const direct=this.direct!;
    if(action.type==="cancel")return false;
    if(this.phase==="committing")return true;
    if(action.type==="retry"&&this.pendingOperations){this.commit(context,this.pendingOperations);return true;}
    if(this.phase==="unknown")return true;
    if(action.type==="input"){this.field=action.index===1?1:0;this.publishDirect(context);return true;}
    if(action.type==="placement"&&direct.kind==="corner"){
      const position=context.viewport.sketchPlacementPoint(action.position[0],action.position[1]);
      if(position){direct.dimensionAnchors??=[];direct.dimensionAnchors[this.field]=[...position];this.previewDirectCorner(context);this.publishDirect(context);}return true;
    }
    if(action.type==="field"){
      if(action.index<0||action.index>1)return true;this.fields[action.index]=action.value;this.error=undefined;
      if(direct.kind==="corner")this.previewDirectCorner(context);this.publishDirect(context);return true;
    }
    if(action.type==="back"){
      this.candidateAbort?.abort();this.candidateGeneration++;this.previewStatus="idle";context.viewport.clearToolPreview();
      if(direct.kind==="corner") {const ids=[...this.selected];this.selected.delete(ids.at(-1)!);direct.clicks.pop();direct.stage="curves";direct.dimensionAnchors=undefined;this.cornerRefs=[];this.cornerPoint=undefined;this.operationID=undefined;this.prepareDirectCorner(context);}
      else if(direct.kind==="mirror"){if(direct.stage==="sources"&&this.selected.size){this.selected.delete([...this.selected].at(-1)!);this.refreshDirectSources(context);}else {this.axis=undefined;direct.stage="axis";context.viewport.clearReferencePreview();}}
      else if(direct.kind==="extend"){direct.stage="curve";this.selected.clear();this.baseline=[];this.extendReference=undefined;this.phase="selection";context.viewport.clearReferencePreview();}
      else if(direct.kind==="split"){direct.first=undefined;this.selected.clear();this.baseline=[];this.phase="selection";context.viewport.clearReferencePreview();}
      this.error=undefined;this.publishDirect(context);return true;
    }
    if(action.type==="confirm"){
      if(direct.kind==="mirror"&&direct.stage==="sources"&&this.selected.size)this.confirm(context);
      if(direct.kind==="corner"&&direct.stage==="value"){
        try {const operation=this.cornerOperation(context);if(operation)this.commit(context,[operation]);}
        catch(error){this.error=error instanceof Error?error.message:String(error);this.publishDirect(context);}
      }
      return true;
    }
    if(action.type==="option"&&(action.name==="mirrorMode"||action.name==="chamferMode")){
      const allowed=action.name==="mirrorMode"?["LINKED","INDEPENDENT"]:["EQUAL","TWO_LENGTHS","LENGTH_ANGLE"];
      if(!allowed.includes(action.value))return true;
      if(action.name==="mirrorMode")this.mirrorMode=action.value as typeof this.mirrorMode;else {this.chamferMode=action.value as typeof this.chamferMode;if(direct.kind==="corner")direct.dimensionAnchors=undefined;}
      this.error=undefined;if(direct.kind==="corner"&&direct.stage==="value")this.previewDirectCorner(context);else if(direct.kind==="mirror"&&this.axis&&this.selected.size)this.preview(context);
      this.publishDirect(context);return true;
    }
    if(action.type==="option")return false;
    if(action.type==="release"){this.toggleReferenceRelease(context);return true;}
    return true;
  }
  private directClick(event:CadPointerEvent,context:ToolContext):void {
    const direct=this.direct!,target=context.viewport.sketchPlacementPoint(event.x,event.y);
    if(direct.kind==="mirror"){
      if(direct.stage==="axis"){
        const reference=context.viewport.sketchReferenceAt(event.x,event.y,"LINE",undefined,ref=>ref.target!=="EXTERNAL");
        if(reference)this.acceptMirrorAxis(reference,context);else {this.error="请选择当前草图的直线、构造线或轴";this.publishDirect(context);}return;
      }
      const selection=(context.viewport.sketchEntityAt??context.viewport.selectionAt)(event.x,event.y),entity=selection&&localSketchSelection([selection],context)[0];
      if(!entity)return;
      if(entity.id===this.axis?.entityId){this.error="镜像轴不计入源对象集合";this.publishDirect(context);return;}
      if(this.selected.has(entity.id))this.selected.delete(entity.id);else this.selected.add(entity.id);this.refreshDirectSources(context);return;
    }
    if(direct.kind==="corner"){
      if(direct.stage==="value")return;
      if(direct.stage==="branch"){if(target){this.cornerPoint=[...target];direct.stage="value";direct.unit??=context.viewport.currentLengthUnit?.()??"mm";direct.anchor??=context.viewport.projectSketchPoint?.(target);this.previewDirectCorner(context);this.publishDirect(context);}return;}
      const reference=context.viewport.sketchReferenceAt(event.x,event.y,"CURVE",undefined,ref=>ref.target==="ENTITY"&&(context.viewport.currentSketchEntities?.()??[]).some(e=>e.id===ref.entityId&&this.cornerSelectable(e)));
      if(!reference?.entityId||!target)return;
      const ids=[...this.selected],index=ids.indexOf(reference.entityId);
      if(index>=0){this.selected.delete(reference.entityId);direct.clicks.splice(index,1);}else {this.selected.add(reference.entityId);direct.clicks.push([...target]);}
      this.prepareDirectCorner(context);return;
    }
    if(direct.kind==="extend"){this.directExtend(event,context,true);return;}
    if(direct.kind==="trim"){this.selectIntervalTarget(event,context,true);return;}
    const reference=context.viewport.sketchReferenceAt(event.x,event.y,"CURVE",undefined,ref=>ref.target==="ENTITY"&&(context.viewport.currentSketchEntities?.()??[]).some(e=>e.id===ref.entityId&&e.kind!=="POINT"));
    const entity=reference?.entityId&&(context.viewport.currentSketchEntities?.()??[]).find(e=>e.id===reference.entityId);
    if(!entity||!target)return;
    const parameter=selectedCurveParameter(entity,target);if(parameter===undefined||!Number.isFinite(parameter))return;
    const periodic=entity.kind==="CIRCLE"||entity.kind==="ELLIPSE"||entity.kind==="SPLINE"&&entity.closed;
    if(direct.first&&direct.first.entityId!==entity.id){this.error="第二个分割位置必须在同一条闭合曲线上";this.publishDirect(context);return;}
    if(!periodic&&!(parameter>0&&parameter<1)){this.error="请选择曲线内部位置，已有端点不生成新段";this.publishDirect(context);return;}
    if(periodic&&!direct.first){direct.first={entityId:entity.id,parameter};this.selected=new Set([entity.id]);this.baseline=[structuredClone(entity)];this.scope=context.viewport.currentSketchIdentity?.();this.phase="definition";context.viewport.showPointPreview(target);this.publishDirect(context);return;}
    const parameters=direct.first?[direct.first.parameter,parameter]:[parameter];
    if(direct.first){const first=curvePoint(entity,direct.first.parameter)!;if(Math.hypot(first[0]-target[0],first[1]-target[1])<SKETCH_INPUT_POLICY.minimumGeometryLength){this.error="请选择与第一个分割点不同的位置";this.publishDirect(context);return;}}
    this.selected=new Set([entity.id]);this.baseline=[structuredClone(entity)];this.scope=context.viewport.currentSketchIdentity?.();this.phase="definition";this.operationID??=randomUUID();this.commit(context,[{type:"SPLIT_ENTITY",operationId:this.operationID,entityIds:[entity.id],parameters,...(this.detach.length?{detachConstraintIds:[...this.detach]}:{})}]);
  }
  private directExtend(event:CadPointerEvent,context:ToolContext,accept:boolean):void {
    const direct=this.direct;if(direct?.kind!=="extend")return;
    const target=context.viewport.sketchPlacementPoint(event.x,event.y);
    if(!target)return;
    const reference=context.viewport.sketchReferenceAt(event.x,event.y,"CURVE",undefined,ref=>direct.stage==="curve"?
      ref.target==="ENTITY"&&(context.viewport.currentSketchEntities?.()??[]).some(e=>e.id===ref.entityId&&["LINE","ARC","ELLIPTICAL_ARC"].includes(e.kind)):
      ref.entityId!==this.baseline[0]?.id);
    if(direct.stage==="curve"){
      if(reference)context.viewport.showReferencePreview(reference);else context.viewport.clearReferenceHover?.();
      if(!accept||!reference?.entityId)return;
      const entity=context.viewport.currentSketchEntities?.().find(e=>e.id===reference.entityId);if(!entity)return;
      const a=curvePoint(entity,0)!,b=curvePoint(entity,1)!;
      this.extendReference={target:"ENTITY",entityId:entity.id,subElement:Math.hypot(target[0]-a[0],target[1]-a[1])<Math.hypot(target[0]-b[0],target[1]-b[1])?"START":"END"};
      this.baseline=[structuredClone(entity)];this.selected=new Set([entity.id]);this.scope=context.viewport.currentSketchIdentity?.();this.operationID=randomUUID();
      this.phase="definition";direct.stage="target";this.highlight(context);this.publishDirect(context);return;
    }
    const operation:SketchOperation={type:"EXTEND_ENTITY",operationId:this.operationID!,entityIds:[this.baseline[0].id],firstReference:this.extendReference!,
      boundaryIds:reference?.entityId?[reference.entityId]:[],point:point(target),...(this.detach.length?{detachConstraintIds:[...this.detach]}:{})};
    if(reference)context.viewport.showReferencePreview(reference,[this.extendReference!]);else context.viewport.clearReferenceHover?.();
    if(accept){this.showCandidate(context,operation,true);if(this.previewStatus==="ready")this.commit(context,[operation]);}
    else {
      // Free-support preview responds immediately. Snapped boundaries are
      // resolved by the shared read-only production preview after coalescing.
      const candidate=buildSketchEditPreview({entities:context.viewport.currentSketchEntities?.()??[],operation:{...operation,boundaryIds:[]}});
      if(candidate.status==="APPROXIMATE")context.viewport.showSketchEditCandidate?.(candidate);
      else context.viewport.clearToolPreview();
      if(this.extendHoverTimer)clearTimeout(this.extendHoverTimer);
      this.candidateAbort?.abort();const generation=++this.candidateGeneration;
      if(reference&&context.viewport.previewSketchOperations){
        const abort=this.candidateAbort=new AbortController(),scope=scopeKey(context.viewport.currentSketchIdentity?.());
        this.extendHoverTimer=setTimeout(()=>{
          if(abort.signal.aborted||generation!==this.candidateGeneration)return;
          void context.viewport.previewSketchOperations!([operation],abort.signal).then(result=>{
            if(abort.signal.aborted||generation!==this.candidateGeneration||scope!==scopeKey(context.viewport.currentSketchIdentity?.()))return;
            const original=new Map((context.viewport.currentSketchEntities?.()??[]).map(e=>[e.id,JSON.stringify(e)]));
            context.viewport.showSketchEditCandidate?.({...candidate,operation,entities:result.entities.filter(e=>JSON.stringify(e)!==original.get(e.id)),status:"APPROXIMATE",diagnostic:"边界交点预览"});
          },()=>{/* Hover failure is transient; accepted intent reports errors. */});
        },80);
      }
    }
  }
  private directHover(event:CadPointerEvent,context:ToolContext):void {
    const direct=this.direct!;
    if(direct.kind==="corner"){
      if(direct.stage!=="curves")return;
      const reference=context.viewport.sketchReferenceAt(event.x,event.y,"CURVE",undefined,ref=>ref.target==="ENTITY"&&(context.viewport.currentSketchEntities?.()??[]).some(e=>e.id===ref.entityId&&this.cornerSelectable(e)));
      if(reference)context.viewport.showReferencePreview(reference,this.baseline.map(entity=>({target:"ENTITY",entityId:entity.id,subElement:"WHOLE"})));else context.viewport.clearReferenceHover?.();return;
    }
    if(direct.kind==="mirror"){
      if(direct.stage==="axis"){
        const candidate=context.viewport.sketchReferenceAt(event.x,event.y,"LINE",undefined,ref=>ref.target!=="EXTERNAL");
        if(candidate){context.viewport.showReferencePreview(candidate);const axis=candidate.target==="SKETCH_X_AXIS"||candidate.target==="SKETCH_Y_AXIS"?{start:[0,0] as Vec2,end:(candidate.target==="SKETCH_X_AXIS"?[1,0]:[0,1]) as Vec2}:(()=>{const e=context.viewport.currentSketchEntities?.().find(e=>e.id===candidate.entityId);return e?.start&&e.end?{start:[e.start.x,e.start.y] as Vec2,end:[e.end.x,e.end.y] as Vec2}:undefined;})();if(axis&&this.baseline.length)context.viewport.showSketchEntityPreview?.(sketchEditPreview(this.baseline,[0,0],[0,0],0,axis));}
        else {context.viewport.clearReferencePreview();context.viewport.clearToolPreview();}
      } else {
        const reference=context.viewport.sketchReferenceAt(event.x,event.y,"ENTITY",undefined,ref=>ref.target==="ENTITY"&&ref.entityId!==this.axis?.entityId&&(context.viewport.currentSketchEntities?.()??[]).some(entity=>entity.id===ref.entityId));
        if(reference)context.viewport.showReferencePreview(reference,this.axis?[this.axis]:[]);else {context.viewport.clearReferenceHover?.();if(this.axis)context.viewport.showReferencePreview(this.axis,[this.axis]);}
      }
      return;
    }
    if(direct.kind==="extend"){this.directExtend(event,context,false);return;}
    if(direct.kind==="trim"){this.selectIntervalTarget(event,context,false);return;}
    const reference=context.viewport.sketchReferenceAt(event.x,event.y,"CURVE",undefined,ref=>ref.target==="ENTITY"&&(!direct.first||ref.entityId===direct.first.entityId));
    const target=context.viewport.sketchPlacementPoint(event.x,event.y);if(reference&&target)context.viewport.showPointPreview(target);
    else {context.viewport.clearToolPreview();context.viewport.clearReferenceHover?.();if(direct.first){const entity=(context.viewport.currentSketchEntities?.()??[]).find(entity=>entity.id===direct.first!.entityId),point=entity&&curvePoint(entity,direct.first.parameter);if(point)context.viewport.showPointPreview(point);}}
  }
  private publish(context:ToolContext):void {
    if(this.direct){this.publishDirect(context);return;}
    const defining=this.armed;
    const fields=this.kind==="move"||this.kind==="copy"?["X (mm)","Y (mm)"]:
      this.kind==="trim"&&this.advancedTrim?["起点参数（高级）","终点参数（高级）"]:
      this.kind==="chamfer"?["第一长度 (mm)",this.chamferMode==="LENGTH_ANGLE"?"角度 (°)":"第二长度 (mm)"]:
      ["fillet","offset","rotate","split","quick_trim", "spline_insert","spline_delete"].includes(this.kind)?[({fillet:"半径 (mm)",offset:"距离 (mm)",rotate:"角度 (°)",split:"分割参数（高级）",quick_trim:"命中参数（高级）"} as Record<string,string>)[this.kind]??"点位置/结点参数"]:[];
    const options:SketchCommandState["options"]=[];
    const option=(name:string,label:string,value:string,choices:[string,string][])=>options.push({name,label,value,choices:choices.map(([value,label])=>({value,label}))});
    if(["copy","move","rotate"].includes(this.kind)){option("policy","副本约束",this.policy,[["GEOMETRY_ONLY","仅几何"],["INTERNAL","保留内部约束"]]);if(this.kind!=="copy")option("copy","变换结果",String(this.copy),[["false","移动原对象"],["true","生成副本"]]);}
    if(this.kind==="mirror")option("mirrorMode","镜像关系",this.mirrorMode,[["LINKED","关联镜像"],["INDEPENDENT","独立副本"]]);
    if(this.kind==="quick_trim"||this.kind==="trim") {
      option("trimMode","区间操作",this.trimMode,[["DELETE_HIT","删除命中段"],["KEEP_HIT","保留命中段"],["BREAK","分割保留各段"]]);
      option("boundaryMode","边界",this.boundaryMode,[["automatic","当前草图交点"],["explicit","指定边界"]]);
      if(this.kind==="trim")option("advancedTrim","输入方式",this.advancedTrim?"advanced":"visual",[["visual","点击命中区间"],["advanced","高级参数范围"]]);
    }
    if(this.kind==="fillet"||this.kind==="chamfer")option("cornerTrimMode","邻接几何",this.cornerTrimMode,[["TRIM","修剪两侧"],["KEEP","保留原元素"]]);
    if(this.kind==="chamfer")option("chamferMode","倒角定义",this.chamferMode,[["EQUAL","等长度"],["TWO_LENGTHS","双长度"],["LENGTH_ANGLE","长度加角度"]]);
    if(this.kind==="offset")option("offsetMode","连接方式",this.offsetMode,[["MITER","斜接"],["ROUND","圆角"]]);
    const role=!defining?"选择源对象":this.kind==="mirror"?this.axis?"预览与确认":"选择镜像轴":this.kind==="extend"?!this.extendReference?"选择端点":!this.boundaries.length?"选择边界":"选择目标交点分支":this.kind==="fillet"||this.kind==="chamfer"?this.cornerRefs.length<2?"选择角点端部":"选择内外侧分支":this.kind==="trim"||this.kind==="quick_trim"?"命中曲线区间":"定义参数与位置";
    const canConfirm=this.phase!=="committing"&&this.phase!=="unknown"&&this.previewStatus!=="pending"&&this.previewStatus!=="failed"&&(!defining?this.selected.size>0||this.batch.length>0:this.kind==="mirror"?Boolean(this.axis):this.kind==="fillet"||this.kind==="chamfer"?Boolean(this.cornerPoint&&this.cornerRefs.length===2):true);
    context.viewport.setSketchCommandState?.({toolId:this.id,operation:labels[this.kind],presentation:"inline",phase:this.phase,role,count:{accepted:this.selected.size},selectedIds:[...this.selected],references:[...this.cornerRefs,...this.boundaries,...this.axis?[this.axis]:[]],input:defining&&fields.length?{id:this.operationID!,fieldIndex:this.field}:undefined,recovery:this.recovery(context),completion:!defining&&this.selected.size?{label:"完成"}:defining&&!fields.length&&canConfirm?{label:"执行"}:undefined,fields:(defining?fields:[]).map((label,index)=>({label,unit:this.kind==="rotate"?"angle":this.kind==="offset"||this.kind==="move"||this.kind==="copy"?"length":"scalar",value:this.fields[index],placeholder:this.kind==="fillet"||this.kind==="chamfer"||this.kind==="offset"?"5":this.kind==="split"||this.kind==="quick_trim"?"0.5":"0"})),options,canConfirm,next:!defining?"点选可增减对象；点击下一步或 Enter":role,error:this.error,preview:this.previewStatus==="pending"?"pending":this.previewStatus==="failed"?"unavailable":this.verifiedCandidateGeneration===this.candidateGeneration?"authoritative":"approximate"});
  }
  commandAction(action:SketchCommandAction,context:ToolContext):void {
    if(this.direct&&this.directAction(action,context))return;
    if(action.type==="cancel"){this.cancel(context);context.viewport.finishToolUse(true);return;}
    if(this.phase==="committing")return;
    if(action.type==="retry"&&this.pendingOperations){this.commit(context,this.pendingOperations);return;}
    if(action.type==="retry"&&this.armed){this.preview(context);this.prompt(context);return;}
    if(this.phase==="unknown")return;
    if(action.type==="back"){this.candidateAbort?.abort();this.candidateGeneration++;this.acceptPendingCandidate=false;this.previewStatus="idle";this.phase="selection";this.axis=undefined;this.cornerRefs=[];this.cornerPoint=undefined;context.viewport.clearToolPreview();context.viewport.clearReferencePreview();this.highlight(context);this.prompt(context);return;}
    if(action.type==="confirm"){if(!this.armed)this.arm(context);else this.confirm(context);return;}
    if(action.type==="field"){if(this.kind==="trim")this.advancedTrim=true;if(action.index<0||action.index>1)return;this.fields[action.index]=action.value;this.error=undefined;if(!this.applyFields()){this.candidateAbort?.abort();this.candidateGeneration++;this.acceptPendingCandidate=false;this.previewStatus="idle";this.error="输入必须为有限数值";context.viewport.clearToolPreview();this.publish(context);return;}if(this.armed)this.preview(context);this.prompt(context);return;}
    if(action.type==="option") {
      if(action.name==="copy"){this.copy=action.value==="true";if(this.armed)this.preview(context);this.prompt(context);return;}
      if(action.name==="advancedTrim"){this.advancedTrim=action.value==="advanced";this.prompt(context);return;}
      const allowed:Record<string,string[]>={policy:["GEOMETRY_ONLY","INTERNAL"],mirrorMode:["LINKED","INDEPENDENT"],cornerTrimMode:["TRIM","KEEP"],chamferMode:["EQUAL","TWO_LENGTHS","LENGTH_ANGLE"],offsetMode:["MITER","ROUND"],trimMode:["DELETE_HIT","KEEP_HIT","BREAK"],boundaryMode:["automatic","explicit"]};
      if(!allowed[action.name]?.includes(action.value))return;
      (this as unknown as Record<string,string>)[action.name]=action.value;
      if(action.name==="boundaryMode")this.boundaries=[];
      this.error=undefined;if(this.armed)this.preview(context);this.prompt(context);return;
    }
    if(action.type==="release"){this.toggleReferenceRelease(context);return;}
    if(action.type==="add-batch")this.keyDown({key:"b",editableTarget:false,state:{modifiers:{}}} as CadKeyboardEvent,context);
  }
  activate(context:ToolContext):void{
    this.phase="selection";
    this.direct=this.kind==="mirror"?{kind:"mirror",route:"axis-first",stage:"axis"}:this.kind==="fillet"||this.kind==="chamfer"?{kind:"corner",stage:"curves",clicks:[],inputId:randomUUID()}:this.kind==="extend"?{kind:"extend",stage:"curve"}:this.kind==="split"?{kind:"split"}:this.kind==="trim"||this.kind==="quick_trim"?{kind:"trim"}:undefined;
    if(this.direct?.kind==="corner")this.fields[0]=this.recentCornerSource;
    if(this.direct?.kind==="trim")this.trimMode="DELETE_HIT";
    this.prompt(context);
  }

  selectionInput(selections:readonly SelectionItem[],context:ToolContext,source:SelectionInputSource="activation"):SelectionInputResult{
    if(this.phase==="committing"||this.phase==="unknown")return SelectionInputResult.Rejected;
    if(this.direct)return this.directSelection(selections,context,source);
    if(this.armed){
      if(this.kind==="mirror") {
        const axis=localSketchSelection(selections,context).find(e=>e.kind==="LINE");
        if(axis){this.axis={target:"ENTITY",entityId:axis.id,subElement:"DIRECTION"};this.preview(context);this.prompt(context);return SelectionInputResult.Accepted;}
      }
      context.viewport.setToolPrompt("当前阶段只接受指定对象角色；命令保持激活");return SelectionInputResult.Rejected;
    }
    if(source==="selection"&&!selections.length){this.selected.clear();this.highlight(context);this.prompt(context);return SelectionInputResult.Accepted;}
    const entities=localSketchSelection(selections,context);
    if(!entities.length||entities.length!==selections.length){
      this.error="选择包含当前草图以外或不可编辑的对象，请修正选择";this.publish(context);return SelectionInputResult.Rejected;
    }
    if(source==="selection")this.selected=new Set(entities.map(entity=>entity.id));else for(const entity of entities)this.selected.add(entity.id);
    this.highlight(context);if(this.kind==="construction"&&entities.length)this.arm(context);else this.prompt(context);return SelectionInputResult.Accepted;
  }
  private prompt(context:ToolContext):void{
    if(this.direct){this.publishDirect(context);return;}
    this.publish(context);
    if(!this.armed){context.viewport.setToolPrompt(`${labels[this.kind]}：选择本地几何（已选 ${this.selected.size}），Enter 确认选择${this.batch.length?`或提交 ${this.batch.length} 个角点批次`:""}；Esc/右键退出`);return;}
    if(this.kind.startsWith("spline_")){
      const entity=this.baseline[0],control=entity.mode==="CONTROL",count=splineEditablePoints(entity).length,start=entity.parameterStart??entity.knots?.[0]??0,end=entity.parameterEnd??entity.knots?.at(-1)??1;
      const action=this.kind==="spline_close"?`切换为${entity.closed?"开放":"闭合"}（闭合不承诺高阶连续）`:this.kind==="spline_control"?"将已求解 FIT 精确 canonical 曲线转为 CONTROL；原拟合点引用可能失效":
        control?`${this.kind==="spline_insert"?"插入":"移除"}结点 u: ${this.fields[0]||String((start+end)/2)}，真实活动域 [${start}, ${end}]；保持原曲线，禁止任意删除 pole`:
        this.kind==="spline_insert"?`在拟合点位置 ${this.fields[0]||count} 之前插入（0~${count}）；点击新拟合点坐标`:`删除拟合点 ${this.fields[0]||count-1}（0~${count-1}），或点击明确拟合点`;
      context.viewport.setToolPrompt(`${labels[this.kind]}：${action}；U 显式解除受影响点/整体引用，Enter 确认，Esc 取消`);return;
    }
    if(this.kind==="extend"){
      context.viewport.setToolPrompt(`延伸：${!this.extendReference?"点击目标曲线靠近要延伸的 START/END":!this.boundaries.length?"选择一条本地或外部只读参考边界":"点击位置选择目标交点分支，再 Enter 确认"}；U 显式解除影响，Esc 取消；内核计算交点，不无限延长`);return;
    }
    if(this.kind==="complement"||this.kind==="close"){
      context.viewport.setToolPrompt(`${labels[this.kind]}：Enter 确认；U 显式解除受影响的整体/端点关系（当前 ${this.detach.length}）；原端点不得静默重连，Esc 取消`);return;
    }
    if(this.kind==="offset"){
      context.viewport.setToolPrompt(`二维独立偏移：正值沿曲线有向左法向（线段 START→END 左侧、逆时针圆/弧向内，顺时针弧向外）；距离 mm: ${this.fields[0]||"5"}；M ${this.offsetMode==="MITER"?"斜接":"圆角连接"}；仅独立副本，无持续距离关联；Enter 确认，Esc 取消；凹角、自交与半径退化由权威计算验证`);return;
    }
    if(this.kind==="fillet"||this.kind==="chamfer"){
      const mode=this.kind==="fillet"?`半径 mm: ${this.fields[0]||"5"}`:
        `${{EQUAL:"等长",TWO_LENGTHS:"双长度",LENGTH_ANGLE:"长度加角度"}[this.chamferMode]}（M 切换）；长度 mm: ${this.fields[0]||"5"}${this.chamferMode!=="EQUAL"?`，${this.chamferMode==="TWO_LENGTHS"?"第二长度 mm":"角度°"}: ${this.fields[1]||(this.chamferMode==="TWO_LENGTHS"?"5":"45")}`:""}；Tab 字段`;
      context.viewport.setToolPrompt(`${labels[this.kind]}：${this.cornerRefs.length<2?`点击所选两条曲线靠近要裁切的端部（已选 ${this.cornerRefs.length}/2）`:this.cornerPoint?"端部与分支已选择，输入预览待权威验证":"点击位置选择内外侧分支"}；${mode}；K ${this.cornerTrimMode==="TRIM"?"修剪两侧（原支撑与尺寸转辅助保留）":"保留原轮廓（结果作为辅助）"}；B 加入批次，Enter 原子提交（已加入 ${this.batch.length}），Esc 取消`);return;
    }
    const inputs=this.kind==="move"||this.kind==="copy"?`X mm: ${this.fields[0]||this.translation[0]}，Y mm: ${this.fields[1]||this.translation[1]}；Tab 切换`:
      this.kind==="rotate"?`角度°: ${this.fields[0]||this.angle*180/Math.PI}`:this.kind==="split"?`参数比例 0~1: ${this.fields[0]||"0.5"}`:this.kind==="trim"?`保留范围 0~1: ${this.fields[0]||"0"} → ${this.fields[1]||"1"}；Tab 切换`:
      this.kind==="quick_trim"?`边界 ${this.boundaries.length} 条；选择边界，再点击目标曲线区间或输入命中比例 ${this.fields[0]||"0.5"}；Q 模式: ${{DELETE_HIT:"删除命中段",KEEP_HIT:"保留命中段",BREAK:"打断保留各段"}[this.trimMode]}`:"选择直线/辅助线作为轴，或 X/Y 内置轴";
    if(["split","trim","quick_trim"].includes(this.kind)){const conversion=this.baseline[0]?.kind==="SPLINE"&&this.baseline[0].mode!=="CONTROL"?"；FIT 将使用精确子曲线转为 CONTROL（不重新拟合），拟合点引用需明确解除或操作拒绝":"";context.viewport.setToolPrompt(`${labels[this.kind]}：${inputs}${conversion}；Enter 确认，Esc 取消；交点由权威内核计算；U ${this.detach.length?`明确解除 ${this.detach.length} 个整体关系（再按可取消）`:"预览整体关系释放影响"}`);return;}
    const gesture=this.kind==="move"||this.kind==="copy"?"单击基点和目标点":this.kind==="rotate"?"单击中心、基准点和目标点":"";
    context.viewport.setToolPrompt(`${labels[this.kind]}：${gesture}；${inputs}；Enter 提交，Esc 取消；${this.kind==="mirror"&&this.mirrorMode==="LINKED"?"关联镜像（M 切换为独立）；正式反射关系，不复制驱动尺寸":this.kind==="mirror"||this.kind==="copy"||this.copy?`${this.kind==="mirror"?"独立镜像（M 切换为关联）；":""}${this.policy==="INTERNAL"?"复制内部约束":"仅几何副本（不复制连接和尺寸）"}（I 切换）`:"保留约束，冲突时拒绝"}；Shift+C 切换带复制`);
  }
  private releasableRelations(context:ToolContext):SketchConstraint[] {
    if(!(this.kind.startsWith("spline_")||["split","trim","quick_trim","extend","complement","close"].includes(this.kind)))return [];
      return (context.viewport.currentSketchConstraints?.()??[]).filter(constraint=>constraint.references.some(reference=>reference.target==="ENTITY"&&reference.entityId===this.baseline[0]?.id&&( ["WHOLE","DIRECTION"].includes(reference.subElement)||["complement","close"].includes(this.kind)&&["START","END"].includes(reference.subElement)||this.kind==="extend"&&reference.subElement===this.extendReference?.subElement||(this.kind.startsWith("spline_")||["split","trim","quick_trim"].includes(this.kind)&&this.baseline[0]?.kind==="SPLINE"&&this.baseline[0]?.mode!=="CONTROL")&&reference.subElement==="CONTROL")));
  }
  private recovery(context:ToolContext):SketchCommandState["recovery"] {
    const count=this.releasableRelations(context).length;
    return this.phase==="failed"&&count?{label:this.detach.length?"撤回关系解除":`解除受影响关系（${count}）`,action:"release"}:undefined;
  }
  private toggleReferenceRelease(context:ToolContext):void {
    const affected=this.releasableRelations(context);
    this.detach=this.detach.length?[]:affected.map(c=>c.id);
    this.error=this.detach.length?`下次操作将解除：${affected.map(c=>constraintDefinition(c.kind).label).join("、")}；可撤回或重新选择位置`:"已撤回关系解除，按原关系验证";
    this.publish(context);
  }
  private highlight(context:ToolContext):void{
    const scope=context.viewport.currentSketchIdentity?.();
    const entities=(context.viewport.currentSketchEntities?.()??[]).filter(entity=>this.selected.has(entity.id));
    if(!scope)return;
    context.viewport.retainSelections?.(entities.map(entity=>({kind:"visual",id:`${this.scope?.occurrencePath||"root"}:${context.viewport.currentSketchIdentity?.()?.sketchId}:${entity.id}`,entityId:entity.id,visualType:entity.kind==="POINT"?"POINT":"CURVE",featureId:scope.sketchId,ownerDocumentId:scope.documentId,occurrencePath:context.viewport.currentSketchIdentity?.()?.occurrencePath})));
  }
  private validScope(context:ToolContext):boolean{
    if((!this.armed&&!this.batch.length)||scopeKey(this.scope)===scopeKey(context.viewport.currentSketchIdentity?.()))return true;
    this.cancel(context);context.viewport.setToolPrompt("草图版本已变化；请重新选择并预览编辑");return false;
  }
  private arm(context:ToolContext):boolean{
    if(this.phase==="committing"||this.phase==="unknown")return false;
    const entities=(context.viewport.currentSketchEntities?.()??[]).filter(entity=>this.selected.has(entity.id));
    if(!entities.length){context.viewport.setToolPrompt("先选择本地草图几何；外部几何保持只读，可先显式分离");return false;}
    if(["split","trim","quick_trim"].includes(this.kind)&&(entities.length!==1||entities[0].kind==="POINT")){context.viewport.setToolPrompt("曲线拓扑编辑只接受一条本地真实曲线；独立点无参数区间");return false;}
    if((this.kind==="fillet"||this.kind==="chamfer")&&(entities.length!==2||entities.some(entity=>this.kind==="chamfer"?entity.kind!=="LINE":!["LINE","ARC"].includes(entity.kind)))){
      context.viewport.setToolPrompt(this.kind==="fillet"?"圆角选择两条不同本地线段/圆弧；样条及椭圆圆角不在当前组合":"倒角只接受两条不同本地线段");return false;
    }
    if(this.kind.startsWith("spline_")&&(entities.length!==1||entities[0].kind!=="SPLINE")){context.viewport.setToolPrompt("样条点/结点编辑只接受当前草图的一条本地样条");return false;}
    if(this.kind==="spline_control"&&(entities[0].mode==="CONTROL"||!entities[0].poles?.length)){context.viewport.setToolPrompt("精确转换需要已求解的 FIT canonical 数据；CONTROL 已在控制点模式");return false;}
    if(["extend","complement","close"].includes(this.kind)&&(entities.length!==1||(this.kind==="extend"?!["LINE","ARC","ELLIPTICAL_ARC"].includes(entities[0].kind):!["ARC","ELLIPTICAL_ARC"].includes(entities[0].kind)))){
      context.viewport.setToolPrompt(this.kind==="extend"?"延伸选择一条本地线段、圆弧或椭圆弧，并明确端点与目标边界":"补弧/闭合选择一条圆弧或椭圆弧；闭合样条使用样条专用开闭命令");return false;
    }
    if(this.kind==="offset"&&entities.some(entity=>!["LINE","CIRCLE","ARC"].includes(entity.kind))){context.viewport.setToolPrompt("恒定距离偏移只接受线段、圆、圆弧及它们的连续链；椭圆/样条不转成采样折线");return false;}
    this.baseline=structuredClone(entities);this.scope=context.viewport.currentSketchIdentity?.();this.phase="definition";this.operationID=randomUUID();this.copy=this.kind==="copy";
    if(this.kind==="delete"){this.commit(context,[{type:"DELETE_ENTITIES",entityIds:entities.map(entity=>entity.id)}]);return true;}
    if(this.kind==="construction"){const role=entities.every(entity=>entity.role==="CONSTRUCTION")?"PROFILE":"CONSTRUCTION";
      this.commit(context,entities.map(entity=>({type:"UPDATE_ENTITY_ROLE",entityId:entity.id,role})));return true;}
    if(context.viewport.previewSketchOperations&&(this.kind.startsWith("spline_")||["complement","close","offset"].includes(this.kind)))this.preview(context);
    this.prompt(context);return true;
  }
  private axisPoints(context:ToolContext):{start:Vec2;end:Vec2}|undefined{
    if(this.axis?.target==="SKETCH_X_AXIS"||this.axis?.target==="SKETCH_Y_AXIS")return{start:[0,0],end:this.axis.target==="SKETCH_Y_AXIS"?[0,1]:[1,0]};
    const entity=context.viewport.currentSketchEntities?.().find(entity=>entity.id===this.axis?.entityId);
    return entity?.kind==="LINE"&&entity.start&&entity.end?{start:[entity.start.x,entity.start.y],end:[entity.end.x,entity.end.y]}:undefined;
  }
  private preview(context:ToolContext):void{
    if(this.kind.startsWith("spline_")||["extend","complement","close","offset"].includes(this.kind)){
      // Reuse the exact submitted definition for operations without an analytic display builder.
      // Selection feedback remains separate from a computed result preview.
      if(context.viewport.previewSketchOperations){this.candidateAbort?.abort();this.candidateGeneration++;this.acceptPendingCandidate=false;this.previewStatus="idle";const operation=this.definitionOperation(context);if(operation)this.showCandidate(context,operation);else context.viewport.clearToolPreview();}
      else if(this.branchPoint)context.viewport.showPointPreview(this.branchPoint);
      else context.viewport.clearToolPreview();return;
    }
    if(this.kind==="fillet"||this.kind==="chamfer") {
      if(this.cornerPoint&&this.cornerRefs.length===2){const operation=this.cornerOperation(context);if(operation)this.showCandidate(context,operation);else {this.previewStatus="failed";this.candidateAbort?.abort();this.candidateGeneration++;context.viewport.clearToolPreview();}}return;
    }
    if(this.kind==="quick_trim"||this.kind==="trim"&&!this.advancedTrim){const operation=this.intervalOperation(context);if(operation)this.showCandidate(context,operation);return;}
    if(this.kind==="split") {
      const parameter=Number(this.fields[0]||"0.5");
      this.showCandidate(context,{type:"SPLIT_ENTITY",operationId:this.operationID??"preview",entityIds:this.baseline.map(e=>e.id),parameters:[parameter]});return;
    }
    if(this.kind==="trim"){
      const start=Number(this.fields[0]||"0"),end=Number(this.fields[1]||"1");if(!(start>=0&&end<=1&&start<end))return;
      const entity=structuredClone(this.baseline[0]);
      if(entity.kind==="LINE"){const first=curvePoint(entity,start),last=curvePoint(entity,end);if(first&&last){entity.start=point(first);entity.end=point(last);}}
      else if(entity.kind==="SPLINE"){const from=entity.parameterStart??entity.knots?.[0],to=entity.parameterEnd??entity.knots?.at(-1);if(from===undefined||to===undefined)return;entity.parameterStart=from+(to-from)*start;entity.parameterEnd=from+(to-from)*end;entity.closed=false;}
      else{const from=entity.startAngle??0,to=entity.endAngle??2*Math.PI;entity.startAngle=from+(to-from)*start;entity.endAngle=from+(to-from)*end;if(entity.kind==="CIRCLE")entity.kind="ARC";if(entity.kind==="ELLIPSE")entity.kind="ELLIPTICAL_ARC";}
      context.viewport.showSketchEntityPreview?.([entity]);return;
    }
    const axis=this.kind==="mirror"?this.axisPoints(context):undefined;
    if(this.kind==="mirror"&&!axis)return;
    context.viewport.showSketchEntityPreview?.(sketchEditPreview(this.baseline,this.origin??[0,0],this.translation,this.angle,axis));
  }
  private intervalOperation(context:ToolContext):SketchOperation|undefined {
    if(!this.baseline[0])return;
    const boundaries=this.boundaryMode==="automatic"?(context.viewport.currentSketchReferenceEntities?.()??context.viewport.currentSketchEntities?.()??[]).filter(e=>e.id!==this.baseline[0].id&&e.kind!=="POINT").map(e=>e.id):this.boundaries.map(ref=>ref.entityId!);
    return {type:"QUICK_TRIM",operationId:this.operationID??"preview",entityIds:[this.baseline[0].id],boundaryIds:boundaries,hitParameter:Number(this.fields[0]||"0.5"),trimMode:this.trimMode,...(this.detach.length?{detachConstraintIds:[...this.detach]}:{})};
  }
  private showCandidate(context:ToolContext,operation:SketchOperation,submitAfterPreview=false):void {
    const entities=context.viewport.currentSketchReferenceEntities?.()??context.viewport.currentSketchEntities?.()??[];
    this.candidateAbort?.abort();this.acceptPendingCandidate=false;const generation=++this.candidateGeneration;this.previewStatus="idle";
    this.candidate=buildSketchEditPreview({entities,constraints:context.viewport.currentSketchConstraints?.(),operation,axisPoints:this.kind==="mirror"?this.axisPoints(context):undefined});
    if(this.direct?.kind==="corner"&&this.candidate.dimensions){
      const direct=this.direct;direct.dimensionAnchors??=[];
      this.candidate.dimensions.forEach((dimension,index)=>{
        const anchor=direct.dimensionAnchors![index]??buildSketchConstraintLayout(dimension,[...entities,...this.candidate!.entities,...this.candidate!.dimensionEntities??[]]).label?.position;
        if(anchor){direct.dimensionAnchors![index]=anchor;dimension.labelPosition={x:anchor[0],y:anchor[1]};}
      });
    }
    if(this.candidate.status==="UNAVAILABLE") {
      context.viewport.clearToolPreview();this.error=this.candidate.diagnostic;
      if(context.viewport.previewSketchOperations){
        this.previewStatus="pending";const abort=this.candidateAbort=new AbortController(),scope=scopeKey(context.viewport.currentSketchIdentity?.());
        this.publish(context);context.viewport.setToolPrompt("正在计算预览；可 Esc 取消");
        void context.viewport.previewSketchOperations([operation],abort.signal).then(result=>{
          if(abort.signal.aborted||generation!==this.candidateGeneration||scope!==scopeKey(context.viewport.currentSketchIdentity?.()))return;
          const ids=new Set("entityIds" in operation?operation.entityIds:"entityId" in operation?[operation.entityId]:[]),target=this.baseline[0];
          const changed=result.entities.filter(e=>ids.has(e.id)||e.sourceEntityId&&ids.has(e.sourceEntityId));
          const cuts:number[]=[];
          if(target)for(const e of changed){
            if(target.kind==="SPLINE"&&e.parameterStart!==undefined&&e.parameterEnd!==undefined){const a=target.parameterStart??target.knots?.[0],b=target.parameterEnd??target.knots?.at(-1);if(a!==undefined&&b!==undefined)cuts.push((e.parameterStart-a)/(b-a),(e.parameterEnd-a)/(b-a));}
            else if(e.startAngle!==undefined&&e.endAngle!==undefined){const a=target.startAngle??0,b=target.endAngle??2*Math.PI;cuts.push((e.startAngle-a)/(b-a),(e.endAngle-a)/(b-a));}
            else if(e.start&&e.end){const a=selectedCurveParameter(target,[e.start.x,e.start.y]),b=selectedCurveParameter(target,[e.end.x,e.end.y]);if(a!==undefined&&b!==undefined)cuts.push(a,b);}
          }
          const display=buildSketchEditPreview({entities,operation,intersectionParameters:cuts});
          this.candidate={...display,status:"APPROXIMATE",entities:changed,diagnostic:"操作预览"};this.error=undefined;this.previewStatus="ready";this.verifiedCandidateGeneration=generation;
          if(context.viewport.showSketchEditCandidate)context.viewport.showSketchEditCandidate(this.candidate);else context.viewport.showSketchEntityPreview?.(changed);
          this.publish(context);context.viewport.setToolPrompt("预览已更新；Enter 执行，Esc 取消");
          if(submitAfterPreview||this.acceptPendingCandidate){this.acceptPendingCandidate=false;this.commit(context,[operation]);}
        },error=>{if(abort.signal.aborted||generation!==this.candidateGeneration)return;this.previewStatus="failed";this.error=error instanceof Error?error.message:String(error);context.viewport.clearToolPreview();this.publish(context);context.viewport.setToolPrompt(this.error);});
      }else this.previewStatus="failed";
      this.publish(context);return;
    }
    this.error=undefined;this.previewStatus="ready";
    if(context.viewport.showSketchEditCandidate)context.viewport.showSketchEditCandidate(this.candidate);
    else context.viewport.showSketchEntityPreview?.([...this.candidate.entities,...this.candidate.hitEntities]);
    this.publish(context);
  }
  private selectIntervalTarget(event:CadPointerEvent,context:ToolContext,commit:boolean):boolean {
    const reference=context.viewport.sketchReferenceAt(event.x,event.y,"CURVE",undefined,ref=>ref.target==="ENTITY"&&(context.viewport.currentSketchEntities?.()??[]).some(e=>e.id===ref.entityId)),target=context.viewport.sketchPlacementPoint(event.x,event.y);
    const entity=reference?.target==="ENTITY"&&(context.viewport.currentSketchEntities?.()??[]).find(e=>e.id===reference.entityId);
    if(!entity||!target){if(!commit){context.viewport.clearToolPreview();context.viewport.clearReferenceHover?.();return false;}this.error="未命中当前草图的可编辑曲线区间";this.publish(context);return false;}
    const parameter=selectedCurveParameter(entity,target);
    if(parameter===undefined||!Number.isFinite(parameter)||parameter<0||parameter>1){this.error="无法确定命中区间";this.publish(context);return false;}
    if(!commit&&this.armed&&entity.id!==this.baseline[0]?.id)return false;
    if(commit||this.armed){
      this.selected=new Set([entity.id]);this.baseline=[structuredClone(entity)];this.scope=context.viewport.currentSketchIdentity?.();this.phase="definition";this.operationID??=randomUUID();this.fields[0]=String(parameter);
      const operation=this.intervalOperation(context)!;this.showCandidate(context,operation,commit&&(this.kind==="quick_trim"||this.direct?.kind==="trim"));this.highlight(context);
      if(this.previewStatus!=="pending"&&this.candidate?.status!=="UNAVAILABLE"&&commit&&(this.kind==="quick_trim"||this.direct?.kind==="trim"))this.commit(context,[operation]);else this.prompt(context);
    } else {
      // Hover affects only a derived candidate. It never accepts a command source.
      const boundaries=(context.viewport.currentSketchEntities?.()??[]).filter(e=>e.id!==entity.id&&e.kind!=="POINT").map(e=>e.id);
      const candidate=buildSketchEditPreview({entities:context.viewport.currentSketchEntities?.()??[],operation:{type:"QUICK_TRIM",operationId:"hover-preview",entityIds:[entity.id],boundaryIds:boundaries,hitParameter:parameter,trimMode:this.trimMode}});
      if(candidate.status==="APPROXIMATE")context.viewport.showSketchEditCandidate?.(candidate);else context.viewport.clearToolPreview();
    }
    return true;
  }
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult{
    if(event.state.buttons.middle||event.state.buttons.right&&event.button===0)return InputResult.Ignored;
    if(event.button===2){if(this.direct)return InputResult.Ignored;this.cancel(context);context.viewport.finishToolUse(true);return InputResult.Consumed;}
    if(event.button!==0||this.captured!==undefined||!context.viewport.hasActiveSketch())return InputResult.Ignored;
    if(this.phase==="committing"||this.phase==="unknown"||this.previewStatus==="pending")return InputResult.Consumed;
    if(!this.validScope(context))return InputResult.Ignored;
    this.captured=event.pointerId;
    if(this.direct){if(this.direct.kind==="extend")this.extendDragStart=[event.x,event.y];this.directClick(event,context);return InputResult.Capture;}
    if((this.kind==="quick_trim"||this.kind==="trim"&&!this.advancedTrim)&&this.boundaryMode==="automatic"){
      this.selectIntervalTarget(event,context,true);return InputResult.Capture;
    }
    if(!this.armed){const selection=(context.viewport.sketchEntityAt??context.viewport.selectionAt)(event.x,event.y),entities=selection?localSketchSelection([selection],context):[];
      if(!entities.length){context.viewport.setToolPrompt("只能编辑当前草图本地几何；外部几何只读");return InputResult.Capture;}
      for(const entity of entities)if(this.selected.has(entity.id))this.selected.delete(entity.id);else this.selected.add(entity.id);
      this.highlight(context);if(this.kind==="construction")this.arm(context);else this.prompt(context);return InputResult.Capture;}
    if(this.kind.startsWith("spline_")){
      const entity=this.baseline[0],target=context.viewport.sketchPlacementPoint(event.x,event.y);
      if(entity.mode==="CONTROL"&&(this.kind==="spline_insert"||this.kind==="spline_delete")){
        if(target){const fraction=selectedCurveParameter(entity,target),start=entity.parameterStart??entity.knots?.[0]??0,end=entity.parameterEnd??entity.knots?.at(-1)??1;
          if(fraction!==undefined){const candidate=start+(end-start)*fraction;
            this.fields[0]=String(this.kind==="spline_delete"?entity.knots?.filter(knot=>knot>start&&knot<end).reduce((best,knot)=>Math.abs(knot-candidate)<Math.abs(best-candidate)?knot:best,Infinity)??candidate:candidate);
            this.preview(context);this.prompt(context);}}
      }else if(this.kind==="spline_insert"&&target){this.branchPoint=[...target];this.preview(context);this.prompt(context);}
      else if(this.kind==="spline_delete"){
        const reference=context.viewport.sketchReferenceAt(event.x,event.y,"EDIT_POINT");
        if(reference?.target==="ENTITY"&&reference.entityId===entity.id&&reference.subElement==="CONTROL"){
          this.splinePointReference={...reference};this.fields[0]=String(reference.controlPointIndex??splineEditablePointIDs(entity).indexOf(reference.controlPointId!));
          context.viewport.showReferencePreview(reference);this.prompt(context);}}
      return InputResult.Capture;
    }
    if(this.kind==="extend"){
      const target=context.viewport.sketchPlacementPoint(event.x,event.y),reference=context.viewport.sketchReferenceAt(event.x,event.y,"CURVE",undefined,ref=>!this.extendReference?ref.target==="ENTITY"&&ref.entityId===this.baseline[0].id:ref.entityId!==this.baseline[0].id);
      if(!this.extendReference){
        if(!target||reference?.target!=="ENTITY"||reference.entityId!==this.baseline[0].id){context.viewport.setToolPrompt("点击所选本地曲线靠近要延伸的端点");return InputResult.Capture;}
        const first=curvePoint(this.baseline[0],0)!,last=curvePoint(this.baseline[0],1)!,subElement=Math.hypot(target[0]-first[0],target[1]-first[1])<=Math.hypot(target[0]-last[0],target[1]-last[1])?"START":"END";
        this.extendReference={target:"ENTITY",entityId:this.baseline[0].id,subElement};context.viewport.showReferencePreview(this.extendReference);this.prompt(context);return InputResult.Capture;
      }
      if(!this.boundaries.length){if(reference?.entityId&&reference.entityId!==this.baseline[0].id){this.boundaries=[reference];context.viewport.showReferencePreview(reference,[this.extendReference]);this.prompt(context);}return InputResult.Capture;}
      if(target){this.branchPoint=[...target];this.preview(context);this.prompt(context);}return InputResult.Capture;
    }
    if(["complement","close","offset"].includes(this.kind)){this.prompt(context);return InputResult.Capture;}
    if(this.kind==="fillet"||this.kind==="chamfer"){
      const target=context.viewport.sketchPlacementPoint(event.x,event.y);if(!target)return InputResult.Capture;
      if(this.cornerRefs.length>=2){this.cornerPoint=[...target];this.preview(context);this.prompt(context);return InputResult.Capture;}
      const reference=context.viewport.sketchReferenceAt(event.x,event.y,"CURVE",undefined,ref=>ref.target==="ENTITY"&&this.baseline.some(entity=>entity.id===ref.entityId));
      const entity=this.baseline.find(entity=>entity.id===reference?.entityId);
      if(!entity||reference?.target!=="ENTITY"){context.viewport.setToolPrompt("点击已选本地曲线靠近要切除的端点，外部支撑保持只读");return InputResult.Capture;}
      const first=curvePoint(entity,0)!,last=curvePoint(entity,1)!,subElement=Math.hypot(target[0]-first[0],target[1]-first[1])<=Math.hypot(target[0]-last[0],target[1]-last[1])?"START":"END";
      const cutReference:SketchGeometryRef={target:"ENTITY",entityId:entity.id,subElement};
      const existing=this.cornerRefs.findIndex(item=>item.entityId===entity.id);if(existing>=0)this.cornerRefs[existing]=cutReference;else this.cornerRefs.push(cutReference);
      context.viewport.showReferencePreview(cutReference,this.cornerRefs);this.prompt(context);return InputResult.Capture;
    }
    if(this.kind==="quick_trim"||this.kind==="trim"&&!this.advancedTrim){
      const reference=context.viewport.sketchReferenceAt(event.x,event.y,"CURVE");
      if(reference?.entityId&&reference.entityId!==this.baseline[0].id){const existing=this.boundaries.findIndex(item=>item.entityId===reference.entityId&&item.target===reference.target);
        if(existing>=0)this.boundaries.splice(existing,1);else this.boundaries.push(reference);
        context.viewport.showReferencePreview(reference,[{target:"ENTITY",entityId:this.baseline[0].id,subElement:"WHOLE"},...this.boundaries]);this.preview(context);this.prompt(context);return InputResult.Capture;}
      if(reference?.entityId!==this.baseline[0].id){context.viewport.setToolPrompt("选择另一条曲线作为边界，或点击选定目标曲线的命中段");return InputResult.Capture;}
      const target=context.viewport.sketchPlacementPoint(event.x,event.y),parameter=target&&selectedCurveParameter(this.baseline[0],target);
      if(parameter!==undefined&&parameter!==null&&parameter>=0&&parameter<=1){this.fields[0]=String(parameter);this.preview(context);if(this.kind==="quick_trim")this.confirm(context);else this.prompt(context);}
      return InputResult.Capture;
    }
    if(this.kind==="split"){
      const target=context.viewport.sketchPlacementPoint(event.x,event.y),parameter=target&&selectedCurveParameter(this.baseline[0],target);
      if(parameter!==undefined&&parameter!==null&&parameter>0&&parameter<1){this.fields[0]=String(parameter);this.preview(context);this.confirm(context);}return InputResult.Capture;
    }
    if(this.kind==="trim"){if(!this.advancedTrim)this.selectIntervalTarget(event,context,false);else context.viewport.setToolPrompt("高级参数：输入起点，Tab 输入终点，Enter 确认");return InputResult.Capture;}
    if(this.kind==="mirror"){
      const reference=context.viewport.sketchReferenceAt(event.x,event.y,"LINE",undefined,ref=>ref.target!=="EXTERNAL");
      if(reference&&reference.target!=="EXTERNAL"){this.axis=reference;this.preview(context);this.prompt(context);}
      else context.viewport.setToolPrompt("镜像轴接受本地直线、辅助线或 X/Y 内置轴");
      return InputResult.Capture;
    }
    const target=context.viewport.sketchPlacementPoint(event.x,event.y);if(!target)return InputResult.Capture;
    if(!this.origin){this.origin=target;this.prompt(context);return InputResult.Capture;}
    if(this.kind==="rotate"&&!this.ray){if(Math.hypot(target[0]-this.origin[0],target[1]-this.origin[1])<SKETCH_INPUT_POLICY.minimumGeometryLength){context.viewport.setToolPrompt("基准点不能与中心重合");return InputResult.Capture;}this.ray=target;this.prompt(context);return InputResult.Capture;}
    this.updateTarget(target);this.preview(context);this.confirm(context);return InputResult.Capture;
  }
  private updateTarget(target:Vec2):void{
    if(!this.origin)return;
    if(this.kind==="move"||this.kind==="copy")this.translation=[target[0]-this.origin[0],target[1]-this.origin[1]];
    else if(this.ray){const base:Vec2=[this.ray[0]-this.origin[0],this.ray[1]-this.origin[1]],next:Vec2=[target[0]-this.origin[0],target[1]-this.origin[1]];
      if(this.kind==="rotate")this.angle=Math.atan2(base[0]*next[1]-base[1]*next[0],base[0]*next[0]+base[1]*next[1]);
}
    this.applyFields();
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult{
    if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;
    if(this.phase==="committing"||this.phase==="unknown")return InputResult.Consumed;
    if(!this.validScope(context))return InputResult.Consumed;
    if(this.previewStatus==="pending")return InputResult.Consumed;
    if(this.direct){this.directHover(event,context);return InputResult.Consumed;}
    if((this.kind==="quick_trim"||this.kind==="trim"&&!this.advancedTrim)&&this.boundaryMode==="automatic"){this.selectIntervalTarget(event,context,false);return InputResult.Consumed;}
    if(!this.armed){
      const reference=context.viewport.sketchReferenceAt(event.x,event.y,"ENTITY",undefined,ref=>ref.target==="ENTITY"&&(context.viewport.currentSketchEntities?.()??[]).some(entity=>entity.id===ref.entityId));
      if(reference)context.viewport.showReferencePreview(reference);else context.viewport.clearReferenceHover?.();
      return InputResult.Consumed;
    }
    const target=context.viewport.sketchPlacementPoint(event.x,event.y);if(target)this.updateTarget(target);
    this.preview(context);this.prompt(context);return InputResult.Consumed;
  }
  pointerUp(event:CadPointerEvent,context:ToolContext):InputResult{
    if(event.pointerId!==this.captured||event.button!==0)return InputResult.Ignored;
    this.captured=undefined;
    if(this.direct?.kind==="extend"&&this.direct.stage==="target"&&this.extendDragStart&&
      Math.hypot(event.x-this.extendDragStart[0],event.y-this.extendDragStart[1])>4&&this.phase!=="committing"&&this.phase!=="unknown")this.directExtend(event,context,true);
    this.extendDragStart=undefined;return InputResult.ReleaseCapture;
  }
  pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult{if(event.pointerId!==this.captured)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  private applyFields():boolean{
    const values=this.fields.map(value=>value===""?undefined:Number(value));if(values.some(value=>value!==undefined&&!Number.isFinite(value)))return false;
    if(this.kind==="move"||this.kind==="copy")this.translation=[values[0]??this.translation[0],values[1]??this.translation[1]];
    else if(this.kind==="rotate"&&values[0]!==undefined)this.angle=values[0]*Math.PI/180;
    return true;
  }
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult{
    if(event.editableTarget||event.state?.modifiers.ctrl||event.state?.modifiers.meta)return InputResult.Ignored;
    if(event.key==="Escape"){this.cancel(context);context.viewport.finishToolUse(true);return InputResult.Consumed;}
    if(this.phase==="committing"||this.phase==="unknown")return InputResult.Consumed;
    if(!this.validScope(context))return InputResult.Consumed;
    if(this.previewStatus==="pending"){
      if(!this.direct&&event.key==="Enter"&&!event.repeat)this.confirm(context);
      return InputResult.Consumed;
    }
    if(this.direct){
      if(event.repeat||["Control","Shift","Alt","Meta"].includes(event.key))return InputResult.Consumed;
      if(event.key==="Backspace"){this.directAction({type:"back"},context);return InputResult.Consumed;}
      if(event.key==="Enter"){this.directAction({type:"confirm"},context);return InputResult.Consumed;}
      return InputResult.Ignored;
    }
    if(event.key==="Enter"){
      if(!this.armed&&this.batch.length&&!this.selected.size)this.commit(context,[...this.batch]);
      else if(!this.armed)this.arm(context);else this.confirm(context);return InputResult.Consumed;
    }
    if(!this.armed)return InputResult.Ignored;
    if(this.kind==="fillet"||this.kind==="chamfer"){
      if(event.key.toLowerCase()==="k"){this.cornerTrimMode=this.cornerTrimMode==="TRIM"?"KEEP":"TRIM";this.prompt(context);return InputResult.Consumed;}
      if(this.kind==="chamfer"&&event.key.toLowerCase()==="m"){this.chamferMode=this.chamferMode==="EQUAL"?"TWO_LENGTHS":this.chamferMode==="TWO_LENGTHS"?"LENGTH_ANGLE":"EQUAL";this.prompt(context);return InputResult.Consumed;}
      if(event.key.toLowerCase()==="b"){
        const operation=this.cornerOperation(context);if(!operation)return InputResult.Consumed;
        this.batch.push(operation);this.selected.clear();this.baseline=[];this.phase="selection";this.operationID=undefined;this.cornerRefs=[];this.cornerPoint=undefined;
        context.viewport.clearToolPreview();context.viewport.clearReferencePreview();this.prompt(context);return InputResult.Consumed;
      }
    }
    if(this.kind==="offset"&&event.key.toLowerCase()==="m"){this.offsetMode=this.offsetMode==="MITER"?"ROUND":"MITER";this.prompt(context);return InputResult.Consumed;}
    if(this.kind==="mirror"&&event.key.toLowerCase()==="m"){this.mirrorMode=this.mirrorMode==="LINKED"?"INDEPENDENT":"LINKED";this.prompt(context);return InputResult.Consumed;}
    if((this.kind.startsWith("spline_")||["split","trim","quick_trim","extend","complement","close"].includes(this.kind))&&event.key.toLowerCase()==="u"){
      this.toggleReferenceRelease(context);return InputResult.Consumed;
    }
    if(this.kind==="quick_trim"&&event.key.toLowerCase()==="q"){this.trimMode=this.trimMode==="DELETE_HIT"?"KEEP_HIT":this.trimMode==="KEEP_HIT"?"BREAK":"DELETE_HIT";this.prompt(context);return InputResult.Consumed;}
    if(event.key.toLowerCase()==="i"){this.policy=this.policy==="INTERNAL"?"GEOMETRY_ONLY":"INTERNAL";this.prompt(context);return InputResult.Consumed;}
    if(event.key.toLowerCase()==="c"&&event.state?.modifiers.shift){this.copy=!this.copy;this.prompt(context);return InputResult.Consumed;}
    if(this.kind==="mirror"&&["x","y"].includes(event.key.toLowerCase())){this.axis={target:event.key.toLowerCase()==="x"?"SKETCH_X_AXIS":"SKETCH_Y_AXIS",subElement:"WHOLE"};this.preview(context);this.prompt(context);return InputResult.Consumed;}
    if(this.kind==="trim"&&/^[0-9.+-]$/.test(event.key))this.advancedTrim=true;
    if(event.key==="Tab")this.field=(["move","copy","trim"].includes(this.kind)||this.kind==="chamfer"&&this.chamferMode!=="EQUAL")?(this.field===0?1:0):0;
    else if(event.key==="Backspace")this.fields[this.field]=this.fields[this.field].slice(0,-1);
    else if(/^[0-9.\-+]$/.test(event.key))this.fields[this.field]+=event.key;
    else return InputResult.Ignored;
    if(this.applyFields())this.preview(context);this.prompt(context);return InputResult.Consumed;
  }
  private cornerOperation(context:ToolContext):SketchOperation|undefined{
    if(this.direct?.kind==="corner"){
      if(!this.cornerPoint||this.cornerRefs.length!==2)return;
      const primary=directLengthInput(this.fields[0],this.direct.unit??context.viewport.currentLengthUnit?.()??"mm",5);
      const base={operationId:this.operationID!,entityIds:this.cornerRefs.map(r=>r.entityId!) as [string,string],firstReference:this.cornerRefs[0],secondReference:this.cornerRefs[1],point:point(this.cornerPoint),trimMode:this.cornerTrimMode,parameterSource:primary.parameterSource};
      if(this.kind==="fillet")return {type:"FILLET_ENTITIES",...base,value:primary.value};
      const secondary=this.chamferMode==="TWO_LENGTHS"?directLengthInput(this.fields[1]||this.fields[0],context.viewport.currentLengthUnit?.()??"mm",primary.value).value:Number(this.fields[1]||"45");
      if(!Number.isFinite(secondary)||secondary<=0||this.chamferMode==="LENGTH_ANGLE"&&secondary>=180)throw new Error("请输入有效的第二距离或角度");
      return {type:"CHAMFER_ENTITIES",...base,chamferMode:this.chamferMode,chamferFirst:primary.value,...(this.chamferMode==="TWO_LENGTHS"?{chamferSecond:secondary}:this.chamferMode==="LENGTH_ANGLE"?{chamferAngle:secondary}:{})};
    }
    if(!this.cornerPoint||this.cornerRefs.length!==2||this.cornerRefs[0].entityId===this.cornerRefs[1].entityId){context.viewport.setToolPrompt("先明确选择两条支撑的裁切端部，再点击分支位置");return;}
    const first=Number(this.fields[0]||"5"),second=Number(this.fields[1]||(this.chamferMode==="TWO_LENGTHS"?"5":"45"));
    if(!Number.isFinite(first)||first<=0||(this.kind==="chamfer"&&this.chamferMode!=="EQUAL"&&(!Number.isFinite(second)||second<=0||this.chamferMode==="LENGTH_ANGLE"&&second>=180))){this.error="圆角半径/倒角长度必须为正有限值，倒角角度必须在 0° 与 180° 之间";context.viewport.setToolPrompt(this.error);return;}
    const base={operationId:this.operationID!,entityIds:this.cornerRefs.map(reference=>reference.entityId!) as [string,string],
      firstReference:{...this.cornerRefs[0]},secondReference:{...this.cornerRefs[1]},point:point(this.cornerPoint),trimMode:this.cornerTrimMode};
    return this.kind==="fillet"?{type:"FILLET_ENTITIES",...base,value:first}:{type:"CHAMFER_ENTITIES",...base,chamferMode:this.chamferMode,chamferFirst:first,
      ...(this.chamferMode==="TWO_LENGTHS"?{chamferSecond:second}:this.chamferMode==="LENGTH_ANGLE"?{chamferAngle:second}:{})};
  }
  private confirm(context:ToolContext):void{
    if(!this.armed||!this.validScope(context)||this.previewStatus==="failed"||this.phase==="committing")return;
    // One accepted Enter can wait for the current read-only preview, without creating a second request.
    if(this.previewStatus==="pending"){this.acceptPendingCandidate=true;return;}
    if(this.kind==="fillet"||this.kind==="chamfer"){const operation=this.cornerOperation(context);if(operation)this.commit(context,[...this.batch,operation]);return;}
    const operation=this.definitionOperation(context);if(operation)this.commit(context,[operation]);
  }
  private definitionOperation(context:ToolContext):SketchOperation|undefined{
    if(!this.baseline.length||!this.applyFields()){context.viewport.setToolPrompt("输入必须为有限数值");return;}
    const base={operationId:this.operationID!,entityIds:this.baseline.map(entity=>entity.id)};
    let operation:SketchOperation;
    if(this.kind.startsWith("spline_")){
      const entity=this.baseline[0],common={operationId:this.operationID!,entityId:entity.id,...(this.detach.length?{detachConstraintIds:[...this.detach]}:{})};
      if(this.kind==="spline_close")operation={type:"SET_SPLINE_CLOSED",...common,closed:!entity.closed};
      else if(this.kind==="spline_control")operation={type:"CONVERT_SPLINE_TO_CONTROL",...common};
      else if(entity.mode==="CONTROL"){
        const start=entity.parameterStart??entity.knots?.[0]??0,end=entity.parameterEnd??entity.knots?.at(-1)??1,knotParameter=Number(this.fields[0]||String((start+end)/2));
        if(!Number.isFinite(knotParameter)||knotParameter<=start||knotParameter>=end){context.viewport.setToolPrompt("结点必须在真实活动参数域内部；删除还须是可精确移除的现有结点");return;}
        operation={type:"EDIT_SPLINE_POINT",...common,pointAction:this.kind==="spline_insert"?"INSERT":"DELETE",knotParameter};
      }else{
        const count=splineEditablePoints(entity).length,index=Number(this.fields[0]||String(this.kind==="spline_insert"?count:count-1));
        if(!Number.isInteger(index)||index<0||index>(this.kind==="spline_insert"?count:count-1)||this.kind==="spline_insert"&&!this.branchPoint){context.viewport.setToolPrompt("指定合法拟合点位置；插入还需点击新拟合点坐标");return;}
        operation={type:"EDIT_SPLINE_POINT",...common,pointAction:this.kind==="spline_insert"?"INSERT":"DELETE",pointIndex:index,
          ...(this.kind==="spline_insert"?{point:point(this.branchPoint!)}:{controlPointId:this.splinePointReference?.controlPointId??entity.controlPointIds?.[index]})};
      }
    }else     if(this.kind==="extend"){
      if(!this.extendReference||this.boundaries.length!==1||!this.branchPoint){context.viewport.setToolPrompt("明确选择要延伸的端点、一条边界及目标交点分支位置");return;}
      operation={type:"EXTEND_ENTITY",...base,firstReference:this.extendReference,boundaryIds:[this.boundaries[0].entityId!],point:point(this.branchPoint),...(this.detach.length?{detachConstraintIds:[...this.detach]}:{})};
    }else if(this.kind==="complement"||this.kind==="close"){
      operation={type:this.kind==="complement"?"ARC_COMPLEMENT":"CLOSE_CURVE",...base,...(this.detach.length?{detachConstraintIds:[...this.detach]}:{})};
    }else if(this.kind==="offset"){
      const value=Number(this.fields[0]||"5");if(!Number.isFinite(value)){context.viewport.setToolPrompt("偏移距离必须为有限有符号模型长度");return;}
      operation={type:"OFFSET_ENTITIES",...base,value,mode:this.offsetMode};
    }else     if(this.kind==="mirror"){
      if(!this.axis||!this.axisPoints(context)){context.viewport.setToolPrompt("先选择有效镜像轴；M 切换关联或独立副本");return;}
      operation={type:"MIRROR_ENTITIES",...base,axis:this.axis,mirrorMode:this.mirrorMode,...(this.mirrorMode==="INDEPENDENT"?{constraintPolicy:this.policy}:{})};
    }else if(this.kind==="split"){
      const parameter=this.fields[0]===""?0.5:Number(this.fields[0]);
      if(!(parameter>0&&parameter<1)){context.viewport.setToolPrompt("分割参数必须严格位于 0 和 1 之间");return;}
      operation={type:"SPLIT_ENTITY",...base,parameters:[parameter],...(this.detach.length?{detachConstraintIds:[...this.detach]}:{})};
    }else if(this.kind==="trim"&&!this.advancedTrim){
      const candidate=this.intervalOperation(context);if(!candidate||!("boundaryIds" in candidate)||!candidate.boundaryIds?.length){this.error="没有可用边界，请明确选择边界";this.publish(context);return;}operation=candidate;
    }else if(this.kind==="trim"){
      const start=Number(this.fields[0]||"0"),end=Number(this.fields[1]||"1");if(!(start>=0&&end<=1&&start<end)){context.viewport.setToolPrompt("保留范围必须满足 0 ≤ 起点 < 终点 ≤ 1");return;}
      operation={type:"TRIM_ENTITY",...base,parameters:[start,end],...(this.detach.length?{detachConstraintIds:[...this.detach]}:{})};
    }else if(this.kind==="quick_trim"){
      const hitParameter=Number(this.fields[0]||"0.5");if(!(hitParameter>=0&&hitParameter<=1)){context.viewport.setToolPrompt("至少选择一个边界，并给出 0~1 命中参数；无交点或重叠将由权威内核拒绝");return;}
      operation=this.intervalOperation(context)!;if(!("boundaryIds" in operation)||!operation.boundaryIds?.length){this.error="请选择至少一条有效边界";this.publish(context);return;}
    }else operation={type:this.kind==="copy"?"COPY_ENTITIES":"TRANSFORM_ENTITIES",...base,origin:point(this.origin??[0,0]),translation:point(this.translation),angle:this.angle,
      ...(this.kind==="copy"?{}:{copy:this.copy}),constraintPolicy:this.policy};
    return operation;
  }
  private commit(context:ToolContext,operations:SketchOperation[]):void{
    if(this.phase==="committing")return;
    this.pendingOperations=structuredClone(operations);
    const receiptRetry=this.phase==="unknown";
    this.phase="committing";this.error=undefined;const generation=++this.commitGeneration;
    this.prompt(context);
    let result:ReturnType<ToolContext["viewport"]["commitSketchOperations"]>;
    try { result=context.viewport.commitSketchOperations(operations,{requestId:this.requestID??=randomUUID(),baseVersionId:this.scope?.versionId,retryReceipt:receiptRetry}); }
    catch(error){this.failCommit(error,context,generation);return;}
    const success=()=>{if(generation!==this.commitGeneration)return;
      const current=context.viewport.currentSketchIdentity?.();
      if(this.scope&&current&&(current.documentId!==this.scope.documentId||current.sketchId!==this.scope.sketchId||(current.occurrencePath??"")!==(this.scope.occurrencePath??"")))return;
      if(this.direct?.kind==="corner"){
        // Continuous corners reuse a numeric input, never an implicit shared formula.
        // The accepted operation keeps its own checked parameter source unchanged.
        const radius=context.viewport.currentSketchEntities?.().find(entity=>entity.createdByOperationId===this.operationID&&entity.kind==="ARC")?.radius;
        const operation=operations[0];
        const value=radius??(operation.type==="FILLET_ENTITIES"?operation.value:operation.type==="CHAMFER_ENTITIES"?operation.chamferFirst:undefined);
        if(value!==undefined&&Number.isFinite(value)&&value>0)this.recentCornerSource=`${value} mm`;
      }
      this.phase="committed";this.cancel(context);context.viewport.finishToolUse();
    };
    if(result&&typeof result.then==="function")void result.then(success,error=>this.failCommit(error,context,generation));else success();
  }
  private failCommit(error:unknown,context:ToolContext,generation:number):void {
    if(generation!==this.commitGeneration)return;
    const current=context.viewport.currentSketchIdentity?.();
    if(this.scope&&current&&(current.documentId!==this.scope.documentId||current.sketchId!==this.scope.sketchId||(current.occurrencePath??"")!==(this.scope.occurrencePath??"")))return;
    this.phase=sketchCommitResultUnknown(error)?"unknown":"failed";
    if(this.phase==="failed")this.requestID=undefined;
    this.error=error instanceof Error?error.message:String(error);
    context.viewport.clearToolPreview();context.viewport.clearReferencePreview();
    this.prompt(context);context.viewport.setToolPrompt(`${this.phase==="unknown"?"提交结果待确认；按原请求查询/重试":"操作失败，可修改后重试"}：${this.error}`);
  }
  deactivate(context:ToolContext):void{this.cancel(context);context.viewport.setSketchCommandState?.(undefined);}
  cancel(context:ToolContext):void{
    this.candidateAbort?.abort();this.candidateGeneration++;this.acceptPendingCandidate=false;this.previewStatus="idle";this.candidate=undefined;
    this.commitGeneration++;this.requestID=undefined;this.pendingOperations=undefined;this.error=undefined;this.hover=undefined;
    this.splinePointReference=undefined;this.extendReference=undefined;this.branchPoint=undefined;this.batch=[];this.cornerRefs=[];this.cornerPoint=undefined;this.selected.clear();this.baseline=[];this.scope=undefined;this.phase="selection";this.operationID=undefined;this.origin=undefined;this.ray=undefined;
    this.boundaries=[];this.detach=[];this.translation=[0,0];this.angle=0;this.axis=undefined;this.copy=false;this.fields=["",""];this.field=0;this.captured=undefined;
    if(this.direct){this.direct=this.direct.kind==="mirror"?{kind:"mirror",route:"axis-first",stage:"axis"}:this.direct.kind==="corner"?{kind:"corner",stage:"curves",clicks:[],inputId:randomUUID()}:this.direct.kind==="split"?{kind:"split"}:this.direct.kind==="extend"?{kind:"extend",stage:"curve"}:{kind:"trim"};if(this.direct.kind==="corner")this.fields[0]=this.recentCornerSource;}
    context.viewport.clearToolPreview();context.viewport.clearReferencePreview();this.prompt(context);
  }
}
