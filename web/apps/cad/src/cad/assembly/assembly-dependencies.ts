import type {AssemblyConstraint,DocumentView,InstancePath} from '../../types';
import {engineeringInstanceName} from './assembly-engineering-state';

export type DependencyCategory='CONSTRAINT'|'ASSOCIATIVITY'|'RELATION';
export type DependencyNode={id:string;kind:'assembly'|'component'|'constraint'|'association'|'parameter'|'formula'|'external';label:string;caption:string;category?:DependencyCategory;state?:string;suppressed?:boolean;hidden?:boolean;missing?:boolean;instanceId?:string;constraintId?:string;parameterId?:string;bindingId?:string;expression?:string;sourceDocumentId?:string;sourceRevisionId?:string};
export type DependencyEdge={id:string;source:string;target:string;category?:DependencyCategory;role:string;directed:boolean};
export type AssemblyDependencyGraph={documentId:string;revisionId:string;ownerOccurrence:string;rootId:string;nodes:DependencyNode[];edges:DependencyEdge[]};
const constraintTypes:Record<AssemblyConstraint['kind'],string>={FIX:'固定',RIGID:'固联',FIX_TOGETHER:'固联组',COINCIDENT:'重合',CONTACT:'接触',CONCENTRIC:'同轴',ANGLE:'角度',DISTANCE:'偏移'};
export function dependencyState(node:DependencyNode):string{
 if(node.suppressed)return '停用';
 const labels:Record<string,string>={VERIFIED:'已满足',BROKEN:'支持断开',BROKEN_PUBLICATION:'支持断开',NOT_UPDATED:'待更新',IMPOSSIBLE:'未接纳',CONNECTED:'已连接',PENDING:'待检查',UNAVAILABLE:'引用信息不可用'};
 return node.missing?'引用已丢失':labels[node.state??'']??'';
}
/** Read-only projection of the owning Product's accepted definitions. Graph
 * addresses use stable entity IDs and occurrence scope, never labels/indices.
 * Constraint connectivity is undirected; it is not a solver execution order. */
