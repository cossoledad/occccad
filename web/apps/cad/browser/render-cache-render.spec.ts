import {expect,test,type Page} from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
const root=path.resolve('../../..'),fixtures=path.join(root,'build/agent-work/visual-studio/fixtures'),output=path.join(root,'build/agent-work/icon-depth');

async function frame(page:Page){return page.evaluate(async()=>{
 const h=(window as any).edgeHarness,e=h.engine,p=e.materials.surfaceDepth;
 const before={...p.statistics};for(const key of Object.keys(h.cacheTiming))h.cacheTiming[key]=0;
 const result=await h.frame();return {...result,...h.cacheTiming,depthPasses:p.statistics.passes-before.passes,cacheHits:p.statistics.hits-before.hits};
})}

test('depth reuse preserves pixels, invalidates real inputs and avoids full scene walks',async({page})=>{
 fs.mkdirSync(output,{recursive:true});
 const file=path.join(fixtures,'fillet-balanced.glb');expect(fs.existsSync(file),'Generate native SurfaceDisplay fixtures first; see visual-studio validation').toBe(true);
 const bytes=fs.readFileSync(file),digest=createHash('sha256').update(bytes).digest('hex'),errors:string[]=[];
 page.on('pageerror',error=>errors.push(error.message));page.on('console',message=>{if(/Shader Error|THREE.WebGLProgram/.test(message.text()))errors.push(message.text())});
 await page.route('**/edge-fixtures/*.glb',route=>{
  const name=path.basename(new URL(route.request().url()).pathname);expect(name).toMatch(/^(fillet|shallow)-balanced\.glb$/);
  return route.fulfill({body:fs.readFileSync(path.join(fixtures,name)),contentType:'model/gltf-binary'});
 });
 await page.goto('/browser/natural-edges.html');await page.waitForFunction(()=>Boolean((window as any).edgeHarness));
 await page.evaluate(()=>{
  const h=(window as any).edgeHarness,e=h.engine,pass=e.materials.surfaceDepth;
  const timing:any={};h.cacheTiming=timing;
  const raf=requestAnimationFrame.bind(window),invalidate=e.invalidate.bind(e);let owned=false;
  window.requestAnimationFrame=callback=>{const measure=owned;return raf(now=>{const start=performance.now();try{callback(now)}finally{if(measure)timing.frameCpuMs=(timing.frameCpuMs??0)+performance.now()-start}})};
  e.invalidate=()=>{owned=true;try{return invalidate()}finally{owned=false}};
  const depth=pass.render.bind(pass);pass.render=(...args:any[])=>{const start=performance.now();try{return depth(...args)}finally{timing.depthCpuMs=(timing.depthCpuMs??0)+performance.now()-start}};
  for(const name of ['traverse','traverseVisible']){const original=e.scene[name].bind(e.scene);e.scene[name]=(...args:any[])=>{timing[name]=(timing[name]??0)+1;return original(...args)}}
 });
 const reports:any[]=[];
 for(const count of [8,32]){
  await page.evaluate(async args=>(window as any).edgeHarness.load(...args),['fillet-balanced',digest,bytes.length,'#98a8b8',count,'natural',{materialDefaults:true}]);
  await page.evaluate(async()=>{const h=(window as any).edgeHarness,{DEFAULT_SCENE_SETTINGS}=await import('/src/cad/rendering/scene-settings.ts');h.engine.setSceneSettings({...DEFAULT_SCENE_SETTINGS,grid:false,worldGrid:false});for(let i=0;i<3;i++)await h.frame();h.engine.materials.surfaceDepth.setCaching(false);await h.frame()});
  const baseline=[];for(let i=0;i<7;i++)baseline.push(await frame(page));const reference=await page.screenshot();fs.writeFileSync(path.join(output,`depth-${count}-uncached.png`),reference);
  await page.evaluate(async()=>{const h=(window as any).edgeHarness;h.engine.materials.surfaceDepth.setCaching(true);await h.frame()});
  const cached=[];for(let i=0;i<7;i++)cached.push(await frame(page));const current=await page.screenshot();expect(current.equals(reference)).toBe(true);fs.writeFileSync(path.join(output,`depth-${count}-cached.png`),current);
  for(const sample of baseline)expect(sample.depthPasses).toBe(1);
  for(const sample of cached){expect(sample.depthPasses).toBe(0);expect(sample.cacheHits).toBe(1);expect(sample.calls).toBeLessThan(baseline[0].calls);expect(sample.traverse??0).toBe(0);expect(sample.traverseVisible??0).toBe(0)}
  const index=await page.evaluate(()=>{const e=(window as any).edgeHarness.engine;return {objects:e.sceneObjects.size,meshes:e.sceneObjects.meshes.size,points:e.sceneObjects.points.size,screenUpdates:e.sceneObjects.screenObjects.size,bindings:e.solidBindings.size,positionAttributes:new Set([...e.solidBindings.values()].map((b:any)=>b.mesh.geometry.attributes.position)).size}});
  reports.push({count,index,pixelsEqual:true,baseline,cached});
 }
 const mutationResults:any[]=[];
 const compare=async(label:string,mutate:()=>Promise<unknown>,dirty=true)=>{
  const before=await page.evaluate(()=>(window as any).edgeHarness.engine.materials.surfaceDepth.statistics.passes);
  await mutate();const result=await frame(page),after=await page.evaluate(()=>(window as any).edgeHarness.engine.materials.surfaceDepth.statistics.passes);
  const invalidationPasses=after-before;if(dirty)expect(invalidationPasses,label).toBeGreaterThan(0);else expect(invalidationPasses,label).toBe(0);
  const cached=await page.screenshot();await page.evaluate(async()=>{const h=(window as any).edgeHarness;h.engine.materials.surfaceDepth.setCaching(false);await h.frame()});
  expect((await page.screenshot()).equals(cached),label).toBe(true);
  await page.evaluate(async()=>{const h=(window as any).edgeHarness;h.engine.materials.surfaceDepth.setCaching(true);await h.frame()});mutationResults.push({label,pixelsEqual:true,invalidationPasses,...result});
 };
 await compare('lighting',()=>page.evaluate(()=>{const e=(window as any).edgeHarness.engine;e.setSceneSettings({...e.sceneSettings,brightness:e.sceneSettings.brightness*.8})}),false);
 await compare('AO intensity',()=>page.evaluate(()=>{const e=(window as any).edgeHarness.engine;e.setSceneSettings({...e.sceneSettings,aoIntensity:.6})}),false);
 await compare('instance pose',()=>page.evaluate(()=>{const e=(window as any).edgeHarness.engine,b=[...e.solidBindings.values()][0] as any;b.group.position.x+=8;b.group.rotation.z+=.25}));
 await compare('reflection and nonuniform scale',()=>page.evaluate(()=>{const b=[...(window as any).edgeHarness.engine.solidBindings.values()][0] as any;b.group.scale.set(-1.2,.7,1.3)}));
 await compare('ancestor visibility',()=>page.evaluate(()=>{const b=[...(window as any).edgeHarness.engine.solidBindings.values()][0] as any;b.group.visible=false}));
 await compare('camera',()=>page.evaluate(()=>{const e=(window as any).edgeHarness.engine;e.camera.position.x+=12;e.camera.lookAt(e.navigation.target);e.camera.updateMatrixWorld(true)}));
 await compare('projection',()=>page.evaluate(()=>{const e=(window as any).edgeHarness.engine;e.setSceneSettings({...e.sceneSettings,projection:e.sceneSettings.projection==='ORTHOGRAPHIC'?'PERSPECTIVE':'ORTHOGRAPHIC'})}));
 await compare('material colour',()=>page.evaluate(()=>{const b=[...(window as any).edgeHarness.engine.solidBindings.values()][1] as any;for(const m of Array.isArray(b.mesh.material)?b.mesh.material:[b.mesh.material])m.color.set('#bb6644')}),false);
 await compare('position buffer version',()=>page.evaluate(()=>{const b=[...(window as any).edgeHarness.engine.solidBindings.values()][1] as any;const p=b.mesh.geometry.attributes.position;p.setX(0,p.getX(0)+.01);p.needsUpdate=true}));
 await compare('global clipping plane',()=>page.evaluate(()=>{const h=(window as any).edgeHarness;h.engine.renderer.clippingPlanes=[new h.THREE.Plane(new h.THREE.Vector3(1,0,0),0)]}));
 await compare('clear clipping',()=>page.evaluate(()=>(window as any).edgeHarness.engine.renderer.clippingPlanes=[]));
 // Glass requires both all-surface and opaque-only depth targets. Both must be cached.
 await page.evaluate(()=>{const b=[...(window as any).edgeHarness.engine.solidBindings.values()][1] as any;for(const m of Array.isArray(b.mesh.material)?b.mesh.material:[b.mesh.material]){m.opacity=.35;m.transparent=true;m.depthWrite=false}});
 const glass=await frame(page);expect(glass.depthPasses).toBe(2);const glassHit=await frame(page);expect(glassHit.depthPasses).toBe(0);const glassImage=await page.screenshot();
 await page.evaluate(async()=>{const h=(window as any).edgeHarness;h.engine.materials.surfaceDepth.setCaching(false);await h.frame()});expect((await page.screenshot()).equals(glassImage)).toBe(true);fs.writeFileSync(path.join(output,'depth-transparent.png'),glassImage);
 await page.evaluate(async()=>{const h=(window as any).edgeHarness;h.engine.materials.surfaceDepth.setCaching(true);await h.frame()});
 await page.setViewportSize({width:1100,height:800});await page.waitForTimeout(100);await frame(page);const resized=await page.screenshot();
 await page.evaluate(async()=>{const h=(window as any).edgeHarness;h.engine.materials.surfaceDepth.setCaching(false);await h.frame()});expect((await page.screenshot()).equals(resized)).toBe(true);
 const environment=await page.evaluate(()=>{const e=(window as any).edgeHarness.engine,gl=e.renderer.getContext(),d=gl.getExtension('WEBGL_debug_renderer_info');return {renderer:d?gl.getParameter(d.UNMASKED_RENDERER_WEBGL):gl.getParameter(gl.RENDERER),dpr:e.renderer.getPixelRatio()}});
 const snapshot=await page.evaluate(async()=>{const e=(window as any).edgeHarness.engine;e.materials.surfaceDepth.setCaching(true);await(window as any).edgeHarness.frame();const before=e.materials.surfaceDepth.statistics.hits,blob=await e.captureScreenshot();return {type:blob.type,bytes:blob.size,cacheHit:e.materials.surfaceDepth.statistics.hits>before}});expect(snapshot.type).toBe('image/png');expect(snapshot.bytes).toBeGreaterThan(1000);expect(snapshot.cacheHit).toBe(true);
 const sheetBytes=fs.readFileSync(path.join(fixtures,'shallow-balanced.glb')),sheetDigest=createHash('sha256').update(sheetBytes).digest('hex');
 await page.evaluate(async args=>(window as any).edgeHarness.load(...args),['shallow-balanced',sheetDigest,sheetBytes.length,'#587884',1,'natural',{materialDefaults:true,domain:'SHEET_SET'}]);
 await page.evaluate(async()=>{const h=(window as any).edgeHarness;for(let i=0;i<3;i++)await h.frame()});const sheetHit=await frame(page);expect(sheetHit.depthPasses).toBe(0);const sheetImage=await page.screenshot();
 await page.evaluate(async()=>{const h=(window as any).edgeHarness;h.engine.materials.surfaceDepth.setCaching(false);await h.frame()});expect((await page.screenshot()).equals(sheetImage)).toBe(true);fs.writeFileSync(path.join(output,'depth-open-surface.png'),sheetImage);
 fs.writeFileSync(path.join(output,'depth-profile.json'),JSON.stringify({environment,fixture:{name:'fillet-balanced.glb',digest,bytes:bytes.length},conditions:'1280x900 DPR 1, fixed camera/material/light for cache comparisons, grids off, actual OCCT fixture through production display pipeline, 3 warmup frames, 7 serial redraws per mode; CPU callback/submission wall time is not isolated GPU time. Scene traversal counters exclude Three internal matrix/render recursion.',reports,mutationResults,glass:{miss:glass,hit:glassHit,pixelsEqual:true},openSurface:{digest:sheetDigest,hit:sheetHit,pixelsEqual:true,viewport:[1100,800]},resizePixelsEqual:true,snapshot,errors},null,2));expect(errors).toEqual([]);
});
