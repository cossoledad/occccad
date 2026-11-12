import {useEffect,useRef,useState} from 'react';
import {Alert,Button,Checkbox,Input,Select,Tag} from 'antd';
import type {DocumentView,SelectionItem} from '../../types';
import {api} from '../../api/client';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandStatus} from '../../cad/overlay/command-form';
import type {FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {FeaturePickField} from './feature-pick-field';
import {defaultMeasurement,measurementCount,measurementPick,measurementProblem,measurementStatus,measurementSupportLabel,measurementTitle,measurementValue,measurementValueLabel,type Measurement,type MeasurementKind,type MeasurementMode,type MeasurementReport,type MeasurementUnit} from '../../cad/measurement/measurement';
import {recordDeveloperEvidence} from '../../cad/command/developer-evidence';

type Props={view:DocumentView;kind:MeasurementKind;initial?:Measurement;unit:MeasurementUnit;seed:SelectionItem[];canSave:boolean;blocked:boolean;onClose:()=>void;onSelectionSession:(s?:FeatureSelectionSession)=>void;onShow:(r?:MeasurementReport)=>void;onLocate:(s:SelectionItem[])=>void;onSave:(m:Measurement,action?:'SAVE_MEASUREMENT'|'UPDATE_MEASUREMENT')=>Promise<void>};
export function MeasurementPanel({view,kind,initial,unit,seed,canSave,blocked,onClose,onSelectionSession,onShow,onLocate,onSave}:Props){
 const [draft,setDraft]=useState<Measurement>(()=>initial??{...defaultMeasurement(kind),supports:seed.flatMap(s=>measurementPick(view,s)??[]).slice(0,measurementCount(kind))});
 const [picks,setPicks]=useState<Array<SelectionItem|undefined>>(()=>initial?[]:seed.slice(0,measurementCount(kind)));
 const [active,setActive]=useState<number|undefined>(()=>draft.supports.length<measurementCount(kind)?draft.supports.length:undefined),[filter,setFilter]=useState(kind==='ANGLE_POINTS'?'VERTEX':'ANY');
 const [mode,setMode]=useState<MeasurementMode>('GEOMETRY'),[continuation,setContinuation]=useState('BETWEEN'),[keep,setKeep]=useState(canSave),[busy,setBusy]=useState(false),[saving,setSaving]=useState(false),[error,setError]=useState<string>();
 const [report,setReport]=useState<MeasurementReport|undefined>(()=>initial?.result?{documentId:view.document.id,revisionId:view.document.versionId,definition:initial,result:initial.result}:undefined);
 const serial=useRef(0),abort=useRef<AbortController|undefined>(undefined),mounted=useRef(true);
 const callbacks=useRef({onShow,onSelectionSession,onSave,onClose});callbacks.current={onShow,onSelectionSession,onSave,onClose};
 const scope=JSON.stringify([view.document.id,view.document.versionId,draft.kind,draft.supports,blocked]),current=useRef(scope);current.current=scope;
 const ready=draft.supports.length===measurementCount(draft.kind)&&draft.supports.every(Boolean);
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;serial.current++;abort.current?.abort();callbacks.current.onSelectionSession();callbacks.current.onShow()}},[]);
 useEffect(()=>{
  if(blocked){callbacks.current.onShow();return}
  if(initial&&draft.kind===initial.kind&&JSON.stringify(draft.supports)===JSON.stringify(initial.supports)&&initial.result){setReport({documentId:view.document.id,revisionId:view.document.versionId,definition:draft,result:initial.result});if(initial.status==='CURRENT')callbacks.current.onShow({documentId:view.document.id,revisionId:view.document.versionId,definition:draft,result:initial.result});return}
  serial.current++;abort.current?.abort();setReport(undefined);callbacks.current.onShow();setError(undefined);setBusy(false);
  if(!ready)return;
  const controller=new AbortController(),ticket=serial.current,key=scope;abort.current=controller;
  const timer=setTimeout(()=>{setBusy(true);void api.measure(view.document.id,{revisionId:view.document.versionId,definition:draft},controller.signal).then(r=>{
   if(controller.signal.aborted||ticket!==serial.current||key!==current.current)return;
   recordDeveloperEvidence('几何测量',r);setReport(r);if(!r.result.complete)setError(measurementProblem(r.result.diagnostic));
  }).catch(e=>{if(!controller.signal.aborted&&ticket===serial.current){recordDeveloperEvidence('几何测量失败',e);setError(measurementProblem(e))}}).finally(()=>{if(ticket===serial.current&&!controller.signal.aborted)setBusy(false)})},160);
  return()=>{clearTimeout(timer);controller.abort()};
 },[scope]);
 useEffect(()=>{if(report?.result.complete&&!blocked&&(initial?.status==='CURRENT'||report.result!==initial?.result))callbacks.current.onShow({...report,definition:{...report.definition,name:draft.name,visible:draft.visible,showComponents:draft.showComponents}})},[draft.showComponents,draft.visible,draft.name,report,blocked]);
 useEffect(()=>{
  if(active===undefined||saving||blocked){callbacks.current.onSelectionSession();return}
  const accept=(s:SelectionItem)=>!!measurementPick(view,s,mode,filter);
  callbacks.current.onSelectionSession({role:'geometry',topologyPick:filter==='VERTEX'?'VERTEX':filter==='EDGE'?'EDGE':undefined,project:s=>filter==='BODY'&&['face','edge','vertex'].includes(s.kind)?{...s,kind:'body',id:s.bodyId??s.id}:s,documentId:view.document.id,versionId:view.document.versionId,selections:picks.filter((s):s is SelectionItem=>!!s),accept,onPick:s=>{
   const support=measurementPick(view,s,mode,filter);if(!support)return;
   setDraft(d=>{const supports=[...d.supports];supports[active]=support;return {...d,supports,result:undefined,status:undefined}});
   setPicks(p=>{const next=[...p];next[active]=s;return next});setActive(active+1<measurementCount(draft.kind)?active+1:undefined);
  }});return()=>callbacks.current.onSelectionSession();
 },[active,scope,mode,filter,saving]);
 const save=async(close=true,action:'SAVE_MEASUREMENT'|'UPDATE_MEASUREMENT'=initial&&initial.status!=='CURRENT'?'UPDATE_MEASUREMENT':'SAVE_MEASUREMENT')=>{
  if(saving||blocked)return false;setActive(undefined);
  if(!keep&&close){onClose();return true}
  if(!canSave||!ready)return false;
  setSaving(true);setError(undefined);
  try{const source=report?.result.complete?report.definition:draft;await callbacks.current.onSave({...draft,supports:source.supports,result:undefined,status:undefined},action);if(mounted.current&&close)callbacks.current.onClose();return true}
  catch(e){if(mounted.current)setError(measurementProblem(e));return false}finally{if(mounted.current)setSaving(false)}
 };
 const next=async()=>{
  if(keep&&canSave&&!await save(false))return;
  const retained=draft.kind==='BETWEEN'&&continuation!=='BETWEEN'?(continuation==='FAN'?0:1):undefined;
  const supports=report?.result.complete?report.definition.supports:draft.supports;
  setDraft({...defaultMeasurement(draft.kind),supports:retained===undefined?[]:[supports[retained]]});setPicks(retained===undefined?[]:[picks[retained]]);setActive(retained===undefined?0:1);setReport(undefined);
 };
 const summary=report?.result.complete?{...draft,result:report.result}:undefined;
 return <CommandDialog id="geometry-measurement" open title={measurementTitle(draft.kind)} size="M" onClose={onClose} onConfirm={async()=>{await save()}} confirmText="确定" cancelText="取消" confirmLoading={saving} confirmDisabled={blocked||busy||keep&&(!canSave||!ready||!summary)} onCancelPick={()=>{if(active===undefined)return false;setActive(undefined);return true}}
  commandStatus={<CommandStatus pending={busy} error={error}/>} secondaryActions={<Button disabled={!summary||busy||saving||blocked} onClick={()=>void next()}>{keep?'保留并继续':'继续测量'}</Button>}>
  <div className="measurement-form" inert={saving}>
   <CommandField label="名称" width="name"><Input disabled={!canSave||!keep} aria-label="测量名称" placeholder="自动命名" value={draft.name} onChange={e=>setDraft(d=>({...d,name:e.target.value}))}/></CommandField>
   <div className="measurement-pick-options"><CommandField label="选择过滤" width="enum"><Select aria-label="测量选择过滤" value={filter} options={[['ANY','任意几何'],['VERTEX','顶点'],['EDGE','边'],['FACE','面'],['BODY','实体 / 组件']].map(([value,label])=>({value,label}))} onChange={setFilter}/></CommandField>
   <CommandField label="取用方式" width="enum"><Select aria-label="测量取用方式" value={mode} options={[{value:'GEOMETRY',label:'实际几何边界'},{value:'CENTER',label:'圆心 / 球心'},{value:'AXIS',label:'解析轴线'}]} onChange={setMode}/></CommandField></div>
   {Array.from({length:measurementCount(draft.kind)},(_,index)=>{const support=draft.supports[index],label=draft.kind==='ITEM'?'测量对象':draft.kind==='ANGLE_POINTS'?['第一点','角度顶点','第三点'][index]:`对象 ${index+1}`;return <FeaturePickField compact key={index} label={label} active={active===index} value={support?measurementSupportLabel(support,view):undefined} disabled={blocked||saving} onActivate={()=>setActive(index)} onCancel={()=>setActive(undefined)} onClear={support?()=>{setDraft(d=>({...d,supports:d.supports.slice(0,index)}));setPicks(p=>p.slice(0,index));setActive(index)}:undefined} onLocate={picks[index]?()=>onLocate([picks[index]!]):undefined}/>})}
   {draft.kind==='BETWEEN'&&<CommandField label="连续方式" width="enum"><Select aria-label="连续测量方式" value={continuation} options={[{value:'BETWEEN',label:'独立测量'},{value:'CHAIN',label:'链式 · 末项作为下一首项'},{value:'FAN',label:'扇形 · 固定首项'}]} onChange={setContinuation}/></CommandField>}
   {blocked&&<Alert type="info" title="请先结束位置预览或运动操作。"/>}
   {initial&&initial.status!=='CURRENT'&&<Alert type="info" title={`${measurementStatus(initial.status)}，显示上次结果。`}/>}
   {summary&&<div className="measurement-results" role="region" aria-label="测量结果"><strong>测量结果</strong>
    <dl>{Object.entries(report!.result.values).map(([key,value])=><div key={key}><dt>{measurementValueLabel(key)}</dt><dd>{measurementValue(key,value,unit)}{initial?.outputCurrency?.[key]&&initial.outputCurrency[key]!=="CURRENT"&&<Tag color="warning">上次值 · 待更新</Tag>}</dd></div>)}</dl>
    {Object.entries(report!.result.anchors).filter(([key])=>['first','second','position','center','centroid','vertex','axisOrigin','apex'].includes(key)&&!(key==='position'&&['xMm','yMm','zMm'].every(k=>k in report!.result.values))).map(([key,p])=><div key={key} className="measurement-coordinate"><span>{{first:'点 1',second:'点 2',position:'坐标',center:'几何中心',centroid:'几何形心',vertex:'顶点',axisOrigin:'轴线上一点',apex:'锥顶'}[key]}</span><output>{p.map(v=>measurementValue('xMm',v,unit)).join(', ')}</output></div>)}

   </div>}
   <div className="measurement-options"><Checkbox checked={keep} disabled={!canSave} onChange={e=>setKeep(e.target.checked)}>随文档保留</Checkbox><Checkbox checked={draft.visible} onChange={e=>setDraft(d=>({...d,visible:e.target.checked}))}>显示标注</Checkbox>{draft.kind==='BETWEEN'&&<Checkbox checked={draft.showComponents} onChange={e=>setDraft(d=>({...d,showComponents:e.target.checked}))}>显示坐标分量</Checkbox>}</div>
  </div>
 </CommandDialog>;
}
