import type {DocumentView,InstancePath,PersistentSelection,SelectionItem,Vec3} from '../../types';

export type MeasurementKind='ITEM'|'BETWEEN'|'ANGLE_POINTS';
export type MeasurementMode='GEOMETRY'|'CENTER'|'AXIS';
export type MeasurementSupport={kind:'BODY'|'OCCURRENCE'|'FACE'|'EDGE'|'VERTEX';bodyId?:string;instancePath?:InstancePath;persistentSelection?:PersistentSelection;sourceRevisionId?:string;mode:MeasurementMode;geometryKey?:string;localId?:number};
export type MeasurementEvidence={sourceRevisionId:string;sourceStamp:string;policy:string;complete:boolean;diagnostic?:string;geometryType?:string;values:Record<string,number>;anchors:Record<string,Vec3>;kernelBuild:string};
export type Measurement={id:string;name:string;kind:MeasurementKind;supports:MeasurementSupport[];visible:boolean;showComponents:boolean;result?:MeasurementEvidence;status?:string;outputCurrency?:Record<string,string>;displayAnchors?:Record<string,Vec3>};
export type MeasurementReport={documentId:string;revisionId:string;definition:Measurement;result:MeasurementEvidence};
export type MeasurementUnit='mm'|'cm'|'m'|'in';
export const measurementTitle=(kind:MeasurementKind)=>({ITEM:'测量对象',BETWEEN:'测量之间',ANGLE_POINTS:'三点角度'}[kind]);
export const measurements=(view:DocumentView)=>view.part?.measurements??view.product?.measurements??[];

export function measurementSelection(view:DocumentView,id:string):SelectionItem|undefined {
 if(!measurements(view).some(m=>m.id===id&&m.visible))return;
 const find=(node:import('../../types').DocumentStructureNode|undefined):import('../../types').DocumentStructureNode|undefined=>{
  if(!node)return;
  if(node.kind==='MEASUREMENT'&&node.entityId===id&&node.documentId===view.document.id&&!(node.instancePath?.segments.length))return node;
  for(const child of node.children??[]){const result=find(child);if(result)return result}
 };
 const node=find(view.structureTree);if(!node)return;
 return {kind:'measurement',id,entityId:id,documentId:view.document.id,ownerDocumentId:view.document.id,versionId:view.document.versionId,entityRef:node.subject,snapshotScope:node.snapshot,instancePath:node.instancePath,occurrencePath:'',rootDocumentId:node.instancePath?.rootDocumentId,treeNodeId:node.id};
}

