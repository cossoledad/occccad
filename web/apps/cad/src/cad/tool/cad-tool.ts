import type { CadKeyboardEvent, CadPointerEvent } from "../input/input-types";
import { InputResult } from "../input/input-types";
import type { AssemblyGeometryRef, SelectionItem, SketchGeometryRef, SketchOperation, Vec2 } from "../../types";
import type { SketchReferencePickKind } from "../interaction/sketch-reference-pick";
import { constraintDefinition, type ConstraintKind } from "../sketch/sketch-constraint-definition";
import { sampleInterpolatingSpline } from "../sketch/sketch-geometry";
import { SKETCH_INPUT_POLICY, type SketchReferenceGeometry } from "../sketch/sketch-input-policy";
import { randomUUID } from "../../utils/random-uuid";

export type ToolViewportPort = {
  sketchPoint(x: number, y: number): Vec2 | null;
  sketchSnapReference(): SketchGeometryRef | undefined;
  sketchPlacementPoint(x: number, y: number): Vec2 | null;
  showPolylinePreview(points: Vec2[], closed?: boolean): void;
  showPointPreview(point: Vec2): void;
  showReferenceDimensions(geometry: readonly SketchReferenceGeometry[]): void;
  clearToolPreview(): void;
  commitSketchOperations(operations: SketchOperation[]): void;
  hasActiveSketch(): boolean;
  sketchReferenceAt(x: number, y: number, kind: SketchReferencePickKind, retained?: SketchGeometryRef): SketchGeometryRef | null;
  showReferencePreview(reference: SketchGeometryRef, retained?: readonly SketchGeometryRef[]): void;
  showConstraintPreview(kind: ConstraintKind, references: readonly SketchGeometryRef[], value?: number, labelPosition?: Vec2): void;
  measureDimension(kind: ConstraintKind, references: readonly SketchGeometryRef[]): number | undefined;
  requestDimensionCreation(kind: ConstraintKind, references: readonly SketchGeometryRef[], value: number,
    unit: "mm" | "deg", labelPosition: Vec2, x: number, y: number): void;
  beginDimensionDrag(x: number, y: number): boolean;
  updateDimensionDrag(x: number, y: number): void;
  finishDimensionDrag(): void;
  cancelDimensionDrag(): void;
  editDimensionAt(x: number, y: number): boolean;
  clearReferencePreview(): void;
  setToolPrompt(prompt: string): void;
  finishToolUse(): void;
  selectionAt(x: number, y: number): SelectionItem | null;
  retainSelections(selections: SelectionItem[]): void;
  requestAssemblyConstraint(kind: AssemblyConstraintToolKind, references: AssemblyGeometryRef[]): void;
};

export type ToolContext = { viewport: ToolViewportPort };
const sameSketchReference = (left: SketchGeometryRef, right: SketchGeometryRef): boolean =>
  left.target === right.target && left.entityId === right.entityId && left.subElement === right.subElement;
export interface CadTool {
  readonly id: string;
  activate?(context: ToolContext): void;
  deactivate?(context: ToolContext): void;
  pointerDown?(event: CadPointerEvent, context: ToolContext): InputResult;
  pointerMove?(event: CadPointerEvent, context: ToolContext): InputResult;
  pointerUp?(event: CadPointerEvent, context: ToolContext): InputResult;
  pointerCancel?(event: CadPointerEvent, context: ToolContext): InputResult;
  keyDown?(event: CadKeyboardEvent, context: ToolContext): InputResult;
  keyUp?(event: CadKeyboardEvent, context: ToolContext): InputResult;
  cancel?(context: ToolContext): void;
}

