import type {SketchEntity,SketchGeometryRef,Vec2} from '../../types';
import {sampleSketchEntity} from '../sketch/sketch-geometry';
export type SketchIntersection={point:Vec2;parameters:[number,number];references:[SketchGeometryRef,SketchGeometryRef];constraintSupported:boolean};
export type SketchIntersections={versionId:string;sketchId:string;sourceDigests:[string,string];intersections:SketchIntersection[]};
export type SketchIntersectionQuery=(ids:[string,string],signal:AbortSignal)=>Promise<SketchIntersections>;
const distance=(p:Vec2,a:Vec2,b:Vec2)=>{const dx=b[0]-a[0],dy=b[1]-a[1],l=dx*dx+dy*dy,t=l?Math.max(0,Math.min(1,((p[0]-a[0])*dx+(p[1]-a[1])*dy)/l)):0;return Math.hypot(p[0]-a[0]-t*dx,p[1]-a[1]-t*dy)};
// At most four nearby curves (six pairs) per cursor neighborhood. Samples only
// reject distant candidates; every offered intersection comes from exact BREP.
export class SketchIntersectionCache{
 private owner='';private ready?:()=>void;private entries=new Map<string,{controller:AbortController;value?:SketchIntersections;pending:boolean}>();
 retry(){for(const [key,e] of this.entries)if(!e.pending&&!e.value)this.entries.delete(key)}
 reset(){for(const e of this.entries.values())e.controller.abort();this.entries.clear();this.owner='';this.ready=undefined;}
 near(owner:string,versionId:string,sketchId:string,raw:Vec2,entities:readonly SketchEntity[],project:(p:Vec2)=>Vec2,query:SketchIntersectionQuery,onReady:()=>void,required:readonly string[]=[]):SketchIntersection[]{
  if(owner!==this.owner){this.reset();this.owner=owner;}
  this.ready=onReady;
  const cursor=project(raw),candidates=entities.filter(e=>!e.suppressed&&e.kind!=='POINT'&&!e.infinite).map(e=>{const pts=sampleSketchEntity(e,48).map(project);let d=Infinity;for(let i=1;i<pts.length;i++)d=Math.min(d,distance(cursor,pts[i-1],pts[i]));return {e,d}}).filter(c=>c.d<=18||required.includes(c.e.id)).sort((a,b)=>a.d-b.d||a.e.id.localeCompare(b.e.id));
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
