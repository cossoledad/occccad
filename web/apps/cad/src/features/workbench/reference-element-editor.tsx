import {Alert,Input,Select} from 'antd';
import {useEffect,useRef,useState} from 'react';
import {api} from '../../api/client';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandStatus} from '../../cad/overlay/command-form';
import {FeaturePickScope,type FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {engineeringProblem} from '../../cad/command/operation-feedback';
import type {DatumPreview} from '../../cad/rendering/datum-reference';
import type {DatumReference,DocumentView,SelectionItem} from '../../types';
import {randomUUID} from '../../utils/random-uuid';
import {displayLengthToMillimeters,millimetersToDisplayLength} from '../../state/ui-preferences';
import {FeaturePickField} from './feature-pick-field';
import {ParameterReferenceInput} from './parameter-reference-input';
import {parameterSourceText,parseParameterSource} from './parameter-editor';
import {useFeaturePreview} from './use-feature-preview';
import {isSolidFeature} from './workbench-tree-model';
import {defaultReferenceConstruction,referenceFields,referenceInputName,referenceModes,setReferenceValue,type ReferenceElementKind} from './reference-element-model';

export function ReferenceElementEditor({kind,view,id,unit,occurrencePath,placement,onClose,onApply,onPreview,onSelectionSession}:{kind:ReferenceElementKind;view:DocumentView;id?:string;unit:'mm'|'cm'|'m'|'in';occurrencePath?:string;placement?:Pick<DatumPreview,'translation'|'rotation'>;onClose:()=>void;onApply:(input:Record<string,unknown>)=>Promise<unknown>;onPreview:(preview?:DatumPreview)=>void;onSelectionSession:(session?:FeatureSelectionSession)=>void}){
 const existing=kind==='point'?view.part?.datumPoints?.find(p=>p.id===id):kind==='line'?view.part?.datumAxes?.find(p=>p.id===id):view.part?.datumPlanes.find(p=>p.id===id);
 const [construction,setConstruction]=useState(()=>structuredClone(existing?.construction??defaultReferenceConstruction(kind)));
 const [name,setName]=useState(existing?.name??''),[setID,setSetID]=useState(existing?.geometricalSetId??view.part?.geometricalSets?.[0]?.id??'');
 const initial=()=>Object.fromEntries(referenceFields(construction).map(field=>{const parameter=view.part?.parameters?.find(p=>p.ownerFeatureId===id&&p.propertySlot===field.slot);return [field.slot,parameter?parameterSourceText(parameter,field.unit==='mm'?unit:field.unit,true):String(field.unit==='mm'?millimetersToDisplayLength(field.value,unit):field.value)]}));
 const [drafts,setDrafts]=useState<Record<string,string>>(initial),[picking,setPicking]=useState<number>(),[error,setError]=useState<string>(),[busy,setBusy]=useState(false),[loading,setLoading]=useState(false);
 const base=useRef(view.document.versionId),intent=useRef(randomUUID()),scope=useRef(new FeaturePickScope()),alive=useRef(true);
 const callbacks=useRef({onPreview,onSelectionSession});callbacks.current={onPreview,onSelectionSession};
 const roles=referenceModes[kind].find(mode=>mode.value===construction.mode)?.roles??[];
 scope.current.activate(picking===undefined?undefined:String(picking),JSON.stringify([view.document.id,base.current,construction.mode,occurrencePath]));
 const pick=async(selection:SelectionItem,index:number)=>{
  if((selection.ownerDocumentId??selection.documentId)!==view.document.id||selection.versionId!==base.current||(selection.occurrencePath??'')!==(occurrencePath??''))return;
  const ticket=scope.current.latest();setLoading(true);setError(undefined);
  try{
   let ref:DatumReference|undefined;
   if(selection.kind==='reference-point'||selection.kind==='datum-point'||selection.kind==='axis-system')ref={kind:'POINT',entityId:selection.entityId};
   else if(selection.kind==='plane')ref={kind:'PLANE',entityId:selection.entityId};
   else if(selection.kind==='axis')ref=selection.axis==='DATUM'?{kind:'AXIS',entityId:selection.entityId}:{kind:'AXIS_SYSTEM',entityId:selection.entityId,axis:selection.axis};
   else if(selection.kind==='vertex'||selection.kind==='edge'||selection.kind==='face'){
    if(!selection.geometryKey)throw new Error('需要当前精确几何');
    const bound=await api.bindTopologySelection(view.document.id,{sourceVersionId:base.current,geometryKey:selection.geometryKey,kind:selection.kind.toUpperCase() as 'FACE'|'EDGE'|'VERTEX',localId:selection.topologyId});
    const featureId=selection.displayStageFeatureId??view.part?.features.slice().reverse().find(f=>f.bodyId===bound.sourceBodyId&&isSolidFeature(f)&&!f.suppressed)?.id;
    if(!featureId)throw new Error('缺少精确来源阶段');ref={kind:'TOPOLOGY',selection:bound,sourceVersionId:base.current,featureId};
   }
   if(!ref)throw new Error('请选择对应的参考元素或精确拓扑');if(ref.entityId===id)throw new Error('参考元素不能引用自身');
   if(!alive.current||!scope.current.current(ticket))return;
   setConstruction(current=>{const inputs=[...(current.inputs??[])];inputs[index]=ref!;return {...current,inputs}});setPicking(undefined);
  }catch(cause){if(alive.current&&scope.current.current(ticket))setError(engineeringProblem(cause))}finally{if(alive.current&&scope.current.current(ticket))setLoading(false)}
 };
 const pickRef=useRef(pick);pickRef.current=pick;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;scope.current.cancel();callbacks.current.onPreview();callbacks.current.onSelectionSession()}},[]);
 useEffect(()=>{if(picking===undefined){callbacks.current.onSelectionSession();return}const role=roles[picking]??'point';callbacks.current.onSelectionSession({documentId:view.document.id,versionId:base.current,occurrencePath,role,selections:[],topologyPick:role==='point'&&construction.mode!=='POINT_CENTER'?'VERTEX':undefined,accept:construction.mode==='POINT_CENTER'?s=>(s.ownerDocumentId??s.documentId)===view.document.id&&(s.occurrencePath??'')===(occurrencePath??'')&&s.versionId===base.current&&['edge','face','vertex'].includes(s.kind):undefined,onPick:s=>{void pickRef.current(s,picking)}});return()=>callbacks.current.onSelectionSession()},[picking,construction.mode]);
 let input:Record<string,unknown>|undefined,problem:string|undefined;
 try{
  const c=structuredClone(construction),expressions:Record<string,string>={};
  if((c.inputs??[]).filter(Boolean).length!==roles.length||roles.some((_,i)=>!c.inputs?.[i]))throw new Error('请选择全部参考元素');
  for(const field of referenceFields(c)){const parsed=parseParameterSource(drafts[field.slot]??String(field.value),field.unit==='mm'?unit:field.unit);if(parsed.kind==='EXPRESSION'){if(!parsed.expression)throw new Error('请输入参数');expressions[field.slot]=parsed.expression}else{if(field.unit==='mm'&&!['mm','cm','m','in'].includes(parsed.unit)||field.unit==='deg'&&!['deg','rad'].includes(parsed.unit)||field.unit===''&&parsed.unit!=='')throw new Error('参数单位不匹配');const value=field.unit==='mm'?displayLengthToMillimeters(parsed.value,parsed.unit as typeof unit):field.unit==='deg'&&parsed.unit==='rad'?parsed.value*180/Math.PI:parsed.value;setReferenceValue(c,field.slot,value)}}
  input={requestId:intent.current,type:kind==='point'?`${id?'EDIT':'CREATE'}_REFERENCE_POINT`:`${id?'EDIT':'CREATE'}_DATUM_${kind==='line'?'AXIS':'PLANE'}`,targetId:id,name:name.trim(),geometricalSetId:setID,referenceConstruction:c,parameterExpressions:expressions};
 }catch(cause){problem=cause instanceof Error?cause.message:String(cause)}
 const preview=useFeaturePreview(view.document.id,base.current,input,undefined,()=>{});
 useEffect(()=>{const refs=preview.referenceGeometry;const candidates=kind==='point'?refs?.datumPoints:kind==='line'?refs?.datumAxes:refs?.datumPlanes;const candidate=id?candidates?.find(p=>p.id===id):candidates?.at(-1);callbacks.current.onPreview(candidate?{...placement,...(kind==='point'?{point:candidate as import('../../types').DatumPoint}:kind==='line'?{axis:candidate as import('../../types').DatumAxis}:{plane:candidate as import('../../types').DatumPlane})}:undefined)},[preview.referenceGeometry,placement?.translation,placement?.rotation]);
 return <CommandDialog id={`reference-${kind}`} open size="M" title={`${id?'编辑':'创建'}参考${kind==='point'?'点':kind==='line'?'线':'面'}`} onClose={onClose} onCancelPick={()=>{if(picking===undefined)return false;setPicking(undefined);return true}} commandStatus={<CommandStatus pending={loading||preview.pending} error={preview.error} onRetry={preview.retry}/>} confirmLoading={busy} confirmDisabled={!input||!preview.previewId||preview.pending||loading||base.current!==view.document.versionId} onConfirm={async()=>{if(!input||!preview.previewId||busy)return;setBusy(true);try{await onApply({...input,previewId:preview.previewId});onClose()}catch(cause){setError(engineeringProblem(cause));setBusy(false)}}}>
  <CommandField label="名称"><Input value={name} placeholder="自动命名" onChange={e=>setName(e.target.value)}/></CommandField>
  <CommandField label="几何图形集"><Select value={setID} options={[{value:'',label:'自动创建 / 默认集合'},...(view.part?.geometricalSets??[]).map(set=>({value:set.id,label:set.name}))]} onChange={setSetID}/></CommandField>
  <CommandField label="构造方式"><Select value={construction.mode} options={referenceModes[kind]} onChange={mode=>{setPicking(undefined);scope.current.cancel();setError(undefined);setConstruction({...defaultReferenceConstruction(kind),mode,inputs:[]});setDrafts({})}}/></CommandField>
  {roles.map((role,index)=><FeaturePickField key={`${construction.mode}:${index}`} label={`${role==='point'?'点':role==='plane'?'平面':'方向'} ${index+1}`} value={referenceInputName(view,construction.inputs?.[index])} selected={!!construction.inputs?.[index]} active={picking===index} onActivate={()=>setPicking(index)} onClear={construction.inputs?.[index]?()=>setConstruction(current=>{const inputs=[...(current.inputs??[])];return {...current,inputs:inputs.slice(0,index)}}):undefined}/>)}
  {referenceFields(construction).map(field=><CommandField key={field.slot} label={`${field.label}${field.unit?'（'+(field.unit==='mm'?unit:field.unit)+'）':''}`}><ParameterReferenceInput value={drafts[field.slot]??String(field.unit==='mm'?millimetersToDisplayLength(field.value,unit):field.value)} onChange={value=>setDrafts(current=>({...current,[field.slot]:value}))} parameters={view.part?.parameters??[]} unit={field.unit==='mm'?unit:undefined}/></CommandField>)}
  {(error||problem)&&<Alert type={error?'error':'info'} title={error??problem}/>}
 </CommandDialog>
}
