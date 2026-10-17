import type { DocumentView, PropertyTarget, ResolvedAppearance, SelectionItem, SurfaceAppearance, SurfaceStyle } from "../../types";
import { occurrenceSnapshot, refreshOccurrenceSelection } from "../../cad/assembly/assembly-occurrence-snapshot";
import { palette } from "../../design/visual-tokens";

export type PropertyEditorTarget={
  target:PropertyTarget;documentId:string;versionId:string;occurrencePath?:string;selection?:SelectionItem;systemColor?:string;
  title:string;name?:string;description?:string;scope:string;
  appearance?:SurfaceAppearance;effective?:SurfaceStyle;sources?:ResolvedAppearance["sources"];canEdit:boolean;canRename:boolean;canGraphics:boolean;
  originalDocumentId?:string;reason?:string;
};

export function propertyEditorTarget(view:DocumentView,editingView:DocumentView|undefined,selection:SelectionItem|undefined,
  activePath:string|undefined,canEdit:boolean,canEditRoot:boolean):PropertyEditorTarget {
  const path=selection?.instancePath,occurrence=path?.canonical??selection?.occurrencePath??"";
  const own=editingView&&selection&&(selection.ownerDocumentId??selection.documentId)===editingView.document.id&&occurrence===(activePath??"");
  const source=own?editingView:view;
  const root=!selection||((selection.kind==="part"||selection.kind==="product")&&!occurrence&&selection.documentId===view.document.id);
  const base:PropertyEditorTarget={target:{kind:"DOCUMENT",id:source.document.id},documentId:source.document.id,versionId:source.document.versionId,
    title:source.document.name,systemColor:view.product?palette.productSolid:palette.solid,scope:"文档内容",canEdit:own?canEdit:canEditRoot,canRename:true,canGraphics:true};
  if(selection?.kind==="kinematic-object"&&view.product?.kinematics){
    const k=view.product.kinematics,id=selection.entityId??selection.id;
    const object=[...k.mechanisms,...k.mechanisms.flatMap(m=>m.joints),...(k.drivers??[]),...k.studies].find(o=>o.id===id);
    if(object)return {...base,selection,target:{kind:"KINEMATIC_OBJECT",id},documentId:view.document.id,versionId:view.document.versionId,
      title:object.name,name:object.name,scope:"当前装配中的机构定义",canEdit:canEditRoot,canRename:true,canGraphics:false};
  }
  if(root)return {...base,target:{kind:"DOCUMENT",id:view.document.id},documentId:view.document.id,versionId:view.document.versionId,title:view.document.name,name:view.document.name,description:view.document.description,appearance:view.part?.defaultAppearance??view.product?.defaultAppearance,effective:view.part?.defaultAppearance??view.product?.defaultAppearance,sources:{color:{kind:(view.part?.defaultAppearance??view.product?.defaultAppearance)?.color?"DOCUMENT_DEFAULT":"SYSTEM"}},scope:"文档默认外观",canEdit:canEditRoot};
  if(!selection)return base;
  const bodyID=selection.bodyId??(selection.kind==="body"?selection.entityId:undefined);
  const part=source.part,body=part?.bodies.find(b=>b.id===bodyID);
  const instanceScoped=Boolean(view.product&&path?.segments.length&&!own);
  const instanceKind=["instance","part","product"].includes(selection.kind);
  const kind=instanceKind?"INSTANCE":selection.kind==="face"?"FACE":["body","solid"].includes(selection.kind)?"BODY":selection.kind==="plane"?"PLANE":selection.kind==="axis"?"DATUM_AXIS":["feature","pad","import","sketch"].includes(selection.kind)?"FEATURE":selection.kind.toUpperCase();
  const id=instanceKind?path?.segments.at(-1)?.instanceId??selection.entityId??selection.id:kind==="BODY"?bodyID??selection.id:selection.entityId??selection.id;
  const effective=instanceScoped?view.resolvedInstances?.find(b=>b.occurrencePath===occurrence&&(!bodyID||b.bodyId===bodyID))?.appearance:source.bodyAppearances?.[bodyID??""];
  let local=body?.appearance;
  let name=kind==="BODY"?body?.name:kind==="FEATURE"?part?.features.find(f=>f.id===id)?.name:kind==="PLANE"?part?.datumPlanes.find(p=>p.id===id)?.name:kind==="DATUM_AXIS"?part?.datumAxes?.find(p=>p.id===id)?.name:undefined;
  if(instanceScoped){
    const overridden=view.product?.appearanceOverrides?.find(o=>o.instancePath.canonical===occurrence&&(o.bodyId??"")===(instanceKind?"":bodyID??""));
    local=overridden?.appearance;
    if(instanceKind)name=overridden?.name??(path!.segments.length===1?view.product?.instances.find(i=>i.id===id)?.name:undefined)??path!.segments.at(-1)?.instanceName;
  }
  if(kind==="FACE"&&selection.kind==="face"){
    const properties=effective?.faceProperties?.[selection.topologyId];const color=effective?.faces?.[selection.topologyId];
    local=properties||color?{...properties,...(color?{color}:{})}:undefined;
  }
  const faceStyle=kind==="FACE"&&selection.kind==="face"?{...effective,...effective?.faceProperties?.[selection.topologyId],color:effective?.faces?.[selection.topologyId]??effective?.color}:effective;
  const editable=instanceScoped?canEditRoot:Boolean(own&&canEdit);
  const surface=["INSTANCE","BODY","FACE"].includes(kind);
  const protectedPlane=kind==="PLANE"&&!part?.datumPlanes.find(p=>p.id===id)?.definition;
  const supportedName=!protectedPlane&&(instanceScoped?kind==="INSTANCE":["BODY","FEATURE","PLANE","DATUM_AXIS"].includes(kind)&&Boolean(name));
  const target:PropertyTarget={kind,id,...(instanceScoped?{instancePath:path}:{}),...(kind==="FACE"&&selection.kind==="face"?{bodyId:bodyID,geometryKey:selection.geometryKey,localId:selection.topologyId}:{})};
  return {...base,target,selection,documentId:instanceScoped?view.document.id:source.document.id,versionId:instanceScoped?view.document.versionId:source.document.versionId,
    occurrencePath:instanceScoped?undefined:occurrence,title:name??(kind==="FACE"?"所选面":selection.entityKind??selection.kind),name,
    appearance:local,effective:faceStyle,sources:kind==="FACE"&&selection.kind==="face"?{...effective?.sources,...effective?.faceSources?.[selection.topologyId]}:effective?.sources,scope:instanceScoped?"仅当前实例":"当前零件定义",canEdit:editable,canRename:supportedName,canGraphics:surface,
    originalDocumentId:instanceScoped?path?.segments.at(-1)?.referencedDocumentId:undefined,
    reason:!editable?"当前上下文只读；请在有编辑权限的文档中修改。":undefined};
}

