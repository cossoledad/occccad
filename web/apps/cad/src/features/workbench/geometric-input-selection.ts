import {api} from '../../api/client';
import {CommandInputError} from '../../cad/command/operation-failure';
import type {DocumentView,Feature,GeometricInput,SelectionItem} from '../../types';
import {sketchGeometryInput,type GeometryReferenceRole} from './geometric-feature-model';

/** Whole-skin inputs promote a hit only through its qualified result owner.
 * Topology lineage, source-feature hints and coincident coordinates cannot
 * turn a material Body face into an independent surface result. */
export function wholeSurfaceResultSelection(view:DocumentView,s:SelectionItem,features=view.part?.features??[]):SelectionItem|null{
 if((s.ownerDocumentId??s.documentId)!==view.document.id||s.versionId!==view.document.versionId||(s.occurrencePath??'')!=='')return null;
 if(['face','edge','vertex'].includes(s.kind)&&!s.result)return null;
 const f=features.find(f=>f.id===(s.result?.featureId??s.entityId??s.id));
 if(f?.type!=='GEOMETRY'||!f.result||f.result.domain!=='SHEET_SET'||f.suppressed||['FAILED','BLOCKED','SUPPRESSED'].includes(f.evaluationStatus??''))return null;
 if(s.geometryKey&&s.geometryKey!==f.result.geometryKey||s.result&&s.result.resultId!==f.result.id)return null;
 const element='topologyId' in s?s:{...s,topologyId:undefined};
 const {topologyId,associatedFeatureIds,sourceFeatureIds,treeNodeId,patternId,patternMemberSlot,entityRef,entityKind,entityType,...context}=element;
 return {...context,kind:'feature',id:f.id,entityId:f.id,result:{featureId:f.id,resultId:f.result.id},geometryKey:f.result.geometryKey};
}

export function acceptsGeometryReference(view:DocumentView,s:SelectionItem,role:GeometryReferenceRole):boolean{
 const feature=view.part?.features.find(f=>f.id===(s.result?.featureId??s.entityId??s.id));
 const id=s.entityId??s.id,reference=[...(view.part?.datumPoints??[]),...(view.part?.datumAxes??[]),...(view.part?.datumPlanes??[])].find(v=>v.id===id);if(reference?.evaluationStatus||feature?.evaluationStatus==='FAILED'||feature?.evaluationStatus==='BLOCKED')return false;
 if(role==='point')return s.kind==='vertex'||['reference-point','datum-point','axis-system'].includes(s.kind)||s.entityKind==='REFERENCE_POINT'||s.kind==='visual'&&s.visualType==='POINT'&&!!s.sketchReference||feature?.result?.domain==='POINT_SET';
 if(s.kind==='plane'||s.kind==='face'||feature?.result?.domain==='SHEET_SET')return role==='plane'||role==='direction';
 if(role==='plane')return false;
 if(s.kind==='axis'||s.kind==='edge'||feature?.result?.domain==='WIRE_SET')return true;
 return s.kind==='visual'&&s.visualType==='CURVE'&&!!view.part?.features.find(f=>f.id===s.featureId)?.sketch?.entities.some(e=>e.id===s.entityId&&e.kind==='LINE');
}

export async function bindGeometryReference(view:DocumentView,s:SelectionItem,versionId:string,role:GeometryReferenceRole):Promise<GeometricInput>{
 if((s.ownerDocumentId??s.documentId)!==view.document.id||s.versionId&&s.versionId!==versionId)throw new CommandInputError('所选对象的文档或版本已变化');
 let input:GeometricInput;
 if(role==='point'&&(s.kind==='reference-point'||s.kind==='datum-point'||s.kind==='axis-system'||s.entityKind==='REFERENCE_POINT'))input={kind:'DATUM',datum:{kind:'POINT',entityId:s.entityId??s.id}};
 else if(s.kind==='axis')input={kind:'DATUM',datum:s.axis==='DATUM'?{kind:'AXIS',entityId:s.entityId??s.id}:{kind:'AXIS_SYSTEM',entityId:s.entityId??s.id,axis:s.axis}};
 else if(role==='point'&&s.kind==='visual'&&s.visualType==='POINT'&&s.sketchReference)input={kind:'POINT',point:{sketchId:s.featureId,reference:s.sketchReference}};
 else if(role!=='point'&&s.kind==='visual'&&s.visualType==='CURVE')input={kind:'DATUM',datum:{kind:'SKETCH_LINE',featureId:s.featureId,entityId:s.entityId}};
 else input=await bindGeometricInput(view,s,versionId);
 if(input.kind==='TOPOLOGY'){
  const selection=input.selection!.selection,type=selection.creationEvidence.geometryType;
  if(role==='point'&&selection.expectedType!=='VERTEX')throw new CommandInputError('请选择点或顶点');
  if(role==='plane'&&(selection.expectedType!=='FACE'||type!=='PLANE'))throw new CommandInputError('请选择平面');
  if(role==='axis'&&(selection.expectedType!=='EDGE'||type!=='LINE'))throw new CommandInputError('请选择直线或参考轴');
  if(role==='direction'&&!(selection.expectedType==='EDGE'&&type==='LINE'||selection.expectedType==='FACE'&&type==='PLANE'))throw new CommandInputError('请选择直线、轴或平面法向');
 }
 return input;
}

export async function bindGeometricInput(view:DocumentView,s:SelectionItem,versionId:string,features:Feature[]=view.part?.features??[]):Promise<GeometricInput>{
 if((s.ownerDocumentId??s.documentId)!==view.document.id||(s.versionId&&s.versionId!==versionId))throw new CommandInputError('所选对象的文档或版本已变化');
 if(s.kind==='plane')return {kind:'PLANE',planeId:s.entityId??s.datumPlane?.id??s.id};
 const sketch=sketchGeometryInput(view,s);if(sketch)return sketch;
 if(s.kind==='edge'||s.kind==='face'||s.kind==='vertex'){
  if(!s.geometryKey)throw new CommandInputError('所选对象没有精确几何');
  const selection=await api.bindTopologySelection(view.document.id,{sourceVersionId:versionId,geometryKey:s.geometryKey,kind:s.kind.toUpperCase() as 'FACE'|'EDGE'|'VERTEX',localId:s.topologyId});
  const sourceFeatureId=selection.sourceResult?.featureId??s.displayStageFeatureId??[...features].reverse().find(f=>f.bodyId===selection.sourceBodyId&&!f.suppressed&&f.type!=='SKETCH'&&f.type!=='GEOMETRY')?.id;
  if(!sourceFeatureId||!features.some(f=>f.id===sourceFeatureId))throw new CommandInputError('所选对象的来源阶段不可用');
  return {kind:'TOPOLOGY',selection:{selection,sourceVersionId:versionId,sourceFeatureId}};
 }
 const f=features.find(f=>f.id===(s.result?.featureId??s.entityId??s.id));if(!f?.result)throw new CommandInputError('请选择曲线或曲面');return {kind:'RESULT',featureId:f.id,resultId:f.result.id};
}
