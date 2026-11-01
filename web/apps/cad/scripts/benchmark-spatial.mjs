import {createServer} from 'vite';
import {performance} from 'node:perf_hooks';
// CPU presentation only: synthetic meshes, no GPU/network/BREP classification.
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const THREE=await server.ssrLoadModule('three'),{CadViewportEngine}=await server.ssrLoadModule('/src/viewport/cad-viewport-engine.ts');
 const content=new THREE.Group(),bindings=new Map(),bodies=[];
 for(let i=0;i<96;i++){
  const path={canonical:String(i),segments:[{instanceId:String(i)}]},pose={translation:[i*15,0,0],rotation:[0,0,0,1]},id=`${i}/body:body`;
  const group=new THREE.Group();group.position.fromArray(pose.translation);const mesh=new THREE.Mesh(i<2?new THREE.SphereGeometry(5,64,32):new THREE.BoxGeometry(10,10,10),new THREE.MeshStandardMaterial());group.add(mesh);content.add(group);bindings.set(id,{group,mesh,artifact:{geometryKey:'source'}});bodies.push({id,instancePath:path,bodyId:'body',geometryKey:'source',pose});
 }
 const e=Object.create(CadViewportEngine.prototype);Object.assign(e,{view:{document:{id:'product',versionId:'rev'}},content,analysisScene:new THREE.Scene(),solidBindings:bindings,sectionEdgeVisibility:new Map(),sectionMaterials:new Map(),moveManipulator:{isDragging:()=>false},invalidate(){}});
 const report={documentId:'product',revisionId:'rev',bodies,pairs:[{firstId:bodies[0].id,secondId:bodies[1].id,complete:true,classification:'PENETRATION',overlapTriangles:[[0,0,0],[1,0,0],[0,1,0]]}]};
 function sample(label,n,run){const ms=[];for(let i=0;i<n;i++){const start=performance.now();run(i);ms.push(performance.now()-start)}console.log(JSON.stringify({label,samples_ms:ms}))}
 sample('role-wire-refresh-2x3968-triangles',16,()=>e.showStaticInterference(report,0));e.showStaticInterference();
 const d={scope:'ALL',frame:{translation:[0,0,0],rotation:[0,0,0,1]},sectionType:'PLANE',cutMode:'KEEP_INSIDE',size:[100,100,10]};
 sample('section-material-update-96-bodies-x100',16,()=>{for(let i=0;i<100;i++)e.previewSectionClipping({...d,frame:{...d.frame,translation:[0,0,i/10]}})});e.restoreSectionClipping();
 for(const b of bindings.values()){b.mesh.geometry.dispose();b.mesh.material.dispose()}
}finally{await server.close()}
