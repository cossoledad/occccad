import {expect,test} from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
import {observeViewport} from './camera-helpers';
import {treeNavigator} from './tree-helpers';

const evidence=path.resolve('../../../build/tree-geometry-visual');
const stage=process.env.OCCCCAD_VISUAL_STAGE??'after';
test('tree branches, quiet sketches and on-demand solid vertices',async({browser})=>{
 test.setTimeout(240_000);fs.mkdirSync(path.join(evidence,stage),{recursive:true});
 const context=await browser.newContext({baseURL:'http://127.0.0.1:5175',viewport:{width:1440,height:1000},deviceScaleFactor:Number(process.env.OCCCCAD_VISUAL_DPR??1)});
 try{
  const page=await context.newPage(),errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
  page.on('console',m=>{if(/Shader Error|VALIDATE_STATUS|THREE.WebGLProgram/.test(m.text()))errors.push(m.text())});
  expect((await context.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();await page.goto('/');
  const manifest=path.join(evidence,'scene.json');let fixture:any;
  if(fs.existsSync(manifest))fixture=JSON.parse(fs.readFileSync(manifest,'utf8'));
  else{
   fixture=await page.evaluate(async()=>{
    // @ts-ignore Vite application module.
    const {restApi:api}=await import('/src/api.ts');
    let part=await api.createDocument('PART','Sketch presentation');const id=part.document.id;
    part=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const profile=part.part!.features.at(-1)!.id;
    part=await api.command(id,{type:'EDIT_SKETCH',sketchId:profile,operations:[{type:'ADD_RECTANGLE',first:{x:-18,y:-16},second:{x:18,y:16}}]});
    part=await api.command(id,{type:'CREATE_SOLID_FEATURE',sketchId:profile,generator:'LINEAR_EXTRUDE',operation:'ADD',length:24});
    part=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const sketch=part.part!.features.at(-1)!.id;
    part=await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:-32,y:-26},second:{x:32,y:26}},{type:'ADD_ENTITY',entity:{id:'crossing-line',kind:'LINE',role:'PROFILE',start:{x:-32,y:-26},end:{x:32,y:26}}},{type:'ADD_ENTITY',entity:{id:'independent-point',kind:'POINT',role:'PROFILE',point:{x:30,y:-26}}}]});
    let product=await api.createDocument('PRODUCT','Tree and sketch presentation');for(let i=0;i<2;i++)product=await api.command(product.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:id});
    product=await api.command(product.document.id,{type:'MOVE_INSTANCE',instanceId:product.product!.instances[1].id,translation:[80,0,0],rotation:[0,0,0,1]});
    return {part:id,product:product.document.id,sketch,revision:part.document.versionId};
   });fs.writeFileSync(manifest,JSON.stringify(fixture,null,2));
  }
  const results:any[]=[];
  for(const mode of ['part','product']){
   await page.goto('/documents/'+fixture[mode]);await observeViewport(page);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.solidBindings.size)).toBe(mode==='part'?1:2);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.viewTransition.active)).toBe(false);
   const response=await (await context.request.get('/api/documents/'+fixture[mode])).json();
   const find=(n:any):any=>n.entityId===fixture.sketch?n:n.children?.map(find).find(Boolean),node=find(response.structureTree);expect(node).toBeTruthy();await treeNavigator(page)(node.name,node.id);
   const camera=await page.evaluate(async(saved:any)=>{const e=(window as any).__cameraEngine;e.viewTransition.cancel();e.setSceneSettings({...e.sceneSettings,projection:'PERSPECTIVE'});if(saved){e.camera.position.fromArray(saved.position);e.camera.quaternion.fromArray(saved.quaternion);e.camera.zoom=saved.zoom;e.camera.updateProjectionMatrix();e.navigation.target.fromArray(saved.target);e.navigation.syncCamera(false)}e.selectMany([],true);e.preselect(null,true);e.invalidate();await new Promise(requestAnimationFrame);return {position:e.camera.position.toArray(),quaternion:e.camera.quaternion.toArray(),zoom:e.camera.zoom,target:e.navigation.target.toArray()}},fixture[mode+'Camera']);
   if(!fixture[mode+'Camera']){fixture[mode+'Camera']=camera;fs.writeFileSync(manifest,JSON.stringify(fixture,null,2))}
   await page.mouse.move(1430,990);await page.screenshot({path:path.join(evidence,stage,mode+'-normal.png')});
   const normal=await page.evaluate(()=>{const e=(window as any).__cameraEngine,objects:any[]=[];e.scene.traverse((o:any)=>{if((o.userData.sketchEntityOverlay||o.userData.kind==='visual'&&o.userData.featureId)&&o.material)objects.push({depthTest:o.material.depthTest,depthWrite:o.material.depthWrite,width:o.material.linewidth,opacity:o.material.opacity,shape:o.material.uniforms?.uMarkerShape?.value})});return {objects,vertexMaterials:[...e.solidBindings.values()].flatMap((b:any)=>b.group.children.filter((o:any)=>o.isPoints).map((o:any)=>({objectVisible:o.visible,materialVisible:o.material.visible,shape:o.material.uniforms.uMarkerShape.value})))}});
   if(stage!=='before'){
    expect(normal.objects.length).toBeGreaterThan(0);expect(normal.objects.every(o=>o.depthTest&&!o.depthWrite)).toBe(true);
    expect(normal.vertexMaterials.every(v=>v.objectVisible&&!v.materialVisible&&v.shape===2)).toBe(true);
    // Real hover/click through the normal pick path; hidden markers retain pickability.
    const target=await page.evaluate(()=>{const e=(window as any).__cameraEngine,c=e.renderer.domElement,r=c.getBoundingClientRect();for(const b of e.solidBindings.values())for(const v of b.artifact.mesh.topologyVertices??[]){const p=b.mesh.position.clone().fromArray(v.point);b.mesh.localToWorld(p);p.project(e.camera);const x=(p.x+1)*r.width/2,y=(1-p.y)*r.height/2;if(document.elementFromPoint(r.x+x,r.y+y)!==c)continue;const hit=e.hitTest(x,y);if(hit?.kind==='vertex')return {x:r.x+x,y:r.y+y}}});expect(target).toBeTruthy();
    await page.mouse.move(target!.x,target!.y);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.preselected?.kind)).toBe('vertex');
    expect(await page.evaluate(()=>(window as any).__cameraEngine.preselectedOverlays.at(-1).material.uniforms.uMarkerShape.value)).toBe(2);
    await page.screenshot({path:path.join(evidence,stage,mode+'-vertex-hover.png')});await page.mouse.click(target!.x,target!.y);await page.mouse.move(1430,990);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.selected[0]?.kind)).toBe('vertex');
    await page.screenshot({path:path.join(evidence,stage,mode+'-vertex-selected.png')});
   }
   if(mode==='part'){
    const editing=await page.evaluate(async(id:string)=>{const e=(window as any).__cameraEngine;e.beginSketch(id,'XY',true);e.invalidate();await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);const objects:any[]=[];e.scene.traverse((o:any)=>{if((o.userData.sketchEntityOverlay||o.userData.kind==='visual'&&o.userData.featureId)&&o.material)objects.push({depthTest:o.material.depthTest,width:o.material.linewidth,opacity:o.material.opacity,shape:o.material.uniforms?.uMarkerShape?.value})});return objects},fixture.sketch);
    if(stage!=='before'){expect(editing.some(o=>o.width===2.5&&!o.depthTest)).toBe(true);expect(editing.some(o=>o.shape===1)).toBe(true)}
    await page.screenshot({path:path.join(evidence,stage,'part-editing.png')});
   }
   if(stage!=='before'){
    await page.evaluate(()=>{const e=(window as any).__cameraEngine;e.endSketch();e.viewTransition.cancel();e.selectMany([],true);e.preselect(null,true)});
    // Explicit "show vertices" remains a separate viewing preference.
    const visible=await page.evaluate(()=>{const e=(window as any).__cameraEngine;e.setDisplaySettings(e.referenceVisibility,{...e.solidDisplay,vertices:true});const points=[...e.solidBindings.values()].flatMap((b:any)=>b.group.children.filter((o:any)=>o.isPoints));const result=points.every((o:any)=>o.material.visible&&o.material.uniforms.uMarkerShape.value===2);e.setDisplaySettings(e.referenceVisibility,{...e.solidDisplay,vertices:false});return result});expect(visible).toBe(true);
    await page.getByRole('button',{name:'选项',exact:true}).click();const dialog=page.getByRole('dialog',{name:'选项',exact:true});await dialog.getByRole('menuitem',{name:'场景',exact:true}).click();await dialog.getByRole('button',{name:'深色工程',exact:true}).click();await dialog.getByRole('button',{name:'关闭',exact:true}).click();
    await expect(page.locator('.workbench-content')).not.toHaveClass(/light-scene/);
    for(const projection of ['PERSPECTIVE','ORTHOGRAPHIC']){
     const result=await page.evaluate(async(projection:string)=>{const e=(window as any).__cameraEngine;e.setSceneSettings({...e.sceneSettings,projection});e.camera.position.sub(e.navigation.target).multiplyScalar(.85).applyAxisAngle(e.camera.up,.12).add(e.navigation.target);e.camera.lookAt(e.navigation.target);e.navigation.syncCamera(false);e.invalidate();await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);const lines:any[]=[];e.scene.traverse((o:any)=>{if((o.userData.sketchEntityOverlay||o.userData.kind==='visual'&&o.userData.featureId)&&o.material)lines.push({depthTest:o.material.depthTest,opacity:o.material.opacity})});return {pixelRatio:e.renderer.getPixelRatio(),lines}},projection);
     expect(result.pixelRatio).toBe(Number(process.env.OCCCCAD_VISUAL_DPR??1));expect(result.lines.every(o=>o.depthTest)).toBe(true);
     await page.screenshot({path:path.join(evidence,stage,`${mode}-dark-${projection.toLowerCase()}.png`)});
    }
    // Native media preference removes both row relocation and caret rotation motion.
    await page.emulateMedia({reducedMotion:'reduce'});
    expect(await page.locator('.specification-tree-virtual-row').first().evaluate(n=>getComputedStyle(n).transitionDuration)).toBe('0s');await page.emulateMedia({reducedMotion:'no-preference'});
    // Restore neutral viewing for the next fixed scene.
    await page.getByRole('button',{name:'选项',exact:true}).click();await dialog.getByRole('menuitem',{name:'场景',exact:true}).click();await dialog.getByRole('button',{name:'中性工程',exact:true}).click();await dialog.getByRole('button',{name:'关闭',exact:true}).click();
   }
   results.push({mode,camera,normal});
  }
  expect((await (await context.request.get('/api/documents/'+fixture.part)).json()).document.versionId).toBe(fixture.revision);expect(errors).toEqual([]);fs.writeFileSync(path.join(evidence,stage,'results.json'),JSON.stringify({results,errors},null,2));
 }finally{await context.close()}
});
