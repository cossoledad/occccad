import {bindGeometricInput} from './geometric-input-selection';
import {Button,Input,InputNumber,Segmented,Select} from 'antd';
import {useEffect,useRef,useState} from 'react';
import type {Artifact,DocumentView,Feature,GeometricInput,SelectionItem} from '../../types';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandStatus,CommandVector} from '../../cad/overlay/command-form';
import {CommandInputError} from '../../cad/command/operation-failure';
import {FeaturePickScope,type FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {randomUUID} from '../../utils/random-uuid';
import {displayLengthToMillimeters,millimetersToDisplayLength,type DisplayLengthUnit} from '../../state/ui-preferences';
import {FeaturePickField} from './feature-pick-field';
import {ParameterReferenceInput} from './parameter-reference-input';
import {parseParameterSource,parameterSourceText} from './parameter-editor';
import {useFeaturePreview} from './use-feature-preview';
import {useFeatureDialogFeedback} from './use-feature-dialog-feedback';
import {geometryFields,geometryInputName,geometryInputRoles,geometryNames,sketchGeometryInput,splineControlMode} from './geometric-feature-model';

type Props={view:DocumentView;feature:Feature;digest?:string;unit:DisplayLengthUnit;seed:SelectionItem[];onClose:()=>void;onApply:(input:Record<string,unknown>)=>Promise<unknown>;onPreview:(artifact?:Artifact,operation?:Feature['operation'])=>void;onSelectionSession:(session?:FeatureSelectionSession)=>void};
export function GeometricFeatureEditor({view,feature,digest,unit,seed,onClose,onApply,onPreview,onSelectionSession}:Props){
 const [draft,setDraft]=useState(()=>structuredClone(feature));
 const initial=()=>Object.fromEntries(geometryFields(structuredClone(feature.geometry!)).map(f=>{const p=view.part?.parameters?.find(p=>p.ownerFeatureId===feature.id&&p.propertySlot===f.slot);return [f.slot,p?parameterSourceText(p,f.unit==='mm'?unit:f.unit,true):String(f.unit==='mm'?millimetersToDisplayLength(f.value,unit):f.value)]}));
 const [values,setValues]=useState(initial),[committed,setCommitted]=useState(initial);
 const [picking,setPicking]=useState<string|undefined>(()=>geometryInputRoles(feature.geometry!.kind).length?'input:0':undefined);
 const [failure,setFailure]=useState<unknown>(),[binding,setBinding]=useState(false),[saving,setSaving]=useState(false);
 const base=useRef(view.document.versionId).current,intent=useRef(randomUUID()).current,alive=useRef(true),scope=useRef(new FeaturePickScope()),bindingRequest=useRef(0);
 const current=useRef({draft,picking});current.current={draft,picking};const callbacks=useRef({onPreview,onSelectionSession});callbacks.current={onPreview,onSelectionSession};
 scope.current.activate(picking,`${view.document.id}/${base}`);
 const kind=draft.geometry!.kind,roles=geometryInputRoles(kind),multiple=kind==='COMPOSITE';
 // A cleared draft role stays empty until repicked; later supports must keep
 // their original roles. Incomplete slots never enter the preview/commit DTO.
 const inputs=draft.geometry!.inputs??[];
 const accept=(s:SelectionItem,field=picking)=>{
  if((s.ownerDocumentId??s.documentId)!==view.document.id||(s.occurrencePath??'')!==''||s.versionId&&s.versionId!==base)return false;
  if(field==='body')return !!s.bodyId;
  if(field==='frame')return s.kind==='plane';
  if(field?.startsWith('point:'))return s.kind==='vertex'||s.entityKind==='REFERENCE_POINT'||!!view.part?.features.find(f=>f.id===(s.entityId??s.id)&&f.geometry?.kind==='POINT_ON_CURVE');
  const role=roles[Number(field?.split(':')[1])]?.role??'curve';
  if(s.kind==='plane')return role==='plane'||kind==='INTERSECTION'||kind==='PROJECT';
  if(role==='plane')return false;
  if(s.kind==='edge')return role==='curve'||role==='geometry';
  if(s.kind==='face')return role==='surface'||role==='geometry';
  if(sketchGeometryInput(view,s))return role==='curve'||role==='geometry';
  const source=view.part?.features.find(f=>f.id===(s.result?.featureId??s.entityId??s.id));
  return !!source?.result&&source.id!==feature.id&&(role==='geometry'||role==='surface'&&source.result.domain==='SHEET_SET'||role==='curve'&&source.result.domain==='WIRE_SET');
 };
 const bind=(s:SelectionItem)=>bindGeometricInput(view,s,base);
 const pick=async(s:SelectionItem)=>{
  const field=current.current.picking,ticket=scope.current.capture();if(!field||!accept(s,field)||binding)return;
  const request=++bindingRequest.current;setBinding(true);setFailure(undefined);
  try{
   if(field==='body'){setDraft(f=>({...f,bodyId:s.bodyId}));setPicking(undefined);return}
   if(field==='frame'){setDraft(f=>({...f,geometry:{...f.geometry!,curve:{...f.geometry!.curve!,framePlaneId:s.entityId??(s.kind==='plane'?s.datumPlane?.id:undefined)??s.id}}}));setPicking(undefined);return}
   const point=field.startsWith('point:'),pointId=point?field.slice(6):undefined;
   const datum=point?view.part?.datumPoints?.find(p=>p.id===(s.entityId??s.id)):undefined;
   const input=datum?undefined:await bind(s);
   if(!alive.current||!scope.current.current(ticket))return;
   setDraft(f=>{const next=structuredClone(f);if(point){const p=next.geometry!.curve!.points.find(p=>p.id===pointId)!;p.pointId=datum?.id;p.reference=input;}else{const index=Number(field.split(':')[1]);const list=next.geometry!.inputs??[];if(multiple){if(!list.some(i=>JSON.stringify(i)===JSON.stringify(input)))list.push(input!)}else list[index]=input!;next.geometry!.inputs=list}return next});
   if(!multiple)setPicking(point?undefined:Number(field.split(':')[1])+1<roles.length?`input:${Number(field.split(':')[1])+1}`:undefined);
  }catch(error){if(alive.current&&scope.current.current(ticket))setFailure(error)}finally{if(alive.current&&bindingRequest.current===request)setBinding(false)}
 };
 const pickRef=useRef(pick);pickRef.current=pick;
 useEffect(()=>{alive.current=true;scope.current.activate(current.current.picking,`${view.document.id}/${base}`);return()=>{alive.current=false;scope.current.cancel();callbacks.current.onPreview();callbacks.current.onSelectionSession()}},[]);
 useEffect(()=>{
  if(!picking){callbacks.current.onSelectionSession();return}
  const point=picking.startsWith('point:'),role=roles[Number(picking.split(':')[1])]?.role;
  callbacks.current.onSelectionSession({documentId:view.document.id,versionId:base,role:picking==='body'?'body':point?'point':picking==='frame'||role==='plane'?'plane':role==='curve'?'edge':'face',topologyPick:point?'VERTEX':picking.startsWith('input:')?'EDGE':undefined,selections:[],accept:s=>accept(s,picking),onPick:s=>{void pickRef.current(s)}});
  return()=>callbacks.current.onSelectionSession();
 },[picking,kind,base]);
 useEffect(()=>{const selected=seed.find(s=>accept(s));if(selected)void pickRef.current(selected)},[]);
 let input:Record<string,unknown>|undefined,problem:unknown;
 const missing=roles.length>0&&(multiple?inputs.length===0:roles.some((_,i)=>!inputs[i]));
 try{
  if(!missing){const next=structuredClone(draft),expressions:Record<string,string>={};
   for(const field of geometryFields(next.geometry!)){
    const text=committed[field.slot]??String(field.unit==='mm'?millimetersToDisplayLength(field.value,unit):field.value),parsed=parseParameterSource(text,field.unit==='mm'?unit:field.unit);
    if(parsed.kind==='EXPRESSION'){if(!parsed.expression)throw new CommandInputError('请输入参数');expressions[field.slot]=parsed.expression}
    else{if(field.unit==='mm'&&!['mm','cm','m','in'].includes(parsed.unit)||field.unit==='deg'&&!['deg','rad'].includes(parsed.unit)||field.unit===''&&parsed.unit!=='')throw new CommandInputError('参数单位不匹配');field.set(field.unit==='mm'?displayLengthToMillimeters(parsed.value,parsed.unit as DisplayLengthUnit):field.unit==='deg'&&parsed.unit==='rad'?parsed.value*180/Math.PI:parsed.value)}
   }
   input={type:feature.id?'EDIT_FEATURE':kind==='THICKEN'?'CREATE_SOLID_FEATURE':'CREATE_GEOMETRY_FEATURE',requestId:intent,targetId:feature.id||undefined,expectedFeatureDigest:digest,feature:next,parameterExpressions:expressions};
  }
 }catch(error){problem=error}
 const preview=useFeaturePreview(view.document.id,base,input,draft.operation,onPreview);
 const feedback=useFeatureDialogFeedback(failure??preview.failure,geometryNames[kind],problem,preview.repairRequired,preview.retry);
 const fields=geometryFields(draft.geometry!);
 const fieldInput=(field:typeof fields[number])=><CommandField key={field.slot} label={field.label} width={field.group?'coordinate':'expression'}><ParameterReferenceInput value={values[field.slot]??String(field.unit==='mm'?millimetersToDisplayLength(field.value,unit):field.value)} parameters={view.part?.parameters??[]} unit={field.unit==='mm'?unit:undefined} onChange={v=>setValues(old=>({...old,[field.slot]:v}))} onBlur={()=>{setCommitted({...values});setFailure(undefined);feedback.reportValidation()}}/></CommandField>;
 const groups=[...new Set(fields.flatMap(f=>f.group?[f.group]:[]))];
 const changeInput=(index:number,fn:(i:GeometricInput)=>GeometricInput)=>setDraft(f=>{const next=structuredClone(f);next.geometry!.inputs![index]=fn(next.geometry!.inputs![index]);return next});
 return <CommandDialog id="geometry-feature" open size="M" title={`${feature.id?'编辑':''}${geometryNames[kind]}`} onClose={onClose} onCancelPick={()=>{if(!picking)return false;setPicking(undefined);return true}} commandStatus={<CommandStatus pending={binding||preview.pending} onRetry={preview.retry}/>} confirmLoading={saving} confirmDisabled={!!problem||missing||!preview.previewId||preview.pending||binding||view.document.versionId!==base||JSON.stringify(values)!==JSON.stringify(committed)} onConfirm={async()=>{if(!input||!preview.previewId||saving)return;setSaving(true);try{await onApply({...input,previewId:preview.previewId});onClose()}catch(error){setFailure(error);setSaving(false)}}}>
  <div onBlurCapture={feedback.onBlurCapture}>
  {roles.map((role,index)=><FeaturePickField compact key={index} label={role.label} value={multiple?`${inputs.length} 条曲线`:geometryInputName(view,inputs[index])} active={picking===`input:${index}`} selected={multiple?!!inputs.length:!!inputs[index]} mode={multiple?'ordered':'single'} onActivate={()=>setPicking(`input:${index}`)} onClear={inputs[index]?()=>setDraft(f=>{const next=structuredClone(f);delete next.geometry!.inputs![index];return next}):undefined} items={multiple?inputs.map((i,n)=>({key:String(n),value:geometryInputName(view,i),onClear:()=>setDraft(f=>({...f,geometry:{...f.geometry!,inputs:f.geometry!.inputs?.filter((_,j)=>j!==n)}})),actions:<Button size="small" type="text" onClick={()=>changeInput(n,i=>({...i,reversed:!i.reversed}))}>反向</Button>})):undefined} actions={!multiple&&inputs[index]?.kind!=='PLANE'?<Button size="small" type="text" disabled={!inputs[index]} onClick={()=>changeInput(index,i=>({...i,reversed:!i.reversed}))}>反向</Button>:undefined}/>)}
  {draft.geometry?.curve?.points.map((p,index)=>{const label=kind==='LINE'?(index?'终点':'起点'):kind==='CIRCLE'||kind==='ARC'?'圆心':`点 ${index+1}`;return <div key={p.id}><FeaturePickField compact key={p.id} label={label} active={picking===`point:${p.id}`} value={p.reference?geometryInputName(view,p.reference):p.pointId?view.part?.datumPoints?.find(d=>d.id===p.pointId)?.name:'坐标'} selected={!!p.pointId||!!p.reference} onActivate={()=>setPicking(`point:${p.id}`)} onClear={p.pointId||p.reference?()=>setDraft(f=>{const next=structuredClone(f),q=next.geometry!.curve!.points[index];delete q.pointId;delete q.reference;return next}):undefined} actions={kind==='SPLINE'&&draft.geometry!.curve!.points.length>2?<Button size="small" type="text" onClick={()=>setDraft(f=>{const next=structuredClone(f),c=next.geometry!.curve!;c.points.splice(index,1);c.weights?.splice(index,1);if(c.knots?.length)splineControlMode(next.geometry!,true);return next})}>移除</Button>:undefined}/>{!p.pointId&&!p.reference&&<CommandVector label={`坐标（${unit}）`}>{fields.filter(f=>f.group===label).map(fieldInput)}</CommandVector>}</div>})}
  {groups.filter(g=>!fields.some(f=>f.group===g&&f.slot.startsWith('geometry:point:'))&&g!=='法向'&&g!=='起始方向').map(group=><CommandVector key={group} label={group+(fields.find(f=>f.group===group)?.unit==='mm'?`（${unit}）`:'')}>{fields.filter(f=>f.group===group).map(fieldInput)}</CommandVector>)}
  {fields.filter(f=>!f.group).map(f=>fieldInput({...f,label:f.label+(f.unit?`（${f.unit==='mm'?unit:f.unit}）`: '')}))}
  {kind==='SPLINE'&&<><CommandField label="样条方式"><Segmented value={draft.geometry!.curve!.knots?.length?'CONTROL':'FIT'} options={[{label:'插值点',value:'FIT'},{label:'控制点',value:'CONTROL'}]} onChange={mode=>setDraft(f=>{const next=structuredClone(f);splineControlMode(next.geometry!,mode==='CONTROL');return next})}/></CommandField>{!!draft.geometry!.curve!.knots?.length&&<CommandField label="次数"><InputNumber min={1} max={Math.min(25,draft.geometry!.curve!.points.length-1)} value={draft.geometry!.curve!.degree} onChange={n=>setDraft(f=>{const next=structuredClone(f);splineControlMode(next.geometry!,true,n??3);return next})}/></CommandField>}<Button size="small" onClick={()=>setDraft(f=>{const next=structuredClone(f),points=next.geometry!.curve!.points,last=points.at(-1)!.position;points.push({id:randomUUID(),position:[last[0]+15,last[1],last[2]]});if(next.geometry!.curve!.knots?.length)splineControlMode(next.geometry!,true);return next})}>添加点</Button></>}
  {kind==='EXTEND'&&<CommandField label="延伸端"><Segmented value={draft.geometry!.parameters.side} options={[{label:'起点',value:'START'},{label:'终点',value:'END'},{label:'两端',value:'BOTH'}]} onChange={side=>setDraft(f=>({...f,geometry:{...f.geometry!,parameters:{...f.geometry!.parameters,side:side as 'START'|'END'|'BOTH'}}}))}/></CommandField>}
  {kind==='THICKEN'&&<><CommandField label="实体操作"><Segmented value={draft.operation} options={[{label:'新实体',value:'NEW_BODY'},{label:'添加',value:'ADD'},{label:'切除',value:'REMOVE'}]} onChange={operation=>setDraft(f=>({...f,operation:operation as Feature['operation']}))}/></CommandField>{draft.operation!=='NEW_BODY'&&<FeaturePickField compact label="目标实体" value={view.part?.bodies.find(b=>b.id===draft.bodyId)?.name??'选择实体'} active={picking==='body'} selected={!!draft.bodyId} onActivate={()=>setPicking('body')}/>}</>}
  {draft.geometry?.curve&&<FeaturePickField compact label="坐标系" value={view.part?.datumPlanes.find(p=>p.id===draft.geometry?.curve?.framePlaneId)?.name??'零件坐标系'} selected={!!draft.geometry.curve.framePlaneId} active={picking==='frame'} onActivate={()=>setPicking('frame')} onClear={draft.geometry.curve.framePlaneId?()=>setDraft(f=>({...f,geometry:{...f.geometry!,curve:{...f.geometry!.curve!,framePlaneId:undefined}}})):undefined}/>}
  <details><summary>更多</summary>
   {groups.filter(g=>g==='法向'||g==='起始方向').map(group=><CommandVector key={group} label={group}>{fields.filter(f=>f.group===group).map(fieldInput)}</CommandVector>)}
   {!feature.id&&<CommandField label="名称"><Input value={draft.name??''} placeholder="自动命名" onChange={e=>setDraft(f=>({...f,name:e.target.value}))}/></CommandField>}
   {!feature.id&&kind!=='THICKEN'&&<CommandField label="几何图形集"><Select value={draft.geometricalSetId??''} options={[{value:'',label:'默认'},...(view.part?.geometricalSets??[]).map(s=>({value:s.id,label:s.name}))]} onChange={geometricalSetId=>setDraft(f=>({...f,geometricalSetId}))}/></CommandField>}
  </details>
  </div>
 </CommandDialog>
}
