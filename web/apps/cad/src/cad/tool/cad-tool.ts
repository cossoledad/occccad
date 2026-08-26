import type { CadKeyboardEvent, CadPointerEvent } from "../input/input-types";
import { InputResult } from "../input/input-types";
import type { SketchGeometryRef, SketchOperation, Vec2 } from "../../types";
import type { SketchReferencePickKind } from "../interaction/sketch-reference-pick";
import { constraintDefinition, type ConstraintKind } from "../sketch/sketch-constraint-definition";
import { randomUUID } from "../../utils/random-uuid";

export type ToolViewportPort = {
  sketchPoint(x: number, y: number): Vec2 | null;
  sketchPlacementPoint(x: number, y: number): Vec2 | null;
  showPolylinePreview(points: Vec2[], closed?: boolean): void;
  showPointPreview(point: Vec2): void;
  clearToolPreview(): void;
  commitSketchOperations(operations: SketchOperation[]): void;
  hasActiveSketch(): boolean;
  sketchReferenceAt(x: number, y: number, kind: SketchReferencePickKind, retained?: SketchGeometryRef): SketchGeometryRef | null;
  showReferencePreview(reference: SketchGeometryRef, retained?: SketchGeometryRef): void;
  showConstraintPreview(kind: ConstraintKind, references: readonly SketchGeometryRef[], value?: number, labelPosition?: Vec2): void;
  beginDimensionDrag(x: number, y: number): boolean;
  updateDimensionDrag(x: number, y: number): void;
  finishDimensionDrag(): void;
  cancelDimensionDrag(): void;
  editDimensionAt(x: number, y: number): boolean;
  clearReferencePreview(): void;
  setToolPrompt(prompt: string): void;
  finishToolUse(): void;
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
  private dimensionPointer?: number;
  activate(context: ToolContext): void { context.viewport.setToolPrompt("选择：选择草图元素，或从工具栏启动创建命令"); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    if (event.originalEvent.detail >= 2 && context.viewport.editDimensionAt(event.x, event.y)) return InputResult.Consumed;
    if (!context.viewport.beginDimensionDrag(event.x, event.y)) return InputResult.Ignored;
    this.dimensionPointer = event.pointerId;
    return InputResult.Capture;
  }
  pointerMove(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.pointerId !== this.dimensionPointer) return InputResult.Ignored;
    context.viewport.updateDimensionDrag(event.x, event.y);
    return InputResult.Consumed;
  }
  pointerUp(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.pointerId !== this.dimensionPointer || event.button !== 0) return InputResult.Ignored;
    this.dimensionPointer = undefined; context.viewport.finishDimensionDrag();
    return InputResult.ReleaseCapture;
  }
  pointerCancel(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.pointerId !== this.dimensionPointer) return InputResult.Ignored;
    this.dimensionPointer = undefined; context.viewport.cancelDimensionDrag();
    return InputResult.Consumed;
  }
  cancel(context: ToolContext): void { this.dimensionPointer = undefined; context.viewport.cancelDimensionDrag(); }
}

abstract class TwoClickSketchTool implements CadTool {
  abstract readonly id: string;
  protected first?: Vec2;
  private capturedPointerID?: number;
  abstract preview(first: Vec2, second: Vec2, context: ToolContext): void;
  abstract commit(first: Vec2, second: Vec2, context: ToolContext): void;
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
      this.preview(point, point, context);
      context.viewport.setToolPrompt(this.secondPrompt);
      return InputResult.Capture;
    }
    const first = this.first; this.first = undefined; context.viewport.clearToolPreview();
    if (Math.hypot(point[0] - first[0], point[1] - first[1]) >= 0.5) this.commit(first, point, context);
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
    context.viewport.clearToolPreview();
    context.viewport.setToolPrompt(this.firstPrompt);
  }
}