export class SelectTool implements CadTool {
  readonly id = "select";
  private dimensionPointer?: { id: number; x: number; y: number; moved: boolean };
  private lastDimensionClick?: { x: number; y: number; at: number };
  private sketchPointPointer?:{id:number;reference:SketchGeometryRef;point:Vec2};
  activate(context: ToolContext): void { context.viewport.clearReferencePreview();context.viewport.clearToolPreview();context.viewport.setToolPrompt("选择：选择草图元素，或从工具栏启动创建命令"); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    if (context.viewport.beginDimensionDrag(event.x, event.y)) {
      this.dimensionPointer = { id: event.pointerId, x: event.x, y: event.y, moved: false };return InputResult.Capture;
    }
    const reference=context.viewport.sketchReferenceAt(event.x,event.y,"EDIT_POINT"),point=context.viewport.sketchPlacementPoint(event.x,event.y);
    if(reference&&reference.entityId&&point){this.sketchPointPointer={id:event.pointerId,reference,point};context.viewport.showPointPreview(point);return InputResult.Capture;}
    return InputResult.Ignored;
  }
  pointerMove(event: CadPointerEvent, context: ToolContext): InputResult {
    if(event.pointerId===this.sketchPointPointer?.id){const point=context.viewport.sketchPlacementPoint(event.x,event.y);if(point){this.sketchPointPointer.point=point;context.viewport.showPointPreview(point);}return InputResult.Consumed;}
    if (event.pointerId !== this.dimensionPointer?.id) return InputResult.Ignored;
    if (Math.hypot(event.x - this.dimensionPointer.x, event.y - this.dimensionPointer.y) >= 3) this.dimensionPointer.moved = true;
    context.viewport.updateDimensionDrag(event.x, event.y);
    return InputResult.Consumed;
  }
  pointerUp(event: CadPointerEvent, context: ToolContext): InputResult {
    if(event.pointerId===this.sketchPointPointer?.id&&event.button===0){const drag=this.sketchPointPointer;this.sketchPointPointer=undefined;context.viewport.clearToolPreview();
      context.viewport.commitSketchOperations([{type:"UPDATE_ENTITY_POINT",entityId:drag.reference.entityId!,subElement:drag.reference.subElement as "CENTER"|"CONTROL",
        controlPointIndex:drag.reference.controlPointIndex,point:{x:drag.point[0],y:drag.point[1]}}]);return InputResult.ReleaseCapture;}
    if (event.pointerId !== this.dimensionPointer?.id || event.button !== 0) return InputResult.Ignored;
    const gesture = this.dimensionPointer; this.dimensionPointer = undefined; context.viewport.finishDimensionDrag();
    if (!gesture.moved) {
      const at = Number(event.originalEvent.timeStamp) || Date.now();
      const previous = this.lastDimensionClick;
      if (previous && at - previous.at <= 450 && Math.hypot(event.x - previous.x, event.y - previous.y) <= 6) {
        this.lastDimensionClick = undefined;
        context.viewport.editDimensionAt(event.x, event.y);
      } else this.lastDimensionClick = { x: event.x, y: event.y, at };
    } else this.lastDimensionClick = undefined;
    return InputResult.ReleaseCapture;
  }
  pointerCancel(event: CadPointerEvent, context: ToolContext): InputResult {
    if(event.pointerId===this.sketchPointPointer?.id){this.sketchPointPointer=undefined;context.viewport.clearToolPreview();return InputResult.Consumed;}
    if (event.pointerId !== this.dimensionPointer?.id) return InputResult.Ignored;
    this.dimensionPointer = undefined; this.lastDimensionClick = undefined; context.viewport.cancelDimensionDrag();
    return InputResult.Consumed;
  }
  cancel(context: ToolContext): void { this.dimensionPointer = undefined;this.sketchPointPointer=undefined; this.lastDimensionClick = undefined;context.viewport.clearToolPreview(); context.viewport.cancelDimensionDrag(); }
}

export type AssemblyConstraintToolKind = "fix"|"rigid"|"coincident"|"concentric"|"angle"|"distance";

export function assemblyGeometryRef(selection: SelectionItem): AssemblyGeometryRef | undefined {
  if (!selection.instanceId) return undefined;
  if (selection.kind === "instance") return { instanceId: selection.instanceId, kind: "BODY" };
  if (selection.kind === "face" && selection.geometryKey && selection.topologyId)
    return { instanceId: selection.instanceId, kind: "FACE", geometryKey: selection.geometryKey, topologyId: selection.topologyId };
  if (selection.kind === "plane" && selection.entityId)
    return { instanceId: selection.instanceId, kind: "PLANE", geometryId: selection.entityId };
  if (selection.kind === "axis" && selection.entityId)
    return { instanceId: selection.instanceId, kind: "AXIS", geometryId: selection.entityId,
      ...(selection.axis === "DATUM" ? {} : { axis: selection.axis }) };
  if (selection.kind === "axis-system" && selection.entityId)
    return { instanceId: selection.instanceId, kind: "POINT", geometryId: selection.entityId };
  return undefined;
}

export class AssemblyConstraintTool implements CadTool {
  readonly id: string;
  private first?: { selection: SelectionItem; reference: AssemblyGeometryRef };
  private capturedPointerID?: number;
  constructor(readonly kind: AssemblyConstraintToolKind) { this.id = `assembly.${kind}`; }
  activate(context: ToolContext): void { context.viewport.setToolPrompt(this.kind === "fix" ? "固定：选择一个实例" : "装配约束：依次选择两个元素"); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || this.capturedPointerID !== undefined || event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    const selection = context.viewport.selectionAt(event.x, event.y);
    const reference = selection && assemblyGeometryRef(selection);
    if (!selection || !reference || (this.kind === "rigid" && reference.kind !== "BODY")) return InputResult.Consumed;
    this.capturedPointerID = event.pointerId;
    if (this.kind === "fix") {
      context.viewport.retainSelections([selection]);
      context.viewport.requestAssemblyConstraint(this.kind, [reference]);
      context.viewport.finishToolUse();
      return InputResult.Capture;
    }
    if (!this.first) {
      this.first = { selection, reference };
      context.viewport.retainSelections([selection]);
      context.viewport.setToolPrompt("装配约束：选择另一个实例上的元素；Esc 取消");
      return InputResult.Capture;
    }
    if (this.first.reference.instanceId === reference.instanceId) return InputResult.Capture;
    const first = this.first; this.first = undefined;
    context.viewport.retainSelections([first.selection, selection]);
    context.viewport.requestAssemblyConstraint(this.kind, [first.reference, reference]);
    context.viewport.finishToolUse();
    return InputResult.Capture;
  }
  pointerUp(event: CadPointerEvent): InputResult {
    if (event.pointerId !== this.capturedPointerID || event.button !== 0) return InputResult.Ignored;
    this.capturedPointerID = undefined; return InputResult.ReleaseCapture;
  }
  pointerCancel(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.cancel(context); return InputResult.Consumed;
  }
  deactivate(context: ToolContext): void { this.cancel(context); }
  cancel(context: ToolContext): void { this.first = undefined; this.capturedPointerID = undefined; context.viewport.setToolPrompt(""); }
}

