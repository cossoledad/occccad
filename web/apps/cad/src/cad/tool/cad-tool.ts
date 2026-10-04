import type { SketchCommandState, SketchCommitIntent, SketchCommitResult } from "./sketch-command-session";
import { dimensionDefinitionOperation, dimensionDisplayValue, SketchDimensionCommitSession } from "../sketch/sketch-dimension-editor";
import { formatDimensionValue } from "../sketch/dimension-value-format";
import type { DisplayLengthUnit } from "../../state/ui-preferences";
import { selectionModeForTool } from "../interaction/selection-mode";
import type { CadKeyboardEvent, CadPointerEvent } from "../input/input-types";
import { InputResult,SelectionInputResult,type SelectionInputSource } from "../input/input-types";
import type { AssemblyGeometryRef, SelectionItem, SketchGeometryRef, SketchOperation, SketchEntity, SketchConstraint, Vec2 } from "../../types";
import { assemblyGeometryRef } from "../assembly/assembly-reference";
export { assemblyGeometryRef } from "../assembly/assembly-reference";
import { preselectedSketchReference, type SketchReferencePickKind } from "../interaction/sketch-reference-pick";
import { constraintDefinition, type ConstraintKind } from "../sketch/sketch-constraint-definition";
import { sketchEntityPoint, splineEditablePoints, splineEditablePointIDs, sampleSketchEntity, sampleInterpolatingSpline } from "../sketch/sketch-geometry";
import { SKETCH_INPUT_POLICY, type SketchReferenceGeometry } from "../sketch/sketch-input-policy";
import { copySketchSelection, pasteSketchClipboard, localSketchSelection } from "./sketch-edit-tool";
import { randomUUID } from "../../utils/random-uuid";

import {sketchCommitResultUnknown,type SketchCommandAction} from "./sketch-command-session";

export type ToolViewportPort = {
  sketchPoint(x: number, y: number): Vec2 | null;
  sketchSnapReference(): SketchGeometryRef | undefined;
  snapSketchEditPoint?(point: Vec2, excludedEntityId: string): Vec2;
  sketchPlacementPoint(x: number, y: number): Vec2 | null;
  showPolylinePreview(points: Vec2[], closed?: boolean): void;
  showSplineControlPreview?(points: readonly Vec2[], mode: "FIT" | "CONTROL"): void;
  showPointPreview(point: Vec2): void;
  showReferenceDimensions(geometry: readonly SketchReferenceGeometry[]): void;
  clearToolPreview(): void;
  commitSketchOperations(operations: SketchOperation[], intent?: SketchCommitIntent): Promise<SketchCommitResult> | void;
  previewSketchOperations?(operations:SketchOperation[],signal?:AbortSignal):Promise<{featureId:string;versionId:string;entities:SketchEntity[]}>;
  currentSketchReferenceEntities?():readonly SketchEntity[];
  showSketchEditCandidate?(candidate:import("../sketch/sketch-edit-preview").SketchEditCandidatePreview):void;
  setSketchCommandState?(state: SketchCommandState | undefined): void;
  isFeatureSelectionActive?(): boolean;
  sketchEntityAt?(x: number, y: number): SelectionItem | null;
  hasActiveSketch(): boolean;
  sketchReferenceAt(x: number, y: number, kind: SketchReferencePickKind, retained?: SketchGeometryRef,allowed?:(reference:SketchGeometryRef)=>boolean): SketchGeometryRef | null;
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
  clearReferenceHover?():void;
  setToolPrompt(prompt: string): void;
  finishToolUse(exit?: boolean): void;
  selectionAt(x: number, y: number): SelectionItem | null;
  commitExternalProjection(selection: SelectionItem & {kind:"edge"|"vertex";topologyId:number}): Promise<SketchCommitResult>|void;
  currentSketchDeletableEntities?(): readonly SketchEntity[];
  currentSelections?(): readonly SelectionItem[];
  currentSelectionSource?():SelectionInputSource;
  currentLengthUnit?():DisplayLengthUnit;
  projectSketchPoint?(point:Vec2):Vec2|undefined;
  consumeActivationSelection?():void;
  currentSketchEntities?(): readonly SketchEntity[];
  currentSketchConstraints?(): readonly SketchConstraint[];
  currentSketchIdentity?(): {documentId:string;sketchId:string;versionId:string;occurrencePath?:string}|undefined;
  showSketchEntityPreview?(entities:readonly SketchEntity[]):void;
  retainSelections(selections: SelectionItem[]): void;
  requestAssemblyConstraint(kind: AssemblyConstraintToolKind, references: AssemblyGeometryRef[]): void;
  showSketchManipulator?(origin:Vec2,mode:"translate"|"rotate"|"both",changed:(value:{translation:Vec2;angle:number;origin:Vec2;finish?:boolean})=>void):void;
  clearSketchManipulator?():void;
  sketchManipulatorPointerDown?(pointerId:number,x:number,y:number):boolean;
  sketchManipulatorPointerMove?(pointerId:number,x:number,y:number):boolean;
  sketchManipulatorPointerUp?(pointerId:number,commit:boolean):boolean;
  moveManipulatorPointerDown(pointerId: number, x: number, y: number): boolean;
  moveManipulatorPointerMove(pointerId: number, x: number, y: number): boolean;
  moveManipulatorPointerUp(pointerId: number, commit: boolean): boolean;
};

export type ToolContext = { viewport: ToolViewportPort };
const sameSketchReference = (left: SketchGeometryRef, right: SketchGeometryRef): boolean =>
  left.target === right.target && left.entityId === right.entityId && left.subElement === right.subElement &&
  (left.controlPointId && right.controlPointId ? left.controlPointId === right.controlPointId : left.controlPointIndex === right.controlPointIndex);
