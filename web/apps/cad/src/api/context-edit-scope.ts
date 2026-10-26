import type {DocumentView,InstancePath} from '../types';
export type ContextEditScope={documentId:string;rootProductDocumentId:string;rootProductRevisionId:string;contextVariantId:string;instancePath:InstancePath};
let active:ContextEditScope|undefined;
export function setContextEditScope(value:ContextEditScope|undefined){active=value;return ()=>{if(active===value)active=undefined}};
export function contextEditScope(documentId:string){return active?.documentId===documentId?active:undefined}
export function contextCommand<T extends object>(documentId:string,input:T):T&Partial<ContextEditScope>{const scope=contextEditScope(documentId);if(!scope)return input;const {documentId:_owner,...accepted}=scope;return {...accepted,...input}}
export function acceptedContextView(base:DocumentView,root:DocumentView,path:string):DocumentView {
 const variant=root.contextVariants?.find(v=>v.owningInstancePath.canonical===path&&v.baseDocumentId===base.document.id);
 if(!variant)return base;
 if(variant.status!=='READY'||!variant.part)return {...base,part:undefined,contextProduct:root};
 return {...base,document:{...base.document,versionId:variant.baseRevisionId},basePart:base.part,contextProduct:root,part:variant.part,datumPlanes:variant.part.datumPlanes,datumAxes:variant.part.datumAxes,axisSystems:variant.part.axisSystems,sketchAnalyses:variant.sketchAnalyses,artifacts:{...base.artifacts,...root.artifacts}};
}
