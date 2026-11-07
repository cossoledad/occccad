import {useEffect,useRef,useState} from 'react';
import {Alert,Button,Checkbox,Segmented,Table,Select,Input} from 'antd';
import {defaultSpatial,spatialStatus,type SpatialAnalysis} from '../../cad/assembly/spatial-analysis';
import {api} from '../../api/client';
import {CadNumberInput as InputNumber} from '../../cad/overlay/cad-number-input';
import type {DocumentView,InstancePath,SelectionItem} from '../../types';
import {staticPartLeaves,staticPartPick,staticPartSelection,staticBodySelection,staticPairLabel,staticReportStatus,staticResultMatches,interferenceProblem,interferenceNumber,interferencePairKey,interferenceGroups,withInterferenceDetail,type InterferenceScope,type StaticInterferenceReport,type StaticInterferencePair} from '../../cad/assembly/static-interference';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandStatus} from '../../cad/overlay/command-form';
import {FeaturePickField} from './feature-pick-field';
import {FeaturePickScope} from '../../cad/interaction/feature-selection';
import type {FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {engineeringProblem} from '../../cad/command/operation-feedback';
import {recordDeveloperEvidence} from '../../cad/command/developer-evidence';

type Props={initial?:SpatialAnalysis;canSave?:boolean;onSave?:(definition:SpatialAnalysis,signal?:AbortSignal)=>Promise<void>;view:DocumentView;unit:'mm'|'cm'|'m'|'in';seed:SelectionItem[];blocked:boolean;onClose:()=>void;onSelectionSession:(s?:FeatureSelectionSession)=>void;onLocate:(items:SelectionItem[])=>void;onShow:(r?:StaticInterferenceReport,index?:number,isolate?:boolean)=>boolean;onPrepare:()=>Promise<void>};
export function StaticInterferencePanel({initial,canSave,onSave,view,unit,seed,blocked,onClose,onSelectionSession,onLocate,onShow,onPrepare}:Props){
 const [mechanismId,setMechanismId]=useState<string>(),[scope,setScope]=useState<InterferenceScope>(initial?.scope??'ALL'),[selected,setSelected]=useState<InstancePath[]>(()=>initial?.selected??[...new Map(seed.flatMap(s=>{const p=staticPartPick(view,s);return p?[[p.canonical,p] as const]:[]})).values()]);
 const [second,setSecond]=useState<InstancePath[]>(initial?.secondSelection??[]),[active,setActive]=useState<0|1>(),[gap,setGap]=useState(initial?.clearanceMm??0),[report,setReport]=useState<StaticInterferenceReport>(),[row,setRow]=useState<string>(),[busy,setBusy]=useState(false),[detailing,setDetailing]=useState(false),[error,setError]=useState<string>(),[status,setStatus]=useState('尚未检查'),[mismatch,setMismatch]=useState(false);
 const [name,setName]=useState(initial?.name??''),[saving,setSaving]=useState(false),[keep,setKeep]=useState(Boolean(canSave));
 const [filter,setFilter]=useState('PROBLEMS'),[grouping,setGrouping]=useState('PAIRS'),[isolate,setIsolate]=useState(false);
 const displayOptions=useRef({isolate});displayOptions.current={isolate};
 const generation=useRef(new FeaturePickScope()),abort=useRef<AbortController|undefined>(undefined),detailAbort=useRef<AbortController|undefined>(undefined),detailEpoch=useRef(0),current=useRef('');
 const key=JSON.stringify([view.document.id,view.document.versionId,scope,selected,second,gap,blocked,mechanismId]);current.current=key;generation.current.activate('static-interference',key);
 const callbacks=useRef({onShow,onSelectionSession,onPrepare,onLocate});callbacks.current={onShow,onSelectionSession,onPrepare,onLocate};
 const details=useRef(new Map<string,StaticInterferencePair>());
 const cancel=()=>{generation.current.cancel();abort.current?.abort();detailAbort.current?.abort();detailEpoch.current++;setBusy(false);setDetailing(false);setReport(undefined);setRow(undefined);setStatus('已取消');callbacks.current.onShow()};
 useEffect(()=>{generation.current.cancel();generation.current.activate('static-interference',key);abort.current?.abort();detailAbort.current?.abort();detailEpoch.current++;details.current.clear();setBusy(false);setDetailing(false);setReport(undefined);setRow(undefined);setMismatch(false);setError(undefined);setStatus('尚未检查');callbacks.current.onShow();callbacks.current.onLocate([]);return()=>{generation.current.cancel();abort.current?.abort();detailAbort.current?.abort();detailEpoch.current++;callbacks.current.onShow()}},[key]);
 useEffect(()=>{if(!initial?.result)return;const controller=new AbortController(),context=current.current;
 void api.getSpatialResult(view.document.id,view.document.versionId,initial.id,controller.signal).then(result=>{if(controller.signal.aborted||current.current!==context)return;setReport(result.check);setStatus(spatialStatus(initial.status));recordDeveloperEvidence('已保存碰撞结果',result)}).catch(e=>{if(!controller.signal.aborted)setError(engineeringProblem(e))});return()=>controller.abort();
 },[initial?.id,view.document.versionId]);
 const save=async()=>{if(!onSave||!report?.complete||report.revisionId!==view.document.versionId||mechanismId)return;setSaving(true);setError(undefined);const controller=new AbortController();abort.current=controller;try{await onSave({...defaultSpatial('CLASH'),...initial,name,scope,selected,secondSelection:second,clearanceMm:gap},controller.signal);close()}catch(e){recordDeveloperEvidence('保存碰撞检查失败',e);setError(engineeringProblem(e))}finally{setSaving(false)}};
 const selectedRevision=useRef(view.document.id+view.document.versionId);
 useEffect(()=>{const next=view.document.id+view.document.versionId;if(selectedRevision.current!==next){selectedRevision.current=next;setSelected([]);setSecond([])}},[view.document.id,view.document.versionId]);
 useEffect(()=>{
  const picks=active===1?second:selected,set=active===1?setSecond:setSelected;
  callbacks.current.onSelectionSession(active!==undefined&&!busy&&scope!=='ALL'?{role:'instance',documentId:view.document.id,versionId:view.document.versionId,selections:picks.map(path=>staticPartSelection(view,path)),accept:s=>!!staticPartPick(view,s),onPick:s=>{const path=staticPartPick(view,s);if(path)set(old=>old.some(p=>p.canonical===path.canonical)?old.filter(p=>p.canonical!==path.canonical):[...old,path])}}:undefined);
  return()=>callbacks.current.onSelectionSession();
 },[active,busy,scope,view,JSON.stringify(selected),JSON.stringify(second)]);
 const scale=({mm:1,cm:10,m:1000,in:25.4})[unit],leaves=staticPartLeaves(view);
 const invalidGap=!Number.isFinite(gap)||gap<0;
 const selectionMissing=scope!=='ALL'&&!selected.length||scope==='BETWEEN'&&!second.length;
 const input=()=>({revisionId:view.document.versionId,scope,selected:scope==='ALL'?undefined:selected,secondSelection:scope==='BETWEEN'?second:undefined,clearanceMm:gap,thresholdOnly:true,mechanismId});
 const run=async()=>{
  if(invalidGap||blocked||busy||saving||selectionMissing)return;
  const n=generation.current.latest(),context=current.current,controller=new AbortController();abort.current?.abort();detailAbort.current?.abort();detailEpoch.current++;details.current.clear();abort.current=controller;setActive(undefined);setBusy(true);setReport(undefined);setError(undefined);setStatus('正在检查');callbacks.current.onShow();
  try{
   await callbacks.current.onPrepare();if(controller.signal.aborted||!generation.current.current(n)||current.current!==context)return;
   const request=input(),result=await api.analyzeStaticInterference(view.document.id,request,controller.signal);
   if(controller.signal.aborted||!generation.current.current(n)||current.current!==context)return;
   if(!staticResultMatches(view,request,result))throw {code:'STATIC_INTERFERENCE_STALE'};
   recordDeveloperEvidence('静态干涉与间隙检查',result);setReport(result);setStatus(staticReportStatus(result));
  }catch(e){if(!controller.signal.aborted&&generation.current.current(n)&&current.current===context){recordDeveloperEvidence('静态干涉检查失败',e);setStatus('检查未完成');setError(engineeringProblem(e))}}finally{if(generation.current.current(n))setBusy(false)}
 };
 const selectRow=async(pair:StaticInterferencePair)=>{
  if(!report)return;if(report.revisionId!==view.document.versionId){setError('结果待更新，请先重新检查。');return;}const id=interferencePairKey(pair),epoch=++detailEpoch.current,context=current.current;
  detailAbort.current?.abort();setDetailing(false);setRow(id);setError(undefined);
  const bodies=[pair.firstId,pair.secondId].flatMap(id=>report.bodies.find(b=>b.id===id)??[]);onLocate(bodies.map(b=>staticBodySelection(view,b)));
  let shown=report;
  const cached=details.current.get(id);
  if(cached)shown={...report,pairs:report.pairs.map(p=>interferencePairKey(p)===id?cached:p)};
  setMismatch(!callbacks.current.onShow(shown,shown.pairs.findIndex(p=>interferencePairKey(p)===id),displayOptions.current.isolate));
  if(!pair.complete||cached)return;
  const controller=new AbortController();detailAbort.current=controller;setDetailing(true);
  try {
   const request={...input(),thresholdOnly:false,detailPair:[pair.firstId,pair.secondId]},result=await api.analyzeStaticInterference(view.document.id,request,controller.signal);
   if(controller.signal.aborted||epoch!==detailEpoch.current||context!==current.current)return;
   const value=result.pairs[0];shown=withInterferenceDetail(view,request,report,result,pair);
   if(value.complete&&value.detailStatus==='COMPLETE')details.current.set(id,value);recordDeveloperEvidence('干涉局部分析',result);
   setReport(shown);
   setMismatch(!callbacks.current.onShow(shown,shown.pairs.findIndex(p=>interferencePairKey(p)===id),displayOptions.current.isolate));
  }catch(e){if(!controller.signal.aborted&&epoch===detailEpoch.current&&context===current.current){recordDeveloperEvidence('干涉局部分析失败',e);setError(engineeringProblem(e))}}finally{if(epoch===detailEpoch.current)setDetailing(false)}
 };
 useEffect(()=>{if(report&&row){const index=report.pairs.findIndex(p=>interferencePairKey(p)===row);if(index>=0)setMismatch(!callbacks.current.onShow(report,index,isolate))}},[isolate]);
 const close=()=>{cancel();setActive(undefined);onClose()};
 const pickField=(index:0|1,label:string)=>{const values=index===0?selected:second,set=index===0?setSelected:setSecond;return <FeaturePickField label={label} active={active===index} mode="multiple" onActivate={()=>setActive(index)} onCancel={()=>setActive(undefined)} onClear={()=>set([])} items={values.map(path=>({key:path.canonical,value:leaves.get(path.canonical)?.name??'引用组件已不可用',onClear:()=>set(old=>old.filter(p=>p.canonical!==path.canonical)),selections:[staticPartSelection(view,path)],onLocate:()=>onLocate([staticPartSelection(view,path)])}))} error={!values.length?'请选择零件或子装配':undefined}/>};
 const visible=(p:StaticInterferencePair)=>filter==='ALL'||filter==='PROBLEMS'&&interferenceProblem(p)||filter===p.classification||filter==='PENETRATION'&&p.classification==='CONTAINMENT'||filter==='GAP'&&p.complete&&p.classification==='SEPARATED'&&!p.clearanceSatisfied;
 const rows=report?.pairs.filter(visible)??[],selectedPair=report?.pairs.find(p=>interferencePairKey(p)===row);
 const currentResult=!!report&&report.revisionId===view.document.versionId&&!['待更新','来源不可用','尚无结果'].includes(status);
 const canRun=!invalidGap&&!busy&&!saving&&!blocked&&!selectionMissing;
 const options=[['PROBLEMS','需检查'],['ALL','全部'],['PENETRATION','干涉'],['CONTACT','接触'],['GAP','间隙不足'],['INCONCLUSIVE','未完成']].map(([value,label])=>({value,label}));
 const metric=(p:StaticInterferencePair)=>{
  if(!p.complete)return '';
  if(['PENETRATION','CONTAINMENT'].includes(p.classification))return p.commonVolumeMm3>0?`交叠体积 ${interferenceNumber(p.commonVolumeMm3/(scale**3))} ${unit}³`:'';
  return p.classification==='SEPARATED'&&p.distanceExact!==false?`间隙 ${interferenceNumber(p.distanceMm/scale)} ${unit}`:'';
 };
 const pairNames=(p:StaticInterferencePair)=><div className="clash-pair-names">{[p.firstId,p.secondId].map((id,i)=>{const text=report?.bodies.find(b=>b.id===id)?.name??'零件不可用';return <div key={id}><span>{i===0?'A':'B'}</span><span title={text}>{text}</span></div>})}</div>;
 const resultTable=(pairs:StaticInterferencePair[])=><Table className="clash-result-table" size="small" tableLayout="fixed" dataSource={pairs} rowKey={interferencePairKey} pagination={pairs.length>8?{pageSize:8,showSizeChanger:false}:false} locale={{emptyText:'当前筛选下没有结果'}} onRow={p=>({onClick:()=>void selectRow(p),'aria-selected':row===interferencePairKey(p)})} columns={[
  {title:'零件对',render:(_,p)=>pairNames(p)},
  {title:'结果',width:160,render:(_,p)=><div className="clash-pair-result" data-problem={interferenceProblem(p)||undefined}><span>{staticPairLabel(p)}</span>{metric(p)&&<small>{metric(p)}</small>}{detailing&&row===interferencePairKey(p)&&<small>正在计算精确图形…</small>}</div>},
 ]}/>;
 return <CommandDialog id="static-interference" title="碰撞" open size="M" onClose={close} onCancelPick={()=>{if(active===undefined)return false;setActive(undefined);return true}} cancelText="取消" confirmText="确定" onConfirm={keep?save:close} confirmLoading={saving} confirmDisabled={saving||blocked||keep&&(!currentResult||!report?.complete||busy||!!mechanismId)} commandStatus={<CommandStatus pending={busy} error={error} onRetry={()=>void (row&&report?selectRow(report.pairs.find(p=>interferencePairKey(p)===row)!):run())}/>} secondaryActions={busy?<Button onClick={cancel}>取消计算</Button>:<Button disabled={!canRun} onClick={()=>void run()}>检查</Button>}>
 <div className="clash-form" inert={saving}>
  {canSave&&<CommandField label="名称" width="name"><Input aria-label="分析名称" disabled={!keep} value={name} onChange={e=>setName(e.target.value)} placeholder="自动命名"/></CommandField>}
  {(view.product?.kinematics?.mechanisms?.length??0)>0&&<CommandField label="检查位置" width="content"><Select aria-label="检查位置" value={mechanismId??'PRODUCT'} options={[{value:'PRODUCT',label:'已接受装配'},...(view.product?.kinematics?.mechanisms??[]).map(m=>({value:m.id,label:m.name}))]} onChange={id=>{setMechanismId(id==='PRODUCT'?undefined:id);if(id!=='PRODUCT')setKeep(false)}}/></CommandField>}
  <CommandField label="检查范围" width="content"><Select aria-label="检查范围" value={scope} options={[{value:'ALL',label:'全部零件'},{value:'SELECTED',label:'所选零件内部'},{value:'AGAINST_ALL',label:'所选零件与其余零件'},{value:'BETWEEN',label:'两组零件之间'}]} onChange={v=>{setScope(v);setActive(v==='ALL'?undefined:0)}}/></CommandField>
  {scope!=='ALL'&&pickField(0,scope==='BETWEEN'?'第一组':'零件')}{scope==='BETWEEN'&&pickField(1,'第二组')}
  <CommandField label={`最小间隙 ${unit}`} width="number" error={invalidGap?'请输入有效的非负长度':undefined}><InputNumber aria-label="最小间隙" min={0} value={gap/scale} changeOnBlur={false} onChange={v=>setGap((v??0)*scale)}/></CommandField>
  {blocked&&<Alert type="info" title="请先结束位置预览或运动操作。"/>}
  <div className="clash-summary" role="status" aria-label="干涉检查状态"><span>{!report?status:!currentResult?status:!report.complete?'检查未完成':report.pairs.some(interferenceProblem)?'存在需要检查的零件对':'间隙满足'}</span>{report&&<small>{report.checkedPairs} / {report.pairs.length} 对已完成</small>}</div>
  {!!report?.issues.length&&<details className="clash-issues"><summary>{report.issues.length} 个零件未完成检查</summary><ul>{report.issues.map((issue,i)=><li key={i}>{issue.name}：{issue.diagnosticCode==='VARIANT_UNAVAILABLE'?'关联构型不可用':issue.diagnosticCode==='BODY_EVALUATION_FAILED'?'实体求值未完成':'缺少精确实体几何'}</li>)}</ul></details>}
  {report&&<>
   <div className="clash-results-toolbar"><Select aria-label="结果过滤" value={filter} onChange={setFilter} options={options}/><Segmented aria-label="结果排列" value={grouping} onChange={setGrouping} options={[{value:'PAIRS',label:'零件对'},{value:'PARTS',label:'按零件'}]}/></div>
   {mismatch&&<Alert type="info" title="视图区位置与检查时不同，已隐藏标记。"/>}
   {grouping==='PARTS'?interferenceGroups(report).filter(g=>g.pairs.some(visible)).map(g=><details className="clash-result-group" key={g.key}><summary>{g.name} · {g.pairs.filter(visible).length} 对</summary>{resultTable(g.pairs.filter(visible))}</details>):resultTable(rows)}
   {(report.layoutParts>0||report.consumedBodies>0)&&<small className="clash-skipped">未参与：{[report.layoutParts?`${report.layoutParts} 个无实体零件`:'',report.consumedBodies?`${report.consumedBodies} 个已消费实体`:''].filter(Boolean).join('、')}</small>}
   {selectedPair&&<div className="clash-selected-options"><Checkbox checked={isolate} onChange={e=>setIsolate(e.target.checked)}>只看这一对</Checkbox>{selectedPair.detailStatus==='LIMIT'&&<small>局部图形超出显示范围</small>}{selectedPair.detailStatus==='UNAVAILABLE'&&<small>局部图形未完成，可重新选择此行。</small>}</div>}
  </>}
  <Checkbox checked={keep} disabled={!canSave||!!mechanismId} onChange={e=>setKeep(e.target.checked)}>随文档保留</Checkbox>
 </div>
 </CommandDialog>;
}