export interface CadTool {
  readonly id: string;
  activate?(context: ToolContext): void;
  commandAction?(action:SketchCommandAction,context:ToolContext):void;
  selectionInput?(selections: readonly SelectionItem[], context: ToolContext,source?:SelectionInputSource): SelectionInputResult;
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
  private deletionPending=false;
  private deletionGeneration=0;
  private dimensionPointer?: { id: number; x: number; y: number; moved: boolean; lastTarget?:Vec2 };
  private lastDimensionClick?: { x: number; y: number; at: number };
  private sketchPointPointer?:{id:number;reference:SketchGeometryRef;point:Vec2;baseline:Vec2;pointerBaseline:Vec2;x:number;y:number;moved:boolean;scopeKey:string|undefined};
  private sketchGestureScope(context:ToolContext):string|undefined {const scope=context.viewport.currentSketchIdentity?.();return scope?JSON.stringify(scope):undefined;}
  private updateSketchPointGesture(event:CadPointerEvent,context:ToolContext):boolean {
    const gesture=this.sketchPointPointer;if(!gesture)return false;
    if(gesture.scopeKey!==this.sketchGestureScope(context)){this.sketchPointPointer=undefined;context.viewport.clearToolPreview();context.viewport.setToolPrompt("草图编辑上下文已变化；拖动已取消");return false;}
    if(Math.hypot(event.x-gesture.x,event.y-gesture.y)>=3)gesture.moved=true;
    const current=context.viewport.sketchPlacementPoint(event.x,event.y);
    if(!current||!current.every(Number.isFinite)){this.sketchPointPointer=undefined;context.viewport.clearToolPreview();context.viewport.setToolPrompt("无法定位当前拖动目标；拖动已取消");return false;}
    // Every target is measured from the frozen gesture baseline. Accepted or
    // constrained coordinates never become a second accumulated displacement.
    gesture.point=[gesture.baseline[0]+current[0]-gesture.pointerBaseline[0],gesture.baseline[1]+current[1]-gesture.pointerBaseline[1]];
    if(gesture.moved) {
      if(gesture.reference.subElement==="CONTROL"&&!event.state.modifiers?.alt)
        gesture.point=context.viewport.snapSketchEditPoint?.(gesture.point,gesture.reference.entityId!)??gesture.point;
      if(gesture.reference.subElement==="CONTROL"&&event.state.modifiers?.alt)context.viewport.clearToolPreview();
      context.viewport.showPointPreview(gesture.point);
    }
    return true;
  }
  activate(context: ToolContext): void { context.viewport.clearReferencePreview();context.viewport.clearToolPreview();context.viewport.setToolPrompt("选择：选择草图元素，或从工具栏启动创建命令"); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (context.viewport.isFeatureSelectionActive?.()) return InputResult.Ignored;
    if (event.button !== 0 || event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    if (context.viewport.beginDimensionDrag(event.x, event.y)) {
      this.dimensionPointer = { id: event.pointerId, x: event.x, y: event.y, moved: false };return InputResult.Capture;
    }
    if(event.state.modifiers?.ctrl||event.state.modifiers?.meta)return InputResult.Ignored;
    const reference=context.viewport.sketchReferenceAt(event.x,event.y,"EDIT_POINT"),pointerBaseline=context.viewport.sketchPlacementPoint(event.x,event.y);
    const entity=context.viewport.currentSketchEntities?.().find(entity=>entity.id===reference?.entityId);
    const primary=context.viewport.sketchEntityAt?.(event.x,event.y);
    if(entity?.role==="CONSTRUCTION"&&primary?.entityId!==entity.id&&context.viewport.currentSketchEntities?.().some(candidate=>candidate.id===primary?.entityId&&candidate.role!=="CONSTRUCTION"))return InputResult.Ignored;
    if(reference?.target==="ENTITY"&&entity&&pointerBaseline){
      const index=reference.controlPointId?splineEditablePointIDs(entity).indexOf(reference.controlPointId):reference.controlPointIndex;
      const original=reference.subElement==="CENTER"?entity.center:reference.subElement==="POINT"?entity.point:
        reference.subElement==="CONTROL"&&index!==undefined?splineEditablePoints(entity)[index]:
        ["START","END"].includes(reference.subElement)?(()=>{const point=sketchEntityPoint(entity,reference.subElement as "START"|"END");return point?{x:point[0],y:point[1]}:undefined;})():undefined;
      if(original){const baseline:Vec2=[original.x,original.y];this.sketchPointPointer={id:event.pointerId,reference,point:baseline,baseline,pointerBaseline:[...pointerBaseline],x:event.x,y:event.y,moved:false,scopeKey:this.sketchGestureScope(context)};
        const selection=context.viewport.selectionAt(event.x,event.y);if(selection)context.viewport.retainSelections([selection]);return InputResult.Capture;}
    }
    return InputResult.Ignored;
  }
  pointerMove(event: CadPointerEvent, context: ToolContext): InputResult {
    if(event.pointerId===this.sketchPointPointer?.id){this.updateSketchPointGesture(event,context);return InputResult.Consumed;}
    if (event.pointerId !== this.dimensionPointer?.id) {
      if (context.viewport.hasActiveSketch() && !event.state.buttons.left && !event.state.buttons.middle && !event.state.buttons.right) {
        const control = context.viewport.sketchReferenceAt?.(event.x,event.y,"EDIT_POINT",undefined,ref=>ref.subElement==="CONTROL");
        if (control) context.viewport.showReferencePreview?.(control);
        else context.viewport.clearReferencePreview?.();
      }
      return InputResult.Ignored;
    }
    if (Math.hypot(event.x - this.dimensionPointer.x, event.y - this.dimensionPointer.y) >= 3) this.dimensionPointer.moved = true;
    context.viewport.updateDimensionDrag(event.x, event.y);this.dimensionPointer.lastTarget=[event.x,event.y];
    return InputResult.Consumed;
  }
  pointerUp(event: CadPointerEvent, context: ToolContext): InputResult {
    if(event.pointerId===this.sketchPointPointer?.id&&event.button===0){
      if(!this.updateSketchPointGesture(event,context))return InputResult.ReleaseCapture;
      const drag=this.sketchPointPointer!;this.sketchPointPointer=undefined;context.viewport.clearToolPreview();
      if(drag.moved)context.viewport.commitSketchOperations([{type:"UPDATE_ENTITY_POINT",entityId:drag.reference.entityId!,subElement:drag.reference.subElement as "POINT"|"CENTER"|"CONTROL"|"START"|"END",
        controlPointIndex:drag.reference.controlPointIndex,controlPointId:drag.reference.controlPointId,point:{x:drag.point[0],y:drag.point[1]}}]);return InputResult.ReleaseCapture;
    }
    if (event.pointerId !== this.dimensionPointer?.id || event.button !== 0) return InputResult.Ignored;
    const gesture = this.dimensionPointer; this.dimensionPointer = undefined;
    if(gesture.moved&&(!gesture.lastTarget||gesture.lastTarget[0]!==event.x||gesture.lastTarget[1]!==event.y))context.viewport.updateDimensionDrag(event.x,event.y);context.viewport.finishDimensionDrag();
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
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult {
    if(context.viewport.isFeatureSelectionActive?.())return InputResult.Ignored;
    if(event.editableTarget||!context.viewport.hasActiveSketch())return InputResult.Ignored;
    if(event.key==="Escape"&&(this.sketchPointPointer||this.dimensionPointer)){this.cancel(context);return InputResult.Consumed;}
    if((event.state?.modifiers.ctrl||event.state?.modifiers.meta)&&event.key.toLowerCase()==="c")return copySketchSelection(context)?InputResult.Consumed:InputResult.Ignored;
    if((event.state?.modifiers.ctrl||event.state?.modifiers.meta)&&event.key.toLowerCase()==="x"){
      const entities=localSketchSelection(context.viewport.currentSelections?.()??[],context);if(!entities.length||!copySketchSelection(context))return InputResult.Ignored;
      context.viewport.commitSketchOperations([{type:"DELETE_ENTITIES",entityIds:entities.map(entity=>entity.id)}]);context.viewport.setToolPrompt("已请求剪切（控制面确认后删除）；Ctrl+V 粘贴独立几何与内部逻辑关系");return InputResult.Consumed;
    }
    if((event.state?.modifiers.ctrl||event.state?.modifiers.meta)&&event.key.toLowerCase()==="v")return pasteSketchClipboard(context)?InputResult.Consumed:InputResult.Ignored;
    if(event.key==="Delete"||event.key==="Backspace"){
      if(event.repeat||this.deletionPending)return InputResult.Consumed;
      const selections=context.viewport.currentSelections?.()??[],entities=localSketchSelection(selections,context,true),scope=context.viewport.currentSketchIdentity?.();
      const constraintsByID=new Map((context.viewport.currentSketchConstraints?.()??[]).map(constraint=>[constraint.id,constraint]));
      const selectedConstraints=selections.filter(selection=>selection.kind==="sketch-constraint"&&constraintsByID.has(selection.constraintId)&&(!scope||selection.featureId===scope.sketchId)&&
        (!scope||!selection.ownerDocumentId||selection.ownerDocumentId===scope.documentId)&&(!scope?.occurrencePath||!selection.occurrencePath||selection.occurrencePath===scope.occurrencePath))
        .filter((selection):selection is Extract<SelectionItem,{kind:"sketch-constraint"}>=>selection.kind==="sketch-constraint");
      if(selectedConstraints.some(selection=>["MIRROR","SAME_SUPPORT"].includes(constraintsByID.get(selection.constraintId)!.kind))){context.viewport.setToolPrompt("关联镜像/支撑关系必须通过受控几何操作解除，不能孤立删除内部关系");return InputResult.Consumed;}
      if(!entities.length&&!selectedConstraints.length)return InputResult.Ignored;
      const operations:SketchOperation[]=[...new Set(selectedConstraints.map(selection=>selection.constraintId))].map(constraintId=>({type:"DELETE_CONSTRAINT",constraintId}));
      if(entities.length)operations.push({type:"DELETE_ENTITIES",entityIds:entities.map(entity=>entity.id)});
      this.deletionPending=true;const generation=++this.deletionGeneration;
      const owner=()=>{const scope=context.viewport.currentSketchIdentity?.();return JSON.stringify([scope?.documentId,scope?.sketchId,scope?.occurrencePath]);};const submittedOwner=owner();
      const current=()=>generation===this.deletionGeneration&&owner()===submittedOwner;
      try {
        const result=context.viewport.commitSketchOperations(operations);
        if(result&&typeof result.then==="function")void result.then(()=>{if(current())context.viewport.setToolPrompt("删除已完成");},error=>{if(current())context.viewport.setToolPrompt(`删除未完成：${error instanceof Error?error.message:String(error)}`);}).finally(()=>{this.deletionPending=false;});
        else this.deletionPending=false;
      } catch(error){this.deletionPending=false;context.viewport.setToolPrompt(`删除未完成：${error instanceof Error?error.message:String(error)}`);}
      return InputResult.Consumed;
    }
    return InputResult.Ignored;
  }
  cancel(context: ToolContext): void { this.deletionGeneration++;this.dimensionPointer = undefined;this.sketchPointPointer=undefined; this.lastDimensionClick = undefined;context.viewport.clearToolPreview(); context.viewport.cancelDimensionDrag(); }
}

export class ProjectExternalGeometrySketchTool implements CadTool {
  readonly id = "sketch.project";
  private capturedPointerID?: number;
  private draft?: SelectionItem & {kind:"edge"|"vertex";topologyId:number};
  private pending=false;
  private generation=0;
  activate(context: ToolContext): void { context.viewport.setToolPrompt("投影：选择已有实体的一条边或一个顶点；Esc 取消"); }
  private submit(context:ToolContext):void {
    if(!this.draft||this.pending)return;
    const generation=++this.generation;this.pending=true;
    const owner=()=>{const scope=context.viewport.currentSketchIdentity?.();return JSON.stringify([scope?.documentId,scope?.sketchId,scope?.occurrencePath]);};const initialOwner=owner();
    const accepted=()=>{if(generation!==this.generation)return;if(owner()!==initialOwner){this.cancel(context);return;}this.pending=false;this.draft=undefined;context.viewport.finishToolUse();};
    const failed=(error:unknown)=>{if(generation!==this.generation)return;if(owner()!==initialOwner){this.cancel(context);return;}this.pending=sketchCommitResultUnknown(error);context.viewport.setToolPrompt(`${this.pending?"投影结果待确认；请查询原提交结果或取消":"投影失败；保留已选引用，Enter 重试或重新选择"}：${error instanceof Error?error.message:String(error)}`);};
    try{const result=context.viewport.commitExternalProjection(this.draft);if(result&&typeof result.then==="function")void Promise.resolve(result).then(accepted,failed);else accepted();}catch(error){failed(error);}
  }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if(this.pending)return InputResult.Consumed;
    if (event.button !== 0 || this.capturedPointerID !== undefined || event.state.buttons.middle || event.state.buttons.right)return InputResult.Ignored;
    this.capturedPointerID = event.pointerId;
    const selection = context.viewport.selectionAt(event.x, event.y);
    if (!selection || (selection.kind !== "edge" && selection.kind !== "vertex") || !selection.geometryKey || !selection.versionId || !selection.topologyId) {
      context.viewport.setToolPrompt("投影只接受当前 Part 中已有实体的边或顶点");return InputResult.Capture;
    }
    this.draft={...selection} as SelectionItem & {kind:"edge"|"vertex";topologyId:number};
    context.viewport.retainSelections([selection]);this.submit(context);return InputResult.Capture;
  }
  pointerUp(event: CadPointerEvent): InputResult {
    if (event.pointerId !== this.capturedPointerID || event.button !== 0) return InputResult.Ignored;
    this.capturedPointerID = undefined; return InputResult.ReleaseCapture;
  }
  pointerCancel(event: CadPointerEvent): InputResult {
    if (event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined; return InputResult.Consumed;
  }
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult {
    if(event.key==="Escape"){this.cancel(context);return InputResult.Ignored;}
    if(event.key==="Enter"&&this.draft){this.submit(context);return InputResult.Consumed;}return InputResult.Ignored;
  }
  deactivate(context:ToolContext):void{this.cancel(context);}
  cancel(context: ToolContext): void {this.generation++;this.pending=false;this.draft=undefined;this.capturedPointerID = undefined; context.viewport.setToolPrompt(""); }
}

export type AssemblyConstraintToolKind = "fix"|"rigid"|"fix_together"|"contact"|"coincident"|"concentric"|"angle"|"parallel"|"perpendicular"|"distance";

export class AssemblyMoveTool implements CadTool {
  readonly id = "assembly.move";
  activate(context: ToolContext): void { context.viewport.setToolPrompt("移动组件：选择一个 Instance，然后使用三维操纵器"); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    if (context.viewport.moveManipulatorPointerDown(event.pointerId, event.x, event.y)) return InputResult.Capture;
    const selection = context.viewport.selectionAt(event.x, event.y);
    if (!selection) context.viewport.retainSelections([]);
    else if (selection.instanceId) context.viewport.retainSelections([selection]);
    return InputResult.Consumed;
  }
  pointerMove(event: CadPointerEvent, context: ToolContext): InputResult {
    return context.viewport.moveManipulatorPointerMove(event.pointerId, event.x, event.y) ? InputResult.Consumed : InputResult.Ignored;
  }
  pointerUp(event: CadPointerEvent, context: ToolContext): InputResult {
    context.viewport.moveManipulatorPointerMove(event.pointerId,event.x,event.y);
    return context.viewport.moveManipulatorPointerUp(event.pointerId, true) ? InputResult.ReleaseCapture : InputResult.Ignored;
  }
  pointerCancel(event: CadPointerEvent, context: ToolContext): InputResult {
    return context.viewport.moveManipulatorPointerUp(event.pointerId, false) ? InputResult.Consumed : InputResult.Ignored;
  }
  cancel(context: ToolContext): void { context.viewport.moveManipulatorPointerUp(-1, false); }
}

export class AssemblyConstraintTool implements CadTool {
  readonly id: string;
  readonly kind: AssemblyConstraintToolKind;
  private first?: { selection: SelectionItem; reference: AssemblyGeometryRef };
  private capturedPointerID?: number;
  constructor(kind: AssemblyConstraintToolKind) { this.kind = kind; this.id = `assembly.${kind}`; }
  activate(context: ToolContext): void {
    context.viewport.setToolPrompt(this.kind === "fix" ? "固定：选择一个实例" : this.kind === "fix_together" || this.kind === "rigid" ? "固联组：选择两个初始组件，随后可添加组件或已有组" : "装配约束：依次选择两个元素；精确类型由服务器解析");
  }
  selectionInput(selections: readonly SelectionItem[], context: ToolContext): SelectionInputResult {
    let result=SelectionInputResult.Unhandled;
    for (const candidate of selections) {
      const selection = selectionModeForTool(this.id).project(candidate);
      const reference = selection && assemblyGeometryRef(selection);
      if (!selection || !reference || (["fix", "rigid", "fix_together"].includes(this.kind) && reference.kind !== "BODY")) continue;
      if (this.kind === "fix") {
        context.viewport.retainSelections([selection]);
        context.viewport.requestAssemblyConstraint(this.kind, [reference]);
        context.viewport.finishToolUse();
        return SelectionInputResult.Accepted;
      }
      if (!this.first) {
        result=SelectionInputResult.Accepted;this.first = { selection, reference };
        context.viewport.retainSelections([selection]);
        context.viewport.setToolPrompt("装配约束：选择另一个实例上的元素；Esc 取消");
        continue;
      }
      const occurrence = (ref: AssemblyGeometryRef) => ref.instancePath?.canonical ?? ref.instanceId;
      if (occurrence(this.first.reference) === occurrence(reference)) {if(result!==SelectionInputResult.Accepted)result=SelectionInputResult.Rejected;continue;}
      const first = this.first; this.first = undefined;
      context.viewport.retainSelections([first.selection, selection]);
      context.viewport.requestAssemblyConstraint(this.kind, [first.reference, reference]);
      context.viewport.finishToolUse();
      return SelectionInputResult.Accepted;
    }
    return result;
  }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || this.capturedPointerID !== undefined || event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    const selection = context.viewport.selectionAt(event.x, event.y);
    if (!selection) return InputResult.Consumed;
    this.capturedPointerID = event.pointerId;
    this.selectionInput([selection], context);
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

// All local creation tools share role toggling and an explicit end action.
// Role is interaction state until a confirmed operation is submitted.
abstract class SketchCreationTool implements CadTool {
  abstract readonly id: string;
  protected construction = false;
  protected creationPending=false;
  private creationGeneration=0;
  protected invalidateCreationSubmission():void{this.creationGeneration++;this.creationPending=false;}
  protected submitCreation(context:ToolContext,invoke:(submission:ToolContext)=>void,accepted:()=>void,exit=false):void{
    if(this.creationPending)return;
    const generation=++this.creationGeneration;this.creationPending=true;
    const owner=()=>{const scope=context.viewport.currentSketchIdentity?.();return JSON.stringify([scope?.documentId,scope?.sketchId,scope?.occurrencePath]);};const submissionOwner=owner();
    const promises:Promise<SketchCommitResult>[]=[];
    const submission:ToolContext={...context,viewport:{...context.viewport,
      commitSketchOperations:(operations,intent)=>{const result=context.viewport.commitSketchOperations(operations,intent);if(result&&typeof result.then==="function")promises.push(Promise.resolve(result));return result;},
      finishToolUse:()=>{},
    }};
    const success=()=>{if(generation!==this.creationGeneration)return;if(owner()!==submissionOwner){this.cancel(context);return;}this.creationPending=false;accepted();context.viewport.finishToolUse(exit);};
    const failure=(error:unknown)=>{if(generation!==this.creationGeneration)return;if(owner()!==submissionOwner){this.cancel(context);return;}this.creationPending=sketchCommitResultUnknown(error);context.viewport.clearToolPreview();context.viewport.setToolPrompt(`${this.creationPending?"创建结果待确认；请查询原提交结果或取消":"创建失败；保留当前输入，可修改后重试"}：${error instanceof Error?error.message:String(error)}`);};
    try{invoke(submission);if(promises.length)void Promise.all(promises).then(success,failure);else success();}catch(error){failure(error);}
  }

  private automaticConstraints = true;
  protected capturedSnap(event:CadPointerEvent,context:ToolContext):SketchGeometryRef|undefined {
    return this.automaticConstraints&&!event.state.modifiers?.alt?context.viewport.sketchSnapReference():undefined;
  }
  protected get role(): "PROFILE" | "CONSTRUCTION" { return this.construction ? "CONSTRUCTION" : "PROFILE"; }
  protected creationKey(event: CadKeyboardEvent, context: ToolContext): InputResult {
    if (event.editableTarget || event.state?.modifiers.ctrl || event.state?.modifiers.meta || event.state?.modifiers.alt) return InputResult.Ignored;
    if (event.key.toLowerCase() === "a") {this.automaticConstraints=!this.automaticConstraints;context.viewport.setToolPrompt(`自动连接约束：${this.automaticConstraints?"开启":"关闭"}；A 切换，Alt 临时关闭，临时吸附不受影响`);return InputResult.Consumed;}
    if (event.key.toLowerCase() !== "c") return InputResult.Ignored;
    this.construction = !this.construction;
    context.viewport.setToolPrompt(`${this.construction ? "辅助几何" : "标准几何"}；C 切换，右键结束`);
    return InputResult.Consumed;
  }
  protected endOnRightClick(event: CadPointerEvent, context: ToolContext): boolean {
    if (event.button !== 2) return false;
    this.cancel(context); context.viewport.finishToolUse(true); return true;
  }
  abstract cancel(context: ToolContext): void;
}

abstract class TwoClickSketchTool extends SketchCreationTool {
  protected first?: Vec2;
  protected firstSnap?: SketchGeometryRef;
  private capturedPointerID?: number;
  private cursor?: Vec2;
  private cursorSnap?:SketchGeometryRef;
  private fields: [string, string] = ["", ""];
  private field = 0;
  private replacing = false;
  abstract preview(first: Vec2, second: Vec2, context: ToolContext): void;
  abstract commit(first: Vec2, second: Vec2, context: ToolContext,
    snaps: { first?: SketchGeometryRef; second?: SketchGeometryRef }): void;
  abstract readonly firstPrompt: string;
  abstract readonly secondPrompt: string;

  selectionInput(selections:readonly SelectionItem[],context:ToolContext):SelectionInputResult {
    if(this.first)return SelectionInputResult.Rejected;
    const points=localSketchSelection(selections,context).filter(entity=>entity.kind==="POINT"&&entity.point);
    const source=points[0];if(!source?.point)return SelectionInputResult.Unhandled;
    this.first=[source.point.x,source.point.y];this.firstSnap={target:"ENTITY",entityId:source.id,subElement:"POINT"};
    const target=points[1];this.cursor=target?.point?[target.point.x,target.point.y]:this.first;
    this.cursorSnap=target?{target:"ENTITY",entityId:target.id,subElement:"POINT"}:undefined;
    this.preview(this.first,this.cursor,context);this.updatePrompt(context);return SelectionInputResult.Accepted;
  }
  activate(context: ToolContext): void { context.viewport.setToolPrompt(`${this.firstPrompt}；C 辅助几何，A 自动连接，Alt 临时禁用连接，右键结束`); }

  private resolvedPoint(point: Vec2): Vec2 | undefined {
    if (!this.first) return point;
    const dx = point[0] - this.first[0], dy = point[1] - this.first[1];
    const first = this.fields[0] === "" ? undefined : Number(this.fields[0]);
    const second = this.fields[1] === "" ? undefined : Number(this.fields[1]);
    if ((first !== undefined && !Number.isFinite(first)) || (second !== undefined && !Number.isFinite(second))) return undefined;
    if (this.id.startsWith("sketch.rectangle")) return [this.first[0] + (first ?? dx), this.first[1] + (second ?? dy)];
    const length = first ?? Math.hypot(dx, dy), angle = second === undefined ? Math.atan2(dy, dx) : second * Math.PI / 180;
    if (length < SKETCH_INPUT_POLICY.minimumGeometryLength) return undefined;
    return [this.first[0] + length * Math.cos(angle), this.first[1] + length * Math.sin(angle)];
  }
  private updatePrompt(context: ToolContext): void {
    const labels = this.id.startsWith("sketch.rectangle") ? ["宽度 mm", "高度 mm"] : [this.id === "sketch.circle" ? "半径 mm" : "长度 mm", "角度°"];
    context.viewport.setToolPrompt(`${this.secondPrompt}；${labels.map((label, index) => `${index === this.field ? "▶" : ""}${label}: ${this.fields[index] || "鼠标"}`).join("，")}；Tab 切换，Enter 确认，C 辅助几何`);
  }
  private confirm(point: Vec2, snap: SketchGeometryRef | undefined, context: ToolContext): boolean {
    const resolved = this.resolvedPoint(point);
    if (!this.first || !resolved || Math.hypot(resolved[0] - this.first[0], resolved[1] - this.first[1]) < SKETCH_INPUT_POLICY.minimumGeometryLength ||
      (this.id.startsWith("sketch.rectangle") && (Math.abs(resolved[0] - this.first[0]) < SKETCH_INPUT_POLICY.minimumGeometryLength || Math.abs(resolved[1] - this.first[1]) < SKETCH_INPUT_POLICY.minimumGeometryLength))) {
      context.viewport.setToolPrompt("不能创建退化几何；输入有效数值或重新选择终点"); return false;
    }
    const first = this.first, firstSnap = this.firstSnap;
    // A numeric target is not the snapped endpoint, so it must not inherit its constraint.
    const secondSnap = this.fields.some(value => value !== "") ? undefined : snap;
    context.viewport.clearToolPreview();this.submitCreation(context,submission=>this.commit(first,resolved,submission,{first:firstSnap,second:secondSnap}),()=>{
      this.first=undefined;this.firstSnap=undefined;this.cursor=undefined;this.cursorSnap=undefined;this.fields=["",""];this.field=0;this.replacing=false;this.activate(context);
    });return true;
  }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if(this.creationPending){if(event.button===2){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}
    if (this.endOnRightClick(event, context)) return InputResult.Consumed;
    if (event.button !== 0 || this.capturedPointerID !== undefined || event.state.buttons.middle || event.state.buttons.right || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y);
    if (!point) return InputResult.Ignored;
    this.capturedPointerID = event.pointerId;
    if (!this.first) {
      this.first = point; this.cursor = point;
      this.firstSnap = this.capturedSnap(event,context);
      this.preview(point, point, context); this.updatePrompt(context);
    } else this.confirm(point, this.capturedSnap(event,context), context);
    return InputResult.Capture;
  }
  pointerUp(event: CadPointerEvent): InputResult {
    if (event.button !== 0 || event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined; return InputResult.ReleaseCapture;
  }
  pointerCancel(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.cancel(context); return InputResult.Consumed;
  }
  pointerMove(event: CadPointerEvent, context: ToolContext): InputResult {
    if(this.creationPending)return InputResult.Consumed;
    if (event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y);
    if (!point) return InputResult.Ignored;
    this.cursor = point;this.cursorSnap=this.capturedSnap(event,context);
    const resolved = this.resolvedPoint(point);
    if (this.first && resolved) this.preview(this.first, resolved, context);
    return InputResult.Consumed;
  }
  keyDown(event: CadKeyboardEvent, context: ToolContext): InputResult {
    if(this.creationPending){if(event.key==="Escape"){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}
    const shared = this.creationKey(event, context); if (shared !== InputResult.Ignored) return shared;
    if (event.editableTarget || event.state?.modifiers.ctrl || event.state?.modifiers.meta || event.state?.modifiers.alt) return InputResult.Ignored;
    if (!this.first) return InputResult.Ignored;
    if (event.key === "Escape") { this.cancel(context); return InputResult.Consumed; }
    if (event.key === "Enter") { this.confirm(this.cursor ?? this.first, this.cursorSnap, context); return InputResult.Consumed; }
    if (event.key === "Tab") { this.field = this.field === 0 ? 1 : 0; this.replacing = true; }
    else if (event.key === "Backspace") { this.fields[this.field] = this.fields[this.field].slice(0, -1); this.replacing = false; }
    else if (/^[0-9.\-+]$/.test(event.key)) { this.fields[this.field] = (this.replacing ? "" : this.fields[this.field]) + event.key; this.replacing = false; }
    else return InputResult.Ignored;
    this.updatePrompt(context);
    const resolved = this.resolvedPoint(this.cursor ?? this.first);
    if (resolved) this.preview(this.first, resolved, context);
    return InputResult.Consumed;
  }
  deactivate(context: ToolContext): void { this.cancel(context); }
  cancel(context: ToolContext): void {
    this.invalidateCreationSubmission();this.capturedPointerID = undefined; this.first = undefined; this.firstSnap = undefined; this.cursor = undefined;this.cursorSnap=undefined;
    this.fields = ["", ""]; this.field = 0; this.replacing = false;
    context.viewport.clearToolPreview(); this.activate(context);
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
    const operations:SketchOperation[]=[{ type: "ADD_ENTITY", entity: { id, kind: "LINE", role: this.role, start: { x: first[0], y: first[1] }, end: { x: second[0], y: second[1] } } }];
    for(const [subElement,target] of [["START",snaps.first],["END",snaps.second]] as const) if(target) operations.push({type:"ADD_CONSTRAINT",
      constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:id,subElement},target]}});
    context.viewport.commitSketchOperations(operations);context.viewport.finishToolUse();

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
    if (!this.construction) {
      context.viewport.commitSketchOperations([{ type: "ADD_RECTANGLE", first: { x: first[0], y: first[1] }, second: { x: second[0], y: second[1] },
        firstReference:snaps.first,secondReference:snaps.second }]);context.viewport.finishToolUse();
      return;
    }
    const operations = polylineOperations([first, [second[0], first[1]], second, [first[0], second[1]]], true, this.role);
    const ids = operations.filter(item => item.type === "ADD_ENTITY").map(item => item.entity.id);
    ids.forEach((id, index) => operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:index % 2 === 0 ? "HORIZONTAL" : "VERTICAL",internal:true,
      references:[{target:"ENTITY",entityId:id,subElement:"WHOLE"}]}}));
    for (const [id, target] of [[ids[0], snaps.first], [ids[2], snaps.second]] as const) if (target) operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",
      references:[{target:"ENTITY",entityId:id,subElement:"START"},target]}});
    context.viewport.commitSketchOperations(operations);context.viewport.finishToolUse();

  }
}

