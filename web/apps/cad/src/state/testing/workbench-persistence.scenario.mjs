import assert from 'node:assert/strict';
import {createServer} from 'vite';
const writes=[];
globalThis.localStorage={getItem:()=>JSON.stringify({state:{inspectorTab:'history'},version:0}),setItem:(key,value)=>writes.push([key,JSON.parse(value)]),removeItem(){}};
globalThis.window={localStorage:globalThis.localStorage};
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {useWorkbenchStore:store}=await server.ssrLoadModule('/src/state/workbench-store.ts');
 assert.equal(store.getState().inspectorTab,'history','existing saved preference still hydrates');
 for(let i=0;i<200;i++)store.getState().setPreselection({kind:'face',id:`face-${i}`,localId:i});
 for(let i=0;i<12;i++)store.getState().setSelections([{kind:'body',id:`body-${i}`}]);
 store.getState().setActiveTool('sketch.rectangle');store.getState().completeToolUse();
 assert.equal(writes.length,0,'interaction never writes or serializes persistent preferences');
 store.getState().setInspectorTab('properties');store.getState().setInspectorTab('properties');
 assert.deepEqual(writes,[['occccad.workbench',{state:{inspectorTab:'properties'},version:0}]]);
 const before=store.getState();store.getState().setPreselection(before.preselection);assert.equal(store.getState(),before);
}finally{delete globalThis.window;delete globalThis.localStorage;await server.close()}
