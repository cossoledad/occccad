import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {filterTreeNodes}=await server.ssrLoadModule('/src/features/workbench/specification-tree.tsx');
 const target={key:'occurrence/stable-body',title:'圆角轴承',selection:{kind:'body',id:'stable-body',occurrencePath:'one/two'}};
 const nodes=[{key:'root',title:'装配',children:[{key:'one',title:'机架',children:[target,{key:'other',title:'螺钉'}]}]}];
 const before=JSON.stringify(nodes),filtered=filterTreeNodes(nodes,'  轴承 ');
 assert.equal(filtered[0].key,'root');assert.equal(filtered[0].children[0].key,'one');
 assert.equal(filtered[0].children[0].children[0],target,'original identity/selection object is retained');
 assert.equal(filtered[0].children[0].children.length,1);assert.equal(JSON.stringify(nodes),before);
 assert.deepEqual(filterTreeNodes(nodes,'missing'),[]);assert.equal(filterTreeNodes(nodes,'装配')[0],nodes[0],'matching parent retains its subtree');
 assert.equal(filterTreeNodes(nodes,''),nodes);
}finally{await server.close()}
