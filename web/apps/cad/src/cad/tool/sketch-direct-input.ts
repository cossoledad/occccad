import type { SketchEntity, SketchGeometryRef, Vec2 } from "../../types";
import { parseParameterSource } from "../../features/workbench/parameter-editor";

export type DirectEditInput =
  | { kind: "mirror"; route: "axis-first"|"source-first"; stage: "axis"|"sources" }
  | { kind: "corner"; stage: "curves"|"branch"|"value"; clicks: Vec2[]; inputId: string; anchor?:Vec2; dimensionAnchors?:Vec2[]; unit?: "mm"|"cm"|"m"|"in" }
  | { kind: "split"; first?: { entityId: string; parameter: number } }
  | { kind: "trim" }
  | { kind: "extend"; stage: "curve"|"target" };

// These intersections choose click intent only. The submitted operation keeps
// stable curve references; production evaluation computes and validates cuts.
export function cornerClickIntent(curves: readonly SketchEntity[], clicks: readonly Vec2[]): { references: SketchGeometryRef[]; point: Vec2 }|undefined {
  if(curves.length!==2||clicks.length!==2)return;
  const [a,b]=curves;
  let corner: Vec2|undefined;
  if(a.kind==="LINE"&&b.kind==="LINE"&&a.start&&a.end&&b.start&&b.end){
    const u=[a.end.x-a.start.x,a.end.y-a.start.y],v=[b.end.x-b.start.x,b.end.y-b.start.y],det=u[0]*v[1]-u[1]*v[0];
    if(det!==0){const t=((b.start.x-a.start.x)*v[1]-(b.start.y-a.start.y)*v[0])/det;corner=[a.start.x+t*u[0],a.start.y+t*u[1]];}
  } else {
    const endpoints=(e:SketchEntity):Vec2[]=>e.kind==="LINE"&&e.start&&e.end?[[e.start.x,e.start.y],[e.end.x,e.end.y]]:e.center&&e.radius!==undefined?[(e.startAngle??0),(e.endAngle??0)].map(t=>[e.center!.x+e.radius!*Math.cos(t),e.center!.y+e.radius!*Math.sin(t)]):[];
    const ends=[endpoints(a),endpoints(b)];
    // A common geometric endpoint supplies branch intent, never a weld.
    const candidates=ends[0].filter(p=>ends[1].some(q=>Math.hypot(p[0]-q[0],p[1]-q[1])<1e-7));
    if(candidates.length===1)corner=candidates[0];
  }
  if(!corner)return;
  const endpoint=(e:SketchEntity,end:boolean):Vec2|undefined=>e.kind==="LINE"?(end?e.end:e.start)&&[(end?e.end:e.start)!.x,(end?e.end:e.start)!.y]:e.center&&e.radius!==undefined?[e.center.x+e.radius*Math.cos((end?e.endAngle:e.startAngle)??0),e.center.y+e.radius*Math.sin((end?e.endAngle:e.startAngle)??0)]:undefined;
  const references=curves.map(e=>{
    const start=endpoint(e,false)!,end=endpoint(e,true)!;
    const subElement:"START"|"END"=Math.hypot(start[0]-corner![0],start[1]-corner![1])<=Math.hypot(end[0]-corner![0],end[1]-corner![1])?"START":"END";
    return {target:"ENTITY" as const,entityId:e.id,subElement,pointId:subElement==="START"?e.startPointId:e.endPointId};
  });
  return {references,point:[(clicks[0][0]+clicks[1][0])/2,(clicks[0][1]+clicks[1][1])/2]};
}

export function directLengthInput(source:string,unit:string,fallback:number):{value:number;parameterSource:string} {
  const parsed=parseParameterSource(source,unit);
  if(parsed.kind==="EXPRESSION"){
    if(!parsed.expression||/^[+\-]$/.test(parsed.expression))throw new Error("请输入长度或完整表达式");
    return {value:fallback,parameterSource:parsed.expression};
  }
  const scale:Record<string,number>={mm:1,cm:10,m:1000,in:25.4};
  const value=parsed.value*scale[parsed.unit];
  if(!Number.isFinite(value)||value<=0)throw new Error("长度必须大于零，单位使用 mm、cm、m 或 in");
  return {value,parameterSource:`${parsed.value} ${parsed.unit}`};
}
