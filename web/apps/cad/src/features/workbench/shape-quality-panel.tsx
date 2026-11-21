import {useEffect,useRef,useState} from 'react';
import {Button,InputNumber,Segmented} from 'antd';
import {api} from '../../api/client';
import type {DocumentView,GeometricInput,SelectionItem} from '../../types';
import type {FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {FeaturePickScope} from '../../cad/interaction/feature-selection';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandStatus} from '../../cad/overlay/command-form';
import {CommandInputError} from '../../cad/command/operation-failure';
import {useOperationFailureNotification} from '../../cad/command/operation-feedback';
import {closedBoundaryLoops,curvatureModes,curvatureNumber,curvatureRange,type CurvatureMode,type ShapeQualityReport} from '../../cad/analysis/shape-quality';
import {FeaturePickField} from './feature-pick-field';
import {bindGeometricInput} from './geometric-input-selection';
import {geometryInputName} from './geometric-feature-model';

type RepairKind='SURFACE_FILL'|'SURFACE_SEW'|'CLOSE_SURFACE';
type Props={onRepair?:(kind:RepairKind,inputs:GeometricInput[])=>void;view:DocumentView;onClose:()=>void;onSelectionSession:(s?:FeatureSelectionSession)=>void;onShow:(report?:ShapeQualityReport,mode?:CurvatureMode,gain?:number)=>void;onLocatePoints?:(points:import('../../types').Vec3[])=>void};
export function ShapeQualityPanel({view,onClose,onSelectionSession,onShow,onLocatePoints,onRepair}:Props){
 const [input,setInput]=useState<GeometricInput>(),[pick,setPick]=useState<SelectionItem>(),[picking,setPicking]=useState(true),[filter,setFilter]=useState<'ANY'|'CURVE'|'SURFACE'>('ANY'),[binding,setBinding]=useState(false),[density,setDensity]=useState(256),[mode,setMode]=useState<CurvatureMode>('mean'),[gain,setGain]=useState(1);
 const [report,setReport]=useState<ShapeQualityReport>(),[busy,setBusy]=useState(false),[failure,setFailure]=useState<unknown>();
 const base=useRef(view.document.versionId).current,serial=useRef(0),scope=useRef(new FeaturePickScope()),callbacks=useRef({onSelectionSession,onShow});callbacks.current={onSelectionSession,onShow};
 const valid=base===view.document.versionId,key=JSON.stringify([view.document.id,base,input,density,valid,filter]);scope.current.activate(`${picking}/${filter}`,`${view.document.id}/${view.document.versionId}`);
 useOperationFailureNotification(failure,'曲率分析');
 useEffect(()=>()=>{serial.current++;scope.current.cancel();callbacks.current.onShow();callbacks.current.onSelectionSession()},[]);
 useEffect(()=>{
  const accept=(s:SelectionItem)=>{
   if(!valid||(s.ownerDocumentId??s.documentId)!==view.document.id||s.occurrencePath||s.versionId&&s.versionId!==base)return false;
   if(s.kind==='edge')return filter!=='SURFACE';if(s.kind==='face')return filter!=='CURVE';
   const domain=view.part?.features.find(f=>f.id===(s.result?.featureId??s.entityId??s.id))?.result?.domain;
   return filter==='CURVE'?domain==='WIRE_SET':filter==='SURFACE'?domain==='SHEET_SET':['WIRE_SET','SHEET_SET','MIXED_SET'].includes(domain??'');
  };
  if(!picking||!valid){callbacks.current.onSelectionSession();return}
  callbacks.current.onSelectionSession({documentId:view.document.id,versionId:base,role:filter==='CURVE'?'edge':filter==='SURFACE'?'face':'geometry',topologyPick:filter==='CURVE'?'EDGE':undefined,selections:pick?[pick]:[],accept,onPick:s=>{if(!accept(s))return;const ticket=scope.current.latest();setBinding(true);setFailure(undefined);void bindGeometricInput(view,s,base,view.part!.features).then(value=>{if(!scope.current.current(ticket))return;setBinding(false);setInput(value);setPick(s);setPicking(false)}).catch(error=>{if(scope.current.current(ticket))setFailure(error)}).finally(()=>{if(scope.current.current(ticket))setBinding(false)})}});
  return()=>callbacks.current.onSelectionSession();
 },[picking,valid,base,filter]);
 useEffect(()=>{
  const ticket=++serial.current,abort=new AbortController();setBinding(false);setReport(undefined);callbacks.current.onShow();setFailure(valid?undefined:new CommandInputError("模型已改变，请重新打开曲率分析"));setBusy(false);
  if(!input||!valid)return;
  setBusy(true);void api.shapeQuality(view.document.id,{revisionId:base,input,sampleCount:density},abort.signal).then(result=>{
   if(abort.signal.aborted||serial.current!==ticket)return;
   if(result.documentId!==view.document.id||result.revisionId!==base)throw new CommandInputError('曲率分析来源已改变');
   if(!result.result.complete)throw new CommandInputError(result.result.diagnostic??'曲率分析未完成');setReport(result);
  }).catch(error=>{if(!abort.signal.aborted&&serial.current===ticket)setFailure(error)}).finally(()=>{if(!abort.signal.aborted&&serial.current===ticket)setBusy(false)});
  return()=>abort.abort();
 },[key]);
 useEffect(()=>{callbacks.current.onShow(valid?report:undefined,mode,gain)},[report,mode,gain,valid]);
 const range=report?curvatureRange(report,mode):undefined,surface=report?.result.samples.some(s=>s.surface),unit=surface&&mode==='gaussian'?'1/mm²':'1/mm';
 const whole=report?.input.kind==='RESULT' ? view.part?.features.find(f=>f.id===report.input.featureId && f.result?.id===report.input.resultId)?.result : undefined;
 const loops=report?.result.boundaryComplete && whole?.domain==='SHEET_SET' ? closedBoundaryLoops(report.result.boundaryEdges??[]) : [];
 const fillLoop=async(edgeIds:number[])=>{
  if(!report||!whole||!valid||!onRepair)return;
  const ticket=++serial.current,inputs:GeometricInput[]=[];
  setBinding(true);
  try {
   for(const localId of edgeIds){
    const selection=await api.bindTopologySelection(view.document.id,{sourceVersionId:base,geometryKey:whole.geometryKey,kind:'EDGE',localId});
    if(serial.current!==ticket)return;
    if(selection.sourceResult?.featureId!==report.input.featureId || selection.sourceResult?.resultId!==whole.id)
     throw new CommandInputError('边界的来源已变化，请重新分析');
    inputs.push({kind:'TOPOLOGY',selection:{selection,sourceVersionId:base,sourceFeatureId:report.input.featureId}});
   }
   onRepair('SURFACE_FILL',inputs);
  }catch(e){if(serial.current===ticket)setFailure(e)}
  finally{if(serial.current===ticket)setBinding(false)}
 };
 return <CommandDialog id="shape-quality" open title="曲率分析" size="S" onClose={onClose} onConfirm={onClose} confirmText="完成" onCancelPick={()=>{if(!picking)return false;scope.current.cancel();setBinding(false);setPicking(false);return true}} commandStatus={<CommandStatus pending={busy||binding}/>}>
  <FeaturePickField compact label="对象" value={geometryInputName(view,input)} selected={!!input} active={picking} disabled={!valid} onActivate={()=>setPicking(true)} onClear={input?()=>{scope.current.cancel();setBinding(false);setInput(undefined);setPick(undefined);setPicking(true)}:undefined}/>
  <CommandField label="选择过滤"><Segmented<'ANY'|'CURVE'|'SURFACE'> value={filter} options={[{label:'任意',value:'ANY'},{label:'曲线',value:'CURVE'},{label:'曲面',value:'SURFACE'}]} onChange={value=>{scope.current.cancel();setBinding(false);setInput(undefined);setPick(undefined);setPicking(true);setFilter(value)}}/></CommandField>
  <CommandField label="采样"><Segmented value={density} options={[{label:'疏',value:64},{label:'中',value:256},{label:'密',value:1024}]} onChange={setDensity}/></CommandField>
  {surface&&<CommandField label="分析"><Segmented<'curvature'|'boundaries'|'gaps'|'normals'> value={['normals','boundaries','gaps'].includes(mode)?mode as 'normals'|'boundaries'|'gaps':'curvature'} options={[{label:'曲率',value:'curvature'},{label:'自由边',value:'boundaries'},{label:'缝隙',value:'gaps'},{label:'法向',value:'normals'}]} onChange={value=>setMode(value==='curvature'?'mean':value)}/></CommandField>}
  {surface&&!['normals','boundaries','gaps'].includes(mode)&&<CommandField label="曲率"><Segmented<CurvatureMode> value={mode} options={curvatureModes.map(o=>({...o,label:({minimum:'最小',maximum:'最大',mean:'平均',gaussian:'高斯'})[o.value]}))} onChange={setMode}/></CommandField>}
  {mode==='normals'&&<CommandField label="长度比例"><InputNumber min={.1} max={10} step={.1} value={gain} onChange={n=>setGain(n??1)}/></CommandField>}
  {(mode==='boundaries'||mode==='gaps')&&report&&<div className="shape-quality-result"><p>{report.result.boundaryEdges?.length??0} 条自由边 · {report.result.gapCandidates?.length??0} 处间隙候选</p>{report.result.boundaryComplete===false&&<p>部分诊断未完成，请选择较小范围。</p>}{mode==='gaps'&&<div className="object-impact-list">{report.result.gapCandidates?.map((gap,i)=><Button type="text" key={i} onClick={()=>onLocatePoints?.([gap.first,gap.second])}>边 {gap.firstEdge} / {gap.secondEdge}<small>{curvatureNumber(gap.distanceMm)} mm · 定位</small></Button>)}</div>}</div>}
  {mode==='boundaries'&&onRepair&&valid&&whole?.domain==='SHEET_SET'&&report&&<div className="object-impact-list">
    {loops.map((edges,i)=><div key={edges[0].edgeId}>
      <Button type="text" onClick={()=>onLocatePoints?.(edges.flatMap(e=>e.points))}>定位边界环 {i+1}（{edges.length} 条边）</Button>
      <Button type="link" disabled={binding||edges.length>128} onClick={()=>void fillLoop(edges.map(e=>e.edgeId))}>填充此环</Button>
    </div>)}
    {report.result.boundaryComplete&&report.result.boundaryEdges?.length===0 ?
      <Button onClick={()=>onRepair('CLOSE_SURFACE',[report.input])}>预览封闭为实体</Button> :
      <Button onClick={()=>onRepair('SURFACE_SEW',[report.input])}>选择相邻面缝合</Button>}
  </div>}
  {report?.result.samples.some(s=>!s.surface)&&<CommandField label="梳齿比例"><InputNumber min={.1} max={10} step={.1} value={gain} onChange={n=>setGain(n??1)}/></CommandField>}
  {range&&<div className="shape-quality-result" role="region" aria-label="曲率分析结果"><dl><div><dt>采样最小值</dt><dd>{curvatureNumber(range[0])} {unit}</dd></div><div><dt>采样最大值</dt><dd>{curvatureNumber(range[1])} {unit}</dd></div></dl>{surface&&<div className="curvature-color-legend" aria-label="蓝色为最小值，红色为最大值"/>}<p>{report!.result.samples.length} 个有效采样 · {report!.result.undefinedSamples} 个未定义位置</p><p>精确几何求值；范围与图形为采样结果。</p></div>}
 </CommandDialog>;
}