function polylineOperations(points:Vec2[],closed:boolean,role:"PROFILE"|"CONSTRUCTION"="PROFILE"):SketchOperation[] {
  const vertices=closed?[...points,points[0]]:points;
  const operationID=randomUUID();
  const ids=Array.from({length:vertices.length-1},()=>randomUUID());
  const operations:SketchOperation[]=ids.map((id,index)=>({type:"ADD_ENTITY",entity:{id,kind:"LINE",role,createdByOperationId:operationID,
    start:{x:vertices[index][0],y:vertices[index][1]},end:{x:vertices[index+1][0],y:vertices[index+1][1]}}}));
  for(let index=1;index<ids.length;index+=1)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",internal:true,
    references:[{target:"ENTITY",entityId:ids[index-1],subElement:"END"},{target:"ENTITY",entityId:ids[index],subElement:"START"}]}});
  if(closed&&ids.length>1)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",internal:true,
    references:[{target:"ENTITY",entityId:ids.at(-1),subElement:"END"},{target:"ENTITY",entityId:ids[0],subElement:"START"}]}});
  return operations;
}

export function polygonVertices(center:Vec2,edge:Vec2,sides:number,mode:"INSCRIBED"|"CIRCUMSCRIBED"):Vec2[]{
 const input=Math.hypot(edge[0]-center[0],edge[1]-center[1]),half=Math.PI/sides;
 const radius=mode==="INSCRIBED"?input/Math.cos(half):input,angle=Math.atan2(edge[1]-center[1],edge[0]-center[0])-(mode==="INSCRIBED"?half:0);
 return Array.from({length:sides},(_,i)=>[center[0]+radius*Math.cos(angle+i*2*Math.PI/sides),center[1]+radius*Math.sin(angle+i*2*Math.PI/sides)]);
}
export class RegularPolygonSketchTool extends SketchCreationTool {
 readonly id:string;
 private stage:"center"|"radius"|"sides"="center";
 private center?:Vec2;private edge?:Vec2;private centerSnap?:SketchGeometryRef;private pointer?:number;
 private sides="6";private operationId?:string;private error?:string;private phase:SketchCommandState["phase"]="selection";
 constructor(private mode:"INSCRIBED"|"CIRCUMSCRIBED"="INSCRIBED"){super();this.id=mode==="INSCRIBED"?"sketch.polygon":"sketch.polygon.circumscribed";}
 activate(context:ToolContext):void{this.publish(context);}
 private publish(context:ToolContext):void{
  const role=this.stage==="center"?"选择中心":this.stage==="radius"?"选择构造圆半径":"输入边数，Enter 完成";
  context.viewport.setToolPrompt(`${this.mode==="INSCRIBED"?"内接":"外接"}多边形｜${role}`);
  context.viewport.setSketchCommandState?.({toolId:this.id,operation:"多边形",phase:this.phase,role,selectedIds:[],references:this.centerSnap?[this.centerSnap]:[],presentation:"inline",fields:this.stage==="sides"?[{label:"边数",value:this.sides,unit:"scalar"}]:[],options:[],input:this.stage==="sides"?{id:this.operationId!,fieldIndex:0,modelAnchor:this.edge}:undefined,canConfirm:this.stage==="sides"&&!this.creationPending,next:role,error:this.error,preview:"approximate"});
 }
 private preview(context:ToolContext):void{
  if(!this.center||!this.edge)return;const n=Number(this.sides);if(!Number.isInteger(n)||n<3||n>50)return;
  context.viewport.showPolylinePreview(polygonVertices(this.center,this.edge,n,this.mode),true);
  context.viewport.showReferenceDimensions([{kind:"CIRCLE",center:this.center,edge:this.edge}]);
 }
 private finish(context:ToolContext):void{
  if(this.creationPending||this.stage!=="sides"||!this.center||!this.edge)return;
  const sides=Number(this.sides);if(!Number.isInteger(sides)||sides<3||sides>50){this.error="边数必须是 3–50 的整数";this.publish(context);return;}
  const operation:SketchOperation={type:"CREATE_POLYGON",operationId:this.operationId!,point:{x:this.center[0],y:this.center[1]},value:Math.hypot(this.edge[0]-this.center[0],this.edge[1]-this.center[1]),angle:Math.atan2(this.edge[1]-this.center[1],this.edge[0]-this.center[0]),sides,mode:this.mode,role:this.role,firstReference:this.centerSnap};
  this.phase="committing";this.publish(context);
  const owner=this.operationId;
  this.submitCreation(context,submission=>{const result=submission.viewport.commitSketchOperations([operation]);if(result&&typeof result.then==="function")void result.catch(error=>{if(owner!==this.operationId)return;this.phase=sketchCommitResultUnknown(error)?"unknown":"failed";this.error=error instanceof Error?error.message:String(error);this.publish(context);});},()=>{this.stage="center";this.center=undefined;this.edge=undefined;this.centerSnap=undefined;this.operationId=undefined;this.error=undefined;this.phase="selection";this.publish(context);});
 }
 commandAction(action:import("./sketch-command-session").SketchCommandAction,context:ToolContext):void{
  if(action.type==="cancel"){this.cancel(context);context.viewport.finishToolUse(true);return;}if(this.creationPending)return;
  if(action.type==="field"&&action.index===0){this.sides=action.value;this.error=undefined;this.preview(context);this.publish(context);}
  if(action.type==="confirm")this.finish(context);
  if(action.type==="back"){this.stage=this.stage==="sides"?"radius":"center";this.publish(context);}
 }
 pointerDown(event:CadPointerEvent,context:ToolContext):InputResult{
  if(event.button!==0||event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;if(this.creationPending)return InputResult.Consumed;
  const point=context.viewport.sketchPoint(event.x,event.y);if(!point)return InputResult.Consumed;this.pointer=event.pointerId;
  if(this.stage==="center"){this.center=point;this.edge=point;this.centerSnap=this.capturedSnap(event,context);this.operationId=randomUUID();this.stage="radius";this.phase="definition";this.publish(context);}
  else if(this.stage==="radius"){if(Math.hypot(point[0]-this.center![0],point[1]-this.center![1])<SKETCH_INPUT_POLICY.minimumGeometryLength){this.error="半径必须大于零";this.publish(context);}else{this.edge=point;this.stage="sides";this.preview(context);this.publish(context);}}
  else this.finish(context);
  return InputResult.Capture;
 }
 pointerMove(event:CadPointerEvent,context:ToolContext):InputResult{
  if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;if(this.creationPending)return InputResult.Consumed;
  if(this.stage==="center"||this.stage==="radius"){
   const p=context.viewport.sketchPoint(event.x,event.y);
   if(p){if(this.stage==="center")context.viewport.showPointPreview(p);else{this.edge=p;this.preview(context);}}
  }return InputResult.Consumed;
 }
 pointerUp(event:CadPointerEvent):InputResult{if(this.pointer!==event.pointerId)return InputResult.Ignored;this.pointer=undefined;return InputResult.ReleaseCapture;}
 pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult{if(this.pointer!==event.pointerId)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
 keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult{
  if(event.editableTarget)return InputResult.Ignored;if(event.key==="Escape"){this.cancel(context);context.viewport.finishToolUse(true);return InputResult.Consumed;}
  if(this.creationPending||event.repeat)return InputResult.Consumed;
  const shared=this.creationKey(event,context);if(shared!==InputResult.Ignored)return shared;
  if(event.state?.modifiers.ctrl||event.state?.modifiers.meta||event.state?.modifiers.alt)return InputResult.Ignored;
  if(event.key==="Enter"&&this.stage==="sides"){this.finish(context);return InputResult.Consumed;}
  if(this.stage!=="center"&&/^[0-9]$/.test(event.key)){this.sides=this.sides==="6"?event.key:this.sides+event.key;this.preview(context);this.publish(context);return InputResult.Consumed;}return InputResult.Ignored;
 }
 deactivate(context:ToolContext):void{this.cancel(context);context.viewport.setSketchCommandState?.(undefined);}
 cancel(context:ToolContext):void{this.invalidateCreationSubmission();this.stage="center";this.center=undefined;this.edge=undefined;this.centerSnap=undefined;this.pointer=undefined;this.operationId=undefined;this.error=undefined;this.phase="selection";context.viewport.clearToolPreview();this.publish(context);}
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
    const id=randomUUID();const operations:SketchOperation[]=[{type:"ADD_ENTITY",entity:{id,kind:"CIRCLE",role:this.role,
      center:{x:center[0],y:center[1]},radius:Math.hypot(edge[0]-center[0],edge[1]-center[1])}}];
    if(snaps.first)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",
      references:[{target:"ENTITY",entityId:id,subElement:"CENTER"},snaps.first]}});
    context.viewport.commitSketchOperations(operations);context.viewport.finishToolUse();

  }
}

