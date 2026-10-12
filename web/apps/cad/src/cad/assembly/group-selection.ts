import type {AssemblyGroupMember,DocumentView,Selection,SelectionItem} from '../../types';
import {owningInstanceSelection} from './instance-selection';

export function assemblyGroupMember(view:DocumentView,item:SelectionItem,ownerOccurrencePath='',excludedGroupId?:string):AssemblyGroupMember|undefined{
 const source=item.highlightTarget??item;
 if(source.kind==='assembly-constraint'){
  const group=view.product?.constraints?.find(c=>c.id===source.constraintId&&c.kind==='FIX_TOGETHER');
  if(group&&group.id!==excludedGroupId&&(source.documentId??view.document.id)===view.document.id&&(source.occurrencePath??'')===ownerOccurrencePath)return {groupId:group.id};
  return;
 }
 const unit=owningInstanceSelection(view,source,ownerOccurrencePath);return unit?.instanceId?{instanceId:unit.instanceId,instancePath:unit.instancePath}:undefined;
}
export function groupMemberToken(member:AssemblyGroupMember):string{return member.groupId?'group:'+member.groupId:'instance:'+member.instanceId}
export function groupMemberSelection(view:DocumentView,member:AssemblyGroupMember,ownerOccurrencePath=''):Selection{
 if(member.groupId)return {kind:'assembly-constraint',id:member.groupId,constraintId:member.groupId,constraintType:'FIX_TOGETHER',documentId:view.document.id,versionId:view.document.versionId,occurrencePath:ownerOccurrencePath};
 if(!member.instanceId)return null;
 const occurrencePath=[ownerOccurrencePath,member.instanceId].filter(Boolean).join('/');
 return {kind:'instance',id:occurrencePath,instanceId:member.instanceId,occurrencePath,instancePath:member.instancePath,visualKey:'occurrence:'+occurrencePath};
}
