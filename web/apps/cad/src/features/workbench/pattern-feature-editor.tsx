import type { FeaturePickRole } from "../../cad/interaction/feature-selection";
import { Alert, Button, Input, Select, Switch } from "antd";
import { useEffect, useRef, useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import type { Feature, PatternDefinition, SelectionItem } from "../../types";
import type { SolidFeatureEditor } from "./solid-feature-editor";
import { FeaturePickField } from "./feature-pick-field";
import { bodyStage, selectedSketch, selectedAxis } from "./feature-picking";
import { parameterSourceText, parseParameterSource } from "./parameter-editor";
import { useFeaturePreview } from "./use-feature-preview";
import { randomUUID } from "../../utils/random-uuid";

export function PatternFeatureEditor({ view, feature, digest, unit, seed, occurrencePath, onClose, onApply, onPreview, onSelectionSession }: Parameters<typeof SolidFeatureEditor>[0]) {
  const internal=feature.type==="SKETCH";
  const sourceSketch=view.part?.features.find(f=>f.id===feature.profile)?.sketch;
  const [entityIds,setEntityIds]=useState<string[]>(seed.flatMap(s=>s.kind==="visual"&&s.featureId===feature.profile&&sourceSketch?.entities.some(e=>e.id===s.entityId)?[s.entityId]:[]));
  const [editingPattern,setEditingPattern]=useState<string>();
  const [draft,setDraft]=useState(feature);
  const [role,setRole]=useState<FeaturePickRole>(internal?"seam":feature.type==="SKETCH_PATTERN"?"profile":"body");
  const [error,setError]=useState<string>(),[busy,setBusy]=useState(false);
  const intent=useRef(randomUUID()),base=useRef(view.document.versionId);
  const pattern=draft.pattern!;
  const upstream=view.part!.features.slice(0,feature.id?view.part!.features.findIndex(f=>f.id===feature.id):undefined);
  const update=(patch:Partial<NonNullable<Feature["pattern"]>>)=>setDraft(f=>({...f,pattern:{...f.pattern!,...patch}}));
  const initial=(slot:string,value:number)=>{
    const parameter=view.part?.parameters?.find(p=>p.ownerFeatureId===feature.id&&p.propertySlot===`pattern:${slot}`);
    return parameter?parameterSourceText(parameter,slot==="count"?"1":slot==="spacing"?unit:"deg",true):String(value);
  };
  const [text,setText]=useState({count:initial("count",pattern.count),spacing:initial("spacing",pattern.spacing??10),angle:initial("angle",pattern.angle??90),phase:initial("phase",pattern.phase??0)});
  const original=useRef({...text});
  const [skipped,setSkipped]=useState((pattern.skippedSlots??[]).join(","));
  const selectSource=(id:string)=>{
    const source=upstream.find(f=>f.id===id);if(!source?.bodyId)return;
    setDraft(f=>({...f,bodyId:feature.id?f.bodyId:source.bodyId,operation:source.operation==="REMOVE"?"REMOVE":"ADD",pattern:{...f.pattern!,resultMode:source.operation==="REMOVE"?"COMBINE":f.pattern!.resultMode,source:{bodyId:source.bodyId!,featureId:id}}}));
  };
  const pick=(selection:SelectionItem)=>{
    if((selection.ownerDocumentId??selection.documentId)!==view.document.id||(selection.occurrencePath??"")!==(occurrencePath??"")||selection.versionId&&selection.versionId!==base.current||busy||base.current!==view.document.versionId)return;
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
    else {const source=bodyStage(selection,upstream.filter(f=>f.bodyId===draft.bodyId&&!f.sketch));if(source)selectSource(source.featureId);}
  };
  const pickRef=useRef(pick);pickRef.current=pick;
  useEffect(()=>{onSelectionSession({role,documentId:view.document.id,versionId:base.current,occurrencePath,sketchIds:internal?[feature.profile!]:undefined,selections:[],onPick:s=>pickRef.current(s)});return()=>onSelectionSession();},[role,view.document.id,occurrencePath]);
  useEffect(()=>{if(!internal&&!feature.id&&seed[0])pickRef.current(seed[0]);},[]);
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
    if(candidate.source.featureId&&base.current===view.document.versionId) {
      input={requestId:intent.current,parameterExpressions:expressions,feature:{...draft,pattern:candidate},...(feature.id?{type:"EDIT_FEATURE",targetId:feature.id,expectedFeatureDigest:digest}:{type:"CREATE_PATTERN"})};
      if(internal) input=entityIds.length?{requestId:intent.current,type:"EDIT_SKETCH",sketchId:feature.profile,operations:[{type:editingPattern?"EDIT_PATTERN":"CREATE_PATTERN",patternId:editingPattern,pattern:{...candidate,id:editingPattern??`pattern-${intent.current}`,entityIds},patternParameterExpressions:Object.fromEntries(Object.entries(expressions).map(([key,value])=>[key.replace("pattern:",""),value]))}]}:undefined;
    }
  } catch(cause){parameterError=String(cause);}
  const preview=useFeaturePreview(view.document.id,view.document.versionId,input,draft.operation,(artifact,operation)=>onPreview(artifact,operation,internal?feature.profile:draft.type==="SKETCH_PATTERN"?feature.id:undefined));
  const apply=async()=>{if(!input||!preview.previewId||preview.pending||busy)return;setBusy(true);try{await onApply({...input,previewId:preview.previewId});onClose();}catch(cause){setError(String(cause));setBusy(false);}};
  const removePattern=async(type:"DELETE_PATTERN"|"DETACH_PATTERN")=>{if(busy||!editingPattern)return;setBusy(true);try{await onApply({type:"EDIT_SKETCH",requestId:randomUUID(),sketchId:feature.profile,operations:[{type,patternId:editingPattern}]});onClose();}catch(cause){setError(String(cause));setBusy(false);}};
  const choices=upstream.filter(f=>!f.suppressed&&(!feature.id||draft.type==="SKETCH_PATTERN"||f.bodyId===draft.bodyId)&&(draft.type==="SKETCH_PATTERN"?!!f.sketch:pattern.sourceKind==="GENERATOR_TOOL"?["PAD","LINEAR_EXTRUDE","REVOLVE"].includes(f.type):!f.sketch&&f.type!=="SKETCH_PATTERN"));
  return <CommandDialog id="pattern-feature-editor" open title={`${feature.id?"编辑":"创建"}${pattern.kind==="LINEAR"?"线性":"圆周"}阵列`} size="S" onClose={onClose} onConfirm={apply} confirmLoading={busy} confirmDisabled={!preview.previewId||preview.pending||busy||!input}>
    <fieldset disabled={busy} className="instance-pattern-fields feature-input-fields">
      {!internal&&!feature.id&&<Select value={pattern.sourceKind} options={[{value:"GENERATOR_TOOL",label:"生成特征 / 切削工具"},{value:"BODY_STAGE",label:"实体"},{value:"SKETCH_FRAME",label:"空间草图"}]} onChange={sourceKind=>{setDraft(f=>({...f,type:sourceKind==="SKETCH_FRAME"?"SKETCH_PATTERN":"SOLID_PATTERN",pattern:{...f.pattern!,sourceKind,source:{bodyId:f.bodyId??"",featureId:""}}}));setRole(sourceKind==="SKETCH_FRAME"?"profile":"body");}}/>}
      {internal&&<>
        <Select placeholder="新建阵列" value={editingPattern} allowClear options={sourceSketch?.patterns?.map(p=>({value:p.id,label:`${p.kind==="CIRCULAR"?"圆周":"线性"}阵列 · ${p.count}`}))} onChange={id=>{setEditingPattern(id);const p=sourceSketch?.patterns?.find(p=>p.id===id);if(p){update(p);setEntityIds(p.entityIds);setSkipped((p.skippedSlots??[]).join(","));const fields={count:String(p.count),spacing:String(p.spacing??10),angle:String(p.angle??90),phase:String(p.phase??0)};for(const slot of ["count","spacing","angle","phase"] as const){const parameter=view.part?.parameters?.find(value=>value.ownerFeatureId===feature.profile&&value.propertySlot===`pattern:${p.id}:${slot}`);if(parameter)fields[slot]=parameterSourceText(parameter,slot==="count"?"1":slot==="spacing"?unit:"deg",true);}original.current=fields;setText(fields);}}}/>
        <FeaturePickField label="种子几何" value={`已选 ${entityIds.length} 个`} active onActivate={()=>setRole("seam")} onClear={()=>setEntityIds([])}/>
        <label>抑制 <Switch checked={!!pattern.suppressed} onChange={suppressed=>update({suppressed})}/></label>
        {editingPattern&&<><Button onClick={()=>void removePattern("DELETE_PATTERN")}>删除阵列</Button><Button onClick={()=>void removePattern("DETACH_PATTERN")}>解除关联</Button></>}
      </>}
      {!internal&&<>
      <FeaturePickField label="种子" value={choices.find(f=>f.id===pattern.source.featureId)?.name??"选择上游特征或草图"} active={role!=="axis"} onActivate={()=>setRole(draft.type==="SKETCH_PATTERN"?"profile":"body")}/>
      <Select value={pattern.source.featureId||undefined} options={choices.map(f=>({value:f.id,label:f.name??f.id}))} onChange={selectSource}/>
      <FeaturePickField label={pattern.kind==="CIRCULAR"?"轴":"方向"} value={pattern.direction.join(", ")} active={role==="axis"} onActivate={()=>setRole("axis")}/>
      </>}
      {!internal&&draft.type==="SOLID_PATTERN"&&draft.operation!=="REMOVE"&&<Select value={pattern.resultMode??"COMBINE"} options={[{value:"COMBINE",label:"与目标 Body 组合"},{value:"INDEPENDENT",label:"保留独立 Solid"}]} onChange={resultMode=>update({resultMode})}/>}
      {internal&&<Select value={pattern.kind==="CIRCULAR"?"Z":Math.abs(pattern.direction[1])>0?"Y":"X"} options={(pattern.kind==="CIRCULAR"?["Z"]:["X","Y"]).map(value=>({value,label:`${value} 方向`}))} onChange={value=>update({direction:value==="X"?[1,0,0]:value==="Y"?[0,1,0]:[0,0,1]})}/>}
      {pattern.kind==="CIRCULAR"&&<label>中心 ({unit})<div>{([0,1,...(!internal?[2]:[])] as const).map(index=><Input key={index} aria-label={`阵列中心 ${["X","Y","Z"][index]}`} type="number" value={pattern.origin[index]/({mm:1,cm:10,m:1000,in:25.4}[unit as "mm"]??1)} onChange={e=>{const origin=[...pattern.origin] as [number,number,number];origin[index]=Number(e.target.value)*({mm:1,cm:10,m:1000,in:25.4}[unit as "mm"]??1);update({origin,axisEntityId:undefined});}}/>)}</div></label>}
      <Select value={pattern.distribution} options={[{value:"FIXED_STEP",label:"固定步距"},{value:"TOTAL_SPAN",label:"总跨度均布"},...(pattern.kind==="CIRCULAR"?[{value:"FULL_CIRCLE",label:"整圆均布"}]:[])]} onChange={value=>update({distribution:value as PatternDefinition["distribution"]})}/>
      {(["count",pattern.kind==="LINEAR"?"spacing":pattern.distribution==="FULL_CIRCLE"?null:"angle",(internal||draft.type==="SKETCH_PATTERN")&&pattern.kind==="CIRCULAR"?"phase":null] as const).filter(x=>x!==null).map(slot=><label key={slot}>{({count:"数量",spacing:`间距 / 跨度 (${unit})`,angle:"角度 (deg)",phase:"相位 (deg)"})[slot]}<Input value={text[slot]} onChange={e=>setText(t=>({...t,[slot]:e.target.value}))}/></label>)}
      <label>跳过槽位（从 0 开始）<Input value={skipped} onChange={e=>setSkipped(e.target.value)}/></label>
    </fieldset>
    {(parameterError||error||preview.error)&&<Alert type="error" title={parameterError??error??preview.error}/>}
  </CommandDialog>;
}
