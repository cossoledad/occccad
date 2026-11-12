import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {CommandSelectionSeed}=await server.ssrLoadModule('/src/cad/interaction/command-selection-seed.ts');
 const curve=id=>({kind:'edge',id,documentId:'part',versionId:'revision',bodyId:'body',geometryKey:'geometry',topologyId:Number(id)});
 const seed=new CommandSelectionSeed(),abort=new AbortController();
 const a=curve('1'),b=curve('2'),foreign={...curve('3'),versionId:'old'};
 const session={documentId:'part',versionId:'revision',role:'edge',selections:[],onPick:()=>{},seedMultiple:true};
 seed.begin([a,a,foreign,b],abort.signal);
 assert.deepEqual(seed.take(session),a);assert.deepEqual(seed.take(session),b);assert.equal(seed.take(session),undefined);
 seed.begin([a,b],abort.signal);assert.deepEqual(seed.take({...session,seedMultiple:false,seedSlot:'profile'}),a);
 assert.equal(seed.take({...session,seedMultiple:false,seedSlot:'profile'}),undefined,'a single role never consumes a second candidate');
 assert.deepEqual(seed.take({...session,seedMultiple:false,seedSlot:'path'}),b,'the next role receives the remaining selection');
 seed.begin([a,b],abort.signal);assert.deepEqual(seed.take({...session,selections:[a]}),b,'existing seed users are not toggled off');
 seed.begin([a],abort.signal);assert.equal(seed.take({...session,seedEnabled:false}),undefined,'editing restores saved inputs');
 abort.abort();assert.equal(seed.take(session),undefined,'cancelled invocations cannot seed a later form');
 console.log('PASS command selection: ordered roles, duplicate, scope, edit and cancellation');
}finally{await server.close()}
