import {expect,test} from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
import {observeViewport} from './camera-helpers';

const output=path.resolve('../../../build/agent-work/visual-studio/comparison/live');
const fixtures=path.resolve('../../../build/agent-work/visual-studio/fixtures');

test('studio display follows real import, parametric commands, previews and assembly poses',async({page})=>{
 test.setTimeout(300_000);fs.mkdirSync(output,{recursive:true});
 expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBe(true);await page.goto('/');
 const documents:string[]=[],reports:any[]=[],errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const open=async(id:string)=>{
  await page.goto(`/documents/${id}`);await observeViewport(page);
  await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine;return e.solidBindings.size>0&&!e.viewTransition.active})).toBe(true);
  await page.evaluate(async()=>{const e=(window as any).__cameraEngine,{DEFAULT_SCENE_SETTINGS}=await import('/src/cad/rendering/scene-settings.ts');e.preselect(null,true);e.selectMany([],true);e.setSceneSettings({...DEFAULT_SCENE_SETTINGS});e.invalidate();await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame)});
 };
 const preview=async(id:string,name:string)=>{
  let image:any;
  await expect.poll(async()=>{image=await page.request.get(`/api/documents/${id}/preview`);expect(image.ok()).toBe(true);return image.headers()['x-occccad-thumbnail-state']},{timeout:60_000}).toBe('ready');
  const png=await image.body();fs.writeFileSync(path.join(output,`thumbnail-${name}.png`),png);
  const pixels=await page.evaluate(async data=>{const raw=Uint8Array.from(atob(data),c=>c.charCodeAt(0)),bitmap=await createImageBitmap(new Blob([raw],{type:'image/png'})),canvas=document.createElement('canvas');canvas.width=bitmap.width;canvas.height=bitmap.height;const ctx=canvas.getContext('2d')!;ctx.drawImage(bitmap,0,0);const p=ctx.getImageData(0,0,1,1).data;bitmap.close();return {size:[canvas.width,canvas.height],corner:Array.from(p)}} ,png.toString('base64'));
  expect(pixels.size).toEqual([640,400]);expect(pixels.corner[0]).toBeGreaterThan(200);return {state:'ready',etag:image.headers().etag,...pixels};
 };
 try{
  for(const name of ['fillet','shallow','saddle']){
   const job=await page.evaluate(async({name,base64})=>{const {restApi:api}=await import('/src/api.ts');return api.importDocument(new File([Uint8Array.from(atob(base64),c=>c.charCodeAt(0))],`studio-${name}.brep`))},{name,base64:fs.readFileSync(path.join(fixtures,`${name}.brep`)).toString('base64')});
   let finished:any;await expect.poll(async()=>{finished=await(await page.request.get(`/api/jobs/${job.id}`)).json();return finished.state},{timeout:90_000}).toBe('SUCCEEDED');
   const id=finished.documentId;documents.push(id);await open(id);
   for(let i=0;i<12;i++){const row=page.locator('[role=treeitem][aria-expanded=false]').first();if(!await row.count())break;await row.locator('.specification-tree-junction.branch').click()}
   const state=await page.evaluate(name=>{const e=(window as any).__cameraEngine;return {revision:e.view.document.versionId,bindings:[...e.solidBindings.values()].map((b:any)=>({geometryKey:b.artifact.geometryKey,domain:b.artifact.geometryDomain,triangles:b.artifact.triangleCount,vertices:b.artifact.displayVertexCount,normalCount:b.artifact.mesh.buffers.normals.length/3,profile:b.artifact.mesh.edgeProfile.source,naming:Boolean(b.artifact.naming),normalError:name==='shallow'?(()=>{const {positions:p,normals:n}=b.artifact.mesh.buffers;let error=0;for(let i=0;i<p.length;i+=3){const slope=-8*p[i]/900,length=Math.hypot(slope,1);error=Math.max(error,1-(-slope*n[i]+n[i+2])/length)}return error})():undefined}))}},name);
   for(const b of state.bindings){expect(b.vertices).toBe(b.normalCount);expect(b.naming).toBe(true);expect(b.triangles).toBeLessThan(({fillet:4500,shallow:500,saddle:900} as any)[name]);if(name==='shallow')expect(b.normalError).toBeLessThan(.00001)}
   const canvasBefore=await page.locator('canvas').first().boundingBox();
   const row=page.getByRole('treeitem').first();await row.click({button:'right'});await expect(page.getByRole('menu')).toBeVisible();await page.keyboard.press('Escape');await expect(page.getByRole('menu')).not.toBeVisible();
   await page.getByRole('button',{name:'收起模型结构',exact:true}).click();expect(await page.locator('canvas').first().boundingBox()).toEqual(canvasBefore);await page.getByRole('button',{name:'展开模型结构',exact:true}).click();expect(await page.locator('canvas').first().boundingBox()).toEqual(canvasBefore);
   await page.screenshot({path:path.join(output,`${name}.png`)});
   if(name==='shallow'){
    const viewing=[];
    for(const projection of ['PERSPECTIVE','ORTHOGRAPHIC'])for(const dpr of [1,2]){
     const sample=await page.evaluate(async({projection,dpr})=>{const e=(window as any).__cameraEngine,before=e.environment.children.find((o:any)=>o.name==='场景地面').position.toArray();e.setSceneSettings({...e.sceneSettings,projection});e.renderer.setPixelRatio(dpr);e.resize();e.invalidate();await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);return {projection,rendererDPR:e.renderer.getPixelRatio(),groundBefore:before,groundAfter:e.environment.children.find((o:any)=>o.name==='场景地面').position.toArray(),revision:e.view.document.versionId}}, {projection,dpr});
     expect(sample.rendererDPR).toBe(dpr);expect(sample.groundAfter).toEqual(sample.groundBefore);expect(sample.revision).toBe(state.revision);viewing.push(sample);
     await page.screenshot({path:path.join(output,`shallow-${projection.toLowerCase()}-dpr${dpr}.png`)});
    }
    reports.push({name:'viewing-projection-and-renderer-dpr',viewing});
    await page.evaluate(()=>{const e=(window as any).__cameraEngine;e.renderer.setPixelRatio(Math.min(window.devicePixelRatio,2));e.resize()});
   }
   const thumbnail=await preview(id,name);expect((await(await page.request.get(`/api/documents/${id}`)).json()).document.versionId).toBe(state.revision);
   reports.push({name,job:job.id,...state,thumbnail});
  }
  const part=await page.evaluate(async()=>{
   const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Studio parametric annulus');const id=v.document.id,body=v.part!.activeBodyId;
   v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY',bodyId:body});const sketch=v.part!.features.at(-1)!.id;
   await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[10,5].map((radius,i)=>({type:'ADD_ENTITY',entity:{id:`ring-${i}`,kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius}}))});
   v=await api.command(id,{type:'CREATE_SOLID_FEATURE',bodyId:body,sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length:12});return {id,revision:v.document.versionId,appearance:v.part!.defaultAppearance};
  });documents.push(part.id);expect(part.appearance).toMatchObject({color:'#8e9faf',roughness:.5,metalness:.02});await open(part.id);await page.screenshot({path:path.join(output,'parametric.png')});reports.push({name:'parametric',...part,thumbnail:await preview(part.id,'parametric')});
  const assembly=await page.evaluate(async id=>{
   const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PRODUCT','Studio repeated components');
   for(let i=0;i<8;i++){v=await api.command(v.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:id});v=await api.command(v.document.id,{type:'MOVE_INSTANCE',instanceId:v.product!.instances.at(-1)!.id,translation:[i%4*35,Math.floor(i/4)*35,0],rotation:[0,Math.sin(i*.12),0,Math.cos(i*.12)]})}return {id:v.document.id,instance:v.product!.instances.at(-1)!.id};
  },documents[0]);documents.push(assembly.id);await open(assembly.id);
  await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.solidBindings.size)).toBe(8);
  const shared=await page.evaluate(()=>{const e=(window as any).__cameraEngine,b=[...e.solidBindings.values()] as any[];return {geometryCount:new Set(b.map(v=>v.mesh.geometry)).size,positionAttributes:new Set(b.map(v=>v.mesh.geometry.getAttribute('position'))).size,indexAttributes:new Set(b.map(v=>v.mesh.geometry.index)).size,positionArrays:new Set(b.map(v=>v.mesh.geometry.getAttribute('position').array)).size,normalArrays:new Set(b.map(v=>v.mesh.geometry.getAttribute('normal').array)).size,edgeGeometries:new Set(b.map(v=>v.group.children.find((c:any)=>c.userData.cadWideEdges)?.geometry)).size,instances:b.length,paths:b.map(v=>v.context.instancePath.canonical)}});expect(shared.geometryCount).toBe(8);expect(shared.positionAttributes).toBe(1);expect(shared.indexAttributes).toBe(1);expect(shared.positionArrays).toBe(1);expect(shared.normalArrays).toBe(1);expect(shared.edgeGeometries).toBe(1);expect(new Set(shared.paths).size).toBe(8);
  await page.evaluate(async({id,instance})=>{const {restApi:api}=await import('/src/api.ts');await api.command(id,{type:'MOVE_INSTANCE',instanceId:instance,translation:[70,35,16],rotation:[0,0,Math.sin(.4),Math.cos(.4)]})},assembly);
  await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine;return !e.viewTransition.active&&e.view.product.instances.at(-1).translation[2]===16})).toBe(true);
  await page.screenshot({path:path.join(output,'assembly.png')});reports.push({name:'assembly',...shared,thumbnail:await preview(assembly.id,'assembly')});
  expect(errors).toEqual([]);fs.writeFileSync(path.join(output,'chain.json'),JSON.stringify({reports,cleanup:'See cleanup.json; shared database and artifact resources not reset'},null,2));
 }finally{
  const csrf=(await page.context().cookies()).find(c=>c.name==='occccad_csrf')?.value;expect(csrf).toBeTruthy();
  const cleanup=[];
  for(const id of documents.reverse()){
   const response=await page.request.delete(`/api/documents/${id}`,{headers:{'X-CSRF-Token':csrf!}});expect(response.ok()).toBe(true);
   const status=(await page.request.get(`/api/documents/${id}`)).status();expect(status).toBe(404);cleanup.push({id,deleted:true,status});
  }
  fs.writeFileSync(path.join(output,'cleanup.json'),JSON.stringify(cleanup,null,2));
 }
});
