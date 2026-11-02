import {useRef} from 'react';
import {api} from '../../api/client';
import type {DocumentView} from '../../types';
import {InsertDocumentDialog} from './insert-document-dialog';

/** The same resource browser as Insert, with one explicit replacement. */
export function ReplaceInstanceDialog({view,initialInstanceId,onClose,onApply}:{
 view:DocumentView;initialInstanceId?:string;onClose:()=>void;onApply:(view:DocumentView)=>Promise<unknown>;
}){
 const baseline=useRef(view.document.versionId),current=useRef(view);current.current=view;
 const instance=view.product?.instances.find(item=>item.id===initialInstanceId);
 if(!instance)return null;
 return <InsertDocumentDialog targetID={view.document.id} rootID={view.document.id} targetFolderID={view.document.folderId} replacementName={instance.name} busy={false} onClose={onClose} onInsertDocuments={async ids=>{
  if(current.current.document.versionId!==baseline.current)throw new Error('装配已更新，请重新发起替换');
  const replacement=await api.getDocument(ids[0]);
  if(current.current.document.versionId!==baseline.current)throw new Error('装配已更新，请重新发起替换');
  const updated=await api.command(view.document.id,{type:'REPLACE_INSTANCE',versionId:baseline.current,instanceId:instance.id,referencedDocumentId:replacement.document.id,replacementVersionId:replacement.document.versionId});
  await onApply(updated);
 }}/>
}
