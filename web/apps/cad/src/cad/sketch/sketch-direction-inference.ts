import type {SketchEntity,SketchGeometryRef,Vec2} from '../../types';

export type SketchDirectionInference={kind:'HORIZONTAL'|'VERTICAL'|'PARALLEL'|'PERPENDICULAR';point:Vec2;reference?:SketchGeometryRef;label:string};
const policy={pixels:7,angle:3*Math.PI/180,minimumPixels:24,references:128};

/** A visible creation suggestion, never a repair of an existing reference.
 * Physical angle and CSS-pixel distance must both agree, including under zoom.
 * Only ordinary local lines supplied by the tool can become formal supports. */
export function inferSketchDirection(first:Vec2,point:Vec2,entities:readonly SketchEntity[],project:(point:Vec2)=>Vec2|undefined,preserveLength=false):SketchDirectionInference|undefined{
 const a=project(first),b=project(point),dx=point[0]-first[0],dy=point[1]-first[1],length=Math.hypot(dx,dy);
 if(!a||!b||length===0||Math.hypot(b[0]-a[0],b[1]-a[1])<policy.minimumPixels)return;
 type Direction={kind:SketchDirectionInference['kind'];u:Vec2;reference?:SketchGeometryRef;label:string};
 const directions:Direction[]=[{kind:'HORIZONTAL',u:[1,0],label:'水平'},{kind:'VERTICAL',u:[0,1],label:'竖直'}];
 const nearby=entities.filter(e=>e.kind==='LINE'&&e.start&&e.end&&!e.suppressed).map(e=>{
  const start=project([e.start!.x,e.start!.y]),end=project([e.end!.x,e.end!.y]);
  if(!start||!end)return {entity:e,distance:Infinity};
  const vx=end[0]-start[0],vy=end[1]-start[1],l2=vx*vx+vy*vy,t=l2?Math.max(0,Math.min(1,((a[0]-start[0])*vx+(a[1]-start[1])*vy)/l2)):0;
  return {entity:e,distance:Math.hypot(a[0]-start[0]-t*vx,a[1]-start[1]-t*vy)};
 }).filter(e=>e.distance<=160).sort((x,y)=>x.distance-y.distance||x.entity.id.localeCompare(y.entity.id)).slice(0,policy.references);
 for(const {entity:e} of nearby){
  const vx=e.end!.x-e.start!.x,vy=e.end!.y-e.start!.y,l=Math.hypot(vx,vy);if(l===0)continue;
  const reference:SketchGeometryRef={target:'ENTITY',entityId:e.id,subElement:'WHOLE'};
  directions.push({kind:'PARALLEL',u:[vx/l,vy/l],reference,label:`平行 · ${e.name??'直线'}`},{kind:'PERPENDICULAR',u:[-vy/l,vx/l],reference,label:`垂直 · ${e.name??'直线'}`});
 }
 let best:{inference:SketchDirectionInference;error:number}|undefined;
 for(const direction of directions){
  const dot=dx*direction.u[0]+dy*direction.u[1];
  if(Math.abs(dx*direction.u[1]-dy*direction.u[0])/length>Math.sin(policy.angle))continue;
  const distance=preserveLength?Math.sign(dot)*length:dot;
  const aligned:Vec2=[first[0]+distance*direction.u[0],first[1]+distance*direction.u[1]],screen=project(aligned);if(!screen)continue;
  const error=Math.hypot(screen[0]-b[0],screen[1]-b[1]);
  if(error>policy.pixels||best&&error>=best.error-1e-7)continue;
  best={error,inference:{kind:direction.kind,point:aligned,reference:direction.reference,label:direction.label}};
 }
 return best?.inference;
}
