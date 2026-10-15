import type {AssemblyGroupMember,DocumentView,Selection,SelectionItem} from '../../types';
import {owningInstanceSelection} from './instance-selection';

function containsGroup(view:DocumentView,id:string,target:string,seen=new Set<string>()):boolean{
 if(id===target)return true;
 if(seen.has(id))return false;
 seen.add(id);
 return view.product?.constraints?.find(c=>c.id===id&&c.kind==='FIX_TOGETHER')?.groupMembers?.some(m=>!!m.groupId&&containsGroup(view,m.groupId,target,seen))??false;
}
function containsInstance(view:DocumentView,id:string,target:string,seen=new Set<string>()):boolean{
 if(seen.has(id))return false;
 seen.add(id);
 return view.product?.constraints?.find(c=>c.id===id&&c.kind==='FIX_TOGETHER')?.groupMembers?.some(m=>m.instanceId===target||!!m.groupId&&containsInstance(view,m.groupId,target,seen))??false;
}

export function groupMemberForInstance(view:DocumentView,member:AssemblyGroupMember,excludedGroupId?:string):AssemblyGroupMember{
 if(!member.instanceId)return member;
 const candidates=(view.product?.constraints??[]).filter(c=>c.kind==='FIX_TOGETHER'&&!c.suppressed&&
  (!excludedGroupId||!containsGroup(view,c.id,excludedGroupId))&&containsInstance(view,c.id,member.instanceId!));
 const outer=candidates.filter(c=>!candidates.some(other=>other.id!==c.id&&containsGroup(view,other.id,c.id)));
 // Overlapping independent groups have no unique owner; users can select the
 // desired group explicitly in the tree. Never choose by name or array order.
 return outer.length===1?{groupId:outer[0].id}:member;
}

export function assemblyGroupMember(view:DocumentView,item:SelectionItem,ownerOccurrencePath='',excludedGroupId?:string):AssemblyGroupMember|undefined{
 const source=item.highlightTarget??item;
 if(source.kind==='assembly-constraint'){
  const group=view.product?.constraints?.find(c=>c.id===source.constraintId&&c.kind==='FIX_TOGETHER');
  if(group&&(!excludedGroupId||!containsGroup(view,group.id,excludedGroupId))&&(source.documentId??view.document.id)===view.document.id&&(source.occurrencePath??'')===ownerOccurrencePath)return {groupId:group.id};
  return;
 }
 const unit=owningInstanceSelection(view,source,ownerOccurrencePath);return unit?.instanceId?groupMemberForInstance(view,{instanceId:unit.instanceId,instancePath:unit.instancePath},excludedGroupId):undefined;
}
export function groupMemberToken(member:AssemblyGroupMember):string{return member.groupId?'group:'+member.groupId:'instance:'+member.instanceId}
export function groupMemberSelection(view:DocumentView,member:AssemblyGroupMember,ownerOccurrencePath=''):Selection{
 if(member.groupId)return {kind:'assembly-constraint',id:member.groupId,constraintId:member.groupId,constraintType:'FIX_TOGETHER',documentId:view.document.id,versionId:view.document.versionId,occurrencePath:ownerOccurrencePath};
 if(!member.instanceId)return null;
 const occurrencePath=[ownerOccurrencePath,member.instanceId].filter(Boolean).join('/');
 return {kind:'instance',id:occurrencePath,instanceId:member.instanceId,occurrencePath,instancePath:member.instancePath,visualKey:'occurrence:'+occurrencePath};
}
