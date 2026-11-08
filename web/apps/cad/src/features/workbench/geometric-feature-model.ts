import type {Feature,GeometricFeatureDefinition,GeometryKind,GeometricInput,GeometryControlPoint,DocumentView,SelectionItem} from '../../types';
import {randomUUID} from '../../utils/random-uuid';
import {referenceInputName} from './reference-element-model';
import {CommandInputError} from '../../cad/command/operation-failure';
export const geometryNames:Record<GeometryKind,string>={IMPORTED:"导入几何",LINE:'直线',CIRCLE:'圆',ARC:'圆弧',SPLINE:'样条',POINT_ON_CURVE:'曲线上点',EXTRACT:'提取',BOUNDARY:'边界',COMPOSITE:'复合曲线',SURFACE_EXTRUDE:'拉伸曲面',THICKEN:'加厚',TRANSLATE:'平移',ROTATE:'旋转',MIRROR:'镜像',SPLIT:'分割',TRIM:'裁剪',EXTEND:'延伸',G1_CONNECT:'连接曲线',OFFSET:'偏移曲线',PROJECT:'投影',INTERSECTION:'交线',SURFACE_SWEEP:'曲面扫掠',SURFACE_LOFT:'曲面放样',SURFACE_FILL:'填充曲面',SURFACE_TRIM:'裁剪曲面',SURFACE_EXTEND:'延长曲面',SURFACE_JOIN:'连接曲面',SURFACE_SEW:'缝合曲面',SURFACE_OFFSET:'偏移曲面',CLOSE_SURFACE:'封闭曲面'};
export const geometryCommands=(Object.keys(geometryNames) as GeometryKind[]).filter(kind=>kind!=="IMPORTED").map(kind=>({kind,id:`geometry.${kind.toLowerCase().replaceAll('_','-')}`}));
export const geometryBodyKind=(kind:GeometryKind)=>kind==='THICKEN'||kind==='CLOSE_SURFACE';
export const geometryMultipleKind=(kind:GeometryKind)=>['COMPOSITE','SURFACE_LOFT','SURFACE_FILL','SURFACE_JOIN','SURFACE_SEW'].includes(kind);
export function defaultGeometricFeature(kind:GeometryKind):Feature{
 const geometry:GeometricFeatureDefinition={kind,inputs:[],parameters:{direction:[0,0,0],origin:[0,0,0],distance:['THICKEN','SURFACE_OFFSET'].includes(kind)?2:20,angle:45,start:kind==='TRIM'?.2:.5,end:.8,side:'END',tension:1}};
 if(['LINE','CIRCLE','ARC','SPLINE'].includes(kind))geometry.curve={points:Array.from({length:kind==='LINE'?2:kind==='SPLINE'?0:1},()=>({id:randomUUID(),position:[0,0,0]})),normal:[0,0,0],xDirection:[0,0,0],radius:10,startAngle:0,endAngle:90};
 if(kind.startsWith('SURFACE_')&&kind!=='SURFACE_EXTRUDE'||kind==='CLOSE_SURFACE')geometry.surface={ruled:false,frameMode:kind==='SURFACE_SWEEP'?'CORRECTED_FRENET':undefined,tolerance:1e-6};
 return {id:'',type:geometryBodyKind(kind)?kind as 'THICKEN'|'CLOSE_SURFACE':'GEOMETRY',...(geometryBodyKind(kind)?{operation:'NEW_BODY' as const}:{}),geometry};
}
export function geometryInputRoles(kind:GeometryKind):Array<{label:string;role:'curve'|'surface'|'geometry'|'plane'}>{
 if(['LINE','CIRCLE','ARC','SPLINE'].includes(kind))return [];
 if(kind==='SURFACE_SWEEP')return [{label:'轮廓',role:'curve'},{label:'路径',role:'curve'}];
 if(kind==='SURFACE_LOFT')return [{label:'截面',role:'curve'}];
 if(kind==='SURFACE_FILL')return [{label:'边界',role:'curve'}];
 if(['SURFACE_JOIN','SURFACE_SEW','SURFACE_OFFSET','CLOSE_SURFACE'].includes(kind))return [{label:'曲面',role:'surface'}];
 if(kind==='SURFACE_TRIM')return [{label:'曲面',role:'surface'},{label:'切割对象',role:'surface'}];
 if(kind==='SURFACE_EXTEND')return [{label:'曲面',role:'surface'},{label:'边界',role:'curve'}];
 if(kind==='G1_CONNECT')return [{label:'起始曲线',role:'curve'},{label:'终止曲线',role:'curve'}];
 if(kind==='INTERSECTION')return [{label:'第一支撑',role:'surface'},{label:'第二支撑',role:'surface'}];
 if(kind==='PROJECT')return [{label:'投影曲线',role:'curve'},{label:'支撑面',role:'surface'}];
 if(kind==='OFFSET')return [{label:'曲线',role:'curve'},{label:'支撑平面',role:'plane'}];
 return [{label:['THICKEN','BOUNDARY'].includes(kind)?'曲面':kind==='EXTRACT'||['TRANSLATE','ROTATE','MIRROR'].includes(kind)?'对象':kind==='SURFACE_EXTRUDE'?'轮廓':'曲线',role:['THICKEN','BOUNDARY'].includes(kind)?'surface':kind==='EXTRACT'||['TRANSLATE','ROTATE','MIRROR'].includes(kind)?'geometry':'curve'}];
}
export type GeometryReferenceRole='direction'|'axis'|'plane'|'point';
export function geometryInputName(view:DocumentView,input?:GeometricInput):string{
 if(!input)return '未选择';
 if(input.kind==='DATUM')return referenceInputName(view,input.datum);
 if(input.kind==='POINT'){
  const sketch=view.part?.features.find(f=>f.id===input.point?.sketchId),ref=input.point?.reference;
  const entity=sketch?.sketch?.entities.find(e=>e.id===ref?.entityId);
  const label=ref?.target==='SKETCH_ORIGIN'?'原点':ref?.subElement==='START'?'起点':ref?.subElement==='END'?'终点':ref?.subElement==='CENTER'?'圆心':'点';
  return `${sketch?.name??'草图'} · ${entity?.name??label}`;
 }
 if(input.kind==='PLANE')return view.part?.datumPlanes.find(p=>p.id===input.planeId)?.name??'平面';
 const id=input.featureId??input.selection?.selection.sourceResult?.featureId??input.selection?.sourceFeatureId;
 return view.part?.features.find(f=>f.id===id)?.name??'来源已缺失';
}
export function geometryInputIdentity(input:GeometricInput):string{
 if(input.selection){const s=input.selection.selection;return JSON.stringify([s.sourceDocumentId,s.sourceBodyId,s.sourceResult,s.anchor,s.expectedType])}
 return JSON.stringify([input.kind,input.featureId,input.resultId,input.planeId,input.entityIds,input.datum,input.point]);
}
export function pointInput(point:GeometryControlPoint):GeometricInput|undefined{return point.reference??(point.pointId?{kind:'DATUM',datum:{kind:'POINT',entityId:point.pointId}}:undefined)}
export function geometryReady(d:GeometricFeatureDefinition):boolean{
 const roles=geometryInputRoles(d.kind),inputs=d.inputs??[];
 if(geometryMultipleKind(d.kind)?inputs.length<(d.kind==='SURFACE_LOFT'?2:1):roles.some((_,i)=>!inputs[i]))return false;
 if(d.kind==='SURFACE_TRIM'&&!d.keepPoint)return false;
 if(d.curve){if(d.curve.points.length<(d.kind==='LINE'||d.kind==='SPLINE'?2:1)||d.curve.points.some(p=>!pointInput(p)))return false;if(['CIRCLE','ARC'].includes(d.kind)&&!d.curve.support)return false}
 if(['SURFACE_EXTRUDE','TRANSLATE','PROJECT'].includes(d.kind)&&!d.direction)return false;
 if(d.kind==='ROTATE'&&!d.axis||d.kind==='MIRROR'&&!d.mirrorPlane)return false;
 return true;
}
export type GeometryField={slot:string;label:string;unit:'mm'|'deg'|'';value:number;set:(n:number)=>void};
export function geometryFields(d:GeometricFeatureDefinition):GeometryField[]{
 const fields:GeometryField[]=[];
 const add=(slot:string,label:string,unit:GeometryField['unit'],value:number,set:(n:number)=>void)=>fields.push({slot:'geometry:'+slot,label,unit,value,set});
 if(d.curve){const c=d.curve;if(['CIRCLE','ARC'].includes(d.kind))add('radius','半径','mm',c.radius??10,n=>c.radius=n);if(d.kind==='ARC'){add('startAngle','起始角','deg',c.startAngle??0,n=>c.startAngle=n);add('endAngle','终止角','deg',c.endAngle??90,n=>c.endAngle=n)}}
 if(d.kind==='SPLINE'&&d.curve?.knots?.length&&d.curve.weights?.length===d.curve.points.length)d.curve.points.forEach((p,i)=>add(`weight:${p.id}`,`权重 ${i+1}`,'',d.curve!.weights![i],n=>d.curve!.weights![i]=n));
 if(['SURFACE_EXTRUDE','THICKEN','TRANSLATE','OFFSET','EXTEND','SURFACE_EXTEND','SURFACE_OFFSET'].includes(d.kind))add('distance',d.kind==='THICKEN'?'厚度':['SURFACE_EXTRUDE','SURFACE_EXTEND'].includes(d.kind)?'长度':'距离','mm',d.parameters.distance,n=>d.parameters.distance=n);
 if(d.kind==='ROTATE')add('angle','角度','deg',d.parameters.angle,n=>d.parameters.angle=n);
 if(['POINT_ON_CURVE','SPLIT','TRIM'].includes(d.kind))add('start',d.kind==='TRIM'?'起始比例':'弧长比例','',d.parameters.start,n=>d.parameters.start=n);
 if(d.kind==='TRIM')add('end','终止比例','',d.parameters.end,n=>d.parameters.end=n);
 if(d.surface)add('tolerance','容差','mm',d.surface.tolerance,n=>d.surface!.tolerance=n);
 if(d.kind==='G1_CONNECT')add('tension','切线系数','',d.parameters.tension,n=>d.parameters.tension=n);
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
 c.closed=false;c.degree=Math.max(1,Math.min(25,degree,Math.max(1,c.points.length-1)));
 if(c.points.length<2){delete c.knots;delete c.multiplicities;c.weights=c.points.map((_,i)=>c.weights?.[i]??1);return}
 const spans=c.points.length-c.degree;c.knots=Array.from({length:spans+1},(_,i)=>i/spans);c.multiplicities=c.knots.map((_,i)=>i===0||i===spans?c.degree!+1:1);c.weights=c.points.map((_,i)=>c.weights?.[i]??1);
}
export type PointEdit={kind:'append'}|{kind:'replace'|'before'|'after';id:string};
export function editGeometryPoint(d:GeometricFeatureDefinition,input:GeometricInput,edit:PointEdit):void{
 const curve=d.curve!,identity=geometryInputIdentity(input);
 if(curve.points.some(p=>p.id!==(edit.kind==='replace'?edit.id:undefined)&&pointInput(p)&&geometryInputIdentity(pointInput(p)!)===identity))throw new CommandInputError('该点已在点列中');
 if(edit.kind==='replace'){const point=curve.points.find(p=>p.id===edit.id);if(!point)throw new CommandInputError('原点位已改变');point.reference=input;point.position=[0,0,0];delete point.pointId;return}
 const point:GeometryControlPoint={id:randomUUID(),position:[0,0,0],reference:input};let at=curve.points.length;
 if(edit.kind!=='append'){at=curve.points.findIndex(p=>p.id===edit.id);if(at<0)throw new CommandInputError('插入位置已改变');if(edit.kind==='after')at++}
 curve.points.splice(at,0,point);curve.weights?.splice(at,0,1);
}
