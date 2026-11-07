import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {edgeFacingAttributes}=await server.ssrLoadModule('/src/cad/rendering/edge-facing.ts');
 const mesh={vertices:[[0,0,0],[1,0,0],[0,0,0],[1,0,0]],normals:[[0,0,1],[0,0,1],[0,1,0],[0,1,0]],triangles:[],faceIds:[],edges:[{localId:77,points:[[0,0,0],[.5,0,0],[1,0,0]]}]};
 const before=JSON.stringify(mesh),result=edgeFacingAttributes(mesh);
 assert.deepEqual(result.first,[0,0,1,0,0,1]);assert.deepEqual(result.second,[0,1,0,0,1,0]);
 assert.equal(JSON.stringify(mesh),before,'display adjacency never rewrites triangle or topology identities');
 const missing=edgeFacingAttributes({...mesh,normals:undefined});assert.deepEqual(missing.first,[0,0,0,0,0,0]);
 const {normalizeSceneSettings,DEFAULT_SCENE_SETTINGS,engineeringScenePreset}=await server.ssrLoadModule('/src/cad/rendering/scene-settings.ts');
 assert.equal(DEFAULT_SCENE_SETTINGS.grid,false);assert.equal(DEFAULT_SCENE_SETTINGS.ambientOcclusion,false);
 assert.equal(normalizeSceneSettings({grid:true,ambientOcclusion:true,aoIntensity:9}).aoIntensity,.8);
 assert.equal(engineeringScenePreset('NEUTRAL').background,'THEME');assert.equal(engineeringScenePreset('DARK').background,'GRADIENT');
}finally{await server.close()}
