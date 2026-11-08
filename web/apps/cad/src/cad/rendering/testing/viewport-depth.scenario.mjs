import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const T=await server.ssrLoadModule("three");
 const {CadMaterialFactory}=await server.ssrLoadModule("/src/cad/rendering/cad-material-factory.ts");
 const {CadShaderLibrary}=await server.ssrLoadModule("/src/cad/rendering/shader/cad-shader-library.ts");
 const factory=new CadMaterialFactory(new CadShaderLibrary()),pass=factory.surfaceDepth;
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const boundsEngine=Object.create(CadViewportEngine.prototype);boundsEngine.content=new T.Group();boundsEngine.helpers=new T.Group();boundsEngine.screenStableReferences=new Map();
 const solid=new T.Mesh(new T.BoxGeometry(20,20,20)),reference=new T.Group(),axis=new T.Line(new T.BufferGeometry().setFromPoints([new T.Vector3(),new T.Vector3(1,0,0)]));
 reference.add(axis);boundsEngine.content.add(solid,reference);boundsEngine.screenStableReferences.set(reference,64);
 const fitted=boundsEngine.visibleContentBounds();reference.scale.setScalar(10000);assert.deepEqual(boundsEngine.visibleContentBounds(),fitted,"Fit must not depend on a helper's last screen scale");
 const point=new T.Points(new T.BufferGeometry().setFromPoints([new T.Vector3()]));point.userData.kind='reference-point';point.position.set(400,30,20);boundsEngine.helpers.add(point);
 assert.equal(boundsEngine.visibleContentBounds().max.x,400,'a finite construction point outside the B-Rep participates in Fit');point.visible=false;assert.deepEqual(boundsEngine.visibleContentBounds(),fitted,'hidden reference points do not expand Fit');
 const scene=new T.Scene(),camera=new T.OrthographicCamera(-40/3,40/3,10,-10,.1,100);
 const geometry=new T.BoxGeometry(2,2,2),glass=factory.surface(0x999999,{opacity:.4}),invisible=factory.surface(0,{opacity:0});
 const mesh=new T.Mesh(geometry,[glass,invisible]);mesh.position.set(3,4,5);scene.add(mesh);
 let target=null,renders=0;const viewport=new T.Vector4(0,0,800,600),scissor=viewport.clone();
 const renderer={getPixelRatio:()=>2,getDrawingBufferSize:v=>v.set(1600,1200),getRenderTarget:()=>target,getViewport:v=>v.copy(viewport),getScissor:v=>v.copy(scissor),getScissorTest:()=>true,
  setRenderTarget:v=>target=v,setViewport(){},setScissor(){},setScissorTest(){},clear(){},render(depthScene){
   renders++;assert.notEqual(depthScene,scene);const proxy=depthScene.children[0];assert.equal(proxy.geometry,geometry);assert.deepEqual(proxy.matrix.elements,mesh.matrixWorld.elements);
   assert.equal(proxy.material[0].visible,true);assert.equal(proxy.material[1].visible,false,"zero-opacity faces never occlude edges");
  }};
 pass.render(renderer,scene,camera);assert.equal(renders,1);assert.equal(target,null);assert.equal(glass.depthWrite,false,"glass must retain its normal colour-pass depth policy");
 assert.equal(pass.uniforms.uCadDepthEnabled.value,true);assert.deepEqual(pass.uniforms.uCadDepthSize.value.toArray(),[1600,1200]);
 const edge=factory.edge();assert.equal(edge.uniforms.uCadSurfaceDepth,pass.uniforms.uCadSurfaceDepth);
 glass.opacity=1;pass.render(renderer,scene,camera);assert.equal(renders,1,"opaque-only frames need no extra depth pass");assert.equal(pass.uniforms.uCadDepthEnabled.value,false);
 // Two opaque bodies share one stable depth reference; visible glass must never
 // become that reference, nor acquire opaque depth-write behaviour.
 const opaqueScene=new T.Scene(),opaqueA=factory.surface(0x112233),opaqueB=factory.surface(0x445566),clear=factory.surface(0xffffff,{opacity:.3});
 opaqueScene.add(new T.Mesh(geometry,opaqueA),new T.Mesh(geometry,opaqueB));
 const draws=[],savedTarget={name:"caller-target"};target=savedTarget;
 const stableRenderer={...renderer,render(depthScene){draws.push({target,visible:depthScene.children.map(p=>(Array.isArray(p.material)?p.material:[p.material]).some(m=>m.visible))})}};
 pass.render(stableRenderer,opaqueScene,camera);assert.equal(draws.length,1);assert.equal(pass.opaqueUniforms.uCadOpaqueResolve.value,true);assert.equal(pass.opaqueUniforms.uCadOpaqueDepth.value,draws[0].target.depthTexture);assert.equal(target,savedTarget);
 opaqueScene.add(new T.Mesh(geometry,clear));draws.length=0;
 pass.render(stableRenderer,opaqueScene,camera);assert.equal(draws.length,2);assert.deepEqual(draws[0].visible,[true,true,true]);assert.deepEqual(draws[1].visible,[true,true,false]);assert.equal(pass.opaqueUniforms.uCadOpaqueDepth.value,draws[1].target.depthTexture);assert.equal(target,savedTarget);assert.equal(clear.depthWrite,false);
 assert.throws(()=>pass.render({...stableRenderer,render(){throw Error("failed draw")}},opaqueScene,camera),/failed draw/);assert.equal(target,savedTarget,"restore caller render target after failure");
 [opaqueA,opaqueB,clear].forEach(m=>m.dispose());
 const {makeSketchConstraintRenderable}=await server.ssrLoadModule("/src/cad/rendering/sketch-constraint-renderer.ts");
 const {updateScreenLines}=await server.ssrLoadModule("/src/cad/rendering/screen-space-lines.ts");
 const {buildSketchConstraintLayout}=await server.ssrLoadModule("/src/cad/sketch/sketch-constraint-layout.ts");
 const entities=[{id:"line",kind:"LINE",start:{x:.4,y:.8},end:{x:1.6,y:.8}}];
 const constraint={id:"parallel",kind:"PARALLEL",references:[{target:"ENTITY",entityId:"line",subElement:"WHOLE"}]};
 const frame=p=>new T.Vector3(2-p[1]/Math.sqrt(2),p[1]/Math.sqrt(2),p[0]);
 const renderable=makeSketchConstraintRenderable(constraint,entities,frame,factory,{width:800,height:600});
 const anchor=frame(buildSketchConstraintLayout(constraint,entities).anchors[0]);
 renderable.position.set(10,20,30);renderable.rotation.z=.4;
 camera.position.set(40,40,80);camera.lookAt(10,20,30);
 for(const zoom of [.1,1,100]){
  camera.zoom=zoom;camera.updateProjectionMatrix();updateScreenLines(renderable,camera,800,600);
  const glyph=renderable.children[0],point=new T.Vector3().fromBufferAttribute(glyph.geometry.getAttribute("position"),0).applyMatrix4(glyph.matrixWorld).project(camera);
  const origin=anchor.clone().applyMatrix4(renderable.matrixWorld).project(camera);
  assert.ok(Math.abs((point.x-origin.x)*400-12)<1e-3);assert.ok(Math.abs((point.y-origin.y)*300-12)<1e-3,"symbol clearance stays 12 CSS pixels on an oblique placed sketch");
 }
 pass.dispose();geometry.dispose();glass.dispose();invisible.dispose();
}finally{await server.close()}
