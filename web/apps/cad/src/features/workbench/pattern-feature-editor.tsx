import {boundContextParameter,ContextInputSource} from './context-input-source';
import {CommandInputError} from "../../cad/command/operation-failure";
import {CommandField,CommandStatus,fieldErrorText} from "../../cad/overlay/command-form";
import {engineeringProblem} from "../../cad/command/operation-feedback";

import type { FeaturePickRole } from "../../cad/interaction/feature-selection";
import { Alert, Button, Input, Select, Segmented,Switch } from "antd";
import { useEffect, useRef, useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import type { Artifact, Feature, PatternDefinition, SelectionItem } from "../../types";
import type { SolidFeatureEditor } from "./solid-feature-editor";
import { FeaturePickField } from "./feature-pick-field";
import { patternRangeEnds, patternSourceCandidates, selectedSketch, selectedAxis, sketchPick } from "./feature-picking";
import { parameterSourceText, parseParameterSource } from "./parameter-editor";
import { useFeaturePreview } from "./use-feature-preview";
import { pickedPatternCenter, sketchPatternDefaultReferences } from "./pattern-selection";
import { api } from "../../api/client";
import {objectName,selectionName} from "../../cad/presentation/engineering-text";
import {patternAxisPick,patternReferencePick,patternReferenceName} from "./pattern-selection";
import { randomUUID } from "../../utils/random-uuid";

export function PatternFeatureEditor({ view, feature, digest, unit, seed, occurrencePath, occurrenceContext, onClose, onApply, onPreview, onSelectionSession, onInputArtifact }: Parameters<typeof SolidFeatureEditor>[0]) {
  const internal=feature.type==="SKETCH";
  const upstream=view.part!.features.slice(0,feature.id?view.part!.features.findIndex(f=>f.id===feature.id):undefined);
  const initialRange={status:"NONE" as const};
  const sourceSketch=view.part?.features.find(f=>f.id===feature.profile)?.sketch;
  const existingPattern=sourceSketch?.patterns?.find(p=>p.id===feature.pattern?.id);
  const [entityIds,setEntityIds]=useState<string[]>(existingPattern?.entityIds??seed.flatMap(s=>s.kind==="visual"&&s.featureId===feature.profile&&sourceSketch?.entities.some(e=>e.id===s.entityId)?[s.entityId]:[]));
  const [editingPattern,setEditingPattern]=useState<string|undefined>(existingPattern?.id);
  const [draft,setDraft]=useState<Feature>(()=>{
    if(internal&&!existingPattern)return {...feature,pattern:{...sketchPatternDefaultReferences(feature.profile!,feature.pattern!.kind),...feature.pattern!}};
    return feature;
  });
  const [role,setRoleState]=useState<FeaturePickRole|undefined>(internal?"geometry":feature.type==="SKETCH_PATTERN"?"profile":"body");
  const boundSlot=(slot:string)=>boundContextParameter(view,view.part?.parameters?.find(p=>p.ownerFeatureId===feature.id&&p.propertySlot===`pattern:${slot}`)?.parameterId);
  const [inputArtifacts,setInputArtifacts]=useState<Artifact[]>();
  const [replacingSeed,setReplacingSeed]=useState<string>();
  const [pickGeneration,setPickGeneration]=useState(0);
  const setRole=(next:FeaturePickRole|undefined)=>{if(next!=="body")setReplacingSeed(undefined);planeGeneration.current++;setBinding(0);setRoleState(next);setPickGeneration(value=>value+1);};
  const activatePick=setRole;
  const cancelPick=()=>{if(!role)return false;setReplacingSeed(undefined);setRole(undefined);return true};
  const [error,setError]=useState<string|undefined>(undefined),[busy,setBusy]=useState(false);
  const intent=useRef(randomUUID()),base=useRef(view.document.versionId);
  const pattern=draft.pattern!;
 const mirror=pattern.kind==="MIRROR";
 const caps=view.replicationSources??{};
 const [planeVisual,setPlaneVisual]=useState<SelectionItem|undefined>(()=>{
   const plane=view.part?.datumPlanes.find(p=>p.id===pattern.mirrorPlaneId);
   return plane?{...occurrenceContext,kind:"plane",id:plane.id,entityId:plane.id,plane:plane.plane,datumPlane:plane,documentId:view.document.id,versionId:base.current,occurrencePath}:undefined;
 });
 const [binding,setBinding]=useState(0);const planeGeneration=useRef(0),alive=useRef(true),currentRevision=useRef(view.document.versionId);currentRevision.current=view.document.versionId;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;planeGeneration.current++;};},[]);

  const update=(patch:Partial<NonNullable<Feature["pattern"]>>)=>setDraft(f=>({...f,pattern:{...f.pattern!,...patch}}));
  const initial=(slot:string,value:number)=>{
    const parameter=view.part?.parameters?.find(p=>p.ownerFeatureId===(internal?feature.profile:feature.id)&&p.propertySlot===(internal?`pattern:${existingPattern?.id}:${slot}`:`pattern:${slot}`));
    return parameter?parameterSourceText(parameter,slot==="count"?"1":slot==="spacing"?unit:"deg",true):String(value);
  };
  const [text,setText]=useState({count:initial("count",pattern.count),spacing:initial("spacing",pattern.spacing??10),angle:initial("angle",pattern.angle??90),phase:initial("phase",pattern.phase??0)});
  const original=useRef({...text});
  const [skipped,setSkipped]=useState((pattern.skippedSlots??[]).join(","));
  type Seed=NonNullable<Feature["pattern"]>["additionalSources"] extends Array<infer T>|undefined?T:never;
  const seeds:Seed[]=pattern.source.featureId?[{source:pattern.source,sourceKind:pattern.sourceKind as Seed['sourceKind'],startFeatureId:pattern.startFeatureId},...(pattern.additionalSources??[])]:[];
  const setSeeds=(items:Seed[])=>setDraft(f=>{const first=items[0],source=upstream.find(s=>s.id===first?.source.featureId),operation=first?.sourceKind==='BODY_STAGE'?'ADD':caps[first?.source.featureId??'']?.operation??f.operation;
    return {...f,type:source?.sketch?'SKETCH_PATTERN':'SOLID_PATTERN',bodyId:feature.id?f.bodyId:source?.bodyId??f.bodyId,operation,pattern:{...f.pattern!,source:first?.source??{bodyId:f.bodyId??'',featureId:''},sourceKind:first?.sourceKind??'AUTO',startFeatureId:first?.startFeatureId,additionalSources:items.slice(1),resultMode:operation==='REMOVE'?'COMBINE':f.pattern!.resultMode}};
  });
  const selectSource=(value:string)=>{
    const body=value.startsWith('body:'),id=body?value.slice(5):value,source=upstream.find(f=>f.id===id);if(!source?.bodyId)return;
    const capability=caps[id];if(!body&&!capability?.sourceKind)return;
    const retained=replacingSeed?seeds.filter(s=>s.source.featureId!==replacingSeed):seeds;
    const existing=retained.find(s=>s.source.featureId===id);
    if(existing){if(replacingSeed){setError("此种子已在列表中");return;}setSeeds(seeds.filter(s=>s!==existing));setError(undefined);return;}
    if(retained.length&&(source.bodyId!==retained[0].source.bodyId||(body?'ADD':capability.operation??'ADD')!==(draft.operation??'ADD'))){setError('种子必须属于同一实体，并具有一致的添加或切除语义');return;}
    if(retained.length&&(source.sketch||draft.type==='SKETCH_PATTERN')){setError('空间草图阵列一次选择一个草图；可在草图内阵列多项几何');return;}
    if(retained.length>=32){setError('最多选择 32 个种子');return;}
    const next=[...retained,{source:{bodyId:source.bodyId,featureId:id},sourceKind:body?'BODY_STAGE' as const:'AUTO' as const}];
    next.sort((a,b)=>upstream.findIndex(f=>f.id===a.source.featureId)-upstream.findIndex(f=>f.id===b.source.featureId));
    setSeeds(next);setError(undefined);if(replacingSeed){setReplacingSeed(undefined);setRole(undefined);}
  };
  const pick=(selection:SelectionItem)=>{
    if(!role||(selection.ownerDocumentId??selection.documentId)!==view.document.id||(selection.occurrencePath??"")!==(occurrencePath??"")||selection.versionId&&selection.versionId!==base.current||busy||base.current!==view.document.versionId)return;
    if(role==="plane" && mirror) {
      if(selection.kind==="plane") {
        planeGeneration.current++; const plane=view.part?.datumPlanes.find(p=>p.id===(selection.entityId??selection.id));
        if(plane){update({mirrorPlaneId:plane.id,mirrorPlane:undefined});setPlaneVisual(selection);setError(undefined);setRole(undefined);}return;
      }
      if(selection.kind!=="face"||!selection.geometryKey)return;
      const generation=++planeGeneration.current;setBinding(n=>n+1);
      void (async()=>{
        const definition=feature.id
          ? (await api.getFeatureInput(view.document.id,{versionId:base.current,featureId:feature.id,bodyId:selection.bodyId,geometryKey:selection.geometryKey,kind:"FACE",localId:selection.topologyId})).selection
          : await api.getTopologyProperties(view.document.id,selection.geometryKey!,"FACE",selection.topologyId,base.current).then(p=>p.persistentSelection?{selection:p.persistentSelection,sourceVersionId:base.current}:undefined);
        if(!definition||definition.selection.creationEvidence.geometryType!=="PLANE")throw new CommandInputError("请选择可绑定的实体平面");
        if(alive.current&&base.current===currentRevision.current&&generation===planeGeneration.current){update({mirrorPlaneId:undefined,mirrorPlane:definition});setPlaneVisual(selection);setError(undefined);setRole(undefined);}
      })().catch(cause=>{if(alive.current&&base.current===currentRevision.current&&generation===planeGeneration.current)setError(engineeringProblem(cause));}).finally(()=>{if(alive.current&&generation===planeGeneration.current)setBinding(n=>Math.max(0,n-1));});return;
    }
    if(role==="point") {if(internal&&(selection.kind!=="visual"||selection.featureId!==feature.profile))return;const centerReference=pickedPatternCenter(selection,view);if(centerReference){update({centerReference});setError(undefined);setRole(undefined);}else setError("请选择原点、草图点、端点或圆心");return;}
    if(internal&&role==="axis") {
      if(selection.kind==="visual"&&selection.featureId===feature.profile&&selection.sketchReference){update({directionReference:selection.sketchReference,axisEntityId:undefined});setError(undefined);setRole(undefined);}else setError("请选择当前草图的 U/V 轴、直线或投影直线");return;
    }
    if(internal) {
      if(selection.kind==="visual"&&selection.featureId===feature.profile&&sourceSketch?.entities.some(e=>e.id===selection.entityId)){
       if(replacingSeed){if(entityIds.includes(selection.entityId)&&selection.entityId!==replacingSeed){setError('此几何已在种子集合中');return;}setEntityIds(ids=>ids.map(id=>id===replacingSeed?selection.entityId:id));setRole(undefined)}
       else setEntityIds(ids=>ids.includes(selection.entityId)?ids.filter(id=>id!==selection.entityId):[...ids,selection.entityId]);
      }
      return;
    }
    if(role==="axis") {
      const axisEntityId=selectedAxis(view,selection,upstream);if(axisEntityId){update({axisEntityId});setRole(undefined);}
      if(selection.kind!=="axis")return;
      const axis=view.part?.datumAxes?.find(a=>a.id===selection.entityId||selection.id.endsWith(a.id));
      if(axis){update({origin:axis.origin,direction:axis.direction});return;}
      const system=view.part?.axisSystems.find(a=>a.id===selection.entityId||selection.id.includes(a.id));
      if(system&&selection.axis!=="DATUM")update({origin:system.origin,direction:selection.axis==="X"?system.xDirection:selection.axis==="Y"?system.yDirection:system.zDirection});
    } else if(role==="profile") {const sketch=selectedSketch(view,selection,upstream);if(sketch){selectSource(sketch.id);setRole(undefined);}}
    else {
      if(selection.kind==='body'){const stages=upstream.filter(f=>f.bodyId===selection.bodyId&&caps[f.id]?.bodyStage);const stage=selection.displayStageFeatureId?stages.find(f=>f.id===selection.displayStageFeatureId):stages.at(-1);if(stage){selectSource('body:'+stage.id)}return;}
      const sketch=selectedSketch(view,selection,upstream);if(sketch&&!mirror&&caps[sketch.id]?.sourceKind){selectSource(sketch.id);setError(undefined);setRole(undefined);return;}
      const candidates=patternSourceCandidates(selection,upstream,"AUTO",caps);
      if(candidates.length===1){selectSource(candidates[0].id);}
      else setError(candidates.length?`请选择种子：${candidates.map(f=>f.name??"未命名特征").join("、")}`:"此选择没有可复制的特征工具，请从结构树选择合法的上游特征");
    }
  };
  useEffect(()=>{
    if(!feature.id||feature.type!=="SOLID_PATTERN")return;
    const controller=new AbortController();let current=true;
    void api.getFeatureInput(view.document.id,{versionId:base.current,featureId:feature.id},controller.signal).then(result=>{if(current){setInputArtifacts(result.artifacts??[result.artifact]);onInputArtifact(result.artifacts??result.artifact);if(result.neutralPick?.status==="RESOLVED"&&result.neutralPick.localId){const p=result.neutralPick;setPlaneVisual({...occurrenceContext,kind:"face",id:`input:mirror:${p.localId}`,documentId:view.document.id,bodyId:p.bodyId,versionId:result.versionId,geometryKey:p.geometryKey,displayStageFeatureId:p.displayStageFeatureId,topologyId:p.localId!,occurrencePath});}}}).catch(cause=>{if(current&&!controller.signal.aborted)setError(engineeringProblem(cause));});
    return ()=>{current=false;controller.abort();onInputArtifact();};
  },[]);
  const pickRef=useRef(pick);pickRef.current=pick;
  const seedPicks=seeds.map(s=>{if(upstream.find(f=>f.id===s.source.featureId)?.sketch)return {...sketchPick(view,s.source.featureId,occurrencePath),...occurrenceContext};const body=view.part?.bodies.find(b=>b.id===s.source.bodyId);const artifact=feature.id?inputArtifacts?.find(a=>a.bodyId===s.source.bodyId):body?.geometryKey?view.artifacts?.[body.geometryKey]:undefined;return {...occurrenceContext,kind:'feature' as const,id:s.source.featureId,entityId:s.source.featureId,bodyId:s.source.bodyId,documentId:view.document.id,versionId:base.current,geometryKey:artifact?.geometryKey,displayStageFeatureId:artifact?.displayStageFeatureId,occurrencePath}});
  const axisPick=pattern.directionReference?patternReferencePick(view,feature.profile!,pattern.directionReference,occurrencePath,occurrenceContext):pattern.axisEntityId?patternAxisPick(view,pattern.axisEntityId,occurrencePath,occurrenceContext):undefined;
  const centerPick=pattern.centerReference?.axisEntityId?patternAxisPick(view,pattern.centerReference.axisEntityId,occurrencePath,occurrenceContext,true):pattern.centerReference?.sketchId?patternReferencePick(view,pattern.centerReference.sketchId,pattern.centerReference.reference,occurrencePath,occurrenceContext):undefined;
  const axisName=pattern.directionReference?patternReferenceName(view,feature.profile!,pattern.directionReference):pattern.axisEntityId?patternReferenceName(view,pattern.axisEntityId):'未选择';
  const centerName=pattern.centerReference?.axisEntityId?`${objectName(view,pattern.centerReference.axisEntityId.split(':')[1])} / 原点`:pattern.centerReference?.sketchId?patternReferenceName(view,pattern.centerReference.sketchId,pattern.centerReference.reference):'所选轴的原点';

  const referenceSelections:SelectionItem[]=internal?[...entityIds.map(entityId=>({target:"ENTITY" as const,entityId,subElement:"WHOLE" as const})),pattern.centerReference?.reference,pattern.directionReference].filter((r):r is NonNullable<typeof r>=>!!r).map(r=>({kind:"visual",id:`pattern-reference:${r.target}:${r.entityId??""}:${r.subElement}`,featureId:feature.profile!,entityId:r.entityId??r.target,visualType:r.subElement==="DIRECTION"?"CURVE":"POINT",sketchReference:r,documentId:view.document.id,versionId:base.current,occurrencePath})):[...seedPicks,...(axisPick?[axisPick]:[]),...(centerPick?[centerPick]:[]),...(mirror&&planeVisual?[planeVisual]:[])];
  const referenceToken=JSON.stringify(referenceSelections);
  useEffect(()=>{onSelectionSession(role?{...occurrenceContext,role,documentId:view.document.id,versionId:base.current,occurrencePath,sketchIds:internal?[feature.profile!]:undefined,localSketchId:internal?feature.profile:undefined,selections:referenceSelections,accept:role==='body'&&!internal?s=>{
 if(occurrenceContext?.contextVariantKey!==undefined&&s.contextVariantKey!==occurrenceContext.contextVariantKey)return false;
 if(occurrenceContext?.rootDocumentId&&(s.instancePath?.rootDocumentId??s.rootDocumentId)!==occurrenceContext.rootDocumentId)return false;
 if(feature.id&&'bodyId' in s&&s.bodyId&&s.bodyId!==draft.bodyId)return false;
 if((s.ownerDocumentId??s.documentId)!==view.document.id||(s.occurrencePath??'')!==(occurrencePath??'')||s.versionId&&s.versionId!==base.current)return false;
 if(s.kind==='body')return upstream.some(f=>f.bodyId===s.bodyId&&caps[f.id]?.bodyStage);
 const sketch=selectedSketch(view,s,upstream);return !!(!mirror&&sketch&&caps[sketch.id]?.sourceKind)||patternSourceCandidates(s,upstream,'AUTO',caps).length>0;
 }:undefined,onPick:s=>pickRef.current(s)}:undefined);return()=>onSelectionSession();},[role,view.document.id,occurrencePath,referenceToken,pickGeneration]);
  useEffect(()=>{
   if(internal||feature.id||initialRange.status!=="NONE")return;
   const candidates=seed.map(s=>patternSourceCandidates(s,upstream,'AUTO',caps));
   if(candidates.some(items=>items.length>1)){setError('所选几何关联多个特征，请从结构树明确选择种子');return;}
   const selected=candidates.flat();
   const unique=upstream.filter(f=>selected.some(s=>s.id===f.id));
   if(unique.length&&unique.every(f=>!f.sketch&&f.bodyId===unique[0].bodyId&&caps[f.id]?.operation===caps[unique[0].id]?.operation))setSeeds(unique.map(f=>({source:{bodyId:f.bodyId!,featureId:f.id},sourceKind:'AUTO'})));
   else if(unique.length>1)setError('种子必须属于同一实体，并具有一致的添加或切除语义');
   else if(seed[0])pickRef.current(seed[0]);
  },[]);
  let input:Record<string,unknown>|undefined,parameterError:string|undefined;
  try {
    const candidate={...pattern},expressions:Record<string,string>={};
    const slots:Array<keyof typeof text>=mirror?[]:pattern.kind==="LINEAR"?["count","spacing"]:pattern.distribution==="FULL_CIRCLE"?["count","phase"]:["count","angle","phase"];
    for(const slot of slots) {
      if((feature.id||editingPattern)&&text[slot]===original.current[slot])continue;
      const value=parseParameterSource(text[slot],slot==="count"?"1":slot==="spacing"?unit:"deg");
      if(value.kind==="EXPRESSION"){if(!value.expression)throw new CommandInputError("请输入数值或表达式");expressions[`pattern:${slot}`]=value.expression;continue;}
      const scale=slot==="spacing"?({mm:1,cm:10,m:1000,in:25.4} as Record<string,number>)[value.unit]:slot==="count"?(value.unit==="1"?1:undefined):({deg:1,rad:180/Math.PI} as Record<string,number>)[value.unit];
      if(scale===undefined||!Number.isFinite(value.value))throw new CommandInputError("参数单位不正确");
      candidate[slot]=value.value*scale;
    }
    if(!Number.isInteger(candidate.count)||candidate.count<1||candidate.count>256)throw new CommandInputError("数量应为 1–256 的整数（包含种子）");
    candidate.skippedSlots=skipped.trim()?skipped.split(",").map(value=>{if(!/^\d+$/.test(value.trim()))throw new CommandInputError("跳过槽位应为整数，以逗号分隔");return Number(value);}):[];
    if(internal&&candidate.kind==="CIRCULAR"&&!candidate.centerReference)throw new CommandInputError("请选择阵列中心点");
    if(candidate.kind==="LINEAR"&&!candidate.axisEntityId&&!candidate.directionReference)throw new CommandInputError("请选择阵列方向");
    if(candidate.sourceKind==="FEATURE_DELTA"&&!patternRangeEnds(upstream,candidate.startFeatureId,caps).some(f=>f.id===candidate.source.featureId))throw new CommandInputError("请选择新增材料范围的起始特征和结束步骤");
    if(mirror&&!candidate.mirrorPlaneId&&!candidate.mirrorPlane)throw new CommandInputError("请选择镜像平面");
    if(candidate.source.featureId&&!binding&&base.current===view.document.versionId) {
      input={requestId:intent.current,parameterExpressions:expressions,feature:{...draft,pattern:candidate},...(feature.id?{type:"EDIT_FEATURE",targetId:feature.id,expectedFeatureDigest:digest}:{type:"CREATE_PATTERN"})};
      if(internal) input=entityIds.length?{requestId:intent.current,type:"EDIT_SKETCH",sketchId:feature.profile,operations:[{type:editingPattern?"EDIT_PATTERN":"CREATE_PATTERN",patternId:editingPattern,pattern:{...candidate,id:editingPattern??`pattern-${intent.current}`,entityIds},patternParameterExpressions:Object.fromEntries(Object.entries(expressions).map(([key,value])=>[key.replace("pattern:",""),value]))}]}:undefined;
    }
  } catch(cause){parameterError=fieldErrorText(cause);}
  const preview=useFeaturePreview(view.document.id,view.document.versionId,input,draft.operation,(artifact,operation)=>onPreview(artifact,operation,internal?feature.profile:draft.type==="SKETCH_PATTERN"?feature.id:undefined));
  const apply=async()=>{if(!input||!preview.previewId||preview.pending||busy)return;setBusy(true);try{await onApply({...input,previewId:preview.previewId});onClose();}catch(cause){setError(engineeringProblem(cause));setBusy(false);}};
  const removePattern=async(type:"DELETE_PATTERN"|"DETACH_PATTERN")=>{if(busy||!editingPattern)return;setBusy(true);try{await onApply({type:"EDIT_SKETCH",requestId:randomUUID(),sketchId:feature.profile,operations:[{type,patternId:editingPattern}]});onClose();}catch(cause){setError(engineeringProblem(cause));setBusy(false);}};

  return <CommandDialog id="pattern-feature-editor" open title={`${(feature.id||editingPattern)?"编辑":"创建"}${mirror?"镜像":"阵列"}`} size="S" onClose={onClose} onCancelPick={cancelPick} commandStatus={<CommandStatus pending={!!binding||preview.pending} error={preview.error} onRetry={preview.retry}/>} onConfirm={apply} confirmLoading={busy} confirmDisabled={!preview.previewId||preview.pending||busy||!input}>
    {!!preview.repairRequired?.length&&<Alert type="warning" title={`下游需要修复：${preview.repairRequired.map(f=>f.name||"引用特征").join("、")}`}/>}
    <fieldset disabled={busy} className="instance-pattern-fields feature-input-fields">
      {!mirror&&<CommandField label="方式" width="content"><Segmented aria-label="阵列方式" value={pattern.kind} options={[{value:'LINEAR',label:'线性'},{value:'CIRCULAR',label:'圆周'}]} onChange={kind=>{update({...(internal||draft.type==='SKETCH_PATTERN'?sketchPatternDefaultReferences(feature.profile??'',kind as 'LINEAR'|'CIRCULAR'):{directionReference:undefined,centerReference:undefined}),kind:kind as 'LINEAR'|'CIRCULAR',distribution:kind==='CIRCULAR'?'FULL_CIRCLE':'FIXED_STEP',axisEntityId:internal?undefined:`AXIS_SYSTEM:${view.part?.axisSystems[0]?.id??'axis-system-default'}:${kind==='CIRCULAR'?'Z':'X'}`});setRole(undefined)}}/></CommandField>}

      {internal&&<>
        <Select placeholder="新建阵列" value={editingPattern} allowClear options={sourceSketch?.patterns?.map(p=>({value:p.id,label:`${p.kind==="CIRCULAR"?"圆周":"线性"}阵列 · ${p.count}`}))} onChange={id=>{setEditingPattern(id);const p=sourceSketch?.patterns?.find(p=>p.id===id);if(p){update(p);setEntityIds(p.entityIds);setSkipped((p.skippedSlots??[]).join(","));const fields={count:String(p.count),spacing:String(p.spacing??10),angle:String(p.angle??90),phase:String(p.phase??0)};for(const slot of ["count","spacing","angle","phase"] as const){const parameter=view.part?.parameters?.find(value=>value.ownerFeatureId===feature.profile&&value.propertySlot===`pattern:${p.id}:${slot}`);if(parameter)fields[slot]=parameterSourceText(parameter,slot==="count"?"1":slot==="spacing"?unit:"deg",true);}original.current=fields;setText(fields);}}}/>
        <FeaturePickField mode="multiple" label="种子几何" active={role==='geometry'&&!replacingSeed} onActivate={()=>{setReplacingSeed(undefined);activatePick('geometry')}} onClear={entityIds.length?()=>setEntityIds([]):undefined} items={entityIds.map((id,i)=>({key:id,actionLabel:`种子几何 ${i+1}`,value:sourceSketch?.entities.find(e=>e.id===id)?.name??'草图几何',active:role==='geometry'&&replacingSeed===id,onActivate:()=>{activatePick('geometry');setReplacingSeed(id)},selections:referenceSelections.filter(s=>s.entityId===id),onClear:()=>setEntityIds(ids=>ids.filter(value=>value!==id))}))}/>

        <CommandField  label={<>抑制</>} width="option"><Switch checked={!!pattern.suppressed} onChange={suppressed=>update({suppressed})}/></CommandField>
        {editingPattern&&<><Button onClick={()=>void removePattern("DELETE_PATTERN")}>删除阵列</Button><Button onClick={()=>void removePattern("DETACH_PATTERN")}>解除关联</Button></>}
      </>}
      {!internal&&<>
      <FeaturePickField label="种子" mode="multiple" active={role==="body"&&!replacingSeed||role==="profile"} onActivate={()=>{setReplacingSeed(undefined);activatePick(draft.type==="SKETCH_PATTERN"?"profile":"body")}} onClear={seeds.length?()=>setSeeds([]):undefined} items={seeds.map((s,i)=>({
       key:s.source.featureId,actionLabel:`种子 ${i+1}`,value:objectName(view,s.source.featureId),detail:s.startFeatureId?`${objectName(view,s.startFeatureId)} → ${objectName(view,s.source.featureId)}`:s.sourceKind==='BODY_STAGE'?'实体阶段':undefined,active:role==='body'&&replacingSeed===s.source.featureId,
       onActivate:()=>{setReplacingSeed(s.source.featureId);activatePick('body')},selections:seedPicks[i]&&(seedPicks[i].kind==='sketch'||seedPicks[i].geometryKey)?[seedPicks[i]]:[],onClear:()=>setSeeds(seeds.filter((_,j)=>j!==i))
      }))}/>
      {mirror?<FeaturePickField label="镜像平面" value={pattern.mirrorPlane?(planeVisual?selectionName(view,planeVisual):"引用平面暂不可定位"):view.part?.datumPlanes.find(p=>p.id===pattern.mirrorPlaneId)?.name??"未选择"} selected={!!(pattern.mirrorPlaneId||pattern.mirrorPlane)} selections={planeVisual?[planeVisual]:[]} onClear={pattern.mirrorPlane||pattern.mirrorPlaneId?()=>{update({mirrorPlane:undefined,mirrorPlaneId:undefined});setPlaneVisual(undefined);}:undefined} active={role==="plane"} onActivate={()=>activatePick("plane")}/>:<FeaturePickField label={pattern.kind==="CIRCULAR"?"轴":"方向"} value={axisName} selections={axisPick?[axisPick]:[]} selected={!!(pattern.axisEntityId||pattern.directionReference)} active={role==="axis"} onActivate={()=>activatePick("axis")} onClear={pattern.axisEntityId||pattern.directionReference?()=>update({axisEntityId:undefined,directionReference:undefined}):undefined}/>}
      </>}
      {!internal&&draft.type==="SOLID_PATTERN"&&<CommandField label="材料" width="content"><Segmented aria-label="材料" value={draft.operation==="REMOVE"?"REMOVE":pattern.resultMode??"COMBINE"} options={draft.operation==="REMOVE"?[{value:"REMOVE",label:"切除"}]:[{value:"COMBINE",label:"添加"},{value:"INDEPENDENT",label:"独立实体"}]} onChange={value=>{if(value!=="REMOVE")update({resultMode:value as "COMBINE"|"INDEPENDENT"})}}/></CommandField>}
      {internal&&pattern.kind==="LINEAR"&&<FeaturePickField label="方向" value={axisName} selections={axisPick?[axisPick]:[]} onClear={pattern.directionReference?()=>update({directionReference:undefined,axisEntityId:undefined}):undefined} selected={!!(pattern.axisEntityId||pattern.directionReference)} active={role==="axis"} onActivate={()=>activatePick("axis")}/>}
      {pattern.kind==="CIRCULAR"&&<FeaturePickField label="中心点" value={centerName} selections={centerPick?[centerPick]:[]} selected={!!pattern.centerReference} active={role==="point"} onActivate={()=>activatePick("point")} onClear={pattern.centerReference?()=>update({centerReference:undefined}):undefined}/>}
      {!mirror&&<>
      <Select value={pattern.distribution} options={[{value:"FIXED_STEP",label:"固定步距"},{value:"TOTAL_SPAN",label:"总跨度均布"},...(pattern.kind==="CIRCULAR"?[{value:"FULL_CIRCLE",label:"整圆均布"}]:[])]} onChange={value=>update({distribution:value as PatternDefinition["distribution"]})}/>
      {(["count",pattern.kind==="LINEAR"?"spacing":pattern.distribution==="FULL_CIRCLE"?null:"angle",(internal||draft.type==="SKETCH_PATTERN")&&pattern.kind==="CIRCULAR"?"phase":null] as const).filter(x=>x!==null).map(slot=><CommandField actions={<ContextInputSource binding={boundSlot(slot)}/>} key={slot} label={<>{({count:"数量",spacing:`间距 / 跨度 (${unit})`,angle:"角度 (deg)",phase:"相位 (deg)"})[slot]}</>} width="expression"><Input disabled={!!boundSlot(slot)} value={text[slot]} onChange={e=>setText(t=>({...t,[slot]:e.target.value}))}/></CommandField>)}
      <CommandField  label={<>跳过槽位（从 0 开始）</>} width="expression"><Input value={skipped} onChange={e=>setSkipped(e.target.value)}/></CommandField>
      <CommandField label="反向" width="option"><Switch aria-label="反向" checked={!!pattern.reversed} onChange={reversed=>update({reversed})}/></CommandField></>}
    </fieldset>
    {(parameterError||error)&&<Alert type="error" title={parameterError??error}/>}

</CommandDialog>;
}
