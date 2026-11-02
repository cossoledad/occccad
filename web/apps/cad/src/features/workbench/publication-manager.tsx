import {Alert,Button,Empty,Input,Select,Space,Table,Tag} from 'antd';
import {useEffect,useMemo,useRef,useState} from 'react';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField} from '../../cad/overlay/command-form';
import {engineeringProblem} from '../../cad/command/operation-feedback';
import {engineeringLabel,selectionName} from '../../cad/presentation/engineering-text';
import type {DocumentView,InstancePath,SelectionItem} from '../../types';
import type {FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {FeaturePickField} from './feature-pick-field';
import {publicationForwardTarget,publicationSelectionTarget} from './publication-target';
import {structureSelection} from './structure-selection';
import {documentParameters} from './parameter-editor';

type Target=Record<string,unknown>;
export function PublicationManager({view,root,ownerPath,initialId,seed,canEdit,onApply,onClose,onSelectionSession,onLocate}:{
 view:DocumentView;root:DocumentView;ownerPath?:InstancePath;initialId?:string;seed?:SelectionItem;canEdit:boolean;
 onApply:(input:Target)=>Promise<DocumentView>;onClose:()=>void;onSelectionSession:(session?:FeatureSelectionSession)=>void;onLocate:(selection:SelectionItem)=>void;
}){
 const publications:Array<import('../../types').Publication|import('../../types').ProductPublication>=view.part?.publications??view.product?.publications??[];
 const initial=publications.find(p=>p.id===initialId);
 const [editing,setEditing]=useState(initialId),[name,setName]=useState(initial?.name??''),[purpose,setPurpose]=useState(initial?.semanticPurpose??'');
 const [pick,setPick]=useState<SelectionItem>(),[target,setTarget]=useState<Target>(),[targetName,setTargetName]=useState<string>(),[picking,setPicking]=useState(!initialId);
 const [busy,setBusy]=useState(false),[error,setError]=useState<string>();
 const base=useRef(view.document.versionId),alive=useRef(true),inFlight=useRef(false),seeded=useRef(false),session=useRef(onSelectionSession);session.current=onSelectionSession;
 const sourceTarget=(s:SelectionItem):Target|undefined=>{
  const local=publicationSelectionTarget(view,s,ownerPath);if(local)return local;
  const forward=publicationForwardTarget(view,s,ownerPath);if(forward)return {targetId:forward.publicationId,instancePath:forward.instancePath};
 };
 const choose=(s:SelectionItem,t=sourceTarget(s),label=selectionName(root,s))=>{
  if(!t||!canEdit||inFlight.current||view.document.versionId!==base.current)return;
  if(!editing){
   const forward=t.instancePath&&!t.targetKind;
   const input:Target={type:forward?'CREATE_PRODUCT_PUBLICATION':'CREATE_PUBLICATION',...t};
   if(forward){input.publicationId=t.targetId;delete input.targetId;}
   void run(input,()=>{setPick(undefined);setPicking(true)});
   return;
  }
  setPick(s);setTarget(t);setTargetName(label);setPicking(false);setError(undefined);
 };
 useEffect(()=>{alive.current=true;if(!seeded.current){seeded.current=true;if(seed&&!initialId)choose(seed)}return()=>{alive.current=false;session.current()};},[]);
 useEffect(()=>{
  if(!picking||busy||!canEdit||view.document.versionId!==base.current){onSelectionSession();return;}
  onSelectionSession({role:'geometry',documentId:view.document.id,versionId:base.current,selections:pick?[pick]:[],accept:s=>!inFlight.current&&!!sourceTarget(s),onPick:s=>choose(s)});
  return()=>onSelectionSession();
 },[picking,busy,view.document.versionId,canEdit,editing]);
 const descendants=useMemo(()=>{
  const rows:Array<{value:string;label:string;pick:SelectionItem;target:Target}>=[];
  const visit=(node:NonNullable<DocumentView['structureTree']>)=>{
   if(node.kind==='PUBLICATION'||node.kind==='PRODUCT_PUBLICATION'){
    const s=structureSelection(node,root),forward=s&&publicationForwardTarget(view,s,ownerPath);
    if(s&&forward)rows.push({value:node.id,label:`${forward.instancePath.display} / ${node.name}`,pick:s,target:{targetId:forward.publicationId,instancePath:forward.instancePath}});
   }
   node.children?.forEach(visit);
  };if(root.structureTree)visit(root.structureTree);return rows;
 },[root,view.document.id,view.document.versionId,ownerPath?.canonical]);
 const reset=(revision=view.document.versionId)=>{setEditing(undefined);setName('');setPurpose('');setPick(undefined);setTarget(undefined);setTargetName(undefined);setPicking(true);setError(undefined);base.current=revision;};
 const edit=(id:string)=>{const p=publications.find(p=>p.id===id);if(!p)return;setEditing(id);setName(p.name);setPurpose(p.semanticPurpose??'');setPick(undefined);setTarget(undefined);setTargetName(undefined);setPicking(false);setError(undefined);base.current=view.document.versionId;};
 const targetLabel=(id:string)=>{
  const p=publications.find(p=>p.id===id);if(!p)return '来源不可用';
  const t=p.target;
  if('parameterId' in t&&t.parameterId)return documentParameters(view).find(p=>p.parameterId===t.parameterId)?.displayName??'参数';
  if('instancePath' in t&&t.instancePath?.segments.length){const source=descendants.find(r=>r.target.targetId===t.publicationId&&(r.target.instancePath as InstancePath).canonical===t.instancePath.canonical);return source?.label??`${t.instancePath.display} / 来源不可用`;}
  if('datumId' in t){const datum=view.part?.datumPlanes.find(p=>p.id===t.datumId)??view.part?.datumAxes?.find(p=>p.id===t.datumId)??view.part?.datumPoints?.find(p=>p.id===t.datumId)??view.part?.axisSystems.find(p=>p.id===t.datumId);if(datum)return datum.name+(t.axis?' / '+t.axis:'');}
  if('bodyId' in t&&t.bodyId)return view.part?.bodies.find(b=>b.id===t.bodyId)?.name??'实体';
  if('sketchId' in t&&t.sketchId)return view.part?.features.find(f=>f.id===t.sketchId)?.name??'草图元素';
  return engineeringLabel('kind' in t?t.kind:'TOPOLOGY','几何元素');
 };
 const locate=(id:string)=>{const visit=(n:NonNullable<DocumentView['structureTree']>):SelectionItem|undefined=>((n.kind==='PUBLICATION'||n.kind==='PRODUCT_PUBLICATION')&&n.entityId===id&&n.documentId===view.document.id?structureSelection(n,root):undefined)??n.children?.map(visit).find(Boolean);const s=root.structureTree&&visit(root.structureTree);if(s)onLocate(s);};
 const run=async(input:Target,after:(updated:DocumentView)=>void)=>{if(inFlight.current||!canEdit||view.document.versionId!==base.current)return;inFlight.current=true;setBusy(true);setError(undefined);try{const updated=await onApply({...input,versionId:base.current});if(alive.current){base.current=updated.document.versionId;after(updated);}}catch(cause){if(alive.current)setError(engineeringProblem(cause));}finally{inFlight.current=false;if(alive.current)setBusy(false);}};
 return <CommandDialog id="publication-manager" open title="发布接口" size="M" footer={false} onClose={onClose} onConfirm={onClose} onCancelPick={()=>{if(!picking)return false;setPicking(false);return true;}}>
  <Table size="small" rowKey="id" dataSource={publications} pagination={false} scroll={{y:220}} locale={{emptyText:<Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚无发布元素"/>}}
   onRow={p=>({onClick:()=>locate(p.id),onDoubleClick:()=>{if(canEdit&&!busy)edit(p.id)}})}
   columns={[{title:'名称',dataIndex:'name',ellipsis:true},{title:'发布元素',render:(_,p)=>targetLabel(p.id),ellipsis:true},{title:'状态',width:85,render:(_,p)=><Tag color={p.resolution.status==='CONNECTED'?'success':'error'}>{engineeringLabel(p.resolution.status)}</Tag>},{title:'操作',width:125,render:(_,p)=><Space size={0}><Button size="small" type="link" disabled={!canEdit||busy} onClick={e=>{e.stopPropagation();edit(p.id);locate(p.id)}}>编辑</Button><Button size="small" type="link" danger disabled={!canEdit||busy} onClick={e=>{e.stopPropagation();void run({type:view.part?'DELETE_PUBLICATION':'DELETE_PRODUCT_PUBLICATION',publicationId:p.id},updated=>{if(editing===p.id)reset(updated.document.versionId)})}}>移除</Button></Space>}]}/>
  <div className={editing?'publication-definition':undefined} style={editing?undefined:{display:'grid',gap:10,marginTop:12}}>
   {editing&&<><Space><strong>编辑发布</strong><Button size="small" disabled={busy} onClick={()=>reset()}>新增发布</Button></Space>
   <FeaturePickField compact label="发布元素" value={targetName??targetLabel(editing)} selected active={picking} onActivate={()=>{if(!busy){base.current=view.document.versionId;setPicking(true)}}} onLocate={pick?()=>onLocate(pick):()=>locate(editing)} onClear={target?()=>{setTarget(undefined);setPick(undefined);setTargetName(undefined)}:undefined}/></>}
   <Space wrap>
    {!editing&&<Button aria-pressed={picking} type={picking?'primary':'default'} disabled={!canEdit||busy} loading={busy} onClick={()=>{base.current=view.document.versionId;setPicking(!picking)}}>选择元素</Button>}
    {view.product&&<Select aria-label="组件发布" disabled={!canEdit||busy} showSearch optionFilterProp="label" placeholder="浏览组件发布" value={null} style={{minWidth:220}} options={descendants} onChange={id=>{const row=descendants.find(r=>r.value===id);if(row)choose(row.pick,row.target,row.label)}}/>}
    {!!documentParameters(view).length&&<Select aria-label="发布参数" disabled={!canEdit||busy} showSearch optionFilterProp="label" placeholder="选择参数" value={null} style={{minWidth:180}} options={documentParameters(view).map(p=>({value:p.parameterId,label:p.displayName??p.label??p.key}))} onChange={id=>{
     if(!id)return;const p=documentParameters(view).find(p=>p.parameterId===id);if(!p)return;
     const s:SelectionItem={kind:'parameter',id,documentId:view.document.id,versionId:view.document.versionId,instancePath:ownerPath,occurrencePath:ownerPath?.canonical};choose(s,{publicationType:'PARAMETER',targetKind:'PARAMETER',targetId:id},p.displayName??p.label??p.key);
    }}/>}
   </Space>
   {editing&&<CommandField label="名称" width="name"><Input value={name} disabled={!canEdit||busy} onChange={e=>setName(e.target.value)}/></CommandField>}
   {error&&<Alert type="error" title={error}/>}
   <Space style={{justifyContent:'flex-end',width:'100%'}}>
    <Button onClick={onClose}>关闭</Button>
    {editing&&<Button type="primary" loading={busy} disabled={!canEdit||!name.trim()||base.current!==view.document.versionId} onClick={()=>{
     const input:Target={type:view.part?'EDIT_PUBLICATION':'EDIT_PRODUCT_PUBLICATION',...target,name,semanticPurpose:purpose,publicationId:editing};
     void run(input,()=>{setEditing(undefined);setName('');setPurpose('');setTarget(undefined);setTargetName(undefined);setPick(undefined);setPicking(true)});
    }}>应用修改</Button>}
   </Space>
  </div>
 </CommandDialog>;
}
