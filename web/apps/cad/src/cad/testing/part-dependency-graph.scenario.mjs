import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {partDependencySlice,partDependencySnapshotMatches}=await server.ssrLoadModule('/src/cad/part/part-dependency-graph.ts');
 const nodes=['width','dimension','plane','sketch','profile','pad','fillet','unrelated'].map(key=>({key,entityId:key,label:key,type:key==='width'?'PARAMETER':'FEATURE',state:key==='pad'?'FAILED':key==='fillet'?'BLOCKED':'CURRENT'}));
 const pairs=[['width','dimension'],['dimension','sketch'],['plane','sketch'],['sketch','profile'],['profile','pad'],['pad','fillet'],['width','unrelated']];
 const graph={documentId:'part',versionId:'head',digest:'formal',nodes,edges:pairs.map(([source,target])=>({source,target,kind:'READ_GEOMETRY'}))};
 const upstream=partDependencySlice(graph,'pad','upstream');assert.deepEqual(new Set(upstream.nodes.map(n=>n.key)),new Set(['pad','profile','sketch','dimension','width','plane']));assert(!upstream.nodes.some(n=>n.key==='unrelated'),'shared parameters cannot drag independent consumers into the source view');
 assert.deepEqual(partDependencySlice(graph,'pad','downstream').nodes.map(n=>n.key),['pad','fillet']);
 const both=partDependencySlice(graph,'pad','both');assert.deepEqual(both.causes.map(n=>n.key),['pad']);assert(!both.nodes.some(n=>n.key==='unrelated'));
 const bounded=partDependencySlice(graph,'pad','both',3);assert.equal(bounded.nodes.length,3);assert.equal(bounded.omitted,4);assert(bounded.edges.every(e=>bounded.nodes.some(n=>n.key===e.source)&&bounded.nodes.some(n=>n.key===e.target)));
 const before=JSON.stringify(graph);partDependencySlice(graph,'width','both');assert.equal(JSON.stringify(graph),before,'graph layout never rewrites the formal snapshot');
 const base={document:{id:'part',versionId:'head'}};assert(partDependencySnapshotMatches(base,graph));assert(!partDependencySnapshotMatches({...base,document:{...base.document,versionId:'new'}},graph));assert(!partDependencySnapshotMatches(base,{...graph,instancePath:{canonical:'other'}}));
 const path={rootDocumentId:'root',canonical:'root/a',segments:[{instanceId:'a'}]};const root={document:{id:'root',versionId:'root-head'},product:{instances:[{id:'a',referenceMode:'FOLLOW'}]},contextVariants:[{variantKey:'accepted',status:'READY',baseDocumentId:'part',baseRevisionId:'head',owningInstancePath:path,part:{features:[]}}]};
 const view={...base,contextProduct:root,contextOccurrencePath:path.canonical};const scoped={...graph,rootProductDocumentId:'root',rootProductRevisionId:'root-head',contextVariantId:'accepted',instancePath:path};assert(partDependencySnapshotMatches(view,scoped));assert(!partDependencySnapshotMatches(view,{...scoped,instancePath:{...path,canonical:'root/b'}}));assert(!partDependencySnapshotMatches(view,{...scoped,rootProductRevisionId:'old'}));
 console.log('PASS: formal directed Part dependencies, bounded display, root cause and frozen occurrence');
}finally{await server.close();}
