import {useEffect,useMemo,useRef,useState} from 'react';
import {Button,Select,Space,Switch} from 'antd';
import {Quaternion,Vector3} from 'three';
import type {SpatialAnalysis,SpatialReport} from '../../cad/assembly/spatial-analysis';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {palette} from '../../design/visual-tokens';

type Plane={label:string;axis:number;offset:number};
export function sectionViewGeometry(report:SpatialReport,plane:Plane){
 const d=report.definition,inverse=new Quaternion().fromArray(d.frame.rotation).invert(),origin=new Vector3().fromArray(d.frame.translation),axes=[0,1,2].filter(a=>a!==plane.axis),tolerance=1e-7*Math.max(1,...d.size);
 const local=(p:[number,number,number])=>new Vector3().fromArray(p).sub(origin).applyQuaternion(inverse).toArray();
 const project=(p:number[])=>[p[axes[0]],-p[axes[1]]];let x0=Infinity,y0=Infinity,x1=-Infinity,y1=-Infinity;
 const include=(p:number[])=>{const [x,y]=project(p);x0=Math.min(x0,x);x1=Math.max(x1,x);y0=Math.min(y0,y);y1=Math.max(y1,y);return `${x},${y}`};
 const bodies=(report.geometry?.bodies??[]).filter(b=>b.complete).map(b=>{let lines='',fill='';
  for(let i=0;i<(b.segments?.length??0);i+=2){const pts=b.segments!.slice(i,i+2).map(local);if(pts.every(p=>Math.abs(p[plane.axis]-plane.offset)<=tolerance))lines+=`M${include(pts[0])}L${include(pts[1])}`}
  for(let i=0;i<(b.triangles?.length??0);i+=3){const pts=b.triangles!.slice(i,i+3).map(local);if(pts.every(p=>Math.abs(p[plane.axis]-plane.offset)<=tolerance))fill+=`M${include(pts[0])}L${include(pts[1])}L${include(pts[2])}Z`}
  return {id:b.id,lines,fill};
 });return {bodies,bounds:Number.isFinite(x0)?{x0,y0,width:Math.max(1,x1-x0),height:Math.max(1,y1-y0)}:undefined};
}
type ViewBounds=ReturnType<typeof sectionViewGeometry>['bounds'];
type SectionView={zoom:number;offset:[number,number]};
type Pan={pointerId:number;x:number;y:number;inverse:DOMMatrix;view:SectionView};
const fittedView=():SectionView=>({zoom:1,offset:[0,0]});

