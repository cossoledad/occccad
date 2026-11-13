// Acceptance-only page: actual OCCT-generated GLBs travel through the production
// VisualRepository and viewport. This fixture never writes a business document.
import * as THREE from 'three';
import {CadViewportEngine} from '../src/viewport/cad-viewport-engine';
import {mockApi} from '../src/api/mock-api';
import {DEFAULT_SCENE_SETTINGS} from '../src/cad/rendering/scene-settings';
import {DEFAULT_SOLID_DISPLAY} from '../src/cad/rendering/display-settings';
import {edgePreparationStats} from '../src/cad/rendering/natural-edges';
import {decodeMeshGLB} from '../src/cad/visual/mesh-glb';

const noop=()=>{},reject=async()=>{throw Error('No domain writes in the edge acceptance harness')};
let complete:(()=>void)|undefined;
const e:any=new CadViewportEngine(document.getElementById('viewport')!,{
 selectionsChanged:noop,preselectionChanged:noop,sketchOperations:reject,toolPromptChanged:noop,toolUseCompleted:noop,
 dimensionEditRequested:noop,dimensionCreateRequested:noop,activeToolChanged:noop,instanceMoved:reject,
 assemblyInteractionBegin:reject,assemblyInteractionUpdate:reject,assemblyInteractionCancel:reject,
 assemblyConstraintRequested:noop,documentViewReady:()=>complete?.(),operationFailed:error=>{throw error},
});
let sequence=0,submitMs=0;const render=e.renderer.render.bind(e.renderer);e.renderer.render=(...args:any[])=>{const start=performance.now();try{return render(...args)}finally{submitMs+=performance.now()-start}};
const reference={points:false,planes:false,axes:false,coordinateSystems:false};
e.setDisplaySettings(reference,{...DEFAULT_SOLID_DISPLAY});
e.setSceneSettings({...DEFAULT_SCENE_SETTINGS,groundMode:"OFF",shadows:false,grid:false,ambientOcclusion:false});
const part=await mockApi.getDocument('mock-part-bracket');

