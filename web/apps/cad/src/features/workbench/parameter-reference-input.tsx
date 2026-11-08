import {AutoComplete,Input,type InputRef} from "antd";
import {useState,type FocusEventHandler,type Ref} from "react";
import type {ParameterDefinition} from "../../types";
import {parameterCompletions,parameterDisplayValue} from "./parameter-editor";

/** A single expression field. Parameter completion replaces only the word at
 * the caret; choosing a reference never opens a second input or writes a value. */
export function ParameterReferenceInput({id,value="",onChange,parameters,excludeId,disabled,placeholder,unit,onBlur,autoFocus,inputRef,onFocus,label,onConfirm,compact}:{
 id?:string;value?:string;onChange?:(value:string)=>void;parameters:ParameterDefinition[];excludeId?:string;disabled?:boolean;placeholder?:string;unit?:"mm"|"cm"|"m"|"in";onBlur?:()=>void;
 autoFocus?:boolean;inputRef?:Ref<InputRef>;onFocus?:FocusEventHandler<HTMLInputElement>;label?:string;onConfirm?:()=>void;compact?:boolean;
}) {
 const [caret,setCaret]=useState<{source:string;at:number}>();
 const options=parameterCompletions(value,parameters,excludeId,caret?.source===value?caret.at:value.length).map(({parameter:p,value})=>({value,label:`${p.displayAlias??p.key} · ${parameterDisplayValue(p,unit)}`}));
 return <AutoComplete value={value} options={options} disabled={disabled} onChange={onChange} onBlur={onBlur} popupMatchSelectWidth={false} variant={compact?'borderless':undefined} style={{minWidth:0,width:'100%'}}>
  <Input ref={inputRef} disabled={disabled} autoFocus={autoFocus} variant={compact?'borderless':undefined} size={compact?'small':undefined} onFocus={onFocus} onSelect={event=>setCaret({source:event.currentTarget.value,at:event.currentTarget.selectionStart??value.length})} aria-label={label} id={id} placeholder={placeholder??"数值或公式"} data-quantity-input="true" onPressEnter={event=>{if(!event.nativeEvent.isComposing&&!options.length){event.preventDefault();event.stopPropagation();if(onConfirm)onConfirm();else event.currentTarget.blur();}}}/>
 </AutoComplete>;
}