export function assemblyDependencies(view:DocumentView,ownerOccurrence=''):AssemblyDependencyGraph{
 const key=(kind:string,id:string)=>JSON.stringify([view.document.id,ownerOccurrence,kind,id]);
 const nodes=new Map<string,DependencyNode>(),edges=new Map<string,DependencyEdge>();
 const rootId=key('assembly',view.document.id),instances=new Set(view.product?.instances.map(i=>i.id)??[]);
 const add=(node:DependencyNode)=>{if(!nodes.has(node.id))nodes.set(node.id,node);return node.id;};
 const connect=(source:string,target:string,role:string,category?:DependencyCategory,directed=false)=>{const id=JSON.stringify([source,target,role]);edges.set(id,{id,source,target,role,category,directed});};
 add({id:rootId,kind:'assembly',label:view.document.name,caption:'当前装配'});
 const component=(id:string)=>add({id:key('component',id),kind:'component',instanceId:id,label:instances.has(id)?engineeringInstanceName(view,id):'缺失组件',caption:'组件',missing:!instances.has(id)});
 for(const i of view.product?.instances??[])connect(rootId,component(i.id),'contains');
 const localInstance=(path:InstancePath|undefined,fallback?:string)=>{
  if(!path)return fallback;
  const prefix=path.rootDocumentId===view.document.id?[]:ownerOccurrence.split('/').filter(Boolean);
  if(prefix.some((id,i)=>path.segments[i]?.instanceId!==id))return undefined;
  const segment=path.segments[prefix.length];return segment?.ownerDocumentId===view.document.id?segment.instanceId:undefined;
 };
 const definitions=new Map((view.product?.constraints??[]).map(c=>[c.id,c]));
 const constraint=(id:string)=>{const c=definitions.get(id);return add({id:key('constraint',id),kind:'constraint',constraintId:id,label:c?.name??(c?constraintTypes[c.kind]:'缺失约束'),caption:c?`${constraintTypes[c.kind]}${c.mode==='MEASURED'?' · 测量':c.kind==='FIX'?c.fixMode==='RELATIVE'?' · 相对':' · 空间':''}`:'约束',category:'CONSTRAINT',state:c?.evaluationStatus,suppressed:c?.suppressed,hidden:c?.visible===false,missing:!c});};
 for(const c of definitions.values()){
  const target=constraint(c.id);let local=false;
  const support=(ref:AssemblyConstraint['first']|undefined,role:string)=>{if(!ref)return;const id=localInstance(ref.instancePath,ref.instanceId);if(id){local ||= instances.has(id);connect(component(id),target,role,'CONSTRAINT');}};
  support(c.first,'support-A');support(c.second,'support-B');support(c.angleAxis,'angle-axis');
  for(const m of c.groupMembers??[]){if(m.groupId){local ||=definitions.has(m.groupId);connect(constraint(m.groupId),target,`member-group:${m.groupId}`,'CONSTRAINT');}else {const id=localInstance(m.instancePath,m.instanceId);if(id){local ||=instances.has(id);connect(component(id),target,`member:${id}`,'CONSTRAINT');}}}
  if(!local)connect(rootId,target,'contains','CONSTRAINT');
 }
 for(const b of view.product?.contextBindings??[]){
  const owner=localInstance(b.owningInstancePath),source=localInstance(b.sourceInstancePath);
  const relation=add({id:key('association',b.id),kind:'association',bindingId:b.id,label:b.name||'上下文关联',caption:'上下文关联',category:'ASSOCIATIVITY',state:b.resolution?.status});
  const outside=(role:string)=>add({id:key('external',`${b.id}:${role}`),kind:'external',label:role==='source'?'层级外的源组件':'层级外的接收组件',caption:'上下文关联',category:'ASSOCIATIVITY',state:'UNAVAILABLE'});
  connect(source?component(source):outside('source'),relation,'source','ASSOCIATIVITY',true);
  connect(relation,owner?component(owner):outside('consumer'),'consumer','ASSOCIATIVITY',true);
  if(!source&&!owner)connect(rootId,relation,'contains','ASSOCIATIVITY');
 }
 const parameters=new Map((view.product?.parameters??[]).map(p=>[p.parameterId,p]));
 const parameter=(id:string)=>{const p=parameters.get(id);return add({id:key('parameter',id),kind:'parameter',parameterId:id,label:p?.displayAlias??p?.displayName??p?.label??p?.key??'缺失参数',caption:'装配参数',category:'RELATION',missing:!p});};
 for(const p of parameters.values()){
  const target=parameter(p.parameterId);connect(rootId,target,'contains','RELATION');
  if(p.ownerFeatureId&&definitions.has(p.ownerFeatureId))connect(target,constraint(p.ownerFeatureId),'drives','RELATION',true);
  if(p.source.expression){
   const expression=p.source.expression,relation=add({id:key('formula',p.parameterId),kind:'formula',label:`公式 · ${nodes.get(target)!.label}`,caption:'公式',category:'RELATION',expression:expression.sourceText,state:expression.reads?'CONNECTED':'UNAVAILABLE'});
   connect(relation,target,'result','RELATION',true);
   // Only checked, stable dependency reads are authoritative. Source text is
   // display-only; names are never parsed to invent graph links.
   for(const read of expression.reads??[])if(read.startsWith('parameter:'))connect(parameter(read.slice('parameter:'.length)),relation,'input','RELATION',true);
  }
  if(p.source.external){const e=p.source.external,source=add({id:key('external',JSON.stringify([e.sourceDocumentId,e.publicationId])),kind:'external',label:'外部参数发布',caption:'参数引用',category:'RELATION',sourceDocumentId:e.sourceDocumentId,sourceRevisionId:e.resolvedRevisionId,state:e.resolutionSnapshot?.status??'PENDING'});connect(source,target,'external-reference','RELATION',true);}
 }
 return {documentId:view.document.id,revisionId:view.document.versionId,ownerOccurrence,rootId,nodes:[...nodes.values()],edges:[...edges.values()]};
}

export type DependencyFilter={categories:DependencyCategory[];includeSuppressed:boolean};
/** Cycles are normal in assembly networks. Expansion is a visited traversal,
 * not recursive copies of the same occurrence, constraint or group. */
export function dependencySlice(graph:AssemblyDependencyGraph,root:string,expanded:ReadonlySet<string>,filter:DependencyFilter){
 const allowed=new Map(graph.nodes.filter(n=>(!n.category||filter.categories.includes(n.category))&&(filter.includeSuppressed||!n.suppressed)).map(n=>[n.id,n]));
 const rootId=allowed.has(root)?root:graph.rootId;
 const adjacency=new Map<string,DependencyEdge[]>();
 for(const e of graph.edges){
  if((e.category&&!filter.categories.includes(e.category))||!allowed.has(e.source)||!allowed.has(e.target))continue;
  for(const id of e.role==='contains'?[e.source]:[e.source,e.target]){const list=adjacency.get(id)??[];list.push(e);adjacency.set(id,list);}
 }
 const visited=new Set([rootId]),ranks=new Map([[rootId,0]]),queue=[rootId],edges=new Map<string,DependencyEdge>();
 for(let i=0;i<queue.length;i++){
  const id=queue[i];if(!expanded.has(id))continue;
  for(const e of adjacency.get(id)??[]){
   const next=e.source===id?e.target:e.source;
   edges.set(e.id,e);if(visited.has(next))continue;
   visited.add(next);ranks.set(next,ranks.get(id)!+1);queue.push(next);
  }
 }
 return {rootId,nodes:queue.map(id=>allowed.get(id)!).filter(Boolean),edges:[...edges.values()],ranks};
}
