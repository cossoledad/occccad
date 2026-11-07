import {expect,test} from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
import {observeViewport} from './camera-helpers';

const evidence=path.resolve('../../../build/visual-system');
const stage=process.env.OCCCCAD_VISUAL_STAGE??'acceptance';
const manifestPath=path.join(evidence,'scenes.json');
test('fixed engineering scenes and render measurements',async({page},info)=>{
 test.setTimeout(480_000);page.setDefaultTimeout(20_000);fs.mkdirSync(path.join(evidence,stage),{recursive:true});
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(/THREE.WebGLProgram|Shader Error|VALIDATE_STATUS/.test(m.text()))errors.push(m.text())});
 expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();await page.goto('/');
 let scenes:any[];
 if(fs.existsSync(manifestPath))scenes=JSON.parse(fs.readFileSync(manifestPath,'utf8'));
 else {
  scenes=await page.evaluate(async()=>{
   const {restApi:api}=await import('/src/api.ts');
   const part=async(name:string,operations:any[],height:number)=>{let v=await api.createDocument('PART',name);v=await api.command(v.document.id,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=v.part!.features.at(-1)!.id;v=await api.command(v.document.id,{type:'EDIT_SKETCH',sketchId,operations});return api.command(v.document.id,{type:'CREATE_SOLID_FEATURE',sketchId,generator:'LINEAR_EXTRUDE',operation:'ADD',length:height})};
   const plate=await part('Visual acceptance plate',[{type:'ADD_RECTANGLE',first:{x:-40,y:-28},second:{x:40,y:28}}],6);
   const curved=await part('Visual acceptance curved bore',[26,10].map((radius,i)=>({type:'ADD_ENTITY',entity:{id:'ring-'+i,kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius}})),24);
   const thin=await part('Visual acceptance thin walls',[{type:'ADD_RECTANGLE',first:{x:-32,y:-23},second:{x:32,y:23}},{type:'ADD_RECTANGLE',first:{x:-30,y:-21},second:{x:30,y:21}}],35);
   const assembly=async(dense:boolean)=>{let v=await api.createDocument('PRODUCT',dense?'Visual acceptance dense constraints':'Visual acceptance double plates');const id=v.document.id;for(let i=0;i<2;i++)v=await api.command(id,{type:'INSERT_INSTANCE',referencedDocumentId:plate.document.id});const ids=v.product!.instances.map(i=>i.id);v=await api.command(id,{type:'MOVE_INSTANCE',instanceId:ids[1],translation:[8,4,24],rotation:[0,0,0,1]});
    v=await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'FIX',firstAssemblyRef:{instanceId:ids[0],kind:'BODY'}});
    for(let i=0;i<(dense?9:1);i++)v=await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:i%3===1?'ANGLE':'DISTANCE',angleRelation:i%3===1?'PARALLEL':undefined,name:'Constraint.'+(i+1),firstAssemblyRef:{instanceId:ids[0],kind:'PLANE',geometryId:'datum-xy'},secondAssemblyRef:{instanceId:ids[1],kind:'PLANE',geometryId:'datum-xy'},distanceRelation:'SELECTED_PLANE_NORMAL_V1',value:i%3===1?undefined:24});
    if(dense)v=await api.command(id,{type:'SET_ASSEMBLY_CONSTRAINT_STATE',constraintIds:[v.product!.constraints!.at(-1)!.id],suppressed:true});return v};
   const dual=await assembly(false),dense=await assembly(true);
   return [{key:'double-plates',id:dual.document.id},{key:'curved-bore',id:curved.document.id},{key:'thin-walls',id:thin.document.id},{key:'dense-constraints',id:dense.document.id}].map(s=>({...s,created:true}));
  });fs.writeFileSync(manifestPath,JSON.stringify(scenes,null,2));
 }
 const existing=await page.request.get('/api/documents/01a19d32-3e08-7c19-a39f-2ded122c0799');
 if(existing.ok()&&!scenes.some(s=>s.key==='current-double-plates')){const v=await existing.json();if(v.product){scenes.unshift({key:'current-double-plates',id:v.document.id,created:false});fs.writeFileSync(manifestPath,JSON.stringify(scenes,null,2))}}
 const measurements:any[]=[];
 for(const scene of scenes){
  await page.goto('/documents/'+scene.id);await expect(page.locator('canvas')).toBeVisible();await observeViewport(page);
  await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.solidBindings.size)).toBeGreaterThan(0);
  await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.viewTransition.active)).toBe(false);
  const snapshot=await page.evaluate(async scene=>{
   const e=(window as any).__cameraEngine;e.viewTransition.cancel();e.preselect(null,true);e.selectMany([],true);
   // Same input lighting controls, projection, camera and viewport for every stage.
   e.setSceneSettings({...e.sceneSettings,projection:'PERSPECTIVE',brightness:1,ambient:1,fill:1,reflection:1,azimuth:-126.87,elevation:54.46});
   if(scene.camera){e.camera.position.fromArray(scene.camera.position);e.camera.quaternion.fromArray(scene.camera.quaternion);e.camera.zoom=scene.camera.zoom;e.camera.fov=scene.camera.fov;e.camera.updateProjectionMatrix();e.navigation.target.fromArray(scene.camera.target);e.navigation.syncCamera(false)}
   else {e.fit();await new Promise<void>(resolve=>{const tick=()=>e.viewTransition.active?requestAnimationFrame(tick):resolve();requestAnimationFrame(tick)})}
   e.invalidate();return {camera:{position:e.camera.position.toArray(),quaternion:e.camera.quaternion.toArray(),zoom:e.camera.zoom,fov:e.camera.fov,target:e.navigation.target.toArray()},revision:e.view.document.versionId,lighting:e.lighting.children.map((l:any)=>({type:l.type,intensity:l.intensity,color:l.color.getHex()})),settings:{...e.sceneSettings}};
  },scene);
  if(!scene.camera||stage==='baseline'){Object.assign(scene,snapshot);fs.writeFileSync(manifestPath,JSON.stringify(scenes,null,2))}else expect(snapshot.revision).toBe(scene.revision);
  await page.mouse.move(1430,990);await page.waitForTimeout(250);
  await page.screenshot({path:path.join(evidence,stage,scene.key+'.png')});
  const performance=await page.evaluate(async()=>{
   const e=(window as any).__cameraEngine,renderer=e.renderer,original=renderer.render,frames:number[]=[],submissions:number[]=[];let sum=0;
   renderer.render=function(...args:any[]){const start=performance.now();const result=original.apply(this,args);sum+=performance.now()-start;return result};
   try{let last=performance.now();for(let i=0;i<44;i++){e.invalidate();await new Promise(requestAnimationFrame);const now=performance.now();if(i>=12){frames.push(now-last);submissions.push(sum)}sum=0;last=now}}finally{renderer.render=original}
   const stats=(a:number[])=>{a.sort((a,b)=>a-b);return {median:a[Math.floor(a.length*.5)],p95:a[Math.floor(a.length*.95)],samples:a.length}};
   return {frameMs:stats(frames),cpuSubmissionMs:stats(submissions),drawCalls:renderer.info.render.calls,triangles:renderer.info.render.triangles,pixelRatio:renderer.getPixelRatio(),drawingBuffer:[renderer.domElement.width,renderer.domElement.height]};
  });measurements.push({scene:scene.key,...snapshot,...performance});
  if(stage==='workbench'){
   // Supplemental material/edge comparison with the baseline's exact light
   // colours, intensities and PMREM cards. The normal stage captures intentionally
   // show the new illumination preset as part of the delivered visual system.
   fs.mkdirSync(path.join(evidence,'locked-lighting'),{recursive:true});
   const baseline=JSON.parse(fs.readFileSync(path.join(evidence,'baseline','measurements.json'),'utf8')).scenes.find((s:any)=>s.scene===scene.key);
   const actual=await page.evaluate(async baseline=>{
    const e=(window as any).__cameraEngine;e.preselect(null,true);e.selectMany([],true);
    const source=await (await fetch('/src/cad/rendering/studio-environment.ts')).text(),url=source.match(/from\s+["']([^"']*three\.js[^"']*)["']/)?.[1];if(!url)throw Error('Three module missing');const T=await import(url);
    const studio=new T.Scene();studio.background=new T.Color().setRGB(.14,.16,.19);
    const cards=[[9,7,[-4,-5,7],[2.2,2.15,2.05]],[4,8,[6,2,3],[1.5,1.6,1.75]],[5,3,[1,4,8],[1.8,1.9,2.1]],[5,5,[-5,4,1],[.65,.72,.85]]];
    for(const [w,h,position,radiance] of cards as any){const mesh=new T.Mesh(new T.PlaneGeometry(w,h),new T.MeshBasicMaterial({color:new T.Color().setRGB(...radiance),side:T.DoubleSide,toneMapped:false}));mesh.position.fromArray(position);mesh.up.set(0,0,1);mesh.lookAt(0,0,0);studio.add(mesh)}
    const generator=new T.PMREMGenerator(e.renderer),baked=generator.fromScene(studio,.06,.1,100,{size:256});generator.dispose();studio.traverse((o:any)=>{o.geometry?.dispose();o.material?.dispose()});
    (window as any).__lockedLight={original:e.scene.environment,baked};e.scene.environment=baked.texture;
    e.lighting.children.forEach((l:any,i:number)=>{l.intensity=baseline.lighting[i].intensity;l.color.setHex(baseline.lighting[i].color);if(l.groundColor)l.groundColor.setHex(0x667381)});e.invalidate();
    await new Promise(requestAnimationFrame);return e.lighting.children.map((l:any)=>({type:l.type,intensity:l.intensity,color:l.color.getHex()}));
   },baseline);
   expect(actual).toEqual(baseline.lighting);
   await page.screenshot({path:path.join(evidence,'locked-lighting',scene.key+'.png')});
   await page.evaluate(()=>{const e=(window as any).__cameraEngine,v=(window as any).__lockedLight;e.scene.environment=v.original;v.baked.dispose();delete (window as any).__lockedLight;e.setSceneSettings({...e.sceneSettings});e.invalidate()});
  }
 }
 fs.writeFileSync(path.join(evidence,stage,'measurements.json'),JSON.stringify({stage,resolution:[1440,1000],renderer:'Chromium SwiftShader',scenes:measurements,errors},null,2));
 expect(errors).toEqual([]);
});

