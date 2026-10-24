import {Button,Tag,Typography} from 'antd';
import {CloseOutlined} from '@ant-design/icons';
import type {AssemblyGroupMember,DocumentView} from '../../types';
import {describeAssemblyReference} from '../../cad/assembly/assembly-reference-presentation';

// Selection is owned by the viewport/tree session. The form manages its ordered
// member references without turning the product into a dropdown catalog.
export function AssemblyGroupMemberList({value=[],onChange,view,replacing,onLocate,onReplace}:{value?:string[];onChange?:(value:string[])=>void;view?:DocumentView;replacing?:string;onLocate?:(member:AssemblyGroupMember)=>void;onReplace?:(token:string)=>void}){
 return <div className="assembly-group-members" aria-label="固联组成员">
  {!value.length&&<Typography.Text type="secondary">选择零件或固联组</Typography.Text>}
  {value.map((token,index)=>{
   const group=token.startsWith('group:'),id=token.slice(group?6:9),member=group?{groupId:id}:{instanceId:id};
   const target=group?view?.product?.constraints?.find(c=>c.id===id&&c.kind==='FIX_TOGETHER'):view?.product?.instances.find(i=>i.id===id);
   const presentation=group?{primary:target?.name??"引用固联组已丢失",secondary:'固联组'}:describeAssemblyReference({instanceId:id,kind:'BODY'},view);
   return <div key={token} className="assembly-support-row">
    <span className={`assembly-support-index status-${target?'connected':'not_connected'}`}>{index+1}</span>
    <span className="assembly-support-detail"><strong title={presentation.primary}>{presentation.primary}</strong><small>{presentation.secondary}</small><small><Tag color={target?'success':'error'}>{target?'有效':'引用已丢失'}</Tag></small></span>
    <div className="assembly-support-actions assembly-group-member-actions"><Button size="small" disabled={!target} onClick={()=>onLocate?.(member)}>定位</Button><Button size="small" type={replacing===token?'primary':'default'} onClick={()=>onReplace?.(token)}>更换</Button><Button size="small" type="text" icon={<CloseOutlined/>} title="移除" aria-label={`移除 ${presentation.primary}`} onClick={()=>onChange?.(value.filter(member=>member!==token))}/></div>
   </div>;
  })}
 </div>;
}
