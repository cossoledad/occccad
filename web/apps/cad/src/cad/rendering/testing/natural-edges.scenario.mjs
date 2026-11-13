import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const THREE=await server.ssrLoadModule('three');
 const edges=await server.ssrLoadModule('/src/cad/rendering/natural-edges.ts');
 const z=new THREE.Vector3(0,0,1),y=new THREE.Vector3(0,1,0);
 assert.equal(edges.edgeSharpness(z,z),0,'tangency uses zero normal angle');
 assert.equal(edges.edgeSharpness(z,y),1,'a right angle remains structural');
 assert.equal(edges.edgeSharpness(z,z.clone().negate()),1,'opposed normals are not made tangent by abs(dot)');
 assert.equal(edges.edgeSharpness(z,new THREE.Vector3()),1,'missing evidence is conservative');
 let previous=0;
 for(let angle=0;angle<=90;angle+=.1){
  const weight=edges.edgeSharpness(z,new THREE.Vector3(Math.sin(angle*Math.PI/180),0,Math.cos(angle*Math.PI/180)));
  assert.ok(weight>=previous-1e-12&&weight-previous<.02,'normal-angle response stays continuous and monotone');previous=weight;
 }
 for(const scale of [.001,1,1000]){
  const m=new THREE.Matrix4().compose(new THREE.Vector3(3,5,7),new THREE.Quaternion().setFromAxisAngle(y,.73),new THREE.Vector3(-scale,scale,scale));
  const n=new THREE.Matrix3().getNormalMatrix(m);
  assert.equal(edges.edgeSharpness(z.clone().applyMatrix3(n),y.clone().applyMatrix3(n)),1,'pose, reflection and uniform scale preserve right angles');
  assert.equal(edges.edgeSharpness(z.clone().applyMatrix3(n),z.clone().applyMatrix3(n)),0);
 }
 const anisotropic=new THREE.Matrix3().getNormalMatrix(new THREE.Matrix4().makeScale(2,1,.5));
 const diagonal=new THREE.Vector3(1,0,1).normalize().applyMatrix3(anisotropic).normalize();
 assert.ok(Math.abs(diagonal.x/diagonal.z-.25)<1e-12,'normals use inverse transpose under nonuniform scale');
 const mesh={vertices:[],triangles:[],faceIds:[],edges:[{localId:7,points:[[0,0,0],[1,0,0]]}],edgeProfile:{source:'OCCT_EDGE_USE_V1',positions:new Float32Array([0,0,0,1,0,0]),normalA:new Float32Array([0,0,1,0,0,1]),normalB:new Float32Array([0,1,0,0,1,0]),metadata:new Uint32Array([7,1,2,3])}};
 const before=JSON.stringify(mesh);edges.edgePreparationStats(true);
 const a=edges.acquireEdgeGeometry(mesh,true),b=edges.acquireEdgeGeometry(mesh,true);
 assert.equal(a.geometry,b.geometry,'occurrences share immutable GPU geometry');
 assert.equal(a.geometry.getAttribute('instanceEdgeAStart').data.array,mesh.edgeProfile.normalA,'normals directly view the decoded GLB');
 let disposals=0;a.geometry.addEventListener('dispose',()=>disposals++);a.release();a.release();assert.equal(disposals,0);b.release();assert.equal(disposals,1);
 const c=edges.acquireEdgeGeometry(mesh,true),d=edges.acquireEdgeGeometry(mesh,false);
 assert.notEqual(c.geometry,d.geometry,'dashed/isolated comparison geometry owns its distance attributes');
 assert.equal(edges.edgePreparationStats().builds,2);c.release();d.release();assert.equal(JSON.stringify(mesh),before);
 const fallback=edges.buildEdgeBuffers({...mesh,edgeProfile:undefined});
 assert.deepEqual(Array.from(fallback.positions),[0,0,0,1,0,0]);assert.deepEqual(Array.from(fallback.kinds),[0]);
 assert.equal(edges.buildEdgeBuffers({...mesh,edgeProfile:undefined,edges:[{localId:1,points:[[0,0,0],[0,0,0],[NaN,0,0]]}]}).positions.length,0,'degenerate/invalid legacy chords never reach the line shader');
 const {applyEdgeAppearance}=await server.ssrLoadModule('/src/cad/rendering/surface-appearance.ts');
 const {applySolidDisplaySettings}=await server.ssrLoadModule('/src/cad/rendering/solid-render-mode.ts');
 const {SurfaceDepthPass}=await server.ssrLoadModule('/src/cad/rendering/surface-depth-pass.ts');
 const depth=new SurfaceDepthPass(),group=new THREE.Group(),geometry=new THREE.BufferGeometry();
 geometry.setAttribute('position',new THREE.Float32BufferAttribute([0,0,0,1,0,0],3));geometry.userData.cadMesh=mesh;
 const source=new THREE.LineSegments(geometry,new THREE.ShaderMaterial({uniforms:{uOpacity:{value:1}}}));
 group.add(source);group.userData.edgePresentation='natural';
 applyEdgeAppearance(group,undefined,0x222222,m=>depth.configureLine(m));
 const wide=group.children.find(o=>o.userData.cadWideEdges);
 assert.equal(source.material.visible,false,'zero-alpha picking lines do not submit a draw');
 const ray=new THREE.Raycaster(new THREE.Vector3(.5,0,1),new THREE.Vector3(0,0,-1));
 assert.equal(ray.intersectObject(source).length,1,'render material visibility preserves original edge raycasting');
 for(const [mode,pick,expected] of [['shaded',undefined,1],['wireframe',undefined,0],['shaded','EDGE',0]]){
  applySolidDisplaySettings(group,{mode,edges:true,vertices:false,edgeStyle:'natural'},pick);
  assert.equal(wide.material.uniforms.uCadEdgeNatural.value,expected);
 }
 assert.equal(wide.material.alphaToCoverage,false,'soft alpha is not quantized into MSAA sample coverage');
 wide.userData.cadGeometryRelease();depth.dispose();geometry.dispose();source.material.dispose();wide.material.dispose();
 const {DEFAULT_SCENE_SETTINGS,normalizeSceneSettings,engineeringScenePreset}=await server.ssrLoadModule('/src/cad/rendering/scene-settings.ts');
 assert.equal(DEFAULT_SCENE_SETTINGS.grid,false);assert.equal(DEFAULT_SCENE_SETTINGS.ambientOcclusion,false);
 assert.equal(normalizeSceneSettings({grid:true,ambientOcclusion:true,aoIntensity:9}).aoIntensity,.8);
 assert.equal(engineeringScenePreset('NEUTRAL').background,'THEME');assert.equal(engineeringScenePreset('DARK').background,'GRADIENT');
 console.log('Natural edge continuity, oriented angles, transforms, sharing/release, conservative fallback, pick visibility and display modes passed');
} finally {await server.close()}
