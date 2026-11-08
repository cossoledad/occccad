import {Button,InputNumber,Segmented,Switch} from 'antd';
import {useEffect,useRef,useState} from 'react';
import type {Artifact,DocumentView,Feature,GeometricInput,SelectionItem} from '../../types';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField,CommandGroup,CommandStatus} from '../../cad/overlay/command-form';
import {CommandInputError} from '../../cad/command/operation-failure';
import {FeaturePickScope,type FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {randomUUID} from '../../utils/random-uuid';
import {displayLengthToMillimeters,millimetersToDisplayLength,type DisplayLengthUnit} from '../../state/ui-preferences';
import {FeaturePickField} from './feature-pick-field';
import {ParameterReferenceInput} from './parameter-reference-input';
import {parseParameterSource,parameterSourceText} from './parameter-editor';
import {useFeaturePreview} from './use-feature-preview';
import {useFeatureDialogFeedback} from './use-feature-dialog-feedback';
import {acceptsGeometryReference,bindGeometryReference,bindGeometricInput} from './geometric-input-selection';
import {editGeometryPoint,geometryFields,geometryInputIdentity,geometryInputName,geometryInputRoles,geometryNames,geometryReady,pointInput,sketchGeometryInput,splineControlMode,type GeometryReferenceRole,type PointEdit} from './geometric-feature-model';

type Props={view:DocumentView;feature:Feature;digest?:string;unit:DisplayLengthUnit;onClose:()=>void;onApply:(input:Record<string,unknown>)=>Promise<unknown>;onPreview:(artifact?:Artifact,operation?:Feature['operation'])=>void;onSelectionSession:(session?:FeatureSelectionSession)=>void};
type ReferenceField={key:'direction'|'axis'|'mirrorPlane'|'support';label:string;role:GeometryReferenceRole;input?:GeometricInput};
function referenceFields(feature:Feature):ReferenceField[]{
 const d=feature.geometry!;
 if(['SURFACE_EXTRUDE','TRANSLATE','PROJECT'].includes(d.kind))return [{key:'direction',label:'方向',role:'direction',input:d.direction}];
 if(d.kind==='ROTATE')return [{key:'axis',label:'旋转轴',role:'axis',input:d.axis}];
 if(d.kind==='MIRROR')return [{key:'mirrorPlane',label:'镜像平面',role:'plane',input:d.mirrorPlane}];
 if(['CIRCLE','ARC'].includes(d.kind))return [{key:'support',label:'支撑平面',role:'plane',input:d.curve?.support}];
 return [];
}
function firstMissing(feature:Feature):string|undefined{
 const d=feature.geometry!;
 if(d.kind==='SPLINE')return 'points';
 const point=d.curve?.points.find(p=>!pointInput(p));if(point)return `point:${point.id}`;
 const role=geometryInputRoles(d.kind).findIndex((_,i)=>!d.inputs?.[i]);if(role>=0)return `input:${role}`;
 const ref=referenceFields(feature).find(f=>!f.input);return ref?.key;
}

export function GeometricFeatureEditor({view,feature,digest,unit,onClose,onApply,onPreview,onSelectionSession}:Props){
 const [draft,setDraft]=useState(()=>structuredClone(feature));
 const draftRef=useRef(draft);draftRef.current=draft;
 const initial=()=>Object.fromEntries(geometryFields(structuredClone(feature.geometry!)).map(field=>{
  const p=view.part?.parameters?.find(p=>p.ownerFeatureId===feature.id&&p.propertySlot===field.slot);
  return [field.slot,p?parameterSourceText(p,field.unit==='mm'?unit:field.unit,true):String(field.unit==='mm'?millimetersToDisplayLength(field.value,unit):field.value)];
 }));
 const [values,setValues]=useState(initial),[committed,setCommitted]=useState(initial);
 const [picking,setPickingState]=useState<string|undefined>(()=>firstMissing(feature));
 const [pointEdit,setPointEdit]=useState<PointEdit>({kind:'append'});
 const [splineMode,setSplineMode]=useState<'FIT'|'CONTROL'>(()=>feature.geometry?.curve?.knots?.length?'CONTROL':'FIT');
 const [failure,setFailure]=useState<unknown>(),[pending,setPending]=useState(0),[saving,setSaving]=useState(false);
 const base=useRef(view.document.versionId).current,intent=useRef(randomUUID()).current,alive=useRef(true),scope=useRef(new FeaturePickScope()),queue=useRef(Promise.resolve());
 const visuals=useRef(new Map<string,SelectionItem>());
 const callbacks=useRef({onPreview,onSelectionSession});callbacks.current={onPreview,onSelectionSession};
 const fieldToken=JSON.stringify([picking,pointEdit]);scope.current.activate(fieldToken,`${view.document.id}/${base}`);
 const kind=draft.geometry!.kind,roles=geometryInputRoles(kind),multiple=kind==='COMPOSITE',refs=referenceFields(draft),inputs=draft.geometry!.inputs??[];
 const upstream=view.part!.features.slice(0,feature.id?view.part!.features.findIndex(f=>f.id===feature.id):undefined);
 const validRevision=base===view.document.versionId;
 const change=(update:(next:Feature)=>void)=>{const next=structuredClone(draftRef.current);update(next);draftRef.current=next;setDraft(next);setFailure(undefined)};
 const setPicking=(field?:string,edit:PointEdit={kind:'append'})=>{scope.current.cancel();setPickingState(field);setPointEdit(edit)};
 const roleFor=(field:string):GeometryReferenceRole|undefined=>field==='points'||field.startsWith('point:')?'point':refs.find(r=>r.key===field)?.role;
 const accept=(s:SelectionItem,field=picking):boolean=>{
  if(!field||(s.ownerDocumentId??s.documentId)!==view.document.id||(s.occurrencePath??'')!==''||s.versionId&&s.versionId!==base)return false;
  const stage=s.result?.featureId??s.displayStageFeatureId;if(stage&&!upstream.some(f=>f.id===stage))return false;
  if(field==='body')return !!s.bodyId;
  const refRole=roleFor(field);if(refRole)return acceptsGeometryReference(view,s,refRole);
  const role=roles[Number(field.split(':')[1])]?.role;if(!role)return false;
  if(s.kind==='plane')return role==='plane'||role==='surface'&&(kind==='PROJECT'||kind==='INTERSECTION');
  if(role==='plane')return acceptsGeometryReference(view,s,'plane');
  if(s.kind==='edge')return role==='curve'||role==='geometry';
  if(s.kind==='face')return role==='surface'||role==='geometry';
  if(s.kind==='vertex')return role==='geometry';
  if(sketchGeometryInput(view,s))return role==='curve'||role==='geometry';
  const source=upstream.find(f=>f.id===(s.result?.featureId??s.entityId??s.id));
  return !!source?.result&&(role==='geometry'||role==='surface'&&source.result.domain==='SHEET_SET'||role==='curve'&&source.result.domain==='WIRE_SET');
 };
 const pick=(s:SelectionItem)=>{
  const field=picking,ticket=scope.current.capture(),edit=pointEdit,role=field?roleFor(field):undefined;
  if(!field||!accept(s,field)||saving||!validRevision)return;
  // Continuous point picks bind in click order, including asynchronous topology
  // requests. A field/session change invalidates the queued work as a whole.
  setPending(n=>n+1);setFailure(undefined);
  queue.current=queue.current.then(async()=>{
   if(!alive.current||!scope.current.current(ticket))return;
   if(field==='body'){change(f=>{f.bodyId=s.bodyId});setPicking();return}
   const input=role?await bindGeometryReference(view,s,base,role):await bindGeometricInput(view,s,base,upstream);
   if(!alive.current||!scope.current.current(ticket))return;
   visuals.current.set(geometryInputIdentity(input),s);
   change(f=>{
    const d=f.geometry!;
    if(field==='points'){editGeometryPoint(d,input,edit);splineControlMode(d,splineMode==='CONTROL')}
    else if(field.startsWith('point:')){editGeometryPoint(d,input,{kind:'replace',id:field.slice(6)})}
    else if(field.startsWith('input:')){const index=Number(field.split(':')[1]),list=d.inputs??[];if(multiple){if(!list.some(i=>geometryInputIdentity(i)===geometryInputIdentity(input)))list.push(input)}else list[index]=input;d.inputs=list}
    else if(field==='support'){d.curve!.support=input;delete d.curve!.framePlaneId;d.curve!.normal=[0,0,0];d.curve!.xDirection=[0,0,0]}
    else {d[field as 'direction'|'axis'|'mirrorPlane']=input;d.parameters.direction=[0,0,0];d.parameters.origin=[0,0,0]}
   });
   if(field==='points'&&edit.kind==='append'||multiple)return;
   setPicking(field==='points'?undefined:firstMissing(draftRef.current));
  }).catch(error=>{if(alive.current&&scope.current.current(ticket))setFailure(error)}).finally(()=>{if(alive.current)setPending(n=>Math.max(0,n-1))});
 };
 const pickRef=useRef(pick);pickRef.current=pick;
 useEffect(()=>{alive.current=true;scope.current.activate(fieldToken,`${view.document.id}/${base}`);return()=>{alive.current=false;scope.current.cancel();callbacks.current.onPreview();callbacks.current.onSelectionSession()}},[]);
 const selectionInputs=[...inputs.filter(Boolean),...refs.flatMap(r=>r.input?[r.input]:[]),...(draft.geometry!.curve?.points.flatMap(p=>pointInput(p)?[pointInput(p)!]:[])??[])];
 const selections=selectionInputs.flatMap(i=>{const s=visuals.current.get(geometryInputIdentity(i));return s?[s]:[]});
 const selectionToken=JSON.stringify(selections);
 useEffect(()=>{
  if(!picking){callbacks.current.onSelectionSession();return}
  const role=roleFor(picking),shapeRole=roles[Number(picking.split(':')[1])]?.role;
  callbacks.current.onSelectionSession({documentId:view.document.id,versionId:base,role:picking==='body'?'body':role==='point'?'point':role==='plane'?'plane':role==='axis'?'axis':role==='direction'?'direction':shapeRole==='curve'?'edge':'face',topologyPick:role==='point'?'VERTEX':role==='axis'||role==='direction'||shapeRole==='curve'?'EDGE':undefined,selections,accept:s=>accept(s,picking),onPick:s=>pickRef.current(s)});
  return()=>callbacks.current.onSelectionSession();
 },[fieldToken,selectionToken,base]);
 let input:Record<string,unknown>|undefined,problem:unknown;
 const missing=!geometryReady(draft.geometry!)||kind==='THICKEN'&&draft.operation!=='NEW_BODY'&&!draft.bodyId;
 try{
  if(!missing&&validRevision){const next=structuredClone(draft),expressions:Record<string,string>={};
   for(const field of geometryFields(next.geometry!)){
    const text=committed[field.slot]??String(field.unit==='mm'?millimetersToDisplayLength(field.value,unit):field.value),parsed=parseParameterSource(text,field.unit==='mm'?unit:field.unit);
    if(parsed.kind==='EXPRESSION'){if(!parsed.expression)throw new CommandInputError('请输入参数');expressions[field.slot]=parsed.expression}
    else{if(field.unit==='mm'&&!['mm','cm','m','in'].includes(parsed.unit)||field.unit==='deg'&&!['deg','rad'].includes(parsed.unit)||field.unit===''&&parsed.unit!=='')throw new CommandInputError('参数单位不匹配');field.set(field.unit==='mm'?displayLengthToMillimeters(parsed.value,parsed.unit as DisplayLengthUnit):field.unit==='deg'&&parsed.unit==='rad'?parsed.value*180/Math.PI:parsed.value)}
   }
   input={type:feature.id?'EDIT_FEATURE':kind==='THICKEN'?'CREATE_SOLID_FEATURE':'CREATE_GEOMETRY_FEATURE',requestId:intent,targetId:feature.id||undefined,expectedFeatureDigest:digest,feature:next,parameterExpressions:expressions};
  }
 }catch(error){problem=error}
 const preview=useFeaturePreview(view.document.id,base,input,draft.operation,onPreview);
 const feedback=useFeatureDialogFeedback(failure??preview.failure??(!validRevision?new CommandInputError('模型已改变，请重新打开命令'):undefined),geometryNames[kind],problem,preview.repairRequired,preview.retry);
 const fields=geometryFields(draft.geometry!);
 const fieldInput=(field:typeof fields[number])=><CommandField key={field.slot} label={field.label+(field.unit?`（${field.unit==='mm'?unit:field.unit}）`:'')} width="expression"><ParameterReferenceInput value={values[field.slot]??String(field.unit==='mm'?millimetersToDisplayLength(field.value,unit):field.value)} parameters={view.part?.parameters??[]} unit={field.unit==='mm'?unit:undefined} onChange={v=>setValues(old=>({...old,[field.slot]:v}))} onBlur={()=>{setCommitted({...values});setFailure(undefined);feedback.reportValidation()}}/></CommandField>;
 const reverse=(field:string)=>change(f=>{const d=f.geometry!,value=field.startsWith('input:')?d.inputs![Number(field.split(':')[1])]:field==='support'?d.curve!.support:d[field as 'direction'|'axis'|'mirrorPlane'];if(value)value.reversed=!value.reversed});
 const referenceField=(field:ReferenceField)=><FeaturePickField compact key={field.key} label={field.label} value={geometryInputName(view,field.input)} selected={!!field.input} active={picking===field.key} onActivate={()=>setPicking(field.key)} onClear={field.input?()=>change(f=>{if(field.key==='support')delete f.geometry!.curve!.support;else delete f.geometry![field.key]}):undefined} selections={field.input?selections.filter(s=>s===visuals.current.get(geometryInputIdentity(field.input!))):undefined} actions={field.input&&field.role!=='plane'?<Button size="small" type={field.input.reversed?'primary':'text'} aria-pressed={!!field.input.reversed} onClick={()=>reverse(field.key)}>反向</Button>:undefined}/>;
 return <CommandDialog id="geometry-feature" open size={kind==='SPLINE'?'M':'S'} title={`${feature.id?'编辑':''}${geometryNames[kind]}`} onClose={onClose} onCancelPick={()=>{if(!picking)return false;setPicking();return true}} commandStatus={<CommandStatus pending={pending>0||preview.pending} onRetry={preview.retry}/>} confirmLoading={saving} confirmDisabled={!!problem||missing||!preview.previewId||preview.pending||pending>0||!validRevision||JSON.stringify(values)!==JSON.stringify(committed)} onConfirm={async()=>{if(!input||!preview.previewId||saving)return;setSaving(true);try{await onApply({...input,previewId:preview.previewId});onClose()}catch(error){setFailure(error);setSaving(false)}}}>
  <fieldset disabled={saving} className="feature-input-fields geometric-feature-fields" onBlurCapture={feedback.onBlurCapture}>
   {roles.map((role,index)=><FeaturePickField compact key={index} label={role.label} value={multiple?`${inputs.length} 条曲线`:geometryInputName(view,inputs[index])} active={picking===`input:${index}`} selected={multiple?!!inputs.length:!!inputs[index]} mode={multiple?'ordered':'single'} onActivate={()=>setPicking(`input:${index}`)} onClear={inputs[index]?()=>change(f=>{if(multiple)f.geometry!.inputs=[];else delete f.geometry!.inputs![index]}):undefined} items={multiple?inputs.map((i,n)=>({key:geometryInputIdentity(i),value:geometryInputName(view,i),onClear:()=>change(f=>{f.geometry!.inputs!.splice(n,1)}),actions:<Button size="small" type={i.reversed?'primary':'text'} onClick={()=>reverse(`input:${n}`)}>反向</Button>})):undefined} actions={!multiple&&inputs[index]&&inputs[index].kind!=='PLANE'?<Button size="small" type={inputs[index].reversed?'primary':'text'} onClick={()=>reverse(`input:${index}`)}>反向</Button>:undefined}/>)}
   {kind!=='SPLINE'&&draft.geometry!.curve?.points.map((p,index)=><FeaturePickField compact key={p.id} label={kind==='LINE'?(index?'终点':'起点'):'圆心'} value={geometryInputName(view,pointInput(p))} selected={!!pointInput(p)} active={picking===`point:${p.id}`} onActivate={()=>setPicking(`point:${p.id}`)} onClear={pointInput(p)?()=>change(f=>{const point=f.geometry!.curve!.points.find(q=>q.id===p.id)!;delete point.pointId;delete point.reference}):undefined}/>)}
   {kind==='SPLINE'&&<>
    <CommandField label="样条方式"><Segmented<'FIT'|'CONTROL'> value={splineMode} options={[{label:'插值点',value:'FIT'},{label:'控制点',value:'CONTROL'}]} onChange={mode=>{setSplineMode(mode);change(f=>splineControlMode(f.geometry!,mode==='CONTROL'))}}/></CommandField>
    <FeaturePickField compact label="点列" mode="ordered" active={picking==='points'&&pointEdit.kind==='append'} selected={!!draft.geometry!.curve!.points.length} onActivate={()=>setPicking('points')} items={draft.geometry!.curve!.points.map((p,index)=>({key:p.id,actionLabel:`点 ${index+1}`,value:geometryInputName(view,pointInput(p)),active:picking===`point:${p.id}`||picking==='points'&&'id'in pointEdit&&pointEdit.id===p.id,onActivate:()=>setPicking(`point:${p.id}`),onClear:()=>change(f=>{const c=f.geometry!.curve!,at=c.points.findIndex(q=>q.id===p.id);c.points.splice(at,1);c.weights?.splice(at,1);splineControlMode(f.geometry!,splineMode==='CONTROL')}),selections:pointInput(p)?[visuals.current.get(geometryInputIdentity(pointInput(p)!))!].filter(Boolean):[],actions:<><Button size="small" type="text" aria-label={`在点 ${index+1} 前插入`} onClick={()=>setPicking('points',{kind:'before',id:p.id})}>插前</Button><Button size="small" type="text" aria-label={`在点 ${index+1} 后插入`} onClick={()=>setPicking('points',{kind:'after',id:p.id})}>插后</Button></>}))}/>
    {splineMode==='CONTROL'?<CommandField label="次数"><InputNumber min={1} max={Math.max(1,Math.min(25,draft.geometry!.curve!.points.length-1))} value={draft.geometry!.curve!.degree??3} onChange={n=>change(f=>splineControlMode(f.geometry!,true,n??3))}/></CommandField>:<CommandField label="闭合"><Switch aria-label="闭合样条" disabled={draft.geometry!.curve!.points.length<3} checked={draft.geometry!.curve!.closed} onChange={closed=>change(f=>{f.geometry!.curve!.closed=closed})}/></CommandField>}
   </>}
   {refs.map(referenceField)}
   {fields.length===1?fieldInput(fields[0]):fields.length>1&&<CommandGroup label="参数">{fields.map(fieldInput)}</CommandGroup>}
   {kind==='EXTEND'&&<CommandField label="延伸端"><Segmented<'START'|'END'|'BOTH'> value={draft.geometry!.parameters.side} options={[{label:'起点',value:'START'},{label:'终点',value:'END'},{label:'两端',value:'BOTH'}]} onChange={side=>change(f=>{f.geometry!.parameters.side=side})}/></CommandField>}
   {kind==='THICKEN'&&<><CommandField label="实体操作"><Segmented value={draft.operation} options={[...(!feature.id?[{label:'新实体',value:'NEW_BODY'}]:[]),{label:'添加',value:'ADD'},{label:'切除',value:'REMOVE'},{label:'交集',value:'INTERSECT'}]} onChange={operation=>{change(f=>{f.operation=operation as Feature['operation']});if(operation!=='NEW_BODY'&&!draft.bodyId)setPicking('body')}}/></CommandField>{draft.operation!=='NEW_BODY'&&<FeaturePickField compact label="目标实体" value={view.part?.bodies.find(b=>b.id===draft.bodyId)?.name??'未选择'} active={picking==='body'} disabled={!!feature.id} selected={!!draft.bodyId} onActivate={()=>setPicking('body')} onClear={!feature.id&&draft.bodyId?()=>change(f=>{delete f.bodyId}):undefined}/>}</>}
  </fieldset>
 </CommandDialog>
}