export class ArcSketchTool extends SketchCreationTool {
  readonly id="sketch.arc"; private clockwise=false; private lastHover?:CadPointerEvent; private center?:Vec2; private start?:Vec2; private centerSnap?:SketchGeometryRef; private startSnap?:SketchGeometryRef; private capturedPointerID?:number;
  selectionInput(selections:readonly SelectionItem[],context:ToolContext):SelectionInputResult {
    if(this.center)return SelectionInputResult.Rejected;const source=localSketchSelection(selections,context).find(entity=>entity.kind==="POINT"&&entity.point);
    if(source?.point){this.center=[source.point.x,source.point.y];this.centerSnap={target:"ENTITY",entityId:source.id,subElement:"POINT"};context.viewport.setToolPrompt("圆弧：已使用预选中心，单击起点");return SelectionInputResult.Accepted;}
    return SelectionInputResult.Unhandled;
  }
  activate(context:ToolContext):void { context.viewport.setToolPrompt("圆弧：单击圆心；按住 Ctrl 反向，R 切换默认方向"); }
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult {
    if(this.creationPending){if(event.button===2){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}
    if(this.endOnRightClick(event,context))return InputResult.Consumed;
    if(event.button!==0||this.capturedPointerID!==undefined||!context.viewport.hasActiveSketch())return InputResult.Ignored;
    const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;this.capturedPointerID=event.pointerId;
    if(!this.center){this.center=value;this.centerSnap=this.capturedSnap(event,context);context.viewport.setToolPrompt("圆弧：单击起点");return InputResult.Capture;}
    if(!this.start){if(Math.hypot(value[0]-this.center[0],value[1]-this.center[1])<SKETCH_INPUT_POLICY.minimumGeometryLength)return InputResult.Consumed;this.start=value;this.startSnap=this.capturedSnap(event,context);context.viewport.setToolPrompt("圆弧：单击终点；按住 Ctrl 反向，Esc 取消");return InputResult.Capture;}
    const center=this.center,start=this.start,radius=Math.hypot(start[0]-center[0],start[1]-center[1]);
    let startAngle=Math.atan2(start[1]-center[1],start[0]-center[0]);let endAngle=Math.atan2(value[1]-center[1],value[0]-center[0]);
    const sweep=positiveTurn(endAngle-startAngle);endAngle=startAngle+((this.clockwise!==!!event.state.modifiers?.ctrl)?sweep-2*Math.PI:sweep);
    if(Math.abs(endAngle-startAngle)<1e-9||Math.abs(endAngle-startAngle)>=Math.PI*2-1e-9)return InputResult.Consumed;
    const id=randomUUID(),operations:SketchOperation[]=[{type:"ADD_ENTITY",entity:{id,kind:"ARC",role:this.role,center:{x:center[0],y:center[1]},radius,startAngle,endAngle}}];
    for(const [subElement,target] of [["CENTER",this.centerSnap],["START",this.startSnap],["END",this.capturedSnap(event,context)]] as const)
      if(target)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:id,subElement},target]}});
    context.viewport.clearToolPreview();this.submitCreation(context,submission=>submission.viewport.commitSketchOperations(operations),()=>{this.center=undefined;this.start=undefined;this.centerSnap=undefined;this.startSnap=undefined;context.viewport.setToolPrompt("圆弧：单击圆心");});return InputResult.Capture;
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult {
    if(this.creationPending)return InputResult.Consumed;
    this.lastHover=event;
    if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;if(!this.center)return InputResult.Consumed;
    if(!this.start){context.viewport.showPolylinePreview([this.center,value]);context.viewport.showReferenceDimensions([{kind:"CIRCLE",center:this.center,edge:value}]);return InputResult.Consumed;}
    const radius=Math.hypot(this.start[0]-this.center[0],this.start[1]-this.center[1]);const first=Math.atan2(this.start[1]-this.center[1],this.start[0]-this.center[0]);let last=Math.atan2(value[1]-this.center[1],value[0]-this.center[0]);const sweep=positiveTurn(last-first);last=first+((this.clockwise!==!!event.state.modifiers?.ctrl)?sweep-2*Math.PI:sweep);
    context.viewport.showPolylinePreview(Array.from({length:49},(_,index):Vec2=>{const angle=first+(last-first)*index/48;return[this.center![0]+radius*Math.cos(angle),this.center![1]+radius*Math.sin(angle)];}));
    context.viewport.showReferenceDimensions([{kind:"CIRCLE",center:this.center,edge:this.start}]);return InputResult.Consumed;
  }
  pointerUp(event:CadPointerEvent):InputResult {if(event.button!==0||event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.capturedPointerID=undefined;return InputResult.ReleaseCapture;}
  pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult {if(event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult {
    if(event.key==="Control"&&!event.editableTarget){this.keyUp(event,context);return InputResult.Consumed;}
    if(this.creationPending){if(event.key==="Escape"){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}const shared=this.creationKey(event,context);if(shared!==InputResult.Ignored)return shared;if(event.key.toLowerCase()==="r"&&!event.editableTarget){this.clockwise=!this.clockwise;context.viewport.setToolPrompt(`圆弧：${this.clockwise?"顺时针":"逆时针"}；R 反向，C 辅助几何`);return InputResult.Consumed;}if(event.key!=="Escape"||!this.center)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  keyUp(event:CadKeyboardEvent,context:ToolContext):InputResult {if(event.key!=="Control"||event.editableTarget)return InputResult.Ignored;if(this.lastHover&&!this.creationPending)this.pointerMove({...this.lastHover,state:{...this.lastHover.state,modifiers:event.state.modifiers}},context);return InputResult.Consumed;}
  deactivate(context:ToolContext):void {this.cancel(context);} cancel(context:ToolContext):void {this.invalidateCreationSubmission();this.lastHover=undefined;this.center=undefined;this.start=undefined;this.centerSnap=undefined;this.startSnap=undefined;this.capturedPointerID=undefined;context.viewport.clearToolPreview();context.viewport.setToolPrompt("圆弧：单击圆心");}
}

abstract class MultiPointSketchTool extends SketchCreationTool {
  abstract readonly id:string; protected points:Vec2[]=[]; protected snaps:(SketchGeometryRef|undefined)[]=[]; private capturedPointerID?:number;
  private lastClick?:{x:number;y:number;at:number};
  abstract readonly prompt:string; abstract minimumPoints:number; abstract commit(context:ToolContext):void;
  protected resetCreation():void {}
  protected previewPoints(points:Vec2[],context:ToolContext):void {context.viewport.showPolylinePreview(points);}
  protected addPoint(value:Vec2,snap:SketchGeometryRef|undefined):void {this.points.push(value);this.snaps.push(snap);}
  selectionInput(selections:readonly SelectionItem[],context:ToolContext):SelectionInputResult {
    if(this.points.length)return SelectionInputResult.Rejected;const source=localSketchSelection(selections,context).find(entity=>entity.kind==="POINT"&&entity.point);
    if(source?.point){this.addPoint([source.point.x,source.point.y],{target:"ENTITY",entityId:source.id,subElement:"POINT"});context.viewport.setToolPrompt(`${this.prompt}；已使用预选起点/中心`);return SelectionInputResult.Accepted;}
    return SelectionInputResult.Unhandled;
  }
  activate(context:ToolContext):void {context.viewport.setToolPrompt(this.prompt);}
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult {
    if(this.creationPending){if(event.button===2){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}
    if(event.button===2){if(this.points.length>=this.minimumPoints)this.finish(context,true);else{this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}
    if(event.button!==0||this.capturedPointerID!==undefined||!context.viewport.hasActiveSketch())return InputResult.Ignored;
    const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;this.capturedPointerID=event.pointerId;
    const at=Number(event.originalEvent.timeStamp)||Date.now(),previous=this.lastClick;
    if(previous&&at-previous.at<=450&&Math.hypot(event.x-previous.x,event.y-previous.y)<=6&&this.points.length>=this.minimumPoints){this.lastClick=undefined;this.finish(context);return InputResult.Capture;}
    const last=this.points.at(-1);if(last&&Math.hypot(value[0]-last[0],value[1]-last[1])<SKETCH_INPUT_POLICY.minimumGeometryLength)return InputResult.Capture;
    if(this.points.length>0&&Math.hypot(value[0]-this.points[0][0],value[1]-this.points[0][1])<SKETCH_INPUT_POLICY.minimumGeometryLength&&this.points.length>=this.minimumPoints){this.addPoint(this.points[0],this.snaps[0]);this.lastClick=undefined;this.finish(context);return InputResult.Capture;}
    this.addPoint(value,this.capturedSnap(event,context));this.lastClick={x:event.x,y:event.y,at};this.previewPoints(this.points,context);return InputResult.Capture;
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult {
    if(this.creationPending)return InputResult.Consumed;if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;if(this.points.length>0)this.previewPoints([...this.points,value],context);return InputResult.Consumed;}
  pointerUp(event:CadPointerEvent):InputResult {if(event.button!==0||event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.capturedPointerID=undefined;return InputResult.ReleaseCapture;}
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult {
    if(this.creationPending){if(event.key==="Escape"){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}const shared=this.creationKey(event,context);if(shared!==InputResult.Ignored)return shared;if(event.key==="Enter"&&this.points.length>=this.minimumPoints){this.finish(context);return InputResult.Consumed;}if(event.key==="Escape"&&this.points.length>0){this.cancel(context);return InputResult.Consumed;}return InputResult.Ignored;}
  pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult {if(event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  private finish(context:ToolContext,exit=false):void {context.viewport.clearToolPreview();this.submitCreation(context,submission=>this.commit(submission),()=>{this.points=[];this.snaps=[];this.resetCreation();this.lastClick=undefined;context.viewport.setToolPrompt(this.prompt);},exit);}
  deactivate(context:ToolContext):void {this.cancel(context);} cancel(context:ToolContext):void {this.invalidateCreationSubmission();this.points=[];this.snaps=[];this.resetCreation();this.capturedPointerID=undefined;this.lastClick=undefined;context.viewport.clearToolPreview();context.viewport.setToolPrompt(this.prompt);}
}

type ProfileCreationSegment = {kind:"LINE";start:Vec2;end:Vec2}|{kind:"ARC";start:Vec2;end:Vec2;center:Vec2;radius:number;startAngle:number;endAngle:number};
function segmentEndTangent(segment:ProfileCreationSegment):Vec2{
  if(segment.kind==="LINE"){const dx=segment.end[0]-segment.start[0],dy=segment.end[1]-segment.start[1],length=Math.hypot(dx,dy);return[dx/length,dy/length];}
  const sign=Math.sign(segment.endAngle-segment.startAngle);return[-Math.sin(segment.endAngle)*sign,Math.cos(segment.endAngle)*sign];
}
function tangentProfileArc(start:Vec2,end:Vec2,tangent:Vec2):ProfileCreationSegment|undefined{
  const dx=end[0]-start[0],dy=end[1]-start[1],length=Math.hypot(dx,dy),nx=-tangent[1],ny=tangent[0],projection=dx*nx+dy*ny;
  if(length<SKETCH_INPUT_POLICY.minimumGeometryLength||projection===0)return undefined;
  const signedRadius=(dx*dx+dy*dy)/(2*projection),radius=Math.abs(signedRadius),center:Vec2=[start[0]+nx*signedRadius,start[1]+ny*signedRadius];
  if(!center.every(Number.isFinite)||!Number.isFinite(radius)||radius<SKETCH_INPUT_POLICY.minimumGeometryLength)return undefined;
  const startAngle=Math.atan2(start[1]-center[1],start[0]-center[0]),finish=Math.atan2(end[1]-center[1],end[0]-center[0]),sweep=positiveTurn(finish-startAngle);
  const endAngle=startAngle+(signedRadius>0?sweep:sweep-2*Math.PI);
  if(Math.abs(endAngle-startAngle)<1e-9||Math.abs(endAngle-startAngle)>=Math.PI*2-1e-9)return undefined;
  return{kind:"ARC",start,end,center,radius,startAngle,endAngle};
}

export class PolylineSketchTool extends MultiPointSketchTool {
  readonly id="sketch.polyline";readonly prompt="连续轮廓：依次单击顶点；T 切换直线/相切弧，Enter 完成，首点闭合，右键结束";minimumPoints=2;
  private arcMode=false;
  private segments:ProfileCreationSegment[]=[];
  protected resetCreation():void{this.segments=[];this.arcMode=false;}
  private nextSegment(value:Vec2):ProfileCreationSegment|undefined{
    const start=this.points.at(-1);if(!start)return undefined;
    if(Math.hypot(value[0]-start[0],value[1]-start[1])<SKETCH_INPUT_POLICY.minimumGeometryLength)return undefined;
    if(!this.arcMode)return{kind:"LINE",start,end:value};
    const previous=this.segments.at(-1);return previous?tangentProfileArc(start,value,segmentEndTangent(previous)):undefined;
  }
  protected addPoint(value:Vec2,snap:SketchGeometryRef|undefined):void{
    if(this.points.length){const segment=this.nextSegment(value);if(segment)this.segments.push(segment);}
    super.addPoint(value,snap);
  }
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult{
    if(this.creationPending){if(event.button===2){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}
    if(event.button===0&&this.points.length){const value=context.viewport.sketchPoint(event.x,event.y);
      // Double-clicking the current endpoint ends the contour through the base
      // lifecycle; it does not add a zero-length segment.
      if(value&&Math.hypot(value[0]-this.points.at(-1)![0],value[1]-this.points.at(-1)![1])>=SKETCH_INPUT_POLICY.minimumGeometryLength&&!this.nextSegment(value)){
        context.viewport.setToolPrompt("无法构造有限相切圆弧；改变终点或按 T 切回直线");return InputResult.Consumed;
      }
    }
    return super.pointerDown(event,context);
  }
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult{
    if(this.creationPending){if(event.key==="Escape"){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}
    if(event.key.toLowerCase()==="t"&&!event.editableTarget&&!event.state?.modifiers.ctrl&&!event.state?.modifiers.meta){
      if(!this.segments.length){context.viewport.setToolPrompt("先创建一条直线，再按 T 创建端部相切圆弧");return InputResult.Consumed;}
      this.arcMode=!this.arcMode;context.viewport.setToolPrompt(`连续轮廓：${this.arcMode?"相切圆弧":"直线"}；T 切换，Enter 完成`);return InputResult.Consumed;
    }
    return super.keyDown(event,context);
  }
  private previewSegments(context:ToolContext,next?:ProfileCreationSegment):void{
    const segments=next?[...this.segments,next]:this.segments,points:Vec2[]=[];
    for(const segment of segments){if(!points.length)points.push(segment.start);
      if(segment.kind==="LINE")points.push(segment.end);
      else for(let index=1;index<=32;index++){const angle=segment.startAngle+(segment.endAngle-segment.startAngle)*index/32;points.push([segment.center[0]+segment.radius*Math.cos(angle),segment.center[1]+segment.radius*Math.sin(angle)]);}}
    if(points.length)context.viewport.showPolylinePreview(points);
    if(next)context.viewport.showReferenceDimensions(next.kind==="LINE"?[{kind:"LINE",start:next.start,end:next.end}]:[{kind:"CIRCLE",center:next.center,edge:next.end}]);
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult{
    if(this.creationPending)return InputResult.Consumed;
    if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;
    const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;
    this.previewSegments(context,this.nextSegment(value));return InputResult.Consumed;
  }
  commit(context:ToolContext):void{
    const closed=this.points.at(-1)===this.points[0],ids=this.segments.map(()=>randomUUID()),operationID=randomUUID();
    const operations:SketchOperation[]=this.segments.map((segment,index)=>({type:"ADD_ENTITY",entity:segment.kind==="LINE"?
      {id:ids[index],kind:"LINE",role:this.role,createdByOperationId:operationID,start:{x:segment.start[0],y:segment.start[1]},end:{x:segment.end[0],y:segment.end[1]}}:
      {id:ids[index],kind:"ARC",role:this.role,createdByOperationId:operationID,center:{x:segment.center[0],y:segment.center[1]},radius:segment.radius,startAngle:segment.startAngle,endAngle:segment.endAngle}}));
    for(let index=1;index<ids.length;index++){
      operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",internal:true,
        references:[{target:"ENTITY",entityId:ids[index-1],subElement:"END"},{target:"ENTITY",entityId:ids[index],subElement:"START"}]}});
      if(this.segments[index].kind==="ARC")operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"TANGENT",internal:true,
        references:[{target:"ENTITY",entityId:ids[index-1],subElement:"WHOLE"},{target:"ENTITY",entityId:ids[index],subElement:"WHOLE"}]}});
    }
    if(closed&&ids.length>1)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",internal:true,
      references:[{target:"ENTITY",entityId:ids.at(-1),subElement:"END"},{target:"ENTITY",entityId:ids[0],subElement:"START"}]}});
    if(this.snaps[0]&&ids[0])operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:ids[0],subElement:"START"},this.snaps[0]]}});
    const lastSnap=this.snaps[closed?0:this.snaps.length-1];if(lastSnap&&ids.at(-1))operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:ids.at(-1),subElement:"END"},lastSnap]}});
    context.viewport.commitSketchOperations(operations);
  }
}

