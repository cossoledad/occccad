import type { SketchEntity, SketchGeometryRef } from "../../types";
import type { SketchReferencePickKind } from "../interaction/sketch-reference-pick";

export type SketchReferenceSelectionRequest = {
 featureId:string; slot:number; pick:SketchReferencePickKind; retained?:SketchEntity;
 references:readonly SketchGeometryRef[];
 allowed?:(candidate:SketchGeometryRef)=>boolean;
 onCandidate:(candidate:SketchGeometryRef)=>boolean;
 onPreview:(candidate:SketchGeometryRef|undefined)=>void;
 onCancel:()=>void;
};
export type SketchReferenceSelectionBindings = {
 onSelectReference?:(request:SketchReferenceSelectionRequest)=>()=>void;
 onHighlightReference?:(featureId:string,reference:SketchGeometryRef|undefined,slot?:number)=>void;
 onLocateReference?:(featureId:string,reference:SketchGeometryRef,slot?:number)=>void;
};
/** A replacement gesture owns its generation and version; it never mutates a dimension draft. */
export class SketchReferenceSelectionSession {
 private generation=0;
 private current?:{token:number;slot:number;version:string};
 constructor(private readonly callbacks:{version:()=>string;validate:(candidate:SketchGeometryRef,slot:number)=>boolean;onAccept:(candidate:SketchGeometryRef,slot:number)=>void;onHighlight:(candidate:SketchGeometryRef|undefined)=>void;onCancel:()=>void}){}
 get active(){return !!this.current;}
 begin(slot:number){this.cancel();const token=++this.generation;this.current={token,slot,version:this.callbacks.version()};return token;}
 private valid(token:number){if(this.current?.token!==token)return false;if(this.current.version!==this.callbacks.version()){this.cancel();return false;}return true;}
 preview(token:number,candidate:SketchGeometryRef|undefined){if(!this.valid(token))return;this.callbacks.onHighlight(candidate&&this.callbacks.validate(candidate,this.current!.slot)?candidate:undefined);}
 accept(token:number,candidate:SketchGeometryRef){if(!this.valid(token)||!this.callbacks.validate(candidate,this.current!.slot))return false;const slot=this.current!.slot;this.current=undefined;this.callbacks.onHighlight(undefined);this.callbacks.onAccept(candidate,slot);return true;}
 cancel(){const existed=!!this.current;this.current=undefined;this.generation++;this.callbacks.onHighlight(undefined);if(existed)this.callbacks.onCancel();}
}
