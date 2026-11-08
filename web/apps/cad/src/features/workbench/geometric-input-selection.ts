import {api} from '../../api/client';
import {CommandInputError} from '../../cad/command/operation-failure';
import type {DocumentView,Feature,GeometricInput,SelectionItem} from '../../types';
import {sketchGeometryInput} from './geometric-feature-model';

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