export function refreshPropertyTargets(targets:PropertyEditorTarget[],view:DocumentView,editingView:DocumentView|undefined,
  activePath:string|undefined,canEdit:boolean,canEditRoot:boolean):PropertyEditorTarget[] {
  return targets.map(target=>{
    const selection=target.selection;
    if(!selection)return propertyEditorTarget(view,editingView,undefined,activePath,canEdit,canEditRoot);
    const path=selection.instancePath?.canonical??selection.occurrencePath;
    if(path){
      const current=occurrenceSnapshot(view,path);
      if(!current||current.segments.at(-1)?.referencedDocumentId!==selection.instancePath?.segments.at(-1)?.referencedDocumentId)
        throw new Error("所选实例已删除或替换，请关闭属性并重新选择。");
    }
    const current=refreshOccurrenceSelection(view,selection);
    if(target.target.kind==="FACE"){
      const key=path?view.resolvedInstances?.find(v=>v.occurrencePath===path&&v.bodyId===target.target.bodyId)?.geometryKey:
        (editingView??view).part?.bodies.find(b=>b.id===target.target.bodyId)?.geometryKey;
      if(key!==target.target.geometryKey)throw new Error("所选面的几何已变化，请关闭属性并重新选择。草稿未写入文档。");
    }
    const next=propertyEditorTarget(view,editingView,current,activePath,canEdit,canEditRoot);
    if(next.documentId!==target.documentId||next.target.kind!==target.target.kind||next.target.id!==target.target.id)
      throw new Error("属性目标已变化，请重新选择。");
    return next;
  });
}

export function commonStyle(targets:PropertyEditorTarget[],key:keyof SurfaceStyle):{mixed:boolean;value:SurfaceStyle[typeof key]} {
  const values=targets.map(t=>t.effective?.[key]);return {mixed:values.some(v=>v!==values[0]),value:values[0]};
}
