import {isSolidStage} from "./solid-feature-model";
import {CommandField} from "../../cad/overlay/command-form";
import {engineeringProblem} from "../../cad/command/operation-feedback";
import {Alert,Select,Segmented} from "antd";
import {useRef,useState} from "react";
import type {Artifact,DocumentView,Feature} from "../../types";
import {CommandDialog} from "../../cad/overlay/floating-panel";
import {randomUUID} from "../../utils/random-uuid";
import {useFeaturePreview} from "./use-feature-preview";

export function FeatureMoveDialog({view,features,digests,orderDigest,onClose,onApply,onPreview}:{view:DocumentView;features:Feature[];digests:Record<string,string>;orderDigest:string;onClose:()=>void;onApply:(input:Record<string,unknown>)=>Promise<unknown>;onPreview:(artifact?:Artifact)=>void}){
 const feature=features[0],ids=features.map(f=>f.id);
 const version=useRef(view.document.versionId),intent=useRef(randomUUID());
 const [anchor,setAnchor]=useState<string>(),[placement,setPlacement]=useState("BEFORE"),[busy,setBusy]=useState(false),[error,setError]=useState<string>();
 const valid=version.current===view.document.versionId;
 const anchors=view.part!.features.filter(f=>!ids.includes(f.id)&&f.bodyId===feature.bodyId&&isSolidStage(f));
 const input=valid&&anchor?{type:"MOVE_FEATURE",requestId:intent.current,featureIds:ids,versionId:version.current,anchorFeatureId:anchor,placement,expectedFeatureDigests:digests,expectedFeatureOrderDigest:orderDigest}:undefined;
 const preview=useFeaturePreview(view.document.id,view.document.versionId,input,"ADD",onPreview);
 const apply=async()=>{
  if(!input||!preview.previewId||preview.pending||busy)return;
  setBusy(true);setError(undefined);
  try{await onApply({...input,previewId:preview.previewId});onClose();}catch(cause){setError(String(cause));setBusy(false);}
 };
 return <CommandDialog id="feature-move" open title="调整特征顺序" size="S" onClose={onClose} onConfirm={apply} confirmLoading={busy} confirmDisabled={!input||!preview.previewId||preview.pending||busy}>
  <fieldset disabled={busy} style={{display:"grid",gap:12,margin:0,border:0,padding:0}}>
   <CommandField label="移动" width="selection">{features.length===1?<span>{feature.name}</span>:<details><summary>{feature.name} → {features.at(-1)!.name} · {features.length} 项</summary><ol>{features.map(f=><li key={f.id}>{f.name}{f.suppressed?'（已抑制）':''}</li>)}</ol></details>}</CommandField>
   <CommandField label="实体" width="selection"><span>{view.part!.bodies.find(b=>b.id===feature.bodyId)?.name}</span></CommandField>
   <CommandField  label={<>锚点特征</>} width="enum"><Select aria-label="锚点特征" placeholder="选择同 Body 的步骤" value={anchor} style={{width:"100%"}} options={anchors.map(f=>({value:f.id,label:`${f.order??view.part!.features.indexOf(f)+1}. ${f.name}`}))} onChange={id=>{setAnchor(id);setError(undefined)}}/></CommandField>
   <CommandField  label={<>位置</>} width="content"><Segmented value={placement} options={[{value:"BEFORE",label:"之前"},{value:"AFTER",label:"之后"}]} onChange={value=>{setPlacement(value);setError(undefined)}}/></CommandField>
   {!valid&&<Alert type="warning" title="模型已改变，请关闭后重新打开"/>}
   {preview.pending&&<p role="status">正在验证候选顺序与几何…</p>}
   {(error??preview.error)&&<Alert type="error" title="无法调整顺序" description={engineeringProblem(error??preview.error)}/>}
  </fieldset>
 </CommandDialog>;
}
