import type {InstancePath,SelectionItem,TopologyElementProperties} from "../../types";
import type {FrozenScope,FrozenSelection} from "./assistant-api";

export type ScopeSource={rootDocumentId:string;documentId:string;revisionId:string;rootRevisionId?:string;workingBodyId?:string;editContextId?:string;instancePath?:InstancePath;selections:readonly SelectionItem[]};
export type TopologyLookup=(documentId:string,geometryKey:string,kind:"FACE"|"EDGE"|"VERTEX",localId:number,revisionId?:string)=>Promise<TopologyElementProperties>;
const instanceIds=(path?:InstancePath)=>path?.segments.map(segment=>segment.instanceId);
export const maxFrozenSelections=256;

// Copy before any lookup: changing selection while binding cannot retarget a message.
export async function freezeScope(source:ScopeSource,lookup:TopologyLookup):Promise<FrozenScope>{
 if(source.selections.length>maxFrozenSelections)throw new Error(`当前选区包含 ${source.selections.length} 个对象，超过单条消息的完整选区容量 ${maxFrozenSelections}；这条消息尚未发送。可改为描述整个实体或正式集合，助手会查询完整拓扑。`);
 const frozen=structuredClone(source),references:FrozenSelection[]=[];
 const bind=async(item:SelectionItem):Promise<FrozenSelection>=>{
  const sourceDocumentId=item.entityRef?.documentId??item.ownerDocumentId??item.documentId??frozen.documentId;
  const path=item.instancePath??item.occurrenceRef?.instancePath;
  const occurrenceRevision=path?.segments.slice().reverse().find(segment=>segment.referencedDocumentId===sourceDocumentId)?.resolvedVersionId;
  const observedRevision=item.kind==="instance"?occurrenceRevision??item.versionId:item.versionId??occurrenceRevision;
  const sourceVersionId=item.snapshotScope?.revisionId??observedRevision??(sourceDocumentId===frozen.documentId?frozen.revisionId:undefined);
  if(item.kind==="instance"&&!path?.segments.length)throw new Error("所选实例缺少正式路径，请重新选择后发送");
  const reference:FrozenSelection={kind:item.kind.toUpperCase(),sourceDocumentId,sourceVersionId,instancePath:instanceIds(path)};
  if(item.kind==="face"||item.kind==="edge"||item.kind==="vertex"){
   if(!item.geometryKey||!sourceVersionId)throw new Error("所选拓扑缺少来源版本，请重新选择后发送");
   const properties=await lookup(sourceDocumentId,item.geometryKey,item.kind.toUpperCase() as "FACE"|"EDGE"|"VERTEX",item.topologyId,sourceVersionId);
   if(!properties.persistentSelection||properties.persistentSelection.sourceDocumentId!==sourceDocumentId||properties.persistentSelection.expectedType!==reference.kind)throw new Error("所选拓扑暂时无法绑定为稳定引用；未发送这条消息，请刷新后重新选择");
   reference.selection=structuredClone(properties.persistentSelection);reference.bodyId=reference.selection.sourceBodyId;
  }else{
   const formalIdentity=item.entityRef?.entityId??item.entityId??(item.kind==="tree"?item.ownerEntityId:undefined);
   if(item.kind==="tree"&&!formalIdentity)throw new Error("这个结构分组尚无可发送的正式对象身份，请选择具体特征、实体或实例；这条消息尚未发送");
   const identity=item.kind==="part"||item.kind==="product"?sourceDocumentId:item.kind==="body"||item.kind==="solid"?item.bodyId??formalIdentity:formalIdentity??item.id;
   if(!identity)throw new Error("所选对象缺少正式稳定身份，这条消息尚未发送");
   reference.id=item.kind==="sketch-constraint"||item.kind==="assembly-constraint"?item.constraintId:item.kind==="instance"?path!.segments.at(-1)!.instanceId:identity;
   reference.bodyId=item.bodyId;
   if("featureId" in item)reference.featureId=item.featureId;
   if(item.publicationId)reference.publicationId=item.publicationId;
   if(item.kind==="axis")reference.axis=item.axis;
   if(["plane","axis","axis-system","datum-point"].includes(item.kind))reference.datumId=item.entityId??item.id;
  }
  return reference;
 };
 let cursor=0;
 await Promise.all(Array.from({length:Math.min(4,frozen.selections.length)},async()=>{while(cursor<frozen.selections.length){const index=cursor++;references[index]=await bind(frozen.selections[index])}}));
 return {rootDocumentId:frozen.rootDocumentId,documentId:frozen.documentId,revisionId:frozen.revisionId,rootRevisionId:frozen.rootRevisionId,workingBodyId:frozen.workingBodyId,editContextId:frozen.editContextId,instancePath:instanceIds(frozen.instancePath),selections:references,selectionCount:frozen.selections.length,complete:true};
}
export const scopeSummary=(scope:Pick<FrozenScope,"selectionCount"|"instancePath"|"workingBodyId"|"complete">)=>`${scope.instancePath?.length?"实例中的定义 · ":""}${scope.selectionCount?`已选 ${scope.selectionCount} 个对象${scope.complete?"":"（集合不完整）"}`:scope.workingBodyId?"当前工作实体":"当前文档"}`;