export class LineSketchTool extends TwoClickSketchTool {
  readonly id = "sketch.line";
  readonly firstPrompt = "直线：单击起点";
  readonly secondPrompt = "直线：移动预览，单击终点；Esc 取消当前线";
  preview(first: Vec2, second: Vec2, context: ToolContext): void { context.viewport.showPolylinePreview([first, second]); }
  commit(first: Vec2, second: Vec2, context: ToolContext): void {
    context.viewport.commitSketchOperations([{ type: "ADD_ENTITY", entity: { id: randomUUID(), kind: "LINE", role: "PROFILE", start: { x: first[0], y: first[1] }, end: { x: second[0], y: second[1] } } }]);
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
  }
  commit(first: Vec2, second: Vec2, context: ToolContext): void {
    context.viewport.commitSketchOperations([{ type: "ADD_RECTANGLE", first: { x: first[0], y: first[1] }, second: { x: second[0], y: second[1] } }]);
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
  preview(center:Vec2,vertex:Vec2,context:ToolContext):void {context.viewport.showPolylinePreview(this.vertices(center,vertex),true);}
  commit(center:Vec2,vertex:Vec2,context:ToolContext):void {const operations=polylineOperations(this.vertices(center,vertex),true);const ids=operations.filter((item)=>item.type==="ADD_ENTITY").map((item)=>item.entity.id);for(let index=1;index<ids.length;index+=1){operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"EQUAL",internal:true,references:[{target:"ENTITY",entityId:ids[0],subElement:"WHOLE"},{target:"ENTITY",entityId:ids[index],subElement:"WHOLE"}]}},{type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"ANGLE",internal:true,value:60,unit:"deg",references:[{target:"ENTITY",entityId:ids[index-1],subElement:"DIRECTION"},{target:"ENTITY",entityId:ids[index],subElement:"DIRECTION"}]}});}context.viewport.commitSketchOperations(operations);context.viewport.finishToolUse();}
}

export class SlotSketchTool implements CadTool {
  readonly id="sketch.slot";private first?:Vec2;private second?:Vec2;private capturedPointerID?:number;
  activate(context:ToolContext):void{context.viewport.setToolPrompt("长圆槽：单击第一圆心");}
  private geometry(widthPoint:Vec2){const first=this.first!,second=this.second!,dx=second[0]-first[0],dy=second[1]-first[1],length=Math.hypot(dx,dy);const normal:Vec2=[-dy/length,dx/length];const middle:Vec2=[(first[0]+second[0])/2,(first[1]+second[1])/2];const radius=Math.max(0.5,Math.abs((widthPoint[0]-middle[0])*normal[0]+(widthPoint[1]-middle[1])*normal[1]));const angle=Math.atan2(normal[1],normal[0]);return{normal,radius,angle};}
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult{if(event.button!==0||this.capturedPointerID!==undefined||!context.viewport.hasActiveSketch())return InputResult.Ignored;const point=context.viewport.sketchPoint(event.x,event.y);if(!point)return InputResult.Ignored;this.capturedPointerID=event.pointerId;
    if(!this.first){this.first=point;context.viewport.setToolPrompt("长圆槽：单击第二圆心");return InputResult.Capture;}if(!this.second){if(Math.hypot(point[0]-this.first[0],point[1]-this.first[1])<0.5)return InputResult.Consumed;this.second=point;context.viewport.setToolPrompt("长圆槽：单击确定半宽；Esc 取消");return InputResult.Capture;}
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
  }
  commit(center: Vec2, edge: Vec2, context: ToolContext): void {
    context.viewport.commitSketchOperations([{type:"ADD_ENTITY",entity:{id:randomUUID(),kind:"CIRCLE",role:"PROFILE",
      center:{x:center[0],y:center[1]},radius:Math.hypot(edge[0]-center[0],edge[1]-center[1])}}]);
    context.viewport.finishToolUse();
  }
}

