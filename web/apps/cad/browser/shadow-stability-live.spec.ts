import {expect,test,type Page} from '@playwright/test';
import fs from 'node:fs/promises';
import {observeViewport} from './camera-helpers';

async function openScene(page:Page){
 expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();await page.goto('/');
 const id=await page.evaluate(async()=>{
  // @ts-ignore actual Vite module
  const {restApi:api}=await import('/src/api.ts');let part=await api.createDocument('PART','Shadow stability cube');const pid=part.document.id;
  part=await api.command(pid,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=part.part!.features.at(-1)!.id;
  await api.command(pid,{type:'EDIT_SKETCH',sketchId,operations:[{type:'ADD_RECTANGLE',first:{x:-5,y:-5},second:{x:5,y:5}}]});await api.command(pid,{type:'CREATE_SOLID_FEATURE',sketchId,generator:'LINEAR_EXTRUDE',operation:'ADD',length:10});
  let v=await api.createDocument('PRODUCT','Shadow stability assembly');for(const x of [0,6,26]){v=await api.command(v.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:pid});v=await api.command(v.document.id,{type:'MOVE_INSTANCE',instanceId:v.product!.instances.at(-1)!.id,translation:[x,0,0],rotation:[0,0,0,1]})}return v.document.id;
 });
 await page.goto('/documents/'+id);await observeViewport(page);await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine;return e.solidBindings.size===3&&!e.viewTransition.active})).toBeTruthy();return id;
}