abstract class TwoClickSketchTool implements CadTool {
  abstract readonly id: string;
  protected first?: Vec2;
  protected firstSnap?: SketchGeometryRef;
  private capturedPointerID?: number;
  abstract preview(first: Vec2, second: Vec2, context: ToolContext): void;
  abstract commit(first: Vec2, second: Vec2, context: ToolContext,
    snaps: { first?: SketchGeometryRef; second?: SketchGeometryRef }): void;
  abstract readonly firstPrompt: string;
  abstract readonly secondPrompt: string;

  activate(context: ToolContext): void { context.viewport.setToolPrompt(this.firstPrompt); }

  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || this.capturedPointerID !== undefined || event.state.buttons.middle || event.state.buttons.right || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y);
    if (!point) return InputResult.Ignored;
    this.capturedPointerID = event.pointerId;
    if (!this.first) {
      this.first = point;
      this.firstSnap = context.viewport.sketchSnapReference();
      this.preview(point, point, context);
      context.viewport.setToolPrompt(this.secondPrompt);
      return InputResult.Capture;
    }
    const first = this.first, firstSnap = this.firstSnap, secondSnap = context.viewport.sketchSnapReference();
    this.first = undefined; this.firstSnap = undefined; context.viewport.clearToolPreview();
    if (Math.hypot(point[0] - first[0], point[1] - first[1]) >= SKETCH_INPUT_POLICY.minimumGeometryLength)
      this.commit(first, point, context, { first: firstSnap, second: secondSnap });
    context.viewport.setToolPrompt(this.firstPrompt);
    return InputResult.Capture;
  }

  pointerUp(event: CadPointerEvent): InputResult {
    if (event.button !== 0 || event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined;
    return InputResult.ReleaseCapture;
  }

  pointerCancel(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined;
    this.cancel(context);
    return InputResult.Consumed;
  }

  pointerMove(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y);
    if (!point) return InputResult.Ignored;
    if (this.first) this.preview(this.first, point, context);
    return InputResult.Consumed;
  }

  keyDown(event: CadKeyboardEvent, context: ToolContext): InputResult {
    if (event.key !== "Escape" || !this.first) return InputResult.Ignored;
    this.cancel(context);
    return InputResult.Consumed;
  }
  deactivate(context: ToolContext): void { this.cancel(context); }
  cancel(context: ToolContext): void {
    this.capturedPointerID = undefined;
    this.first = undefined;
    this.firstSnap = undefined;
    context.viewport.clearToolPreview();
    context.viewport.setToolPrompt(this.firstPrompt);
  }
}

export class LineSketchTool extends TwoClickSketchTool {
  readonly id = "sketch.line";
  readonly firstPrompt = "直线：单击起点";
  readonly secondPrompt = "直线：移动预览，单击终点；Esc 取消当前线";
  preview(first: Vec2, second: Vec2, context: ToolContext): void {
    context.viewport.showPolylinePreview([first, second]);
    context.viewport.showReferenceDimensions([{kind:"LINE",start:first,end:second}]);
  }
  commit(first: Vec2, second: Vec2, context: ToolContext, snaps: { first?: SketchGeometryRef; second?: SketchGeometryRef }): void {
    const id=randomUUID();
    const operations:SketchOperation[]=[{ type: "ADD_ENTITY", entity: { id, kind: "LINE", role: "PROFILE", start: { x: first[0], y: first[1] }, end: { x: second[0], y: second[1] } } }];
    for(const [subElement,target] of [["START",snaps.first],["END",snaps.second]] as const) if(target) operations.push({type:"ADD_CONSTRAINT",
      constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:id,subElement},target]}});
    context.viewport.commitSketchOperations(operations);
    context.viewport.finishToolUse();
  }
}

export class RectangleSketchTool extends TwoClickSketchTool {
  readonly id = "sketch.rectangle";
  readonly firstPrompt = "矩形：单击第一个角点";
  readonly secondPrompt = "矩形：移动预览，单击对角点；Esc 取消当前矩形";
  preview(first: Vec2, second: Vec2, context: ToolContext): void {
    // The rectangle macro has a deterministic local solution on every pointer
    // move; the authoritative PlaneGCS solve still happens at commit.
    context.viewport.showPolylinePreview([first, [second[0], first[1]], second, [first[0], second[1]]], true);
    context.viewport.showReferenceDimensions([
      {kind:"LINE",start:first,end:[second[0],first[1]]},
      {kind:"LINE",start:first,end:[first[0],second[1]]},
    ]);
  }
  commit(first: Vec2, second: Vec2, context: ToolContext, snaps: { first?: SketchGeometryRef; second?: SketchGeometryRef }): void {
    context.viewport.commitSketchOperations([{ type: "ADD_RECTANGLE", first: { x: first[0], y: first[1] }, second: { x: second[0], y: second[1] },
      firstReference:snaps.first,secondReference:snaps.second }]);
    context.viewport.finishToolUse();
  }
}

function polylineOperations(points:Vec2[],closed:boolean):SketchOperation[] {
  const vertices=closed?[...points,points[0]]:points;
  const ids=Array.from({length:vertices.length-1},()=>randomUUID());
  const operations:SketchOperation[]=ids.map((id,index)=>({type:"ADD_ENTITY",entity:{id,kind:"LINE",role:"PROFILE",
    start:{x:vertices[index][0],y:vertices[index][1]},end:{x:vertices[index+1][0],y:vertices[index+1][1]}}}));
  for(let index=1;index<ids.length;index+=1)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",internal:true,
    references:[{target:"ENTITY",entityId:ids[index-1],subElement:"END"},{target:"ENTITY",entityId:ids[index],subElement:"START"}]}});
  if(closed&&ids.length>1)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",internal:true,
    references:[{target:"ENTITY",entityId:ids.at(-1),subElement:"END"},{target:"ENTITY",entityId:ids[0],subElement:"START"}]}});
  return operations;
}

