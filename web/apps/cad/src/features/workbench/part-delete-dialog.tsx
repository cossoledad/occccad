import {Button,Checkbox,Tabs,Input,Pagination} from 'antd';
import {useEffect,useRef,useState} from 'react';
import {api} from '../../api/client';
import type {DeleteImpactItem,DeleteOptions,DocumentView,PartDeletePlan} from '../../types';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {useOperationFailureNotification} from '../../cad/command/operation-feedback';
import type {ObjectDeleteTarget} from './object-operations';

function ImpactList({items,onLocate}:{items:DeleteImpactItem[];onLocate:(items:DeleteImpactItem[])=>void}) {
 const [search,setSearch]=useState(''),[page,setPage]=useState(1);
 const filtered=items.filter(i=>!search||i.name.toLocaleLowerCase().includes(search.toLocaleLowerCase()));
 const current=Math.min(page,Math.max(1,Math.ceil(filtered.length/50)));
 return <>{items.length>50&&<Input aria-label="筛选影响对象" placeholder="查找对象" value={search} onChange={e=>{setSearch(e.target.value);setPage(1)}}/>}<div className="object-impact-list">{filtered.slice((current-1)*50,current*50).map(item=><Button type="text" key={item.kind+item.id} onClick={()=>onLocate([item])}><span>{item.name||item.id}</span><small>{item.kind==='BODY'?'实体':item.kind==='GEOMETRICAL_SET'?'几何图形集':item.kind==='PUBLICATION'?'发布':item.kind==='CONTEXT_INPUT'?'输入接口':item.kind==='MEASUREMENT'?'测量':item.state==='DELETE'?'删除':item.state==='INPUT_MISSING'?'输入将缺失':item.state==='BLOCKED'?'需要上游修复':'重新计算'}</small></Button>)}</div>{filtered.length>50&&<Pagination size="small" current={current} pageSize={50} total={filtered.length} showSizeChanger={false} onChange={setPage}/>}</>;
}
export function PartDeleteDialog({view,targets,onClose,onApply,onLocate}:{view:DocumentView;targets:ObjectDeleteTarget[];onClose:()=>void;onApply:(plan:PartDeletePlan)=>Promise<unknown>;onLocate:(items:DeleteImpactItem[])=>void}){
 const base=useRef(view.document.versionId).current;
 const [options,setOptions]=useState<DeleteOptions>({}),[plan,setPlan]=useState<PartDeletePlan>(),[busy,setBusy]=useState(true),[saving,setSaving]=useState(false),[failure,setFailure]=useState<unknown>(),[attempt,setAttempt]=useState(0);
 const valid=base===view.document.versionId;
 useOperationFailureNotification(failure,'删除对象',{retry:()=>setAttempt(n=>n+1),retryLabel:'重试'});
 useEffect(()=>{if(!valid){setPlan(undefined);return};const abort=new AbortController();setBusy(true);setPlan(undefined);setFailure(undefined);
  void api.getPartDeletePlan(view.document.id,{versionId:base,targets,options},abort.signal).then(value=>{if(!abort.signal.aborted)setPlan(value)}).catch(error=>{if(!abort.signal.aborted)setFailure(error)}).finally(()=>{if(!abort.signal.aborted)setBusy(false)});return()=>abort.abort();
 },[base,valid,view.document.id,targets,options,attempt]);
 const deletedObjects=plan?.deleted.filter(item=>item.kind!=='PARAMETER')??[];
 const rows=(items:DeleteImpactItem[])=> <ImpactList items={items} onLocate={onLocate}/>;
 return <CommandDialog id="part-delete" open title="删除对象" size="M" onClose={onClose} confirmLoading={saving} confirmDisabled={busy||!plan||!valid} onConfirm={async()=>{if(!plan||!valid)return;setSaving(true);try{await onApply(plan);onClose()}finally{setSaving(false)}}}>
  <div className="command-form">
   <Checkbox disabled={saving} checked={!!options.deleteDependents} onChange={e=>setOptions(o=>({...o,deleteDependents:e.target.checked}))}>同时删除依赖特征</Checkbox>
   <Checkbox disabled={saving} checked={!!options.deleteExclusiveParents} onChange={e=>setOptions(o=>({...o,deleteExclusiveParents:e.target.checked}))}>清理独占的草图与参考几何</Checkbox>
   {!valid?<span className="command-field-error">模型已改变，请重新打开删除。</span>:busy?<span>正在分析影响…</span>:plan&&<>
    <Tabs items={[{key:'delete',label:`删除 ${deletedObjects.length}`,children:rows(deletedObjects)},{key:'affected',label:`保留并更新 ${plan.affected.length}`,children:rows(plan.affected)},...(plan.publications.length?[{key:'publications',label:`受影响接口 ${plan.publications.length}`,children:rows(plan.publications)}]:[])]}/>
    <Button size="small" onClick={()=>onLocate(deletedObjects)}>定位删除范围</Button>
   </>}
  </div>
 </CommandDialog>;
}
