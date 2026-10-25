import type {SketchAngleSector,SketchEntity,SketchGeometryRef,Vec2} from '../../types';
const tau=2*Math.PI;
const positive=(a:number)=>(a%tau+tau)%tau;
export function sketchAngleLines(refs:readonly SketchGeometryRef[],entities:readonly SketchEntity[]):[Vec2,Vec2,Vec2]|undefined{
 const line=(r:SketchGeometryRef):[Vec2,Vec2]|undefined=>{if(r.target==='SKETCH_X_AXIS')return [[0,0],[1,0]];if(r.target==='SKETCH_Y_AXIS')return [[0,0],[0,1]];const e=entities.find(e=>e.id===r.entityId);return e?.kind==='LINE'&&e.start&&e.end?[[e.start.x,e.start.y],[e.end.x,e.end.y]]:undefined};
 if(refs.length!==2)return;const a=line(refs[0]),b=line(refs[1]);if(!a||!b)return;
 const u:Vec2=[a[1][0]-a[0][0],a[1][1]-a[0][1]],v:Vec2=[b[1][0]-b[0][0],b[1][1]-b[0][1]],det=u[0]*v[1]-u[1]*v[0];
 if(Math.hypot(...u)<1e-12||Math.hypot(...v)<1e-12)return;
 const t=Math.abs(det)>1e-12?((b[0][0]-a[0][0])*v[1]-(b[0][1]-a[0][1])*v[0])/det:0;
 return [[a[0][0]+t*u[0],a[0][1]+t*u[1]],u,v];
}
export function chooseSketchAngleSector(refs:readonly SketchGeometryRef[],entities:readonly SketchEntity[],point:Vec2,reflex=false):SketchAngleSector|undefined{
 const lines=sketchAngleLines(refs,entities);if(!lines)return;
 const [origin,a,b]=lines,mouse=positive(Math.atan2(point[1]-origin[1],point[0]-origin[0]));
 const rays=[{angle:positive(Math.atan2(a[1],a[0])),first:true,reversed:false},{angle:positive(Math.atan2(a[1],a[0])+Math.PI),first:true,reversed:true},{angle:positive(Math.atan2(b[1],b[0])),first:false,reversed:false},{angle:positive(Math.atan2(b[1],b[0])+Math.PI),first:false,reversed:true}].sort((x,y)=>x.angle-y.angle);
 for(let i=0;i<4;i++){const left=rays[i],right=rays[(i+1)%4],span=positive(right.angle-left.angle);if(left.first===right.first||span<1e-12||positive(mouse-left.angle)>span)continue;
  return {firstReversed:left.first?left.reversed:right.reversed,secondReversed:left.first?right.reversed:left.reversed,clockwise:(!left.first)!==reflex};
 }
}
export function sketchAngleSweep(a:Vec2,b:Vec2,sector:SketchAngleSector):{start:number;sweep:number;degrees:number}{
 const start=Math.atan2(a[1],a[0])+(sector.firstReversed?Math.PI:0),end=Math.atan2(b[1],b[0])+(sector.secondReversed?Math.PI:0);
 const sweep=sector.clockwise?-positive(start-end):positive(end-start);return {start,sweep,degrees:Math.abs(sweep)*180/Math.PI};
}