export class SplineSketchTool extends MultiPointSketchTool {
  readonly id:string="sketch.spline";readonly prompt:string="拟合点样条：依次单击通过点，双击或 Enter 完成，单击首点闭合";minimumPoints=3;
  protected mode:"FIT"|"CONTROL"="FIT";
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult {
    if(this.creationPending)return InputResult.Consumed;if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;
    const value=context.viewport.sketchPoint(event.x,event.y);if(!value)return InputResult.Ignored;if(this.points.length>0){const fit=[...this.points,value];
      this.previewPoints(fit,context);
      context.viewport.showReferenceDimensions([{kind:"LINE",start:fit.at(-2)!,end:value}]);}return InputResult.Consumed;}
  protected previewPoints(points:Vec2[],context:ToolContext):void {
    const curve = points.length < 2 ? points : this.mode === "FIT" ? sampleInterpolatingSpline(points,false,64)
      : sampleSketchEntity(this.controlEntity("preview",points.map(([x,y])=>({x,y})),false),64);
    context.viewport.showPolylinePreview(curve);
    context.viewport.showSplineControlPreview?.(points,this.mode);
  }
  private controlEntity(id:string,poles:SketchEntity["poles"],closed:boolean):SketchEntity {
    const count=poles!.length,degree=Math.min(3,count-1),spans=count-degree;
    return{id,kind:"SPLINE",role:this.role,mode:"CONTROL",poles,poleIds:poles!.map(()=>randomUUID()),degree,closed,
      knots:[0,...Array.from({length:spans-1},(_,index)=>(index+1)/spans),1],multiplicities:[degree+1,...Array.from({length:spans-1},()=>1),degree+1],
      weights:poles!.map(()=>1),periodic:false,parameterStart:0,parameterEnd:1};
  }
  commit(context:ToolContext):void {const closed=this.points.length>3&&this.points.at(-1)===this.points[0];const controls=closed?this.points.slice(0,-1):this.points,id=randomUUID();
    const points=controls.map(([x,y])=>({x,y}));
    const entity:SketchEntity=this.mode==="CONTROL"?this.controlEntity(id,closed?[...points,{...points[0]}]:points,closed):
      {id,kind:"SPLINE",role:this.role,mode:"FIT",controlPoints:points,controlPointIds:points.map(()=>randomUUID()),degree:Math.min(3,controls.length-1),closed};
    const operations:SketchOperation[]=[{type:"ADD_ENTITY",entity}];
    if(this.snaps[0])operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:id,subElement:"START"},this.snaps[0]]}});
    const endSnap=this.snaps[closed?0:controls.length-1];if(endSnap)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:id,subElement:"END"},endSnap]}});
    context.viewport.commitSketchOperations(operations);}
}

export class ControlSplineSketchTool extends SplineSketchTool {
  readonly id="sketch.spline.control";readonly prompt="控制点样条：依次选择控制点（不要求经过它们），Enter 完成，首点闭合；C 辅助几何";protected mode:"FIT"|"CONTROL"="CONTROL";
}

export class PointSketchTool extends SketchCreationTool {
  readonly id = "sketch.point";
  private capturedPointerID?: number;
  activate(context: ToolContext): void { context.viewport.setToolPrompt("点：单击放置；Esc 返回选择"); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if(this.creationPending){if(event.button===2){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}
    if (this.endOnRightClick(event, context)) return InputResult.Consumed;
    if (event.button !== 0 || this.capturedPointerID !== undefined || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y); if (!point) return InputResult.Ignored;
    this.capturedPointerID = event.pointerId;
    const id=randomUUID(),operations:SketchOperation[]=[{ type: "ADD_ENTITY", entity: { id, kind: "POINT", role: this.role, point: { x: point[0], y: point[1] } } }];
    const snap=this.capturedSnap(event,context);if(snap)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",references:[{target:"ENTITY",entityId:id,subElement:"POINT"},snap]}});
    this.submitCreation(context,submission=>submission.viewport.commitSketchOperations(operations),()=>{});

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
    if(this.creationPending)return InputResult.Consumed;
    if (event.state.buttons.middle || event.state.buttons.right || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y); if (!point) return InputResult.Ignored;
    context.viewport.showPointPreview(point);
    context.viewport.showReferenceDimensions([{kind:"POINT",point}]);
    return InputResult.Consumed;
  }
  keyDown(event: CadKeyboardEvent, context: ToolContext): InputResult {
    if(this.creationPending){if(event.key==="Escape"){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;} return this.creationKey(event, context); }
  deactivate(context: ToolContext): void { this.cancel(context); }
  cancel(context: ToolContext): void { this.invalidateCreationSubmission();this.capturedPointerID = undefined; context.viewport.clearToolPreview(); }
}

// These creation modes reuse the same multipoint gesture lifecycle and submit
// ordinary stable entities. Preview sampling is never the authoritative curve.
function circleThroughPoints(first: Vec2, middle: Vec2, last: Vec2): {center: Vec2; radius: number} | undefined {
  const bx=middle[0]-first[0], by=middle[1]-first[1], cx=last[0]-first[0], cy=last[1]-first[1];
  const determinant=2*(bx*cy-by*cx);
  if(determinant===0)return undefined;
  const b=bx*bx+by*by,c=cx*cx+cy*cy;
  const center:Vec2=[first[0]+(b*cy-c*by)/determinant,first[1]+(bx*c-cx*b)/determinant];
  const radius=Math.hypot(center[0]-first[0],center[1]-first[1]);
  return center.every(Number.isFinite)&&Number.isFinite(radius)&&radius>=SKETCH_INPUT_POLICY.minimumGeometryLength?{center,radius}:undefined;
}
const positiveTurn = (angle: number): number => ((angle % (2*Math.PI))+2*Math.PI)%(2*Math.PI);
function threePointArcRange(center:Vec2,first:Vec2,middle:Vec2,last:Vec2):[number,number]{
  const start=Math.atan2(first[1]-center[1],first[0]-center[0]),finish=Math.atan2(last[1]-center[1],last[0]-center[0]);
  const middleAngle=Math.atan2(middle[1]-center[1],middle[0]-center[0]);
  const ccw=positiveTurn(finish-start);
  return [start,start+(positiveTurn(middleAngle-start)<ccw?ccw:ccw-2*Math.PI)];
}

export class ThreePointCircleSketchTool extends MultiPointSketchTool {
  readonly id:string="sketch.circle.three_point";
  readonly prompt:string="三点圆：依次选择圆周上的三个点；C 辅助几何，Esc 取消，右键结束";
  minimumPoints=3;
  protected arc=false;
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult{
    if(this.creationPending){if(event.button===2){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}
    if(event.button===0&&this.points.length===2){
      const last=context.viewport.sketchPoint(event.x,event.y);
      if(!last||!circleThroughPoints(this.points[0],this.points[1],last)){
        context.viewport.setToolPrompt("三个点不能重合或共线；重新选择第三点");return InputResult.Consumed;
      }
    }
    const result=super.pointerDown(event,context);
    if(event.button===0&&this.points.length===3)super.keyDown({key:"Enter"} as CadKeyboardEvent,context);
    return result;
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult{
    if(this.creationPending)return InputResult.Consumed;
    if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;
    const last=context.viewport.sketchPoint(event.x,event.y);if(!last)return InputResult.Ignored;
    if(this.points.length!==2)return super.pointerMove(event,context);
    const circle=circleThroughPoints(this.points[0],this.points[1],last);if(!circle){context.viewport.showPolylinePreview([...this.points,last]);return InputResult.Consumed;}
    const range=this.arc?threePointArcRange(circle.center,this.points[0],this.points[1],last):[0,Math.PI*2];
    context.viewport.showPolylinePreview(Array.from({length:65},(_,index):Vec2=>{const angle=range[0]+(range[1]-range[0])*index/64;
      return[circle.center[0]+circle.radius*Math.cos(angle),circle.center[1]+circle.radius*Math.sin(angle)];}));
    context.viewport.showReferenceDimensions([{kind:"CIRCLE",center:circle.center,edge:last}]);return InputResult.Consumed;
  }
  commit(context:ToolContext):void{
    const [first,middle,last]=this.points,circle=circleThroughPoints(first,middle,last)!;
    const id=randomUUID(),range=threePointArcRange(circle.center,first,middle,last);
    const operations:SketchOperation[]=[{type:"ADD_ENTITY",entity:{id,kind:this.arc?"ARC":"CIRCLE",role:this.role,
      center:{x:circle.center[0],y:circle.center[1]},radius:circle.radius,...(this.arc?{startAngle:range[0],endAngle:range[1]}:{})}}];
    for(let index=0;index<3;index++){
      const snap=this.snaps[index];if(!snap)continue;
      if(this.arc&&(index===0||index===2))operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",
        references:[{target:"ENTITY",entityId:id,subElement:index===0?"START":"END"},snap]}});
      else operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"POINT_ON_OBJECT",
        references:[snap,{target:"ENTITY",entityId:id,subElement:"WHOLE"}]}});
    }
    context.viewport.commitSketchOperations(operations);
  }
}
export class ThreePointArcSketchTool extends ThreePointCircleSketchTool {
  readonly id="sketch.arc.three_point";readonly prompt="三点圆弧：依次选择起点、经过点和终点；C 辅助几何，Esc 取消，右键结束";protected arc=true;
}

