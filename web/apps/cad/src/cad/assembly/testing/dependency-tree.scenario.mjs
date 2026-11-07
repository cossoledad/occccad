import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {assemblyDependencies}=await server.ssrLoadModule('/src/cad/assembly/assembly-dependencies.ts');
 const {dependencyTree}=await server.ssrLoadModule('/src/cad/assembly/dependency-tree.ts');
 const view={document:{id:'doc',versionId:'r1',name:'Assembly'},product:{instances:['a','b','c','free'].map(id=>({id,name:id})),constraints:[{id:'ab',kind:'COINCIDENT',first:{instanceId:'a'},second:{instanceId:'b'}},{id:'bc',kind:'DISTANCE',first:{instanceId:'b'},second:{instanceId:'c'},suppressed:true},{id:'ca',kind:'ANGLE',first:{instanceId:'c'},second:{instanceId:'a'}}],parameters:[{parameterId:'input',key:'input',source:{literal:{siValue:1}}},{parameterId:'out',key:'out',source:{expression:{sourceText:'input * 2',reads:['parameter:input']}}}]}};
 const graph=assemblyDependencies(view),all=new Set(graph.nodes.map(n=>n.id)),filter={categories:['CONSTRAINT','RELATION'],includeSuppressed:true};
 const tree=dependencyTree(graph,graph.rootId,all,filter),byId=new Map(tree.items.map(n=>[n.id,n]));
 const root=tree.items[0],direct=tree.items.filter(n=>n.parentId===root.id);
 assert(direct.some(n=>n.entity.constraintId==='ab'),'assembly opens through relationships');
 assert(!direct.some(n=>n.entity.instanceId==='a'),'a constrained support belongs below its constraint');
 assert(direct.some(n=>n.entity.instanceId==='free'),'unconstrained components remain accessible');
 const ab=direct.find(n=>n.entity.constraintId==='ab');assert.deepEqual(new Set(tree.items.filter(n=>n.parentId===ab.id).map(n=>n.entity.instanceId)),new Set(['a','b']));
 assert.equal(root.position.y,(Math.min(...tree.items.map(n=>n.position.y))+Math.max(...tree.items.map(n=>n.position.y)))/2,'root is vertically centered on visible branches');
 for(const n of tree.items){
  if(n.parentId)assert(n.position.x>byId.get(n.parentId).position.x+byId.get(n.parentId).width,'columns clear actual label widths');
  const peers=tree.items.filter(m=>m.depth===n.depth&&m.id!==n.id);assert(peers.every(m=>Math.abs(m.position.y-n.position.y)>=36),'rows do not overlap');
  if(n.reference)assert(!tree.items.some(m=>m.parentId===n.id),'repeat references terminate cycles');
 }
 assert(tree.items.some(n=>n.reference),'a repeated support is a display occurrence, not a duplicated domain object');
 assert.equal(new Set(tree.items.map(n=>n.id)).size,tree.items.length);
 for(const link of tree.links.filter(l=>l.directed)){
  const source=byId.get(link.source).entity,target=byId.get(link.target).entity;
  assert(graph.edges.some(e=>e.source===source.id&&e.target===target.id&&e.directed),'layout preserves semantic arrow direction, including right-to-left input edges');
 }
 assert(tree.links.some(l=>l.reverse),'formula inputs route back toward the formula without reversing meaning');
 const component=graph.nodes.find(n=>n.instanceId==='a');
 const one=dependencyTree(graph,component.id,new Set([component.id]),filter);assert(one.items.slice(1).every(n=>n.entity.kind==='constraint'),'component expands to its own constraints');
 const hidden=dependencyTree(graph,graph.rootId,all,{...filter,includeSuppressed:false});assert(!hidden.items.some(n=>n.entity.constraintId==='bc'));
 assert.deepEqual(dependencyTree(graph,graph.rootId,all,filter),tree,'layout is deterministic');
 // A long cyclic network stays bounded and uses iterative traversal/layout.
 const count=1000,large=assemblyDependencies({document:view.document,product:{instances:Array.from({length:count},(_,i)=>({id:`i${i}`,name:'Unit'})),constraints:Array.from({length:count},(_,i)=>({id:`c${i}`,kind:'COINCIDENT',first:{instanceId:`i${i}`},second:{instanceId:`i${(i+1)%count}`}}))}});
 const ring=dependencyTree(large,large.nodes.find(n=>n.instanceId==='i0').id,new Set(large.nodes.map(n=>n.id)),filter);assert(ring.items.length<=2*large.edges.length+1);assert(ring.items.every(n=>Number.isFinite(n.position.y)));
}finally{await server.close();}
