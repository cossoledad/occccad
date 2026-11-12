import type {GeometricInput,Vec3} from '../../types';
export type CurvatureSample={position:Vec3;normal:Vec3;minimum:number;maximum:number;mean:number;gaussian:number;surface:boolean};
export type ShapeQualityReport={documentId:string;revisionId:string;input:GeometricInput;result:{complete:boolean;diagnostic?:string;geometryType:string;samples:CurvatureSample[];undefinedSamples:number;kernelBuild:string;boundaryComplete?:boolean;boundaryEdges?:{edgeId:number;points:Vec3[]}[];gapCandidates?:{firstEdge:number;secondEdge:number;first:Vec3;second:Vec3;distanceMm:number}[]}};
export type CurvatureMode='minimum'|'maximum'|'mean'|'gaussian'|'normals'|'boundaries'|'gaps';
export const curvatureModes=[{label:'最小主曲率',value:'minimum'},{label:'最大主曲率',value:'maximum'},{label:'平均曲率',value:'mean'},{label:'高斯曲率',value:'gaussian'}] as const;
export function curvatureRange(report:ShapeQualityReport,mode:CurvatureMode):[number,number]|undefined {
 if(mode==='normals'||mode==='boundaries'||mode==='gaps')return;
 const surface=report.result.samples.filter(s=>s.surface),samples=surface.length?surface:report.result.samples;
 const values=samples.map(s=>s[mode]).filter(Number.isFinite);if(!values.length)return;
 return [Math.min(...values),Math.max(...values)];
}
export function curvatureNumber(value:number):string {return value===0?'0':Math.abs(value)<1e-4||Math.abs(value)>=1e5?value.toExponential(4):Number(value.toPrecision(6)).toString()}
