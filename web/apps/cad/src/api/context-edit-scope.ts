import type {DocumentView,InstancePath} from '../types';
export type ContextEditScope={documentId:string;rootProductDocumentId:string;rootProductRevisionId:string;contextVariantId:string;instancePath:InstancePath};
let active:ContextEditScope|undefined;
export function setContextEditScope(value:ContextEditScope|undefined){active=value;return ()=>{if(active===value)active=undefined}};
export function contextEditScope(documentId:string){return active?.documentId===documentId?active:undefined}
/** Snapshot from the accepted view, independent of the mutable command router. */
export function contextScopeForView(view:DocumentView):ContextEditScope|undefined {
 const root=view.contextProduct,path=view.contextOccurrencePath;
 if(!root||!path)return;
 const variant=root.contextVariants?.find(v=>v.owningInstancePath.canonical===path&&v.baseDocumentId===view.document.id);
 if(!variant?.part||variant.status!=='READY'||variant.baseRevisionId!==view.document.versionId||variant.owningInstancePath.segments.length!==1)return;
 const instance=root.product?.instances.find(i=>i.id===variant.owningInstancePath.segments[0].instanceId);
 if(!instance||instance.referenceMode==='PINNED')return;
 return {documentId:view.document.id,rootProductDocumentId:root.document.id,rootProductRevisionId:root.document.versionId,contextVariantId:variant.variantKey,instancePath:variant.owningInstancePath};
}
export function viewContextKey(view:DocumentView):string {return JSON.stringify([view.document.id,view.document.versionId,view.contextOccurrencePath,contextScopeForView(view)]);}
export function commandForView<T extends object>(view:DocumentView,input:T):T&Partial<ContextEditScope> {
 const scope=contextScopeForView(view);
 if(view.contextOccurrencePath&&!scope)throw new Error('关联输入尚未就绪，请等待更新后重试');
 if(!scope)return input;
 const {documentId:_owner,...frozen}=scope;return {...frozen,...input};
}
/** A reply may advance its own baseline, but cannot rewind a later accepted scope. */
export function contextResultMatchesActiveView(current:DocumentView|undefined,submitted:{baseVersionId?:string;context?:Omit<ContextEditScope,'documentId'>},result:DocumentView):boolean {
 if(!submitted.context)return true;
 if(!current)return false;
 const scope=contextScopeForView(current),expected=submitted.context;
 if(!scope||scope.rootProductDocumentId!==expected.rootProductDocumentId||scope.instancePath.canonical!==expected.instancePath.canonical)return false;
 const unchanged=current.document.versionId===submitted.baseVersionId&&scope.rootProductRevisionId===expected.rootProductRevisionId&&scope.contextVariantId===expected.contextVariantId;
 return unchanged||viewContextKey(current)===viewContextKey(result);
}
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
