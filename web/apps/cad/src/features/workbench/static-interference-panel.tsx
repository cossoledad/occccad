import {useEffect,useRef,useState} from 'react';
import {Alert,Button,InputNumber,Segmented,Space,Table,Tag} from 'antd';
import {api} from '../../api/client';
import type {DocumentView,InstancePath,SelectionItem} from '../../types';
import {staticPartLeaves,staticPartPick,staticPartSelection,staticBodySelection,staticPairLabel,staticReportStatus,staticResultMatches,type StaticInterferenceReport,type StaticInterferencePair} from '../../cad/assembly/static-interference';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandStatus} from '../../cad/overlay/command-form';
import {FeaturePickField} from './feature-pick-field';
import {FeaturePickScope} from '../../cad/interaction/feature-selection';
import type {FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {engineeringProblem} from '../../cad/command/operation-feedback';
import {recordDeveloperEvidence} from '../../cad/command/developer-evidence';
import {formatDisplayNumber} from '../../utils/display-number';

type Props={view:DocumentView;unit:'mm'|'cm'|'m'|'in';seed:SelectionItem[];blocked:boolean;onClose:()=>void;onSelectionSession:(s?:FeatureSelectionSession)=>void;onLocate:(items:SelectionItem[])=>void;onShow:(r?:StaticInterferenceReport,index?:number)=>boolean;onPrepare:()=>Promise<void>};
export function StaticInterferencePanel({view,unit,seed,blocked,onClose,onSelectionSession,onLocate,onShow,onPrepare}:Props){
 const [scope,setScope]=useState<'ALL'|'SELECTED'>('ALL'),[selected,setSelected]=useState<InstancePath[]>(()=>[...new Map(seed.flatMap(s=>{const p=staticPartPick(view,s);return p?[[p.canonical,p] as const]:[]})).values()]);
 const [gap,setGap]=useState(0),[picking,setPicking]=useState(false),[report,setReport]=useState<StaticInterferenceReport>(),[row,setRow]=useState<number>(),[busy,setBusy]=useState(false),[error,setError]=useState<string>(),[status,setStatus]=useState('尚未检查'),[mismatch,setMismatch]=useState(false);
 const generation=useRef(new FeaturePickScope()),abort=useRef<AbortController|undefined>(undefined),current=useRef('');
 const key=JSON.stringify([view.document.id,view.document.versionId,scope,selected,gap,blocked]);current.current=key;generation.current.activate("static-interference",key);
 const callbacks=useRef({onShow,onSelectionSession,onPrepare,onLocate});callbacks.current={onShow,onSelectionSession,onPrepare,onLocate};
 const cancel=()=>{generation.current.cancel();abort.current?.abort();setBusy(false);setReport(undefined);setRow(undefined);setStatus('已取消');callbacks.current.onShow()};
 useEffect(()=>{generation.current.cancel();generation.current.activate("static-interference",key);abort.current?.abort();setBusy(false);setReport(undefined);setRow(undefined);setMismatch(false);setError(undefined);setStatus('尚未检查');callbacks.current.onShow();callbacks.current.onLocate([]);return()=>{generation.current.cancel();abort.current?.abort();callbacks.current.onShow()}},[key]);
 const selectedRevision=useRef(view.document.id+view.document.versionId);
 useEffect(()=>{const next=view.document.id+view.document.versionId;if(selectedRevision.current!==next){selectedRevision.current=next;setSelected([])}},[view.document.id,view.document.versionId]);
 useEffect(()=>{
  callbacks.current.onSelectionSession(picking&&!busy&&scope==='SELECTED'?{role:'instance',documentId:view.document.id,versionId:view.document.versionId,selections:selected.map(path=>staticPartSelection(view,path)),accept:s=>!!staticPartPick(view,s),onPick:s=>{const path=staticPartPick(view,s);if(path)setSelected(old=>old.some(p=>p.canonical===path.canonical)?old.filter(p=>p.canonical!==path.canonical):[...old,path])}}:undefined);
  return()=>callbacks.current.onSelectionSession();
 },[picking,busy,scope,view,JSON.stringify(selected)]);
 const scale=({mm:1,cm:10,m:1000,in:25.4})[unit],leaves=staticPartLeaves(view);
 const invalidGap=!Number.isFinite(gap)||gap<0;
 const run=async()=>{
  if(invalidGap||blocked||busy||scope==='SELECTED'&&selected.length<2)return;
  const n=generation.current.latest(),context=current.current,controller=new AbortController();abort.current?.abort();abort.current=controller;setPicking(false);setBusy(true);setReport(undefined);setError(undefined);setStatus('正在检查');callbacks.current.onShow();
  try{
   await callbacks.current.onPrepare();if(controller.signal.aborted||!generation.current.current(n)||current.current!==context)return;
   const input={revisionId:view.document.versionId,scope,selected:scope==='SELECTED'?selected:undefined,clearanceMm:gap};
   const result=await api.analyzeStaticInterference(view.document.id,input,controller.signal);
   if(controller.signal.aborted||!generation.current.current(n)||current.current!==context)return;
   if(!staticResultMatches(view,input,result))throw {code:'STATIC_INTERFERENCE_STALE'};
   recordDeveloperEvidence('静态干涉与间隙检查',result);setReport(result);setStatus(staticReportStatus(result));
  }catch(e){if(!controller.signal.aborted&&generation.current.current(n)&&current.current===context){recordDeveloperEvidence('静态干涉检查失败',e);setStatus('检查未完成');setError(engineeringProblem(e))}}finally{if(generation.current.current(n))setBusy(false)}
 };
 const selectRow=(pair:StaticInterferencePair,index:number)=>{if(!report)return;setRow(index);const bodies=[pair.firstId,pair.secondId].flatMap(id=>report.bodies.find(b=>b.id===id)??[]);onLocate(bodies.map(b=>staticBodySelection(view,b)));setMismatch(!callbacks.current.onShow(report,index))};
 const close=()=>{cancel();setPicking(false);onClose()};
 return <CommandDialog id="static-interference" title="干涉与间隙检查" open size="L" onClose={close} onCancelPick={()=>{if(!picking)return false;setPicking(false);return true}} cancelText="关闭" confirmText={error?'重试':'检查'} onConfirm={run} confirmDisabled={invalidGap||busy||blocked||scope==='SELECTED'&&selected.length<2} commandStatus={<CommandStatus pending={busy} error={error} onRetry={()=>void run()}/>} secondaryActions={busy?<Button onClick={cancel}>取消计算</Button>:undefined}>
  <CommandField label="检查范围" width="content"><Segmented value={scope} options={[{value:'ALL',label:'全部零件'},{value:'SELECTED',label:'所选零件之间'}]} onChange={v=>{setScope(v as "ALL"|"SELECTED");setPicking(v==='SELECTED')}}/></CommandField>
  {scope==='SELECTED'&&<FeaturePickField label="零件" active={picking} mode="multiple" onActivate={()=>setPicking(true)} onCancel={()=>setPicking(false)} onClear={()=>setSelected([])} items={selected.map(path=>({key:path.canonical,value:leaves.get(path.canonical)?.name??'引用零件已不可用',onClear:()=>setSelected(old=>old.filter(p=>p.canonical!==path.canonical)),selections:[staticPartSelection(view,path)],onLocate:()=>onLocate([staticPartSelection(view,path)])}))} error={selected.length<2?'至少选择两个零件':undefined}/>}
  <CommandField label={`最小间隙（${unit}）`} width="number" error={invalidGap?'请输入有效的非负长度':undefined}><InputNumber min={0} value={gap/scale} changeOnBlur={false} onChange={v=>setGap((v??0)*scale)}/></CommandField>
  <Space wrap><Tag role="status">{status}</Tag>{report&&<span>{report.checkedPairs} / {report.pairs.length} 对已完成</span>}</Space>
  {blocked&&<Alert type="info" title="请先结束当前位置预览或运动操作，再检查已保存装配。"/>}
  {report?.issues.map((issue,i)=><Alert key={i} type="warning" title={`${issue.name}：${issue.diagnosticCode==='VARIANT_UNAVAILABLE'?'关联构型不可用':issue.diagnosticCode==='BODY_EVALUATION_FAILED'?'实体求值未完成':'缺少精确实体几何'}`}/>)}
  {report&&<>
   {(report.layoutParts>0||report.consumedBodies>0)&&<p>{report.layoutParts>0?`${report.layoutParts} 个无实体零件` : ''}{report.layoutParts>0&&report.consumedBodies>0?' · ':''}{report.consumedBodies>0?`${report.consumedBodies} 个已消费实体` : ''}</p>}
   {mismatch&&<Alert type="info" title="视图区位置与检查时不同，已隐藏标记。"/>}
   <Table size="small" dataSource={report.pairs} rowKey={p=>JSON.stringify([p.firstId,p.secondId])} pagination={{pageSize:10,showSizeChanger:false}} onRow={p=>({onClick:()=>selectRow(p,report.pairs.indexOf(p)),'aria-selected':row===report.pairs.indexOf(p)})} columns={[
    {title:<Tag color="orange">第一零件</Tag>,ellipsis:true,render:(_,p)=>report.bodies.find(b=>b.id===p.firstId)?.name??'零件不可用'},
    {title:<Tag color="cyan">第二零件</Tag>,ellipsis:true,render:(_,p)=>report.bodies.find(b=>b.id===p.secondId)?.name??'零件不可用'},
    {title:'状态',render:(_,p)=>staticPairLabel(p)},
    {title:`最小距离（${unit}）`,render:(_,p)=>p.complete?formatDisplayNumber(p.distanceMm/scale):'—'},
    {title:`交叠体积（${unit}³）`,render:(_,p)=>p.complete?formatDisplayNumber(p.commonVolumeMm3/(scale**3)):'—'},
   ]}/>
  </>}
 </CommandDialog>;
}
