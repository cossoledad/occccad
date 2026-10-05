import type { FeaturePickRole } from "../../cad/interaction/feature-selection";
import { Alert, Button, Input, Select, Switch } from "antd";
import { useEffect, useRef, useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import type { Feature, PatternDefinition, SelectionItem } from "../../types";
import type { SolidFeatureEditor } from "./solid-feature-editor";
import { FeaturePickField } from "./feature-pick-field";
import { patternRangeFromSelections, patternRangeEnds, patternSourceCandidates, selectedSketch, selectedAxis } from "./feature-picking";
import { parameterSourceText, parseParameterSource } from "./parameter-editor";
import { useFeaturePreview } from "./use-feature-preview";
import { pickedPatternCenter, sketchPatternDefaultReferences } from "./pattern-selection";
import { api } from "../../api/client";
import { randomUUID } from "../../utils/random-uuid";

export function PatternFeatureEditor({ view, feature, digest, unit, seed, occurrencePath, occurrenceContext, onClose, onApply, onPreview, onSelectionSession, onInputArtifact }: Parameters<typeof SolidFeatureEditor>[0]) {
  const internal=feature.type==="SKETCH";
  const upstream=view.part!.features.slice(0,feature.id?view.part!.features.findIndex(f=>f.id===feature.id):undefined);
  const [initialRange]=useState(()=>!internal&&!feature.id?patternRangeFromSelections(view,seed,upstream,occurrencePath):{status:"NONE" as const});
  const sourceSketch=view.part?.features.find(f=>f.id===feature.profile)?.sketch;
  const existingPattern=sourceSketch?.patterns?.find(p=>p.id===feature.pattern?.id);
  const [entityIds,setEntityIds]=useState<string[]>(existingPattern?.entityIds??seed.flatMap(s=>s.kind==="visual"&&s.featureId===feature.profile&&sourceSketch?.entities.some(e=>e.id===s.entityId)?[s.entityId]:[]));
  const [editingPattern,setEditingPattern]=useState<string|undefined>(existingPattern?.id);
  const [draft,setDraft]=useState<Feature>(()=>{
    if(internal&&!existingPattern)return {...feature,pattern:{...sketchPatternDefaultReferences(feature.profile!,feature.pattern!.kind),...feature.pattern!}};
    if(initialRange.status==="READY")return {...feature,bodyId:initialRange.bodyId,operation:"ADD",pattern:{...feature.pattern!,sourceKind:"FEATURE_DELTA" as const,startFeatureId:initialRange.startFeatureId,source:{bodyId:initialRange.bodyId,featureId:initialRange.endFeatureId}}};
    return feature;
  });
  const [role,setRole]=useState<FeaturePickRole>(internal?"geometry":feature.type==="SKETCH_PATTERN"?"profile":"body");
  const [pickGeneration,setPickGeneration]=useState(0);
  const activatePick=(next:FeaturePickRole)=>{setRole(next);setPickGeneration(value=>value+1);};
  const [error,setError]=useState<string|undefined>(initialRange.status==="INVALID"?"所选特征不能组成同一实例中的拉伸/旋转及圆角、倒角组合，请明确选择起始和结束特征":undefined),[busy,setBusy]=useState(false);
  const intent=useRef(randomUUID()),base=useRef(view.document.versionId);
  const pattern=draft.pattern!;

  const update=(patch:Partial<NonNullable<Feature["pattern"]>>)=>setDraft(f=>({...f,pattern:{...f.pattern!,...patch}}));
  const initial=(slot:string,value:number)=>{
    const parameter=view.part?.parameters?.find(p=>p.ownerFeatureId===(internal?feature.profile:feature.id)&&p.propertySlot===(internal?`pattern:${existingPattern?.id}:${slot}`:`pattern:${slot}`));
    return parameter?parameterSourceText(parameter,slot==="count"?"1":slot==="spacing"?unit:"deg",true):String(value);
  };
  const [text,setText]=useState({count:initial("count",pattern.count),spacing:initial("spacing",pattern.spacing??10),angle:initial("angle",pattern.angle??90),phase:initial("phase",pattern.phase??0)});
  const original=useRef({...text});
  const [skipped,setSkipped]=useState((pattern.skippedSlots??[]).join(","));
  const selectSource=(id:string)=>{
    const source=upstream.find(f=>f.id===id);if(!source?.bodyId)return;
    setDraft(f=>({...f,bodyId:feature.id?f.bodyId:source.bodyId,operation:f.pattern!.sourceKind!=="FEATURE_DELTA"&&source.operation==="REMOVE"?"REMOVE":"ADD",pattern:{...f.pattern!,resultMode:source.operation==="REMOVE"?"COMBINE":f.pattern!.resultMode,source:{bodyId:source.bodyId!,featureId:id}}}));
  };
  const pick=(selection:SelectionItem)=>{
    if((selection.ownerDocumentId??selection.documentId)!==view.document.id||(selection.occurrencePath??"")!==(occurrencePath??"")||selection.versionId&&selection.versionId!==base.current||busy||base.current!==view.document.versionId)return;
    if(role==="point") {if(internal&&(selection.kind!=="visual"||selection.featureId!==feature.profile))return;const centerReference=pickedPatternCenter(selection);if(centerReference){update({centerReference});setError(undefined);}else setError("请选择原点、草图点、端点或圆心");return;}
    if(internal&&role==="axis") {
      if(selection.kind==="visual"&&selection.featureId===feature.profile&&selection.sketchReference){update({directionReference:selection.sketchReference,axisEntityId:undefined});setError(undefined);}else setError("请选择当前草图的 U/V 轴、直线或投影直线");return;
    }
    if(internal) {
      if(selection.kind==="visual"&&selection.featureId===feature.profile&&sourceSketch?.entities.some(e=>e.id===selection.entityId))setEntityIds(ids=>ids.includes(selection.entityId)?ids.filter(id=>id!==selection.entityId):[...ids,selection.entityId]);
      return;
    }
    if(role==="axis") {
      const axisEntityId=selectedAxis(view,selection,upstream);if(axisEntityId)update({axisEntityId});
      if(selection.kind!=="axis")return;
      const axis=view.part?.datumAxes?.find(a=>a.id===selection.entityId||selection.id.endsWith(a.id));
      if(axis){update({origin:axis.origin,direction:axis.direction});return;}
      const system=view.part?.axisSystems.find(a=>a.id===selection.entityId||selection.id.includes(a.id));
      if(system&&selection.axis!=="DATUM")update({origin:system.origin,direction:selection.axis==="X"?system.xDirection:selection.axis==="Y"?system.yDirection:system.zDirection});
    } else if(role==="profile") {const sketch=selectedSketch(view,selection,upstream);if(sketch)selectSource(sketch.id);}
    else {
      const candidates=patternSourceCandidates(selection,pattern.sourceKind==="FEATURE_DELTA"?patternRangeEnds(upstream,pattern.startFeatureId):upstream,pattern.sourceKind==="FEATURE_DELTA"?"FEATURE_DELTA":pattern.sourceKind==="GENERATOR_TOOL"?"GENERATOR_TOOL":"BODY_STAGE");
      if(candidates.length===1){selectSource(candidates[0].id);setError(undefined);}
      else setError(candidates.length?`请选择种子：${candidates.map(f=>f.name??f.id).join("、")}`:"此选择没有可用的种子工具；请从种子列表选择明确的上游定义");
    }
  };
  useEffect(()=>{
    if(!feature.id||feature.type!=="SOLID_PATTERN")return;
    const controller=new AbortController();let current=true;
    void api.getFeatureInput(view.document.id,{versionId:base.current,featureId:feature.id},controller.signal).then(result=>{if(current)onInputArtifact(result.artifact);}).catch(cause=>{if(current&&!controller.signal.aborted)setError(String(cause));});
    return ()=>{current=false;controller.abort();onInputArtifact();};
  },[]);
  const pickRef=useRef(pick);pickRef.current=pick;
  const referenceSelections:SelectionItem[]=internal?[...entityIds.map(entityId=>({target:"ENTITY" as const,entityId,subElement:"WHOLE" as const})),pattern.centerReference?.reference,pattern.directionReference].filter((r):r is NonNullable<typeof r>=>!!r).map(r=>({kind:"visual",id:`pattern-reference:${r.target}:${r.entityId??""}:${r.subElement}`,featureId:feature.profile!,entityId:r.entityId??r.target,visualType:r.subElement==="DIRECTION"?"CURVE":"POINT",sketchReference:r,documentId:view.document.id,versionId:base.current,occurrencePath})):[];
  const referenceToken=JSON.stringify(referenceSelections);
  useEffect(()=>{onSelectionSession({...occurrenceContext,role,documentId:view.document.id,versionId:base.current,occurrencePath,sketchIds:internal?[feature.profile!]:undefined,localSketchId:internal?feature.profile:undefined,selections:referenceSelections,onPick:s=>pickRef.current(s)});return()=>onSelectionSession();},[role,view.document.id,occurrencePath,referenceToken,pickGeneration]);
  useEffect(()=>{if(!internal&&!feature.id&&initialRange.status==="NONE"&&seed[0])pickRef.current(seed[0]);},[]);
  let input:Record<string,unknown>|undefined,parameterError:string|undefined;
  try {
    const candidate={...pattern},expressions:Record<string,string>={};
    const slots:Array<keyof typeof text>=pattern.kind==="LINEAR"?["count","spacing"]:pattern.distribution==="FULL_CIRCLE"?["count","phase"]:["count","angle","phase"];
    for(const slot of slots) {
      if((feature.id||editingPattern)&&text[slot]===original.current[slot])continue;
      const value=parseParameterSource(text[slot],slot==="count"?"1":slot==="spacing"?unit:"deg");
      if(value.kind==="EXPRESSION"){if(!value.expression)throw Error("请输入数值或表达式");expressions[`pattern:${slot}`]=value.expression;continue;}
      const scale=slot==="spacing"?({mm:1,cm:10,m:1000,in:25.4} as Record<string,number>)[value.unit]:slot==="count"?(value.unit==="1"?1:undefined):({deg:1,rad:180/Math.PI} as Record<string,number>)[value.unit];
      if(scale===undefined||!Number.isFinite(value.value))throw Error("参数单位不正确");
      candidate[slot]=value.value*scale;
    }
    if(!Number.isInteger(candidate.count)||candidate.count<1||candidate.count>256)throw Error("数量应为 1–256 的整数（包含种子）");
    candidate.skippedSlots=skipped.trim()?skipped.split(",").map(value=>{if(!/^\d+$/.test(value.trim()))throw Error("跳过槽位应为整数，以逗号分隔");return Number(value);}):[];
    if(internal&&candidate.kind==="CIRCULAR"&&!candidate.centerReference)throw Error("请选择阵列中心点");
    if(candidate.kind==="LINEAR"&&!candidate.axisEntityId&&!candidate.directionReference)throw Error("请选择阵列方向");
    if(candidate.sourceKind==="FEATURE_DELTA"&&!patternRangeEnds(upstream,candidate.startFeatureId).some(f=>f.id===candidate.source.featureId))throw Error("请选择新增材料范围的起始特征和结束步骤");
    if(candidate.source.featureId&&base.current===view.document.versionId) {
      input={requestId:intent.current,parameterExpressions:expressions,feature:{...draft,pattern:candidate},...(feature.id?{type:"EDIT_FEATURE",targetId:feature.id,expectedFeatureDigest:digest}:{type:"CREATE_PATTERN"})};
      if(internal) input=entityIds.length?{requestId:intent.current,type:"EDIT_SKETCH",sketchId:feature.profile,operations:[{type:editingPattern?"EDIT_PATTERN":"CREATE_PATTERN",patternId:editingPattern,pattern:{...candidate,id:editingPattern??`pattern-${intent.current}`,entityIds},patternParameterExpressions:Object.fromEntries(Object.entries(expressions).map(([key,value])=>[key.replace("pattern:",""),value]))}]}:undefined;
    }
  } catch(cause){parameterError=String(cause);}
  const preview=useFeaturePreview(view.document.id,view.document.versionId,input,draft.operation,(artifact,operation)=>onPreview(artifact,operation,internal?feature.profile:draft.type==="SKETCH_PATTERN"?feature.id:undefined));
  const apply=async()=>{if(!input||!preview.previewId||preview.pending||busy)return;setBusy(true);try{await onApply({...input,previewId:preview.previewId});onClose();}catch(cause){setError(String(cause));setBusy(false);}};
  const removePattern=async(type:"DELETE_PATTERN"|"DETACH_PATTERN")=>{if(busy||!editingPattern)return;setBusy(true);try{await onApply({type:"EDIT_SKETCH",requestId:randomUUID(),sketchId:feature.profile,operations:[{type,patternId:editingPattern}]});onClose();}catch(cause){setError(String(cause));setBusy(false);}};
  const choices=(pattern.sourceKind==="FEATURE_DELTA"?patternRangeEnds(upstream,pattern.startFeatureId):upstream).filter(f=>!f.suppressed&&(!feature.id||draft.type==="SKETCH_PATTERN"||f.bodyId===draft.bodyId)&&(draft.type==="SKETCH_PATTERN"?!!f.sketch:pattern.sourceKind==="GENERATOR_TOOL"?["PAD","LINEAR_EXTRUDE","REVOLVE"].includes(f.type):!f.sketch&&f.type!=="SKETCH_PATTERN"));
  return <CommandDialog id="pattern-feature-editor" open title={`${(feature.id||editingPattern)?"编辑":"创建"}${pattern.kind==="LINEAR"?"线性":"圆周"}阵列`} size="S" onClose={onClose} onConfirm={apply} confirmLoading={busy} confirmDisabled={!preview.previewId||preview.pending||busy||!input}>
    <fieldset disabled={busy} className="instance-pattern-fields feature-input-fields">
      {!internal&&(!feature.id||feature.type==="SOLID_PATTERN")&&<Select value={pattern.sourceKind} options={[{value:"GENERATOR_TOOL",label:"生成特征 / 切削工具"},{value:"FEATURE_DELTA",label:"特征组合（含圆角 / 倒角）"},{value:"BODY_STAGE",label:"整个实体阶段"},...(!feature.id?[{value:"SKETCH_FRAME",label:"空间草图"}]:[])]} onChange={sourceKind=>{setDraft(f=>({...f,type:sourceKind==="SKETCH_FRAME"?"SKETCH_PATTERN":"SOLID_PATTERN",pattern:{...f.pattern!,sourceKind,startFeatureId:undefined,source:{bodyId:f.bodyId??"",featureId:""}}}));setRole(sourceKind==="SKETCH_FRAME"?"profile":"body");}}/>}
      {internal&&<>
        <Select placeholder="新建阵列" value={editingPattern} allowClear options={sourceSketch?.patterns?.map(p=>({value:p.id,label:`${p.kind==="CIRCULAR"?"圆周":"线性"}阵列 · ${p.count}`}))} onChange={id=>{setEditingPattern(id);const p=sourceSketch?.patterns?.find(p=>p.id===id);if(p){update(p);setEntityIds(p.entityIds);setSkipped((p.skippedSlots??[]).join(","));const fields={count:String(p.count),spacing:String(p.spacing??10),angle:String(p.angle??90),phase:String(p.phase??0)};for(const slot of ["count","spacing","angle","phase"] as const){const parameter=view.part?.parameters?.find(value=>value.ownerFeatureId===feature.profile&&value.propertySlot===`pattern:${p.id}:${slot}`);if(parameter)fields[slot]=parameterSourceText(parameter,slot==="count"?"1":slot==="spacing"?unit:"deg",true);}original.current=fields;setText(fields);}}}/>
        <FeaturePickField label="种子几何" value={`已选 ${entityIds.length} 个`} active={role==="geometry"} onActivate={()=>activatePick("geometry")} onClear={()=>setEntityIds([])}/>
        <label>抑制 <Switch checked={!!pattern.suppressed} onChange={suppressed=>update({suppressed})}/></label>
        {editingPattern&&<><Button onClick={()=>void removePattern("DELETE_PATTERN")}>删除阵列</Button><Button onClick={()=>void removePattern("DETACH_PATTERN")}>解除关联</Button></>}
      </>}
      {!internal&&<>
      {pattern.sourceKind==="FEATURE_DELTA"&&<><label>起始特征<Select value={pattern.startFeatureId} placeholder="选择拉伸或旋转" options={upstream.filter(f=>!f.suppressed&&["PAD","LINEAR_EXTRUDE","REVOLVE"].includes(f.type)&&["ADD","NEW_BODY"].includes(f.operation??"ADD")&&f.extent!=="THROUGH_ALL").map(f=>({value:f.id,label:f.name??f.id}))} onChange={startFeatureId=>{const start=upstream.find(f=>f.id===startFeatureId)!;setDraft(f=>({...f,bodyId:start.bodyId,operation:"ADD",pattern:{...f.pattern!,startFeatureId,source:{bodyId:start.bodyId!,featureId:startFeatureId}}}));}}/></label><Alert type="info" title="复制所选拉伸/旋转及其后的圆角、倒角，保留此前实体。"/></>}
      <FeaturePickField label={pattern.sourceKind==="FEATURE_DELTA"?"包含至":"种子"} value={choices.find(f=>f.id===pattern.source.featureId)?.name??"选择上游特征或草图"} active={role==="body"||role==="profile"} onActivate={()=>activatePick(draft.type==="SKETCH_PATTERN"?"profile":"body")}/>
      <Select value={pattern.source.featureId||undefined} options={choices.map(f=>({value:f.id,label:f.name??f.id}))} onChange={selectSource}/>
      <FeaturePickField label={pattern.kind==="CIRCULAR"?"轴":"方向"} value={pattern.axisEntityId?"已选择轴线":"请选择轴线"} active={role==="axis"} onActivate={()=>activatePick("axis")}/>
      </>}
      {!internal&&draft.type==="SOLID_PATTERN"&&draft.operation!=="REMOVE"&&<Select value={pattern.resultMode??"COMBINE"} options={[{value:"COMBINE",label:"与目标 Body 组合"},{value:"INDEPENDENT",label:"保留独立 Solid"}]} onChange={resultMode=>update({resultMode})}/>}
      {internal&&pattern.kind==="LINEAR"&&<FeaturePickField label="方向" value={pattern.directionReference||pattern.axisEntityId?"已选择草图方向":"选择 U/V 轴、直线或投影直线"} active={role==="axis"} onActivate={()=>activatePick("axis")}/>}
      {pattern.kind==="CIRCULAR"&&<FeaturePickField label="中心点" value={pattern.centerReference?"已选择中心点":internal?"选择原点、端点或圆心":"所选轴的原点"} active={role==="point"} onActivate={()=>activatePick("point")} onClear={pattern.centerReference?()=>update({centerReference:undefined}):undefined}/>}
      <label>反向<Switch checked={!!pattern.reversed} onChange={reversed=>update({reversed})}/></label>
      <Select value={pattern.distribution} options={[{value:"FIXED_STEP",label:"固定步距"},{value:"TOTAL_SPAN",label:"总跨度均布"},...(pattern.kind==="CIRCULAR"?[{value:"FULL_CIRCLE",label:"整圆均布"}]:[])]} onChange={value=>update({distribution:value as PatternDefinition["distribution"]})}/>
      {(["count",pattern.kind==="LINEAR"?"spacing":pattern.distribution==="FULL_CIRCLE"?null:"angle",(internal||draft.type==="SKETCH_PATTERN")&&pattern.kind==="CIRCULAR"?"phase":null] as const).filter(x=>x!==null).map(slot=><label key={slot}>{({count:"数量",spacing:`间距 / 跨度 (${unit})`,angle:"角度 (deg)",phase:"相位 (deg)"})[slot]}<Input value={text[slot]} onChange={e=>setText(t=>({...t,[slot]:e.target.value}))}/></label>)}
      <label>跳过槽位（从 0 开始）<Input value={skipped} onChange={e=>setSkipped(e.target.value)}/></label>
    </fieldset>
    {(parameterError||error||preview.error)&&<Alert type="error" title={parameterError??error??preview.error}/>}
  </CommandDialog>;
}
