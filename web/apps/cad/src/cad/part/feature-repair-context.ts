import type {Artifact,DatumPlane,DocumentView,Feature,InstancePath,ParameterDefinition} from '../../types';
import {partDependencySnapshotMatches,type PartDependencyInput,type PartDependencyNode} from './part-dependency-graph';

export type FeatureRepairSnapshot={versionId:string;rootProductRevisionId?:string;contextVariantId?:string;createdAt:string;feature:Feature;parameters:ParameterDefinition[];inputs:PartDependencyInput[];artifacts:Artifact[];sketchFrame?:DatumPlane;evaluatorVersion:string;reconstructed:boolean};
export type FeatureRepairContext={documentId:string;baseVersionId:string;featureId:string;state:string;examined:number;searchLimit:number;searchDocumentId:string;displayId?:string;snapshot?:FeatureRepairSnapshot;downstream:PartDependencyNode[];currentInputs:PartDependencyInput[];currentParameters:ParameterDefinition[];rootProductDocumentId?:string;rootProductRevisionId?:string;contextVariantId?:string;instancePath?:InstancePath};
export function featureRepairSnapshotMatches(view:DocumentView,data:FeatureRepairContext,featureId:string){
 return data.featureId===featureId&&(!data.snapshot||data.snapshot.feature.id===featureId)&&partDependencySnapshotMatches(view,{...data,versionId:data.baseVersionId,nodes:[],edges:[],digest:''});
}
export function repairParameterPairs(context:FeatureRepairContext){
 const old=new Map(context.snapshot?.parameters.map(p=>[p.parameterId,p]));
 const current=new Map(context.currentParameters.map(p=>[p.parameterId,p]));
 return [...new Set([...current.keys(),...old.keys()])].map(id=>({id,current:current.get(id),historical:old.get(id)}));
}