async function load(name:string,digest:string,size:number,color='#aebac7',count=1,stage:'uniform'|'basic'|'natural'='natural',options:{materialDefaults?:boolean;domain?:string}={}){
 const bytes=await (await fetch(`/edge-fixtures/${name}.glb`)).arrayBuffer(),decoded=decodeMeshGLB(bytes),mesh=decoded.mesh;
 const box=new THREE.Box3().setFromBufferAttribute(new THREE.BufferAttribute(mesh.buffers!.positions,3));
 const key=`edge-fixture:${name}`,id=`edge-fixture:${++sequence}`;
 const domain=options.domain??(['open','varying','coplanar'].includes(name)?'SHEET_SET':'SOLID_SET');
 const coefficients=options.materialDefaults?{}:{roughness:.72,metalness:0};
 const artifact:any={geometryKey:key,geometryId:key,representationKind:'PERSISTENT',geometryDomain:domain,
  representations:{VISUAL:{objectId:key,digest,schemaVersion:2,size,contentType:'model/gltf-binary',url:`/edge-fixtures/${name}.glb`}},
  triangleCount:mesh.buffers!.indices.length/3,displayVertexCount:mesh.buffers!.positions.length/3,bbox:{min:box.min.toArray(),max:box.max.toArray()},
  topology:{faces:new Set(mesh.buffers!.faceIds).size,edges:new Set(mesh.edges?.map(x=>x.localId)).size,vertices:mesh.topologyVertices?.length??0,solids:domain==='SOLID_SET'?1:0},
  visualization:{schemaVersion:2,primitives:[]},volume:0,occtVersion:'native-fixture',glbBytes:size,brepBytes:0,evaluatorVersion:'edge-profile-v1',workerId:'native-fixture',storageState:'OBJECT'};
 const view:any=structuredClone(part);
 view.document={...view.document,id,type:count===1?'PART':'PRODUCT',name:'edge acceptance',versionId:`${id}:v1`};view.artifacts={[key]:artifact};view.artifact=undefined;view.structureTree=undefined;
 view.datumPlanes=[];view.axisSystems=[];view.datumAxes=[];view.referenceDisplayScopes=[];view.contextVariants=[];view.measurements=[];
 if(count===1){
  view.part={...part.part,features:[],bodies:[{...part.part!.bodies[0],id:'fixture-body',geometryKey:key,visible:true,consumed:false}],activeBodyId:'fixture-body',datumPlanes:[],axisSystems:[],datumAxes:[],datumPoints:[],publications:[]};
  view.bodyAppearances={'fixture-body':{geometryKey:key,color,...coefficients}};
 }else{
  const width=Math.ceil(Math.sqrt(count)),stride=box.getSize(new THREE.Vector3()).length()*.8;
  view.part=undefined;view.product={instances:[],constraints:[],publications:[]};view.resolvedInstances=[];
  for(let i=0;i<count;i++){
   const instanceId=`i-${String(i).padStart(3,'0')}`,translation=[(i%width-(width-1)/2)*stride,(Math.floor(i/width)-(width-1)/2)*stride,0],
    rotation=new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(1,2,3).normalize(),i*.37).toArray();
   const path={canonical:instanceId,segments:[{instanceId,documentId:'fixture-part',resolvedVersionId:'fixture-v1'}]};
   view.product.instances.push({name:instanceId,id:instanceId,documentId:'fixture-part',versionId:'fixture-v1',visible:true,translation,rotation});
   view.resolvedInstances.push({id:`edge acceptance/${instanceId}`,occurrencePath:instanceId,documentId:'fixture-part',instancePath:path,bodyId:'fixture-body',bodyVisible:true,geometryKey:key,translation,rotation,bodyTreeNodeId:`document:${id}/instance:${instanceId}/body:fixture-body`,appearance:{geometryKey:key,color,...coefficients},referenceGeometry:{datumPlanes:[],datumAxes:[],axisSystems:[]}});
  }
 }
 // Choose the next scene's implementation without first preparing the previous
 // scene in that mode. The comparison must include cold buffer preparation.
 e.edgePresentation=stage;e.setDisplaySettings(reference,{...DEFAULT_SOLID_DISPLAY,edgeStyle:stage==='uniform'?'uniform':'natural'});edgePreparationStats(true);
 const start=performance.now();await new Promise<void>(resolve=>{complete=resolve;e.render(view)});complete=undefined;
 while(e.viewTransition.active)await new Promise(requestAnimationFrame);
 await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);
 const center=e.visibleContentBounds().getCenter(new THREE.Vector3()),radius=e.visibleContentBounds().getSize(new THREE.Vector3()).length();
 e.camera.position.copy(center).add(new THREE.Vector3(1,-1.5,1).normalize().multiplyScalar(radius*1.8));e.navigation.target.copy(center);e.camera.up.set(0,0,1);e.camera.lookAt(center);e.camera.zoom=1;e.camera.updateProjectionMatrix();e.camera.updateMatrixWorld(true);e.navigation.syncCamera(false);e.updateCameraClipping();e.invalidate();
 await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);
 return {edgePreparation:edgePreparationStats(),preparationMs:performance.now()-start,profileSegments:mesh.edgeProfile?.metadata.length!/4,bindings:e.solidBindings.size};
}
async function style(value:'uniform'|'basic'|'natural'){
 e.setEdgePresentation(value);e.setDisplaySettings(reference,{...DEFAULT_SOLID_DISPLAY,edgeStyle:value==='uniform'?'uniform':'natural'});e.invalidate();await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);
}
async function frame(angle=0){
 if(angle){const offset=e.camera.position.clone().sub(e.navigation.target).applyAxisAngle(new THREE.Vector3(0,0,1),angle);e.camera.position.copy(e.navigation.target).add(offset);e.camera.lookAt(e.navigation.target);e.camera.updateMatrixWorld(true)}
 e.renderer.info.autoReset=false;e.renderer.info.reset();submitMs=0;const start=performance.now();e.invalidate();await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);
 const result={submitMs,edgeGeometries:new Set([...e.solidBindings.values()].map((b:any)=>b.group.children.find((o:any)=>o.userData.cadWideEdges)?.geometry)).size,wallMs:performance.now()-start,calls:e.renderer.info.render.calls,triangles:e.renderer.info.render.triangles,geometries:e.renderer.info.memory.geometries};e.renderer.info.autoReset=true;return result;
}
Object.assign(window,{edgeHarness:{engine:e,load,style,frame,THREE}});