export class RegularPolygonSketchTool extends TwoClickSketchTool {
  readonly id="sketch.polygon";readonly firstPrompt="正六边形：单击中心";readonly secondPrompt="正六边形：单击一个顶点；Esc 取消";
  private vertices(center:Vec2,vertex:Vec2):Vec2[]{const radius=Math.hypot(vertex[0]-center[0],vertex[1]-center[1]);const start=Math.atan2(vertex[1]-center[1],vertex[0]-center[0]);return Array.from({length:6},(_,index)=>[center[0]+radius*Math.cos(start+index*Math.PI/3),center[1]+radius*Math.sin(start+index*Math.PI/3)]);}
  preview(center:Vec2,vertex:Vec2,context:ToolContext):void {const vertices=this.vertices(center,vertex);context.viewport.showPolylinePreview(vertices,true);
    context.viewport.showReferenceDimensions([{kind:"LINE",start:vertices[0],end:vertices[1]}]);}
  commit(center:Vec2,vertex:Vec2,context:ToolContext):void {const operations=polylineOperations(this.vertices(center,vertex),true);const ids=operations.filter((item)=>item.type==="ADD_ENTITY").map((item)=>item.entity.id);for(let index=1;index<ids.length;index+=1){operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"EQUAL",internal:true,references:[{target:"ENTITY",entityId:ids[0],subElement:"WHOLE"},{target:"ENTITY",entityId:ids[index],subElement:"WHOLE"}]}},{type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"ANGLE",internal:true,value:60,unit:"deg",references:[{target:"ENTITY",entityId:ids[index-1],subElement:"DIRECTION"},{target:"ENTITY",entityId:ids[index],subElement:"DIRECTION"}]}});}context.viewport.commitSketchOperations(operations);context.viewport.finishToolUse();}
}

export class SlotSketchTool implements CadTool {
  readonly id="sketch.slot";private first?:Vec2;private second?:Vec2;private capturedPointerID?:number;
  activate(context:ToolContext):void{context.viewport.setToolPrompt("长圆槽：单击第一圆心");}
  private geometry(widthPoint:Vec2){const first=this.first!,second=this.second!,dx=second[0]-first[0],dy=second[1]-first[1],length=Math.hypot(dx,dy);const normal:Vec2=[-dy/length,dx/length];const middle:Vec2=[(first[0]+second[0])/2,(first[1]+second[1])/2];const radius=Math.max(SKETCH_INPUT_POLICY.minimumGeometryLength,Math.abs((widthPoint[0]-middle[0])*normal[0]+(widthPoint[1]-middle[1])*normal[1]));const angle=Math.atan2(normal[1],normal[0]);return{normal,radius,angle};}
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult{if(event.button!==0||this.capturedPointerID!==undefined||!context.viewport.hasActiveSketch())return InputResult.Ignored;const point=context.viewport.sketchPoint(event.x,event.y);if(!point)return InputResult.Ignored;this.capturedPointerID=event.pointerId;
    if(!this.first){this.first=point;context.viewport.setToolPrompt("长圆槽：单击第二圆心");return InputResult.Capture;}if(!this.second){if(Math.hypot(point[0]-this.first[0],point[1]-this.first[1])<SKETCH_INPUT_POLICY.minimumGeometryLength)return InputResult.Consumed;this.second=point;context.viewport.setToolPrompt("长圆槽：单击确定半宽；Esc 取消");return InputResult.Capture;}
    const {normal,radius,angle}=this.geometry(point),first=this.first,second=this.second;const ids=Array.from({length:4},()=>randomUUID());const topA:Vec2=[first[0]+normal[0]*radius,first[1]+normal[1]*radius],topB:Vec2=[second[0]+normal[0]*radius,second[1]+normal[1]*radius],bottomB:Vec2=[second[0]-normal[0]*radius,second[1]-normal[1]*radius],bottomA:Vec2=[first[0]-normal[0]*radius,first[1]-normal[1]*radius];
    const entities:SketchOperation[]=[{type:"ADD_ENTITY",entity:{id:ids[0],kind:"LINE",role:"PROFILE",start:{x:topA[0],y:topA[1]},end:{x:topB[0],y:topB[1]}}},{type:"ADD_ENTITY",entity:{id:ids[1],kind:"ARC",role:"PROFILE",center:{x:second[0],y:second[1]},radius,startAngle:angle,endAngle:angle+Math.PI}},{type:"ADD_ENTITY",entity:{id:ids[2],kind:"LINE",role:"PROFILE",start:{x:bottomB[0],y:bottomB[1]},end:{x:bottomA[0],y:bottomA[1]}}},{type:"ADD_ENTITY",entity:{id:ids[3],kind:"ARC",role:"PROFILE",center:{x:first[0],y:first[1]},radius,startAngle:angle+Math.PI,endAngle:angle+Math.PI*2}}];
    for(let index=0;index<4;index+=1)entities.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",internal:true,references:[{target:"ENTITY",entityId:ids[index],subElement:"END"},{target:"ENTITY",entityId:ids[(index+1)%4],subElement:"START"}]}},{type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"TANGENT",internal:true,references:[{target:"ENTITY",entityId:ids[index],subElement:"WHOLE"},{target:"ENTITY",entityId:ids[(index+1)%4],subElement:"WHOLE"}]}});entities.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"EQUAL",internal:true,references:[{target:"ENTITY",entityId:ids[1],subElement:"WHOLE"},{target:"ENTITY",entityId:ids[3],subElement:"WHOLE"}]}});context.viewport.commitSketchOperations(entities);this.reset(context);context.viewport.finishToolUse();return InputResult.Capture;}
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult{if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;const point=context.viewport.sketchPoint(event.x,event.y);if(!point)return InputResult.Ignored;if(!this.first)return InputResult.Consumed;if(!this.second){context.viewport.showPolylinePreview([this.first,point]);return InputResult.Consumed;}const{normal,radius}=this.geometry(point);context.viewport.showPolylinePreview([[this.first[0]+normal[0]*radius,this.first[1]+normal[1]*radius],[this.second[0]+normal[0]*radius,this.second[1]+normal[1]*radius],[this.second[0]-normal[0]*radius,this.second[1]-normal[1]*radius],[this.first[0]-normal[0]*radius,this.first[1]-normal[1]*radius]],true);return InputResult.Consumed;}
  pointerUp(event:CadPointerEvent):InputResult{if(event.button!==0||event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.capturedPointerID=undefined;return InputResult.ReleaseCapture;}pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult{if(event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.reset(context);return InputResult.Consumed;}keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult{if(event.key!=="Escape")return InputResult.Ignored;this.reset(context);return InputResult.Consumed;}deactivate(context:ToolContext):void{this.reset(context);}private reset(context:ToolContext):void{this.first=undefined;this.second=undefined;this.capturedPointerID=undefined;context.viewport.clearToolPreview();context.viewport.setToolPrompt("长圆槽：单击第一圆心");}
}

export class CircleSketchTool extends TwoClickSketchTool {
  readonly id = "sketch.circle";
  readonly firstPrompt = "圆：单击圆心";
  readonly secondPrompt = "圆：移动预览，单击圆周点；Esc 取消";
  preview(center: Vec2, edge: Vec2, context: ToolContext): void {
    const radius=Math.hypot(edge[0]-center[0],edge[1]-center[1]);
    context.viewport.showPolylinePreview(Array.from({length:65},(_,index):Vec2=>[center[0]+radius*Math.cos(index*Math.PI/32),center[1]+radius*Math.sin(index*Math.PI/32)]));
    context.viewport.showReferenceDimensions([{kind:"CIRCLE",center,edge}]);
  }
  commit(center: Vec2, edge: Vec2, context: ToolContext, snaps: { first?: SketchGeometryRef }): void {
    const id=randomUUID();const operations:SketchOperation[]=[{type:"ADD_ENTITY",entity:{id,kind:"CIRCLE",role:"PROFILE",
      center:{x:center[0],y:center[1]},radius:Math.hypot(edge[0]-center[0],edge[1]-center[1])}}];
    if(snaps.first)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",
      references:[{target:"ENTITY",entityId:id,subElement:"CENTER"},snaps.first]}});
    context.viewport.commitSketchOperations(operations);
    context.viewport.finishToolUse();
  }
}

