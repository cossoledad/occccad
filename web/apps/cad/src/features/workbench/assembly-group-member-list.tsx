import {Button,Typography} from 'antd';
import type {DocumentView} from '../../types';

// Selection is owned by the viewport/tree session. The form manages its ordered
// member references without turning the product into a dropdown catalog.
export function AssemblyGroupMemberList({value=[],onChange,view}:{value?:string[];onChange?:(value:string[])=>void;view?:DocumentView}){
 return <div className="assembly-group-members" aria-label="固联组成员">
  {!value.length&&<Typography.Text type="secondary">选择零件或固联组</Typography.Text>}
  {value.map(token=>{const [kind,id]=token.split(':');const name=kind==='group'?view?.product?.constraints?.find(c=>c.id===id)?.name:view?.product?.instances.find(i=>i.id===id)?.name;return <div key={token} className="assembly-support-row"><Typography.Text>{name??id}</Typography.Text><Button size="small" aria-label={`移除 ${name??id}`} onClick={()=>onChange?.(value.filter(member=>member!==token))}>移除</Button></div>})}
 </div>;
}
