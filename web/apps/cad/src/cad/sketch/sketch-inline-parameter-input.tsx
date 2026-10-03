import {inlineEditorPosition,inlineParameterPrefix} from "../../utils/inline-editor-position";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Button, Input, type InputRef } from "antd";
import type { SketchCommandAction, SketchCommandState } from "../tool/sketch-command-session";
import type { DisplayLengthUnit } from "../../state/ui-preferences";

export function defaultSketchToolMode(_toolId:string,doubleClick=false):"once"|"continuous" {
 return doubleClick?"continuous":"once";
}

export function SketchParameterValueInput({field,pending,onChange,inputRef}:{field:SketchCommandState["fields"][number];pending:boolean;onChange:(value:string)=>void;inputRef?:React.Ref<InputRef>}) {
 return <Input ref={inputRef} autoFocus variant="borderless" size="small" aria-label={field.label} value={field.value} placeholder={field.placeholder??"值或表达式"} disabled={pending}
 onChange={event=>onChange(event.target.value)}/>;
}

/** Parameter annotation, not a command card. Editing affects only the actor's
 * existing draft; blur never confirms and Escape still cancels the command. */
export function SketchInlineParameterInput({state,lengthUnit="mm",onAction,onEditing}:{state:SketchCommandState;lengthUnit?:DisplayLengthUnit;onAction:(action:SketchCommandAction)=>void;onEditing?:(editing:boolean)=>void}) {
 const host=useRef<HTMLDivElement>(null);
 const [position,setPosition]=useState({x:12,y:12});
 const frozenAnchor=useRef<{id:string;anchor?:[number,number]}>({id:""});
 const input=useRef<InputRef>(null),value=useRef<HTMLButtonElement>(null),confirmed=useRef(false),cursor=useRef<"all"|"end">("all");
 const [editing,setEditing]=useState(!!state.input);
 const drag=useRef<{pointer:number;x:number;y:number;anchor:[number,number];moved:boolean}|undefined>(undefined);
 const suppressClick=useRef(false);
 const request=state.input,field=request?state.fields[request.fieldIndex]:undefined;
 const editingPort=useRef(onEditing);editingPort.current=onEditing;
 useEffect(()=>{editingPort.current?.(editing);return ()=>editingPort.current?.(false);},[editing,request?.id]);
 const pending=state.phase==="committing"||state.phase==="unknown";
 useEffect(()=>{setEditing(!!request);confirmed.current=false;},[request?.id]);
 useEffect(()=>{if(request&&!pending){if(editing)input.current?.focus({cursor:cursor.current,preventScroll:true});else value.current?.focus({preventScroll:true});}},[request?.id,request?.fieldIndex,editing]);
 useEffect(()=>{confirmed.current=false;},[state.phase,field?.value,state.error]);
 useLayoutEffect(()=>{
  const element=host.current,parent=element?.parentElement;if(!element||!parent||!request)return;
  const anchorKey=`${request.id}:${request.fieldIndex}:${editing}`;
  if(frozenAnchor.current.id!==anchorKey)frozenAnchor.current={id:anchorKey,anchor:request.anchor?[...request.anchor]:undefined};
  const measure=()=>{
   const rect=element.getBoundingClientRect(),viewport=parent.getBoundingClientRect();
   const anchor=editing?frozenAnchor.current.anchor:request.anchor;
   const editor=element.querySelector<HTMLElement>(".cad-inline-editor")?.getBoundingClientRect();
   const next=inlineEditorPosition(anchor??[12,viewport.height-12],{width:rect.width,height:editor?.height??rect.height},viewport,editing);
   // Error text extends below the input without moving its anchor; only clamp
   // the complete measured group when it would leave the viewport.
   next.y=Math.max(12,Math.min(next.y,viewport.height-rect.height-12));
   setPosition(previous=>previous.x===next.x&&previous.y===next.y?previous:next);
  };
  measure();const observer=new ResizeObserver(measure);observer.observe(element);observer.observe(parent);return ()=>observer.disconnect();
 },[request?.id,request?.fieldIndex,request?.anchor?.[0],request?.anchor?.[1],editing,state.error]);
 if(state.presentation!=="inline"||!request||!field)return null;
 const confirm=()=>{if(confirmed.current||pending||!state.canConfirm)return;confirmed.current=true;onAction({type:"confirm"});};
 const unit=field.displayUnit??(field.unit==="length"?lengthUnit:field.unit==="angle"?"deg":undefined);
 const suffix=unit&&!/\b(mm|cm|m|in|deg)\b/.test(field.value)?` ${unit}`:"";
 return <div ref={host} className="cad-inline-annotation" data-cad-parameter-annotation="true" role="group" aria-label={`${state.operation}参数`}
  style={{left:position.x,top:position.y}}
  onPointerDown={event=>{
   event.stopPropagation();
   if(event.button===0&&!(event.target as HTMLElement).closest("input")&&!pending&&request.dimension&&request.anchor){
    drag.current={pointer:event.pointerId,x:event.clientX,y:event.clientY,anchor:[...(editing?frozenAnchor.current.anchor??request.anchor:request.anchor)],moved:false};event.currentTarget.setPointerCapture(event.pointerId);
   }
  }} onPointerMove={event=>{
   const gesture=drag.current;if(!gesture||gesture.pointer!==event.pointerId)return;
   event.stopPropagation();const dx=event.clientX-gesture.x,dy=event.clientY-gesture.y;
   if(Math.hypot(dx,dy)>3)gesture.moved=true;
   if(gesture.moved){
    const anchor:[number,number]=[gesture.anchor[0]+dx,gesture.anchor[1]+dy];
    frozenAnchor.current.anchor=anchor;
    onAction({type:"placement",position:anchor});
   }
  }} onPointerUp={event=>{
   event.stopPropagation();if(drag.current?.pointer!==event.pointerId)return;
   suppressClick.current=drag.current.moved;drag.current=undefined;
   if(event.currentTarget.hasPointerCapture(event.pointerId))event.currentTarget.releasePointerCapture(event.pointerId);
  }} onPointerCancel={()=>{drag.current=undefined;}}
  onLostPointerCapture={()=>{drag.current=undefined;}}
  onKeyDownCapture={event=>{
   if(event.nativeEvent.isComposing||event.nativeEvent.keyCode===229||event.repeat)return;
   if(event.key==="Escape"){event.preventDefault();event.stopPropagation();onAction({type:"cancel"});}
   else if(event.key==="Enter"){event.preventDefault();event.stopPropagation();confirm();}
   else if(event.key==="Tab"&&state.fields.length>1){event.preventDefault();event.stopPropagation();onAction({type:"input",index:(request.fieldIndex+(event.shiftKey?state.fields.length-1:1))%state.fields.length});}
   else if(!editing&&!pending&&!event.ctrlKey&&!event.metaKey&&!event.altKey&&event.key.length===1){event.preventDefault();event.stopPropagation();cursor.current="end";setEditing(true);onAction({type:"field",index:request.fieldIndex,value:event.key});}
  }}>
  {editing?<div className="cad-inline-editor">
    <span className="cad-inline-prefix" aria-label={request.dimension?"拖动尺寸标注":field.label} style={{cursor:request.dimension?"move":"default"}}>{inlineParameterPrefix(field.label)}</span>
    <SketchParameterValueInput field={field} pending={pending} inputRef={input} onChange={value=>onAction({type:"field",index:request.fieldIndex,value})}/>
    {suffix&&<small>{suffix.trim()}</small>}
  </div>:<Button ref={value} type="text" size="small" disabled={pending} className={request.dimension?"cad-inline-hit":"cad-inline-value"} onClick={()=>{if(suppressClick.current){suppressClick.current=false;return;}cursor.current="all";setEditing(true);}} aria-label={`编辑${field.label} ${field.value}${suffix}`}>{field.label} {field.value}{suffix}</Button>}
  {state.error&&<div className="cad-inline-error" role="alert">{state.error}</div>}
 </div>;
}
