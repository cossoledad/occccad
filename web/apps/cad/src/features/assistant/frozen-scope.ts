import type {DocumentView,InstancePath,SelectionItem,TopologyElementProperties} from "../../types";
import {selectionKey} from "../../cad/interaction/selection-identity";
import type {FrozenScope,FrozenSelection} from "./assistant-api";

export type ScopeSource={view?:DocumentView;rootDocumentId:string;documentId:string;revisionId:string;rootRevisionId?:string;workingBodyId?:string;editContextId?:string;instancePath?:InstancePath;selections:readonly SelectionItem[]};
export type TopologyLookup=(documentId:string,geometryKey:string,kind:"FACE"|"EDGE"|"VERTEX",localId:number,revisionId?:string)=>Promise<TopologyElementProperties>;
const instanceIds=(path?:InstancePath)=>path?.segments.map(segment=>segment.instanceId);
export const maxFrozenSelections=256;

// Copy before any lookup: changing selection while binding cannot retarget a message.
export async function freezeScope(source:ScopeSource,lookup:TopologyLookup):Promise<FrozenScope>{
 if(source.selections.length>maxFrozenSelections)throw new Error(`当前选区包含 ${source.selections.length} 个对象，超过单条消息的完整选区容量 ${maxFrozenSelections}；这条消息尚未发送。可改为描述整个实体或正式集合，助手会查询完整拓扑。`);
 const {view,...scopeFields}=source;
 const frozen=structuredClone(scopeFields),references:FrozenSelection[]=[];
 frozen.selections=[...new Map(frozen.selections.map(item=>[selectionKey(item),item])).values()];
 const metadata=frozen.selections.map(item=>selectionMetadata(item,view));
 const bind=async(item:SelectionItem):Promise<FrozenSelection>=>{
  const path=item.instancePath??item.occurrenceRef?.instancePath;
  const sourceDocumentId=(item.kind==="instance"?path?.segments.at(-1)?.referencedDocumentId:undefined)??item.entityRef?.documentId??item.ownerDocumentId??item.documentId??frozen.documentId;
  const occurrenceRevision=path?.segments.slice().reverse().find(segment=>segment.referencedDocumentId===sourceDocumentId)?.resolvedVersionId;
  const observedRevision=item.kind==="instance"?occurrenceRevision??item.versionId:item.versionId??occurrenceRevision;
  const sourceVersionId=(item.kind==="instance"?occurrenceRevision:undefined)??item.snapshotScope?.revisionId??observedRevision??(sourceDocumentId===frozen.documentId?frozen.revisionId:undefined);
  if(item.kind==="instance"&&!path?.segments.length)throw new Error("所选实例缺少正式路径，请重新选择后发送");
  const observed=metadata[frozen.selections.indexOf(item)];
  const reference:FrozenSelection={...observed,sourceDocumentId,sourceVersionId,instancePath:instanceIds(path),ownerDocumentId:item.kind==="instance"?(path!.segments.at(-2)?.referencedDocumentId??path!.rootDocumentId):sourceDocumentId};
  if(item.kind==="face"||item.kind==="edge"||item.kind==="vertex"){
   if(!item.geometryKey||!sourceVersionId)throw new Error("所选拓扑缺少来源版本，请重新选择后发送");
   const properties=await lookup(sourceDocumentId,item.geometryKey,item.kind.toUpperCase() as "FACE"|"EDGE"|"VERTEX",item.topologyId,sourceVersionId);
   if(!properties.persistentSelection||properties.persistentSelection.sourceDocumentId!==sourceDocumentId||properties.persistentSelection.expectedType!==reference.kind||(item.bodyId&&properties.persistentSelection.sourceBodyId!==item.bodyId))throw new Error("所选拓扑暂时无法绑定为稳定引用；未发送这条消息，请刷新后重新选择");
   reference.selection=structuredClone(properties.persistentSelection);reference.bodyId=reference.selection.sourceBodyId;reference.geometryType=properties.geometryType;
  }else{
   const formalIdentity=item.entityRef?.entityId??item.entityId;
   if(item.kind==="tree"&&!formalIdentity)throw new Error("这个结构分组尚无可发送的正式对象身份，请选择具体特征、实体或实例；这条消息尚未发送");
   const identity=item.kind==="part"||item.kind==="product"?sourceDocumentId:item.kind==="body"||item.kind==="solid"?item.bodyId??formalIdentity:formalIdentity??item.id;
   if(!identity)throw new Error("所选对象缺少正式稳定身份，这条消息尚未发送");
   reference.id=item.kind==="sketch-constraint"||item.kind==="assembly-constraint"?item.constraintId:item.kind==="instance"?path!.segments.at(-1)!.instanceId:identity;
   reference.bodyId=item.bodyId;
   if("featureId" in item)reference.featureId=item.featureId;
   if(item.kind==="visual"){reference.role=item.role;reference.sketchReference=item.sketchReference;if(item.sketchReference?.target.startsWith("SKETCH_")){reference.kind="SKETCH_REFERENCE";reference.id=undefined;reference.ownerId=item.featureId}}
   if(item.publicationId)reference.publicationId=item.publicationId;
   if(item.kind==="axis")reference.axis=item.axis;
   if(["plane","axis","axis-system","datum-point"].includes(item.kind))reference.datumId=item.entityId??item.id;
  }
  return reference;
 };
 let cursor=0;
 await Promise.all(Array.from({length:Math.min(4,frozen.selections.length)},async()=>{while(cursor<frozen.selections.length){const index=cursor++;references[index]=await bind(frozen.selections[index])}}));
 const unique=[...new Map(references.map(ref=>[JSON.stringify([ref.kind,ref.sourceDocumentId,ref.sourceVersionId,ref.instancePath,ref.bodyId,ref.id,ref.featureId,ref.selection?.anchor,ref.selection?.selector,ref.sketchReference]),ref])).values()];
 return {rootDocumentId:frozen.rootDocumentId,documentId:frozen.documentId,revisionId:frozen.revisionId,rootRevisionId:frozen.rootRevisionId,workingBodyId:frozen.workingBodyId,editContextId:frozen.editContextId,instancePath:instanceIds(frozen.instancePath),selections:unique,selectionCount:unique.length,complete:true};
}
export const scopeSummary=(scope:Pick<FrozenScope,"selectionCount"|"instancePath"|"workingBodyId"|"complete">&Partial<Pick<FrozenScope,"selections">>)=>`${scope.instancePath?.length?"实例中的定义 · ":""}${scope.selectionCount?`已选 ${scope.selections?.length?countedSummary(scope.selections):`${scope.selectionCount} 个对象`}${scope.complete?"":"（集合不完整）"}`:scope.workingBodyId?"当前工作实体":"当前文档"}`;