test('engineering display matrix covers projections, orbit, zoom and native DPR 1/2',async({browser},info)=>{
 test.skip(['baseline','scene','constraints'].includes(stage));test.setTimeout(360_000);
 const scenes=JSON.parse(fs.readFileSync(manifestPath,'utf8')),results:any[]=[];
 fs.mkdirSync(path.join(evidence,'matrix'),{recursive:true});
 for(const dpr of [1,2]){
  const context=await browser.newContext({baseURL:'http://127.0.0.1:5175',viewport:{width:1440,height:1000},deviceScaleFactor:dpr});
  try{
   expect((await context.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();
   const page=await context.newPage(),errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(/Shader Error|VALIDATE_STATUS/.test(m.text()))errors.push(m.text())});
   for(const scene of scenes){
    await page.goto('/documents/'+scene.id);await observeViewport(page);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.solidBindings.size)).toBeGreaterThan(0);
    for(const projection of ['PERSPECTIVE','ORTHOGRAPHIC']){
     const result=await page.evaluate(async({scene,projection})=>{
      const e=(window as any).__cameraEngine;e.viewTransition.cancel();e.setSceneSettings({...e.sceneSettings,projection:'PERSPECTIVE'});e.camera.position.fromArray(scene.camera.position);e.camera.quaternion.fromArray(scene.camera.quaternion);e.camera.zoom=scene.camera.zoom;e.camera.updateProjectionMatrix();e.navigation.target.fromArray(scene.camera.target);e.navigation.syncCamera(false);e.setSceneSettings({...e.sceneSettings,projection});
      for(const scale of [1,.72,1.7]){e.camera.position.sub(e.navigation.target).multiplyScalar(scale).applyAxisAngle(e.camera.up,.16).add(e.navigation.target);e.camera.lookAt(e.navigation.target);e.navigation.syncCamera(false);e.invalidate();await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame)}
      return {pixelRatio:e.renderer.getPixelRatio(),projection:e.camera.isOrthographicCamera?'ORTHOGRAPHIC':'PERSPECTIVE',buffer:[e.renderer.domElement.width,e.renderer.domElement.height],css:[e.renderer.domElement.clientWidth,e.renderer.domElement.clientHeight],settings:e.sceneSettings};
     },{scene,projection});
     expect(result.pixelRatio).toBe(dpr);expect(result.projection).toBe(projection);expect(result.buffer).toEqual(result.css.map(v=>v*dpr));
     await page.screenshot({path:path.join(evidence,'matrix',`${scene.key}-${projection.toLowerCase()}-dpr${dpr}.png`)});results.push({scene:scene.key,dpr,...result});
    }
   }
   expect(errors).toEqual([]);
  }finally{await context.close()}
 }
 fs.writeFileSync(path.join(evidence,'matrix','results.json'),JSON.stringify(results,null,2));
});