export class ArcSketchTool implements CadTool {
  readonly id="sketch.arc"; private center?:Vec2; private start?:Vec2; private centerSnap?:SketchGeometryRef; private startSnap?:SketchGeometryRef; private capturedPointerID?:number;
  activate(context:ToolContext):void { context.viewport.setToolPrompt("圆弧：单击圆心"); }
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult {
    if(event.button!==0||this.capturedPointerID!==undefined||!context.viewport.hasActiveSketch())return InputResult.Ignored;
    const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;this.capturedPointerID=event.pointerId;
    if(!this.center){this.center=value;this.centerSnap=context.viewport.sketchSnapReference();context.viewport.setToolPrompt("圆弧：单击起点");return InputResult.Capture;}
    if(!this.start){if(Math.hypot(value[0]-this.center[0],value[1]-this.center[1])<SKETCH_INPUT_POLICY.minimumGeometryLength)return InputResult.Consumed;this.start=value;this.startSnap=context.viewport.sketchSnapReference();context.viewport.setToolPrompt("圆弧：单击终点；Esc 取消");return InputResult.Capture;}
    const center=this.center,start=this.start,radius=Math.hypot(start[0]-center[0],start[1]-center[1]);
    let startAngle=Math.atan2(start[1]-center[1],start[0]-center[0]);let endAngle=Math.atan2(value[1]-center[1],value[0]-center[0]);
    while(endAngle<=startAngle+1e-6)endAngle+=Math.PI*2;
    if(endAngle-startAngle>=Math.PI*2-1e-6)return InputResult.Consumed;
    const id=randomUUID(),operations:SketchOperation[]=[{type:"ADD_ENTITY",entity:{id,kind:"ARC",role:"PROFILE",center:{x:center[0],y:center[1]},radius,startAngle,endAngle}}];
    for(const [subElement,target] of [["CENTER",this.centerSnap],["START",this.startSnap],["END",context.viewport.sketchSnapReference()]] as const)
      if(target)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:id,subElement},target]}});
    context.viewport.clearToolPreview();context.viewport.commitSketchOperations(operations);
    this.center=undefined;this.start=undefined;this.centerSnap=undefined;this.startSnap=undefined;context.viewport.finishToolUse();context.viewport.setToolPrompt("圆弧：单击圆心");return InputResult.Capture;
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult {
    if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;if(!this.center)return InputResult.Consumed;
    if(!this.start){context.viewport.showPolylinePreview([this.center,value]);context.viewport.showReferenceDimensions([{kind:"CIRCLE",center:this.center,edge:value}]);return InputResult.Consumed;}
    const radius=Math.hypot(this.start[0]-this.center[0],this.start[1]-this.center[1]);const first=Math.atan2(this.start[1]-this.center[1],this.start[0]-this.center[0]);let last=Math.atan2(value[1]-this.center[1],value[0]-this.center[0]);while(last<=first)last+=Math.PI*2;
    context.viewport.showPolylinePreview(Array.from({length:49},(_,index):Vec2=>{const angle=first+(last-first)*index/48;return[this.center![0]+radius*Math.cos(angle),this.center![1]+radius*Math.sin(angle)];}));
    context.viewport.showReferenceDimensions([{kind:"CIRCLE",center:this.center,edge:this.start}]);return InputResult.Consumed;
  }
  pointerUp(event:CadPointerEvent):InputResult {if(event.button!==0||event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.capturedPointerID=undefined;return InputResult.ReleaseCapture;}
  pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult {if(event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult {if(event.key!=="Escape")return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  deactivate(context:ToolContext):void {this.cancel(context);} cancel(context:ToolContext):void {this.center=undefined;this.start=undefined;this.centerSnap=undefined;this.startSnap=undefined;this.capturedPointerID=undefined;context.viewport.clearToolPreview();context.viewport.setToolPrompt("圆弧：单击圆心");}
}

abstract class MultiPointSketchTool implements CadTool {
  abstract readonly id:string; protected points:Vec2[]=[]; protected snaps:(SketchGeometryRef|undefined)[]=[]; private capturedPointerID?:number;
  private lastClick?:{x:number;y:number;at:number};
  abstract readonly prompt:string; abstract minimumPoints:number; abstract commit(context:ToolContext):void;
  activate(context:ToolContext):void {context.viewport.setToolPrompt(this.prompt);}
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult {
    if(event.button!==0||this.capturedPointerID!==undefined||!context.viewport.hasActiveSketch())return InputResult.Ignored;
    const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;this.capturedPointerID=event.pointerId;
    const at=Number(event.originalEvent.timeStamp)||Date.now(),previous=this.lastClick;
    if(previous&&at-previous.at<=450&&Math.hypot(event.x-previous.x,event.y-previous.y)<=6&&this.points.length>=this.minimumPoints){this.lastClick=undefined;this.finish(context);return InputResult.Capture;}
    if(this.points.length>0&&Math.hypot(value[0]-this.points[0][0],value[1]-this.points[0][1])<SKETCH_INPUT_POLICY.minimumGeometryLength&&this.points.length>=this.minimumPoints){this.points.push(this.points[0]);this.lastClick=undefined;this.finish(context);return InputResult.Capture;}
    this.points.push(value);this.snaps.push(context.viewport.sketchSnapReference());this.lastClick={x:event.x,y:event.y,at};context.viewport.showPolylinePreview(this.points);return InputResult.Capture;
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult {if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;if(this.points.length>0)context.viewport.showPolylinePreview([...this.points,value]);return InputResult.Consumed;}
  pointerUp(event:CadPointerEvent):InputResult {if(event.button!==0||event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.capturedPointerID=undefined;return InputResult.ReleaseCapture;}
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult {if(event.key==="Enter"&&this.points.length>=this.minimumPoints){this.finish(context);return InputResult.Consumed;}if(event.key==="Escape"){this.cancel(context);return InputResult.Consumed;}return InputResult.Ignored;}
  pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult {if(event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  private finish(context:ToolContext):void {context.viewport.clearToolPreview();this.commit(context);this.points=[];this.snaps=[];this.lastClick=undefined;context.viewport.finishToolUse();context.viewport.setToolPrompt(this.prompt);}
  deactivate(context:ToolContext):void {this.cancel(context);} cancel(context:ToolContext):void {this.points=[];this.snaps=[];this.capturedPointerID=undefined;this.lastClick=undefined;context.viewport.clearToolPreview();context.viewport.setToolPrompt(this.prompt);}
}

export class PolylineSketchTool extends MultiPointSketchTool {
  readonly id="sketch.polyline";readonly prompt="多段线：依次单击顶点，双击或 Enter 完成，单击首点闭合";minimumPoints=2;
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult {if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;
    const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;const last=this.points.at(-1);if(last){context.viewport.showPolylinePreview([...this.points,value]);
      context.viewport.showReferenceDimensions([{kind:"LINE",start:last,end:value}]);}return InputResult.Consumed;}
  commit(context:ToolContext):void {const closed=this.points.at(-1)===this.points[0];const points=closed?this.points.slice(0,-1):this.points;
    const operations=polylineOperations(points,closed),ids=operations.filter((item)=>item.type==="ADD_ENTITY").map((item)=>item.entity.id);
    if(this.snaps[0]&&ids[0])operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:ids[0],subElement:"START"},this.snaps[0]]}});
    const lastSnap=this.snaps[closed?0:points.length-1];if(lastSnap&&ids.at(-1))operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:ids.at(-1),subElement:"END"},lastSnap]}});
    context.viewport.commitSketchOperations(operations);}
}

