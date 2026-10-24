import {FeaturePickField} from "./feature-pick-field";
import type {AssemblyGroupMember,DocumentView} from '../../types';
import {describeAssemblyReference} from '../../cad/assembly/assembly-reference-presentation';

// Selection is owned by the viewport/tree session. The form manages its ordered
// member references without turning the product into a dropdown catalog.
export function AssemblyGroupMemberList({value=[],onChange,view,replacing,onLocate,onReplace,picking,onActivate}:{picking?:boolean;onActivate?:()=>void;value?:string[];onChange?:(value:string[])=>void;view?:DocumentView;replacing?:string;onLocate?:(member:AssemblyGroupMember)=>void;onReplace?:(token:string)=>void}){
 return <div className="assembly-group-members" aria-label="固联组成员">
  <FeaturePickField mode="multiple" label="成员集" value={value.length?`已选择 ${value.length} 个成员`:"选择零件或固联组"} selected={!!value.length} active={!!picking&&!replacing} onActivate={onActivate} onClear={value.length?()=>onChange?.([]):undefined}/>
  {value.map((token,index)=>{
   const group=token.startsWith('group:'),id=token.slice(group?6:9),member=group?{groupId:id}:{instanceId:id};
   const target=group?view?.product?.constraints?.find(c=>c.id===id&&c.kind==='FIX_TOGETHER'):view?.product?.instances.find(i=>i.id===id);
   const presentation=group?{primary:target?.name??"引用固联组已丢失",secondary:'固联组'}:describeAssemblyReference({instanceId:id,kind:'BODY'},view);
   return <FeaturePickField key={token} label={`成员 ${index+1}`} value={presentation.primary} detail={<>{presentation.secondary} · {target?'有效':'引用已丢失'}</>} selected active={replacing===token} onActivate={()=>onReplace?.(token)} onLocate={target?()=>onLocate?.(member):undefined} onClear={()=>onChange?.(value.filter(member=>member!==token))}/>;
  })}
 </div>;
}