export function measurementInteraction(view:DocumentView,selected:readonly SelectionItem[],hovered?:SelectionItem|null){
 const match=(s:SelectionItem)=>s.kind==='measurement'&&(s.ownerDocumentId??s.documentId)===view.document.id&&!(s.instancePath?.segments.length)&&!(s.occurrencePath)&&(!s.versionId||s.versionId===view.document.versionId);
 return {selected:selected.filter(match).map(s=>s.entityId??s.id),hovered:hovered&&match(hovered)?hovered.entityId??hovered.id:undefined};
}
export const measurementStatus=(status?:string)=>({CURRENT:'已更新',STALE:'待更新',BROKEN:'来源不可用',MISSING:'尚无结果',UNKNOWN:'状态暂不可用'}[status??'']??'待更新');
export const defaultMeasurement=(kind:MeasurementKind):Measurement=>({id:'',name:'',kind,supports:[],visible:true,showComponents:false});
export const measurementCount=(kind:MeasurementKind)=>kind==='ITEM'?1:kind==='BETWEEN'?2:3;
export function measurementPick(view:DocumentView,s:SelectionItem,mode:MeasurementMode='GEOMETRY',filter='ANY'):MeasurementSupport|undefined{
 if(s.contextVariantKey)return;
 const topology=['face','edge','vertex'].includes(s.kind);
 if(filter!=='ANY'&&s.kind!==filter.toLowerCase()&&!(filter==='BODY'&&['body','instance','part','product'].includes(s.kind)))return;
 if(!topology&&!['body','instance','part','product'].includes(s.kind))return;
 if(mode!=='GEOMETRY'&&!topology)return;
 let path:InstancePath|undefined;
 if(view.product){
  path=s.instancePath??s.occurrenceRef?.instancePath;
  if(!path?.segments.length||path.rootDocumentId!==view.document.id)return;
  const leaf=path.segments.at(-1)!;
  if(s.versionId&&s.kind!=='instance'&&s.versionId!==leaf.resolvedVersionId)return;
 }else if((s.ownerDocumentId??s.documentId)!==view.document.id||s.versionId&&s.versionId!==view.document.versionId||s.occurrencePath)return;
 if(topology){
  if(!s.bodyId||!s.geometryKey||!('topologyId' in s)||s.topologyId<=0)return;
  return {kind:s.kind.toUpperCase() as MeasurementSupport['kind'],bodyId:s.bodyId,instancePath:path,mode,geometryKey:s.geometryKey,localId:s.topologyId};
 }
 if(s.kind==='body'){const bodyId=s.bodyId??s.entityId;if(!bodyId)return;return {kind:'BODY',bodyId,instancePath:path,mode};}
 if(path)return {kind:'OCCURRENCE',instancePath:path,mode};
 return;
}
export function measurementSupportLabel(s:MeasurementSupport,view:DocumentView):string{
 const kind={BODY:'实体',OCCURRENCE:'组件',FACE:'面',EDGE:'边',VERTEX:'顶点'}[s.kind];
 const body=view.part?.bodies.find(b=>b.id===s.bodyId)?.name;
 const path=s.instancePath?.display;
 return [path||body,kind,s.mode==='CENTER'?'圆心 / 球心':s.mode==='AXIS'?'轴线':undefined].filter(Boolean).join(' · ');
}
const labels:Record<string,string>={distanceMm:'最短距离',angleRad:'夹角',lengthMm:'长度',perimeterMm:'周长',areaMm2:'面积',volumeMm3:'体积',radiusMm:'半径',diameterMm:'直径',majorRadiusMm:'主半径',minorRadiusMm:'次半径',arcAngleRad:'圆弧角度',xMm:'X',yMm:'Y',zMm:'Z',deltaXMm:'ΔX',deltaYMm:'ΔY',deltaZMm:'ΔZ',sizeXMm:'尺寸 X',sizeYMm:'尺寸 Y',sizeZMm:'尺寸 Z'};
export const measurementValueLabel=(key:string)=>labels[key]??key;
export function measurementNumber(value:number):string{return new Intl.NumberFormat('zh-CN',{maximumSignificantDigits:7,useGrouping:false}).format(Math.abs(value)<1e-12?0:value)}
export function measurementValue(key:string,value:number,unit:MeasurementUnit):string{
 if(key.endsWith('Rad'))return `${measurementNumber(value*180/Math.PI)}°`;
 const scale={mm:1,cm:10,m:1000,in:25.4}[unit],power=key.endsWith('Mm3')?3:key.endsWith('Mm2')?2:1;
 return `${measurementNumber(value/scale**power)} ${unit}${power===2?'²':power===3?'³':''}`;
}
export function measurementSummary(m:Measurement,unit:MeasurementUnit):string{
 const values=m.result?.values??{},key=['distanceMm','angleRad','lengthMm','areaMm2','volumeMm3','xMm'].find(k=>k in values);
 return key?`${measurementValueLabel(key)} ${measurementValue(key,values[key],unit)}`:'尚无结果';
}
export function measurementProblem(error:unknown):string{
 const text=error instanceof Error?error.message:String(error),code=text.match(/MEASUREMENT_[A-Z_]+/)?.[0];
 const messages:Record<string,string>={MEASUREMENT_CENTER_UNSUPPORTED:'该对象没有可用于测量的圆心或球心，请选择圆边、椭圆边或球面。',MEASUREMENT_AXIS_COMBINATION_UNSUPPORTED:'当前轴线距离支持另一条解析轴线或精确点 / 中心，请调整取用方式。',MEASUREMENT_AXIS_UNSUPPORTED:'该对象没有可用的解析轴线，请选择圆边、圆柱面或圆锥面。',MEASUREMENT_ITEM_AXIS_UNBOUNDED:'无限轴线没有可测长度，请使用“测量之间”。',MEASUREMENT_DEGENERATE_ANGLE:'角度的顶点与端点重合，请重新选择三个点。',MEASUREMENT_REQUIRES_THREE_POINTS:'请选择三个顶点，或以圆心 / 球心方式选择。',MEASUREMENT_CONTEXT_VARIANT_UNSUPPORTED:'当前测量尚不支持上下文专用几何，请打开对应 Part 测量。',MEASUREMENT_STALE_REVISION:'模型已更新，请重新选择或更新测量。',MEASUREMENT_BODY_NOT_UPDATED:'所选实体尚未成功更新，请先修复模型。',MEASUREMENT_OCCURRENCE_BROKEN:'所选组件已失效，请重新选择。',MEASUREMENT_SCOPE_LIMIT:'测量超过了 128 个实体或 256 MiB B-Rep 的计算范围。'};
 return code?messages[code]??'无法完成精确测量，请检查几何来源并重新选择。':'测量未完成，请重试。';
}
