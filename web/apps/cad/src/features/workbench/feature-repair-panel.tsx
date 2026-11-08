import {Alert,Tag,Descriptions,Empty,Select,Space,Switch,Table,Typography} from 'antd';
import {useEffect,useRef,useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import type {DocumentView} from '../../types';
import type {DisplayLengthUnit} from '../../state/ui-preferences';
import {api} from '../../api/client';
import {commandForView,contextScopeForView,viewContextKey} from '../../api/context-edit-scope';
import {featureRepairSnapshotMatches,repairParameterPairs,type FeatureRepairSnapshot} from '../../cad/part/feature-repair-context';
import {dependencyStateLabel,type PartDependencyInput} from '../../cad/part/part-dependency-graph';
import {useFeatureDialogFeedback} from './use-feature-dialog-feedback';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandStatus} from '../../cad/overlay/command-form';
import {parameterDisplayValue,parameterInputUnit,parameterSourceText} from './parameter-editor';

export function FeatureRepairPanel({view,featureId,unit,canEdit,onClose,onPreview,onRepair}:{view:DocumentView;featureId:string;unit:DisplayLengthUnit;canEdit:boolean;onClose:()=>void;onPreview:(snapshot?:FeatureRepairSnapshot)=>void;onRepair:()=>void}){
 const [limit,setLimit]=useState(64),[revision,setRevision]=useState<string>(),[show,setShow]=useState(true),[manual,setManual]=useState(false);
 const scope=viewContextKey(view),owner=contextScopeForView(view)?.rootProductDocumentId??view.document.id;
 const query=useQuery({queryKey:['feature-repair-context',scope,featureId,revision,limit],queryFn:({signal})=>api.getFeatureRepairContext(view.document.id,commandForView(view,{versionId:view.document.versionId,featureId,compareVersionId:revision,maxRevisions:limit}),signal),gcTime:0});
 const history=useQuery({queryKey:['repair-history',owner,scope],queryFn:()=>api.getHistory(owner),enabled:manual});
 useFeatureDialogFeedback(query.error??history.error,'历史对照',undefined,undefined,()=>{void query.refetch();if(history.error)void history.refetch()});
 const data=query.data&&featureRepairSnapshotMatches(view,query.data,featureId)?query.data:undefined;
 const snapshot=data?.snapshot,feature=view.part?.features.find(f=>f.id===featureId),callback=useRef(onPreview);callback.current=onPreview;
 useEffect(()=>{callback.current(show?snapshot:undefined);return()=>callback.current();},[show,snapshot]);
 useEffect(()=>{const id=data?.displayId;return()=>{if(id)void api.discardFeatureRepairContext(view.document.id,id).catch(()=>{});}},[data?.displayId,view.document.id]);
 const versions=[...new Map(history.data?.map(h=>[h.versionId,h])??[]).values()];
 const inputs=(items:PartDependencyInput[]|undefined)=><div aria-label="引用定义">{items?.length?items.map((p,i)=><div key={`${p.role}:${i}`}><strong>{p.role}</strong> · {p.sourceName??'引用对象'} · {dependencyStateLabel(p.state)}{p.regionId?' · 区域':''}{p.selection?` · ${p.selection.expectedType==='FACE'?'面':p.selection.expectedType==='EDGE'?'边':'顶点'}`:''}</div>):<span>无几何引用</span>}</div>;
 const source=(p:ReturnType<typeof repairParameterPairs>[number]['current'])=>p?<><Typography.Text code>{parameterSourceText(p,parameterInputUnit(p,unit))}</Typography.Text><small> = {parameterDisplayValue(p,unit)}</small></>:<span>无此参数</span>;
 const close=()=>{callback.current();onClose()};
 return <CommandDialog id="feature-repair-context" title={`历史对照 · ${feature?.name??'特征'}`} open size="L" cancelText="关闭对照" confirmText="使用当前输入修复" confirmDisabled={!canEdit||query.isFetching} onClose={close} onConfirm={()=>{callback.current();onRepair()}} commandStatus={<CommandStatus pending={query.isFetching}/>}>
  <Space wrap>
   <Select aria-label="历史对照版本" style={{width:280}} value={revision??''} onOpenChange={open=>{if(open)setManual(true)}} options={[{value:'',label:'最近的健康历史'},...versions.map(v=>({value:v.versionId,label:v.versionName||new Date(v.createdAt).toLocaleString()}))]} onChange={id=>setRevision(id||undefined)}/>
   <Select aria-label="历史搜索上限" value={limit} options={[64,128,256].map(n=>({value:n,label:`最多检查 ${n} 个版本`}))} onChange={setLimit}/>
   <span>显示历史几何</span><Switch aria-label="显示历史几何" checked={show} disabled={!snapshot} onChange={value=>{setShow(value);if(value&&Date.now()-query.dataUpdatedAt>30_000)void query.refetch()}}/>
  </Space>
  {snapshot?<>
   <Space><Tag>{show?'历史对照 · 只读':'当前模型'}</Tag>{snapshot.reconstructed&&<Tag>重建结果</Tag>}</Space>
   <Descriptions size="small" column={2} items={[{key:'time',label:'历史时间',children:new Date(snapshot.createdAt).toLocaleString()},{key:'search',label:'检查范围',children:`已检查 ${data.examined} 个版本 / 上限 ${data.searchLimit}`} ]}/>
   <div className="command-vector-fields"><section><Typography.Title level={5}>当前引用</Typography.Title>{inputs(data.currentInputs)}</section><section><Typography.Title level={5}>历史引用</Typography.Title>{inputs(snapshot.inputs)}</section></div>
   <Table size="small" pagination={false} rowKey="id" dataSource={repairParameterPairs(data)} columns={[{title:'数量',key:'quantity',render:(_,p)=>p.current?.label??p.historical?.label},{title:'当前来源和值',key:'current',render:(_,p)=>source(p.current)},{title:'历史来源和值',key:'historical',render:(_,p)=>source(p.historical)}]}/>
  </>:!query.isFetching&&!query.error?<Empty description={`在检查的 ${data?.examined??0} 个版本内未找到健康定义。`}/>:null}
  {data?.downstream.length?<Alert type="warning" title={`可能受影响的下游：${data.downstream.map(n=>n.label).join('、')}`}/>:null}
 </CommandDialog>;
}