test('optional AO is bounded, has no background halo, disables for glass, and presets remain selectable',async({page},info)=>{
 test.skip(['baseline','scene','constraints'].includes(stage));test.setTimeout(180_000);
 expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();
 const scene=JSON.parse(fs.readFileSync(manifestPath,'utf8')).find((s:any)=>s.key==='current-double-plates');await page.goto('/documents/'+scene.id);await observeViewport(page);
 await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.solidBindings.size)).toBeGreaterThan(0);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(/Shader Error|VALIDATE_STATUS/.test(m.text()))errors.push(m.text())});
 const measured=await page.evaluate(async()=>{
  const e=(window as any).__cameraEngine,source=await (await fetch('/src/cad/rendering/scene-presentation.ts')).text(),path=source.match(/from\s+["']([^"']*three\.js[^"']*)["']/)?.[1];if(!path)throw Error('Three module missing');const T=await import(path);
  const renderer=e.renderer,size=renderer.getDrawingBufferSize(new T.Vector2()),target=new T.WebGLRenderTarget(size.x,size.y),depth=e.materials.surfaceDepth,gl=renderer.getContext();
  const draw=(enabled:boolean)=>{depth.render(renderer,e.scene,e.camera,enabled,.6);renderer.setRenderTarget(target);renderer.clear(true,true,false);renderer.render(e.scene,e.camera);gl.finish();renderer.setRenderTarget(null)};
  const pixels=()=>{const p=new Uint8Array(size.x*size.y*4);renderer.readRenderTargetPixels(target,0,0,size.x,size.y,p);return p};
  const sample=(enabled:boolean)=>{const times=[];for(let i=0;i<20;i++){const start=performance.now();draw(enabled);if(i>=4)times.push(performance.now()-start)}times.sort((a,b)=>a-b);return {median:times[8],p95:times[15],samples:times.length}};
  const originalVisibility=[...e.solidBindings.values()].map((b:any)=>[b.mesh,b.mesh.visible]);
  try{
   const off=sample(false),plain=pixels(),on=sample(true),occluded=pixels(),flatEvidence:any[]=[];let darker=0,maxDifference=0,backgroundChanged=0,flatMaxDifference=0,flatSamples=0;
   for(let i=0;i<plain.length;i+=4){const d=Math.max(plain[i]-occluded[i],plain[i+1]-occluded[i+1],plain[i+2]-occluded[i+2]);if(d>1)darker++;maxDifference=Math.max(maxDifference,d);if(plain[i+3]===0&&occluded[i+3]!==0)backgroundChanged++}
   const raycaster=new T.Raycaster(),bodies=[...e.solidBindings.values()].map((b:any)=>b.mesh);
   // Isolate each body: an assembly-plane sample can legitimately be occluded
   // by another component. This separately checks self-occlusion on flat faces.
   const visibility=bodies.map((mesh:any)=>mesh.visible);
   for(const binding of e.solidBindings.values()){
    bodies.forEach((mesh:any)=>{mesh.visible=mesh===binding.mesh});
    draw(false);const isolatedPlain=pixels();draw(true);const isolatedAO=pixels();
    for(const indices of binding.artifact.mesh.triangles){
    const v=indices.map((i:number)=>new T.Vector3(...binding.artifact.mesh.vertices[i]).applyMatrix4(binding.mesh.matrixWorld));
    const normal=v[1].clone().sub(v[0]).cross(v[2].clone().sub(v[0])).normalize(),center=v[0].clone().add(v[1]).add(v[2]).multiplyScalar(1/3);
    if(normal.dot(e.camera.position.clone().sub(center).normalize())<.55)continue;
    const p=center.clone().project(e.camera),px=Math.floor((p.x+1)*size.x/2),py=Math.floor((p.y+1)*size.y/2);if(px<2||py<2||px>=size.x-2||py>=size.y-2)continue;
    raycaster.setFromCamera(new T.Vector2(p.x,p.y),e.camera);const hit=raycaster.intersectObjects(bodies,false)[0];if(!hit||hit.object!==binding.mesh||hit.point.distanceTo(center)>.01)continue;
    const projected=v.map((q:any)=>q.clone().project(e.camera)).map((q:any)=>new T.Vector2(q.x*size.x/2,q.y*size.y/2)),at=new T.Vector2(p.x*size.x/2,p.y*size.y/2);
    if(projected.some((a:any,i:number)=>{const b=projected[(i+1)%3],edge=b.clone().sub(a);return Math.abs(edge.x*(at.y-a.y)-edge.y*(at.x-a.x))/edge.length()<20}))continue;
    const index=(py*size.x+px)*4,difference=Math.max(...[0,1,2].map(c=>Math.abs(isolatedPlain[index+c]-isolatedAO[index+c])));flatSamples++;flatMaxDifference=Math.max(flatMaxDifference,difference);flatEvidence.push({center:center.toArray(),difference});
    }
   }
   bodies.forEach((mesh:any,i:number)=>{mesh.visible=visibility[i]});
   const mesh=e.solidBindings.values().next().value.mesh,materials=Array.isArray(mesh.material)?mesh.material:[mesh.material],saved=materials.map((m:any)=>[m.opacity,m.transparent]);
   materials.forEach((m:any)=>{m.opacity=.5;m.transparent=true});depth.render(renderer,e.scene,e.camera,true,.6);const glassEnabled=depth.aoUniforms.uCadAOEnabled.value;materials.forEach((m:any,i:number)=>{[m.opacity,m.transparent]=saved[i]});
   depth.render(renderer,e.scene,e.camera,false);e.invalidate();return {off,on,darker,maxDifference,backgroundChanged,flatMaxDifference,flatSamples,flatEvidence,glassEnabled,resolution:size.toArray()};
  }finally{originalVisibility.forEach(([mesh,visible]:any)=>{mesh.visible=visible});renderer.setRenderTarget(null);target.dispose();e.invalidate()}
 });
 expect(measured.darker).toBeGreaterThan(50);expect(measured.maxDifference).toBeGreaterThan(1);expect(measured.maxDifference).toBeLessThan(90);expect(measured.backgroundChanged).toBe(0);expect(measured.flatSamples).toBeGreaterThan(2);expect(measured.flatMaxDifference,'a planar face must not occlude itself').toBeLessThanOrEqual(3);expect(measured.glassEnabled).toBe(false);expect(errors).toEqual([]);
 fs.writeFileSync(path.join(evidence,'ao-measurements.json'),JSON.stringify({renderer:'SwiftShader',timing:'Synchronous render with gl.finish; not production FPS',...measured},null,2));
 const options=async()=>{await page.getByRole('button',{name:'选项',exact:true}).click();const d=page.getByRole('dialog',{name:'选项',exact:true});await d.getByRole('menuitem',{name:'场景',exact:true}).click();return d};
 let dialog=await options();await dialog.getByRole('button',{name:'深色工程',exact:true}).click();await dialog.getByRole('button',{name:'关闭',exact:true}).click();await expect(page.locator('.workbench-content')).not.toHaveClass(/light-scene/);await page.screenshot({path:path.join(evidence,'workbench','dark-preset.png')});
 dialog=await options();await dialog.getByRole('button',{name:'中性工程',exact:true}).click();await dialog.getByRole('tab',{name:'光照',exact:true}).click();await dialog.getByRole('checkbox',{name:'轻量环境遮蔽（AO）',exact:true}).check();await dialog.getByRole('button',{name:'关闭',exact:true}).click();
 await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.materials.surfaceDepth.aoUniforms.uCadAOEnabled.value)).toBe(true);await page.screenshot({path:path.join(evidence,'workbench','ao-enabled.png')});
});