test('ground shadows remain stable through orbit, projection and DPR changes',async({browser},info)=>{
 test.setTimeout(240_000);const evidence:any[]=[];
 for(const dpr of [1,2]){
  const context=await browser.newContext({baseURL:'http://127.0.0.1:5175',viewport:{width:1440,height:1000},deviceScaleFactor:dpr});
  try{
   const page=await context.newPage(),errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));const id=await openScene(page);
   const results=await page.evaluate(async dpr=>{
    const e=(window as any).__cameraEngine,source=await (await fetch('/src/cad/rendering/scene-presentation.ts')).text(),url=source.match(/from\s+["']([^"']*three\.js[^"']*)["']/)?.[1];if(!url)throw Error('Three module missing');const T=await import(url);
    e.viewTransition.cancel();e.preselect(null,true);e.selectMany([],true);e.helpers.visible=false;e.setSceneSettings({...e.sceneSettings,projection:'PERSPECTIVE',background:'GRADIENT',backgroundTop:'#233346',backgroundBottom:'#506171',grid:false,worldGrid:false,ambientOcclusion:false});
    const meshes=[...e.solidBindings.values()].map((b:any)=>b.mesh);e.content.traverse((o:any)=>{if(o.isLine||o.isPoints||o.isMesh&&!meshes.includes(o))o.visible=false});
    e.renderer.setPixelRatio(dpr);e.renderer.setSize(960,600,false);e.navigation.target.set(13,0,5);
    const floor=e.environment.children.find((o:any)=>o.name==='场景地面'),receiver={position:floor.position.clone(),scale:floor.scale.clone()},footprint=JSON.stringify(e.groundFootprint),groundHeight=e.groundHeight,revision=e.view.document.versionId;
    const gl=e.renderer.getContext(),debug=gl.getExtension('WEBGL_debug_renderer_info'),gpu=debug?gl.getParameter(debug.UNMASKED_RENDERER_WEBGL):gl.getParameter(gl.RENDERER),width=960*dpr,height=600*dpr,pixels=new Uint8Array(width*height*4);
    const samples:number[][]=[];for(const z of [.3,.7,1.5,3,6,8])for(const x of [23,26,29])samples.push([x,-5,z]);
    const draw=()=>{e.renderer.setRenderTarget(null);e.renderer.clear(true,true,true);e.background.render(e.renderer);e.renderer.clearDepth();e.renderer.render(e.scene,e.camera);gl.readPixels(0,0,width,height,gl.RGBA,gl.UNSIGNED_BYTE,pixels);return e.renderer.info.render.calls};
    const read=()=>samples.map(point=>{const p=new T.Vector3().fromArray(point).project(e.camera);if(Math.abs(p.x)>=1||Math.abs(p.y)>=1||Math.abs(p.z)>=1)throw Error('sample outside view');const x=Math.round((p.x+1)*width/2),y=Math.round((p.y+1)*height/2),v=[];for(let j=-1;j<=1;j++)for(let k=-1;k<=1;k++){const n=((y+j)*width+x+k)*4;v.push((pixels[n]+pixels[n+1]+pixels[n+2])/3)}v.sort((a,b)=>a-b);return v[4]});
    const range=(frames:number[][])=>Math.max(...samples.map((_,i)=>Math.max(...frames.map(f=>f[i]))-Math.min(...frames.map(f=>f[i]))));
    const results:any[]=[];
    for(const projection of ['PERSPECTIVE','ORTHOGRAPHIC'])for(const zoom of [1,1.6]){
     e.setSceneSettings({...e.sceneSettings,projection});e.renderer.setSize(960,600,false);
     if(e.camera.isPerspectiveCamera){e.camera.aspect=1.6;e.camera.fov=35;e.camera.zoom=1}else{e.camera.left=-240;e.camera.right=240;e.camera.top=150;e.camera.bottom=-150;e.camera.zoom=300/(25.7*zoom)}
     const frames:number[][]=[],baseline:number[][]=[];let imageBefore='',imageAfter='',drawCalls=0,shadowPixels=0,interiorOverlay=0,near=0;
     for(let i=0;i<25;i++){
      const offset=new T.Vector3(2,-37,17).multiplyScalar(zoom).applyAxisAngle(new T.Vector3(0,0,1),(i-12)*.003);e.camera.position.copy(e.navigation.target).add(offset);e.camera.lookAt(e.navigation.target);e.camera.updateProjectionMatrix();e.camera.updateMatrixWorld(true);e.updateCameraClipping();
      floor.visible=true;floor.position.copy(receiver.position);floor.scale.copy(receiver.scale);drawCalls=draw();frames.push(read());near=e.camera.near;
      if(i===12){imageAfter=e.renderer.domElement.toDataURL();const withFloor=pixels.slice(),interior=frames.at(-1)!;floor.visible=false;draw();interiorOverlay=Math.max(...read().map((v:number,j:number)=>Math.abs(v-interior[j])));for(let n=0;n<pixels.length;n+=4)if(withFloor[n]+withFloor[n+1]+withFloor[n+2]<pixels[n]+pixels[n+1]+pixels[n+2]-6)shadowPixels++;floor.visible=true;}
      if(projection==='PERSPECTIVE'&&zoom===1){
       // Reconstruct only the two diagnosed display settings from the frozen
       // baseline; geometry, lighting, camera pose and resolution stay identical.
       floor.position.set(0,0,groundHeight-1e-5);floor.scale.set(20000,20000,1);const acceptedNear=e.camera.near;e.camera.near=e.camera.position.distanceTo(e.navigation.target)*1e-4;e.camera.updateProjectionMatrix();draw();baseline.push(read());if(i===12)imageBefore=e.renderer.domElement.toDataURL();e.camera.near=acceptedNear;e.camera.updateProjectionMatrix();
      }
     }
     floor.position.copy(receiver.position);floor.scale.copy(receiver.scale);
     results.push({dpr,projection,zoom,maxVariation:range(frames),baselineVariation:baseline.length?range(baseline):undefined,shadowPixels,interiorOverlay,drawCalls,near,imageBefore,imageAfter});
    }
    return {results,gpu,revision,gridUnchanged:footprint===JSON.stringify(e.groundFootprint)&&groundHeight===e.groundHeight,receiver:{position:receiver.position.toArray(),scale:receiver.scale.toArray()},objectShadows:meshes.every((m:any)=>m.castShadow&&m.receiveShadow)};
   },dpr);
   for(const r of results.results){for(const [key,name] of [['imageBefore','before'],['imageAfter','after']] as const){if(r[key])await fs.writeFile(info.outputPath(`${name}-${r.projection.toLowerCase()}-zoom${r.zoom}-dpr${dpr}.png`),Buffer.from(r[key].split(',')[1],'base64'));delete r[key]}evidence.push({...results,results:undefined,...r});expect(r.maxVariation).toBeLessThanOrEqual(1);expect(r.interiorOverlay,'ground cannot shade pixels inside the visible solid').toBeLessThanOrEqual(1);expect(r.shadowPixels,'ground contact shadows remain visible').toBeGreaterThan(50);if(r.baselineVariation!==undefined)expect(r.baselineVariation).toBeGreaterThan(5)}
   expect(results.gridUnchanged).toBe(true);expect(results.objectShadows).toBe(true);expect(errors).toEqual([]);expect((await (await page.request.get('/api/documents/'+id)).json()).document.versionId).toBe(results.revision);
  }finally{await context.close()}
 }
 await fs.writeFile(info.outputPath('shadow-stability.json'),JSON.stringify(evidence,null,2));
});

