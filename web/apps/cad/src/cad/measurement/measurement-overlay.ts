import {Camera,Vector3} from 'three';
import type {Vec3} from '../../types';
import {measurementSummary,measurementValue,type Measurement,type MeasurementUnit} from './measurement';
import type {AnnotationLayouts} from './annotation-layout';

type Point=[number,number];
const NS='http://www.w3.org/2000/svg';
function svg<K extends keyof SVGElementTagNameMap>(tag:K,attributes:Record<string,string|number>={}){const n=document.createElementNS(NS,tag);for(const [k,v] of Object.entries(attributes))n.setAttribute(k,String(v));return n;}
export type MeasurementDrawing={anchor:Vec3;segments:Array<[Vec3,Vec3]>;points:Vec3[];arc?:Vec3[];dimension?:[Vec3,Vec3];text:string;components?:Array<{axis:number;first:Vec3;second:Vec3;value:number}>};
/** Pure geometry presentation. All authority values/anchors come from Worker;
 * arrow sizes, offsets and angle arc radius are display-only choices. */
export function measurementDrawing(m:Measurement,unit:MeasurementUnit):MeasurementDrawing|undefined{
 const r=m.result;if(!r?.complete)return;const a=m.displayAnchors??r.anchors;
 const point=a.vertex??a.centroid??a.position??a.center??a.first??a.start;
 if(!point)return;
 const out:MeasurementDrawing={anchor:point,segments:[],points:[],text:measurementSummary(m,unit)};
 if(m.kind==='BETWEEN'&&a.first&&a.second){
  out.dimension=[a.first,a.second];out.anchor=a.first.map((v,i)=>(v+a.second[i])/2) as Vec3;out.points=[a.first,a.second];
  if(r.values.angleRad!==undefined)out.text+=` · ${measurementValue('angleRad',r.values.angleRad,unit)}`;
  if(m.showComponents&&!['deltaXMm','deltaYMm','deltaZMm'].some(key=>m.outputCurrency?.[key]&&m.outputCurrency[key]!=='CURRENT')){let p=[...a.first] as Vec3;out.components=[];for(let i=0;i<3;i++){const next=[...p] as Vec3;next[i]=a.second[i];out.components.push({axis:i,first:p,second:next,value:next[i]-p[i]});p=next;}}
 }else if(m.kind==='ANGLE_POINTS'&&a.first&&a.vertex&&a.second){
  const center=new Vector3().fromArray(a.vertex),u=new Vector3().fromArray(a.first).sub(center),v=new Vector3().fromArray(a.second).sub(center);
  const radius=Math.min(u.length(),v.length())*.28,angle=u.angleTo(v);u.normalize();v.normalize();
  const normal=u.clone().cross(v);if(normal.lengthSq()<1e-16){normal.copy(u).cross(Math.abs(u.z)<.9?new Vector3(0,0,1):new Vector3(0,1,0))}normal.normalize();
  out.arc=Array.from({length:33},(_,i)=>u.clone().applyAxisAngle(normal,angle*i/32).multiplyScalar(radius).add(center).toArray() as Vec3);
  out.anchor=out.arc[16];out.segments=[[a.first,a.vertex],[a.vertex,a.second]];out.points=[a.first,a.vertex,a.second];
 }else{
  if(a.start&&a.end){out.segments.push([a.start,a.end]);out.points.push(a.start,a.end)}
  if(a.center&&a.start&&r.values.radiusMm!==undefined){out.segments.push([a.center,a.start]);out.points.push(a.center);out.text+=` · R ${measurementValue('radiusMm',r.values.radiusMm,unit)}`}
  if(a.bboxMin&&a.bboxMax&&!['sizeXMm','sizeYMm','sizeZMm'].some(key=>m.outputCurrency?.[key]&&m.outputCurrency[key]!=='CURRENT')){
   const lo=a.bboxMin,hi=a.bboxMax,corners=Array.from({length:8},(_,i)=>[i&1?hi[0]:lo[0],i&2?hi[1]:lo[1],i&4?hi[2]:lo[2]] as Vec3);
   for(let i=0;i<8;i++)for(const bit of [1,2,4])if(!(i&bit))out.segments.push([corners[i],corners[i|bit]]);
  }
  out.points.push(point);
 }
 return out;
}

