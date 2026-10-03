import type {SelectionItem,SketchConstraint,SketchEntity,SketchGeometryRef,SketchOperation,SketchPoint2,Vec2} from "../../types";
import {InputResult,type CadKeyboardEvent,type CadPointerEvent} from "../input/input-types";
import type {CadTool,ToolContext} from "./cad-tool";
import {randomUUID} from "../../utils/random-uuid";
import {splineEditablePoints,splineEditablePointIDs,sampleSketchEntity,evaluateCanonicalSpline} from "../sketch/sketch-geometry";
import {SKETCH_INPUT_POLICY} from "../sketch/sketch-input-policy";

export type SketchEditKind="delete"|"copy"|"move"|"rotate"|"scale"|"mirror"|"split"|"trim"|"quick_trim"|"fillet"|"chamfer"|"extend"|"complement"|"close"|"offset"|"spline_insert"|"spline_delete"|"spline_close"|"spline_control"|"construction";
type SketchScope=NonNullable<ReturnType<NonNullable<ToolContext["viewport"]["currentSketchIdentity"]>>>;
const scopeKey=(scope:SketchScope|undefined)=>scope&&`${scope.documentId}/${scope.sketchId}/${scope.versionId}/${scope.occurrencePath??""}`;
const point=(value:Vec2):SketchPoint2=>({x:value[0],y:value[1]});
const labels:Record<SketchEditKind,string>={delete:"删除",copy:"复制",move:"移动",rotate:"旋转",scale:"统一缩放",mirror:"镜像",split:"分割",trim:"修剪保留范围",quick_trim:"快速修剪",fillet:"草图圆角",chamfer:"草图倒角",extend:"延伸到交点",complement:"圆弧补弧",close:"闭合曲线",offset:"二维独立偏移",spline_insert:"样条插入拟合点/结点",spline_delete:"样条删除拟合点/结点",spline_close:"样条开/闭合",spline_control:"转为控制点样条",construction:"标准/辅助转换"};

export function localSketchSelection(selections:readonly SelectionItem[],context:ToolContext):SketchEntity[]{
  const scope=context.viewport.currentSketchIdentity?.(),entities=context.viewport.currentSketchEntities?.()??[];
  const ids=[...new Set(selections.filter(selection=>selection.kind==="visual"&&(!scope||selection.featureId===scope.sketchId)&&
    (!scope||!selection.ownerDocumentId||selection.ownerDocumentId===scope.documentId)&&(!scope?.occurrencePath||!selection.occurrencePath||selection.occurrencePath===scope.occurrencePath)).map(selection=>selection.entityId))];
  const byID=new Map(entities.map(entity=>[entity.id,entity]));
  return ids.flatMap(id=>byID.has(id!)?[byID.get(id!)!]:[]);
}

