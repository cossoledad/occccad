import {Alert,Button,Select} from 'antd';
import {useEffect,useRef,useState} from 'react';
import type {SolidFeatureEditor} from './solid-feature-editor';
import type {SketchOperation} from '../../types';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandStatus} from '../../cad/overlay/command-form';
import {engineeringProblem} from '../../cad/command/operation-feedback';
import {creationQuantity} from '../../cad/sketch/sketch-quantity';
import {ParameterReferenceInput} from './parameter-reference-input';
import {parameterSourceText} from './parameter-editor';
import {useFeaturePreview} from './use-feature-preview';
import {randomUUID} from '../../utils/random-uuid';

type Props=Pick<Parameters<typeof SolidFeatureEditor>[0],'view'|'unit'|'onClose'|'onApply'|'onPreview'|'onSelectionSession'|'occurrencePath'|'occurrenceContext'>&{sketchId:string;offsetId:string};
export function SketchOffsetEditor({view,sketchId,offsetId,unit,onClose,onApply,onPreview,onSelectionSession,occurrencePath,occurrenceContext}:Props){
 const sketch=view.part?.features.find(f=>f.id===sketchId)?.sketch;
 const original=sketch?.offsets?.find(o=>o.id===offsetId);
 const parameter=view.part?.parameters?.find(p=>p.parameterId===`parameter:${sketchId}:offset:${offsetId}:distance`);
 const [sources,setSources]=useState(original?.entityIds??[]),[join,setJoin]=useState(original?.join??'MITER');
 const [text,setText]=useState(parameter?parameterSourceText(parameter,unit,true):`${original?.distance??5} mm`);
 const [busy,setBusy]=useState(false),[error,setError]=useState<string>();
 const base=useRef(view.document.versionId),intent=useRef(randomUUID());
 useEffect(()=>{
  onSelectionSession({...occurrenceContext,role:'geometry',documentId:view.document.id,versionId:base.current,occurrencePath,sketchIds:[sketchId],localSketchId:sketchId,selections:sources.map(entityId=>({...occurrenceContext,kind:'visual',id:`offset-source:${entityId}`,visualType:'CURVE',featureId:sketchId,entityId,documentId:view.document.id,versionId:base.current,occurrencePath})),accept:selection=>!busy&&selection.kind==='visual'&&selection.featureId===sketchId&&(selection.ownerDocumentId??selection.documentId)===view.document.id&&(selection.occurrencePath??'')===(occurrencePath??'')&&!!sketch?.entities.some(e=>e.id===selection.entityId&&['LINE','CIRCLE','ARC'].includes(e.kind)),onPick:selection=>{if(!busy&&selection.entityId)setSources(previous=>previous.includes(selection.entityId!)?previous.filter(id=>id!==selection.entityId):[...previous,selection.entityId!]);}});
  return ()=>onSelectionSession();
 },[view.document.id,view.document.versionId,occurrencePath,JSON.stringify(sources),busy]);
 let input:Record<string,unknown>|undefined,fieldError:string|undefined;
 try{
  const quantity=creationQuantity(text,unit,original?.distance??5);
  if(!original||!sources.length||!quantity||quantity.value===0)throw new Error('选择源曲线并输入非零有符号距离');
  if(base.current===view.document.versionId)input={type:'EDIT_SKETCH',requestId:intent.current,sketchId,operations:[{type:'EDIT_OFFSET',offsetId,offset:{...original,entityIds:sources,join,distance:quantity.value},parameterSource:quantity.source}]};
 }catch(cause){fieldError=engineeringProblem(cause);}
 const preview=useFeaturePreview(view.document.id,view.document.versionId,input,'NEW_BODY',artifact=>onPreview(artifact,'NEW_BODY',sketchId));
 const apply=async()=>{if(!input||!preview.previewId||preview.pending||busy)return;setBusy(true);try{await onApply({...input,previewId:preview.previewId});onClose();}catch(cause){setError(engineeringProblem(cause));setBusy(false);}};
 const remove=async(type:'DELETE_OFFSET'|'DETACH_OFFSET')=>{if(busy)return;setBusy(true);try{const operation:SketchOperation={type,offsetId};await onApply({type:'EDIT_SKETCH',requestId:randomUUID(),sketchId,operations:[operation]});onClose();}catch(cause){setError(engineeringProblem(cause));setBusy(false);}};
 return <CommandDialog id="sketch-offset-editor" open title="编辑关联偏移" size="S" onClose={onClose} onConfirm={apply} confirmLoading={busy} confirmDisabled={!preview.previewId||preview.pending||busy||!input} commandStatus={<CommandStatus pending={preview.pending} error={fieldError??error??preview.error} onRetry={preview.retry}/>}>
  <CommandField label="源曲线"><Select aria-label="偏移源曲线" mode="multiple" value={sources} disabled={busy} onChange={setSources} options={sketch?.entities.filter(e=>['LINE','CIRCLE','ARC'].includes(e.kind)).map((e,i)=>({value:e.id,label:e.name??`${e.kind} ${i+1}`}))}/></CommandField>
  <CommandField label={`距离 (${unit})`}><ParameterReferenceInput label="偏移距离" value={text} onChange={setText} parameters={view.part?.parameters??[]} excludeId={parameter?.parameterId} unit={unit} disabled={busy}/></CommandField>
  <CommandField label="连接方式"><Select aria-label="偏移连接方式" value={join} onChange={setJoin} disabled={busy} options={[{value:'MITER',label:'斜接'},{value:'ROUND',label:'圆角'}]}/></CommandField>
  <Alert type="info" title="正值沿曲线有向左法向；源曲线与距离共同决定结果。解除关联后，输出曲线保留身份并可独立编辑。"/>
  <Button disabled={busy} onClick={()=>void remove('DELETE_OFFSET')}>删除偏移</Button> <Button disabled={busy||original?.suppressed} onClick={()=>void remove('DETACH_OFFSET')}>解除关联</Button>
 </CommandDialog>;
}