export class SplineSketchTool extends MultiPointSketchTool {
  readonly id="sketch.spline";readonly prompt="插值曲线：依次单击通过点，双击或 Enter 完成，单击首点闭合";minimumPoints=3;
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult {if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;
    const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;if(this.points.length>0){const fit=[...this.points,value];
      context.viewport.showPolylinePreview(sampleInterpolatingSpline(fit,false,64));
      context.viewport.showReferenceDimensions([{kind:"LINE",start:fit.at(-2)!,end:value}]);}return InputResult.Consumed;}
  commit(context:ToolContext):void {const closed=this.points.length>3&&this.points.at(-1)===this.points[0];const controls=closed?this.points.slice(0,-1):this.points,id=randomUUID();
    const operations:SketchOperation[]=[{type:"ADD_ENTITY",entity:{id,kind:"SPLINE",role:"PROFILE",controlPoints:controls.map(([x,y])=>({x,y})),degree:Math.min(3,controls.length-1),closed}}];
    if(this.snaps[0])operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:id,subElement:"START"},this.snaps[0]]}});
    const endSnap=this.snaps[closed?0:controls.length-1];if(endSnap)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:id,subElement:"END"},endSnap]}});
    context.viewport.commitSketchOperations(operations);}
}

export class PointSketchTool implements CadTool {
  readonly id = "sketch.point";
  private capturedPointerID?: number;
  activate(context: ToolContext): void { context.viewport.setToolPrompt("点：单击放置；Esc 返回选择"); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || this.capturedPointerID !== undefined || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y); if (!point) return InputResult.Ignored;
    this.capturedPointerID = event.pointerId;
    const id=randomUUID(),operations:SketchOperation[]=[{ type: "ADD_ENTITY", entity: { id, kind: "POINT", role: "PROFILE", point: { x: point[0], y: point[1] } } }];
    const snap=context.viewport.sketchSnapReference();if(snap)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:id,subElement:"POINT"},snap]}});
    context.viewport.commitSketchOperations(operations);
    context.viewport.finishToolUse();
    return InputResult.Capture;
  }
  pointerUp(event: CadPointerEvent): InputResult {
    if (event.button !== 0 || event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined;
    return InputResult.ReleaseCapture;
  }
  pointerCancel(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined;
    this.cancel(context);
    return InputResult.Consumed;
  }
  pointerMove(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.state.buttons.middle || event.state.buttons.right || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y); if (!point) return InputResult.Ignored;
    context.viewport.showPointPreview(point);
    context.viewport.showReferenceDimensions([{kind:"POINT",point}]);
    return InputResult.Consumed;
  }
  deactivate(context: ToolContext): void { this.cancel(context); }
  cancel(context: ToolContext): void { this.capturedPointerID = undefined; context.viewport.clearToolPreview(); }
}

