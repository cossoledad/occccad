import {Checkbox} from 'antd';
import {useEffect,useRef,useState} from 'react';
import type {DocumentView} from '../../types';
import type {FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {FeaturePickField} from './feature-pick-field';
import type {ObjectDeleteTarget} from './object-operations';
export function PartContainerDialog({view,targets,workingSetID,dissolve,onSelectionSession,onClose,onApply}:{view:DocumentView;targets:ObjectDeleteTarget[];workingSetID?:string;dissolve?:boolean;onSelectionSession:(s?:FeatureSelectionSession)=>void;onClose:()=>void;onApply:(input:Record<string,unknown>)=>Promise<unknown>}){
 const base=useRef(view.document.versionId).current,sets=view.part?.geometricalSets??[];
 const source=targets.length===1&&targets[0].targetKind==='GEOMETRICAL_SET'?sets.find(s=>s.id===targets[0].targetId):undefined;
 const [destination,setDestination]=useState(source?.parentId??(workingSetID!==source?.id?workingSetID:undefined)),[picking,setPicking]=useState(false),[removeSet,setRemoveSet]=useState(!!dissolve),[saving,setSaving]=useState(false);
 const valid=base===view.document.versionId;
 const acceptable=(id?:string)=>{if(!id)return false;const seen=new Set<string>();while(id&&!seen.has(id)){seen.add(id);if(targets.some(t=>t.targetKind==='GEOMETRICAL_SET'&&t.targetId===id))return false;id=sets.find(s=>s.id===id)?.parentId}return true};
 useEffect(()=>{if(!destination)setPicking(true)},[]);
 useEffect(()=>{onSelectionSession(picking?{seedEnabled:false,documentId:view.document.id,versionId:base,role:'geometry',selections:[],accept:s=>s.kind==='tree'&&s.entityKind==='GEOMETRICAL_SET'&&s.documentId===view.document.id&&(!s.versionId||s.versionId===base)&&acceptable(s.entityId),onPick:s=>{setDestination(s.entityId);setPicking(false)}}:undefined);return()=>onSelectionSession()},[picking,base]);
 return <CommandDialog id="part-container" open title={removeSet?'解散几何图形集':'移入几何图形集'} size="S" onClose={onClose} confirmLoading={saving} confirmDisabled={!valid||!acceptable(destination)} onConfirm={async()=>{setSaving(true);try{await onApply({type:removeSet?'DISSOLVE_GEOMETRICAL_SET':'MOVE_TO_GEOMETRICAL_SET',versionId:base,targetId:source?.id,targets,geometricalSetId:destination});onClose()}finally{setSaving(false)}}}>
  <div className="command-form"><FeaturePickField compact label="目标集合" value={sets.find(s=>s.id===destination)?.name} active={picking} selected={!!destination} onActivate={()=>setPicking(true)}/>{source&&<Checkbox checked={removeSet} onChange={e=>setRemoveSet(e.target.checked)}>解散此集合，保留内容</Checkbox>}{!valid&&<span className="command-field-error">模型已改变，请重新打开命令。</span>}</div>
 </CommandDialog>;
}
