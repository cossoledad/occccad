import type {DocumentView,InstancePath} from '../types';
export type ContextEditScope={documentId:string;rootProductDocumentId:string;rootProductRevisionId:string;contextVariantId:string;instancePath:InstancePath};
let active:ContextEditScope|undefined;
export function setContextEditScope(value:ContextEditScope|undefined){active=value;return ()=>{if(active===value)active=undefined}};
export function contextEditScope(documentId:string){return active?.documentId===documentId?active:undefined}
export function contextCommand<T extends object>(documentId:string,input:T):T&Partial<ContextEditScope>{const scope=contextEditScope(documentId);if(!scope)return input;const {documentId:_owner,...accepted}=scope;return {...accepted,...input}}
/** Shared document caches contain only complete independent views. */
export function baseDocumentView(view:DocumentView):DocumentView {return view.baseView??view}
export function acceptedContextView(input:DocumentView,root:DocumentView,path:string):DocumentView {
 const base=baseDocumentView(input);
 const variant=root.contextVariants?.find(v=>v.owningInstancePath.canonical===path&&v.baseDocumentId===base.document.id);
 if(!variant)return base;
 const common={...base,baseView:base,contextOccurrencePath:path,contextProduct:root,document:{...base.document,versionId:variant.baseRevisionId}};
 if(variant.status!=='READY'||!variant.part)return {...common,part:undefined,datumPlanes:[],datumAxes:[],axisSystems:[],sketchAnalyses:{},artifacts:{},artifact:undefined,replicationSources:undefined,sketchPatternMembers:undefined};
 const artifacts={...base.artifacts,...root.artifacts};
 const activeBody=variant.part.bodies?.find(b=>b.id===variant.part!.activeBodyId);
 return {...common,artifact:activeBody?.geometryKey?artifacts[activeBody.geometryKey]:undefined,replicationSources:variant.replicationSources,sketchPatternMembers:variant.sketchPatternMembers,part:variant.part,datumPlanes:variant.part.datumPlanes,datumAxes:variant.part.datumAxes,axisSystems:variant.part.axisSystems,sketchAnalyses:variant.sketchAnalyses,artifacts};
}
