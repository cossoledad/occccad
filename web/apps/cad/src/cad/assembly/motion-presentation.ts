import {formatDisplayNumber} from "../../utils/display-number";
import type {AssemblyBodyFreedom,DocumentView,Vec3} from "../../types";
import {engineeringInstanceName,type AssemblyEngineeringEvidence} from "./assembly-engineering-state";

export type MotionPresentation={id:string;bodyId:string;documentId:string;revisionId:string;ownerOccurrence:string;name:string;reference:string;description:string;status:"可运动"|"已定位"|"待计算"|"结果已失效";scope:string[];freedom?:AssemblyBodyFreedom;gaugeDof:number;frame:"OWNING_PRODUCT";unit:"mm"};
const dot=(a:Vec3,b:Vec3)=>a.reduce((s,v,i)=>s+v*b[i],0);
export const displayNumber=formatDisplayNumber;
export function displayDirection(v:Vec3):string {return v.map(n=>displayNumber(n)).join(", ");}
// Length-only display noise floor in millimetres, below physical tolerances.
// Display precision follows the shared UI policy; computation stays in millimetres.
export function displayLength(mm:number,unit:"mm"|"cm"|"m"|"in"="mm"):string {
 const scale={mm:1,cm:10,m:1000,in:25.4}[unit];
 const value=Math.abs(mm)<=1e-12?0:mm/scale;
 return `${formatDisplayNumber(value)} ${unit}`;
}
// Compare the span, not SVD column order/sign. Axis names describe directions,
// never the location of a rotation axis. All directions are owning-Product axes.
export function translationLabel(directions:Vec3[]):string {
 if(directions.length===1){const d=directions[0],length=Math.sqrt(dot(d,d));const axis=d.findIndex(n=>Math.abs(n)/length>1-1e-8);return axis<0?"沿图示方向平移":`沿装配 ${["X","Y","Z"][axis]} 方向平移`;}
 if(directions.length===2){const [a,b]=directions;const n:Vec3=[a[1]*b[2]-a[2]*b[1],a[2]*b[0]-a[0]*b[2],a[0]*b[1]-a[1]*b[0]];const length=Math.sqrt(dot(n,n));const axis=n.findIndex(v=>Math.abs(v)/length>1-1e-8);return axis<0?"在图示平面内平移":`在装配 ${["YZ","XZ","XY"][axis]} 方向平面内平移`;}
 return "沿图示方向平移";
}
function describe(f:AssemblyBodyFreedom):string {
 switch(f.kind){
 case 0:return "保持刚性关系";
 case 1:return "绕图示轴旋转";
 case 2:return translationLabel(f.translationDirections);
 case 3:return "沿轴平移并绕该轴旋转";
 case 4:return `${translationLabel(f.translationDirections)}，并绕其法向旋转`;
 case 5:return "绕图示中心转动";
 case 6:return "自由移动和旋转";
 default:return "组合运动";
 }
}
export function motionPresentations(view:DocumentView|undefined,evidence?:AssemblyEngineeringEvidence,ownerOccurrence=""):MotionPresentation[]{
 if(!view)return [];
 const revisionId=view.document.versionId,documentId=view.document.id;
 const current=evidence?.documentId===documentId&&evidence.revisionId===revisionId;
 const components=new Map(evidence?.components.flatMap(c=>c.bodyIds.map(id=>[id,c] as const))??[]);
 const freedoms=new Map(evidence?.components.flatMap(c=>c.freedoms.map(f=>[f.bodyId,f] as const))??[]);
 return (view.product?.instances??[]).map(i=>{
  const row:MotionPresentation={id:`${documentId}/${ownerOccurrence}/${i.id}`,bodyId:i.id,documentId,revisionId,ownerOccurrence,name:engineeringInstanceName(view,i.id),reference:"尚无参考证据",description:"此版本暂无运动证据",status:"待计算",scope:[],gaugeDof:0,frame:"OWNING_PRODUCT",unit:"mm"};
  if(evidence&&!current)return {...row,status:"结果已失效",description:"结果已失效"};
  const c=components.get(i.id),f=freedoms.get(i.id);
  if(!current||!evidence?.available||!c||!c.solved||!f||evidence.coordinateFrame!=="OWNING_PRODUCT"||evidence.lengthUnit!=="mm"||!evidence.referenceSemantics)return {...row,description:c&&!c.solved?"所在范围尚未求解":c?"参考语义尚未确认":"此版本未覆盖该组件"};
  row.scope=c.bodyIds;row.gaugeDof=c.gaugeDof;
  const relative=evidence.referenceSemantics==="STATIC_RELATIVE"&&c.gaugeDof>0;
  if(relative&&(!f.relativeToBodyId||!c.bodyIds.includes(f.relativeToBodyId)))return {...row,description:"参考对象证据不足"};
  row.reference=relative?"连通组件（整体可动）":"当前装配框架";
  if(relative&&f.relativeToBodyId&&f.relativeToBodyId!==i.id)row.reference=engineeringInstanceName(view,f.relativeToBodyId);
  // The numerical anchor has zero *relative* motion. It is not a ground.
  if(relative&&f.relativeToBodyId===i.id)return {...row,freedom:f,status:"可运动",description:"随连通组件整体移动和旋转"};
  row.freedom=f;row.description=describe(f);row.status=f.kind===0&&!relative?"已定位":"可运动";
  if(f.kind===0)row.description=relative?"相对参考组件保持刚性关系；整体可动":"在当前装配中保持固定";
  return row;
 });
}
