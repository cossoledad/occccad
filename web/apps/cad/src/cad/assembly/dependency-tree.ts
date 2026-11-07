import type {AssemblyDependencyGraph,DependencyEdge,DependencyFilter,DependencyNode} from './assembly-dependencies';

export type DependencyTreeItem={id:string;entity:DependencyNode;parentId?:string;reference:boolean;depth:number;width:number;position:{x:number;y:number}};
export type DependencyTreeLink={id:string;source:string;target:string;category?:DependencyEdge['category'];directed:boolean;reverse:boolean};
/** Display occurrences are separate from domain identity. Each entity expands
 * once; remaining incident edges terminate in repeated references, bounding
 * cyclic networks by O(V + E), rather than enumerating all possible paths. */
export function dependencyTree(graph:AssemblyDependencyGraph,root:string,expanded:ReadonlySet<string>,filter:DependencyFilter){
 const allowed=new Map(graph.nodes.filter(n=>(!n.category||filter.categories.includes(n.category))&&(filter.includeSuppressed||!n.suppressed)).map(n=>[n.id,n]));
 const rootId=allowed.has(root)?root:graph.rootId;
 const adjacency=new Map<string,DependencyEdge[]>();
 for(const edge of graph.edges){
  if(edge.role==='contains'||(edge.category&&!filter.categories.includes(edge.category))||!allowed.has(edge.source)||!allowed.has(edge.target))continue;
  for(const id of new Set([edge.source,edge.target])){const list=adjacency.get(id)??[];list.push(edge);adjacency.set(id,list);}
 }
 // An assembly is an analysis scope, not an extra component in the network:
 // show its relationships first, then their actual supports. Keep isolated
 // objects inspectable as well, including broken and unconstrained objects.
 if(rootId===graph.rootId){
  const children=[...allowed.values()].filter(n=>n.id!==rootId&&(['constraint','association','formula'].includes(n.kind)||!adjacency.has(n.id)));
  adjacency.set(rootId,children.map(n=>({id:JSON.stringify(['scope',rootId,n.id]),source:rootId,target:n.id,role:'scope',category:n.category,directed:false})));
 }
 const items:DependencyTreeItem[]=[],links:DependencyTreeLink[]=[],claimed=new Set<string>([rootId]),displayIndex=new Map<string,DependencyTreeItem>(),children=new Map<string,DependencyTreeItem[]>();
 const add=(entity:DependencyNode,id:string,depth:number,parentId?:string,reference=false)=>{
  const textWidth=[...entity.label].reduce((sum,c)=>sum+(c.charCodeAt(0)>255?13:7),0);
  const item:DependencyTreeItem={id,entity,parentId,reference,depth,width:Math.min(246,Math.max(100,textWidth+48)),position:{x:0,y:0}};
  items.push(item);displayIndex.set(id,item);if(parentId){const list=children.get(parentId)??[];list.push(item);children.set(parentId,list);}return item;
 };
 const entity=allowed.get(rootId);if(!entity)return {rootId,items,links};
 add(entity,JSON.stringify(['root',rootId]),0);
 for(let i=0;i<items.length;i++){
  const item=items[i];if(item.reference||!expanded.has(item.entity.id))continue;
  for(const edge of adjacency.get(item.entity.id)??[]){
   const next=edge.source===item.entity.id?edge.target:edge.source;
   if(item.parentId&&displayIndex.get(item.parentId)?.entity.id===next)continue;
   const target=allowed.get(next);if(!target)continue;
   const child=add(target,JSON.stringify([edge.id,item.entity.id,next]),item.depth+1,item.id,claimed.has(next));claimed.add(next);
   const reverse=edge.directed&&edge.target===item.entity.id;
   links.push({id:child.id,source:reverse?child.id:item.id,target:reverse?item.id:child.id,category:edge.category,directed:edge.directed,reverse});
  }
 }
 const widths=new Map<number,number>();for(const n of items)widths.set(n.depth,Math.max(widths.get(n.depth)??0,n.width));
 const columns=[0];for(let depth=0;depth<widths.size;depth++)columns.push(columns[depth]+widths.get(depth)!+42);
 const spans=new Map<string,[number,number]>();let row=0;
 const stack:Array<{item:DependencyTreeItem;finish:boolean}>=[{item:items[0],finish:false}];
 while(stack.length){const entry=stack.pop()!,branches=children.get(entry.item.id)??[];
  if(!entry.finish&&branches.length){stack.push({...entry,finish:true});for(let i=branches.length-1;i>=0;i--)stack.push({item:branches[i],finish:false});continue;}
  const span:[number,number]=branches.length?[spans.get(branches[0].id)![0],spans.get(branches.at(-1)!.id)![1]]:[row,row++];
  spans.set(entry.item.id,span);entry.item.position={x:columns[entry.item.depth],y:(span[0]+span[1])*18};
 }
 return {rootId,items,links};
}
