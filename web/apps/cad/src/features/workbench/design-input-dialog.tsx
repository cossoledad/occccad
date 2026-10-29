import {Button,Checkbox,Input,Select} from 'antd';
import {useEffect,useMemo,useRef,useState} from 'react';
import {api} from '../../api/client';
import type {Artifact,ContextInput,DocumentView,SelectionItem} from '../../types';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandStatus} from '../../cad/overlay/command-form';
import type {FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {FeaturePickField} from './feature-pick-field';
import {useFeaturePreview} from './use-feature-preview';
import {selectionInSnapshot} from './publication-target';
import {parameterDisplayValue} from './parameter-editor';
import {formatDisplayNumber as number} from '../../utils/display-number';
import {engineeringProblem} from '../../cad/command/operation-feedback';

type Target={key:string;id:string;sketchId?:string;name:string;selection:SelectionItem;description:string};
export function DesignInputDialog({view,onClose,onApply,onPreview,onSelectionSession,onLocate}:{view:DocumentView;onClose:()=>void;onApply:(v:DocumentView)=>Promise<unknown>;onPreview:(a?:Artifact)=>void;onSelectionSession:(s?:FeatureSelectionSession)=>void;onLocate:(s:SelectionItem)=>void}){
 const [baseline,setBaseline]=useState(view.document.versionId),[type,setType]=useState<ContextInput['type']>('PARAMETER'),[target,setTarget]=useState(''),[name,setName]=useState(''),[required,setRequired]=useState(false),[connect,setConnect]=useState(true),[editing,setEditing]=useState<ContextInput>(),[picking,setPicking]=useState(false),[busy,setBusy]=useState(false),[error,setError]=useState<string>();
 const alive=useRef(true),callbacks=useRef({onSelectionSession,onPreview});callbacks.current={onSelectionSession,onPreview};
 const valid=baseline===view.document.versionId;
 const targets=useMemo(()=>{
  const p=view.part;if(!p)return [];
  const identity={documentId:view.document.id,versionId:baseline};
  if(type==='PARAMETER')return (p.parameters??[]).map(q=>({key:q.parameterId,id:q.parameterId,name:q.displayAlias||q.displayName||q.label,selection:{...identity,kind:'parameter',id:q.parameterId},description:parameterDisplayValue(q)} as Target));
  if(type==='PLANE')return (p.datumPlanes??[]).map(q=>({key:q.id,id:q.id,name:q.name,selection:{...identity,kind:'plane',id:q.id,entityId:q.id,plane:'CUSTOM'},description:`原点 (${q.origin.map(v=>number(v)).join(', ')}) mm · 法向 (${q.normal.map(v=>number(v)).join(', ')})`} as Target));
  if(type==='AXIS')return (p.datumAxes??[]).map(q=>({key:q.id,id:q.id,name:q.name,selection:{...identity,kind:'axis',id:q.id,entityId:q.id,axis:'DATUM'},description:`原点 (${q.origin.map(v=>number(v)).join(', ')}) mm · 方向 (${q.direction.map(v=>number(v)).join(', ')})`} as Target));
  return p.features.filter(f=>!f.suppressed).flatMap(f=>(f.sketch?.entities??[]).filter(e=>!e.suppressed&&(type==='POINT'?e.kind==='POINT'&&!e.sourceEntityId:['LINE','CIRCLE','ARC'].includes(e.kind))).map(e=>({key:`${f.id}/${e.id}`,id:e.id,sketchId:f.id,name:`${f.name} / ${e.name||({POINT:'点',LINE:'直线',CIRCLE:'圆',ARC:'圆弧'} as Record<string,string>)[e.kind]}`,selection:{...identity,kind:'visual',id:e.id,entityId:e.id,featureId:f.id,visualType:e.kind==='POINT'?'POINT':'CURVE'},description:e.point?`(${number(e.point.x)}, ${number(e.point.y)}) mm`:e.radius?`半径 ${number(e.radius)} mm`:e.start&&e.end?`(${number(e.start.x)}, ${number(e.start.y)}) → (${number(e.end.x)}, ${number(e.end.y)}) mm`:'草图默认几何'} as Target)));
 },[view.part,baseline,type]);
 const selected=targets.find(t=>t.key===target);
 const editingTarget=editing?targets.find(t=>t.sketchId? t.sketchId===editing.target.targetId:t.id===editing.target.targetId):undefined;
 const defaultGeometry=editing?.default?.external;
 const defaultReading=defaultGeometry?.point?`(${number(defaultGeometry.point.x)}, ${number(defaultGeometry.point.y)}) mm`:defaultGeometry?.radius?`半径 ${number(defaultGeometry.radius)} mm`:defaultGeometry?.start&&defaultGeometry.end?`(${number(defaultGeometry.start.x)}, ${number(defaultGeometry.start.y)}) → (${number(defaultGeometry.end.x)}, ${number(defaultGeometry.end.y)}) mm`:editingTarget?.description;

 const targetFor=(s:SelectionItem)=>selectionInSnapshot(s,view.document.id,baseline)?targets.find(t=>s.kind==='visual'?s.featureId===t.sketchId&&s.entityId===t.id:!t.sketchId&&(s.entityId??s.id)===t.id):undefined;
 useEffect(()=>{if(!picking||!valid)return;callbacks.current.onSelectionSession({role:type==='PLANE'?'plane':type==='AXIS'?'axis':type==='POINT'?'sketch-point':'geometry',sketchIds:[...new Set(targets.flatMap(t=>t.sketchId?[t.sketchId]:[]))],sketchPoints:targets.flatMap(t=>t.sketchId?[{sketchId:t.sketchId,entityId:t.id}]:[]),documentId:view.document.id,versionId:baseline,selections:[],accept:s=>!!targetFor(s),onPick:s=>{const t=targetFor(s);if(t){setTarget(t.key);setPicking(false)}}});return()=>callbacks.current.onSelectionSession()},[picking,valid,baseline,targets]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;callbacks.current.onSelectionSession();callbacks.current.onPreview()}},[]);
 const reset=()=>{setEditing(undefined);setName('');setTarget('');setRequired(false);setPicking(false);setError(undefined)};
 const command=useMemo(()=>!valid?undefined:editing?name.trim()?{type:'EDIT_CONTEXT_INPUT',versionId:baseline,contextInputId:editing.id,name:name.trim(),required}:undefined:selected?{type:'CREATE_CONTEXT_INPUT',versionId:baseline,publicationType:type,targetKind:selected.sketchId?'SKETCH_EXTERNAL_GEOMETRY':type==='PARAMETER'?'PARAMETER':'DATUM',targetId:selected.sketchId??selected.id,sketchId:selected.sketchId,ownerEntityId:selected.sketchId?selected.id:undefined,connectInputPoint:!!selected.sketchId&&type==='POINT'&&connect,name:name.trim()||undefined,required}:undefined,[valid,baseline,editing,name,required,selected,type,connect]);
 const preview=useFeaturePreview(view.document.id,baseline,command,undefined,onPreview);
 const apply=async(again=false)=>{if(!command||!preview.previewId||busy)return;setBusy(true);setPicking(false);try{const next=await api.command(view.document.id,{...command,previewId:preview.previewId});if(!alive.current)return;await onApply(next);if(!alive.current)return;if(again){setBaseline(next.document.versionId);reset()}else onClose()}catch(e){if(alive.current)setError(engineeringProblem(e))}finally{if(alive.current)setBusy(false)}};
 const canConfirm=valid&&!!command&&!preview.pending&&!!preview.previewId&&!preview.error&&!busy;
 return <CommandDialog id="design-inputs" open title="设计输入" size="M" placement="right" onClose={onClose} onConfirm={()=>void apply()} confirmLoading={busy} confirmDisabled={!canConfirm} onCancelPick={()=>{if(!picking)return false;setPicking(false);return true}} secondaryActions={<Button disabled={!canConfirm} onClick={()=>void apply(true)}>应用并继续</Button>} commandStatus={<CommandStatus pending={preview.pending} error={!valid?'零件已更新，请重新打开':error||preview.error} onRetry={preview.retry}/>}>
  <CommandField label="输入类型"><Select aria-label="输入类型" disabled={!!editing} value={type} onChange={v=>{setType(v);setTarget('');setPicking(false)}} options={[['PARAMETER','参数'],['PLANE','平面'],['AXIS','轴'],['POINT','草图点'],['CURVE','草图曲线']].map(([value,label])=>({value,label}))}/></CommandField>
  {!editing&&<><FeaturePickField label={selected?.sketchId?'默认图元':'本地对象'} value={selected?.name} active={picking} selected={!!selected} onActivate={()=>setPicking(true)} onCancel={()=>setPicking(false)} onClear={()=>setTarget('')} onLocate={selected?()=>onLocate(selected.selection):undefined}/><CommandField label="本地对象" width="expression"><Select aria-label="输入本地对象" showSearch optionFilterProp="label" placeholder="选择本地默认对象" value={target||undefined} onChange={v=>{setTarget(v);setPicking(false)}} options={targets.map(t=>({value:t.key,label:t.name}))}/></CommandField>{selected&&<CommandField label="独立打开" width="expression"><span>{selected.description}</span></CommandField>}{selected?.sketchId&&type==='POINT'&&<Checkbox checked={connect} onChange={e=>setConnect(e.target.checked)}>关联此本地点</Checkbox>}</>}
  {editing&&<><CommandField label="本地对象" width="expression"><span>{editing.target.kind==='SKETCH_EXTERNAL_GEOMETRY'?view.part?.features.find(f=>f.id===editing.target.targetId)?.name:editingTarget?.name??'本地对象已丢失'}</span></CommandField><CommandField label="独立默认" width="expression"><span>{defaultReading??'几何默认值'}</span></CommandField></>}
  <CommandField label="名称" width="name"><Input aria-label="设计输入名称" placeholder="自动命名" value={name} onChange={e=>setName(e.target.value)}/></CommandField>
  <Checkbox checked={required} onChange={e=>setRequired(e.target.checked)}>替换时必须关联此输入</Checkbox>
  <FeaturePickField label="已有输入" active={false} disabled={!valid||busy} items={(view.part?.contextInputs??[]).map(i=>({key:i.id,value:i.name,detail:({PARAMETER:'参数',PLANE:'平面',AXIS:'轴',POINT:'草图点',CURVE:'草图曲线'} as Record<string,string>)[i.type],actions:<Button size="small" disabled={!valid||busy} onClick={()=>{setEditing(i);setType(i.type);setName(i.name);setRequired(!!i.required);setPicking(false);setError(undefined)}}>编辑</Button>,onLocate:()=>{const t=i.target,identity={documentId:view.document.id,versionId:view.document.versionId,id:t.targetId,entityId:t.targetId};onLocate(t.kind==='PARAMETER'?{...identity,kind:'parameter'}:t.kind==='SKETCH_EXTERNAL_GEOMETRY'?{...identity,kind:'sketch'}:i.type==='AXIS'?{...identity,kind:'axis',axis:'DATUM'}:{...identity,kind:'plane',plane:'CUSTOM'})},onClear:()=>{setBusy(true);void api.command(view.document.id,{type:"DELETE_CONTEXT_INPUT",contextInputId:i.id,versionId:baseline}).then(async next=>{if(!alive.current)return;await onApply(next);if(alive.current){setBaseline(next.document.versionId);reset()}}).catch(e=>{if(alive.current)setError(engineeringProblem(e))}).finally(()=>{if(alive.current)setBusy(false)})}}))}/>
  {editing&&<Button onClick={reset}>新建输入</Button>}
 </CommandDialog>;
}
