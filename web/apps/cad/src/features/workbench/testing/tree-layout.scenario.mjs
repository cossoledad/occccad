import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {visibleTreeLayout:layout}=await server.ssrLoadModule('/src/features/workbench/tree-layout.ts');
 const nodes=[{key:'root',children:[{key:'a',children:[{key:'a1'},{key:'a2',children:[{key:'deep'}]}]},{key:'b',children:[{key:'b1'}]}]}];
 const rows=layout(nodes,new Set(['a','a2','b']));
 assert.deepEqual(rows.map(r=>r.node.key),['root','a','a1','a2','deep','b','b1']);
 assert.deepEqual(rows.map(r=>r.continuationLevels),[[],[],[0],[0],[0],[],[]],'only ancestors with following siblings continue through descendants');
 assert.deepEqual(rows.map(r=>r.lastSibling),[true,false,false,true,true,true,true],'terminal elbows stop at each branch last child');
 assert.deepEqual(layout(nodes,new Set()).map(r=>r.node.key),['root','a','b'],'root stays open and collapsed children do not produce guide fragments');
 assert.deepEqual(layout(nodes,new Set(['b'])).map(r=>[r.node.key,r.continuationLevels]),[['root',[]],['a',[]],['b',[]],['b1',[]]],'collapsing an earlier sibling cannot leave a dangling guide');
}finally{await server.close()}
