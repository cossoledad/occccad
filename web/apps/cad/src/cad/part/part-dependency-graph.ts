import type {DocumentView,InstancePath,PersistentSelection} from '../../types';
import {contextScopeForView} from '../../api/context-edit-scope';

export type PartDependencyInput={role:string;kind:string;sourceId?:string;sourceName?:string;regionId?:string;entityId?:string;state:string;selection?:PersistentSelection};
export type PartDependencyNode={key:string;type:string;label:string;entityId?:string;ownerFeatureId?:string;bodyId?:string;state:string;diagnostic?:string;inputs?:PartDependencyInput[]};
export type PartDependencyEdge={source:string;target:string;kind:string};
export type PartDependencies={documentId:string;versionId:string;digest:string;contextVariantId?:string;rootProductDocumentId?:string;rootProductRevisionId?:string;instancePath?:InstancePath;nodes:PartDependencyNode[];edges:PartDependencyEdge[]};
export type DependencyDirection='upstream'|'downstream'|'both';
export const dependencyWarning=(state:string)=>['FAILED','BLOCKED','FAILED_SUPPORT','MISSING','BROKEN','UNAVAILABLE','STALE','INVALID'].includes(state);
export const dependencyStateLabel=(state:string)=>({FAILED:'失败',BLOCKED:'受上游阻断',FAILED_SUPPORT:'支撑失效',MISSING:'引用缺失',BROKEN:'关联失效',UNAVAILABLE:'不可用',STALE:'待更新',INVALID:'无效',SUPPRESSED:'已抑制',DEFINED:'已定义',AVAILABLE:'可用',OPEN:'开放',EMPTY:'空',CURRENT:'当前',CONNECTED:'已连接',CLEAN:'当前',SUCCEEDED:'成功',EVALUATED:'已求值'}[state]??state);
export const dependencyTypeLabel=(type:string)=>({SKETCH:'草图',PROFILE:'轮廓',PARAMETER:'参数',DATUM_PLANE:'基准面',DATUM_AXIS:'基准轴',AXIS_SYSTEM:'轴系',REFERENCE_POINT:'参考点',BODY:'实体',LINEAR_EXTRUDE:'拉伸',PAD:'拉伸',REVOLVE:'旋转',LOFT:'放样',HOLE:'孔',BOOLEAN:'布尔',FILLET:'圆角',CHAMFER:'倒角',DRAFT:'拔模',SHELL:'抽壳',SOLID_PATTERN:'实体阵列',SKETCH_PATTERN:'草图阵列',EXTERNAL_PROJECTION:'投影',CONTEXT_INPUT:'设计输入',CONTEXT_REFERENCE_SNAPSHOT:'上下文引用',EXTERNAL_PARAMETER_SNAPSHOT:'外部参数',PUBLICATION:'发布',LOFT_CORRESPONDENCE:'放样对应',APPEARANCE:'外观',METADATA:'元数据'}[type]??'设计对象');

export function partDependencySnapshotMatches(view:DocumentView,data:PartDependencies):boolean {
 const scope=contextScopeForView(view);
 return data.documentId===view.document.id&&data.versionId===view.document.versionId&&
  (data.contextVariantId??'')===(scope?.contextVariantId??'')&&(data.instancePath?.canonical??'')===(view.contextOccurrencePath??'')&&
  (data.rootProductDocumentId??'')===(scope?.rootProductDocumentId??'')&&(data.rootProductRevisionId??'')===(scope?.rootProductRevisionId??'');
}

/** Traverse formal directed dependencies independently in each direction.
 * Common parameters must not pull unrelated consumers into an upstream view. */
export function partDependencySlice(graph:PartDependencies,root:string,direction:DependencyDirection,limit=200,includeDisplay=false){
 const incoming=new Map<string,string[]>(),outgoing=new Map<string,string[]>(),index=new Map(graph.nodes.map(n=>[n.key,n]));
 for(const e of graph.edges){incoming.set(e.target,[...(incoming.get(e.target)??[]),e.source]);outgoing.set(e.source,[...(outgoing.get(e.source)??[]),e.target]);}
 const levels=new Map<string,number>();if(index.has(root))levels.set(root,0);
 const walk=(links:Map<string,string[]>,sign:number)=>{
  const visited=new Set([root]),queue=[root];
  for(let i=0;i<queue.length;i++){const id=queue[i],level=levels.get(id)??0;for(const next of (links.get(id)??[]).slice().sort())if(!visited.has(next)){visited.add(next);queue.push(next);if(!levels.has(next))levels.set(next,level+sign);}}
 };
 if(direction!=='downstream')walk(incoming,-1);if(direction!=='upstream')walk(outgoing,1);
 const eligible=[...levels.keys()].filter(id=>includeDisplay||!['APPEARANCE','METADATA'].includes(index.get(id)?.type??''));
 eligible.sort((a,b)=>Math.abs(levels.get(a)!)-Math.abs(levels.get(b)!)||a.localeCompare(b));
 const budget=Math.max(1,Math.min(1000,Math.floor(limit))),keys=new Set(eligible.slice(0,budget)),rows=new Map<number,number>();
 const nodes=eligible.filter(id=>keys.has(id)).map(id=>{const n=index.get(id)!,level=levels.get(id)!,row=rows.get(level)??0;rows.set(level,row+1);return {...n,position:{x:level*260,y:row*76}}});
 const edges=graph.edges.filter(e=>keys.has(e.source)&&keys.has(e.target));
 const causes=nodes.filter(n=>dependencyWarning(n.state)&&n.state!=='BLOCKED'&&!edges.some(e=>e.target===n.key&&dependencyWarning(index.get(e.source)?.state??'')));
 return {nodes,edges,causes,omitted:eligible.length-nodes.length,total:eligible.length};
}
