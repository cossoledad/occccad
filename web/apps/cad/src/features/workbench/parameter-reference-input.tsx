import {AutoComplete,Input} from "antd";
import type {ParameterDefinition} from "../../types";
import {parameterDisplayValue} from "./parameter-editor";

/** A single expression field. Parameter completion replaces only the word at
 * the caret; choosing a reference never opens a second input or writes a value. */
export function ParameterReferenceInput({id,value="",onChange,parameters,excludeId,disabled,placeholder,unit,onBlur}:{
 id?:string;value?:string;onChange?:(value:string)=>void;parameters:ParameterDefinition[];excludeId?:string;disabled?:boolean;placeholder?:string;unit?:"mm"|"cm"|"m"|"in";onBlur?:()=>void;
}) {
 const word=value.match(/[A-Za-z_][A-Za-z0-9_]*$/)?.[0];
 const options=word?parameters.filter(p=>p.parameterId!==excludeId&&p.key.toLowerCase().startsWith(word.toLowerCase())&&p.key!==word).map(p=>({value:value.slice(0,-word.length)+p.key,label:`${p.displayAlias??p.key} · ${parameterDisplayValue(p,unit)}`})):[];
 return <AutoComplete value={value} options={options} disabled={disabled} onChange={onChange} onBlur={onBlur} popupMatchSelectWidth={false}>
  <Input id={id} placeholder={placeholder??"数值或公式"} data-quantity-input="true" onPressEnter={event=>{if(!event.nativeEvent.isComposing&&!options.length){event.preventDefault();event.stopPropagation();event.currentTarget.blur();}}}/>
 </AutoComplete>;
}
