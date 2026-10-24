import {engineeringProblem} from "../../cad/command/operation-feedback";
import {Alert,Select,Segmented} from "antd";
import {useRef,useState} from "react";
import type {Artifact,DocumentView,Feature} from "../../types";
import {CommandDialog} from "../../cad/overlay/floating-panel";
import {randomUUID} from "../../utils/random-uuid";
import {useFeaturePreview} from "./use-feature-preview";

export function FeatureMoveDialog({view,feature,digest,onClose,onApply,onPreview}:{view:DocumentView;feature:Feature;digest:string;onClose:()=>void;onApply:(input:Record<string,unknown>)=>Promise<unknown>;onPreview:(artifact?:Artifact)=>void}){
 const version=useRef(view.document.versionId),intent=useRef(randomUUID());
 const [anchor,setAnchor]=useState<string>(),[placement,setPlacement]=useState("BEFORE"),[busy,setBusy]=useState(false),[error,setError]=useState<string>();
 const valid=version.current===view.document.versionId;
 const anchors=view.part!.features.filter(f=>f.id!==feature.id&&f.bodyId===feature.bodyId);
 const input=valid&&anchor?{type:"MOVE_FEATURE",requestId:intent.current,targetId:feature.id,anchorFeatureId:anchor,placement,expectedFeatureDigest:digest}:undefined;
 const preview=useFeaturePreview(view.document.id,view.document.versionId,input,"ADD",onPreview);
 const apply=async()=>{
  if(!input||!preview.previewId||preview.pending||busy)return;
  setBusy(true);setError(undefined);
  try{await onApply({...input,previewId:preview.previewId});onClose();}catch(cause){setError(String(cause));setBusy(false);}
 };
 return <CommandDialog id="feature-move" open title="调整特征顺序" size="S" onClose={onClose} onConfirm={apply} confirmLoading={busy} confirmDisabled={!input||!preview.previewId||preview.pending||busy}>
  <fieldset disabled={busy} style={{display:"grid",gap:12,margin:0,border:0,padding:0}}>
   <p>移动：{feature.name} · {view.part!.bodies.find(b=>b.id===feature.bodyId)?.name}</p>
   <label style={{display:"grid",gap:4}}>锚点特征<Select aria-label="锚点特征" placeholder="选择同 Body 的步骤" value={anchor} style={{width:"100%"}} options={anchors.map(f=>({value:f.id,label:`${f.order??view.part!.features.indexOf(f)+1}. ${f.name}`}))} onChange={id=>{setAnchor(id);setError(undefined)}}/></label>
   <label style={{display:"grid",gap:4}}>位置<Segmented value={placement} options={[{value:"BEFORE",label:"之前"},{value:"AFTER",label:"之后"}]} onChange={value=>{setPlacement(value);setError(undefined)}}/></label>
   <small>只移动当前特征，其他步骤保持相对顺序。固定步骤和受保护范围不能跨越；预览通过后才能确认。</small>
   {!valid&&<Alert type="warning" title="模型已改变，请关闭后重新打开"/>}
   {preview.pending&&<p role="status">正在验证候选顺序与几何…</p>}
   {(error??preview.error)&&<Alert type="error" title="无法调整顺序" description={engineeringProblem(error??preview.error)}/>}
  </fieldset>
 </CommandDialog>;
}
