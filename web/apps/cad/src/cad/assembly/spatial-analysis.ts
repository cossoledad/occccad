import type {InstancePath,DocumentView} from '../../types';
import type {StaticInterferenceReport,InterferenceScope} from './static-interference';
import {randomUUID} from '../../utils/random-uuid';
export type SpatialAnalysis={id:string;name:string;kind:'CLASH'|'SECTION'|'DISTANCE'|'BAND';scope:InterferenceScope;selected?:InstancePath[];secondSelection?:InstancePath[];clearanceMm:number;sectionType:'PLANE'|'SLICE'|'BOX';frame:{translation:[number,number,number];rotation:[number,number,number,number]};size:[number,number,number];cutMode:'NONE'|'KEEP_INSIDE'|'KEEP_OUTSIDE';minimumMm:number;maximumMm:number;accuracyMm:number;result?:{artifactId:string;sha256:string;sourceRevisionId:string;sourceStamp:string;policy:string};status?:string};
export type SpatialReport={documentId:string;revisionId:string;definition:SpatialAnalysis;sourceStamp:string;policy:string;check:StaticInterferenceReport;geometry?:{complete:boolean;kernelBuild:string;bodies:{id:string;complete:boolean;diagnostic?:string;segments?:[number,number,number][];triangles?:[number,number,number][];regions?:number[];areaMm2:number;centroid:[number,number,number]}[]};complete:boolean};
export const spatialStatus=(s?:string)=>({CURRENT:'已更新',STALE:'待更新',BROKEN:'来源不可用',MISSING:'尚无结果',UNKNOWN:'状态暂不可用'}[s??'']??'待更新');
export const spatialTitle=(kind:SpatialAnalysis['kind'])=>({CLASH:'碰撞',SECTION:'切割',DISTANCE:'距离和区域分析',BAND:'距离和区域分析'}[kind]);
export function defaultSpatial(kind:SpatialAnalysis['kind']):SpatialAnalysis{return {id:randomUUID(),name:'',kind,scope:kind==='BAND'?'BETWEEN':'ALL',clearanceMm:0,sectionType:'PLANE',frame:{translation:[0,0,0],rotation:[0,0,0,1]},size:[100,100,10],cutMode:'NONE',minimumMm:0,maximumMm:10,accuracyMm:2}}
/** Saved path identities are rebased only against the new accepted tree, never by label. */
export function spatialPathsForView(view:DocumentView,definition:SpatialAnalysis):SpatialAnalysis {
 const paths=new Map<string,InstancePath>();const walk=(n:NonNullable<DocumentView['structureTree']>)=>{const p=n.instancePath??n.occurrence?.instancePath;if(p?.canonical)paths.set(p.canonical,p);n.children?.forEach(walk)};if(view.structureTree)walk(view.structureTree);
 return {...definition,selected:definition.selected?.map(p=>paths.get(p.canonical)??p),secondSelection:definition.secondSelection?.map(p=>paths.get(p.canonical)??p)};
}