export class ArcSketchTool implements CadTool {
  readonly id="sketch.arc"; private center?:Vec2; private start?:Vec2; private capturedPointerID?:number;
  activate(context:ToolContext):void { context.viewport.setToolPrompt("圆弧：单击圆心"); }
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult {
    if(event.button!==0||this.capturedPointerID!==undefined||!context.viewport.hasActiveSketch())return InputResult.Ignored;
    const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;this.capturedPointerID=event.pointerId;
    if(!this.center){this.center=value;context.viewport.setToolPrompt("圆弧：单击起点");return InputResult.Capture;}
    if(!this.start){if(Math.hypot(value[0]-this.center[0],value[1]-this.center[1])<0.5)return InputResult.Consumed;this.start=value;context.viewport.setToolPrompt("圆弧：单击终点；Esc 取消");return InputResult.Capture;}
    const center=this.center,start=this.start,radius=Math.hypot(start[0]-center[0],start[1]-center[1]);
    let startAngle=Math.atan2(start[1]-center[1],start[0]-center[0]);let endAngle=Math.atan2(value[1]-center[1],value[0]-center[0]);
    while(endAngle<=startAngle+1e-6)endAngle+=Math.PI*2;
    if(endAngle-startAngle>=Math.PI*2-1e-6)return InputResult.Consumed;
    context.viewport.clearToolPreview();context.viewport.commitSketchOperations([{type:"ADD_ENTITY",entity:{id:randomUUID(),kind:"ARC",role:"PROFILE",center:{x:center[0],y:center[1]},radius,startAngle,endAngle}}]);
    this.center=undefined;this.start=undefined;context.viewport.finishToolUse();context.viewport.setToolPrompt("圆弧：单击圆心");return InputResult.Capture;
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult {
    if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;if(!this.center)return InputResult.Consumed;
    if(!this.start){context.viewport.showPolylinePreview([this.center,value]);return InputResult.Consumed;}
    const radius=Math.hypot(this.start[0]-this.center[0],this.start[1]-this.center[1]);const first=Math.atan2(this.start[1]-this.center[1],this.start[0]-this.center[0]);let last=Math.atan2(value[1]-this.center[1],value[0]-this.center[0]);while(last<=first)last+=Math.PI*2;
    context.viewport.showPolylinePreview(Array.from({length:49},(_,index):Vec2=>{const angle=first+(last-first)*index/48;return[this.center![0]+radius*Math.cos(angle),this.center![1]+radius*Math.sin(angle)];}));return InputResult.Consumed;
  }
  pointerUp(event:CadPointerEvent):InputResult {if(event.button!==0||event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.capturedPointerID=undefined;return InputResult.ReleaseCapture;}
  pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult {if(event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult {if(event.key!=="Escape")return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  deactivate(context:ToolContext):void {this.cancel(context);} cancel(context:ToolContext):void {this.center=undefined;this.start=undefined;this.capturedPointerID=undefined;context.viewport.clearToolPreview();context.viewport.setToolPrompt("圆弧：单击圆心");}
}

abstract class MultiPointSketchTool implements CadTool {
  abstract readonly id:string; protected points:Vec2[]=[]; private capturedPointerID?:number;
  abstract readonly prompt:string; abstract minimumPoints:number; abstract commit(context:ToolContext):void;
  activate(context:ToolContext):void {context.viewport.setToolPrompt(this.prompt);}
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult {
    if(event.button!==0||this.capturedPointerID!==undefined||!context.viewport.hasActiveSketch())return InputResult.Ignored;
    const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;this.capturedPointerID=event.pointerId;
    if(event.originalEvent?.detail>=2&&this.points.length>=this.minimumPoints){this.finish(context);return InputResult.Capture;}
    if(this.points.length>0&&Math.hypot(value[0]-this.points[0][0],value[1]-this.points[0][1])<0.5&&this.points.length>=this.minimumPoints){this.points.push(this.points[0]);this.finish(context);return InputResult.Capture;}
    this.points.push(value);context.viewport.showPolylinePreview(this.points);return InputResult.Capture;
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult {if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;if(this.points.length>0)context.viewport.showPolylinePreview([...this.points,value]);return InputResult.Consumed;}
  pointerUp(event:CadPointerEvent):InputResult {if(event.button!==0||event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.capturedPointerID=undefined;return InputResult.ReleaseCapture;}
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult {if(event.key==="Enter"&&this.points.length>=this.minimumPoints){this.finish(context);return InputResult.Consumed;}if(event.key==="Escape"){this.cancel(context);return InputResult.Consumed;}return InputResult.Ignored;}
  pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult {if(event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  private finish(context:ToolContext):void {context.viewport.clearToolPreview();this.commit(context);this.points=[];context.viewport.finishToolUse();context.viewport.setToolPrompt(this.prompt);}
  deactivate(context:ToolContext):void {this.cancel(context);} cancel(context:ToolContext):void {this.points=[];this.capturedPointerID=undefined;context.viewport.clearToolPreview();context.viewport.setToolPrompt(this.prompt);}
}

export class PolylineSketchTool extends MultiPointSketchTool {
  readonly id="sketch.polyline";readonly prompt="多段线：依次单击顶点，双击或 Enter 完成，单击首点闭合";minimumPoints=2;
  commit(context:ToolContext):void {const closed=this.points.at(-1)===this.points[0];context.viewport.commitSketchOperations(polylineOperations(closed?this.points.slice(0,-1):this.points,closed));}
}

export class SplineSketchTool extends MultiPointSketchTool {
  readonly id="sketch.spline";readonly prompt="草图曲线：依次单击控制点，双击或 Enter 完成，单击首点闭合";minimumPoints=3;
  commit(context:ToolContext):void {const closed=this.points.length>3&&this.points.at(-1)===this.points[0];const controls=closed?this.points.slice(0,-1):this.points;
    context.viewport.commitSketchOperations([{type:"ADD_ENTITY",entity:{id:randomUUID(),kind:"SPLINE",role:"PROFILE",controlPoints:controls.map(([x,y])=>({x,y})),degree:Math.min(3,controls.length-1),closed}}]);}
}

export class PointSketchTool implements CadTool {
  readonly id = "sketch.point";
  private capturedPointerID?: number;
  activate(context: ToolContext): void { context.viewport.setToolPrompt("点：单击放置；Esc 返回选择"); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || this.capturedPointerID !== undefined || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y); if (!point) return InputResult.Ignored;
    this.capturedPointerID = event.pointerId;
    context.viewport.commitSketchOperations([{ type: "ADD_ENTITY", entity: { id: randomUUID(), kind: "POINT", role: "PROFILE", point: { x: point[0], y: point[1] } } }]);
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
  private numeric="";
  private labelPosition?:Vec2;
  readonly id:string;
  private readonly kind:ConstraintKind;
  constructor(kind:ConstraintKind|string) {
    this.kind=(kind.startsWith("sketch.constraint.")?kind.slice("sketch.constraint.".length).toUpperCase():kind) as ConstraintKind;
    this.id=`sketch.constraint.${this.kind.toLowerCase()}`;
  }
  private get spec(){return constraintDefinition(this.kind);}
  private prompt():string {if(this.references.length===this.spec.picks.length&&this.spec.unit)return `${this.spec.label}：输入数值 (${this.spec.unit})，Enter 确认`;
    return `${this.spec.label}约束：选择${this.spec.pickLabels[this.references.length]}；Esc 取消`;}
  activate(context: ToolContext): void { context.viewport.setToolPrompt(this.prompt()); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || this.capturedPointerID !== undefined || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    if(this.references.length>=this.spec.picks.length){
      if(!this.spec.unit||this.labelPosition)return InputResult.Ignored;
      const position=context.viewport.sketchPlacementPoint(event.x,event.y);if(!position)return InputResult.Ignored;
      this.capturedPointerID=event.pointerId;this.labelPosition=position;
      context.viewport.showConstraintPreview(this.kind,this.references,undefined,position);
      context.viewport.setToolPrompt(this.prompt());return InputResult.Capture;
    }
    const reference = context.viewport.sketchReferenceAt(event.x, event.y, this.spec.picks[this.references.length], this.references[0]); if (!reference) return InputResult.Ignored;
    if (this.references.some((item)=>sameSketchReference(item, reference))) {
      context.viewport.showReferencePreview(reference,this.references[0]);
      return InputResult.Consumed;
    }
    this.capturedPointerID = event.pointerId;
    this.references.push(reference);context.viewport.showReferencePreview(reference,this.references[0]);
    if(this.references.length===this.spec.picks.length&&!this.spec.unit)this.commit(context);
    else {
      if(this.references.length===this.spec.picks.length)context.viewport.showConstraintPreview(this.kind,this.references);
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
      context.viewport.showConstraintPreview(this.kind,this.references,undefined,position);return InputResult.Consumed;
    }
    const reference = context.viewport.sketchReferenceAt(event.x, event.y, this.spec.picks[this.references.length], this.references[0]);
    if (!reference) {
      if (this.references[0]) context.viewport.showReferencePreview(this.references[0]);
      else context.viewport.clearReferencePreview();
      return InputResult.Ignored;
    }
    if (this.references.some((item)=>sameSketchReference(item,reference))) {
      context.viewport.showReferencePreview(this.references[0]);
      return InputResult.Consumed;
    }
    context.viewport.showReferencePreview(reference, this.references[0]);
    return InputResult.Consumed;
  }
  keyDown(event: CadKeyboardEvent, context: ToolContext): InputResult {
    if(event.key==="Escape"){this.cancel(context);return InputResult.Consumed;}
    if(!this.spec.unit||this.references.length!==this.spec.picks.length||!this.labelPosition)return InputResult.Ignored;
    if(event.key==="Backspace"){this.numeric=this.numeric.slice(0,-1);const value=Number(this.numeric);context.viewport.showConstraintPreview(this.kind,this.references,value>0?value:undefined,this.labelPosition);context.viewport.setToolPrompt(`${this.prompt()}：${this.numeric}`);return InputResult.Consumed;}
    if(event.key==="Enter"){const value=Number(this.numeric);if(Number.isFinite(value)&&value>0)this.commit(context,value);return InputResult.Consumed;}
    if(/^[0-9.]$/.test(event.key)&&!(event.key==="."&&this.numeric.includes("."))){this.numeric+=event.key;const value=Number(this.numeric);context.viewport.showConstraintPreview(this.kind,this.references,value>0?value:undefined,this.labelPosition);context.viewport.setToolPrompt(`${this.prompt()}：${this.numeric}`);return InputResult.Consumed;}
    return InputResult.Ignored;
  }
  deactivate(context: ToolContext): void { this.cancel(context); }
  cancel(context: ToolContext): void {
    this.capturedPointerID = undefined;
    this.references = [];
    this.numeric="";
    this.labelPosition=undefined;
    context.viewport.clearReferencePreview();
    context.viewport.setToolPrompt(this.prompt());
  }
  private commit(context:ToolContext,value?:number):void {context.viewport.clearReferencePreview();context.viewport.commitSketchOperations([{type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:this.kind,references:this.references,...(value===undefined?{}:{value,unit:this.spec.unit}),...(this.labelPosition?{labelPosition:{x:this.labelPosition[0],y:this.labelPosition[1]}}:{})}}]);this.references=[];this.numeric="";this.labelPosition=undefined;context.viewport.finishToolUse();context.viewport.setToolPrompt(this.prompt());}
}

export class LinearDimensionSketchTool implements CadTool {
  readonly id="sketch.dimension.linear";
  private references:SketchGeometryRef[]=[];
  private kind?:"DISTANCE"|"LENGTH";
  private labelPosition?:Vec2;
  private numeric="";
  private capturedPointerID?:number;
  activate(context:ToolContext):void {context.viewport.setToolPrompt("线性尺寸：选择直线或第一个点");}
  private referencesComplete():boolean {return this.kind==="LENGTH"?this.references.length===1:this.kind==="DISTANCE"&&this.references.length===2;}
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult {
    if(event.button!==0||this.capturedPointerID!==undefined||!context.viewport.hasActiveSketch())return InputResult.Ignored;
    if(this.referencesComplete()){
      if(this.labelPosition)return InputResult.Ignored;
      const position=context.viewport.sketchPlacementPoint(event.x,event.y);if(!position)return InputResult.Ignored;
      this.capturedPointerID=event.pointerId;this.labelPosition=position;
      context.viewport.showConstraintPreview(this.kind!,this.references,undefined,position);return InputResult.Capture;
    }
    const reference=context.viewport.sketchReferenceAt(event.x,event.y,this.references.length===0?"LINEAR_DIMENSION":"POINT",this.references[0]);
    if(!reference||this.references.some((item)=>sameSketchReference(item,reference)))return InputResult.Ignored;
    this.capturedPointerID=event.pointerId;this.references.push(reference);
    if(this.references.length===1)this.kind=reference.subElement==="WHOLE"?"LENGTH":"DISTANCE";
    context.viewport.showReferencePreview(reference,this.references[0]);
    if(this.referencesComplete())context.viewport.showConstraintPreview(this.kind!,this.references);
    return InputResult.Capture;
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult {
    if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;
    if(this.referencesComplete()){
      if(this.labelPosition)return InputResult.Ignored;
      const position=context.viewport.sketchPlacementPoint(event.x,event.y);if(!position)return InputResult.Ignored;
      context.viewport.showConstraintPreview(this.kind!,this.references,undefined,position);return InputResult.Consumed;
    }
    const reference=context.viewport.sketchReferenceAt(event.x,event.y,this.references.length===0?"LINEAR_DIMENSION":"POINT",this.references[0]);
    if(reference&&!this.references.some((item)=>sameSketchReference(item,reference)))context.viewport.showReferencePreview(reference,this.references[0]);
    else if(this.references[0])context.viewport.showReferencePreview(this.references[0]);
    else context.viewport.clearReferencePreview();
    return InputResult.Consumed;
  }
  pointerUp(event:CadPointerEvent):InputResult {if(event.button!==0||event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.capturedPointerID=undefined;return InputResult.ReleaseCapture;}
  pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult {if(event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult {
    if(event.key==="Escape"){this.cancel(context);return InputResult.Consumed;}
    if(!this.referencesComplete()||!this.labelPosition)return InputResult.Ignored;
    if(event.key==="Backspace")this.numeric=this.numeric.slice(0,-1);
    else if(event.key==="Enter"){const value=Number(this.numeric);if(Number.isFinite(value)&&value>0)this.commit(context,value);return InputResult.Consumed;}
    else if(/^[0-9.]$/.test(event.key)&&!(event.key==="."&&this.numeric.includes(".")))this.numeric+=event.key;
    else return InputResult.Ignored;
    const value=Number(this.numeric);context.viewport.showConstraintPreview(this.kind!,this.references,value>0?value:undefined,this.labelPosition);
    return InputResult.Consumed;
  }
  deactivate(context:ToolContext):void {this.cancel(context);}
  cancel(context:ToolContext):void {this.references=[];this.kind=undefined;this.labelPosition=undefined;this.numeric="";this.capturedPointerID=undefined;context.viewport.clearReferencePreview();}
  private commit(context:ToolContext,value:number):void {
    context.viewport.clearReferencePreview();context.viewport.commitSketchOperations([{type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:this.kind!,references:this.references,value,unit:"mm",labelPosition:{x:this.labelPosition![0],y:this.labelPosition![1]}}}]);
    this.references=[];this.kind=undefined;this.labelPosition=undefined;this.numeric="";context.viewport.finishToolUse();
  }
}