// This is an approximate display transform only. The operation carries original
// stable references to the control plane, which validates and solves the model.
export function sketchEditPreview(entities:readonly SketchEntity[],origin:Vec2,translation:Vec2,angle:number,scale:number,
  axis?:{start:Vec2;end:Vec2}):SketchEntity[]{
  const c=Math.cos(angle),s=Math.sin(angle);
  const map=(p:SketchPoint2):SketchPoint2=>{
    if(axis){const dx=axis.end[0]-axis.start[0],dy=axis.end[1]-axis.start[1],length=dx*dx+dy*dy,
      projection=((p.x-axis.start[0])*dx+(p.y-axis.start[1])*dy)/length;
      return{x:2*(axis.start[0]+projection*dx)-p.x,y:2*(axis.start[1]+projection*dy)-p.y};}
    const dx=(p.x-origin[0])*scale,dy=(p.y-origin[1])*scale;
    return{x:origin[0]+dx*c-dy*s+translation[0],y:origin[1]+dx*s+dy*c+translation[1]};
  };
  const reflectionAngle=axis?2*Math.atan2(axis.end[1]-axis.start[1],axis.end[0]-axis.start[0]):0;
  return entities.map(source=>{
    const entity=structuredClone(source);
    for(const key of ["point","start","end","center"] as const)if(entity[key])entity[key]=map(entity[key]!);
    if(entity.controlPoints)entity.controlPoints=entity.controlPoints.map(map);
    if(entity.poles)entity.poles=entity.poles.map(map);
    if(entity.radius!==undefined)entity.radius*=axis?1:scale;
    if(entity.majorRadius!==undefined)entity.majorRadius*=axis?1:scale;
    if(entity.minorRadius!==undefined)entity.minorRadius*=axis?1:scale;
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
  const entities=sketchEditPreview(clipboard.entities,[0,0],offset,0,1).map(entity=>({...entity,id:idMap.get(entity.id)!,createdByOperationId:operationID,sourceEntityId:entity.id,
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
  private selected=new Set<string>();
  private baseline:SketchEntity[]=[];
  private scope?:SketchScope;
  private armed=false;
  private operationID?:string;
  private origin?:Vec2;
  private ray?:Vec2;
  private translation:Vec2=[0,0];
  private angle=0;
  private scale=1;
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
  constructor(readonly kind:SketchEditKind){this.id=`sketch.edit.${kind}`;}
  activate(context:ToolContext):void{this.prompt(context);}
  selectionInput(selections:readonly SelectionItem[],context:ToolContext):void{
    if(this.armed)return;
    for(const entity of localSketchSelection(selections,context))this.selected.add(entity.id);
    this.highlight(context);this.prompt(context);
  }
  private prompt(context:ToolContext):void{
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
      this.kind==="rotate"?`角度°: ${this.fields[0]||this.angle*180/Math.PI}`:this.kind==="scale"?`比例: ${this.fields[0]||this.scale}`:this.kind==="split"?`参数比例 0~1: ${this.fields[0]||"0.5"}`:this.kind==="trim"?`保留范围 0~1: ${this.fields[0]||"0"} → ${this.fields[1]||"1"}；Tab 切换`:
      this.kind==="quick_trim"?`边界 ${this.boundaries.length} 条；选择边界，再点击目标曲线区间或输入命中比例 ${this.fields[0]||"0.5"}；Q 模式: ${{DELETE_HIT:"删除命中段",KEEP_HIT:"保留命中段",BREAK:"打断保留各段"}[this.trimMode]}`:"选择直线/辅助线作为轴，或 X/Y 内置轴";
    if(["split","trim","quick_trim"].includes(this.kind)){const conversion=this.baseline[0]?.kind==="SPLINE"&&this.baseline[0].mode!=="CONTROL"?"；FIT 将使用精确子曲线转为 CONTROL（不重新拟合），拟合点引用需明确解除或操作拒绝":"";context.viewport.setToolPrompt(`${labels[this.kind]}：${inputs}${conversion}；Enter 确认，Esc 取消；交点由权威内核计算；U ${this.detach.length?`明确解除 ${this.detach.length} 个整体关系（再按可取消）`:"预览整体关系释放影响"}`);return;}
    const gesture=this.kind==="move"||this.kind==="copy"?"单击基点和目标点":this.kind==="rotate"||this.kind==="scale"?"单击中心、基准点和目标点":"";
    context.viewport.setToolPrompt(`${labels[this.kind]}：${gesture}；${inputs}；Enter 提交，Esc 取消；${this.kind==="mirror"&&this.mirrorMode==="LINKED"?"关联镜像（M 切换为独立）；正式反射关系，不复制驱动尺寸":this.kind==="mirror"||this.kind==="copy"||this.copy?`${this.kind==="mirror"?"独立镜像（M 切换为关联）；":""}${this.policy==="INTERNAL"?"复制内部约束":"仅几何副本（不复制连接和尺寸）"}（I 切换）`:"保留约束，冲突时拒绝"}；Shift+C 切换带复制`);
  }
  private highlight(context:ToolContext):void{
    const entities=(context.viewport.currentSketchEntities?.()??[]).filter(entity=>this.selected.has(entity.id));
    context.viewport.showSketchEntityPreview?.(entities);
  }
  private validScope(context:ToolContext):boolean{
    if((!this.armed&&!this.batch.length)||scopeKey(this.scope)===scopeKey(context.viewport.currentSketchIdentity?.()))return true;
    this.cancel(context);context.viewport.setToolPrompt("草图版本已变化；请重新选择并预览编辑");return false;
  }
  private arm(context:ToolContext):boolean{
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
    this.baseline=structuredClone(entities);this.scope=context.viewport.currentSketchIdentity?.();this.armed=true;this.operationID=randomUUID();this.copy=this.kind==="copy";
    if(this.kind==="delete"){this.commit(context,[{type:"DELETE_ENTITIES",entityIds:entities.map(entity=>entity.id)}]);return true;}
    if(this.kind==="construction"){const role=entities.every(entity=>entity.role==="CONSTRUCTION")?"PROFILE":"CONSTRUCTION";
      this.commit(context,entities.map(entity=>({type:"UPDATE_ENTITY_ROLE",entityId:entity.id,role})));return true;}
    this.prompt(context);return true;
  }
  private axisPoints(context:ToolContext):{start:Vec2;end:Vec2}|undefined{
    if(this.axis?.target==="SKETCH_X_AXIS"||this.axis?.target==="SKETCH_Y_AXIS")return{start:[0,0],end:this.axis.target==="SKETCH_Y_AXIS"?[0,1]:[1,0]};
    const entity=context.viewport.currentSketchEntities?.().find(entity=>entity.id===this.axis?.entityId);
    return entity?.kind==="LINE"&&entity.start&&entity.end?{start:[entity.start.x,entity.start.y],end:[entity.end.x,entity.end.y]}:undefined;
  }
  private preview(context:ToolContext):void{
    if(this.kind.startsWith("spline_")||["extend","complement","close","offset"].includes(this.kind)){
      if(this.branchPoint)context.viewport.showPointPreview(this.branchPoint);else context.viewport.showSketchEntityPreview?.(this.baseline);return;
    }
    if(this.kind==="fillet"||this.kind==="chamfer"){
      if(this.cornerPoint)context.viewport.showPointPreview(this.cornerPoint);
      else context.viewport.showSketchEntityPreview?.(this.baseline);
      return;
    }
    if(this.kind==="split"||this.kind==="quick_trim"){
      const parameter=Number(this.fields[0]||"0.5"),target=curvePoint(this.baseline[0],parameter);
      if(target)context.viewport.showPointPreview(target);return;
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
    context.viewport.showSketchEntityPreview?.(sketchEditPreview(this.baseline,this.origin??[0,0],this.translation,this.angle,this.scale,axis));
  }
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult{
    if(event.state.buttons.middle)return InputResult.Ignored;
    if(event.button===2){this.cancel(context);context.viewport.finishToolUse(true);return InputResult.Consumed;}
    if(event.button!==0||this.captured!==undefined||!context.viewport.hasActiveSketch()||!this.validScope(context))return InputResult.Ignored;
    this.captured=event.pointerId;
    if(!this.armed){const selection=context.viewport.selectionAt(event.x,event.y),entities=selection?localSketchSelection([selection],context):[];
      if(!entities.length){context.viewport.setToolPrompt("只能编辑当前草图本地几何；外部几何只读");return InputResult.Capture;}
      for(const entity of entities)if(this.selected.has(entity.id))this.selected.delete(entity.id);else this.selected.add(entity.id);
      this.highlight(context);this.prompt(context);return InputResult.Capture;}
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
      const target=context.viewport.sketchPlacementPoint(event.x,event.y),reference=context.viewport.sketchReferenceAt(event.x,event.y,"CURVE");
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
      const reference=context.viewport.sketchReferenceAt(event.x,event.y,"CURVE");
      const entity=this.baseline.find(entity=>entity.id===reference?.entityId);
      if(!entity||reference?.target!=="ENTITY"){context.viewport.setToolPrompt("点击已选本地曲线靠近要切除的端点，外部支撑保持只读");return InputResult.Capture;}
      const first=curvePoint(entity,0)!,last=curvePoint(entity,1)!,subElement=Math.hypot(target[0]-first[0],target[1]-first[1])<=Math.hypot(target[0]-last[0],target[1]-last[1])?"START":"END";
      const cutReference:SketchGeometryRef={target:"ENTITY",entityId:entity.id,subElement};
      const existing=this.cornerRefs.findIndex(item=>item.entityId===entity.id);if(existing>=0)this.cornerRefs[existing]=cutReference;else this.cornerRefs.push(cutReference);
      context.viewport.showReferencePreview(cutReference,this.cornerRefs);this.prompt(context);return InputResult.Capture;
    }
    if(this.kind==="quick_trim"){
      const reference=context.viewport.sketchReferenceAt(event.x,event.y,"CURVE");
      if(reference?.entityId&&reference.entityId!==this.baseline[0].id){const existing=this.boundaries.findIndex(item=>item.entityId===reference.entityId&&item.target===reference.target);
        if(existing>=0)this.boundaries.splice(existing,1);else this.boundaries.push(reference);
        context.viewport.showReferencePreview(reference,[{target:"ENTITY",entityId:this.baseline[0].id,subElement:"WHOLE"},...this.boundaries]);this.prompt(context);return InputResult.Capture;}
      if(reference?.entityId!==this.baseline[0].id){context.viewport.setToolPrompt("选择另一条曲线作为边界，或点击选定目标曲线的命中段");return InputResult.Capture;}
      const target=context.viewport.sketchPlacementPoint(event.x,event.y),parameter=target&&selectedCurveParameter(this.baseline[0],target);
      if(parameter!==undefined&&parameter!==null&&parameter>=0&&parameter<=1){this.fields[0]=String(parameter);this.preview(context);this.confirm(context);}
      return InputResult.Capture;
    }
    if(this.kind==="split"){
      const target=context.viewport.sketchPlacementPoint(event.x,event.y),parameter=target&&selectedCurveParameter(this.baseline[0],target);
      if(parameter!==undefined&&parameter!==null&&parameter>0&&parameter<1){this.fields[0]=String(parameter);this.preview(context);this.confirm(context);}return InputResult.Capture;
    }
    if(this.kind==="trim"){context.viewport.setToolPrompt("修剪保留范围使用规范参数 0~1；输入起点，Tab 输入终点，Enter 确认");return InputResult.Capture;}
    if(this.kind==="mirror"){
      const reference=context.viewport.sketchReferenceAt(event.x,event.y,"LINE");
      if(reference&&reference.target!=="EXTERNAL"){this.axis=reference;this.preview(context);this.prompt(context);}
      else context.viewport.setToolPrompt("镜像轴接受本地直线、辅助线或 X/Y 内置轴");
      return InputResult.Capture;
    }
    const target=context.viewport.sketchPlacementPoint(event.x,event.y);if(!target)return InputResult.Capture;
    if(!this.origin){this.origin=target;this.prompt(context);return InputResult.Capture;}
    if((this.kind==="rotate"||this.kind==="scale")&&!this.ray){if(Math.hypot(target[0]-this.origin[0],target[1]-this.origin[1])<SKETCH_INPUT_POLICY.minimumGeometryLength){context.viewport.setToolPrompt("基准点不能与中心重合");return InputResult.Capture;}this.ray=target;this.prompt(context);return InputResult.Capture;}
    this.updateTarget(target);this.preview(context);this.confirm(context);return InputResult.Capture;
  }
  private updateTarget(target:Vec2):void{
    if(!this.origin)return;
    if(this.kind==="move"||this.kind==="copy")this.translation=[target[0]-this.origin[0],target[1]-this.origin[1]];
    else if(this.ray){const base:Vec2=[this.ray[0]-this.origin[0],this.ray[1]-this.origin[1]],next:Vec2=[target[0]-this.origin[0],target[1]-this.origin[1]];
      if(this.kind==="rotate")this.angle=Math.atan2(base[0]*next[1]-base[1]*next[0],base[0]*next[0]+base[1]*next[1]);
      else if(this.kind==="scale")this.scale=Math.hypot(...next)/Math.hypot(...base);}
    this.applyFields();
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult{
    if(!this.armed||event.state.buttons.middle||event.state.buttons.right||!this.validScope(context))return InputResult.Ignored;
    const target=context.viewport.sketchPlacementPoint(event.x,event.y);if(target)this.updateTarget(target);
    this.preview(context);this.prompt(context);return InputResult.Consumed;
  }
  pointerUp(event:CadPointerEvent):InputResult{if(event.pointerId!==this.captured||event.button!==0)return InputResult.Ignored;this.captured=undefined;return InputResult.ReleaseCapture;}
  pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult{if(event.pointerId!==this.captured)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  private applyFields():boolean{
    const values=this.fields.map(value=>value===""?undefined:Number(value));if(values.some(value=>value!==undefined&&!Number.isFinite(value)))return false;
    if(this.kind==="move"||this.kind==="copy")this.translation=[values[0]??this.translation[0],values[1]??this.translation[1]];
    else if(this.kind==="rotate"&&values[0]!==undefined)this.angle=values[0]*Math.PI/180;
    else if(this.kind==="scale"&&values[0]!==undefined)this.scale=values[0];
    return this.scale>0&&Number.isFinite(this.scale);
  }
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult{
    if(event.editableTarget||event.state?.modifiers.ctrl||event.state?.modifiers.meta)return InputResult.Ignored;
    if(event.key==="Escape"){const hadState=this.armed||this.selected.size>0;this.cancel(context);return hadState?InputResult.Consumed:InputResult.Ignored;}
    if(!this.validScope(context))return InputResult.Consumed;
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
        this.batch.push(operation);this.selected.clear();this.baseline=[];this.armed=false;this.operationID=undefined;this.cornerRefs=[];this.cornerPoint=undefined;
        context.viewport.clearToolPreview();context.viewport.clearReferencePreview();this.prompt(context);return InputResult.Consumed;
      }
    }
    if(this.kind==="offset"&&event.key.toLowerCase()==="m"){this.offsetMode=this.offsetMode==="MITER"?"ROUND":"MITER";this.prompt(context);return InputResult.Consumed;}
    if(this.kind==="mirror"&&event.key.toLowerCase()==="m"){this.mirrorMode=this.mirrorMode==="LINKED"?"INDEPENDENT":"LINKED";this.prompt(context);return InputResult.Consumed;}
    if((this.kind.startsWith("spline_")||["split","trim","quick_trim","extend","complement","close"].includes(this.kind))&&event.key.toLowerCase()==="u"){
      const affected=(context.viewport.currentSketchConstraints?.()??[]).filter(constraint=>constraint.references.some(reference=>reference.target==="ENTITY"&&reference.entityId===this.baseline[0].id&&( ["WHOLE","DIRECTION"].includes(reference.subElement)||["complement","close"].includes(this.kind)&&["START","END"].includes(reference.subElement)||this.kind==="extend"&&reference.subElement===this.extendReference?.subElement||(this.kind.startsWith("spline_")||["split","trim","quick_trim"].includes(this.kind)&&this.baseline[0].kind==="SPLINE"&&this.baseline[0].mode!=="CONTROL")&&reference.subElement==="CONTROL")));
      this.detach=this.detach.length?[]:affected.map(constraint=>constraint.id);
      context.viewport.setToolPrompt(this.detach.length?`将显式解除整体/点关系：${affected.map(constraint=>constraint.kind).join("、")}；U 撤回，Enter 连同拓扑编辑原子确认`:"没有解除任何关系；原约束继续接受严格验证");return InputResult.Consumed;
    }
    if(this.kind==="quick_trim"&&event.key.toLowerCase()==="q"){this.trimMode=this.trimMode==="DELETE_HIT"?"KEEP_HIT":this.trimMode==="KEEP_HIT"?"BREAK":"DELETE_HIT";this.prompt(context);return InputResult.Consumed;}
    if(event.key.toLowerCase()==="i"){this.policy=this.policy==="INTERNAL"?"GEOMETRY_ONLY":"INTERNAL";this.prompt(context);return InputResult.Consumed;}
    if(event.key.toLowerCase()==="c"&&event.state?.modifiers.shift){this.copy=!this.copy;this.prompt(context);return InputResult.Consumed;}
    if(this.kind==="mirror"&&["x","y"].includes(event.key.toLowerCase())){this.axis={target:event.key.toLowerCase()==="x"?"SKETCH_X_AXIS":"SKETCH_Y_AXIS",subElement:"WHOLE"};this.preview(context);this.prompt(context);return InputResult.Consumed;}
    if(event.key==="Tab")this.field=(["move","copy","trim"].includes(this.kind)||this.kind==="chamfer"&&this.chamferMode!=="EQUAL")?(this.field===0?1:0):0;
    else if(event.key==="Backspace")this.fields[this.field]=this.fields[this.field].slice(0,-1);
    else if(/^[0-9.\-+]$/.test(event.key))this.fields[this.field]+=event.key;
    else return InputResult.Ignored;
    if(this.applyFields())this.preview(context);this.prompt(context);return InputResult.Consumed;
  }
  private cornerOperation(context:ToolContext):SketchOperation|undefined{
    if(!this.cornerPoint||this.cornerRefs.length!==2||this.cornerRefs[0].entityId===this.cornerRefs[1].entityId){context.viewport.setToolPrompt("先明确选择两条支撑的裁切端部，再点击分支位置");return;}
    const first=Number(this.fields[0]||"5"),second=Number(this.fields[1]||(this.chamferMode==="TWO_LENGTHS"?"5":"45"));
    if(!Number.isFinite(first)||first<=0||(this.kind==="chamfer"&&this.chamferMode!=="EQUAL"&&(!Number.isFinite(second)||second<=0||this.chamferMode==="LENGTH_ANGLE"&&second>=180))){context.viewport.setToolPrompt("圆角半径/倒角长度必须为正有限值，倒角角度必须在 0° 与 180° 之间");return;}
    const base={operationId:this.operationID!,entityIds:this.cornerRefs.map(reference=>reference.entityId!) as [string,string],
      firstReference:{...this.cornerRefs[0]},secondReference:{...this.cornerRefs[1]},point:point(this.cornerPoint),trimMode:this.cornerTrimMode};
    return this.kind==="fillet"?{type:"FILLET_ENTITIES",...base,value:first}:{type:"CHAMFER_ENTITIES",...base,chamferMode:this.chamferMode,chamferFirst:first,
      ...(this.chamferMode==="TWO_LENGTHS"?{chamferSecond:second}:this.chamferMode==="LENGTH_ANGLE"?{chamferAngle:second}:{})};
  }
  private confirm(context:ToolContext):void{
    if(!this.armed||!this.validScope(context))return;
    if(this.kind==="fillet"||this.kind==="chamfer"){const operation=this.cornerOperation(context);if(operation)this.commit(context,[...this.batch,operation]);return;}
    if(!this.applyFields()){context.viewport.setToolPrompt("输入必须为有限数值，缩放比例必须大于零");return;}
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
    }else if(this.kind==="trim"){
      const start=Number(this.fields[0]||"0"),end=Number(this.fields[1]||"1");if(!(start>=0&&end<=1&&start<end)){context.viewport.setToolPrompt("保留范围必须满足 0 ≤ 起点 < 终点 ≤ 1");return;}
      operation={type:"TRIM_ENTITY",...base,parameters:[start,end],...(this.detach.length?{detachConstraintIds:[...this.detach]}:{})};
    }else if(this.kind==="quick_trim"){
      const hitParameter=Number(this.fields[0]||"0.5");if(!this.boundaries.length||!(hitParameter>=0&&hitParameter<=1)){context.viewport.setToolPrompt("至少选择一个边界，并给出 0~1 命中参数；无交点或重叠将由权威内核拒绝");return;}
      operation={type:"QUICK_TRIM",...base,boundaryIds:this.boundaries.map(reference=>reference.entityId!),hitParameter,trimMode:this.trimMode,...(this.detach.length?{detachConstraintIds:[...this.detach]}:{})};
    }else operation={type:this.kind==="copy"?"COPY_ENTITIES":"TRANSFORM_ENTITIES",...base,origin:point(this.origin??[0,0]),translation:point(this.translation),angle:this.angle,scale:this.scale,
      ...(this.kind==="copy"?{}:{copy:this.copy}),constraintPolicy:this.policy};
    this.commit(context,[operation]);
  }
  private commit(context:ToolContext,operations:SketchOperation[]):void{
    context.viewport.commitSketchOperations(operations);this.cancel(context);context.viewport.finishToolUse();
  }
  deactivate(context:ToolContext):void{this.cancel(context);}
  cancel(context:ToolContext):void{
    this.splinePointReference=undefined;this.extendReference=undefined;this.branchPoint=undefined;this.batch=[];this.cornerRefs=[];this.cornerPoint=undefined;this.selected.clear();this.baseline=[];this.scope=undefined;this.armed=false;this.operationID=undefined;this.origin=undefined;this.ray=undefined;
    this.boundaries=[];this.detach=[];this.translation=[0,0];this.angle=0;this.scale=1;this.axis=undefined;this.copy=false;this.fields=["",""];this.field=0;this.captured=undefined;
    context.viewport.clearToolPreview();context.viewport.clearReferencePreview();this.prompt(context);
  }
}
