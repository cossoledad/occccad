import {useEffect,useRef,useState} from 'react';
import {Alert,Button,Select,Space,Table} from 'antd';
import {api} from '../../api/client';
import type {DocumentView,SelectionItem} from '../../types';
import type {Mechanism} from '../../cad/assembly/motion-study';
import {mechanismAnalysisCSV,type MechanismAnalysis} from '../../cad/assembly/mechanism-analysis';
import {motionPresentations,type MotionPresentation} from '../../cad/assembly/motion-presentation';
import {jointDefinitions} from '../../cad/assembly/joint-definitions';
import {FloatingPanel} from '../../cad/overlay/floating-panel';

type Props={view:DocumentView;currentId?:string;canEdit:boolean;onClose:()=>void;onLocate:(items:SelectionItem[])=>void;onRepair:(mechanismId:string,jointId:string)=>void;onShow:(mechanism?:Mechanism,report?:MechanismAnalysis,jointId?:string)=>boolean;onMotion:(motion?:MotionPresentation)=>void};
export function MechanismAnalysisPanel({view,currentId,canEdit,onClose,onLocate,onRepair,onShow,onMotion}:Props){
 const mechanisms=view.product?.kinematics?.mechanisms??[];
 const [id,setId]=useState(()=>currentId??(mechanisms.length===1?mechanisms[0].id:undefined));
 const [report,setReport]=useState<MechanismAnalysis>(),[status,setStatus]=useState('待检查'),[error,setError]=useState<string>(),[selected,setSelected]=useState<string>(),[mismatch,setMismatch]=useState(false);
 const controller=useRef<AbortController|undefined>(undefined),generation=useRef(0),context=useRef('');
 context.current=JSON.stringify([view.document.id,view.document.versionId,id]);
 const callbacks=useRef({onShow,onMotion});callbacks.current={onShow,onMotion};
 const cancel=()=>{generation.current++;controller.current?.abort();setStatus('已取消');callbacks.current.onShow();callbacks.current.onMotion()};
 const analyze=async(numerical:boolean)=>{
  if(!id)return;
  controller.current?.abort();const abort=new AbortController();controller.current=abort;
  const seq=++generation.current,key=context.current;setStatus(numerical?'正在计算':'正在检查');setError(undefined);setReport(undefined);callbacks.current.onShow();callbacks.current.onMotion();
  try{const result=await api.analyzeMechanism(view.document.id,view.document.versionId,id,numerical,abort.signal);
   if(abort.signal.aborted||seq!==generation.current||context.current!==key)return;
   if(result.documentId!==view.document.id||result.revisionId!==view.document.versionId||result.mechanismId!==id)throw new Error('机构分析来源不匹配');
   setReport(result);setStatus(numerical?(result.evidence.available?'计算完成':'数值结果未知'):'结构检查完成');
  }catch(e){if(!abort.signal.aborted&&seq===generation.current&&context.current===key){setStatus('失败');setError(String(e))}}
 };
 useEffect(()=>{setReport(undefined);setSelected(undefined);setMismatch(false);void analyze(false);return()=>{generation.current++;controller.current?.abort();callbacks.current.onShow();callbacks.current.onMotion()}},[view.document.id,view.document.versionId,id]);
 const mechanism=mechanisms.find(m=>m.id===id),busy=status==='正在计算'||status==='正在检查';
 const locate=(row:MechanismAnalysis['joints'][number])=>{
  setSelected(row.joint.id);
  const find=(node:NonNullable<DocumentView['structureTree']>):string|undefined=>node.entityId===row.joint.id&&node.mechanismId===id?node.id:node.children?.map(find).find(Boolean);
  onLocate([...row.endpoints.filter(e=>e.instanceId).map(e=>({kind:'instance' as const,id:e.instanceId,instanceId:e.instanceId})),{kind:'kinematic-object',id:row.joint.id,mechanismId:id,documentId:view.document.id,versionId:view.document.versionId,treeNodeId:view.structureTree?find(view.structureTree):undefined}]);
 };
 const show=(jointId?:string)=>{if(report&&mechanism)setMismatch(!onShow(mechanism,report,jointId))};
 const download=(format:'json'|'csv')=>{if(!report)return;const url=URL.createObjectURL(new Blob([format==='json'?JSON.stringify(report,null,2):mechanismAnalysisCSV(report)],{type:format==='json'?'application/json':'text/csv;charset=utf-8'}));const a=document.createElement('a');a.href=url;a.download=`mechanism-analysis-${report.mechanismId}.${format}`;a.click();URL.revokeObjectURL(url)};
 const poseView:DocumentView=report&&view.product?{...view,product:{...view.product,instances:view.product.instances.filter(i=>report.memberIds.includes(i.id)).map(i=>({...i,...report.poses?.[i.id]}))}}:view;
 const motions=report?motionPresentations(poseView,report.evidence):[];
 return <FloatingPanel title={<Space>机构分析<Button onClick={onClose}>关闭分析</Button></Space>} position="top-right" className="mechanism-analysis-panel"><div style={{padding:12,maxHeight:'75vh',overflow:'auto',width:720,maxWidth:'80vw'}}>
  <Space wrap><Select aria-label="分析机构" value={id} placeholder="选择已保存机构" style={{minWidth:190}} options={mechanisms.map(m=>({value:m.id,label:m.name}))} onChange={setId}/><Button onClick={()=>void analyze(true)} disabled={!id||busy}>刷新分析</Button><Button onClick={cancel} disabled={!busy}>取消计算</Button>{status==='失败'&&<Button onClick={()=>void analyze(true)}>重试</Button>}<span role="status">{status}</span></Space>
  <p>来源 Revision：<code>{view.document.versionId}</code></p>
  <p>仅分析所选机构的完整物理关系与已接受位姿；不施加命令锁，不激活机构，不采用求解姿态。</p>
  {error&&<Alert type="error" title={error}/>}{mismatch&&<Alert type="info" title="视口与分析构型不同，接合与方向标记已隐藏。可查看报告或编辑支撑。"/>}
  {report&&<><p>成员 {report.memberIds.length} · 接合 {report.joints.length} · Ground {report.groundCount} · 命令 {report.commandCount}</p>
   {report.diagnostics.map((d,i)=><Alert key={i} type="warning" title={d}/>)}
   <Space wrap><Button onClick={()=>show()}>显示接合</Button><Button onClick={()=>{onShow();onMotion()}}>隐藏标记</Button><Button onClick={()=>download('json')}>导出 JSON</Button><Button onClick={()=>download('csv')}>导出 CSV</Button></Space>
   <Table size="small" rowKey={r=>r.joint.id} dataSource={report.joints} pagination={false} scroll={{x:1050}} onRow={r=>({onClick:()=>locate(r),'data-selected':selected===r.joint.id})} columns={[
    {title:'接合',render:(_,r)=><>{r.joint.name}<br/>{jointDefinitions[r.joint.kind]?.name??r.joint.kind}</>},
    {title:'两端 occurrence / 几何支撑',render:(_,r)=><>{r.endpoints.map(e=><div key={e.side}><strong>{e.side==='first'?'端 1':'端 2'}：{e.name||e.instanceId||'缺失'}</strong><br/><small>{e.instanceId} · {e.status}</small><Button size="small" disabled={!e.instanceId} onClick={event=>{event.stopPropagation();onLocate([{kind:'instance',id:e.instanceId,instanceId:e.instanceId}])}}>定位端点</Button>{e.diagnostic&&<div>{e.diagnostic}</div>}{e.supports.map(s=><div key={s.role}>{s.role}: {s.reference?.derivedRole??s.reference?.kind??'局部框架'} · {s.status}{s.exactType&&` / ${s.exactType}`}<small>{s.reference?.geometryId}</small><div>{s.diagnostic}</div></div>)}</div>)}</>},
    {title:'真实约束 / 命令与坐标',render:(_,r)=><>{r.joint.constraints?.map(c=><div key={c.id}>{c.name??c.kind} · {c.relationRole}<small> {c.id}</small></div>)}{r.drivers.map(d=><div key={d.id}>{d.name} → {r.joint.coordinates?.find(c=>c.id===d.coordinateId)?.role??d.coordinateId}</div>)}{!r.drivers.length&&'无关联命令'}</>},
    {title:'检查',render:(_,r)=><>结构：{r.structureStatus}<br/>支撑：{r.supportStatus}<br/>当前构型：{r.configurationStatus}<div>{r.diagnostic}</div></>},
    {title:'操作',render:(_,r)=><Space orientation="vertical"><Button aria-label="定位" onClick={()=>locate(r)}>定位</Button><Button onClick={()=>show(r.joint.id)}>显示此接合</Button><Button disabled={!canEdit||busy} onClick={()=>{onShow();onMotion();onRepair(report.mechanismId,r.joint.id)}}>编辑 / 重选</Button></Space>}
   ]}/>
   <p>物理自由度：{report.evidence.status==='NOT_COMPUTED'?'待刷新':report.evidence.status} {report.evidence.diagnosticCode} {report.evidence.diagnostic}</p>
   {report.evidence.components.map((c,i)=><p key={i}>{c.bodyIds.map(id=>view.product?.instances.find(v=>v.id===id)?.name??id).join('、')}：{c.solved?`相对 DOF ${c.relativeDof}；整体运动 ${c.gaugeDof}`:'未知'} {c.gaugeDof>0&&'（数值 gauge 不是物理固定）'}</p>)}
   {motions.map(m=><p key={m.id}>{m.name}：{m.description} <Button disabled={!m.freedom} onClick={()=>{if(mechanism&&onShow(mechanism,report,selected)){onMotion({...m,configurationPoses:report.poses});setMismatch(false)}else{onMotion();setMismatch(true)}}}>显示方向</Button></p>)}
   <details><summary>来源与策略</summary><p>输入：{report.inputDigest}</p><p>策略：{report.policyDigest}</p></details>
  </>}
 </div></FloatingPanel>;
}
