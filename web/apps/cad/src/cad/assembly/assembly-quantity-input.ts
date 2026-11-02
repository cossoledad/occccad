import {parseParameterSource} from '../../features/workbench/parameter-editor';
import type {AssemblyQuantityFields} from './assembly-quantity';

export type AssemblyQuantityDraft=AssemblyQuantityFields&{value?:number;quantityInput?:string;distanceRelation?:string};
/** A single editable source, compiled once for both Preview and commit. */
export function assemblyQuantityInputFields(kind:string,fields:AssemblyQuantityDraft,unit:string):AssemblyQuantityDraft {
 if(fields.quantityInput===undefined||!['DISTANCE','ANGLE'].includes(kind))return fields;
 const input=parseParameterSource(fields.quantityInput,kind==='ANGLE'?'deg':unit);
 if(input.kind==='EXPRESSION'){
  if(!input.expression)throw new Error('请输入数值或公式');
  return {...fields,quantityExpression:input.expression};
 }
 const scales:Record<string,number>=kind==='ANGLE'?{deg:1,rad:180/Math.PI}:{mm:1,cm:10,m:1000,in:25.4};
 if(!(input.unit in scales))throw new Error(kind==='ANGLE'?'请输入角度单位 deg 或 rad':'请输入长度单位 mm、cm、m 或 in');
 const value=input.value*scales[input.unit]/(kind==='ANGLE'?1:scales[unit]);
 if(!Number.isFinite(value))throw new Error('请输入有限数值');
 if(kind==='DISTANCE'&&(fields.distanceRelation??'UNSIGNED')==='UNSIGNED'&&value<0)throw new Error('无符号距离不能为负');
 if(kind==='ANGLE'&&(value<0||value>360))throw new Error('角度应在 0° 到 360° 之间');
 return {...fields,value,quantityExpression:''};
}
