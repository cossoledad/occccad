import {useMemo,useState} from 'react';
import {Button,Select,Space} from 'antd';
import {Quaternion,Vector3} from 'three';
import type {SpatialReport} from '../../cad/assembly/spatial-analysis';
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
export function SectionResultView({report,onLocate}:{report:SpatialReport;onLocate:(id:string)=>void}){
 const [selected,setSelected]=useState(0),[zoom,setZoom]=useState(1),d=report.definition;
 const planes:Plane[]=d.sectionType==='PLANE'?[{label:'主平面',axis:2,offset:0}]:d.sectionType==='SLICE'?[-1,1].map(s=>({label:s<0?'反侧':'正侧',axis:2,offset:s*d.size[2]/2})):([0,1,2] as const).flatMap(axis=>[-1,1].map(s=>({label:`${['U','V','W'][axis]}${s>0?'+':'−'}`,axis,offset:s*d.size[axis]/2})));
 const index=Math.min(selected,planes.length-1),geometry=useMemo(()=>sectionViewGeometry(report,planes[index]),[report,index]),b=geometry.bounds;
 const width=b?b.width*1.15/zoom:1,height=b?b.height*1.15/zoom:1,box=b?`${b.x0+b.width/2-width/2} ${b.y0+b.height/2-height/2} ${width} ${height}`:'0 0 1 1';
 return <details className="section-result-view"><summary>截面视图</summary><Space wrap>{planes.length>1&&<Select aria-label="查看截面" value={index} options={planes.map((p,value)=>({value,label:p.label}))} onChange={v=>{setSelected(v);setZoom(1)}}/>}<Button size="small" onClick={()=>setZoom(v=>Math.min(16,v*1.5))}>放大</Button><Button size="small" onClick={()=>setZoom(v=>Math.max(.1,v/1.5))}>缩小</Button><Button size="small" onClick={()=>setZoom(1)}>适合</Button></Space>
  {!report.complete?<p>截面未完成</p>:!b?<p>此截面没有相交材料</p>:<svg role="img" aria-label="剖切二维视图" viewBox={box} width="100%" height="220" style={{background:'var(--color-surface)',overflow:'hidden'}}>{geometry.bodies.map(body=><g key={body.id} onClick={()=>onLocate(body.id)}><path d={body.fill} fill={palette.selected} fillOpacity={.16}/><path d={body.lines} fill="none" stroke={palette.selected} strokeWidth={1.4} vectorEffect="non-scaling-stroke"/></g>)}</svg>}
 </details>;
}
