import {Alert,Button,Checkbox,Empty,Select,Space,Typography} from 'antd';
import {useMemo,useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import type {DocumentView,SelectionItem} from '../../types';
import {api} from '../../api/client';
import {commandForView,viewContextKey} from '../../api/context-edit-scope';
import {engineeringProblem} from '../../cad/command/operation-feedback';
import {useFeatureDialogFeedback} from './use-feature-dialog-feedback';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandStatus} from '../../cad/overlay/command-form';
import {GraphView} from '../../components/graph-view';
import {dependencyStateLabel,dependencyTypeLabel,dependencyWarning,partDependencySlice,partDependencySnapshotMatches,type DependencyDirection,type PartDependencyNode} from '../../cad/part/part-dependency-graph';

export function PartDependencyPanel({view,selection,canEdit,comparisonAllowed,onCompare,onClose,onLocate,onEdit}:{view:DocumentView;selection?:SelectionItem|null;canEdit:boolean;comparisonAllowed:boolean;onCompare:(node:PartDependencyNode,revision:string)=>void;onClose:()=>void;onLocate:(node:PartDependencyNode,revision:string)=>void;onEdit:(node:PartDependencyNode,revision:string)=>void}){
 const request=commandForView(view,{versionId:view.document.versionId}),scope=viewContextKey(view);
 const query=useQuery({queryKey:['part-dependencies',scope],queryFn:({signal})=>api.getPartDependencies(view.document.id,request,signal),staleTime:Infinity});
 useFeatureDialogFeedback(query.error,'零件依赖',undefined,undefined,()=>{void query.refetch()});
 const graph=query.data&&partDependencySnapshotMatches(view,query.data)?query.data:undefined;
 const [chosen,setChosen]=useState<string>(),[selected,setSelected]=useState<string>(),[direction,setDirection]=useState<DependencyDirection>('both'),[limit,setLimit]=useState(200),[display,setDisplay]=useState(false),[layout,setLayout]=useState(0);
 const initial=graph?.nodes.find(n=>n.entityId===(selection?.kind==='parameter'?selection.id:selection?.entityId??selection?.id));
 const root=chosen??initial?.key??graph?.nodes.find(n=>n.state==='FAILED')?.key??[...(view.part?.features??[])].reverse().map(f=>`feature:${f.id}`).find(key=>graph?.nodes.some(n=>n.key===key))??graph?.nodes[0]?.key??'';
 const slice=useMemo(()=>graph?partDependencySlice(graph,root,direction,limit,display):undefined,[graph,root,direction,limit,display]);
 const current=graph?.nodes.find(n=>n.key===(selected??root));
 const nodes=useMemo(()=>slice?.nodes.map(n=>({id:n.key,label:n.label,caption:dependencyTypeLabel(n.type),kind:n.type,status:dependencyStateLabel(n.state),icon:n.type==='PARAMETER'?'parameters':n.type==='PROFILE'?'sketch':n.type==='BODY'?'body':'reference',muted:n.state==='SUPPRESSED',warning:dependencyWarning(n.state),position:n.position}))??[],[slice]);
 const edges=useMemo(()=>slice?.edges.map(e=>({id:JSON.stringify([e.source,e.kind,e.target]),source:e.source,target:e.target,directed:true,color:e.kind==='READ_VALUE'?'#89729f':e.kind==='READ_TOPOLOGY'?'#bb823d':'#37816b'}))??[],[slice]);
 const select=(key:string)=>setSelected(key);
 return <CommandDialog id="part-dependencies" title={`零件依赖 · ${view.document.name}`} open size="L" footer={false} onClose={onClose} onConfirm={onClose} bodyClassName="assembly-dependency-body" scrollBody={false} commandStatus={<CommandStatus pending={query.isFetching}/>}>
  <Space wrap className="assembly-dependency-toolbar">
   <Select aria-label="依赖分析对象" showSearch optionFilterProp="label" style={{width:250}} value={root||undefined} options={graph?.nodes.filter(n=>!['APPEARANCE','METADATA'].includes(n.type)).map(n=>({value:n.key,label:n.label}))} onChange={key=>{setChosen(key);setSelected(key)}}/>
   <Select aria-label="依赖方向" value={direction} options={[{value:'both',label:'上游与下游'},{value:'upstream',label:'读取来源'},{value:'downstream',label:'影响范围'}]} onChange={setDirection}/>
   <Button size="small" onClick={()=>setLayout(n=>n+1)}>整理布局</Button>
   <Checkbox checked={display} onChange={e=>setDisplay(e.target.checked)}>包含显示属性</Checkbox>
  </Space>
  {slice?.omitted? <Alert type="info" title={`已显示 ${slice.nodes.length} / ${slice.total} 个相关对象`} description="切换分析对象可缩小范围。" action={limit<1000?<Button size="small" onClick={()=>setLimit(Math.min(1000,limit*2))}>增加显示数量</Button>:undefined}/>:null}
  {nodes.length?<GraphView label="零件依赖关系图" nodes={nodes} edges={edges} selectedId={selected??root} layoutKey={`${scope}:${root}:${direction}:${layout}`} onSelect={select} onOpen={key=>{setChosen(key);setSelected(key)}}/>:!query.isFetching&&<Empty description={query.error?'读取依赖失败':'没有依赖对象'}/>}
  <div className="assembly-dependency-summary">
   <span>箭头由来源指向消费者；紫色为参数，橙色为拓扑输入。</span>
   {current&&<><Typography.Text strong>{current.label}</Typography.Text><span>{dependencyStateLabel(current.state)}</span><Button size="small" onClick={()=>onLocate(current,view.document.versionId)}>定位对象</Button><Button size="small" disabled={!comparisonAllowed||!view.part?.features.some(f=>f.id===(current.ownerFeatureId??current.entityId))} title={!comparisonAllowed?'请先结束当前几何编辑':undefined} onClick={()=>onCompare(current,view.document.versionId)}>健康历史对照</Button><Button size="small" disabled={!canEdit||!['SKETCH','PARAMETER','DATUM_PLANE','DATUM_AXIS','REFERENCE_POINT','LINEAR_EXTRUDE','PAD','REVOLVE','LOFT','HOLE','FILLET','CHAMFER','DRAFT','SHELL','SOLID_PATTERN'].includes(current.type)} onClick={()=>onEdit(current,view.document.versionId)}>编辑或修复</Button></>}
  </div>
  {current?.diagnostic&&<Alert type="warning" title={engineeringProblem(current.diagnostic)}/>}
  {current?.inputs?.length?<div className="feature-input-fields" aria-label="所选对象的输入关系" style={{maxHeight:160,overflow:'auto'}}>{current.inputs.map((p,i)=><div key={`${p.role}:${p.sourceId}:${p.regionId??p.entityId??i}`}><Typography.Text strong>{p.role}</Typography.Text> · {graph?.nodes.find(n=>n.entityId===p.sourceId)?.label??p.kind} · {dependencyStateLabel(p.state)}{p.regionId&&<span> · 区域引用</span>}{p.selection&&<small>拓扑输入可在特征面板中检查或替换。</small>}<Button size="small" disabled={!graph?.nodes.some(n=>n.entityId===p.sourceId)} onClick={()=>{const n=graph?.nodes.find(n=>n.entityId===p.sourceId);if(n){setChosen(n.key);setSelected(n.key);onLocate(n,view.document.versionId)}}}>定位来源</Button></div>)}</div>:null}
  {slice?.causes.length?<Space wrap aria-label="依赖根因">{slice.causes.map(n=><Button key={n.key} size="small" danger onClick={()=>{setChosen(n.key);setSelected(n.key);onLocate(n,view.document.versionId)}}>{n.label} · {dependencyStateLabel(n.state)}</Button>)}</Space>:null}
 </CommandDialog>;
}