// Constraint tools intentionally share the same tool lifecycle now. Entity
// reference picking is the next extension point; commands stay typed and no
// topology or array index is persisted.
export class ConstraintSketchTool implements CadTool {
  private references:SketchGeometryRef[]=[];
  private capturedPointerID?: number;
  private labelPosition?:Vec2;
  readonly id:string;
  private readonly kind:ConstraintKind;
  constructor(kind:ConstraintKind|string) {
    this.kind=(kind.startsWith("sketch.constraint.")?kind.slice("sketch.constraint.".length).toUpperCase():kind) as ConstraintKind;
    this.id=`sketch.constraint.${this.kind.toLowerCase()}`;
  }
  private get spec(){return constraintDefinition(this.kind);}
  private prompt():string {if(this.references.length===this.spec.picks.length&&this.spec.unit)return `${this.spec.label}：移动并单击放置尺寸，随后编辑当前值`;
    return `${this.spec.label}约束：选择${this.spec.pickLabels[this.references.length]}；Esc 取消`;}
  activate(context: ToolContext): void { context.viewport.setToolPrompt(this.prompt()); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || this.capturedPointerID !== undefined || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    if(this.references.length>=this.spec.picks.length){
      if(!this.spec.unit||this.labelPosition)return InputResult.Ignored;
      const position=context.viewport.sketchPlacementPoint(event.x,event.y);if(!position)return InputResult.Ignored;
      const value=context.viewport.measureDimension(this.kind,this.references);if(!value||!this.spec.unit)return InputResult.Ignored;
      this.capturedPointerID=event.pointerId;this.labelPosition=position;
      context.viewport.requestDimensionCreation(this.kind,this.references,value,this.spec.unit,position,event.x,event.y);
      this.references=[];this.labelPosition=undefined;context.viewport.clearReferencePreview();
      context.viewport.setToolPrompt(this.prompt());context.viewport.finishToolUse();return InputResult.Capture;
    }
    const reference = context.viewport.sketchReferenceAt(event.x, event.y, this.spec.picks[this.references.length], this.references[0]); if (!reference) return InputResult.Ignored;
    if (this.references.some((item)=>sameSketchReference(item, reference))) {
      context.viewport.showReferencePreview(reference,this.references);
      return InputResult.Consumed;
    }
    this.capturedPointerID = event.pointerId;
    this.references.push(reference);context.viewport.showReferencePreview(reference,this.references);
    if(this.references.length===this.spec.picks.length&&!this.spec.unit)this.commit(context);
    else {
      if(this.references.length===this.spec.picks.length)context.viewport.showConstraintPreview(this.kind,this.references,
        context.viewport.measureDimension(this.kind,this.references));
      context.viewport.setToolPrompt(this.prompt());
    }
    return InputResult.Capture;
  }
  pointerUp(event: CadPointerEvent): InputResult {
    if (event.button !== 0 || event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined;
    return InputResult.ReleaseCapture;
  }
  pointerCancel(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined;
    this.cancel(context);
    return InputResult.Consumed;
  }
  pointerMove(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    if(this.references.length>=this.spec.picks.length){
      if(!this.spec.unit||this.labelPosition)return InputResult.Ignored;
      const position=context.viewport.sketchPlacementPoint(event.x,event.y);if(!position)return InputResult.Ignored;
      context.viewport.showConstraintPreview(this.kind,this.references,context.viewport.measureDimension(this.kind,this.references),position);return InputResult.Consumed;
    }
    const reference = context.viewport.sketchReferenceAt(event.x, event.y, this.spec.picks[this.references.length], this.references[0]);
    if (!reference) {
      if (this.references[0]) context.viewport.showReferencePreview(this.references.at(-1)!,this.references);
      else context.viewport.clearReferencePreview();
      return InputResult.Ignored;
    }
    if (this.references.some((item)=>sameSketchReference(item,reference))) {
      context.viewport.showReferencePreview(reference,this.references);
      return InputResult.Consumed;
    }
    context.viewport.showReferencePreview(reference, this.references);
    return InputResult.Consumed;
  }
  keyDown(event: CadKeyboardEvent, context: ToolContext): InputResult {
    if(event.key==="Escape"){this.cancel(context);return InputResult.Consumed;}
    return InputResult.Ignored;
  }
  deactivate(context: ToolContext): void { this.cancel(context); }
  cancel(context: ToolContext): void {
    this.capturedPointerID = undefined;
    this.references = [];
    this.labelPosition=undefined;
    context.viewport.clearReferencePreview();
    context.viewport.setToolPrompt(this.prompt());
  }
  private commit(context:ToolContext):void {context.viewport.clearReferencePreview();context.viewport.commitSketchOperations([{type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:this.kind,references:this.references}}]);this.references=[];this.labelPosition=undefined;context.viewport.finishToolUse();context.viewport.setToolPrompt(this.prompt());}
}

