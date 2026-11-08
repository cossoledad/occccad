import type {Feature,GeometricFeatureDefinition,GeometryKind,GeometricInput,DocumentView,SelectionItem} from '../../types';
import {randomUUID} from '../../utils/random-uuid';
export const geometryNames:Record<GeometryKind,string>={LINE:'直线',CIRCLE:'圆',ARC:'圆弧',SPLINE:'样条',POINT_ON_CURVE:'曲线上点',EXTRACT:'提取',BOUNDARY:'边界',COMPOSITE:'复合曲线',SURFACE_EXTRUDE:'拉伸曲面',THICKEN:'加厚',TRANSLATE:'平移',ROTATE:'旋转',MIRROR:'镜像',SPLIT:'分割',TRIM:'裁剪',EXTEND:'延伸',G1_CONNECT:'连接曲线',OFFSET:'偏移曲线',PROJECT:'投影',INTERSECTION:'交线'};
export const geometryCommands=(Object.keys(geometryNames) as GeometryKind[]).map(kind=>({kind,id:`geometry.${kind.toLowerCase().replaceAll('_','-')}`}));
export function defaultGeometricFeature(kind:GeometryKind,bodyId?:string):Feature{
 const geometry:GeometricFeatureDefinition={kind,inputs:[],parameters:{direction:[0,0,1],origin:[0,0,0],distance:kind==='THICKEN'?2:20,angle:45,start:kind==='TRIM'?.2:.5,end:.8,side:'END',tension:1}};
 if(['LINE','CIRCLE','ARC','SPLINE'].includes(kind))geometry.curve={points:(kind==='LINE'?[[0,0,0],[30,0,0]]:kind==='SPLINE'?[[0,0,0],[15,10,5],[30,0,10]]:[[0,0,0]]).map(position=>({id:randomUUID(),position:position as [number,number,number]})),normal:[0,0,1],xDirection:[1,0,0],radius:10,startAngle:0,endAngle:90};
 return {id:'',type:kind==='THICKEN'?'THICKEN':'GEOMETRY',...(kind==='THICKEN'?{bodyId,operation:'NEW_BODY' as const}:{}),geometry};
}
export function geometryInputRoles(kind:GeometryKind):Array<{label:string;role:'curve'|'surface'|'geometry'|'plane'}>{
 if(['LINE','CIRCLE','ARC','SPLINE'].includes(kind))return [];
 if(kind==='G1_CONNECT')return [{label:'起始曲线',role:'curve'},{label:'终止曲线',role:'curve'}];
 if(kind==='INTERSECTION')return [{label:'第一支撑',role:'surface'},{label:'第二支撑',role:'surface'}];
 if(kind==='PROJECT')return [{label:'曲线',role:'curve'},{label:'支撑',role:'surface'}];
 if(kind==='OFFSET')return [{label:'曲线',role:'curve'},{label:'平面',role:'plane'}];
 return [{label:['THICKEN','BOUNDARY'].includes(kind)?'曲面':kind==='EXTRACT'||['TRANSLATE','ROTATE','MIRROR'].includes(kind)?'对象':'曲线',role:['THICKEN','BOUNDARY'].includes(kind)?'surface':kind==='EXTRACT'||['TRANSLATE','ROTATE','MIRROR'].includes(kind)?'geometry':'curve'}];
}
export function geometryInputName(view:DocumentView,input?:GeometricInput){if(!input)return '选择对象';if(input.kind==='PLANE')return view.part?.datumPlanes.find(p=>p.id===input.planeId)?.name??'平面';const id=input.featureId??input.selection?.selection.sourceResult?.featureId??input.selection?.sourceFeatureId;return view.part?.features.find(f=>f.id===id)?.name??(input.kind==='SKETCH'?'草图':input.kind==='TOPOLOGY'?({FACE:'面',EDGE:'边',VERTEX:'顶点'}[input.selection?.selection.expectedType??'EDGE']):'几何结果')}
export type GeometryField={slot:string;label:string;unit:'mm'|'deg'|'';value:number;set:(value:number)=>void;group?:string};
export function geometryFields(d:GeometricFeatureDefinition):GeometryField[]{
 const fields:GeometryField[]=[];
 const add=(slot:string,label:string,unit:GeometryField['unit'],value:number,set:(n:number)=>void,group?:string)=>fields.push({slot:'geometry:'+slot,label,unit,value,set,group});
 const vector=(slot:string,label:string,v:[number,number,number],unit:GeometryField['unit'])=>['x','y','z'].forEach((axis,i)=>add(`${slot}:${axis}`,axis.toUpperCase(),unit,v[i],n=>v[i]=n,label));
 if(d.curve){const c=d.curve;c.points.forEach((p,i)=>{if(!p.pointId&&!p.reference)vector(`point:${p.id}`,d.kind==='LINE'?(i?'终点':'起点'):d.kind==='CIRCLE'||d.kind==='ARC'?'圆心':`点 ${i+1}`,p.position,'mm')});if(d.kind==='CIRCLE'||d.kind==='ARC'){add('radius','半径','mm',c.radius??10,n=>c.radius=n);if(!c.framePlaneId){vector('normal','法向',c.normal,'');vector('xDirection','起始方向',c.xDirection,'')}}if(d.kind==='ARC'){add('startAngle','起始角','deg',c.startAngle??0,n=>c.startAngle=n);add('endAngle','终止角','deg',c.endAngle??90,n=>c.endAngle=n)}}
 if(d.kind==='SPLINE'&&d.curve?.knots?.length&&d.curve.weights?.length===d.curve.points.length)d.curve.points.forEach((p,i)=>add(`weight:${p.id}`,`权重 ${i+1}`,'',d.curve!.weights![i],n=>d.curve!.weights![i]=n));
 if(['SURFACE_EXTRUDE','TRANSLATE','ROTATE','MIRROR','PROJECT'].includes(d.kind))vector('direction',d.kind==='MIRROR'?'镜像面法向':'方向',d.parameters.direction,'');
 if(['SURFACE_EXTRUDE','THICKEN','TRANSLATE','OFFSET','EXTEND'].includes(d.kind))add('distance',d.kind==='THICKEN'?'厚度':'距离','mm',d.parameters.distance,n=>d.parameters.distance=n);
 if(['ROTATE','MIRROR'].includes(d.kind))vector('origin','原点',d.parameters.origin,'mm');
 if(d.kind==='ROTATE')add('angle','角度','deg',d.parameters.angle,n=>d.parameters.angle=n);
 if(['POINT_ON_CURVE','SPLIT','TRIM'].includes(d.kind))add('start',d.kind==='TRIM'?'起始比例':'长度比例','',d.parameters.start,n=>d.parameters.start=n);
 if(d.kind==='TRIM')add('end','终止比例','',d.parameters.end,n=>d.parameters.end=n);
 if(d.kind==='G1_CONNECT')add('tension','切线比例','',d.parameters.tension,n=>d.parameters.tension=n);
 return fields;
}
export function sketchGeometryInput(view:DocumentView,s:SelectionItem):GeometricInput|undefined{
 const id=s.kind==='sketch'?s.entityId??s.id:s.kind==='visual'?s.featureId:undefined;
 const f=view.part?.features.find(f=>f.id===id&&f.sketch);if(!f?.sketch)return;
 const selected=s.sketchReference?.target==='ENTITY'?s.sketchReference.entityId:undefined;
 const entityIds=selected?[selected]:f.sketch.entities.filter(e=>e.role!=='CONSTRUCTION'&&e.kind!=='POINT'&&!e.suppressed).map(e=>e.id);
 return entityIds.length?{kind:'SKETCH',featureId:f.id,entityIds}:undefined;
}

export function splineControlMode(d:GeometricFeatureDefinition,control:boolean,degree=d.curve?.degree??3){
 const c=d.curve!;if(!control){delete c.knots;delete c.multiplicities;delete c.weights;delete c.degree;return}
 c.closed=false;c.degree=Math.max(1,Math.min(25,degree,c.points.length-1));const spans=c.points.length-c.degree;c.knots=Array.from({length:spans+1},(_,i)=>i/spans);c.multiplicities=c.knots.map((_,i)=>i===0||i===spans?c.degree!+1:1);c.weights=c.points.map((_,i)=>c.weights?.[i]??1);
}
