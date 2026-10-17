import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const THREE=await server.ssrLoadModule("three");
 const {surfaceAppearanceMaterials}=await server.ssrLoadModule("/src/cad/rendering/surface-appearance.ts");
 const {CadMaterialFactory}=await server.ssrLoadModule("/src/cad/rendering/cad-material-factory.ts");
 const {CadShaderLibrary}=await server.ssrLoadModule("/src/cad/rendering/shader/cad-shader-library.ts");
 const factory=new CadMaterialFactory(new CadShaderLibrary());
 const geometry=new THREE.BufferGeometry();geometry.setIndex([0,1,2,0,2,3,4,5,6]);
 const indices=Array.from(geometry.index.array);
 const materials=surfaceAppearanceMaterials(geometry,[7,7,2],{color:"#d64a3a",faces:{2:"#000000"}},0x98a8b8,c=>factory.surface(c));
 assert.equal(materials.length,2);assert.deepEqual(materials.map(m=>m.color.getHex()),[0xd64a3a,0]);
 assert.deepEqual(geometry.groups,[{start:0,count:6,materialIndex:0},{start:6,count:3,materialIndex:1}]);
 assert.deepEqual(Array.from(geometry.index.array),indices,"color must retain triangle order for picking");
 const mesh=new THREE.Mesh(geometry,materials);factory.setInteractionState(mesh,"selected");factory.setInteractionState(mesh,"default");
 assert.deepEqual(materials.map(m=>m.color.getHex()),[0xd64a3a,0],"selection restores each original material including black");
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const engine=Object.create(CadViewportEngine.prototype);
 const view={document:{id:"part",type:"PART"},part:{bodies:[],datumPlanes:[],axisSystems:[]}};
 const initial=engine.geometrySignature(view);
 view.bodyAppearances={body:{geometryKey:"g",color:"#d64a3a"}};
 assert.notEqual(engine.geometrySignature(view),initial,"color-only Revision must refresh render materials");
 console.log("Surface appearance palette, black, selection restore, triangle mapping and render invalidation passed");
} finally {await server.close()}