export function SectionResultView({definition,report,onLocate}:{definition:SpatialAnalysis;report?:SpatialReport;onLocate:(id:string)=>void}){
 const [open,setOpen]=useState(false),[selected,setSelected]=useState(0),[view,setView]=useState<SectionView>(fittedView),[panning,setPanning]=useState(false);
 const surface=useRef<HTMLDivElement>(null),svg=useRef<SVGSVGElement>(null),fit=useRef<ViewBounds>(undefined),pan=useRef<Pan>(undefined);
 const d=report?.definition??definition;
 const finishPan=(commit:boolean)=>{
  const drag=pan.current;if(!drag)return;pan.current=undefined;setPanning(false);
  if(!commit)setView(drag.view);
  if(svg.current?.hasPointerCapture(drag.pointerId))svg.current.releasePointerCapture(drag.pointerId);
 };
 useEffect(()=>{
  if(!open)return;const element=surface.current;if(!element)return;
  const wheel=(event:WheelEvent)=>{event.preventDefault();if(pan.current)return;const delta=event.deltaY*(event.deltaMode===1?16:event.deltaMode===2?element.clientHeight:1);setView(v=>({...v,zoom:Math.max(.1,Math.min(32,v.zoom*Math.exp(-Math.max(-500,Math.min(500,delta))*.002)))}))};
  const blur=()=>finishPan(false);
  element.addEventListener('wheel',wheel,{passive:false});window.addEventListener('blur',blur);
  return()=>{element.removeEventListener('wheel',wheel);window.removeEventListener('blur',blur);finishPan(false)};
 },[open]);
 useEffect(()=>{setSelected(0);fit.current=undefined;setView(fittedView())},[definition.sectionType]);
 const planes:Plane[]=d.sectionType==='PLANE'?[{label:'主平面',axis:2,offset:0}]:d.sectionType==='SLICE'?[-1,1].map(s=>({label:s<0?'反侧':'正侧',axis:2,offset:s*d.size[2]/2})):([0,1,2] as const).flatMap(axis=>[-1,1].map(s=>({label:`${['U','V','W'][axis]}${s>0?'+':'−'}`,axis,offset:s*d.size[axis]/2})));
 const index=Math.min(selected,planes.length-1),geometry=useMemo(()=>report?.complete?sectionViewGeometry(report,planes[index]):{bodies:[],bounds:undefined},[report,index]);
 const fitView=()=>{finishPan(true);if(geometry.bounds)fit.current=geometry.bounds;setView(fittedView())};
 // Fit each completed sample, rather than keeping the first plane's envelope.
 // Empty/pending samples keep the last camera while the fixed canvas stays blank.
 useEffect(()=>{if(geometry.bounds)fitView()},[geometry]);
 const b=fit.current,width=b?b.width*1.15/view.zoom:1,height=b?b.height*1.15/view.zoom:1;
 const box=b?`${b.x0+b.width/2-width/2+view.offset[0]} ${b.y0+b.height/2-height/2+view.offset[1]} ${width} ${height}`:'0 0 1 1';
 return <><Space><span>截面视图</span><Switch aria-label="截面视图" checked={open} onChange={setOpen}/></Space>
 <CommandDialog id="section-result" title="截面视图" open={open} size="L" footer={false} bodyClassName="section-result-body" scrollBody={false} onConfirm={()=>setOpen(false)} onClose={()=>setOpen(false)} onCancelPick={()=>{if(!pan.current)return false;finishPan(false);return true}}>
  <Space wrap>{planes.length>1&&<Select aria-label="查看截面" value={index} options={planes.map((p,value)=>({value,label:p.label}))} onChange={v=>{finishPan(true);setSelected(v)}}/>}
   <Button size="small" onClick={()=>setView(v=>({...v,zoom:Math.min(32,v.zoom*1.5)}))}>放大</Button><Button size="small" onClick={()=>setView(v=>({...v,zoom:Math.max(.1,v.zoom/1.5)}))}>缩小</Button><Button size="small" onClick={fitView}>适合</Button>
  </Space>
  <div ref={surface} className="section-result-canvas"><svg ref={svg} role="img" aria-label="剖切二维视图" viewBox={box} width="100%" height="100%" data-panning={panning||undefined}
   onPointerDown={e=>{if(e.button!==1||pan.current)return;const matrix=e.currentTarget.getScreenCTM();if(!matrix)return;e.preventDefault();e.stopPropagation();pan.current={pointerId:e.pointerId,x:e.clientX,y:e.clientY,inverse:matrix.inverse(),view};e.currentTarget.setPointerCapture(e.pointerId);setPanning(true)}}
   onPointerMove={e=>{const drag=pan.current;if(!drag||drag.pointerId!==e.pointerId)return;e.preventDefault();const dx=e.clientX-drag.x,dy=e.clientY-drag.y,m=drag.inverse;setView({...drag.view,offset:[drag.view.offset[0]-m.a*dx-m.c*dy,drag.view.offset[1]-m.b*dx-m.d*dy]})}}
   onPointerUp={e=>{if(pan.current?.pointerId===e.pointerId&&e.button===1){e.preventDefault();finishPan(true)}}}
   onPointerCancel={()=>finishPan(false)} onLostPointerCapture={()=>finishPan(false)} onAuxClick={e=>e.preventDefault()}
   style={{background:'var(--color-surface)',overflow:'hidden'}}>
   {geometry.bodies.map(body=><g key={body.id} onClick={()=>onLocate(body.id)}><path d={body.fill} fill={palette.selected} fillOpacity={.16}/><path d={body.lines} fill="none" stroke={palette.selected} strokeWidth={1.4} vectorEffect="non-scaling-stroke"/></g>)}
  </svg></div>
 </CommandDialog></>;
}
