import {expect,test} from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
import {observeViewport} from './camera-helpers';
import {edgeFixtures as fixtures,prepareEdgeFixtures} from './edge-fixtures';

const output=path.resolve('../../../build/agent-work/natural-edges/comparison');
test.beforeAll(async()=>{test.setTimeout(180_000);await prepareEdgeFixtures()});

test('natural edges survive real import Jobs, Router, Worker, artifacts and document/assembly views',async({page})=>{
 test.setTimeout(300_000);fs.mkdirSync(output,{recursive:true});
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});
 expect(login.ok()).toBe(true);await page.goto('/');
 const errors:string[]=[],documents:string[]=[],reports:any[]=[];
 page.on('pageerror',error=>errors.push(error.message));
 try {
  for(const name of ['cube','fillet','cylinder','sphere','coplanar','varying','open']){
   const job=await page.evaluate(async({name,base64})=>{
    const {restApi:api}=await import('/src/api.ts');
    const bytes=Uint8Array.from(atob(base64),c=>c.charCodeAt(0));
    return api.importDocument(new File([bytes],`natural-edge-${name}.brep`,{type:'application/octet-stream'}));
   },{name,base64:fs.readFileSync(path.join(fixtures,name+'.brep')).toString('base64')});
   let finished:any;
   await expect.poll(async()=>{finished=await(await page.request.get(`/api/jobs/${job.id}`)).json();return finished.state},{timeout:90_000}).toBe('SUCCEEDED');
   expect(finished.documentId).toBeTruthy();documents.push(finished.documentId);
   await page.goto(`/documents/${finished.documentId}`);await expect(page.locator('canvas').first()).toBeVisible();await observeViewport(page);
   await expect.poll(()=>page.evaluate(()=>[...(window as any).__cameraEngine.solidBindings.values()].filter((b:any)=>b.artifact.mesh.edgeProfile?.source==='OCCT_EDGE_USE_V1').length)).toBeGreaterThan(0);
   const report=await page.evaluate(()=>{
    const e=(window as any).__cameraEngine;
    return [...e.solidBindings.values()].map((b:any)=>({geometryKey:b.artifact.geometryKey,representation:b.artifact.representations.VISUAL,profileSegments:b.artifact.mesh.edgeProfile.metadata.length/4,kinds:[...new Set(Array.from(b.artifact.mesh.edgeProfile.metadata).filter((_,i)=>i%4===3).map((v:any)=>v&255))],naming:Boolean(b.artifact.naming),normalBufferBytes:b.artifact.mesh.edgeProfile.normalA.byteLength+b.artifact.mesh.edgeProfile.normalB.byteLength}));
   });
   reports.push({name,jobId:job.id,documentId:finished.documentId,bindings:report});
   await page.screenshot({path:path.join(output,`live-${name}.png`)});
  }
  const parametric=await page.evaluate(async()=>{
   const {restApi:api}=await import('/src/api.ts');const v=await api.createDocument('PART','Parametric natural edge acceptance');
   return {id:v.document.id,body:v.part!.activeBodyId};
  });documents.push(parametric.id);
  await page.evaluate(async({id,body})=>{
   const {restApi:api}=await import('/src/api.ts');let v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY',bodyId:body});
   const sketch=v.part!.features.at(-1)!.id;
   await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:0,y:0},second:{x:20,y:20}}]});
   v=await api.command(id,{type:'CREATE_SOLID_FEATURE',bodyId:body,sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length:20});
   return v.document.versionId;
  },parametric);
  await page.goto(`/documents/${parametric.id}`);await observeViewport(page);
  await expect.poll(()=>page.evaluate(()=>[...(window as any).__cameraEngine.solidBindings.values()].filter((b:any)=>b.artifact.mesh.edgeProfile?.source==='OCCT_EDGE_USE_V1').length)).toBe(1);
  reports.push({name:'parametric-cube',documentId:parametric.id,source:'Sketch/Profile/LinearExtrude via formal commands'});
  await page.screenshot({path:path.join(output,'live-parametric-cube.png')});
  const assembly=await page.evaluate(async cube=>{
   const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PRODUCT','Natural edge instance acceptance');
   for(let i=0;i<4;i++){
    v=await api.command(v.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:cube});
    const instance=v.product!.instances.at(-1)!;
    v=await api.command(v.document.id,{type:'MOVE_INSTANCE',instanceId:instance.id,translation:[(i%2)*40,Math.floor(i/2)*40,0],rotation:[0,0,Math.sin(i*.2),Math.cos(i*.2)]});
   }
   return v.document.id;
  },documents[0]);documents.push(assembly);
  await page.goto(`/documents/${assembly}`);await observeViewport(page);
  await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.solidBindings.size)).toBe(4);
  const shared=await page.evaluate(()=>{
   const e=(window as any).__cameraEngine,b=[...e.solidBindings.values()];
   return {bindings:b.length,geometries:new Set(b.map((v:any)=>v.group.children.find((c:any)=>c.userData.cadWideEdges).geometry)).size,paths:b.map((v:any)=>v.context.instancePath.canonical),profiles:b.map((v:any)=>v.artifact.mesh.edgeProfile.source)};
  });
  expect(shared.geometries).toBe(1);expect(new Set(shared.paths).size).toBe(4);expect(errors).toEqual([]);
  await page.screenshot({path:path.join(output,'live-assembly.png')});
  fs.writeFileSync(path.join(output,'live-chain.json'),JSON.stringify({reports,assembly:shared,cleanup:'Created documents deleted after acceptance; shared storage retained'},null,2));
 } finally {
  for(const id of documents.reverse())await page.request.delete(`/api/documents/${id}`);
 }
});