function selectionMetadata(item:SelectionItem,view?:DocumentView):Pick<FrozenSelection,"kind"|"entityType"|"featureId"|"ownerId"|"name">{
 const id=item.entityRef?.entityId??item.entityId??item.id;
 const matchingView=view?.document.id===(item.entityRef?.documentId??item.ownerDocumentId??item.documentId??view?.document.id)&&(!item.snapshotScope?.revisionId||item.snapshotScope.revisionId===view?.document.versionId)?view:undefined;
 const feature=matchingView?.part?.features.find(feature=>feature.id===(item.kind==="visual"?item.featureId:id));
 let kind=item.kind.toUpperCase().replaceAll("-","_"),entityType=feature?.type??item.entityType,ownerId:string|undefined=item.ownerEntityId;
 if(["pad","import","feature"].includes(item.kind))kind="FEATURE";
 if(item.kind==="solid")kind="BODY";
 if(item.kind==="visual"&&(feature?.sketch||item.entityKind?.startsWith("SKETCH_"))){
  kind="SKETCH_ENTITY";ownerId=item.featureId;
  entityType=feature?.sketch?.entities.find(entity=>entity.id===item.entityId)?.kind??item.entityType;
 }
 if(item.kind==="sketch-constraint"){ownerId=item.featureId;entityType=item.constraintType}
 if(item.kind==="instance")entityType="INSTANCE";
 return {kind,entityType,ownerId,name:feature?.id===id?feature.name:undefined};
}
const labels:Record<string,string>={FEATURE:"特征",SKETCH:"草图",SKETCH_ENTITY:"草图元素",SKETCH_REFERENCE:"草图参考",SKETCH_CONSTRAINT:"草图约束",FACE:"面",EDGE:"边",VERTEX:"顶点",BODY:"实体",INSTANCE:"实例",PART:"零件",PRODUCT:"产品",ASSEMBLY_CONSTRAINT:"装配约束",VISUAL:"几何对象"};
const countedSummary=(items:{kind:string;entityType?:string}[])=>{
 const counts=new Map<string,number>();for(const item of items){const featureNames:Record<string,string>={FILLET:"圆角特征",CHAMFER:"倒角特征",PAD:"拉伸特征",LINEAR_EXTRUDE:"拉伸特征",REVOLVE:"旋转特征"};const label=(item.kind==="FEATURE"&&item.entityType?featureNames[item.entityType]:undefined)??labels[item.kind]??"对象";counts.set(label,(counts.get(label)??0)+1)}
 return [...counts].map(([label,count])=>`${count} ${label}`).join("、");
};
export const liveSelectionSummary=(items:readonly SelectionItem[],view?:DocumentView)=>countedSummary([...new Map(items.map(item=>[selectionKey(item),item])).values()].map(item=>selectionMetadata(item,view)));