export class CenterRectangleSketchTool extends TwoClickSketchTool {
  readonly id="sketch.rectangle.center";
  readonly firstPrompt="中心矩形：单击中心";
  readonly secondPrompt="中心矩形：单击角点，数值输入为半宽/半高；Esc 取消";
  private corners(center:Vec2,corner:Vec2):Vec2[]{return[[2*center[0]-corner[0],2*center[1]-corner[1]],[corner[0],2*center[1]-corner[1]],corner,[2*center[0]-corner[0],corner[1]]];}
  preview(center:Vec2,corner:Vec2,context:ToolContext):void{const points=this.corners(center,corner);context.viewport.showPolylinePreview(points,true);
    context.viewport.showReferenceDimensions([{kind:"LINE",start:points[0],end:points[1]},{kind:"LINE",start:points[1],end:points[2]}]);}
  commit(center:Vec2,corner:Vec2,context:ToolContext,snaps:{first?:SketchGeometryRef;second?:SketchGeometryRef}):void{
    const points=this.corners(center,corner),operations=polylineOperations(points,true,this.role);
    const ids=operations.filter(item=>item.type==="ADD_ENTITY").map(item=>item.entity.id);
    ids.forEach((id,index)=>operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:index%2===0?"HORIZONTAL":"VERTICAL",internal:true,
      references:[{target:"ENTITY",entityId:id,subElement:"WHOLE"}]}}));
    const pointID=randomUUID(),diagonalID=randomUUID();
    operations.push({type:"ADD_ENTITY",entity:{id:pointID,kind:"POINT",role:"CONSTRUCTION",point:{x:center[0],y:center[1]}}},
      {type:"ADD_ENTITY",entity:{id:diagonalID,kind:"LINE",role:"CONSTRUCTION",start:{x:points[0][0],y:points[0][1]},end:{x:corner[0],y:corner[1]}}},
      {type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"MIDPOINT",internal:true,references:[{target:"ENTITY",entityId:pointID,subElement:"POINT"},{target:"ENTITY",entityId:diagonalID,subElement:"WHOLE"}]}});
    for(const [subElement,entityID] of [["START",ids[0]],["END",ids[2]]] as const)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",internal:true,
      references:[{target:"ENTITY",entityId:diagonalID,subElement},{target:"ENTITY",entityId:entityID,subElement:"START"}]}});
    for(const [entityID,subElement,snap] of [[pointID,"POINT",snaps.first],[ids[2],"START",snaps.second]] as const)if(snap)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",
      references:[{target:"ENTITY",entityId:entityID,subElement},snap]}});
    context.viewport.commitSketchOperations(operations);context.viewport.finishToolUse();
  }
}

export class OrientedRectangleSketchTool extends MultiPointSketchTool {
  readonly id="sketch.rectangle.oriented";readonly prompt="定向矩形：依次选择底边起点、底边终点和高度；C 辅助几何，Esc 取消";minimumPoints=3;
  private corners(last:Vec2):Vec2[]|undefined{
    if(this.points.length<2)return undefined;
    const [first,second]=this.points,dx=second[0]-first[0],dy=second[1]-first[1],length=Math.hypot(dx,dy);
    if(length<SKETCH_INPUT_POLICY.minimumGeometryLength)return undefined;
    const nx=-dy/length,ny=dx/length,height=(last[0]-second[0])*nx+(last[1]-second[1])*ny;
    if(Math.abs(height)<SKETCH_INPUT_POLICY.minimumGeometryLength)return undefined;
    return[first,second,[second[0]+nx*height,second[1]+ny*height],[first[0]+nx*height,first[1]+ny*height]];
  }
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult{
    if(this.creationPending){if(event.button===2){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}
    if(event.button===0&&this.points.length===1){const point=context.viewport.sketchPoint(event.x,event.y);if(!point||Math.hypot(point[0]-this.points[0][0],point[1]-this.points[0][1])<SKETCH_INPUT_POLICY.minimumGeometryLength)return InputResult.Consumed;}
    if(event.button===0&&this.points.length===2){const point=context.viewport.sketchPoint(event.x,event.y);if(!point||!this.corners(point)){context.viewport.setToolPrompt("矩形高度不能为零；重新选择高度");return InputResult.Consumed;}}
    const result=super.pointerDown(event,context);if(event.button===0&&this.points.length===3)super.keyDown({key:"Enter"} as CadKeyboardEvent,context);return result;
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult{
    if(this.creationPending)return InputResult.Consumed;
    const point=context.viewport.sketchPoint(event.x,event.y),corners=point&&this.corners(point);
    if(corners){context.viewport.showPolylinePreview(corners,true);context.viewport.showReferenceDimensions([{kind:"LINE",start:corners[0],end:corners[1]},{kind:"LINE",start:corners[1],end:corners[2]}]);return InputResult.Consumed;}
    return super.pointerMove(event,context);
  }
  commit(context:ToolContext):void{
    const operations=polylineOperations(this.corners(this.points[2])!,true,this.role),ids=operations.filter(item=>item.type==="ADD_ENTITY").map(item=>item.entity.id);
    for(const [first,second,kind] of [[0,2,"PARALLEL"],[1,3,"PARALLEL"],[0,1,"PERPENDICULAR"]] as const)operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind,internal:true,
      references:[{target:"ENTITY",entityId:ids[first],subElement:"WHOLE"},{target:"ENTITY",entityId:ids[second],subElement:"WHOLE"}]}});
    for(let index=0;index<2;index++)if(this.snaps[index])operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",
      references:[{target:"ENTITY",entityId:ids[index],subElement:"START"},this.snaps[index]!]}});
    // Height is a perpendicular projection. A nearby cursor reference is not
    // automatically a corner reference and must not be persisted as one.
    context.viewport.commitSketchOperations(operations);
  }
}

export class EllipseSketchTool extends MultiPointSketchTool {
  readonly id:string="sketch.ellipse";
  readonly prompt:string="椭圆：依次选择中心、主轴端点和次轴宽度；C 辅助几何，Esc 取消";
  minimumPoints=3;
  protected arc=false;
  private lastHover?:CadPointerEvent; private reverseHeld=false; private clockwise=false;
  private numericFields:[string,string]=["",""];
  private numericField=0;
  protected resetCreation():void{this.numericFields=["",""];this.numericField=0;this.lastHover=undefined;this.reverseHeld=false;}
  private numericPoint():Vec2|undefined{
    if(!this.numericFields.some(Boolean)||!this.points.length)return undefined;
    const radius=Number(this.numericFields[0]),center=this.points[0];
    if(!Number.isFinite(radius))return undefined;
    if(this.points.length===1){const angle=Number(this.numericFields[1]||"0")*Math.PI/180;
      return radius>=SKETCH_INPUT_POLICY.minimumGeometryLength&&Number.isFinite(angle)?[center[0]+radius*Math.cos(angle),center[1]+radius*Math.sin(angle)]:undefined;}
    if(this.points.length===2){const major=this.points[1],angle=Math.atan2(major[1]-center[1],major[0]-center[0]),length=Math.hypot(major[0]-center[0],major[1]-center[1]);
      return radius>=SKETCH_INPUT_POLICY.minimumGeometryLength&&radius<length?[center[0]-radius*Math.sin(angle),center[1]+radius*Math.cos(angle)]:undefined;}
    const ellipse=this.ellipse(this.points);return ellipse?this.at(ellipse,radius*Math.PI/180):undefined;
  }
  private numericPrompt(context:ToolContext):void{
    context.viewport.setToolPrompt(this.points.length===1?`椭圆主轴半径 mm: ${this.numericFields[0]||"待输入"}，角度°: ${this.numericFields[1]||"0"}；Tab 切换，Enter 确认`:
      this.points.length===2?`椭圆次轴半径 mm: ${this.numericFields[0]||"待输入"}（小于主轴）；Enter 确认`:
      `椭圆弧${this.points.length===3?"起":"终"}参数角°: ${this.numericFields[0]||"待输入"}（相对主轴）；Enter 确认，R 反向`);
  }
  private numericPreview(context:ToolContext):void{
    const target=this.numericPoint();if(!target)return;
    if(this.points.length===1){context.viewport.showPolylinePreview([this.points[0],target]);return;}
    const ellipse=this.ellipse(this.points.length===2?[...this.points,target]:this.points);if(!ellipse)return;
    const range=this.points.length===4?this.range(ellipse,target):[0,2*Math.PI];
    context.viewport.showPolylinePreview(Array.from({length:97},(_,index)=>this.at(ellipse,range[0]+(range[1]-range[0])*index/96)),this.points.length<4);
    if(this.points.length===3)context.viewport.showPointPreview(target);
  }
  private ellipse(points:readonly Vec2[]){
    if(points.length<3)return undefined;
    const [center,major,minor]=points,dx=major[0]-center[0],dy=major[1]-center[1],majorRadius=Math.hypot(dx,dy);
    if(majorRadius<SKETCH_INPUT_POLICY.minimumGeometryLength)return undefined;
    const rotation=Math.atan2(dy,dx),minorRadius=Math.abs(-(minor[0]-center[0])*Math.sin(rotation)+(minor[1]-center[1])*Math.cos(rotation));
    if(minorRadius<SKETCH_INPUT_POLICY.minimumGeometryLength||minorRadius>=majorRadius)return undefined;
    return{center,majorRadius,minorRadius,rotation};
  }
  private parameter(ellipse:NonNullable<ReturnType<EllipseSketchTool["ellipse"]>>,point:Vec2):number{
    const dx=point[0]-ellipse.center[0],dy=point[1]-ellipse.center[1],c=Math.cos(ellipse.rotation),s=Math.sin(ellipse.rotation);
    return Math.atan2((-dx*s+dy*c)/ellipse.minorRadius,(dx*c+dy*s)/ellipse.majorRadius);
  }
  private at(ellipse:NonNullable<ReturnType<EllipseSketchTool["ellipse"]>>,parameter:number):Vec2{
    const x=ellipse.majorRadius*Math.cos(parameter),y=ellipse.minorRadius*Math.sin(parameter),c=Math.cos(ellipse.rotation),s=Math.sin(ellipse.rotation);
    return[ellipse.center[0]+x*c-y*s,ellipse.center[1]+x*s+y*c];
  }
  private range(ellipse:NonNullable<ReturnType<EllipseSketchTool["ellipse"]>>,last:Vec2):[number,number]{
    const start=this.parameter(ellipse,this.points[3]),sweep=positiveTurn(this.parameter(ellipse,last)-start);
    return[start,start+((this.clockwise!==this.reverseHeld)?sweep-2*Math.PI:sweep)];
  }
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult{
    this.reverseHeld=!!event.state.modifiers?.ctrl;
    if(this.creationPending){if(event.button===2){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}
    if(event.button===0){
      const point=context.viewport.sketchPoint(event.x,event.y);if(!point)return InputResult.Ignored;
      if(this.points.length===1&&Math.hypot(point[0]-this.points[0][0],point[1]-this.points[0][1])<SKETCH_INPUT_POLICY.minimumGeometryLength)return InputResult.Consumed;
      if(this.points.length===2&&!this.ellipse([...this.points,point])){context.viewport.setToolPrompt("次轴半径必须大于零且小于主轴半径；相等时请用圆");return InputResult.Consumed;}
      if(this.arc&&this.points.length>=3&&Math.hypot(point[0]-this.points[0][0],point[1]-this.points[0][1])<SKETCH_INPUT_POLICY.minimumGeometryLength){context.viewport.setToolPrompt("弧端点不能位于椭圆中心");return InputResult.Consumed;}
      if(this.arc&&this.points.length===4){const range=this.range(this.ellipse(this.points)!,point),sweep=Math.abs(range[1]-range[0]);
        if(sweep<1e-9||sweep>=Math.PI*2-1e-9){context.viewport.setToolPrompt("弧范围不能为零或完整周期；完整轮廓请用椭圆");return InputResult.Consumed;}}
    }
    if(event.button===0){this.numericFields=["",""];this.numericField=0;}
    const result=super.pointerDown(event,context);
    if(event.button===0&&this.points.length===this.minimumPoints)super.keyDown({key:"Enter"} as CadKeyboardEvent,context);
    else if(this.arc&&this.points.length===3)context.viewport.setToolPrompt("椭圆弧：选择范围起点，端点按椭圆参数投影；R 反向");
    else if(this.arc&&this.points.length===4)context.viewport.setToolPrompt("椭圆弧：选择范围终点；R 反向，Esc 取消");
    return result;
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult{
    this.reverseHeld=!!event.state.modifiers?.ctrl;this.lastHover=event;
    if(this.creationPending)return InputResult.Consumed;
    if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;
    if(this.numericFields.some(Boolean)){this.numericPreview(context);return InputResult.Consumed;}
    const point=context.viewport.sketchPoint(event.x,event.y);if(!point)return InputResult.Ignored;
    const ellipse=this.ellipse(this.points.length===2?[...this.points,point]:this.points);
    if(!ellipse)return super.pointerMove(event,context);
    const range=this.arc&&this.points.length>=4?this.range(ellipse,point):[0,2*Math.PI];
    context.viewport.showPolylinePreview(Array.from({length:97},(_,index)=>this.at(ellipse,range[0]+(range[1]-range[0])*index/96)),!this.arc||this.points.length<4);
    context.viewport.showReferenceDimensions([{kind:"LINE",start:ellipse.center,end:this.at(ellipse,0)},{kind:"LINE",start:ellipse.center,end:this.at(ellipse,Math.PI/2)}]);
    return InputResult.Consumed;
  }
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult{
    if(this.creationPending){if(event.key==="Escape"){this.cancel(context);context.viewport.finishToolUse(true);}return InputResult.Consumed;}
    if(event.editableTarget)return InputResult.Ignored;
    if(event.key==="Control"){this.keyUp(event,context);return InputResult.Consumed;}
    this.reverseHeld=!!event.state?.modifiers.ctrl;
    if(this.arc&&event.key.toLowerCase()==="r"){this.clockwise=!this.clockwise;context.viewport.setToolPrompt(`椭圆弧：${this.clockwise?"顺时针":"逆时针"}；选择范围端点，R 反向`);this.numericPreview(context);return InputResult.Consumed;}
    if(this.points.length&&this.points.length<this.minimumPoints&&!event.state?.modifiers.ctrl&&!event.state?.modifiers.meta){
      if(event.key==="Tab"){this.numericField=this.points.length===1?1-this.numericField:0;this.numericPrompt(context);return InputResult.Consumed;}
      if(/^[0-9.\-+]$/.test(event.key)||event.key==="Backspace"){
        this.numericFields[this.numericField]=event.key==="Backspace"?this.numericFields[this.numericField].slice(0,-1):this.numericFields[this.numericField]+event.key;
        this.numericPreview(context);this.numericPrompt(context);return InputResult.Consumed;
      }
      if(event.key==="Enter"&&this.numericFields.some(Boolean)){
        const target=this.numericPoint();if(!target){context.viewport.setToolPrompt("输入有限参数；主轴半径>0，次轴半径>0且小于主轴，弧角以度输入");return InputResult.Consumed;}
        if(this.points.length===4){const range=this.range(this.ellipse(this.points)!,target),sweep=Math.abs(range[1]-range[0]);if(sweep<1e-9||sweep>=Math.PI*2-1e-9){context.viewport.setToolPrompt("弧范围不能为零或完整周期");return InputResult.Consumed;}}
        this.addPoint(target,undefined);this.numericFields=["",""];this.numericField=0;
        if(this.points.length===this.minimumPoints)return super.keyDown(event,context);
        this.numericPrompt(context);return InputResult.Consumed;
      }
    }
    return super.keyDown(event,context);
  }
  keyUp(event:CadKeyboardEvent,context:ToolContext):InputResult {if(event.key!=="Control"||event.editableTarget)return InputResult.Ignored;this.reverseHeld=!!event.state.modifiers.ctrl;if(this.lastHover&&!this.creationPending)this.pointerMove({...this.lastHover,state:{...this.lastHover.state,modifiers:event.state.modifiers}},context);return InputResult.Consumed;}
  commit(context:ToolContext):void{
    const ellipse=this.ellipse(this.points)!,id=randomUUID(),range=this.arc?this.range(ellipse,this.points[4]):[0,2*Math.PI];
    const operations:SketchOperation[]=[{type:"ADD_ENTITY",entity:{id,kind:this.arc?"ELLIPTICAL_ARC":"ELLIPSE",role:this.role,
      center:{x:ellipse.center[0],y:ellipse.center[1]},majorRadius:ellipse.majorRadius,minorRadius:ellipse.minorRadius,rotation:ellipse.rotation,
      ...(this.arc?{startAngle:range[0],endAngle:range[1]}:{})}}];
    if(this.snaps[0])operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",
      references:[{target:"ENTITY",entityId:id,subElement:"CENTER"},this.snaps[0]]}});
    // Range picks project onto the exact ellipse. Only an already-on-curve
    // snapped endpoint can persist an endpoint connection.
    if(this.arc)for(const [index,subElement,parameter] of [[3,"START",range[0]],[4,"END",range[1]]] as const){
      const snap=this.snaps[index],actual=this.at(ellipse,parameter),picked=this.points[index];
      if(snap&&Math.hypot(actual[0]-picked[0],actual[1]-picked[1])<=1e-10*Math.max(1,ellipse.majorRadius))operations.push({type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:"COINCIDENT",
        references:[{target:"ENTITY",entityId:id,subElement},snap]}});
    }
    context.viewport.commitSketchOperations(operations);
  }
}
export class EllipticalArcSketchTool extends EllipseSketchTool {
  readonly id="sketch.elliptical_arc";readonly prompt="椭圆弧：中心、主轴、次轴，然后选择起点与终点；按住 Ctrl 反向，R 切换默认方向";minimumPoints=5;protected arc=true;
}

