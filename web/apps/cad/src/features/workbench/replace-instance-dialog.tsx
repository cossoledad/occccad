import {randomUUID} from '../../utils/random-uuid';
import {Select} from 'antd';
import {useEffect,useMemo,useRef,useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {api} from '../../api/client';
import type {DocumentView,SelectionItem} from '../../types';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandGroup,CommandStatus} from '../../cad/overlay/command-form';
import type {FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {FeaturePickField} from './feature-pick-field';
import {useFeaturePreview} from './use-feature-preview';
import {engineeringProblem} from '../../cad/command/operation-feedback';
import {publicDescriptors,replacementCompatible,replacementImpact,replacementInputCompatible,receiverMappingComplete} from './instance-replacement-model';

export function ReplaceInstanceDialog({view,initialInstanceId,onClose,onApply,onPreview,onSelectionSession,onLocate}:{
 view:DocumentView;initialInstanceId?:string;onClose:()=>void;onApply:(v:DocumentView)=>Promise<unknown>;onPreview:(v?:DocumentView)=>void;onSelectionSession:(s?:FeatureSelectionSession)=>void;onLocate:(s:SelectionItem)=>void;
}){
 const [operationId]=useState(randomUUID),[sourceGeneration,setSourceGeneration]=useState(0);
 const [baseline]=useState(view.document.versionId),[instanceId,setInstanceId]=useState(initialInstanceId),[picking,setPicking]=useState(!initialInstanceId);
 const [newId,setNewId]=useState<string>(),[search,setSearch]=useState(''),[mapping,setMapping]=useState<Record<string,string>>({}),[inputMapping,setInputMapping]=useState<Record<string,string>>({}),[busy,setBusy]=useState(false),[error,setError]=useState<string>();
 const alive=useRef(true),callbacks=useRef({onPreview,onSelectionSession});callbacks.current={onPreview,onSelectionSession};
 const valid=baseline===view.document.versionId;
 const instance=view.product?.instances.find(i=>i.id===instanceId);
 const catalog=useQuery({queryKey:['replacement-publications',view.document.id,baseline],queryFn:()=>api.getContextCatalog(view.document.id,''),refetchOnWindowFocus:false});
 const oldEntries=(catalog.data?.rootProductRevisionId===baseline?catalog.data.publications:[])??[];
 const oldPublications=oldEntries.filter(p=>p.instancePath.segments.length===1&&p.instancePath.segments[0].instanceId===instanceId).map(p=>p.publication);
 const findKind=(n:DocumentView['structureTree']):string|undefined=>{if(n?.kind==='INSTANCE'&&n.entityId===instanceId&&n.instancePath?.segments.length===1)return n.documentType;for(const c of n?.children??[]){const k=findKind(c);if(k)return k}};
 const kind=findKind(view.structureTree);
 const documents=useQuery({queryKey:['replacement-documents',kind,search],queryFn:()=>api.listDocuments({type:kind,query:search,allFolders:true,limit:50}),enabled:!!instance&&!!kind});
 const [sourceSnapshot,setSourceSnapshot]=useState<{generation:number;id:string;view:DocumentView}>();
 const frozen=sourceSnapshot?.generation===sourceGeneration&&sourceSnapshot.id===newId?sourceSnapshot:undefined;
 const acquisition=useQuery({queryKey:['replacement-document',operationId,sourceGeneration,newId],queryFn:()=>api.getDocument(newId!),enabled:!!newId&&!frozen,refetchOnWindowFocus:false,refetchOnReconnect:false,refetchOnMount:false,gcTime:0,staleTime:Infinity});
 useEffect(()=>{if(!frozen&&newId&&acquisition.data?.document.id===newId)setSourceSnapshot({generation:sourceGeneration,id:newId,view:acquisition.data})},[frozen,newId,sourceGeneration,acquisition.data]);
 // A completed acquisition belongs to the command, independently of subsequent
 // query invalidation/cache publication. Only an explicit selection replaces it.
 const replacement={...acquisition,data:frozen?.view,error:frozen?null:acquisition.error,isFetching:!frozen&&acquisition.isFetching};
 const impact=useMemo(()=>replacementImpact(view,instanceId??''),[view,instanceId]);
 const candidates=replacement.data?publicDescriptors(replacement.data):[];
 const chooseInstance=(id:string)=>{setInstanceId(id);setNewId(undefined);setMapping({});setInputMapping({});setPicking(false);setError(undefined)};
 const accepts=(s:SelectionItem)=>{const path=s.instancePath;return path?.segments.length===1&&path.rootDocumentId===view.document.id&&path.segments[0].ownerVersionId===baseline&&!!view.product?.instances.some(i=>i.id===path.segments[0].instanceId)};
 useEffect(()=>{if(!picking||!valid)return;callbacks.current.onSelectionSession({role:'instance',documentId:view.document.id,versionId:baseline,selections:[],accept:accepts,onPick:s=>{if(accepts(s))chooseInstance(s.instancePath!.segments[0].instanceId)}});return()=>callbacks.current.onSelectionSession()},[picking,valid,baseline]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;callbacks.current.onSelectionSession();callbacks.current.onPreview()}},[]);
 const newInputs=replacement.data?.part?.contextInputs??[],inputIds=[...new Set(impact.incoming.map(b=>b.contextInputId))];
 const inputsComplete=replacement.data?.part?receiverMappingComplete(impact.receiverInputs,newInputs,inputIds,inputMapping):!inputIds.length;
 const complete=inputsComplete&&!!replacement.data&&impact.used.every(id=>{const a=oldPublications.find(p=>p.id===id),b=candidates.find(p=>p.id===mapping[id]);return !!a&&!!b&&replacementCompatible(a,b)});
 const command=useMemo(()=>valid&&instance&&replacement.data&&complete&&!impact.blocked.length?{
  type:'REPLACE_INSTANCE',versionId:baseline,instanceId:instance.id,referencedDocumentId:replacement.data.document.id,replacementVersionId:replacement.data.document.versionId,publicationMapping:mapping,contextInputMapping:inputMapping,
 }:undefined,[valid,instance?.id,replacement.data,complete,JSON.stringify(mapping),JSON.stringify(inputMapping),impact.blocked.join('|')]);
 const preview=useFeaturePreview(view.document.id,baseline,command,undefined,()=>{});
 useEffect(()=>{callbacks.current.onPreview(preview.result?.replacementView);return()=>callbacks.current.onPreview()},[preview.result]);
 const chooseReplacement=(id:string)=>{setSourceSnapshot(undefined);setSourceGeneration(g=>g+1);setNewId(id);setMapping({});setInputMapping({});setError(undefined)};
 const suggest=useMemo(()=>{
  const result:Record<string,string>={};for(const id of impact.used){const old=oldPublications.find(p=>p.id===id);if(!old)continue;const same=candidates.filter(p=>p.name===old.name&&replacementCompatible(old,p));if(same.length===1)result[id]=same[0].id;}return result;
 },[JSON.stringify(oldPublications),JSON.stringify(candidates),impact.used.join('|')]);
 // A name only recommends an explicit identity pair; ambiguous matches stay empty.
 useEffect(()=>{setMapping(suggest);const next:Record<string,string>={};for(const id of inputIds){const old=impact.receiverInputs.find(i=>i.id===id);if(old){const same=newInputs.filter(i=>i.name===old.name&&replacementInputCompatible(old,i));if(same.length===1)next[id]=same[0].id}}setInputMapping(next)},[newId,replacement.data?.document.versionId,instanceId,JSON.stringify(suggest),JSON.stringify(newInputs)]);
 const apply=async()=>{if(!command||!preview.previewId||busy)return;setBusy(true);try{const next=await api.command(view.document.id,{...command,previewId:preview.previewId});if(!alive.current)return;await onApply(next);onClose()}catch(e){if(alive.current)setError(engineeringProblem(e))}finally{if(alive.current)setBusy(false)}};
 return <CommandDialog id="replace-instance" open title="替换组件" size="M" placement="right" onClose={onClose} onConfirm={()=>void apply()} confirmLoading={busy} confirmDisabled={!valid||!command||preview.pending||!preview.previewId||!!preview.error} onCancelPick={()=>{if(!picking)return false;setPicking(false);return true}} commandStatus={<CommandStatus pending={preview.pending} error={!valid?'装配已更新，请重新打开':error||impact.blocked[0]||preview.error||(replacement.error?engineeringProblem(replacement.error):catalog.error?engineeringProblem(catalog.error):undefined)} onRetry={replacement.error?()=>{setSourceSnapshot(undefined);setSourceGeneration(g=>g+1)}:catalog.error?()=>{void catalog.refetch()}:preview.retry}/>}>
  <FeaturePickField label="原组件" value={instance?.name} selected={!!instance} active={picking} onActivate={()=>setPicking(true)} onCancel={()=>setPicking(false)} onClear={()=>{setInstanceId(undefined);setNewId(undefined);setMapping({})}} onLocate={instance?()=>onLocate({kind:'instance',id:instance.id,instanceId:instance.id,occurrencePath:instance.id}):undefined}/>
  <CommandField label="新组件" width="expression"><Select aria-label="新组件" showSearch filterOption={false} searchValue={search} onSearch={setSearch} placeholder="搜索同类型组件" value={newId} onSelect={chooseReplacement} loading={documents.isFetching||replacement.isFetching} options={(documents.data?.documents??[]).filter(d=>d.id!==view.document.id&&d.id!==instance?.documentId).map(d=>({value:d.id,label:d.name}))}/></CommandField>
  {instance&&<CommandField label="引用方式"><span>{instance.referenceMode==='PINNED'?'固定版本':instance.referenceMode==='FOLLOW_WORKSPACE_WITH_ACCEPT'?'接受更新后采用':'跟随更新'}</span></CommandField>}
  {!!impact.used.length&&<CommandGroup label="公开接口对应">{impact.used.map(id=>{const old=oldPublications.find(p=>p.id===id);return <CommandField key={id} label={old?.name??'来源接口已丢失'} width="expression"><Select allowClear aria-label={`对应接口：${old?.name??'来源接口已丢失'}`} placeholder="选择对应接口" value={mapping[id]} onChange={value=>setMapping(m=>{const next={...m};if(value)next[id]=value;else delete next[id];return next})} options={candidates.filter(p=>old&&replacementCompatible(old,p)).map(p=>({value:p.id,label:p.name}))}/></CommandField>})}</CommandGroup>}
  {!!inputIds.length&&<CommandGroup label="接收输入对应">{inputIds.map(id=>{const old=impact.receiverInputs.find(i=>i.id===id);return <CommandField key={id} label={old?.name??'本地输入已丢失'} width="expression"><Select aria-label={`对应输入：${old?.name??'本地输入已丢失'}`} allowClear placeholder="选择新零件输入" value={inputMapping[id]} onChange={v=>setInputMapping(m=>{const n={...m};if(v)n[id]=v;else delete n[id];return n})} options={newInputs.filter(i=>old&&replacementInputCompatible(old,i)).map(i=>({value:i.id,label:i.name}))}/></CommandField>})}</CommandGroup>}
  {newInputs.some(i=>i.required&&!Object.values(inputMapping).includes(i.id))&&<CommandStatus error="新零件有必需输入尚未对应"/>}
  {instance&&<CommandField label="影响范围" width="expression"><span>{impact.bindings.length} 项关联输入 · {impact.constraints.length} 项约束 · {impact.forwarding.length} 项转发接口</span></CommandField>}
  <FeaturePickField label="受影响对象" active={false} items={[...impact.bindings.map((value,i)=>({key:`b${i}`,value})),...impact.constraints.map((value,i)=>({key:`c${i}`,value})),...impact.forwarding.map((value,i)=>({key:`p${i}`,value}))]}/>
 </CommandDialog>;
}
