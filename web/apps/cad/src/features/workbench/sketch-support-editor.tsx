import {recordDeveloperEvidence} from "../../cad/command/developer-evidence";
import {engineeringProblem} from "../../cad/command/operation-feedback";
import {Alert} from "antd";
import {useEffect,useRef,useState} from "react";
import {api} from "../../api/client";
import type {Artifact,DocumentView,Feature,SelectionItem} from "../../types";
import type {FeatureSelectionSession} from "../../cad/interaction/feature-selection";
import type {DatumPreview} from "../../cad/rendering/datum-reference";
import {CommandStatus} from "../../cad/overlay/command-form";
import {CommandDialog} from "../../cad/overlay/floating-panel";
import {FeaturePickField} from "./feature-pick-field";
import {useFeaturePreview} from "./use-feature-preview";
import {randomUUID} from "../../utils/random-uuid";

export function SketchSupportEditor({view,feature,occurrencePath,occurrenceContext,onSelectionSession,onInputs,onPreview,onFrame,onApply,onClose}:{
 view:DocumentView;feature:Feature;occurrencePath?:string;occurrenceContext?:Pick<SelectionItem,"instancePath"|"contextVariantKey"|"rootDocumentId">;
 onSelectionSession:(s?:FeatureSelectionSession)=>void;onInputs:(a?:Artifact[])=>void;
 onPreview:(a?:Artifact)=>void;onFrame:(frame?:DatumPreview)=>void;onApply:(input:Record<string,unknown>)=>Promise<unknown>;onClose:()=>void;
}){
 const [chosen,setChosen]=useState<SelectionItem>(),[inputs,setInputs]=useState<Artifact[]>([]),[error,setError]=useState<string>(),[busy,setBusy]=useState(false);
 const [picking,setPicking]=useState(true);
 const cancelPick=()=>{if(!picking)return false;setPicking(false);return true};
 const revision=useRef(view.document.versionId),intent=useRef(randomUUID());
 const current=revision.current===view.document.versionId;
 const callbacks=useRef({onSelectionSession,onInputs,onFrame});callbacks.current={onSelectionSession,onInputs,onFrame};
 useEffect(()=>{
  const controller=new AbortController();let alive=true;
  // An unavailable independent Body must not hide other legal input stages.
  void Promise.allSettled(view.part!.bodies.map(body=>api.getFeatureInput(view.document.id,{versionId:revision.current,featureId:feature.id,bodyId:body.id},controller.signal))).then(results=>{
   if(!alive)return;const artifacts:Artifact[]=[];
   results.forEach(r=>{if(r.status==="fulfilled")artifacts.push(...(r.value.artifacts??[]));else {recordDeveloperEvidence("草图支撑",r.reason)}});
   setInputs(artifacts);callbacks.current.onInputs(artifacts);
  });
  return()=>{alive=false;controller.abort();callbacks.current.onInputs();callbacks.current.onSelectionSession();callbacks.current.onFrame();};
 },[]);
 const accepted=(s:SelectionItem)=>{
  if((s.ownerDocumentId??s.documentId)!==view.document.id||(s.occurrencePath??"")!==(occurrencePath??"")||s.versionId&&s.versionId!==revision.current)return false;
  if(s.kind==="plane")return !!view.part?.datumPlanes.some(p=>p.id===(s.datumPlane?.id??s.entityId??s.id));
  return s.kind==="face"&&inputs.some(a=>a.geometryKey===s.geometryKey&&a.bodyId===s.bodyId);
 };
 const pickRef=useRef((s:SelectionItem)=>{});pickRef.current=s=>{if(current&&!busy&&accepted(s)){setChosen(s);setPicking(false);setError(undefined)}};
 useEffect(()=>{callbacks.current.onSelectionSession(picking?{...occurrenceContext,role:"plane",documentId:view.document.id,versionId:revision.current,occurrencePath,selections:chosen?[{...occurrenceContext,...chosen}]:[],accept:accepted,onPick:s=>pickRef.current(s)}:undefined);},[JSON.stringify(chosen),JSON.stringify(inputs.map(a=>[a.bodyId,a.geometryKey])),current,busy,picking]);
 let input:Record<string,unknown>|undefined;
 if(chosen&&current)input={type:"REASSIGN_SKETCH_SUPPORT",requestId:intent.current,versionId:revision.current,sketchId:feature.id,...(chosen.kind==="plane"?{datumPlaneId:chosen.datumPlane?.id??chosen.entityId??chosen.id}:{targetKind:"FACE",bodyId:chosen.bodyId,geometryKey:chosen.geometryKey,topologyId:"topologyId"in chosen?chosen.topologyId:undefined})};
 const preview=useFeaturePreview(view.document.id,revision.current,input,undefined,onPreview);
 const plane=preview.referenceGeometry?.datumPlanes.find(p=>p.id==="sketch-support-preview");
 useEffect(()=>{callbacks.current.onFrame(plane?{plane,showFrame:true}:undefined)},[JSON.stringify(plane)]);
 const commit=async()=>{if(!input||!preview.previewId||preview.pending||busy)return;setBusy(true);try{await onApply({...input,previewId:preview.previewId});onClose()}catch(e){setError(engineeringProblem(e));setBusy(false)}};
 return <CommandDialog id="sketch-support-editor" open title="更换草图支撑" size="S" onClose={onClose} onCancelPick={cancelPick} commandStatus={<CommandStatus pending={preview.pending} error={preview.error} onRetry={preview.retry}/>} onConfirm={commit} confirmLoading={busy} confirmDisabled={!current||!input||!preview.previewId||preview.pending||busy}>
  <fieldset disabled={busy} className="feature-input-fields">
   <p>草图随新支撑移动，保留尺寸和约束；关联几何按新位置更新。</p>
   {feature.sketch?.support.status==="FAILED_SUPPORT"&&<Alert type="warning" title="原草图支撑已失效" description="请在视图区选择新的支撑面。"/>}
   <FeaturePickField label="新支撑" value={chosen?.kind==="plane"?view.part?.datumPlanes.find(p=>p.id===(chosen.datumPlane?.id??chosen.entityId??chosen.id))?.name??"已选基准面":chosen?"已选上游实体平面":"选择基准面或上游平面"} selected={!!chosen} selections={chosen?[chosen]:[]} active={picking} onActivate={()=>setPicking(true)} onClear={chosen?()=>setChosen(undefined):undefined}/>
   {!current&&<Alert type="warning" title="模型版本已改变，请关闭并重新进入"/>}
   {error&&<Alert type="error" title={error}/>}

  </fieldset>
 </CommandDialog>;
}