// Constraint tools intentionally share the same tool lifecycle now. Entity
// reference picking is the next extension point; commands stay typed and no
// topology or array index is persisted.
export class ConstraintSketchTool implements CadTool {
  private phase:{step:"SELECTING";references:SketchGeometryRef[]}|{step:"PLACING"|"COMMITTING"|"FAILED"|"UNKNOWN";references:SketchGeometryRef[]}={step:"SELECTING",references:[]};
  private get references(){return this.phase.references;}
  private set references(references:SketchGeometryRef[]){this.phase={step:"SELECTING",references};}
  private freezeDefinition():void{this.phase={step:"PLACING",references:Object.freeze(this.references.map(ref=>Object.freeze({...ref}))) as unknown as SketchGeometryRef[]};}
  private pendingOperation?:SketchOperation;
  private requestID?:string;
  private generation=0;
  private error?:string;
  private definitionScope?:string;
  private baseVersionID?:string;
  private rememberScope(context:ToolContext):void {if(this.definitionScope!==undefined)return;this.definitionScope=JSON.stringify(context.viewport.currentSketchIdentity?.())??"";this.baseVersionID=context.viewport.currentSketchIdentity?.()?.versionId;}
  private publish(context:ToolContext):void {
    if(this.spec.unit)return;
    const phase:SketchCommandState["phase"]=this.phase.step==="COMMITTING"?"committing":this.phase.step==="FAILED"?"failed":this.phase.step==="UNKNOWN"?"unknown":"selection";
    context.viewport.setSketchCommandState?.({toolId:this.id,operation:`${this.spec.label}约束`,phase,presentation:this.phase.step==="FAILED"||this.phase.step==="UNKNOWN"?"advanced":"inline",role:this.spec.pickLabels[this.references.length]??"确认约束",selectedIds:this.references.flatMap(ref=>ref.entityId?[ref.entityId]:[]),references:[...this.references],fields:[],options:[],canConfirm:this.phase.step==="FAILED",next:this.phase.step==="UNKNOWN"?"查询原请求结果后重试":this.phase.step==="FAILED"?"重试或返回重新选择":this.phase.step==="COMMITTING"?"等待权威验证":this.prompt(),error:this.error});
  }
  commandAction(action:SketchCommandAction,context:ToolContext):void {
    if(action.type==="cancel"){this.cancel(context);context.viewport.finishToolUse(true);return;}
    if(this.phase.step==="COMMITTING")return;
    if(action.type==="retry"&&this.pendingOperation){this.commit(context);return;}
    if(this.phase.step==="UNKNOWN")return;
    if(action.type==="back"){this.cancel(context);return;}
    if(action.type==="confirm"&&this.phase.step==="FAILED")this.commit(context);
  }
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
  selectionInput(selections:readonly SelectionItem[],context:ToolContext):SelectionInputResult {
    if(this.phase.step!=="SELECTING")return SelectionInputResult.Rejected;
    const previousCount=this.references.length;
    const entities=localSketchSelection(selections,context),all=context.viewport.currentSketchEntities?.()??[];
    for(const entity of entities){if(this.references.length>=this.spec.picks.length)break;
      const retained=all.find(candidate=>candidate.id===this.references[0]?.entityId);
      const reference=preselectedSketchReference(entity,this.spec.picks[this.references.length],retained);
      if(reference&&!this.references.some(item=>sameSketchReference(item,reference))){this.rememberScope(context);this.references.push({...reference});}
    }
    if(this.references.length===previousCount)return entities.length?SelectionInputResult.Rejected:SelectionInputResult.Unhandled;
    if(this.references.length===this.spec.picks.length&&!this.spec.unit){this.commit(context);return SelectionInputResult.Accepted;}
    context.viewport.showReferencePreview(this.references.at(-1)!,this.references);
    if(this.references.length===this.spec.picks.length)context.viewport.showConstraintPreview(this.kind,this.references,context.viewport.measureDimension(this.kind,this.references));
    if(this.references.length===this.spec.picks.length)this.freezeDefinition();
    context.viewport.setToolPrompt(this.prompt());this.publish(context);return SelectionInputResult.Accepted;
  }
  activate(context: ToolContext): void { context.viewport.setToolPrompt(this.prompt());this.publish(context); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || this.capturedPointerID !== undefined || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    if(["COMMITTING","FAILED","UNKNOWN"].includes(this.phase.step))return InputResult.Consumed;
    if(this.phase.step==="PLACING"){
      context.viewport.clearReferenceHover?.();
      if(!this.spec.unit||this.labelPosition)return InputResult.Ignored;
      const position=context.viewport.sketchPlacementPoint(event.x,event.y);if(!position)return InputResult.Consumed;
      const value=context.viewport.measureDimension(this.kind,this.references);if(value===undefined||!Number.isFinite(value)||!this.spec.unit){context.viewport.setToolPrompt("当前尺寸不可测；定义保持不变，请取消或重新选择");return InputResult.Consumed;}
      this.capturedPointerID=event.pointerId;this.labelPosition=position;
      context.viewport.requestDimensionCreation(this.kind,this.references,value,this.spec.unit,position,event.x,event.y);
      this.references=[];this.labelPosition=undefined;context.viewport.clearReferencePreview();
      context.viewport.setToolPrompt(this.prompt());context.viewport.finishToolUse();return InputResult.Capture;
    }
    const reference = context.viewport.sketchReferenceAt(event.x, event.y, this.spec.picks[this.references.length], this.references[0]); if (!reference) return InputResult.Consumed;
    if (this.references.some((item)=>sameSketchReference(item, reference))) {
      context.viewport.showReferencePreview(reference,this.references);
      return InputResult.Consumed;
    }
    this.capturedPointerID = event.pointerId;
    this.rememberScope(context);this.references.push({...reference});context.viewport.showReferencePreview(reference,this.references);
    if(this.references.length===this.spec.picks.length&&!this.spec.unit)this.commit(context);
    else {
      if(this.references.length===this.spec.picks.length){this.freezeDefinition();context.viewport.showConstraintPreview(this.kind,this.references,
        context.viewport.measureDimension(this.kind,this.references));}
      context.viewport.setToolPrompt(this.prompt());
    }
    this.publish(context);return InputResult.Capture;
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
    if(["COMMITTING","FAILED","UNKNOWN"].includes(this.phase.step))return InputResult.Consumed;
    if(this.phase.step==="PLACING"){
      context.viewport.clearReferenceHover?.();
      if(!this.spec.unit||this.labelPosition)return InputResult.Ignored;
      const position=context.viewport.sketchPlacementPoint(event.x,event.y);if(!position)return InputResult.Consumed;
      context.viewport.showConstraintPreview(this.kind,this.references,context.viewport.measureDimension(this.kind,this.references),position);return InputResult.Consumed;
    }
    const reference = context.viewport.sketchReferenceAt(event.x, event.y, this.spec.picks[this.references.length], this.references[0]);
    if (!reference) {
      if (this.references[0]) context.viewport.showReferencePreview(this.references.at(-1)!,this.references);
      else context.viewport.clearReferenceHover?.();
      return InputResult.Consumed;
    }
    if (this.references.some((item)=>sameSketchReference(item,reference))) {
      context.viewport.showReferencePreview(reference,this.references);
      return InputResult.Consumed;
    }
    context.viewport.showReferencePreview(reference, this.references);
    return InputResult.Consumed;
  }
  keyDown(event: CadKeyboardEvent, context: ToolContext): InputResult {
    if(event.editableTarget||event.isComposing)return InputResult.Ignored;
    if(event.key==="Escape"){this.cancel(context);return InputResult.Ignored;}
    return InputResult.Ignored;
  }
  deactivate(context: ToolContext): void { this.cancel(context);context.viewport.setSketchCommandState?.(undefined); }
  cancel(context: ToolContext): void {
    this.generation++;this.pendingOperation=undefined;this.requestID=undefined;this.error=undefined;this.definitionScope=undefined;this.baseVersionID=undefined;
    this.capturedPointerID = undefined;
    this.references = [];
    this.labelPosition=undefined;
    context.viewport.clearReferencePreview();
    context.viewport.setToolPrompt(this.prompt());this.publish(context);
  }
  private commit(context:ToolContext):void {
    if(this.phase.step==="COMMITTING")return;
    const receiptRetry=this.phase.step==="UNKNOWN";
    if(!receiptRetry&&this.definitionScope!==(JSON.stringify(context.viewport.currentSketchIdentity?.())??"")){this.phase={step:"FAILED",references:this.references};this.error="草图版本或上下文已变化；返回重新选择后再确认";this.publish(context);return;}
    this.pendingOperation??={type:"ADD_CONSTRAINT",constraint:{id:randomUUID(),kind:this.kind,references:this.references.map(ref=>({...ref}))}};
    this.phase={step:"COMMITTING",references:this.references};this.error=undefined;const generation=++this.generation;
    context.viewport.clearReferencePreview();this.publish(context);
    const accepted=()=>{if(generation!==this.generation)return;this.cancel(context);context.viewport.finishToolUse();};
    const failed=(error:unknown)=>{if(generation!==this.generation)return;const unknown=sketchCommitResultUnknown(error);this.phase={step:unknown?"UNKNOWN":"FAILED",references:this.references};if(!unknown)this.requestID=undefined;this.error=error instanceof Error?error.message:String(error);this.publish(context);context.viewport.setToolPrompt(this.error);};
    try {const result=context.viewport.commitSketchOperations([this.pendingOperation],{requestId:this.requestID??=randomUUID(),baseVersionId:this.baseVersionID,retryReceipt:receiptRetry});
      if(result&&typeof result.then==="function")void result.then(accepted,failed);else accepted();
    }catch(error){failed(error);}
  }
}

type LinearDimensionPhase =
  | {step:"SELECTING";references:SketchGeometryRef[]}
  | {step:"LINE_PENDING";references:readonly SketchGeometryRef[]}
  | {step:"PLACING";kind:"LENGTH"|"DISTANCE";references:readonly SketchGeometryRef[]}
  | {step:"VALUE";kind:"LENGTH"|"DISTANCE";references:readonly SketchGeometryRef[];position:Vec2;value:number;constraintId:string};
