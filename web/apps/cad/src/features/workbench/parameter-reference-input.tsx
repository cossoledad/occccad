import {Input,Select} from "antd";
import {useRef} from "react";
import type {InputRef} from "antd";
import type {ParameterDefinition} from "../../types";
import {parameterDisplayValue,insertParameterReference} from "./parameter-editor";

export function ParameterReferenceInput({value="",onChange,parameters,excludeId,disabled,placeholder,unit}:{
 value?:string;onChange?:(value:string)=>void;parameters:ParameterDefinition[];excludeId?:string;disabled?:boolean;placeholder?:string;unit?:"mm"|"cm"|"m"|"in";
}) {
 const input=useRef<InputRef>(null),caret=useRef<{start:number;end:number}|undefined>(undefined);
 return <div className="parameter-reference-input">
  <Input ref={input} value={value} disabled={disabled} placeholder={placeholder} data-quantity-input="true"
   onChange={event=>onChange?.(event.target.value)} onBlur={()=>{const field=input.current?.input;caret.current=field?{start:field.selectionStart??value.length,end:field.selectionEnd??value.length}:undefined;}}/>
  <Select aria-label="插入参数引用" showSearch optionFilterProp="label" placeholder="插入参数引用" value={undefined} disabled={disabled}
   options={parameters.filter(p=>p.parameterId!==excludeId).map(p=>({value:p.parameterId,label:`${p.displayAlias??p.key} · ${p.qualifiedDisplayPath??p.label} · ${parameterDisplayValue(p,unit)}`}))}
   onChange={id=>{const parameter=parameters.find(p=>p.parameterId===id);if(!parameter)return;const text=insertParameterReference(value,parameter.key,caret.current);onChange?.(text);input.current?.focus();}}/>
 </div>;
}
