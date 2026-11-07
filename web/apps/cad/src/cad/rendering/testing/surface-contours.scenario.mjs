import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const T=await server.ssrLoadModule('three');
 const {surfaceContourData}=await server.ssrLoadModule('/src/cad/rendering/face-outline.ts');
 const {CadMaterialFactory}=await server.ssrLoadModule('/src/cad/rendering/cad-material-factory.ts');
 const {CadShaderLibrary}=await server.ssrLoadModule('/src/cad/rendering/shader/cad-shader-library.ts');
 const {stableCadOpaqueSort}=await server.ssrLoadModule('/src/cad/rendering/surface-depth-pass.ts');
 const square=[0,0,0,1,0,0,1,1,0,0,0,0,1,1,0,0,1,0],before=[...square],outline=surfaceContourData(square);
 assert.equal(outline.segments.length,4,'two triangles have four boundary segments, no diagonal');assert.deepEqual(square,before);
 assert(outline.second.every(v=>v===0),'trim boundaries remain visible without silhouette classification');
 const cylinder=new T.CylinderGeometry(1,1,2,16,1,true),positions=[];const p=cylinder.attributes.position,idx=cylinder.index;
 for(let i=0;i<idx.count;i++)positions.push(p.getX(idx.getX(i)),p.getY(idx.getX(i)),p.getZ(idx.getX(i)));
 const curved=surfaceContourData(positions);assert(curved.second.some(v=>v!==0),'curved surfaces carry silhouette adjacency candidates');
 const factory=new CadMaterialFactory(new CadShaderLibrary()),material=factory.surface(0x345678),mesh=new T.Mesh(new T.BoxGeometry(),material);
 for(const state of ['hover','selected','context','default']){factory.setInteractionState(mesh,state);assert.equal(material.color.getHex(),0x345678);assert.equal(material.emissiveIntensity,0);assert.equal(material.emissive.getHex(),0,'surface lighting remains unchanged during interaction')}
 const draw=(key,id,z)=>({groupOrder:0,renderOrder:0,id,z,object:{userData:{cadSurfaceKey:key}},material:{id:100-id}});
 const a=draw('occurrence:A/body:X',1,100),b=draw('occurrence:B/body:X',2,1);
 assert(stableCadOpaqueSort(a,b)<0);assert(stableCadOpaqueSort({...a,z:1},{...b,z:100})<0,'camera depth and material allocation do not change tie ownership');assert(stableCadOpaqueSort({...a,renderOrder:3},b)>0,'explicit drawing layers retain precedence');
 factory.surfaceDepth.dispose();cylinder.dispose();mesh.geometry.dispose();material.dispose();
}finally{await server.close()}
