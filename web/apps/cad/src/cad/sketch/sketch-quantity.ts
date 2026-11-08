import type {DisplayLengthUnit} from '../../state/ui-preferences';
import {parseParameterSource} from '../../features/workbench/parameter-editor';
import {CommandInputError} from '../command/operation-failure';

export type CreationQuantity={value:number;source:string};

// Explicit editing tools keep the original quantity source for authoritative evaluation.
export function creationQuantity(text:string,unit:DisplayLengthUnit|'deg',fallback:number):CreationQuantity|undefined {
 if(!text.trim())return;
 const parsed=parseParameterSource(text,unit);
 if(parsed.kind==='EXPRESSION'){
  if(!parsed.expression||/^[+\-.]+$/.test(parsed.expression))throw new CommandInputError('请输入完整的尺寸或表达式');
  return {value:fallback,source:parsed.expression};
 }
 const scales:Record<string,number>={mm:1,cm:10,m:1000,in:25.4,deg:1,rad:180/Math.PI};
 if((unit==='deg')!==['deg','rad'].includes(parsed.unit)||scales[parsed.unit]===undefined)throw new CommandInputError(unit==='deg'?'请输入角度或角度表达式':'请输入长度或长度表达式');
 return {value:parsed.value*scales[parsed.unit],source:`${parsed.value} ${parsed.unit}`};
}