export class LinearDimensionSketchTool implements CadTool {
  readonly id="sketch.dimension.linear";
  private phase:LinearDimensionPhase={step:"SELECTING",references:[]};
  private capturedPointerID?:number;
  private cursor?:Vec2;
  private anchor?:Vec2;
  private source="";
  private sourceEdited=false;
  private unit:DisplayLengthUnit="mm";
  private inputGeneration=0;
  private generation=0;
  private error?:string;
  private status?:"committing"|"failed"|"unknown";
  private session=new SketchDimensionCommitSession();
  private scope?:ReturnType<NonNullable<ToolViewportPort["currentSketchIdentity"]>>;
  private freeze(kind:"LENGTH"|"DISTANCE",references:readonly SketchGeometryRef[]):void {
    this.error=undefined;this.phase={step:"PLACING",kind,references:Object.freeze(references.map(reference=>Object.freeze({...reference})))};
  }
  private rememberScope(context:ToolContext):void{const scope=context.viewport.currentSketchIdentity?.();if(!this.scope&&scope)this.scope={...scope};}
  private preview(context:ToolContext,position?:Vec2):void {
    const refs=[...this.phase.references];
    if(this.phase.step!=="SELECTING"){
      const kind=this.phase.step==="LINE_PENDING"?"LENGTH":this.phase.kind;
      context.viewport.showConstraintPreview(kind,refs,context.viewport.measureDimension(kind,refs),this.phase.step==="VALUE"?this.phase.position:position);
    }else if(refs.length)context.viewport.showReferencePreview(refs.at(-1)!,refs);
  }
  private publish(context:ToolContext):void {
    const step=this.phase.step,value=step==="VALUE",refs=[...this.phase.references];
    context.viewport.setSketchCommandState?.({toolId:this.id,operation:"线性尺寸",phase:this.status??(step==="PLACING"?"placement":step==="SELECTING"?"selection":"definition"),
      presentation:"inline",role:step==="LINE_PENDING"?"暂定线长或选择第二对象":step==="PLACING"?"放置尺寸":value?"输入尺寸值或表达式":"选择点或直线",selectedIds:refs.flatMap(ref=>ref.entityId?[ref.entityId]:[]),references:refs,
      count:{accepted:refs.length,required:step==="SELECTING"?2:undefined},completion:{label:step==="LINE_PENDING"?"锁定长度":value?"创建尺寸":"放置尺寸"},
      fields:value?[{label:"尺寸",value:this.source,unit:"length",displayUnit:this.unit,placeholder:this.unit}]:[],options:[],
      input:value&&this.status!=="committing"&&this.status!=="unknown"?{id:`${this.id}:${this.inputGeneration}`,fieldIndex:0,anchor:this.phase.step==="VALUE"?context.viewport.projectSketchPoint?.(this.phase.position)??this.anchor:this.anchor,modelAnchor:this.phase.step==="VALUE"?this.phase.position:undefined,dimension:true}:undefined,
      canConfirm:this.status!=="committing"&&this.status!=="unknown"&&(step==="LINE_PENDING"||value||step==="PLACING"&&!!this.cursor),next:this.definitionPrompt(),error:this.error,preview:"approximate"});
    context.viewport.setToolPrompt(this.definitionPrompt());
  }
  private validPair(refs:readonly SketchGeometryRef[],context:ToolContext):boolean {
    if(refs.every(ref=>ref.subElement==="WHOLE"||ref.subElement==="DIRECTION")){
      const entities=context.viewport.currentSketchReferenceEntities?.()??context.viewport.currentSketchEntities?.()??[];
      const direction=(ref:SketchGeometryRef):Vec2|undefined=>{
        if(ref.target==="SKETCH_X_AXIS")return [1,0];if(ref.target==="SKETCH_Y_AXIS")return [0,1];
        const entity=entities.find(entity=>entity.id===ref.entityId);return entity?.kind==="LINE"&&entity.start&&entity.end?[entity.end.x-entity.start.x,entity.end.y-entity.start.y]:undefined;
      };
      const a=direction(refs[0]),b=direction(refs[1]),norm=a&&b?Math.hypot(...a)*Math.hypot(...b):0;
      if(!a||!b||!norm||Math.abs(a[0]*b[1]-a[1]*b[0])>1e-10*norm){this.error="线线间距需要平行直线；保留当前单线，或使用显式距离命令建立平行关系";this.publish(context);return false;}
    }
    return true;
  }
  private accept(references:SketchGeometryRef[],context:ToolContext):boolean {
    if(references.length===2&&!this.validPair(references,context))return false;
    this.rememberScope(context);this.error=undefined;
    if(references.length===2)this.freeze("DISTANCE",references);
    else if(references[0]?.target==="ENTITY"&&references[0].subElement==="WHOLE")this.phase={step:"LINE_PENDING",references:Object.freeze(references.map(ref=>Object.freeze({...ref})))};
    else this.phase={step:"SELECTING",references};
    this.preview(context);this.publish(context);return true;
  }
  selectionInput(selections:readonly SelectionItem[],context:ToolContext,source:SelectionInputSource="selection"):SelectionInputResult {
    if(source==="command"||source==="result")return SelectionInputResult.Unhandled;
    if(this.phase.step==="PLACING"||this.phase.step==="VALUE")return SelectionInputResult.Rejected;
    const existing=this.phase.references,entities=localSketchSelection(selections,context),picked=entities.flatMap(entity=>{const ref=preselectedSketchReference(entity,"LINEAR_DIMENSION");return ref?[ref]:[];});
    if(!entities.length)return SelectionInputResult.Unhandled;
    const added=picked.filter(ref=>!existing.some(item=>sameSketchReference(item,ref))),references=[...existing,...added];
    if(!added.length)return SelectionInputResult.Rejected;
    if(picked.length!==entities.length||entities.length!==selections.length||references.length>2){this.error="线性尺寸需要一条直线，或合法的两个点/直线；重新选择明确对象";this.publish(context);return SelectionInputResult.Rejected;}
    return this.accept(references,context)?SelectionInputResult.Accepted:SelectionInputResult.Rejected;
  }
  private definitionPrompt():string {
    if(this.status==="committing")return "尺寸正在提交，请等待权威结果";
    if(this.status==="unknown")return "尺寸结果待确认；查询原提交结果或取消";
    return this.phase.step==="LINE_PENDING"?"暂定线长：选择第二条平行直线/点改为距离，空白单击放置长度，Enter 锁定长度；Backspace 重选":
      this.phase.step==="PLACING"?"尺寸定义已冻结：单击放置标注，命中其他几何不会改变定义；Backspace 返回":
      this.phase.step==="VALUE"?"输入尺寸值或表达式，Enter 创建；Backspace 返回放置，Esc 退出":"线性尺寸：选择点或直线；Backspace 返回上一选择";
  }
  activate(context:ToolContext):void{this.publish(context);}
  private place(context:ToolContext,position:Vec2):void {
    if(this.phase.step!=="PLACING")return;
    const value=context.viewport.measureDimension(this.phase.kind,[...this.phase.references]);
    if(value===undefined||!Number.isFinite(value)){this.error="当前尺寸不可测；定义保持不变，请取消或重新选择";this.publish(context);return;}
    this.unit=context.viewport.currentLengthUnit?.()??"mm";
    this.source=formatDimensionValue(dimensionDisplayValue(value,this.unit));this.sourceEdited=false;this.inputGeneration++;
    this.phase={...this.phase,step:"VALUE",position:[...position],value,constraintId:randomUUID()};this.preview(context);this.publish(context);
  }
  pointerDown(event:CadPointerEvent,context:ToolContext):InputResult {
    if(event.button!==0||this.capturedPointerID!==undefined||event.state.buttons.middle||event.state.buttons.right||!context.viewport.hasActiveSketch())return InputResult.Ignored;
    if(this.status==="committing"||this.status==="unknown"||this.phase.step==="VALUE")return InputResult.Consumed;
    if(this.phase.step==="LINE_PENDING"){
      const reference=context.viewport.sketchReferenceAt(event.x,event.y,"LINEAR_DIMENSION",this.phase.references[0]);
      if(reference){if(!this.phase.references.some(item=>sameSketchReference(item,reference))&&this.accept([...this.phase.references,{...reference}],context)){this.capturedPointerID=event.pointerId;return InputResult.Capture;}return InputResult.Consumed;}
      this.freeze("LENGTH",this.phase.references);
    }
    if(this.phase.step==="PLACING"){
      const position=context.viewport.sketchPlacementPoint(event.x,event.y);if(!position)return InputResult.Consumed;
      this.capturedPointerID=event.pointerId;this.anchor=[event.x,event.y];this.cursor=position;this.place(context,position);return InputResult.Capture;
    }
    const reference=context.viewport.sketchReferenceAt(event.x,event.y,"LINEAR_DIMENSION",this.phase.references[0]);
    if(!reference||this.phase.references.some(item=>sameSketchReference(item,reference)))return InputResult.Consumed;
    if(!this.accept([...this.phase.references,{...reference}],context))return InputResult.Consumed;
    this.capturedPointerID=event.pointerId;return InputResult.Capture;
  }
  pointerMove(event:CadPointerEvent,context:ToolContext):InputResult {
    if(event.state.buttons.middle||event.state.buttons.right)return InputResult.Ignored;
    if(this.status==="committing"||this.status==="unknown"||this.phase.step==="VALUE")return InputResult.Consumed;
    const position=context.viewport.sketchPlacementPoint(event.x,event.y);if(position){this.cursor=position;this.anchor=[event.x,event.y];}
    if(this.phase.step==="PLACING"){context.viewport.clearReferenceHover?.();if(position)this.preview(context,position);return InputResult.Consumed;}
    if(this.phase.step==="LINE_PENDING"&&position)this.preview(context,position);
    const reference=context.viewport.sketchReferenceAt(event.x,event.y,"LINEAR_DIMENSION",this.phase.references[0]);
    if(reference&&!this.phase.references.some(item=>sameSketchReference(item,reference)))context.viewport.showReferencePreview(reference,this.phase.references);else {context.viewport.clearReferenceHover?.();if(this.phase.step==="SELECTING")this.preview(context);}
    return InputResult.Consumed;
  }
  pointerUp(event:CadPointerEvent):InputResult{if(event.button!==0||event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.capturedPointerID=undefined;return InputResult.ReleaseCapture;}
  pointerCancel(event:CadPointerEvent,context:ToolContext):InputResult{if(event.pointerId!==this.capturedPointerID)return InputResult.Ignored;this.cancel(context);return InputResult.Consumed;}
  private back(context:ToolContext):void {
    if(this.status==="committing"||this.status==="unknown")return;
    const phase=this.phase,refs=[...phase.references];this.error=undefined;this.status=undefined;
    if(phase.step==="VALUE")this.freeze(phase.kind,refs);
    else if(phase.step==="PLACING")this.phase=refs.length===1&&refs[0].subElement==="WHOLE"?{step:"LINE_PENDING",references:refs}:{step:"SELECTING",references:refs.slice(0,-1)};
    else this.phase={step:"SELECTING",references:refs.slice(0,-1)};
    context.viewport.clearReferencePreview();this.preview(context);this.publish(context);
  }
  private confirm(context:ToolContext):void {
    if(this.status==="committing"||this.status==="unknown")return;
    if(this.phase.step==="LINE_PENDING"){this.freeze("LENGTH",this.phase.references);this.preview(context,this.cursor);this.publish(context);}
    else if(this.phase.step==="PLACING"&&this.cursor)this.place(context,this.cursor);
    else if(this.phase.step==="VALUE")this.submit(context);
  }
  private submit(context:ToolContext,retry=false):void {
    if(this.phase.step!=="VALUE"||this.status==="committing"||this.status==="unknown"&&!retry)return;
    const current=context.viewport.currentSketchIdentity?.();
    if(!retry&&this.scope&&JSON.stringify(this.scope)!==JSON.stringify(current)){this.error="草图版本或上下文已变，请取消后重新选择";this.publish(context);return;}
    const phase=this.phase;let operation:SketchOperation;
    try{operation=dimensionDefinitionOperation({id:phase.constraintId,kind:phase.kind,references:[...phase.references],unit:"mm",value:phase.value,labelPosition:{x:phase.position[0],y:phase.position[1]}},undefined,this.source,"","",false,"ORIGINAL",this.unit);}catch(error){this.error=error instanceof Error?error.message:String(error);this.publish(context);return;}
    const owner=JSON.stringify([this.scope?.documentId,this.scope?.sketchId,this.scope?.occurrencePath]);
    const generation=++this.generation;this.status="committing";this.error=undefined;this.publish(context);
    void this.session.submit([operation],this.scope?.versionId??current?.versionId??"",async(ops,intent)=>context.viewport.commitSketchOperations(ops,intent)).then(()=>{
      if(generation!==this.generation)return;const scope=context.viewport.currentSketchIdentity?.();if(owner!==JSON.stringify([scope?.documentId,scope?.sketchId,scope?.occurrencePath])){this.cancel(context);this.publish(context);return;}this.cancel(context);context.viewport.finishToolUse();
    },error=>{if(generation!==this.generation)return;const scope=context.viewport.currentSketchIdentity?.();if(owner!==JSON.stringify([scope?.documentId,scope?.sketchId,scope?.occurrencePath])){this.cancel(context);this.publish(context);return;}this.status=this.session.unknown?"unknown":"failed";this.error=error instanceof Error?error.message:String(error);this.publish(context);});
  }
  commandAction(action:SketchCommandAction,context:ToolContext):void {
    if(action.type==="placement"&&this.phase.step==="VALUE"&&this.status!=="committing"&&this.status!=="unknown"){
      const position=context.viewport.sketchPlacementPoint(action.position[0],action.position[1]);
      if(position){this.phase={...this.phase,position};this.preview(context,position);this.publish(context);}return;
    }
    if(action.type==="cancel"){this.cancel(context);context.viewport.finishToolUse(true);return;}
    if(action.type==="retry"){this.submit(context,true);return;}
    if(this.status==="committing"||this.status==="unknown")return;
    if(action.type==="back"){this.back(context);return;}
    if(action.type==="confirm"){this.confirm(context);return;}
    if(action.type==="input"&&this.phase.step==="VALUE"){this.inputGeneration++;this.publish(context);return;}
    if(action.type==="field"&&action.index===0&&this.phase.step==="VALUE"){this.source=action.value;this.sourceEdited=true;this.status=undefined;this.error=undefined;this.publish(context);}
  }
  keyDown(event:CadKeyboardEvent,context:ToolContext):InputResult {
    if(event.repeat||event.editableTarget||event.isComposing||event.state?.modifiers.ctrl||event.state?.modifiers.meta||event.state?.modifiers.alt)return InputResult.Ignored;
    if(event.key==="Escape"){this.cancel(context);return InputResult.Ignored;}
    if(event.key==="Enter"){this.confirm(context);return InputResult.Consumed;}
    if(event.key==="Backspace"){this.back(context);return InputResult.Consumed;}
    if(this.phase.step==="VALUE"&&!this.status&&event.key.length===1&&/[a-zA-Z0-9_.+*/() -]/.test(event.key)){this.source=this.sourceEdited?this.source+event.key:event.key;this.sourceEdited=true;this.publish(context);return InputResult.Consumed;}
    return InputResult.Ignored;
  }
  deactivate(context:ToolContext):void{this.cancel(context);context.viewport.setSketchCommandState?.(undefined);}
  cancel(context:ToolContext):void{this.generation++;this.phase={step:"SELECTING",references:[]};this.scope=undefined;this.source="";this.status=undefined;this.error=undefined;this.session=new SketchDimensionCommitSession();this.cursor=undefined;this.anchor=undefined;this.capturedPointerID=undefined;context.viewport.clearReferencePreview();}
}
