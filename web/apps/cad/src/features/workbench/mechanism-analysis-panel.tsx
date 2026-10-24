import {useEffect,useMemo,useRef,useState} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import {Alert,Button,Empty,Select,Space,Table,Tag,Typography} from 'antd';
import {api} from '../../api/client';
import type {DocumentView,SelectionItem} from '../../types';
import type {Mechanism} from '../../cad/assembly/motion-study';
import type {MechanismAnalysis} from '../../cad/assembly/mechanism-analysis';
import {motionPresentations,type MotionPresentation} from '../../cad/assembly/motion-presentation';
import {engineeringInstanceName} from '../../cad/assembly/assembly-engineering-state';
import {describeAssemblyReference} from '../../cad/assembly/assembly-reference-presentation';
import {engineeringLabel} from '../../cad/presentation/engineering-text';
import {engineeringProblem} from '../../cad/command/operation-feedback';
import {recordDeveloperEvidence} from '../../cad/command/developer-evidence';
import {CommandDialog} from '../../cad/overlay/floating-panel';

type Props={view:DocumentView;currentId?:string;selection?:SelectionItem|null;canEdit:boolean;onClose:()=>void;onLocate:(items:SelectionItem[])=>void;onRepair:(mechanismId:string,jointId:string)=>void;onShow:(mechanism?:Mechanism,report?:MechanismAnalysis,jointId?:string)=>boolean;onMotion:(motion?:MotionPresentation)=>void};
export function MechanismAnalysisPanel({view,currentId,selection,canEdit,onClose,onLocate,onRepair,onShow,onMotion}:Props){
 const client=useQueryClient(),mechanisms=view.product?.kinematics?.mechanisms??[];
 const [id,setId]=useState(()=>currentId??(mechanisms.length===1?mechanisms[0].id:undefined));
 const [report,setReport]=useState<MechanismAnalysis>(),[status,setStatus]=useState('待分析'),[error,setError]=useState<string>(),[selected,setSelected]=useState<string>(),[mismatch,setMismatch]=useState(false);
 const cleanup=useRef<{timer:ReturnType<typeof setTimeout>;key:unknown[]}|undefined>(undefined);
 const controller=useRef<AbortController|undefined>(undefined),generation=useRef(0),context=useRef('');
 context.current=JSON.stringify([view.document.id,view.document.versionId,id]);
 const callbacks=useRef({onShow,onMotion});callbacks.current={onShow,onMotion};
 const clear=()=>{callbacks.current.onShow();callbacks.current.onMotion()};
 const cancel=()=>{generation.current++;controller.current?.abort();void client.cancelQueries({queryKey:['mechanism-analysis',view.document.id,view.document.versionId,id],exact:true});setStatus('已取消');clear()};
 const analyze=async(force=false)=>{
  if(!id){setStatus('请选择机构');return}
  const queryKey=['mechanism-analysis',view.document.id,view.document.versionId,id];
  controller.current?.abort();const abort=new AbortController();controller.current=abort;
  const seq=++generation.current,key=context.current;setError(undefined);clear();
  const cached=force?undefined:client.getQueryData<MechanismAnalysis>(queryKey);
  if(cached){setReport(cached);setStatus(cached.evidence.available&&cached.evidence.status!=='PARTIAL'?'分析完成':'部分结果未知');return}
  setStatus('正在分析');setReport(undefined);
  try{if(force)await client.cancelQueries({queryKey,exact:true});
   const result=await client.fetchQuery({queryKey,staleTime:force?0:Infinity,retry:false,queryFn:({signal})=>api.analyzeMechanism(view.document.id,view.document.versionId,id,true,signal)});
   if(abort.signal.aborted||seq!==generation.current||context.current!==key)return;
   if(result.documentId!==view.document.id||result.revisionId!==view.document.versionId||result.mechanismId!==id)throw {code:'MECHANISM_ANALYSIS_STALE'};
   client.setQueryData(queryKey,result);recordDeveloperEvidence('机构分析',result);
   setReport(result);setStatus(result.evidence.available&&result.evidence.status!=='PARTIAL'?'分析完成':'部分结果未知');
  }catch(e){if(!abort.signal.aborted&&seq===generation.current&&context.current===key){recordDeveloperEvidence('机构分析失败',e);setStatus('分析未完成');setError(engineeringProblem(e))}}
 };
 useEffect(()=>{
  const key=['mechanism-analysis',view.document.id,view.document.versionId,id];
  if(cleanup.current){clearTimeout(cleanup.current.timer);if(JSON.stringify(cleanup.current.key)!==JSON.stringify(key))void client.cancelQueries({queryKey:cleanup.current.key,exact:true});cleanup.current=undefined;}
  setReport(undefined);setSelected(undefined);setMismatch(false);void analyze();
  return()=>{generation.current++;controller.current?.abort();clear();cleanup.current={key,timer:setTimeout(()=>void client.cancelQueries({queryKey:key,exact:true}),0)}};
 },[view.document.id,view.document.versionId,id]);
 const mechanism=mechanisms.find(m=>m.id===id),busy=status==='正在分析';
 const poseView=useMemo<DocumentView>(()=>report&&view.product?{...view,product:{...view.product,instances:view.product.instances.filter(i=>report.memberIds.includes(i.id)).map(i=>({...i,...report.poses?.[i.id]}))}}:view,[view,report]);
 const motions=useMemo(()=>report?motionPresentations(poseView,report.evidence,"",view):[],[poseView,report]);
 const showMotion=(motion?:MotionPresentation)=>{
  if(!report||!mechanism||!motion?.freedom||motion.freedom.kind===0&&motion.gaugeDof===0){callbacks.current.onMotion();return}
  const matches=callbacks.current.onShow(mechanism,report);setMismatch(!matches);
  callbacks.current.onMotion(matches?{...motion,configurationPoses:report.poses}:undefined);
 };
 useEffect(()=>{const instance=selection?.instanceId??(selection?.kind==='instance'?selection.id:undefined);showMotion(motions.find(m=>m.bodyId===instance));},[selection,motions]);
 const locate=(row:MechanismAnalysis['joints'][number])=>{
  setSelected(row.joint.id);callbacks.current.onMotion();
  const find=(node:NonNullable<DocumentView['structureTree']>):string|undefined=>node.entityId===row.joint.id&&node.mechanismId===id?node.id:node.children?.map(find).find(Boolean);
  onLocate([...row.endpoints.filter(e=>e.instanceId).map(e=>({kind:'instance' as const,id:e.instanceId,instanceId:e.instanceId,occurrencePath:e.instanceId})),{kind:'kinematic-object',id:row.joint.id,mechanismId:id,documentId:view.document.id,versionId:view.document.versionId,treeNodeId:view.structureTree?find(view.structureTree):undefined}]);
 };
 const issues=report?.joints.filter(r=>r.structureStatus!=='VALID'||r.supportStatus!=='VALID'||r.configurationStatus!=='VALID')??[];
 const detail=report?.joints.find(r=>r.joint.id===selected);
 const known=report?.evidence.components.filter(c=>c.solved)??[],relative=known.reduce((n,c)=>n+c.relativeDof,0),overall=known.reduce((n,c)=>n+c.gaugeDof,0);
 return <CommandDialog id="mechanism-analysis" title="机构分析" open size="L" footer={false} onClose={onClose} onConfirm={onClose}><div className="mechanism-analysis-panel">
  <Space wrap><Select aria-label="分析机构" value={id} placeholder="选择已保存机构" style={{minWidth:190}} options={mechanisms.map(m=>({value:m.id,label:m.name}))} onChange={setId}/><Button onClick={()=>void analyze(true)} disabled={!id||busy}>{error?'重试':'重新分析'}</Button>{busy&&<Button onClick={cancel}>取消计算</Button>}<Tag role="status" aria-label="机构分析状态">{status}</Tag></Space>
  {!mechanisms.length&&<Empty description="尚未保存机构，请先创建机构。"/>}
  {error&&<Alert type="error" title={error}/>}
  {report&&<>
   <div className="engineering-summary" aria-label="机构运动概况"><strong>{report.evidence.available&&report.evidence.status!=='PARTIAL'?`相对自由度 ${relative}`:'相对自由度待确定'}</strong><span>{!report.evidence.available||report.evidence.status==='PARTIAL'?'整体运动待确定':overall>0?`机构整体还可移动（${overall} 个自由度）`:'无整体运动'}</span><span>{report.memberIds.length} 个零件 · {report.joints.length} 个接合 · {report.groundCount} 个固定零件 · {report.commandCount} 个命令</span></div>
   {(issues.length>0||!report.evidence.available||report.evidence.status==='PARTIAL')&&<Alert type="warning" showIcon title={issues.length?`${issues.length} 个接合需要检查`:'当前构型的自由度尚无法确定'} description={issues.some(r=>r.supportStatus!=='VALID')?'支撑已丢失或无法对应，请选中接合并重新选择支撑。':issues.some(r=>r.configurationStatus==='INVALID')?'部分接合在当前位置不满足要求，请检查接合与零件位置。':'现有结果不足以确定全部运动，请重新分析或检查接合。'}/>}
   <div className="engineering-motion-list">{motions.map(m=><Button key={m.id} type={selection?.instanceId===m.bodyId?'primary':'default'} onClick={()=>{onLocate([{kind:'instance',id:m.bodyId,instanceId:m.bodyId,occurrencePath:m.bodyId}]);showMotion(m)}}>{m.name} · {m.description}</Button>)}</div>
   {mismatch&&<Alert type="info" title="视口位置与分析时不同，已隐藏运动标记。"/>}
   <Space wrap><Button onClick={()=>{if(mechanism)setMismatch(!onShow(mechanism,report))}}>显示接合</Button><Button onClick={clear}>隐藏标记</Button></Space>
   <Table size="small" rowKey={r=>r.joint.id} dataSource={report.joints} pagination={false} locale={{emptyText:'该机构尚无接合'}} onRow={r=>({onClick:()=>locate(r),'aria-selected':selected===r.joint.id})} columns={[
    {title:'接合',render:(_,r)=><>{r.joint.name}<Typography.Text type="secondary"> · {engineeringLabel(r.joint.kind,'接合')}</Typography.Text></>},
    {title:'关联零件',render:(_,r)=>r.endpoints.map(e=>engineeringInstanceName(view,e.instanceId)).join(' / ')},
    {title:'状态',render:(_,r)=>r.supportStatus!=='VALID'?'支撑需要修复':r.structureStatus!=='VALID'?'关系需要检查':r.configurationStatus==='INVALID'?'当前位置不满足':r.configurationStatus!=='VALID'?'当前位置待检查':'接合有效'},
    {title:'操作',render:(_,r)=><Space><Button size="small" aria-label={`定位 ${r.joint.name}`} onClick={e=>{e.stopPropagation();locate(r)}}>定位</Button><Button size="small" disabled={!canEdit||busy} onClick={e=>{e.stopPropagation();clear();onRepair(report.mechanismId,r.joint.id)}}>修复支撑</Button></Space>}
   ]}/>
   {detail&&<section className="engineering-detail"><strong>{detail.joint.name}</strong>{detail.endpoints.map(e=><div key={e.side}><p>{e.side==='first'?'第一零件':'第二零件'}：{engineeringInstanceName(view,e.instanceId)}</p>{e.supports.map(s=>{const ref=describeAssemblyReference(s.reference,view);return <p key={s.role}>{s.role==='axis'?'轴线':s.role==='plane'?'定位面':'定位点'}：{ref.secondary} · {engineeringLabel(s.status,'支撑待检查')}</p>})}</div>)}<p>约束：{detail.joint.constraints?.map(c=>c.name||engineeringLabel(c.kind,'约束')).join('、')||'未定义'}</p><p>关联命令：{detail.drivers.map(d=>`${d.name}（${engineeringLabel(detail.joint.coordinates?.find(c=>c.id===d.coordinateId)?.role,'坐标')}）`).join('、')||'无'}</p></section>}
  </>}
 </div></CommandDialog>;
}
