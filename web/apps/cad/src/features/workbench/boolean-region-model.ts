import type {Artifact} from '../../types';
export type BooleanRegionRef={boundaryUnique:boolean;boundary:string[];inputDigest:string;cellDigest:string};
export type BooleanResultSelection={mode:'ALL'|'EXPLICIT';regions?:BooleanRegionRef[]};
export type BooleanRegionCell={reference:BooleanRegionRef;inTarget:boolean;inTool:boolean;eligible:boolean;operations:string[];artifact:Artifact};
export type BooleanRegionsAnalysis={analysisId:string;manifestDigest:string;versionId:string;regions:BooleanRegionCell[];inputs:Artifact[]};
export type BooleanRegionsPresentation={inputs:Artifact[];cells:(BooleanRegionCell&{retained:boolean})[];isolateId?:string;focusId?:string;focusRequest?:number;targetBodyId:string;toolBodyId:string};
export function matchBooleanRegion(ref:BooleanRegionRef,cells:readonly BooleanRegionCell[]):{cell?:BooleanRegionCell;status:'RESOLVED'|'MISSING'|'AMBIGUOUS'}{
 const matches=cells.filter(c=>c.reference.boundary.length===ref.boundary.length&&c.reference.boundary.every((v,i)=>v===ref.boundary[i]));
 if(ref.boundaryUnique&&matches.length===1&&matches[0].reference.boundaryUnique&&matches[0].eligible)return {cell:matches[0],status:'RESOLVED'};
 const frozen=cells.filter(c=>c.eligible&&c.reference.inputDigest===ref.inputDigest&&c.reference.cellDigest===ref.cellDigest);
 if(frozen.length===1)return {cell:frozen[0],status:'RESOLVED'};
 return {status:matches.length>1||matches.some(c=>!c.reference.boundaryUnique)||!ref.boundaryUnique?'AMBIGUOUS':'MISSING'};
}

// Replacements are a set of resolved current cells; unresolved inputs stay visible.
export function selectBooleanRegion(refs:readonly BooleanRegionRef[],cells:readonly BooleanRegionCell[],cell:BooleanRegionCell,replacing?:number):BooleanRegionRef[]{
 if(!cell.eligible)return [...refs];
 const index=refs.findIndex(ref=>matchBooleanRegion(ref,cells).cell?.reference.cellDigest===cell.reference.cellDigest);
 const next=replacing!==undefined?refs.map((ref,i)=>i===replacing?cell.reference:ref):index>=0?refs.filter(ref=>matchBooleanRegion(ref,cells).cell?.reference.cellDigest!==cell.reference.cellDigest):[...refs,cell.reference];
 const seen=new Set<string>();return next.flatMap(ref=>{const current=matchBooleanRegion(ref,cells).cell?.reference;if(!current)return [ref];const key=current.inputDigest+'/'+current.cellDigest;if(seen.has(key))return [];seen.add(key);return [current]});
}
