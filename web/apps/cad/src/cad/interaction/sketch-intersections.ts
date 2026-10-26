import type {SketchEntity,SketchGeometryRef,Vec2} from '../../types';
import {ellipsePoint} from '../sketch/sketch-geometry';
export type SketchIntersection={point:Vec2;parameters:[number,number];references:[SketchGeometryRef,SketchGeometryRef];constraintSupported:boolean};
export type SketchIntersections={versionId:string;sketchId:string;sourceDigests:[string,string];intersections:SketchIntersection[]};
export type SketchIntersectionQuery=(ids:[string,string],signal:AbortSignal)=>Promise<SketchIntersections>;
const distance=(p:Vec2,a:Vec2,b:Vec2)=>{const dx=b[0]-a[0],dy=b[1]-a[1],l=dx*dx+dy*dy,t=l?Math.max(0,Math.min(1,((p[0]-a[0])*dx+(p[1]-a[1])*dy)/l)):0;return Math.hypot(p[0]-a[0]-t*dx,p[1]-a[1]-t*dy)};
// Distance to a projected control hull is a lower bound, unlike chords
// between samples. Positive rational conics/B-splines stay inside their hulls.
// This broad phase cannot certify an intersection: only the Worker can.
function hullDistance(cursor:Vec2,points:Vec2[]):number {
 if(!points.length||points.some(p=>!p.every(Number.isFinite)))return 0;
 if(points.length===2)return distance(cursor,points[0],points[1]);
 if(points.length===3){
  const cross=(a:Vec2,b:Vec2)=>(b[0]-a[0])*(cursor[1]-a[1])-(b[1]-a[1])*(cursor[0]-a[0]);
  const sides=points.map((p,i)=>cross(p,points[(i+1)%3]));
  if(sides.every(s=>s>=0)||sides.every(s=>s<=0))return 0;
  return Math.min(...points.map((p,i)=>distance(cursor,p,points[(i+1)%3])));
 }
 const xs=points.map(p=>p[0]),ys=points.map(p=>p[1]);
 return Math.hypot(Math.max(0,Math.min(...xs)-cursor[0],cursor[0]-Math.max(...xs)),Math.max(0,Math.min(...ys)-cursor[1],cursor[1]-Math.max(...ys)));
}
function curveNeighborhoodDistance(entity:SketchEntity,cursor:Vec2,project:(p:Vec2)=>Vec2):number {
 if(entity.kind==='LINE'&&entity.start&&entity.end)return distance(cursor,project([entity.start.x,entity.start.y]),project([entity.end.x,entity.end.y]));
 if(['CIRCLE','ARC','ELLIPSE','ELLIPTICAL_ARC'].includes(entity.kind)&&entity.center){
  const full=entity.kind==='CIRCLE'||entity.kind==='ELLIPSE';
  const start=full?0:entity.startAngle??0,end=full?2*Math.PI:entity.endAngle??0;
  const at=(t:number):Vec2|undefined=>entity.kind==='CIRCLE'||entity.kind==='ARC'
   ?[entity.center!.x+entity.radius!*Math.cos(t),entity.center!.y+entity.radius!*Math.sin(t)]:ellipsePoint(entity,t);
  // Exact rational quadratic sectors: endpoints and intersection of tangents.
  // A fixed number of conservative hulls stays safe at every zoom level.
  const count=Math.max(1,Math.ceil(Math.abs(end-start)/(Math.PI/8)));let lower=Infinity;
  for(let i=0;i<count;i++){
   const a=start+(end-start)*i/count,b=start+(end-start)*(i+1)/count,m=(a+b)/2;
   const p=at(a),q=at(b),mid=at(m);if(!p||!q||!mid)return 0;
   const weight=Math.cos((b-a)/2),c=entity.center;
   const control:Vec2=[c.x+(mid[0]-c.x)/weight,c.y+(mid[1]-c.y)/weight];
   lower=Math.min(lower,hullDistance(cursor,[p,control,q].map(project)));
  }
  return lower;
 }
 if(entity.kind==='SPLINE'&&entity.poles?.length&&entity.weights?.every(w=>w>0))
  return hullDistance(cursor,entity.poles.map(p=>project([p.x,p.y])));
 // Missing authoritative canonical geometry must not become a false rejection.
 return 0;
}
// At most four nearby curves (six pairs); no all-sketch pairwise queries.
export class SketchIntersectionCache{
 private owner='';private ready?:()=>void;private entries=new Map<string,{controller:AbortController;value?:SketchIntersections;pending:boolean}>();
 retry(){for(const [key,e] of this.entries)if(!e.pending&&!e.value)this.entries.delete(key)}
 reset(){for(const e of this.entries.values())e.controller.abort();this.entries.clear();this.owner='';this.ready=undefined;}
 near(owner:string,versionId:string,sketchId:string,raw:Vec2,entities:readonly SketchEntity[],project:(p:Vec2)=>Vec2,query:SketchIntersectionQuery,onReady:()=>void,required:readonly string[]=[]):SketchIntersection[]{
  if(owner!==this.owner){this.reset();this.owner=owner;}
  this.ready=onReady;
  const cursor=project(raw),candidates=entities.filter(e=>!e.suppressed&&e.kind!=='POINT'&&!e.infinite).map(e=>{return {e,d:curveNeighborhoodDistance(e,cursor,project)}}).filter(c=>c.d<=18||required.includes(c.e.id)).sort((a,b)=>a.d-b.d||a.e.id.localeCompare(b.e.id));
  const retained=candidates.filter(c=>required.includes(c.e.id));
  const near=required.length===2?retained:[...retained,...candidates.filter(c=>!required.includes(c.e.id)).slice(0,4-retained.length)];
  const hits:SketchIntersection[]=[];
  for(let i=0;i<near.length;i++)for(let j=i+1;j<near.length;j++){
   const pair=[near[i].e,near[j].e].sort((a,b)=>a.id.localeCompare(b.id));if(required.length&&!required.every(id=>pair.some(e=>e.id===id)))continue;
   const key=JSON.stringify(pair);let entry=this.entries.get(key);
   if(!entry&&this.entries.size>=256){const stale=[...this.entries].find(([,e])=>!e.pending);if(stale)this.entries.delete(stale[0]);}
   if(!entry&&this.entries.size<256&&[...this.entries.values()].filter(e=>e.pending).length<6){entry={controller:new AbortController(),pending:true};this.entries.set(key,entry);const current=entry;
    void query([pair[0].id,pair[1].id],current.controller.signal).then(value=>{if(this.owner!==owner||current.controller.signal.aborted||value.versionId!==versionId||value.sketchId!==sketchId)return;current.value=value;current.pending=false;this.ready?.()},()=>{current.pending=false;if(this.owner===owner&&!current.controller.signal.aborted)this.ready?.()}).finally(()=>{current.pending=false});
   }
   if(entry?.value)hits.push(...entry.value.intersections);
  }
  return hits.filter(hit=>{const p=project(hit.point);return Math.hypot(p[0]-cursor[0],p[1]-cursor[1])<=13}).sort((a,b)=>{const p=project(a.point),q=project(b.point);return Math.hypot(p[0]-cursor[0],p[1]-cursor[1])-Math.hypot(q[0]-cursor[0],q[1]-cursor[1])||JSON.stringify(a.references).localeCompare(JSON.stringify(b.references))});
 }
}
