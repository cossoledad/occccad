import {useEffect,useState} from 'react';
import {randomUUID} from '../../utils/random-uuid';
import {Button,Segmented} from 'antd';
import type {Feature,GeometricInput} from '../../types';
import {CommandItemDetails} from '../../cad/overlay/command-item-details';
import {CommandField,CommandGroup} from '../../cad/overlay/command-form';
import {geometryInputName,type GeometryReferenceRole} from './geometric-feature-model';
import type {DocumentView} from '../../types';
import type {ReactNode} from 'react';
export type GeometryReferenceField={key:string;label:string;role:GeometryReferenceRole|'curve'|'surface';input?:GeometricInput;required?:boolean};
export function surfaceReferenceFields(feature:Feature,view?:DocumentView):GeometryReferenceField[]{
 const d=feature.geometry!,s=d.surface;if(!s)return [];const out:GeometryReferenceField[]=[];
 if(d.kind==='SURFACE_LOFT')out.push({key:'surface.spine',label:'脊线',role:'curve',input:s.spine});
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
 else d[key as 'direction'|'axis'|'mirrorPlane'|'keepPoint'|'secondKeepPoint']=input;
}
export function SurfaceControlFields({view,feature,activeField,change,renderPick,renderTension,onSelectInput}:{view:DocumentView;feature:Feature;activeField?:string;change:(update:(f:Feature)=>void)=>void;renderPick:(f:GeometryReferenceField)=>ReactNode;renderTension:(id:string)=>ReactNode;onSelectInput?:(input:GeometricInput)=>void}){
 const d=feature.geometry!,s=d.surface;
 const [sectionID,setSectionID]=useState<string>(),[boundaryID,setBoundaryID]=useState<string>();
 useEffect(()=>{if(!activeField)return;const id=activeField.split(':')[1];if(activeField.startsWith('boundary-support:'))setBoundaryID(id);else if(/^(closing|station|coupling):/.test(activeField))setSectionID(id)},[activeField]);
 if(!s)return null;
 const fields=surfaceReferenceFields(feature,view),field=(key:string)=>{const f=fields.find(f=>f.key===key);return f?renderPick(f):null};
 const section=s.sections?.find(c=>c.id===sectionID)??s.sections?.[0],boundary=s.boundaries?.find(c=>c.id===boundaryID)??s.boundaries?.[0];
 const addCoupling=()=>change(f=>{const id=randomUUID();for(const c of f.geometry!.surface!.sections??[])c.couplings=[...c.couplings??[],{id}]});
 return <>
  {fields.some(f=>f.key.startsWith('surface.'))&&<CommandGroup label="形状控制">{field('surface.spine')}{field('surface.guide')}{s.guide&&<CommandField label="导向方式"><Segmented<'GUIDE_FRAME'|'GUIDE_BORDER'> value={s.frameMode==='GUIDE_BORDER'?'GUIDE_BORDER':'GUIDE_FRAME'} options={[{label:'姿态',value:'GUIDE_FRAME'},{label:'贴合边界',value:'GUIDE_BORDER'}]} onChange={mode=>change(f=>{f.geometry!.surface!.frameMode=mode})}/></CommandField>}</CommandGroup>}
  {section&&<CommandGroup label="截面与对应"><CommandItemDetails label="截面设置" items={s.sections!.map((c,i)=>({id:c.id,name:`${i+1} · ${geometryInputName(view,c.section)}`,detail:`${c.couplings?.length??0} 个对应点`}))} selected={section.id} onSelect={id=>{setSectionID(id);const input=s.sections!.find(c=>c.id===id)?.section;if(input)onSelectInput?.(input)}}>
   {field(`closing:${section.id}`)}{field(`station:${section.id}`)}
   {section.couplings?.map(p=><div className="command-correspondence" key={p.id}>{field(`coupling:${section.id}:${p.id}`)}<Button type="text" size="small" aria-label={`移除对应 ${(section.couplings?.indexOf(p)??0)+1}`} onClick={()=>change(f=>{for(const c of f.geometry!.surface!.sections??[])c.couplings=c.couplings?.filter(x=>x.id!==p.id)})}>移除</Button></div>)}
   {d.kind==='SURFACE_LOFT'&&<Button size="small" disabled={(section.couplings?.length??0)>=16} onClick={addCoupling}>添加对应点</Button>}
  </CommandItemDetails></CommandGroup>}
  {boundary&&<CommandGroup label="边界连续性"><CommandItemDetails label="边界设置" items={s.boundaries!.map((c,i)=>({id:c.id,name:`边界 ${i+1}`,detail:c.continuity}))} selected={boundary.id} onSelect={id=>{setBoundaryID(id);const c=s.boundaries!.find(c=>c.id===id),input=c?.boundary;if(input)onSelectInput?.(input)}}>
   <CommandField label="连续性"><Segmented<'G0'|'G1'|'G2'> value={boundary.continuity} options={[{label:'G0',value:'G0',title:'位置连续'},{label:'G1',value:'G1',title:'相切连续'},{label:'G2',value:'G2',title:'曲率连续'}]} onChange={value=>change(f=>{const c=f.geometry!.surface!.boundaries!.find(c=>c.id===boundary.id)!;c.continuity=value;if(value==='G0')delete c.support})}/></CommandField>
   {field(`boundary-support:${boundary.id}`)}{boundary.continuity!=='G0'&&renderTension(boundary.id)}
  </CommandItemDetails></CommandGroup>}
 </>;
}
