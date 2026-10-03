import type { Vec2 } from "../../types";
export type SketchScreenPoint = { x:number; y:number };
/** Left-to-right encloses, right-to-left crosses. Display samples only decide
 * explicit selection; they never create topology or mutate authoritative curves. */
export function sketchMarqueeContains(points:readonly Vec2[],from:SketchScreenPoint,to:SketchScreenPoint):boolean {
 const minX=Math.min(from.x,to.x),maxX=Math.max(from.x,to.x),minY=Math.min(from.y,to.y),maxY=Math.max(from.y,to.y);
 const inside=([x,y]:Vec2)=>x>=minX&&x<=maxX&&y>=minY&&y<=maxY;
 if(!points.length||points.some(p=>!p.every(Number.isFinite)))return false;
 if(to.x>=from.x)return points.every(inside);
 if(points.some(inside))return true;
 // Liang-Barsky segment/rectangle clipping, including a segment whose two
 // endpoints lie outside opposite sides of the rectangle.
 for(let i=1;i<points.length;i++){
  const [x,y]=points[i-1],dx=points[i][0]-x,dy=points[i][1]-y;
  let enter=0,exit=1,valid=true;
  for(const [p,q] of [[-dx,x-minX],[dx,maxX-x],[-dy,y-minY],[dy,maxY-y]]){
   if(p===0){if(q<0){valid=false;break;}continue;}
   const t=q/p;if(p<0)enter=Math.max(enter,t);else exit=Math.min(exit,t);
   if(enter>exit){valid=false;break;}
  }
  if(valid)return true;
 }
 return false;
}
