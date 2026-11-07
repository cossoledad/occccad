import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {assemblyDependencies,dependencySlice}=await server.ssrLoadModule('/src/cad/assembly/assembly-dependencies.ts');
 const parameter=(id,source)=>({parameterId:id,label:id,key:id,source});
 const path=id=>({rootDocumentId:'root',canonical:id,segments:[{ownerDocumentId:'root',instanceId:id}]});
 const view={document:{id:'root',versionId:'r1',name:'Assembly'},product:{instances:['a','b','c'].map(id=>({id,name:'Same Part',documentId:'shared-part'})),
  constraints:[{id:'ab',name:'Coincident.1',kind:'COINCIDENT',first:{instanceId:'a'},second:{instanceId:'b'},evaluationStatus:'VERIFIED',visible:false},
   {id:'bc',kind:'DISTANCE',first:{instanceId:'b'},second:{instanceId:'c'},suppressed:true},
   {id:'ca',kind:'ANGLE',first:{instanceId:'c'},second:{instanceId:'a'},angleAxis:{instanceId:'b'}},
   {id:'group',kind:'FIX_TOGETHER',first:{instanceId:'a'},groupMembers:[{instanceId:'b'},{instanceId:'c'}]},
   {id:'lost',kind:'CONTACT',first:{instanceId:'deleted'},second:{instanceId:'gone'},evaluationStatus:'BROKEN'}],
  contextBindings:[{id:'binding',name:'Skeleton input',owningInstancePath:path('b'),sourceInstancePath:path('a'),resolution:{status:'CONNECTED'}}],
  parameters:[parameter('input',{literal:{siValue:1}}),parameter('output',{expression:{sourceText:'renamed_display * 2',reads:['parameter:input']}}),parameter('bad',{expression:{sourceText:'input + deleted',reads:['parameter:deleted']}}),parameter('unchecked',{expression:{sourceText:'input * 3'}})]}};
 const frozen=JSON.stringify(view),graph=assemblyDependencies(view);
 assert.equal(new Set(graph.nodes.map(n=>n.id)).size,graph.nodes.length);
 const a=graph.nodes.find(n=>n.instanceId==='a'),b=graph.nodes.find(n=>n.instanceId==='b');assert.notEqual(a.id,b.id,'two occurrences of the same Part remain distinct');
 const filter={categories:['CONSTRAINT'],includeSuppressed:true},initial=dependencySlice(graph,a.id,new Set([a.id]),filter);
 assert(initial.nodes.some(n=>n.constraintId==='ab'));assert(!initial.nodes.some(n=>n.instanceId==='b'),'one level reveals constraints before their other support');
 const all=dependencySlice(graph,graph.rootId,new Set(graph.nodes.map(n=>n.id)),filter);
 assert.equal(all.nodes.filter(n=>n.kind==='constraint').length,5);assert(all.nodes.some(n=>n.instanceId==='deleted'&&n.missing),'orphan broken constraints stay inspectable');
 assert(all.edges.filter(e=>e.category==='CONSTRAINT').every(e=>!e.directed),'symmetric constraints are not causal arrows');
 assert.equal(all.nodes.filter(n=>n.constraintId==='group').length,1,'hyperedges and cycles never duplicate business objects');
 const active=dependencySlice(graph,graph.rootId,new Set(graph.nodes.map(n=>n.id)),{...filter,includeSuppressed:false});assert(!active.nodes.some(n=>n.constraintId==='bc'));assert(active.nodes.find(n=>n.constraintId==='ab').hidden,'hiding a 3D glyph never deactivates its relation');
 const association=dependencySlice(graph,a.id,new Set(graph.nodes.map(n=>n.id)),{...filter,categories:['ASSOCIATIVITY']});assert(association.nodes.some(n=>n.bindingId==='binding'));assert(association.edges.every(e=>e.directed));
 const relations=dependencySlice(graph,graph.rootId,new Set(graph.nodes.map(n=>n.id)),{...filter,categories:['RELATION']});
 const output=relations.nodes.find(n=>n.kind==='formula'&&n.expression==='renamed_display * 2');assert(output);const input=relations.nodes.find(n=>n.parameterId==='input');assert(relations.edges.some(e=>e.source===input.id&&e.target===output.id),'checked stable reads survive alias differences');
 const unchecked=relations.nodes.find(n=>n.expression==='input * 3');assert.equal(unchecked.state,'UNAVAILABLE');assert(!relations.edges.some(e=>e.source===input.id&&e.target===unchecked.id),'never infer links by parsing display names');
 assert(relations.nodes.some(n=>n.parameterId==='deleted'&&n.missing));
 const renamed=assemblyDependencies({...view,document:{...view.document,versionId:'r2'},product:{...view.product,instances:[...view.product.instances].reverse().map(i=>({...i,name:'Renamed'}))}});assert.deepEqual(new Set(renamed.nodes.map(n=>n.id)),new Set(graph.nodes.map(n=>n.id)),'labels, order and Revision do not replace entity identity');
 assert.notEqual(assemblyDependencies(view,'first').rootId,assemblyDependencies(view,'second').rootId,'owning occurrence scopes the graph');assert.equal(JSON.stringify(view),frozen);
 const large={document:view.document,product:{instances:Array.from({length:1000},(_,i)=>({id:`unit-${i}`,name:'Unit'})),constraints:Array.from({length:1000},(_,i)=>({id:`link-${i}`,kind:'COINCIDENT',first:{instanceId:`unit-${i}`},second:{instanceId:`unit-${(i+1)%1000}`}}))}};
 const cycle=assemblyDependencies(large),visible=dependencySlice(cycle,cycle.rootId,new Set(cycle.nodes.map(n=>n.id)),filter);assert.equal(visible.nodes.length,2001);assert.equal(visible.edges.length,3000,'visited traversal terminates on a closed network');
}finally{await server.close()}
