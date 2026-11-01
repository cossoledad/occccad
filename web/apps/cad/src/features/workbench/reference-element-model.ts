import type {DatumReference,DocumentView,ReferenceConstruction} from '../../types';

export type ReferenceElementKind='point'|'line'|'plane';
export const referenceModes:Record<ReferenceElementKind,Array<{value:string;label:string;roles:Array<'point'|'plane'|'direction'>}>>={
 point:[{value:'POINT_COORDINATES',label:'坐标',roles:[]},{value:'POINT_BETWEEN',label:'两点之间',roles:['point','point']},{value:'POINT_CENTER',label:'圆 / 球中心',roles:['point']},{value:'POINT_ON_PLANE',label:'平面内坐标',roles:['plane']}],
 line:[{value:'LINE_TWO_POINTS',label:'两点',roles:['point','point']},{value:'LINE_POINT_DIRECTION',label:'点与方向',roles:['point','direction']},{value:'LINE_NORMAL_PLANE',label:'过点垂直于平面',roles:['point','plane']},{value:'LINE_COORDINATES',label:'原点与方向',roles:[]}],
 plane:[{value:'PLANE_OFFSET',label:'偏移平面',roles:['plane']},{value:'PLANE_PARALLEL_POINT',label:'过点平行',roles:['plane','point']},{value:'PLANE_THREE_POINTS',label:'经过三点',roles:['point','point','point']},{value:'PLANE_ANGLE',label:'绕轴成角',roles:['plane','direction']},{value:'PLANE_EQUATION',label:'平面方程',roles:[]},{value:'PLANE_COORDINATES',label:'原点与法向',roles:[]}],
};
export function defaultReferenceConstruction(kind:ReferenceElementKind):ReferenceConstruction{return {mode:referenceModes[kind][0].value,inputs:kind==='plane'?[{kind:'PLANE',entityId:'datum-xy'}]:[],coordinates:[0,0,0],direction:[0,0,1],ratio:.5,offset:0,angle:0,coefficients:[0,0,1,0]}}
export function referenceFields(c:ReferenceConstruction):Array<{slot:string;label:string;unit:'mm'|'deg'|'';value:number}>{
 const coordinate=['POINT_COORDINATES','LINE_COORDINATES','PLANE_COORDINATES'].includes(c.mode);
 const fields:Array<{slot:string;label:string;unit:'mm'|'deg'|'';value:number}>=[];
 if(coordinate)for(const [i,axis] of ['x','y','z'].entries())fields.push({slot:'reference:'+axis,label:axis.toUpperCase(),unit:'mm',value:c.coordinates[i]});
 if(coordinate&&c.mode!=='POINT_COORDINATES')for(const [i,axis] of ['x','y','z'].entries())fields.push({slot:'reference:d'+axis,label:(c.mode==='PLANE_COORDINATES'?'法向 ':'方向 ')+axis.toUpperCase(),unit:'',value:c.direction[i]});
 if(c.mode==='POINT_BETWEEN')fields.push({slot:'reference:ratio',label:'比例',unit:'',value:c.ratio});
 if(c.mode==='POINT_ON_PLANE')for(const [i,axis] of ['u','v'].entries())fields.push({slot:'reference:'+axis,label:axis.toUpperCase(),unit:'mm',value:c.coordinates[i]});
 if(c.mode==='PLANE_OFFSET')fields.push({slot:'reference:offset',label:'偏移',unit:'mm',value:c.offset});
 if(c.mode==='PLANE_ANGLE')fields.push({slot:'reference:angle',label:'角度',unit:'deg',value:c.angle});
 if(c.mode==='PLANE_EQUATION')for(const [i,axis] of ['a','b','c','d'].entries())fields.push({slot:'reference:'+axis,label:axis.toUpperCase(),unit:i===3?'mm':'',value:c.coefficients[i]});
 return fields;
}
export function setReferenceValue(c:ReferenceConstruction,slot:string,value:number){
 const key=slot.replace('reference:',''),axis=['x','y','z'].indexOf(key);
 if(axis>=0)c.coordinates[axis]=value;else if(/^d[xyz]$/.test(key))c.direction[['dx','dy','dz'].indexOf(key)]=value;
 else if(key==='u'||key==='v')c.coordinates[key==='u'?0:1]=value;
 else if(key==='ratio'||key==='offset'||key==='angle')c[key]=value;
 else if(['a','b','c','d'].includes(key))c.coefficients[['a','b','c','d'].indexOf(key)]=value;
}
export function referenceInputName(view:DocumentView,ref:DatumReference|undefined):string{
 if(!ref)return '未选择';const part=view.part;
 const value=part?.datumPoints?.find(p=>p.id===ref.entityId)??part?.datumPlanes.find(p=>p.id===ref.entityId)??part?.datumAxes?.find(p=>p.id===ref.entityId)??part?.axisSystems.find(p=>p.id===ref.entityId);
 return value?value.name+(ref.axis?' · '+ref.axis:''):ref.kind==='TOPOLOGY'?'关联拓扑元素':'来源不可用';
}
