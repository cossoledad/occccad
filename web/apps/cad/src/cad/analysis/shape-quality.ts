import type {GeometricInput,Vec3} from '../../types';
export type CurvatureSample={position:Vec3;normal:Vec3;minimum:number;maximum:number;mean:number;gaussian:number;surface:boolean};
export type QualityBoundaryEdge={edgeId:number;firstVertex:number;lastVertex:number;points:Vec3[]};
export type ShapeQualityReport={documentId:string;revisionId:string;input:GeometricInput;result:{complete:boolean;diagnostic?:string;geometryType:string;samples:CurvatureSample[];undefinedSamples:number;kernelBuild:string;boundaryComplete?:boolean;boundaryEdges?:QualityBoundaryEdge[];gapCandidates?:{firstEdge:number;secondEdge:number;first:Vec3;second:Vec3;distanceMm:number}[]}};
export type CurvatureMode='minimum'|'maximum'|'mean'|'gaussian'|'normals'|'boundaries'|'gaps';
export const curvatureModes=[{label:'最小主曲率',value:'minimum'},{label:'最大主曲率',value:'maximum'},{label:'平均曲率',value:'mean'},{label:'高斯曲率',value:'gaussian'}] as const;
export function curvatureRange(report:ShapeQualityReport,mode:CurvatureMode):[number,number]|undefined {
 if(mode==='normals'||mode==='boundaries'||mode==='gaps')return;
 const surface=report.result.samples.filter(s=>s.surface),samples=surface.length?surface:report.result.samples;
 const values=samples.map(s=>s[mode]).filter(Number.isFinite);if(!values.length)return;
 return [Math.min(...values),Math.max(...values)];
}
export function curvatureNumber(value:number):string {return value===0?'0':Math.abs(value)<1e-4||Math.abs(value)>=1e5?value.toExponential(4):Number(value.toPrecision(6)).toString()}

// Volatile locators are scoped to this analysis. Only components with exactly
// two incident uses at every vertex can seed a Fill. A periodic edge has two
// uses of the same vertex.
export function closedBoundaryLoops(edges:QualityBoundaryEdge[]):QualityBoundaryEdge[][] {
 const adjacent=new Map<number,QualityBoundaryEdge[]>();
 for(const edge of edges) for(const vertex of [edge.firstVertex,edge.lastVertex]){
  const incident=adjacent.get(vertex)??[];incident.push(edge);adjacent.set(vertex,incident);
 }
 const seen=new Set<number>(),loops:QualityBoundaryEdge[][]=[];
 for(const start of edges){
  if(seen.has(start.edgeId))continue;
  const component:QualityBoundaryEdge[]=[],pending=[start];let closed=true;
  while(pending.length){
   const edge=pending.pop()!;
   if(seen.has(edge.edgeId))continue;
   seen.add(edge.edgeId);component.push(edge);
   for(const vertex of [edge.firstVertex,edge.lastVertex]){
    const incident=adjacent.get(vertex)!;
    if(!vertex||incident.length!==2)closed=false;
    pending.push(...incident.filter(e=>!seen.has(e.edgeId)));
   }
  }
  if(closed)loops.push(component);
 }
 return loops;
}
