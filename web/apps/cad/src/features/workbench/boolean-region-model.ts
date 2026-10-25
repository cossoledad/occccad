import type {Artifact} from '../../types';
export type BooleanRegionRef={boundaryUnique:boolean;boundary:string[];inputDigest:string;cellDigest:string};
export type BooleanResultSelection={mode:'ALL'|'EXPLICIT';regions?:BooleanRegionRef[]};
export type BooleanRegionCell={reference:BooleanRegionRef;inTarget:boolean;inTool:boolean;eligible:boolean;operations:string[];artifact:Artifact};
export type BooleanRegionsAnalysis={versionId:string;regions:BooleanRegionCell[];inputs:Artifact[]};
export type BooleanRegionsPresentation={inputs:Artifact[];cells:(BooleanRegionCell&{retained:boolean})[];isolateId?:string;focusId?:string;focusRequest?:number;targetBodyId:string;toolBodyId:string};
export function matchBooleanRegion(ref:BooleanRegionRef,cells:readonly BooleanRegionCell[]):{cell?:BooleanRegionCell;status:'RESOLVED'|'MISSING'|'AMBIGUOUS'}{
 const matches=cells.filter(c=>c.reference.boundary.length===ref.boundary.length&&c.reference.boundary.every((v,i)=>v===ref.boundary[i]));
 if(ref.boundaryUnique&&matches.length===1&&matches[0].reference.boundaryUnique&&matches[0].eligible)return {cell:matches[0],status:'RESOLVED'};
 const frozen=cells.filter(c=>c.eligible&&c.reference.inputDigest===ref.inputDigest&&c.reference.cellDigest===ref.cellDigest);
 if(frozen.length===1)return {cell:frozen[0],status:'RESOLVED'};
 return {status:matches.length>1||matches.some(c=>!c.reference.boundaryUnique)||!ref.boundaryUnique?'AMBIGUOUS':'MISSING'};
}