/** Viewport-owned screen overlay, with no model picking or scene ownership.
 * Labels keep CSS-pixel size, while every witness remains world-anchored. */
export class MeasurementOverlay {
 private readonly root=svg('svg',{'class':'measurement-overlay','aria-label':'几何测量标注'});
 private offsets=new Map<string,Point>();
 private documentId='';
 private layoutDocumentId='';
 private layouts:AnnotationLayouts={};
 private sourceKey='';
 private frameKey='';
 private dragging?:{cancel:()=>void};
 private saveOffset?: (id:string,offset:Point|undefined)=>void;
 private selected=new Set<string>();
 private hovered?:string;
 setInteraction(selected:readonly string[],hovered?:string){this.selected=new Set(selected);this.hovered=hovered;this.applyInteraction()}
 private applyInteraction(){for(const group of this.root.querySelectorAll<SVGGElement>('.measurement-retained')){const id=group.dataset.measurementId!;group.classList.toggle('measurement-selected',this.selected.has(id));group.classList.toggle('measurement-hover',id===this.hovered);group.querySelector('.measurement-label')?.setAttribute('aria-pressed',String(this.selected.has(id)))}}
 setLayouts(documentId:string,layouts:AnnotationLayouts,save?:(id:string,offset:Point|undefined)=>void){
  this.layoutDocumentId=documentId;this.layouts=layouts;
  this.saveOffset=save;
  if(documentId===this.documentId)this.hydrateLayouts();
 }
 private hydrateLayouts(){for(const [id,layout] of Object.entries(this.layouts)){if(layout.offset)this.offsets.set(id,layout.offset);else this.offsets.delete(id)}}
 constructor(host:HTMLElement,private interaction?:{hover:(id?:string)=>void;select:(id:string,toggle:boolean)=>void}){
  host.append(this.root);
  const id=(target:EventTarget|null)=>target instanceof Element?target.closest('.measurement-retained')?.getAttribute('data-measurement-id')??undefined:undefined;
  this.root.addEventListener('pointermove',e=>{if(!this.dragging)this.interaction?.hover(id(e.target))});
  this.root.addEventListener('pointerleave',()=>{if(!this.dragging)this.interaction?.hover()});
  this.root.addEventListener('pointerdown',e=>{const value=id(e.target);if(value&&e.button===0){e.preventDefault();e.stopPropagation();this.interaction?.select(value,e.ctrlKey||e.metaKey)}});
 }
 dispose(){this.dragging?.cancel();this.root.remove();this.offsets.clear()}
 render(documentId:string,items:Measurement[],unit:MeasurementUnit,camera:Camera,width:number,height:number,enabled=true){
  const sourceKey=JSON.stringify(items.map(m=>[m.id,m.result?.sourceStamp,m.status,m.visible,m.showComponents]));
  if(sourceKey!==this.sourceKey){this.dragging?.cancel();this.sourceKey=sourceKey;const live=new Set(items.map(m=>m.id||'preview'));for(const key of this.offsets.keys())if(!live.has(key))this.offsets.delete(key)}
  if(documentId!==this.documentId||!enabled){this.dragging?.cancel();if(documentId!==this.documentId)this.offsets.clear();this.documentId=documentId}
  if(this.dragging)return;
  if(documentId===this.layoutDocumentId)this.hydrateLayouts();
  const frameKey=JSON.stringify([documentId,sourceKey,unit,width,height,enabled,camera.projectionMatrix.elements,camera.matrixWorldInverse.elements,items.map(m=>[m.name,m.displayAnchors,m.result?.anchors,m.outputCurrency]),[...this.offsets]]);
  if(frameKey===this.frameKey)return;this.frameKey=frameKey;
  this.root.replaceChildren();this.root.setAttribute('viewBox',`0 0 ${width} ${height}`);
  if(!enabled)return;
  const occupied:Array<{x:number;y:number;w:number;h:number}>=[];
  const project=(point:Vec3):Point|undefined=>{const p=new Vector3().fromArray(point).project(camera);return Number.isFinite(p.x)&&Number.isFinite(p.y)&&p.z>=-1&&p.z<=1?[((p.x+1)/2)*width,((1-p.y)/2)*height]:undefined};
  const line=(parent:SVGElement,a:Point,b:Point,cls:string)=>{const n=svg('line',{x1:a[0],y1:a[1],x2:b[0],y2:b[1],class:cls});parent.append(n);return n};
  const arrow=(parent:SVGElement,p:Point,d:Point)=>{const l=Math.hypot(...d);if(l<.1)return;const x=d[0]/l,y=d[1]/l;parent.append(svg('polygon',{points:`${p[0]},${p[1]} ${p[0]+x*8-y*3},${p[1]+y*8+x*3} ${p[0]+x*8+y*3},${p[1]+y*8-x*3}`,class:'measurement-arrow'}))};
  for(const [index,m] of items.entries()){
   if(!m.visible)continue;const drawing=measurementDrawing(m,unit);if(!drawing)continue;
   const stale=m.status!=='CURRENT';if(stale){drawing.text=`上次值 · ${drawing.text} · ${m.status==='BROKEN'?'来源断裂':'待更新'}`;drawing.components=undefined}
   const anchor=project(drawing.anchor);if(!anchor)continue;
   const group=svg('g',{'data-measurement-id':m.id||'preview',class:`${m.id?'measurement-retained':'measurement-preview'}${stale?' measurement-stale':''}${m.status==='BROKEN'?' measurement-broken':''}`});this.root.append(group);
   for(const [a,b] of drawing.segments){const x=project(a),y=project(b);if(x&&y)line(group,x,y,'measurement-support-line')}
   let labelAnchor=anchor;
   if(drawing.dimension){
    const a=project(drawing.dimension[0]),b=project(drawing.dimension[1]);
    if(a&&b){const len=Math.hypot(b[0]-a[0],b[1]-a[1]),normal:Point=len>1?[18*(b[1]-a[1])/len,-18*(b[0]-a[0])/len]:[18,-18];
     const x:Point=[a[0]+normal[0],a[1]+normal[1]],y:Point=[b[0]+normal[0],b[1]+normal[1]];
     line(group,a,x,'measurement-extension');line(group,b,y,'measurement-extension');line(group,x,y,'measurement-dimension');
     if(len>20){arrow(group,x,[y[0]-x[0],y[1]-x[1]]);arrow(group,y,[x[0]-y[0],x[1]-y[1]])}
     labelAnchor=[(x[0]+y[0])/2,(x[1]+y[1])/2];
    }
   }
   if(drawing.arc){const pts=drawing.arc.map(project);if(pts.every((p):p is Point=>!!p))group.append(svg('polyline',{points:pts.map(p=>p.join(',')).join(' '),class:'measurement-dimension',fill:'none'}))}
   for(const [i,p] of drawing.points.entries()){const xy=project(p);if(xy){group.append(svg('circle',{cx:xy[0],cy:xy[1],r:3.5,class:'measurement-witness'}));if(m.kind==='BETWEEN'){const t=svg('text',{x:xy[0]+7,y:xy[1]-7,class:'measurement-role'});t.textContent=String(i+1);group.append(t)}}}
   for(const c of drawing.components??[]){const a=project(c.first),b=project(c.second);if(a&&b){line(group,a,b,`measurement-component measurement-component-${c.axis}`);if(Math.hypot(b[0]-a[0],b[1]-a[1])>30){const t=svg('text',{x:(a[0]+b[0])/2+4,y:(a[1]+b[1])/2-5,class:`measurement-component-text measurement-component-${c.axis}`});t.textContent=`${'XYZ'[c.axis]} ${measurementValue('xMm',c.value,unit)}`;group.append(t)}}}
   const text=(m.name?m.name+' · ':'')+drawing.text,w=Math.min(width-24,Math.max(120,text.length*7.5+24)),h=30,key=m.id||'preview';
   let offset=this.offsets.get(key)??[26,-38-index%3*12];
   let x=Math.max(8,Math.min(width-w-8,labelAnchor[0]+offset[0])),y=Math.max(8,Math.min(height-h-8,labelAnchor[1]+offset[1]));
   if(!this.offsets.has(key))for(let attempt=0;attempt<12&&occupied.some(r=>x<r.x+r.w+4&&x+w+4>r.x&&y<r.y+r.h+4&&y+h+4>r.y);attempt++){y+=h+6;if(y+h>height-8){y=8;x=Math.min(width-w-8,x+w/2)}}
   occupied.push({x,y,w,h});offset=[x-labelAnchor[0],y-labelAnchor[1]];
   const leader=line(group,labelAnchor,[x,y+h/2],'measurement-leader');
   const label=svg('g',{transform:`translate(${x} ${y})`,class:'measurement-label',tabindex:0,role:m.id?'button':'img','aria-label':text});
   label.append(svg('rect',{width:w,height:h,rx:5}));const title=svg('title');title.textContent=`${text}；拖动可调整标签位置，双击复位`;label.append(title);
   const t=svg('text',{x:10,y:19});t.textContent=text.length*7.5+24>w?text.slice(0,Math.max(8,Math.floor((w-40)/7.5)))+'…':text;label.append(t);group.append(label);
   label.addEventListener('dblclick',e=>{e.stopPropagation();this.offsets.delete(key);if(m.id)this.saveOffset?.(m.id,undefined);x=Math.max(8,Math.min(width-w-8,labelAnchor[0]+26));y=Math.max(8,Math.min(height-h-8,labelAnchor[1]-38));label.setAttribute('transform',`translate(${x} ${y})`);leader.setAttribute('x2',String(x));leader.setAttribute('y2',String(y+h/2))});
   label.addEventListener('pointerdown',e=>{
    if(e.button!==0)return;e.preventDefault();e.stopPropagation();label.setPointerCapture(e.pointerId);
    const start:Point=[e.clientX,e.clientY],base:Point=[x,y],previous=this.offsets.get(key);
    let moved=false;
    const move=(event:PointerEvent)=>{event.stopPropagation();moved ||= Math.hypot(event.clientX-start[0],event.clientY-start[1])>3;const box=this.root.getBoundingClientRect(),sx=width/box.width,sy=height/box.height;x=Math.max(8,Math.min(width-w-8,base[0]+(event.clientX-start[0])*sx));y=Math.max(8,Math.min(height-h-8,base[1]+(event.clientY-start[1])*sy));label.setAttribute('transform',`translate(${x} ${y})`);leader.setAttribute('x2',String(x));leader.setAttribute('y2',String(y+h/2));this.offsets.set(key,[x-labelAnchor[0],y-labelAnchor[1]])};
    const finish=(event?:Event,cancel=false)=>{event?.stopPropagation();if(cancel){if(previous)this.offsets.set(key,previous);else this.offsets.delete(key);label.setAttribute('transform',`translate(${base[0]} ${base[1]})`);leader.setAttribute('x2',String(base[0]));leader.setAttribute('y2',String(base[1]+h/2))}if(!cancel&&m.id&&moved)this.saveOffset?.(m.id,this.offsets.get(key));this.dragging=undefined;label.removeEventListener('pointermove',move);label.removeEventListener('pointerup',up);label.removeEventListener('pointercancel',cancelled);label.removeEventListener('lostpointercapture',cancelled);window.removeEventListener('blur',cancelled);window.removeEventListener('keydown',keyDown,true);if(label.hasPointerCapture(e.pointerId))label.releasePointerCapture(e.pointerId)};
    const up=(event:PointerEvent)=>finish(event),cancelled=(event:Event)=>finish(event,true),keyDown=(event:KeyboardEvent)=>{if(event.key==='Escape'){event.preventDefault();event.stopImmediatePropagation();finish(event,true)}};
    this.dragging={cancel:()=>finish(undefined,true)};label.addEventListener('pointermove',move);label.addEventListener('pointerup',up);label.addEventListener('pointercancel',cancelled);label.addEventListener('lostpointercapture',cancelled);window.addEventListener('blur',cancelled);window.addEventListener('keydown',keyDown,true);
    if(m.id)this.interaction?.select(m.id,e.ctrlKey||e.metaKey);
   });
   label.addEventListener('keydown',e=>{if(m.id&&(e.key==='Enter'||e.key===' ')){e.preventDefault();e.stopPropagation();this.interaction?.select(m.id,e.ctrlKey||e.metaKey)}});
  }
  this.applyInteraction();
 }
}
