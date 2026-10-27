import {bindingSourceSelection} from './context-input-source';
import {Button,Input,Select} from 'antd';
import {useEffect,useMemo,useRef,useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {api} from '../../api/client';
import type {Artifact,ContextBinding,ContextCatalogPublication,DocumentView,InstancePath,SelectionItem} from '../../types';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandStatus} from '../../cad/overlay/command-form';
import type {FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {FeaturePickField} from './feature-pick-field';
import {useFeaturePreview} from './use-feature-preview';
import {engineeringProblem} from '../../cad/command/operation-feedback';
import {selectionInSnapshot} from './publication-target';
import {engineeringLabel} from '../../cad/presentation/engineering-text';

const supported=new Set(['PARAMETER','PLANE','AXIS','POINT','CURVE']);
const key=(p:ContextCatalogPublication)=>`${p.instancePath.canonical}:${p.publication.id}`;
export function DesignAssociationDialog({root,owner,path,initialParameterId,activeSketchId,canEdit,onClose,onApply,onPreview,onSelectionSession,onLocate}:{
 root:DocumentView;owner:DocumentView;path?:InstancePath;initialParameterId?:string;activeSketchId?:string;canEdit:boolean;onClose:()=>void;
 onApply:(view:DocumentView)=>Promise<unknown>;onPreview:(artifact?:Artifact)=>void;onSelectionSession:(session?:FeatureSelectionSession)=>void;onLocate:(selection:SelectionItem)=>void;
}){
 const [sourceKey,setSourceKey]=useState<string>(),[target,setTarget]=useState(initialParameterId??''),[inputId,setInputId]=useState(''),[name,setName]=useState('');
 const [reconnecting,setReconnecting]=useState(false),[editing,setEditing]=useState<ContextBinding>(),[mode,setMode]=useState('FOLLOW_WORKSPACE_WITH_ACCEPT');
 const [picking,setPicking]=useState<'source'|'target'>(),[busy,setBusy]=useState(false),[error,setError]=useState<string>();
 const [baseline,setBaseline]=useState(root.document.versionId);
 const alive=useRef(true),callbacks=useRef({onSelectionSession,onPreview});callbacks.current={onSelectionSession,onPreview};
 const valid=baseline===root.document.versionId;
 const catalog=useQuery({queryKey:['design-publications',root.document.id,root.document.versionId,path?.canonical],queryFn:()=>api.getContextCatalog(root.document.id,path?.canonical??''),enabled:!!path});
 const catalogEntries=(catalog.data?.rootProductRevisionId===root.document.versionId?catalog.data.publications:[])??[];
 const allEntries=catalogEntries.filter(p=>supported.has(p.publication.type)&&p.selectable&&(p.publication.type!=='CURVE'||(p.publication.resolution.nativeSketch?['LINE','CIRCLE','ARC'].includes(p.publication.resolution.nativeSketch.geometry.kind):p.publication.resolution.topologyKind==='EDGE'&&['LINE','CIRCLE'].includes(p.publication.resolution.geometryKind??''))));
 const entries=allEntries.filter(p=>!editing||p.publication.type===editing.publication.expectedType);
 const source=entries.find(p=>key(p)===sourceKey),type=source?.publication.type;
 const inputs=(owner.part?.contextInputs??[]).filter(p=>p.type===type);
 const input=inputs.find(p=>p.id===inputId);
 const kind=type==='PARAMETER'?'PARAMETER':type==='POINT'||type==='CURVE'?'SKETCH_EXTERNAL_GEOMETRY':'DATUM';
 const targets=type==='PARAMETER'?(owner.part?.parameters??[]).map(p=>({value:p.parameterId,label:p.displayAlias||p.displayName||p.label})):
  type==='PLANE'?(owner.part?.datumPlanes??[]).map(p=>({value:p.id,label:p.name})):
  type==='AXIS'?(owner.part?.datumAxes??[]).map(p=>({value:p.id,label:p.name})):
  (owner.part?.features??[]).filter(f=>f.sketch).map(f=>({value:f.id,label:f.name||'草图'}));
 const targetId=input?.target.targetId||target||(kind==='SKETCH_EXTERNAL_GEOMETRY'?activeSketchId:'');
 const bindings=(root.product?.contextBindings??[]).filter(b=>b.owningInstancePath.canonical===path?.canonical);
 const plan=useQuery({queryKey:['product-update-plan',root.document.id,root.document.versionId],queryFn:()=>api.getProductUpdatePlan(root.document.id),enabled:!!root.product,refetchOnMount:'always'});
 const choose=(p:ContextCatalogPublication)=>{if(p.publication.type!==type){if(!(['POINT','CURVE'].includes(p.publication.type)&&target&&owner.part?.features.some(f=>f.id===target&&f.sketch)))setTarget(p.publication.type==='PARAMETER'?initialParameterId??'':'');setInputId('')}setSourceKey(key(p));if(editing)setReconnecting(true);setPicking(undefined);setError(undefined)};
 const sourceFor=(s:SelectionItem)=>entries.find(p=>selectionInSnapshot(s,p.documentId,p.revisionId,p.instancePath)&&(
  s.publicationId===p.publication.id || p.publication.target.kind==='SKETCH_ENTITY'&&s.kind==='visual'&&s.featureId===p.publication.target.sketchId&&s.entityId===p.publication.target.entityId ||
  p.publication.target.kind==='DATUM'&&s.entityId===p.publication.target.datumId || p.publication.target.kind==='PARAMETER'&&s.id===p.publication.target.parameterId));
 const localFor=(s:SelectionItem)=>{
  if(!selectionInSnapshot(s,owner.document.id,owner.document.versionId,path))return undefined;
  const id=kind==='SKETCH_EXTERNAL_GEOMETRY'?(s.kind==='sketch'?s.id:s.kind==='visual'?s.featureId:undefined):s.entityId??s.id;
  return targets.find(t=>t.value===id)?.value;
 };
 useEffect(()=>{if(!picking||!valid)return;
  const field=picking;
  callbacks.current.onSelectionSession({role:field==='source'?'geometry':'profile',documentId:owner.document.id,versionId:owner.document.versionId,selections:[],accept:s=>field==='source'?!!sourceFor(s):!!localFor(s),onPick:s=>{if(field==='source'){const p=sourceFor(s);if(p)choose(p)}else{const id=localFor(s);if(id){setTarget(id);setInputId('');setPicking(undefined)}}}});
  return()=>callbacks.current.onSelectionSession();
 },[picking,valid,JSON.stringify(entries),kind,JSON.stringify(targets),owner.document.versionId]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;callbacks.current.onSelectionSession();callbacks.current.onPreview()}},[]);
 const command=useMemo(()=>{
  if(!valid||!path)return;
  if(editing){
   if(reconnecting&&!source)return;
   return {type:'EDIT_CONTEXT_BINDING',versionId:root.document.versionId,contextBindingId:editing.id,contextInputId:editing.contextInputId,name:name||editing.name,referenceMode:mode,owningInstancePath:path,...(reconnecting&&source?{sourceInstancePath:source.instancePath,publicationId:source.publication.id}:{})};
  }
  if(!source||!targetId)return;
  return {type:'CREATE_PRODUCT_CONTEXT_BINDING',expectedRootRevisionId:root.document.versionId,name:name||undefined,owningInstancePath:path,sourceInstancePath:source.instancePath,publicationId:source.publication.id,publicationType:type,targetKind:kind,targetId,contextInputId:inputId||undefined,referenceMode:mode};
 },[valid,sourceKey,catalog.data?.digest,path,targetId,inputId,name,mode,reconnecting,editing?.id,root.document.versionId]);
 const preview=useFeaturePreview(root.document.id,root.document.versionId,command,undefined,onPreview);
 const cancelPick=()=>{if(!picking)return false;if(picking==='source'&&editing&&reconnecting){setReconnecting(false);setSourceKey(undefined)}setPicking(undefined);return true};
 const apply=async(keepOpen=false)=>{if(!command||!preview.previewId||busy)return;setBusy(true);setError(undefined);setPicking(undefined);try{
  const v=await api.command(root.document.id,{...command,previewId:preview.previewId});if(!alive.current)return;
  await onApply(v);if(!alive.current)return;
  if(keepOpen){setBaseline(v.document.versionId);setSourceKey(undefined);setEditing(undefined);setReconnecting(false);setInputId('');setName('');setMode('FOLLOW_WORKSPACE_WITH_ACCEPT');callbacks.current.onPreview();}
  else onClose();
 }catch(e){if(alive.current)setError(engineeringProblem(e))}finally{if(alive.current)setBusy(false)}};
 const action=async(run:()=>Promise<DocumentView>)=>{setBusy(true);try{await onApply(await run());onClose()}catch(e){setError(engineeringProblem(e))}finally{setBusy(false)}};
 const locate=(p:ContextCatalogPublication)=>{const t=p.publication.target,identity={documentId:p.documentId,versionId:p.revisionId,instancePath:p.instancePath,occurrencePath:p.instancePath.canonical};
 let highlightTarget:SelectionItem|undefined;
 if(t.kind==='SKETCH_ENTITY'&&t.sketchId&&t.entityId)highlightTarget={...identity,kind:'visual',visualType:t.entityKind==='POINT'?'POINT':'CURVE',id:t.entityId,entityId:t.entityId,featureId:t.sketchId};
 else if(t.kind==='DATUM'&&t.datumId)highlightTarget={...identity,id:t.datumId,entityId:t.datumId,...(p.publication.type==='AXIS'?{kind:'axis' as const,axis:'DATUM' as const}:{kind:'plane' as const,plane:'CUSTOM' as const})};
 onLocate({...identity,kind:'publication',id:p.publication.id,publicationId:p.publication.id,publication:p.publication,highlightTarget});
 };
 const locateLocal=()=>{if(!targetId||!path)return;const identity={documentId:owner.document.id,versionId:owner.document.versionId,instancePath:path,occurrencePath:path.canonical,id:targetId,entityId:targetId};onLocate(kind==='SKETCH_EXTERNAL_GEOMETRY'?{...identity,kind:'sketch'}:kind==='PARAMETER'?{...identity,kind:'parameter'}:type==='AXIS'?{...identity,kind:'axis',axis:'DATUM'}:{...identity,kind:'plane',plane:'CUSTOM'})};
 return <CommandDialog id="design-associations" open title="引用设计信息与关联管理" size="M" placement="right" onClose={onClose} onConfirm={()=>void apply()} confirmLoading={busy} confirmDisabled={!canEdit||!valid||!command||preview.pending||!preview.previewId||!!preview.error} onCancelPick={cancelPick}
  secondaryActions={<><Button disabled={!canEdit||!valid||!command||busy||preview.pending||!preview.previewId||!!preview.error||!!editing} onClick={()=>void apply(true)}>应用并继续引用</Button><Button size="small" loading={plan.isFetching} onClick={()=>void plan.refetch()}>检查更新</Button></>} commandStatus={<CommandStatus pending={preview.pending} error={error||preview.error||(catalog.error?engineeringProblem(catalog.error):undefined)||(!valid?'模型已更新，请重新打开':undefined)} onRetry={preview.retry}/>}>
  {!path?<p>请在产品中激活接收设计信息的零件。</p>:<>
   <CommandField label="接收零件" width="expression"><span>{path.display}</span></CommandField>
   <FeaturePickField label="发布接口" value={source?.displayPath??(editing&&!reconnecting?`${editing.sourceInstancePath.display} / 已接受接口`:undefined)} active={picking==='source'} selected={!!source||!!editing&&!reconnecting} onActivate={()=>setPicking('source')} onCancel={cancelPick} onClear={source?()=>{setSourceKey(undefined);if(editing)setReconnecting(true)}:undefined} onLocate={source?()=>locate(source):undefined}/>
   <CommandField label="接口目录" width="expression"><Select aria-label="发布接口目录" showSearch optionFilterProp="label" placeholder="选择已发布的设计信息" loading={catalog.isLoading} value={source?sourceKey:undefined} options={entries.map(p=>({value:key(p),label:p.displayPath}))} onChange={k=>{const p=entries.find(p=>key(p)===k);if(p)choose(p)}}/></CommandField>
   {(source||editing)&&<>
    <CommandField label="名称" width="expression"><Input aria-label="关联名称" placeholder="自动命名" value={name} onChange={e=>setName(e.target.value)}/></CommandField>
    {!editing&&<CommandField label="本地输入" width="expression"><Select aria-label="本地输入" value={inputId} onChange={setInputId} options={[{value:'',label:'新建本地输入'},...inputs.map(p=>({value:p.id,label:p.name,disabled:bindings.some(b=>b.contextInputId===p.id)}))]}/></CommandField>}
    {!editing&&!inputId&&<><FeaturePickField label={kind==='SKETCH_EXTERNAL_GEOMETRY'?'接收草图':'本地对象'} value={targets.find(t=>t.value===targetId)?.label} active={picking==='target'} selected={!!targetId} onActivate={()=>setPicking('target')} onCancel={cancelPick} onLocate={targetId?locateLocal:undefined} onClear={target?()=>setTarget(''):undefined}/>
     <CommandField label="本地对象" width="expression"><Select aria-label="本地对象" value={targetId||undefined} placeholder="选择本地对象" options={targets} onChange={setTarget}/></CommandField></>}
    <CommandField label="更新方式" width="expression"><Select aria-label="关联更新方式" value={mode} onChange={setMode} options={[{value:'FOLLOW_WORKSPACE_WITH_ACCEPT',label:'接受更新后采用'},{value:'PINNED',label:'固定当前输入'}]}/></CommandField>
   </>}
  </>}
  <FeaturePickField label="已有关联" active={false} items={bindings.map(b=>{
   const p=catalogEntries.find(p=>p.instancePath.canonical===b.sourceInstancePath.canonical&&p.publication.id===b.publication.publicationId),update=plan.data?.entries.find(e=>e.bindingId===b.id),local=owner.part?.contextInputs?.find(i=>i.id===b.contextInputId);
   const editBinding=(reconnect:boolean)=>{setEditing(b);setReconnecting(reconnect);setName(b.name);setMode(b.referenceMode);setInputId(b.contextInputId);setSourceKey(!reconnect&&p?.selectable?key(p):undefined);setPicking(reconnect?'source':undefined);setError(undefined)};
   return {key:b.id,value:`${p?.publication.name??'来源接口'} → ${local?.name??'本地输入'}`,actionLabel:b.name,error:!!update&&update.currency==='UPDATE_BLOCKED',detail:`${b.sourceInstancePath.display} · ${b.referenceMode==='PINNED'?'固定输入':update?.currency==='UPDATE_BLOCKED'?'来源不可用，保留已接受输入':update?.currency==='UPDATE_AVAILABLE'?'有可用更新':engineeringLabel(b.accepted.status)}`,
    onLocate:()=>onLocate(bindingSourceSelection(b)),onActivate:()=>editBinding(true),
    actions:<><Button size="small" disabled={!canEdit||busy} onClick={()=>editBinding(false)}>编辑</Button><Button size="small" disabled={!canEdit||busy} onClick={()=>{editBinding(false);setMode(b.referenceMode==='PINNED'?'FOLLOW_WORKSPACE_WITH_ACCEPT':'PINNED')}}>{b.referenceMode==='PINNED'?'恢复跟随':'固定'}</Button></>};
  })}/>
  {plan.data?.hasUpdates&&<Button disabled={!canEdit||!plan.data.canAccept||busy} onClick={()=>void action(()=>api.acceptProductUpdatePlan(root.document.id,plan.data!.digest))}>接受关联更新</Button>}
 </CommandDialog>;
}
