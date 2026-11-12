import {CommandField} from "../../cad/overlay/command-form";
import {engineeringProblem} from "../../cad/command/operation-feedback";
import { Alert, Button, Checkbox, Collapse, ColorPicker, Descriptions, Input, InputNumber, Select, Slider, Space, Tabs, Typography } from "antd";
import { useEffect, useMemo, useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import type { CommandPreview, PropertyPreview, PropertyUpdateRequest, SurfaceStyle } from "../../types";
import { commonStyle, type PropertyEditorTarget } from "./property-target";
import { palette } from "../../design/visual-tokens";

export function ObjectPropertiesDialog({targets,pending,onPreviewRequest,onPreview,onApply,onRefresh,onClose,onOpenOriginal}: {
  targets:PropertyEditorTarget[];pending:boolean;
  onPreviewRequest:(request:PropertyUpdateRequest,signal:AbortSignal)=>Promise<CommandPreview>;
  onPreview:(preview:PropertyPreview|undefined)=>void;onApply:(request:PropertyUpdateRequest)=>Promise<unknown>;
  onRefresh:()=>Promise<void>;
  onClose:()=>void;onOpenOriginal?:(documentID:string)=>void;
}) {
  const first=targets[0];const single=targets.length===1;
  const [name,setName]=useState(first.name??"");const [description,setDescription]=useState(first.description??"");
  const [nameDirty,setNameDirty]=useState(false);const [descriptionDirty,setDescriptionDirty]=useState(false);
  const [commitError,setCommitError]=useState<string>();const [refreshing,setRefreshing]=useState(false);
  useEffect(()=>{if(!nameDirty)setName(first.name??"");if(!descriptionDirty)setDescription(first.description??"")},[first,nameDirty,descriptionDirty]);
  const [appearance,setAppearance]=useState<NonNullable<PropertyUpdateRequest["appearance"]>>({});
  const [previewAppearances,setPreviewAppearances]=useState<import("../../types").PropertyAppearanceProjection[]>();
  const [clearFaces,setClearFaces]=useState(false);const [previewError,setPreviewError]=useState<string>();const [retry,setRetry]=useState(0);
  const sameOwner=targets.every(t=>t.documentId===first.documentId&&t.versionId===first.versionId);
  const canModify=sameOwner&&targets.every(t=>t.canEdit);const editable=!pending&&!refreshing&&canModify;const graphics=targets.every(t=>t.canGraphics);
  const rename=single&&first.canRename;
  const nameValid=!rename||(name.trim().length>0&&Array.from(name.trim()).length<=120&&!/[\u0000-\u001f/\\]/.test(name));
  const changedName=rename&&name.trim()!==(first.name??"");const changedDescription=single&&first.target.kind==="DOCUMENT"&&description.trim()!==(first.description??"");
  const effectivePatch=useMemo(()=>Object.fromEntries(Object.entries(appearance).filter(([key,value])=>targets.some(t=>(t.appearance?.[key as keyof SurfaceStyle]??null)!==value))),[appearance,targets]);
  const changedGraphics=Object.keys(effectivePatch).length>0||clearFaces;
  const request=useMemo<PropertyUpdateRequest>(()=>({targets:targets.map(t=>t.target),...(changedName?{name:name.trim()}:{}),...(changedDescription?{description:description.trim()}:{}),...(Object.keys(effectivePatch).length?{appearance:effectivePatch}:{}),...(clearFaces?{clearFaceOverrides:true}:{})}),[targets,changedName,name,changedDescription,description,effectivePatch,clearFaces]);
  const valid=nameValid&&Array.from(description).length<=500&&Object.entries(effectivePatch).every(([key,value])=>value===null||!["color","edgeColor"].includes(key)||typeof value==="string"&&/^#[0-9a-fA-F]{6}$/.test(value));
  useEffect(()=>{
    if(!canModify||!changedGraphics||!valid){onPreview(undefined);setPreviewAppearances(undefined);return}
    const abort=new AbortController();let current=true;
    const timer=setTimeout(()=>{void onPreviewRequest(request,abort.signal).then(result=>{
      if(!current)return;setPreviewError(undefined);setPreviewAppearances(result.propertyAppearances);
      onPreview({documentId:first.documentId,versionId:first.versionId,occurrencePath:first.occurrencePath,appearances:result.propertyAppearances??[]});
    }).catch(error=>{if(current&&!abort.signal.aborted){onPreview(undefined);setPreviewError(error instanceof Error?error.message:String(error))}})},100);
    return ()=>{current=false;clearTimeout(timer);abort.abort()};
  },[request,canModify,changedGraphics,valid,onPreviewRequest,onPreview,first.documentId,first.versionId,first.occurrencePath,retry]);
  useEffect(()=>()=>onPreview(undefined),[onPreview]);
  const set=(key:keyof SurfaceStyle,value:string|number|boolean|null)=>setAppearance(s=>({...s,[key]:value}));
  const defaults:SurfaceStyle={lineStyle:"SOLID",pointSize:11,pointSymbol:"CROSS",color:first.systemColor??palette.solid,opacity:1,roughness:.5,metalness:.04,edges:true,pickable:true,edgeColor:palette.edge,edgeWidth:1,edgeOpacityMode:"FOLLOW"};
  const previewEntry=(target:PropertyEditorTarget)=>previewAppearances?.find(p=>{
    const path=target.target.instancePath?.canonical??"";
    const matches=path?p.occurrencePath===path||target.target.kind==="INSTANCE"&&p.occurrencePath.startsWith(path+"/"):p.documentId===target.documentId;
    if(!matches)return false;
    if(target.target.sourceResult)return p.sourceResult?.featureId===target.target.sourceResult.featureId&&p.sourceResult?.resultId===target.target.sourceResult.resultId;
    if(target.target.kind==='FEATURE')return p.sourceResult?.featureId===target.target.id;
    if(['GEOMETRICAL_SET','REFERENCE_POINT','PLANE','DATUM_AXIS'].includes(target.target.kind))return p.objectId===(target.target.kind==='PLANE'?'DATUM_PLANE':target.target.kind)+':'+target.target.id;
    return !p.sourceResult&&!p.objectId&&(target.target.kind==="BODY"?p.bodyId===target.target.id:target.target.kind==="FACE"?p.bodyId===target.target.bodyId:true);
  })?.appearance;
  const current=(key:keyof SurfaceStyle)=>{
    if(appearance[key]===null&&changedGraphics){
      if(first.target.kind==="DOCUMENT")return defaults[key];
      const projected=previewEntry(first);if(projected){
        const local=first.target.kind==="FACE"?projected.faceProperties?.[first.target.localId??0]:undefined;
        return key==="color"&&first.target.kind==="FACE"?projected.faces?.[first.target.localId??0]??projected.color??defaults.color:local?.[key as keyof import("../../types").GraphicProperties]??projected[key]??defaults[key];
      }
    }
    return appearance[key]??commonStyle(targets,key).value??defaults[key];
  };

  const mode=(key:keyof SurfaceStyle)=><Select aria-label={`${({color:"表面颜色",opacity:"表面透明度",edgeColor:"边线颜色",edgeOpacityMode:"透明边线",edgeWidth:"边线宽度",roughness:"粗糙度",metalness:"金属度",lineStyle:"线型",pointSize:"点大小",pointSymbol:"点符号",edges:"显示边线",pickable:"可选择性"})[key]}来源`} size="small" disabled={!editable} placeholder="多个值" value={key in appearance?appearance[key]===null?"inherit":"custom":targets.some(t=>(t.appearance?.[key]!==undefined)!==(first.appearance?.[key]!==undefined))?undefined:first.appearance?.[key]!==undefined?"custom":"inherit"}
    onChange={v=>set(key,v==="inherit"?null:current(key)!)} options={[{value:"inherit",label:"继承"},{value:"custom",label:"自定义"}]}/>;
  const color=(key:"color"|"edgeColor",label:string)=><CommandField label={label} width="content"><Space className="command-property-controls"><ColorPicker value={/^#[0-9a-fA-F]{6}$/.test(String(current(key)))?current(key) as string:defaults[key]} disabledAlpha disabled={!editable} onChange={v=>set(key,v.toHexString())}/><Input style={{width:94}} aria-label={key==="color"?"颜色 HEX":"边线 HEX"} maxLength={7} placeholder="多个值" value={commonStyle(targets,key).mixed&&!Object.hasOwn(appearance,key)?"":String(current(key))} disabled={!editable} status={/^#[0-9a-fA-F]{6}$/.test(String(current(key)))?undefined:"error"} onChange={e=>set(key,e.target.value)}/>{mode(key)}</Space></CommandField>;

  const numeric=(key:"roughness"|"metalness"|"edgeWidth"|"pointSize",label:string)=><CommandField label={label} width="content"><Space className="command-property-controls"><InputNumber aria-label={label} placeholder="多个值" value={commonStyle(targets,key).mixed&&!Object.hasOwn(appearance,key)?undefined:current(key) as number} min={key==="pointSize"?3:key==="edgeWidth"?.5:0} max={key==="pointSize"?32:key==="edgeWidth"?8:1} step={key==="pointSize"?1:key==="edgeWidth"?.5:.05} disabled={!editable} onChange={v=>{if(v!==null)set(key,v)}}/>{mode(key)}</Space></CommandField>;
  const projected=previewEntry(first);
  const colorSource=projected?(first.target.kind==="FACE"?projected.faceSources?.[first.target.localId??0]?.color??projected.sources?.color:projected.sources?.color):first.sources?.color;
  const sourceLabels:Record<string,string>={SYSTEM:"系统默认",DOCUMENT_DEFAULT:"零件默认外观",BODY:"实体外观",FACE:"分面外观",INSTANCE:"实例覆盖",INSTANCE_FACE:"实例分面覆盖",ASSEMBLY_DEFAULT:"装配默认外观",IMPORTED_INSTANCE:"导入的实例外观",GEOMETRICAL_SET:"几何图形集",GEOMETRY_RESULT:"几何结果",IMPORTED_RESULT:"导入外观",REFERENCE_POINT:"参考点",DATUM_PLANE:"参考面",DATUM_AXIS:"参考线"};
  const basic=<Space orientation="vertical" style={{width:"100%"}}>
    {rename?<CommandField  label={<><span>{first.target.kind==="DOCUMENT"?"文档名称":first.target.kind==="INSTANCE"?"实例名称":"名称"}</span></>} width="expression"><Input aria-label="属性名称" value={name} disabled={!editable} status={nameValid?undefined:"error"} onChange={e=>{setNameDirty(true);setName(e.target.value)}}/></CommandField>:<Typography.Text>{single?first.title:`已选择 ${targets.length} 个对象`}</Typography.Text>}
    {single&&first.target.kind==="DOCUMENT"&&<CommandField label="描述" width="expression"><Input.TextArea aria-label="文档描述" value={description} disabled={!editable} autoSize={{minRows:2,maxRows:5}} onChange={e=>{setDescriptionDirty(true);setDescription(e.target.value)}}/></CommandField>}
    {first.originalDocumentId&&<Button type="link" disabled={changedName||changedDescription||changedGraphics||pending} onClick={()=>onOpenOriginal?.(first.originalDocumentId!)}>打开原始文档</Button>}
  </Space>;
  const surfaceFields=targets.every(t=>!t.graphicDomain||['SURFACE','CONTAINER'].includes(t.graphicDomain));
  const curveFields=targets.every(t=>['CURVE','CONTAINER'].includes(t.graphicDomain??''));
  const pointFields=targets.every(t=>['POINT','CONTAINER'].includes(t.graphicDomain??''));
  const enumeration=(key:'lineStyle'|'pointSymbol',label:string,options:{value:string;label:string}[])=><CommandField label={label} width="content"><Space className="command-property-controls"><Select aria-label={label} disabled={!editable} value={current(key)} onChange={v=>set(key,v)} options={options}/>{mode(key)}</Space></CommandField>;
  const graphic=<div className="command-form">
    {color("color",curveFields?"曲线颜色":pointFields?"点颜色":"颜色")}
    <CommandField label="透明度" width="content"><Space className="command-property-controls"><InputNumber aria-label="表面透明度" min={0} max={100} placeholder="多个值" value={commonStyle(targets,"opacity").mixed&&!Object.hasOwn(appearance,"opacity")?undefined:Math.round((1-(current("opacity") as number))*100)} suffix="%" disabled={!editable} onChange={v=>{if(v!==null)set("opacity",1-v/100)}}/>{mode("opacity")}</Space></CommandField>
    {curveFields&&<>{numeric('edgeWidth','线宽（px）')}{enumeration('lineStyle','线型',[{value:'SOLID',label:'实线'},{value:'DASHED',label:'虚线'}])}</>}
    {pointFields&&<>{numeric('pointSize','点大小（px）')}{enumeration('pointSymbol','点符号',[{value:'CROSS',label:'叉号'},{value:'PLUS',label:'十字'},{value:'DOT',label:'圆点'}])}</>}
    {surfaceFields&&<>
      {targets.every(t=>t.target.kind!=='FACE')&&<><Checkbox disabled={!editable} checked={current('edges') as boolean} onChange={e=>set('edges',e.target.checked)}>显示边线</Checkbox>{color('edgeColor','边线颜色')}{numeric('edgeWidth','边线宽度（px）')}</>}
      {numeric('roughness','粗糙度')}{numeric('metalness','金属度')}
    </>}
    <Checkbox disabled={!editable} indeterminate={commonStyle(targets,"pickable").mixed&&!Object.hasOwn(appearance,"pickable")} checked={current("pickable") as boolean} onChange={e=>set("pickable",e.target.checked)}>允许从视区选择</Checkbox>
    {targets.every(t=>['BODY','FEATURE','INSTANCE','DOCUMENT'].includes(t.target.kind))&&targets.some(t=>Boolean(t.hasFaceOverrides||t.appearance?.faces?.length))&&<Checkbox disabled={!editable} checked={clearFaces} onChange={e=>setClearFaces(e.target.checked)}>清除分面覆盖</Checkbox>}
    <Button type="link" disabled={!editable} onClick={()=>setAppearance(Object.fromEntries(Object.keys(defaults).map(key=>[key,null])))}>恢复继承</Button>
    <Typography.Text type="secondary">颜色来源：{appearance.color!==undefined&&appearance.color!==null?'当前修改':sourceLabels[colorSource?.kind??'SYSTEM']??'对象属性'}</Typography.Text>
    {Boolean(projected?.unresolvedFaces||projected?.issues?.length)&&<Alert type="warning" title="部分外观引用已失效"/>}
  </div>;
  const info=<><Descriptions column={1} size="small" items={[{key:"scope",label:"作用范围",children:first.scope},{key:"object",label:"对象",children:first.title},{key:"path",label:"所在位置",children:first.target.instancePath?.display??"当前文档"}]}/>{targets.every(target=>target.target.kind==="INSTANCE")&&<CommandField label="属性基线" width="content"><Button aria-label="加载最新属性（保留修改）" type="link" disabled={pending} loading={refreshing} onClick={async()=>{setRefreshing(true);try{await onRefresh();setCommitError(undefined);setPreviewError(undefined);setRetry(v=>v+1)}catch(error){setCommitError(error instanceof Error?error.message:String(error))}finally{setRefreshing(false)}}}>加载最新属性（保留修改）</Button></CommandField>}</>;
  return <CommandDialog id="object-properties" open title={`属性 · ${single?first.title:`${targets.length} 个对象`}`} size="M" onClose={onClose} confirmLoading={pending} confirmDisabled={!editable||!valid||!(changedName||changedDescription||changedGraphics)} onConfirm={async()=>{try{await onApply(request)}catch(error){setCommitError(error instanceof Error?error.message:String(error));throw error}}}>
    {!canModify&&<Alert type="info" title={sameOwner?first.reason??"当前对象只读":"批量修改需要同一写入文档。请分别编辑原始文档，或使用当前装配的实例覆盖。"}/>}
    {previewError&&<Alert type="error" title="外观预览失败" description={engineeringProblem(previewError)} action={<Button size="small" onClick={()=>setRetry(v=>v+1)}>重试</Button>}/>}
    {commitError&&<Alert type="error" title="属性尚未保存，修改已保留" description={engineeringProblem(commitError)}/>}

    <Tabs defaultActiveKey={graphics&&first.target.kind!=="DOCUMENT"?"graphic":"basic"} items={[{key:"basic",label:"基本",children:basic},...(graphics?[{key:"graphic",label:"图形",children:graphic}]:[]),{key:"info",label:"信息",children:info}]}/>
  </CommandDialog>;
}
