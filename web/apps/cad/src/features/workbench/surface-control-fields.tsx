import {randomUUID} from '../../utils/random-uuid';
import {Button,Segmented} from 'antd';
import type {Feature,GeometricInput} from '../../types';
import {CommandField,CommandGroup} from '../../cad/overlay/command-form';
import {geometryInputName,type GeometryReferenceRole} from './geometric-feature-model';
import type {DocumentView} from '../../types';
import type {ReactNode} from 'react';
export type GeometryReferenceField={key:string;label:string;role:GeometryReferenceRole|'curve'|'surface';input?:GeometricInput;required?:boolean};
export function surfaceReferenceFields(feature:Feature,view?:DocumentView):GeometryReferenceField[]{
 const d=feature.geometry!,s=d.surface;if(!s)return [];const out:GeometryReferenceField[]=[];
 if(d.kind==='SURFACE_LOFT')out.push({key:'surface.spine',label:'Spine',role:'curve',input:s.spine});
 if(['SURFACE_LOFT','SURFACE_SWEEP'].includes(d.kind))out.push({key:'surface.guide',label:'导向曲线',role:'curve',input:s.guide});
 for(const c of s.sections??[]){const source=view?.part?.features.find(f=>f.id===(c.section.featureId??c.section.selection?.sourceFeatureId));const knownOpen=source?.geometry&&(['LINE','ARC'].includes(source.geometry.kind)||source.geometry.kind==='SPLINE'&&!source.geometry.curve?.closed)||c.section.selection?.selection.creationEvidence.geometryType==='LINE';if(!knownOpen)out.push({key:`closing:${c.id}`,label:'闭合点',role:'point',input:c.closingPoint});if(s.spine)out.push({key:`station:${c.id}`,label:'站位点',role:'point',input:c.stationPoint,required:true});}
 for(const c of s.sections??[])for(const p of c.couplings??[])out.push({key:`coupling:${c.id}:${p.id}`,label:`对应点 ${(c.couplings?.indexOf(p)??0)+1}`,role:'point',input:p.point,required:true});
 for(const c of s.boundaries??[])if(c.continuity!=='G0')out.push({key:`boundary-support:${c.id}`,label:'支撑',role:'surface',input:c.support,required:true});
 return out;
}
export function setGeometryReference(feature:Feature,key:string,input?:GeometricInput):void {
 const d=feature.geometry!,s=d.surface;
 if(key==='surface.spine'){s!.spine=input;if(input)s!.ruled=false;else s!.sections?.forEach(c=>delete c.stationPoint)}
 else if(key==='surface.guide'){s!.guide=input;if(input){s!.ruled=false;if(!['GUIDE_FRAME','GUIDE_BORDER'].includes(s!.frameMode??''))s!.frameMode='GUIDE_FRAME'}else s!.frameMode=d.kind==='SURFACE_SWEEP'?'CORRECTED_FRENET':undefined}
 else if(key.startsWith('closing:')||key.startsWith('station:')){const c=s!.sections!.find(c=>c.id===key.split(':')[1])!;if(key.startsWith('closing:'))c.closingPoint=input;else c.stationPoint=input}
 else if(key.startsWith('coupling:')){const [,id,point]=key.split(':');s!.sections!.find(c=>c.id===id)!.couplings!.find(p=>p.id===point)!.point=input}
 else if(key.startsWith('boundary-support:'))s!.boundaries!.find(c=>c.id===key.split(':')[1])!.support=input;
 else if(key==='support')d.curve!.support=input;
 else d[key as 'direction'|'axis'|'mirrorPlane'|'keepPoint']=input;
}
export function SurfaceControlFields({view,feature,change,renderPick,renderTension}:{view:DocumentView;feature:Feature;change:(update:(f:Feature)=>void)=>void;renderPick:(f:GeometryReferenceField)=>ReactNode;renderTension:(id:string)=>ReactNode}){
 const d=feature.geometry!,s=d.surface;if(!s)return null;
 const fields=surfaceReferenceFields(feature,view),field=(key:string)=>{const f=fields.find(f=>f.key===key);return f?renderPick(f):null};
 return <>
  {fields.some(f=>f.key.startsWith('surface.'))&&<CommandGroup label="形状控制">{field('surface.spine')}{field('surface.guide')}{s.guide&&<CommandField label="导向方式"><Segmented<'GUIDE_FRAME'|'GUIDE_BORDER'> value={s.frameMode==='GUIDE_BORDER'?'GUIDE_BORDER':'GUIDE_FRAME'} options={[{label:'姿态',value:'GUIDE_FRAME'},{label:'贴合边界',value:'GUIDE_BORDER'}]} onChange={mode=>change(f=>{f.geometry!.surface!.frameMode=mode})}/></CommandField>}</CommandGroup>}
  {(s.sections??[]).filter(c=>fields.some(f=>f.key===`closing:${c.id}`||f.key===`station:${c.id}`||f.key.startsWith(`coupling:${c.id}:`))).map(c=><CommandGroup key={c.id} label={geometryInputName(view,c.section)}>{field(`closing:${c.id}`)}{field(`station:${c.id}`)}{c.couplings?.map(p=>field(`coupling:${c.id}:${p.id}`))}</CommandGroup>)}
  {d.kind==='SURFACE_LOFT'&&!!s.sections?.length&&<CommandField label="点对应"><Button size="small" disabled={(s.sections[0].couplings?.length??0)>=16} onClick={()=>change(f=>{const id=randomUUID();for(const c of f.geometry!.surface!.sections??[])c.couplings=[...c.couplings??[],{id}]})}>添加对应</Button>{s.sections[0].couplings?.map((p,i)=><Button size="small" key={p.id} type="text" onClick={()=>change(f=>{for(const c of f.geometry!.surface!.sections??[])c.couplings=c.couplings?.filter(x=>x.id!==p.id)})}>移除对应 {i+1}</Button>)}</CommandField>}
  {(s.boundaries??[]).map((c,index)=><CommandGroup key={c.id} label={`边界 ${index+1}`}><CommandField label="连续性"><Segmented<'G0'|'G1'|'G2'> value={c.continuity} options={[{label:'位置 G0',value:'G0'},{label:'相切 G1',value:'G1'},{label:'曲率 G2',value:'G2'}]} onChange={value=>change(f=>{const control=f.geometry!.surface!.boundaries!.find(n=>n.id===c.id)!;control.continuity=value;if(value==='G0')delete control.support})}/></CommandField>{field(`boundary-support:${c.id}`)}{c.continuity!=='G0'&&renderTension(c.id)}</CommandGroup>)}
 </>;
}
