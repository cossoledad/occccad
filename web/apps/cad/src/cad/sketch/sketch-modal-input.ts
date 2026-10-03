import { InputResult, type CadInputSink, type CadKeyboardEvent, type CadPointerEvent, type CadWheelEvent } from "../input/input-types";
import type { SketchGeometryRef } from "../../types";
import type { SketchReferenceSelectionRequest } from "./sketch-reference-selection-session";

/** Owns viewport input while the dimension panel is visible. It deliberately
 * keeps the panel barrier after a reference pick finishes, including its up. */
export class SketchModalInputController implements CadInputSink {
 private open=false;
 private child?:{generation:number;request:SketchReferenceSelectionRequest};
 private generation=0;
 private readonly owners=new Map<number,"modal"|"navigation"|"fallback">();
 constructor(private readonly fallback:CadInputSink,private readonly navigation:CadInputSink,private readonly ports:{
  pick:(request:SketchReferenceSelectionRequest,event:CadPointerEvent)=>SketchGeometryRef|undefined;
  blocked?:()=>boolean;
  highlight?:(request:SketchReferenceSelectionRequest,reference:SketchGeometryRef|undefined)=>void;
 }){}
 get visible(){return this.open;}
 get selecting(){return !!this.child;}
 setOpen(open:boolean){
  if(this.open===open)return;
  this.open=open;
  if(open){this.fallback.cancel?.();for(const [id,owner] of this.owners)if(owner==="fallback")this.owners.set(id,"modal");}
  else {this.cancelChild();if(!this.ports.blocked?.())this.fallback.cancel?.();}
 }
 begin(request:SketchReferenceSelectionRequest):()=>void {
  this.cancelChild();this.setOpen(true);
  const generation=++this.generation;this.child={generation,request};
  return ()=>{if(this.child?.generation===generation)this.finishChild(false);};
 }
 private barred(){return this.open||!!this.ports.blocked?.();}
 private preview(reference:SketchGeometryRef|undefined){const child=this.child;if(!child)return;const accepted=reference&&(!child.request.allowed||child.request.allowed(reference))?reference:undefined;this.ports.highlight?.(child.request,accepted);child.request.onPreview(accepted);}
 private finishChild(cancelled:boolean){const child=this.child;if(!child)return;this.child=undefined;this.generation++;this.ports.highlight?.(child.request,undefined);child.request.onPreview(undefined);if(cancelled)child.request.onCancel();}
 private cancelChild(){this.finishChild(true);}
 pointerDown(event:CadPointerEvent):InputResult {
  if(this.barred()&&(event.state.buttons.middle||this.owners.get(event.pointerId)==="navigation")){
   const previous=this.owners.get(event.pointerId);if(previous==="fallback")this.fallback.pointerCancel?.({...event,phase:"cancel"});
   this.owners.set(event.pointerId,"navigation");return this.navigation.pointerDown?.(event)??InputResult.Consumed;
  }
  if(!this.barred()){const result=this.fallback.pointerDown?.(event)??InputResult.Ignored;if(result!==InputResult.Ignored)this.owners.set(event.pointerId,this.barred()?"modal":"fallback");return result;}
  this.owners.set(event.pointerId,"modal");
  if(event.button===2){this.cancelChild();return InputResult.Consumed;}
  if(event.button===0&&this.child){
   const child=this.child,candidate=this.ports.pick(child.request,event);
   if(candidate&&(!child.request.allowed||child.request.allowed(candidate))&&child.request.onCandidate(candidate)&&this.child===child)this.finishChild(false);
  }
  return event.button===0?InputResult.Capture:InputResult.Consumed;
 }
 pointerMove(event:CadPointerEvent):InputResult {
  const owner=this.owners.get(event.pointerId);
  if(owner==="navigation")return this.navigation.pointerMove?.(event)??InputResult.Consumed;
  if(owner==="fallback")return this.barred()?InputResult.Consumed:(this.fallback.pointerMove?.(event)??InputResult.Ignored);
  if(this.barred()||owner==="modal"){if(this.open&&this.child&&!event.state.buttons.middle)this.preview(this.ports.pick(this.child.request,event));return InputResult.Consumed;}
  return this.fallback.pointerMove?.(event)??InputResult.Ignored;
 }
 pointerUp(event:CadPointerEvent):InputResult {
  const owner=this.owners.get(event.pointerId);
  if(!event.state.buttons.left&&!event.state.buttons.middle&&!event.state.buttons.right)this.owners.delete(event.pointerId);
  if(owner==="navigation")return this.navigation.pointerUp?.(event)??InputResult.ReleaseCapture;
  if(owner==="fallback")return this.fallback.pointerUp?.(event)??InputResult.Ignored;
  if(owner==="modal"||this.barred())return InputResult.ReleaseCapture;
  // An up without its down is never delivered to the newly active tool.
  return InputResult.Consumed;
 }
 pointerCancel(event:CadPointerEvent):InputResult {
  const owner=this.owners.get(event.pointerId);this.owners.delete(event.pointerId);
  if(owner==="navigation")this.navigation.pointerCancel?.(event);
  if(owner==="fallback")this.fallback.pointerCancel?.(event);
  if(this.open)this.cancelChild();return InputResult.ReleaseCapture;
 }
 wheel(event:CadWheelEvent):InputResult{return (this.barred()?this.navigation:this.fallback).wheel?.(event)??InputResult.Ignored;}
 auxiliaryClick(event:MouseEvent):InputResult{return this.barred()?(this.navigation.auxiliaryClick?.(event)??InputResult.Consumed):(this.fallback.auxiliaryClick?.(event)??InputResult.Ignored);}
 modalKeyDown=(event:KeyboardEvent):InputResult=>{
  if(!this.open||event.isComposing||event.keyCode===229)return InputResult.Ignored;
  if(event.key==="Escape"&&this.child){this.cancelChild();return InputResult.Consumed;}
  return InputResult.Ignored;
 };
 modalKeyUp=(_event:KeyboardEvent):InputResult=>InputResult.Ignored;
 keyDown(event:CadKeyboardEvent):InputResult {
  if(!this.barred())return this.fallback.keyDown?.(event)??InputResult.Ignored;
  if(event.isComposing||event.editableTarget)return InputResult.Ignored;
  if(event.key==="Escape"){if(this.child){this.cancelChild();return InputResult.Consumed;}if(!this.open)return this.fallback.keyDown?.(event)??InputResult.Consumed;return InputResult.Ignored;}
  if(["Shift","Control","Alt","Meta"].includes(event.key))this.navigation.keyDown?.(event);
  return InputResult.Consumed;
 }
 keyUp(event:CadKeyboardEvent):InputResult {
  if(!this.barred())return this.fallback.keyUp?.(event)??InputResult.Ignored;
  if(event.isComposing||event.editableTarget)return InputResult.Ignored;
  this.navigation.keyUp?.(event);return InputResult.Consumed;
 }
 cancel(){this.owners.clear();this.cancelChild();this.navigation.cancel?.();if(!this.barred())this.fallback.cancel?.();}
}
