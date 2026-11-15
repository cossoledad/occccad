import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const {readLibraryLocation:read,writeLibraryLocation:write}=await server.ssrLoadModule('/src/features/documents/library-location.ts');
 const empty=read('');assert.equal(write('',empty),'');
 const state={scope:'parts',currentFolderID:'folder/a & b',search:'齿轮 + 壳体',type:'PART',sort:'name',offset:50,pageSize:50,viewMode:'list'};
 assert.deepEqual(read(write('',state)),state,'restorable URL uses IDs and encoded values');
 const malformed=read('?scope=unknown&type=ANY&sort=random&offset=-3&size=0&view=other');
 assert.deepEqual(malformed,empty,'unsupported options and resource limits retain valid defaults');
 assert.equal(read('?offset=Infinity').offset,0);assert.equal(read('?offset=9007199254740992').offset,0);
 const next=write('?debug=on&folder=old&q=old&offset=50',{currentFolderID:'new',search:'',offset:0});
 assert.equal(new URLSearchParams(next).get('debug'),'on');assert.equal(read(next).search,'');assert.equal(read(next).currentFolderID,'new');
 console.log('Library URL state, encoding, defaults and bounded pagination passed');
} finally {await server.close()}