export class LinearDimensionSketchTool implements CadTool {
  readonly id="sketch.dimension.linear";
  private references:SketchGeometryRef[]=[];
  private kind?:"DISTANCE"|"LENGTH";
  private labelPosition?:Vec2;
  private capturedPointerID?:number;
  activate(context:ToolContext):void {context.viewport.setToolPrompt("线性尺寸：选择直线或第一个点");}
  private referencesComplete():boolean {return this.kind==="LENGTH"?this.references.length===1:this.kind==="DISTANCE"&&this.references.length===2;}
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult {
    if(event.button!==0||this.capturedPointerID!==undefined||!context.viewport.hasActiveSketch())return InputResult.Ignored;
    if(this.referencesComplete()){
      if(this.labelPosition)return InputResult.Ignored;
      const position=context.viewport.sketchPlacementPoint(event.x,event.y);if(!position)return InputResult.Ignored;
      const value=context.viewport.measureDimension(this.kind!,this.references);if(!value)return InputResult.Ignored;
      this.capturedPointerID=event.pointerId;this.labelPosition=position;
      context.viewport.requestDimensionCreation(this.kind!,this.references,value,"mm",position,event.x,event.y);
      this.references=[];this.kind=undefined;this.labelPosition=undefined;context.viewport.clearReferencePreview();
      context.viewport.finishToolUse();return InputResult.Capture;
    }
    const reference=context.viewport.sketchReferenceAt(event.x,event.y,"LINEAR_DIMENSION",this.references[0]);
    if(!reference||this.references.some((item)=>sameSketchReference(item,reference)))return InputResult.Ignored;
    this.capturedPointerID=event.pointerId;this.references.push(reference);
    if(this.references.length===1)this.kind=reference.target==="ENTITY"&&reference.subElement==="WHOLE"?"LENGTH":"DISTANCE";
    else this.kind="DISTANCE";
    context.viewport.showReferencePreview(reference,this.references);
    if(this.referencesComplete())context.viewport.showConstraintPreview(this.kind!,this.references,
      context.viewport.measureDimension(this.kind!,this.references));
    return InputResult.Capture;
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult {
    if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;
    if(this.referencesComplete()){
      if(this.labelPosition)return InputResult.Ignored;
      const position=context.viewport.sketchPlacementPoint(event.x,event.y);if(!position)return InputResult.Ignored;
      context.viewport.showConstraintPreview(this.kind!,this.references,context.viewport.measureDimension(this.kind!,this.references),position);return InputResult.Consumed;
    }
    const reference=context.viewport.sketchReferenceAt(event.x,event.y,"LINEAR_DIMENSION",this.references[0]);
    if(reference&&!this.references.some((item)=>sameSketchReference(item,reference)))context.viewport.showReferencePreview(reference,this.references);
    else if(this.references[0])context.viewport.showReferencePreview(this.references.at(-1)!,this.references);
    else context.viewport.clearReferencePreview();
    return InputResult.Consumed;
  }
  pointerUp(event:CadPointerEvent):InputResult {if(event.button!==0||event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.capturedPointerID=undefined;return InputResult.ReleaseCapture;}
  pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult {if(event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult {
    if(event.key==="Escape"){this.cancel(context);return InputResult.Consumed;}
    return InputResult.Ignored;
  }
  deactivate(context:ToolContext):void {this.cancel(context);}
  cancel(context:ToolContext):void {this.references=[];this.kind=undefined;this.labelPosition=undefined;this.capturedPointerID=undefined;context.viewport.clearReferencePreview();}
}
