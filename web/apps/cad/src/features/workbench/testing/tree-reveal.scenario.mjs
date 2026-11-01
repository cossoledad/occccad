import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {treeRevealPath,revealExpansion,createdObjectReveal}=await server.ssrLoadModule('/src/features/workbench/tree-reveal.ts');
 const nodes=[{key:'root',children:[{key:'a',documentId:'part',entityId:'same',instancePath:{canonical:'a'},children:[{key:'a:p',kind:'PARAMETER',documentId:'part',entityId:'p',instancePath:{canonical:'a'}}]},{key:'b',documentId:'part',entityId:'same',instancePath:{canonical:'b'}}]}];
 assert.deepEqual(treeRevealPath(nodes,{documentId:'part',entityId:'same',occurrence:'b'}),['root','b']);
 assert.deepEqual(treeRevealPath(nodes,{documentId:'part',entityId:'p',kind:'PARAMETER',occurrence:'a'}),['root','a','a:p']);
 assert.equal(treeRevealPath(nodes,{documentId:'another',entityId:'same'}),undefined);
 assert.deepEqual(revealExpansion(['root','manual','old-auto'],['root','old-auto'],['root','new','target']),{expanded:['manual','root','new'],automatic:['root','new']});
 const before={document:{id:'doc'},structureTree:{id:'root',kind:'PART',documentId:'doc',entityId:'doc',children:[]}};
 const after=structuredClone(before);after.structureTree.children=[{id:'measurement-group',kind:'MEASUREMENT_GROUP',children:[{id:'m',kind:'MEASUREMENT',documentId:'doc',entityId:'m',capabilities:['EDIT'],children:[{id:'p',kind:'PARAMETER',documentId:'doc',entityId:'p',capabilities:['EDIT']}]}]}];
 assert.equal(createdObjectReveal(before,after).entityId,'m');assert.equal(createdObjectReveal(after,after),undefined);
}finally{await server.close()}
