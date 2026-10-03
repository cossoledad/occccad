import { useEffect, useRef, useState } from "react";
import { Button, Input, Typography, type InputRef } from "antd";
import type { SketchCommandAction, SketchCommandState } from "../tool/sketch-command-session";
import type { DisplayLengthUnit } from "../../state/ui-preferences";

export function defaultSketchToolMode(_toolId:string,doubleClick=false):"once"|"continuous" {
 return doubleClick?"continuous":"once";
}

export function SketchParameterValueInput({field,pending,onChange,inputRef}:{field:SketchCommandState["fields"][number];pending:boolean;onChange:(value:string)=>void;inputRef?:React.Ref<InputRef>}) {
 return <Input ref={inputRef} autoFocus variant="borderless" size="small" aria-label={field.label} value={field.value} placeholder={field.placeholder??"值或表达式"} disabled={pending}
  style={{width:160,padding:0,color:"inherit",background:"transparent"}} onChange={event=>onChange(event.target.value)}/>;
}

/** Parameter annotation, not a command card. Editing affects only the actor's
 * existing draft; blur never confirms and Escape still cancels the command. */
export function SketchInlineParameterInput({state,lengthUnit="mm",onAction}:{state:SketchCommandState;lengthUnit?:DisplayLengthUnit;onAction:(action:SketchCommandAction)=>void}) {
 const input=useRef<InputRef>(null),value=useRef<HTMLButtonElement>(null),confirmed=useRef(false),cursor=useRef<"all"|"end">("all");
 const [editing,setEditing]=useState(false);
 const request=state.input,field=request?state.fields[request.fieldIndex]:undefined;
 const pending=state.phase==="committing"||state.phase==="unknown";
 useEffect(()=>{setEditing(false);confirmed.current=false;},[request?.id]);
 useEffect(()=>{if(request&&!pending){if(editing)input.current?.focus({cursor:cursor.current,preventScroll:true});else value.current?.focus({preventScroll:true});}},[request?.id,request?.fieldIndex,editing]);
 useEffect(()=>{confirmed.current=false;},[state.phase,field?.value,state.error]);
 if(state.presentation!=="inline"||!request||!field)return null;
 const confirm=()=>{if(confirmed.current||pending||!state.canConfirm)return;confirmed.current=true;onAction({type:"confirm"});};
 const unit=field.displayUnit??(field.unit==="length"?lengthUnit:field.unit==="angle"?"deg":undefined);
 const suffix=unit&&!/\b(mm|cm|m|in|deg)\b/.test(field.value)?` ${unit}`:"";
 return <div data-cad-parameter-annotation="true" role="group" aria-label={`${state.operation}参数`}
  style={{position:"absolute",...(request.anchor?{left:`clamp(8px, ${request.anchor[0]+32}px, calc(100% - 190px))`,top:`clamp(8px, ${request.anchor[1]-32}px, calc(100% - 60px))`}:{left:12,bottom:12}),maxWidth:180,zIndex:10,color:"#175e40"}}
  onPointerDown={event=>event.stopPropagation()} onPointerUp={event=>event.stopPropagation()}
  onKeyDownCapture={event=>{
   if(event.nativeEvent.isComposing||event.nativeEvent.keyCode===229||event.repeat)return;
   if(event.key==="Escape"){event.preventDefault();event.stopPropagation();onAction({type:"cancel"});}
   else if(event.key==="Enter"){event.preventDefault();event.stopPropagation();confirm();}
   else if(event.key==="Tab"&&state.fields.length>1){event.preventDefault();event.stopPropagation();onAction({type:"input",index:(request.fieldIndex+(event.shiftKey?state.fields.length-1:1))%state.fields.length});}
   else if(!editing&&!pending&&!event.ctrlKey&&!event.metaKey&&!event.altKey&&event.key.length===1){event.preventDefault();event.stopPropagation();cursor.current="end";setEditing(true);onAction({type:"field",index:request.fieldIndex,value:event.key});}
  }}>
  {request.anchor&&<svg aria-hidden="true" width="36" height="34" style={{position:"absolute",left:-32,top:10,pointerEvents:"none",overflow:"visible"}}><path d="M0 24 L22 0 H36" fill="none" stroke="currentColor" strokeWidth="1"/></svg>}
  {editing?<SketchParameterValueInput field={field} pending={pending} inputRef={input} onChange={value=>onAction({type:"field",index:request.fieldIndex,value})}/>
    :<Button ref={value} type="text" size="small" disabled={pending} aria-label={`编辑${field.label}`} style={{padding:0,height:"auto",color:"inherit",textShadow:"0 0 3px white"}} onClick={()=>{cursor.current="all";setEditing(true);}}>{field.label} {field.value}{suffix}</Button>}
  {state.error&&<Typography.Text role="alert" type="danger" style={{display:"block",fontSize:12}}>{state.error}</Typography.Text>}
 </div>;
}
