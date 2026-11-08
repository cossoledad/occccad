import {Select} from 'antd';
import {useEffect,useRef,useState} from 'react';
import {api} from '../../api/client';
import {CommandInputError} from '../../cad/command/operation-failure';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandStatus,CommandVector} from '../../cad/overlay/command-form';
import {FeaturePickScope,type FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import type {DatumPreview} from '../../cad/rendering/datum-reference';
import type {DatumReference,DocumentView,SelectionItem} from '../../types';
import {randomUUID} from '../../utils/random-uuid';
import {displayLengthToMillimeters,millimetersToDisplayLength} from '../../state/ui-preferences';
import {FeaturePickField} from './feature-pick-field';
import {ParameterReferenceInput} from './parameter-reference-input';
import {parameterSourceText,parseParameterSource} from './parameter-editor';
import {useFeaturePreview} from './use-feature-preview';
import {useFeatureDialogFeedback} from './use-feature-dialog-feedback';
import {isSolidFeature} from './workbench-tree-model';
import {defaultReferenceConstruction,referenceFields,referenceInputName,referenceModes,setReferenceValue,type ReferenceElementKind} from './reference-element-model';

export function ReferenceElementEditor({kind,view,id,unit,occurrencePath,placement,onClose,onApply,onPreview,onSelectionSession}:{kind:ReferenceElementKind;view:DocumentView;id?:string;unit:'mm'|'cm'|'m'|'in';occurrencePath?:string;placement?:Pick<DatumPreview,'translation'|'rotation'>;onClose:()=>void;onApply:(input:Record<string,unknown>)=>Promise<unknown>;onPreview:(preview?:DatumPreview)=>void;onSelectionSession:(session?:FeatureSelectionSession)=>void}){
 const existing=kind==='point'?view.part?.datumPoints?.find(p=>p.id===id):kind==='line'?view.part?.datumAxes?.find(p=>p.id===id):view.part?.datumPlanes.find(p=>p.id===id);
 const [construction,setConstruction]=useState(()=>structuredClone(existing?.construction??defaultReferenceConstruction(kind)));
 const name=existing?.name??'',setID=existing?.geometricalSetId??view.part?.geometricalSets?.[0]?.id??'';
 const initial=()=>Object.fromEntries(referenceFields(construction).map(field=>{const parameter=view.part?.parameters?.find(p=>p.ownerFeatureId===id&&p.propertySlot===field.slot);return [field.slot,parameter?parameterSourceText(parameter,field.unit==='mm'?unit:field.unit,true):String(field.unit==='mm'?millimetersToDisplayLength(field.value,unit):field.value)]}));
 const [drafts,setDrafts]=useState<Record<string,string>>(initial),[committedDrafts,setCommittedDrafts]=useState<Record<string,string>>(initial),[picking,setPicking]=useState<number|undefined>(()=>{const c=existing?.construction??defaultReferenceConstruction(kind);const roles=referenceModes[kind].find(m=>m.value===c.mode)?.roles??[];const at=roles.findIndex((_,i)=>!c.inputs?.[i]);return at<0?undefined:at}),[error,setError]=useState<unknown>(),[busy,setBusy]=useState(false),[loading,setLoading]=useState(false);
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
    if(!selection.geometryKey)throw new CommandInputError('需要当前精确几何');
    const properties=await api.getTopologyProperties(view.document.id,selection.geometryKey,selection.kind.toUpperCase() as 'FACE'|'EDGE'|'VERTEX',selection.topologyId,base.current);
    const role=roles[index];
    if(role==='plane'&&properties.geometryType!=='PLANE')throw new CommandInputError('请选择平面，曲面不能作为此构造的参考');
    if(role==='direction'&&properties.geometryType!=='LINE')throw new CommandInputError('请选择直线边或参考轴');
    if(role==='point'&&construction.mode==='POINT_CENTER'&&!['CIRCLE','ELLIPSE','SPHERE'].includes(properties.geometryType))throw new CommandInputError('请选择圆边、椭圆边或球面');
    const bound=await api.bindTopologySelection(view.document.id,{sourceVersionId:base.current,geometryKey:selection.geometryKey,kind:selection.kind.toUpperCase() as 'FACE'|'EDGE'|'VERTEX',localId:selection.topologyId});
    const featureId=bound.sourceResult?.featureId??selection.displayStageFeatureId??view.part?.features.slice().reverse().find(f=>f.bodyId===bound.sourceBodyId&&isSolidFeature(f)&&!f.suppressed)?.id;
    if(!featureId)throw new CommandInputError('缺少精确来源阶段');ref={kind:'TOPOLOGY',selection:bound,sourceVersionId:base.current,featureId};
   }
   if(!ref&&selection.kind==='visual'&&selection.visualType==='CURVE'&&view.part?.features.find(f=>f.id===selection.featureId)?.sketch?.entities.some(e=>e.id===selection.entityId&&e.kind==='LINE'))ref={kind:'SKETCH_LINE',featureId:selection.featureId,entityId:selection.entityId};
   if(!ref)throw new CommandInputError('请选择对应的参考元素或精确拓扑');if(id&&ref.entityId===id)throw new CommandInputError('参考元素不能引用自身');
   if(!alive.current||!scope.current.current(ticket))return;
   setConstruction(current=>{const inputs=[...(current.inputs??[])];inputs[index]=ref!;return {...current,inputs}});const next=roles.findIndex((_,i)=>i!==index&&!construction.inputs?.[i]);setPicking(next<0?undefined:next);
  }catch(cause){if(alive.current&&scope.current.current(ticket))setError(cause)}finally{if(alive.current&&scope.current.current(ticket))setLoading(false)}
 };
 const pickRef=useRef(pick);pickRef.current=pick;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;scope.current.cancel();callbacks.current.onPreview();callbacks.current.onSelectionSession()}},[]);
 useEffect(()=>{if(picking===undefined){callbacks.current.onSelectionSession();return}const role=roles[picking]??'point';callbacks.current.onSelectionSession({documentId:view.document.id,versionId:base.current,occurrencePath,role,selections:[],topologyPick:role==='point'&&construction.mode!=='POINT_CENTER'?'VERTEX':undefined,accept:construction.mode==='POINT_CENTER'?s=>(s.ownerDocumentId??s.documentId)===view.document.id&&(s.occurrencePath??'')===(occurrencePath??'')&&s.versionId===base.current&&['edge','face','vertex'].includes(s.kind):undefined,onPick:s=>{void pickRef.current(s,picking)}});return()=>callbacks.current.onSelectionSession()},[picking,construction.mode]);
 let input:Record<string,unknown>|undefined,problem:unknown;
 const missing=roles.some((_,i)=>!construction.inputs?.[i]);
 if(!missing&&base.current===view.document.versionId)try{
  const c=structuredClone(construction),expressions:Record<string,string>={};
  for(const field of referenceFields(c)){const parsed=parseParameterSource(committedDrafts[field.slot]??String(field.value),field.unit==='mm'?unit:field.unit);if(parsed.kind==='EXPRESSION'){if(!parsed.expression)throw new CommandInputError('请输入参数');expressions[field.slot]=parsed.expression}else{if(field.unit==='mm'&&!['mm','cm','m','in'].includes(parsed.unit)||field.unit==='deg'&&!['deg','rad'].includes(parsed.unit)||field.unit===''&&parsed.unit!=='')throw new CommandInputError('参数单位不匹配');const value=field.unit==='mm'?displayLengthToMillimeters(parsed.value,parsed.unit as typeof unit):field.unit==='deg'&&parsed.unit==='rad'?parsed.value*180/Math.PI:parsed.value;setReferenceValue(c,field.slot,value)}}
  input={requestId:intent.current,type:kind==='point'?`${id?'EDIT':'CREATE'}_REFERENCE_POINT`:`${id?'EDIT':'CREATE'}_DATUM_${kind==='line'?'AXIS':'PLANE'}`,targetId:id,name:name.trim(),geometricalSetId:setID,referenceConstruction:c,parameterExpressions:expressions};
 }catch(cause){problem=cause}
 const preview=useFeaturePreview(view.document.id,base.current,input,undefined,()=>{});
 useEffect(()=>{const refs=preview.referenceGeometry;const candidates=kind==='point'?refs?.datumPoints:kind==='line'?refs?.datumAxes:refs?.datumPlanes;const candidate=id?candidates?.find(p=>p.id===id):candidates?.at(-1);callbacks.current.onPreview(candidate?{...placement,...(kind==='point'?{point:candidate as import('../../types').DatumPoint}:kind==='line'?{axis:candidate as import('../../types').DatumAxis}:{plane:candidate as import('../../types').DatumPlane})}:undefined)},[preview.referenceGeometry,placement?.translation,placement?.rotation]);
 const feedback=useFeatureDialogFeedback(error??preview.failure??(base.current!==view.document.versionId?new CommandInputError('模型已改变，请重新打开命令'):undefined),'参考元素',problem,preview.repairRequired,preview.retry);
 return <CommandDialog id={`reference-${kind}`} open size={kind==='point'?'S':'M'} title={`${id?'编辑':'创建'}参考${kind==='point'?'点':kind==='line'?'线':'面'}`} onClose={onClose} onCancelPick={()=>{if(picking===undefined)return false;setPicking(undefined);return true}} commandStatus={<CommandStatus pending={loading||preview.pending} onRetry={preview.retry}/>} confirmLoading={busy} confirmDisabled={JSON.stringify(drafts)!==JSON.stringify(committedDrafts)||!input||!preview.previewId||preview.pending||loading||base.current!==view.document.versionId} onConfirm={async()=>{if(!input||!preview.previewId||busy||JSON.stringify(drafts)!==JSON.stringify(committedDrafts))return;setBusy(true);try{await onApply({...input,previewId:preview.previewId});onClose()}catch(cause){setError(cause);setBusy(false)}}}>
  <fieldset disabled={busy} className="feature-input-fields reference-element-fields" onBlurCapture={feedback.onBlurCapture}>
  <CommandField label="构造方式"><Select value={construction.mode} options={referenceModes[kind]} onChange={mode=>{setPicking(referenceModes[kind].find(m=>m.value===mode)?.roles.length?0:undefined);scope.current.cancel();setError(undefined);setConstruction({...defaultReferenceConstruction(kind),mode,inputs:[]});setDrafts({});setCommittedDrafts({})}}/></CommandField>
  {roles.map((role,index)=><FeaturePickField compact key={`${construction.mode}:${index}`} label={`${role==='point'?'点':role==='plane'?'平面':'方向'} ${index+1}`} value={referenceInputName(view,construction.inputs?.[index])} selected={!!construction.inputs?.[index]} active={picking===index} onActivate={()=>setPicking(index)} onClear={construction.inputs?.[index]?()=>setConstruction(current=>{const inputs=[...(current.inputs??[])];return {...current,inputs:inputs.map((ref,i)=>i===index?undefined:ref) as DatumReference[]}}):undefined}/>)}
  {(() => {
   const fields=referenceFields(construction);
   const fieldInput=(field:typeof fields[number],compact=false)=><CommandField key={field.slot} label={field.label} width={compact?'coordinate':'expression'}><ParameterReferenceInput value={drafts[field.slot]??String(field.unit==='mm'?millimetersToDisplayLength(field.value,unit):field.value)} onChange={value=>setDrafts(current=>({...current,[field.slot]:value}))} onBlur={()=>setCommittedDrafts({...drafts})} parameters={view.part?.parameters??[]} unit={field.unit==='mm'?unit:undefined}/></CommandField>;
   const coordinates=fields.filter(f=>/^reference:[xyz]$/.test(f.slot)),direction=fields.filter(f=>/^reference:d[xyz]$/.test(f.slot));
   return <>
    {!!coordinates.length&&<CommandVector label={`坐标（${unit}）`}>{coordinates.map(f=>fieldInput(f,true))}</CommandVector>}
    {!!direction.length&&<CommandVector label={kind==='plane'?'法向':'方向'}>{direction.map(f=>fieldInput({...f,label:f.label.slice(-1)},true))}</CommandVector>}
    {fields.filter(f=>!coordinates.includes(f)&&!direction.includes(f)).map(f=>fieldInput({...f,label:f.label+(f.unit?`（${f.unit==='mm'?unit:f.unit}）`:'')}))}
   </>;
  })()}

  </fieldset>
 </CommandDialog>
}
