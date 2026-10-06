import {Alert,Button,Input,InputNumber,Segmented} from "antd";
import {useEffect,useRef,useState} from "react";
import {api} from "../../api/client";
import {CommandDialog} from "../../cad/overlay/floating-panel";
import type {FeatureSelectionSession} from "../../cad/interaction/feature-selection";
import type {DatumPreview} from "../../cad/rendering/datum-reference";
import type {DatumReference,DatumTransform,DocumentView,SelectionItem,Vec3} from "../../types";
import {displayLengthToMillimeters,millimetersToDisplayLength} from "../../state/ui-preferences";
import {finiteVector,planarFaceFrame,planeFrame,unitVector} from "./datum-definition";
import {isSolidFeature} from "./workbench-tree-model";
import {FeaturePickField} from "./feature-pick-field";
import {ParameterReferenceInput} from "./parameter-reference-input";
import {parameterSourceText,parseParameterSource} from "./parameter-editor";
import {useFeaturePreview} from "./use-feature-preview";
import {randomUUID} from "../../utils/random-uuid";
import {DiagnosticCopy,diagnosticReference} from "../../cad/command/diagnostic-copy";

type Slot="source"|"rotationAxis"|"translationDirection";
export function DatumEditor({kind,view,seed,unit,datumId,occurrencePath,placement,onClose,onApply,onPreview,onSelectionSession}:{
 kind:"plane"|"axis";view:DocumentView;seed:SelectionItem[];unit:"mm"|"cm"|"m"|"in";datumId?:string;occurrencePath?:string;placement?:Pick<DatumPreview,"translation"|"rotation">;
 onClose:()=>void;onApply:(input:Record<string,unknown>)=>Promise<unknown>;onPreview:(preview?:DatumPreview)=>void;onSelectionSession:(session?:FeatureSelectionSession)=>void;
}) {
 const existing=kind==="plane"?view.part?.datumPlanes.find(p=>p.id===datumId):view.part?.datumAxes?.find(p=>p.id===datumId);
 const [mode,setMode]=useState(existing&&!existing.definition?"CUSTOM":"REFERENCE"),[name,setName]=useState(existing?.name??(kind==="plane"?"Plane":"Axis"));
 const [origin,setOrigin]=useState<Vec3>((existing?.origin??[0,0,0]).map(v=>millimetersToDisplayLength(v,unit)) as Vec3);
 const [direction,setDirection]=useState<Vec3>(existing&&"normal" in existing?existing.normal:existing&&"direction" in existing?existing.direction:[0,0,1]);
 const [refs,setRefs]=useState<Partial<Record<Slot,DatumReference>>>(existing?.definition??{});
 const initial=(slot:string,fallback:number,inputUnit:string)=>{const p=view.part?.parameters?.find(p=>p.ownerFeatureId===datumId&&p.propertySlot===slot);return p?parameterSourceText(p,inputUnit,true):String(fallback);};
 const [angle,setAngle]=useState(initial("datum:angle",existing?.definition?.angle??0,"deg"));
 const [distance,setDistance]=useState(initial("datum:distance",millimetersToDisplayLength(existing?.definition?.distance??0,unit),unit));
 const [picking,setPicking]=useState<Slot|undefined>(),[loading,setLoading]=useState(false),[busy,setBusy]=useState(false),[error,setError]=useState<string>();
 const base=useRef(view.document.versionId),intent=useRef(randomUUID()),alive=useRef(true),generation=useRef(0);
 const callbacks=useRef({onPreview,onSelectionSession});callbacks.current={onPreview,onSelectionSession};
 const valid=base.current===view.document.versionId;
 const pick=async(selection:SelectionItem,slot:Slot)=>{
  if((selection.ownerDocumentId??selection.documentId)!==view.document.id||(selection.occurrencePath??"")!==(occurrencePath??"")||selection.versionId!==base.current)return;
  const token=++generation.current;setLoading(true);setError(undefined);
  try{
   let ref:DatumReference;
   if(slot==="source"&&kind==="plane"&&selection.kind==="plane"&&selection.entityId)ref={kind:"PLANE",entityId:selection.entityId};
   else if(selection.kind==="axis"&&selection.entityId)ref=selection.axis==="DATUM"?{kind:"AXIS",entityId:selection.entityId}:{kind:"AXIS_SYSTEM",entityId:selection.entityId,axis:selection.axis};
   else if(selection.kind==="edge"||slot==="source"&&kind==="plane"&&selection.kind==="face"){
    if(!selection.geometryKey)throw new Error("需要当前精确几何");
    const properties=await api.getTopologyProperties(view.document.id,selection.geometryKey,selection.kind.toUpperCase() as "FACE"|"EDGE",selection.topologyId,base.current);
    if(selection.kind==="face")planarFaceFrame(properties);else if(properties.geometryType!=="LINE")throw new Error("方向引用只接受精确直线");
    const bound=await api.bindTopologySelection(view.document.id,{sourceVersionId:base.current,geometryKey:selection.geometryKey,kind:selection.kind.toUpperCase() as "FACE"|"EDGE",localId:selection.topologyId});
    const stage=selection.displayStageFeatureId??view.part?.features.slice().reverse().find(f=>f.bodyId===bound.sourceBodyId&&isSolidFeature(f)&&!f.suppressed)?.id;
    if(!stage)throw new Error("缺少精确来源阶段");
    ref={kind:"TOPOLOGY",featureId:stage,selection:bound,sourceVersionId:base.current};
   }else if(selection.kind==="visual"&&selection.visualType==="CURVE"){
    const sketch=view.part?.features.find(f=>f.id===selection.featureId)?.sketch;
    const entity=sketch?.entities.find(e=>e.id===selection.entityId)??sketch?.externalGeometry?.find(e=>e.id===selection.entityId&&e.status==="CONNECTED")?.snapshot;
    if(entity?.kind!=="LINE")throw new Error("方向引用只接受草图直线");ref={kind:"SKETCH_LINE",featureId:selection.featureId,entityId:selection.entityId};
   }else throw new Error(slot==="source"&&kind==="plane"?"请选择标准面、基准面或实体平面":"请选择标准轴、基准轴、草图直线或实体直线边");
   if(!alive.current||token!==generation.current)return;
   if(ref.entityId===datumId)throw new Error("基准不能引用自身");
   setRefs(old=>({...old,[slot]:ref}));setPicking(undefined);
  }catch(cause){if(alive.current&&token===generation.current)setError(String(cause));}
  finally{if(alive.current&&token===generation.current)setLoading(false);}
 };
 const pickRef=useRef(pick);pickRef.current=pick;
 useEffect(()=>{
  alive.current=true;
  if(!existing){const initial=seed.find(s=>kind==="plane"?s.kind==="plane"||s.kind==="face":s.kind==="axis"||s.kind==="edge"||s.kind==="visual");if(initial)void pickRef.current(initial,"source");else if(kind==="plane")setRefs({source:{kind:"PLANE",entityId:"datum-xy"}});else setRefs({source:{kind:"AXIS_SYSTEM",entityId:view.part?.axisSystems[0]?.id,axis:"Z"}});}
  return()=>{alive.current=false;generation.current++;callbacks.current.onPreview();callbacks.current.onSelectionSession();};
 },[]);
 useEffect(()=>{callbacks.current.onSelectionSession(mode==="REFERENCE"&&picking&&valid?{role:picking==="source"&&kind==="plane"?"plane":"direction",documentId:view.document.id,versionId:base.current,occurrencePath,selections:[],onPick:s=>{void pickRef.current(s,picking);}}:undefined);},[mode,picking,valid]);
 let input:Record<string,unknown>|undefined,parameterError:string|undefined;
 try{
  if(!valid)throw new Error("模型已改变，请关闭并重新打开命令");
  if(loading||!name.trim())throw new Error("请完成参考和名称");
  const center=origin.map(v=>displayLengthToMillimeters(v,unit)) as Vec3;
  const n=unitVector(direction);if(!finiteVector(center))throw new Error("原点需要有限数值");
  const frame=kind==="plane"?planeFrame(center,n,existing&&"uDirection" in existing?existing.uDirection:undefined):{origin:center,direction:n};
  input={requestId:intent.current,type:`${datumId?"EDIT":"CREATE"}_DATUM_${kind.toUpperCase()}`,targetId:datumId,name:name.trim(),...frame};
  if(mode==="REFERENCE"){
   if(!refs.source)throw new Error("请选择源基准");
   const a=parseParameterSource(angle,"deg"),d=parseParameterSource(distance,unit);
   const definition:DatumTransform={source:refs.source,rotationAxis:refs.rotationAxis,translationDirection:refs.translationDirection,angle:0,distance:0};
   if(a.kind==="EXPRESSION"){if(!a.expression)throw new Error("请输入旋转角度");input.datumAngleExpression=a.expression;}else{if(a.unit!=="deg"&&a.unit!=="rad")throw new Error("角度需要 deg 或 rad");definition.angle=a.value*(a.unit==="rad"?180/Math.PI:1);}
   if(d.kind==="EXPRESSION"){if(!d.expression)throw new Error("请输入平移距离");input.datumDistanceExpression=d.expression;}else{if(!["mm","cm","m","in"].includes(d.unit))throw new Error("距离需要长度单位");definition.distance=displayLengthToMillimeters(d.value,d.unit as typeof unit);}
   input.datumDefinition=definition;
  }
 }catch(cause){input=undefined;parameterError=String(cause);}
 const preview=useFeaturePreview(view.document.id,base.current,input,undefined,()=>{});
 useEffect(()=>{const geometry=preview.referenceGeometry;const candidate=kind==="plane"?(datumId?geometry?.datumPlanes.find(p=>p.id===datumId):geometry?.datumPlanes.at(-1)):(datumId?geometry?.datumAxes?.find(p=>p.id===datumId):geometry?.datumAxes?.at(-1));callbacks.current.onPreview(candidate?{...placement,...(kind==="plane"?{plane:candidate as import("../../types").DatumPlane}:{axis:candidate as import("../../types").DatumAxis})}:undefined);},[preview.referenceGeometry,placement?.translation,placement?.rotation]);
 const label=(ref?:DatumReference)=>{
  if(!ref)return "未指定";
  const id=ref.featureId??ref.entityId;
  const datum=view.part?.datumPlanes.find(p=>p.id===id)??view.part?.datumAxes?.find(p=>p.id===id)??view.part?.axisSystems.find(p=>p.id===id)??view.part?.features.find(f=>f.id===id);
  return `${datum?.name??id??"精确参考"}${ref.axis?` · ${ref.axis}`:""}`;
 };
 const vector=(title:string,values:Vec3,set:(value:Vec3)=>void)=><fieldset className="datum-vector-fields"><legend>{title}</legend>{["X","Y","Z"].map((axis,i)=><label key={axis}>{axis}<InputNumber value={Number.isFinite(values[i])?values[i]:null} onChange={v=>set(values.map((x,j)=>i===j?v??NaN:x) as Vec3)}/></label>)}</fieldset>;
 return <CommandDialog id={`datum-${kind}`} open size="M" title={`${datumId?"编辑":"创建"}基准${kind==="plane"?"面":"轴"}`} onClose={onClose} confirmLoading={busy} confirmDisabled={!input||!preview.previewId||preview.pending||loading} onConfirm={async()=>{if(!input||!preview.previewId||busy)return;setBusy(true);try{await onApply({...input,previewId:preview.previewId});onClose();}catch(cause){setError(String(cause));setBusy(false);}}}>
  <label>名称<Input value={name} onChange={e=>setName(e.target.value)}/></label>
  <Segmented block value={mode} options={[{label:"关联参考",value:"REFERENCE"},{label:"自由坐标",value:"CUSTOM"}]} onChange={v=>{setMode(v);setPicking(undefined);}}/>
  {mode==="CUSTOM"?<>{vector(`原点（${unit}）`,origin,setOrigin)}{vector(kind==="plane"?"法向":"方向",direction,setDirection)}</>:<>
   {(["source","rotationAxis","translationDirection"] as Slot[]).map(slot=><div key={slot}><FeaturePickField label={slot==="source"?"源基准":slot==="rotationAxis"?"旋转轴（右手方向）":"平移方向（默认随源方向）"} value={label(refs[slot])} active={picking===slot} onActivate={()=>setPicking(slot)}/>{slot!=="source"&&refs[slot]&&<Button size="small" onClick={()=>setRefs(old=>({...old,[slot]:undefined}))}>清除</Button>}</div>)}
   <label>旋转角度（deg）<ParameterReferenceInput value={angle} onChange={setAngle} parameters={view.part?.parameters??[]} placeholder="倾角 或 30 deg"/></label>
   <label>平移距离（{unit}）<ParameterReferenceInput value={distance} onChange={setDistance} parameters={view.part?.parameters??[]} unit={unit} placeholder="高度 或 20 mm"/></label>
   <small>先绕参考轴旋转，再沿指定方向平移；确认后保留关联。</small>
  </>}
  {(error??parameterError??preview.error)&&<Alert type="error" message={error??parameterError??preview.error}/>}
  {Boolean(preview.failure)&&<DiagnosticCopy text={diagnosticReference(preview.failure,{documentId:view.document.id,versionId:base.current})}/>}
  <small role="status">{preview.pending?"正在权威预览…":preview.previewId?"预览成功，可确认":"等待有效定义"}</small>
 </CommandDialog>;
}