test('perspective clipping preserves a real bore wall at very close distance',async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();await page.goto('/');
 const id=await page.evaluate(async()=>{
  // @ts-ignore actual Vite module
  const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Close bore clipping');const id=v.document.id;v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=v.part!.features.at(-1)!.id;
  await api.command(id,{type:'EDIT_SKETCH',sketchId,operations:[10,4].map((radius,i)=>({type:'ADD_ENTITY',entity:{id:'ring-'+i,kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius}}))});v=await api.command(id,{type:'CREATE_SOLID_FEATURE',sketchId,generator:'LINEAR_EXTRUDE',operation:'ADD',length:10});return id;
 });
 await page.goto('/documents/'+id);await observeViewport(page);await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine;return e.solidBindings.size===1&&!e.viewTransition.active})).toBeTruthy();
 const result=await page.evaluate(()=>{
  const e=(window as any).__cameraEngine;e.viewTransition.cancel();e.setSceneSettings({...e.sceneSettings,projection:'PERSPECTIVE'});e.camera.position.set(0,0,5);e.navigation.target.set(0,10,5);e.camera.up.set(0,0,1);e.camera.lookAt(e.navigation.target);e.camera.updateMatrixWorld(true);e.navigation.syncCamera(false);
  const canvas=e.renderer.domElement,hit=e.navigationPicker.pickSurface(canvas.clientWidth/2,canvas.clientHeight/2);if(!hit)throw Error('bore wall not found');const direction=e.camera.getWorldDirection(e.camera.position.clone()),point=hit.point.clone();
  e.camera.position.copy(point).addScaledVector(direction,-.0001);e.navigation.target.copy(point).addScaledVector(direction,10);e.camera.lookAt(e.navigation.target);e.camera.updateMatrixWorld(true);e.navigation.syncCamera(false);const before=e.camera.position.toArray();e.updateCameraClipping();e.invalidate();
  const afterHit=e.navigationPicker.pickSurface(canvas.clientWidth/2,canvas.clientHeight/2);return {before,after:e.camera.position.toArray(),near:e.camera.near,wallDepth:afterHit?.distance,projected:point.project(e.camera).z,revision:e.view.document.versionId};
 });
 expect(result.after).toEqual(result.before);expect(result.wallDepth).toBeCloseTo(.0001,6);expect(result.near).toBeLessThan(result.wallDepth!*.1);expect(result.projected).toBeGreaterThan(-1);expect(result.projected).toBeLessThan(1);await page.screenshot({path:info.outputPath('close-bore-wall.png')});await fs.writeFile(info.outputPath('close-bore-clipping.json'),JSON.stringify(result,null,2));expect(errors).toEqual([]);expect((await (await page.request.get('/api/documents/'+id)).json()).document.versionId).toBe(result.revision);
});
