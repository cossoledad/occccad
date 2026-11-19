import {expect,test} from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';

const root=path.resolve('../../..'),fixtures=path.join(root,'build/agent-work/visual-studio/fixtures');
const phase=process.env.OCCCCAD_VISUAL_STUDIO_PHASE??'current',output=path.join(root,'build/agent-work/visual-studio/comparison',phase);
test.beforeAll(async()=>{
 test.setTimeout(180_000);fs.mkdirSync(output,{recursive:true});
 const result=await promisify(execFile)('mise',['run','check','--scope','geometry','--match','SurfaceDisplay','--jobs','4'],{cwd:root,env:{...process.env,OCCCCAD_VISUAL_STUDIO_DIR:fixtures},timeout:180_000,maxBuffer:8*1024*1024});
 fs.writeFileSync(path.join(output,'native-preparation.log'),result.stdout+result.stderr);
 fs.copyFileSync(path.join(fixtures,'geometry.json'),path.join(output,'geometry.json'));
});

test('virtualized structure tree presentation and closed-menu render cost',async({page})=>{
 await page.goto('/browser/tree-studio.html');await page.waitForFunction(()=>Boolean((window as any).treeHarness));
 await expect(page.getByRole('treeitem').first()).toBeVisible();
 await page.evaluate(()=>(window as any).treeHarness.samples.splice(0));
 for(let i=0;i<45;i++)await page.evaluate(async i=>{(window as any).treeHarness.highlight(i);await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame)},i);
 const result=await page.evaluate(()=>({nodes:5001,renderedRows:document.querySelectorAll('.specification-tree-virtual-row').length,reactCommitMs:(window as any).treeHarness.samples,treeWidth:document.querySelector('.workbench-structure')!.getBoundingClientRect().width}));
 expect(result.renderedRows).toBeLessThan(80);expect(result.treeWidth).toBe(330);
 await page.getByRole('treeitem').nth(1).click();await expect(page.getByRole('treeitem').nth(1)).toHaveAttribute('aria-selected','true');
 await page.getByRole('treeitem').nth(1).click({button:'right'});await expect(page.getByRole('menu')).toBeVisible();await page.keyboard.press('Escape');await page.mouse.click(900,850);await expect(page.getByRole('menu')).not.toBeVisible();
 await page.screenshot({path:path.join(output,'tree.png')});fs.writeFileSync(path.join(output,'tree-measurements.json'),JSON.stringify(result,null,2));
});

test('studio surfaces, shadows and material contrast use native geometry under fixed view conditions',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(/Shader Error|THREE.WebGLProgram/.test(m.text()))errors.push(m.text())});
 await page.route('**/edge-fixtures/*.glb',async route=>{const file=path.basename(new URL(route.request().url()).pathname);expect(file).toMatch(/^[a-z]+-(fine|legacy|balanced|fast)\.glb$/);await route.fulfill({body:fs.readFileSync(path.join(fixtures,file)),contentType:'model/gltf-binary'})});
 await page.goto('/browser/natural-edges.html');await page.waitForFunction(()=>Boolean((window as any).edgeHarness));
 await page.evaluate(()=>{
  const h=(window as any).edgeHarness,e=h.engine,render=e.renderer.render.bind(e.renderer),gl=e.renderer.getContext();
  const pixel=new Uint8Array(4);
  h.studioTiming={submitMs:0,finishMs:0};e.renderer.render=(...args:any[])=>{const start=performance.now();const result=render(...args);h.studioTiming.submitMs+=performance.now()-start;const finish=performance.now();gl.finish();gl.readPixels(0,0,1,1,gl.RGBA,gl.UNSIGNED_BYTE,pixel);h.studioTiming.finishMs+=performance.now()-finish;return result};
 });
 const reports:any[]=[];
 const cases:Array<[string,string,number]>=[['cube','#98a8b8',1],['fillet','#98a8b8',1],['sphere','#98a8b8',1],['shallow','#98a8b8',1],['saddle','#98a8b8',1],['cylinder','#98a8b8',1],['fillet','#fafafa',1],['fillet','#151b24',1],['fillet','#cf664d',1],['sphere','#98a8b8',16],['sphere','#98a8b8',64]];
 for(const [name,color,count] of cases.filter(c=>!process.env.OCCCCAD_STUDIO_CASES||new RegExp(process.env.OCCCCAD_STUDIO_CASES).test(c.join('-')))){for(const quality of ['legacy','fine','balanced']){
  const file=`${name}-${quality}`,bytes=fs.readFileSync(path.join(fixtures,file+'.glb')),digest=createHash('sha256').update(bytes).digest('hex');
  const loaded=await page.evaluate(async args=>{const h=(window as any).edgeHarness;return h.load(...args)},[file,digest,bytes.length,color,count,'natural',{materialDefaults:true,domain:['shallow','saddle'].includes(name)?'SHEET_SET':'SOLID_SET'}]);
  await page.evaluate(async()=>{
   const h=(window as any).edgeHarness,e=h.engine,{DEFAULT_SCENE_SETTINGS}=await import('/src/cad/rendering/scene-settings.ts');
   e.setSceneSettings({...DEFAULT_SCENE_SETTINGS,grid:false,worldGrid:false});await h.frame();
  });
  const key=`${name}-${color.slice(1)}-${count}-${quality}`;await page.screenshot({path:path.join(output,key+'.png')});
  const frames=[];
  for(let i=0;i<7;i++){
   const f=await page.evaluate(async()=>{const h=(window as any).edgeHarness;h.studioTiming.submitMs=0;h.studioTiming.finishMs=0;const f=await h.frame(.012);return {...f,...h.studioTiming}});
   if(i>1)frames.push(f);
  }
  reports.push({key,name,quality,color,count,digest,...loaded,frames});expect(errors).toEqual([]);
 }}
 // Changing only the map refresh policy must preserve the rendered pixels.
 await page.evaluate(async()=>{const h=(window as any).edgeHarness;h.engine.scenePresentation.setShadowCaching(true);await h.frame()});
 const cached=await page.screenshot();
 const cachedFrame=await page.evaluate(async()=>{const h=(window as any).edgeHarness;return h.frame()});
 const forcedFrame=await page.evaluate(async()=>{const h=(window as any).edgeHarness;h.engine.scenePresentation.setShadowCaching(false);return h.frame()});
 const forced=await page.screenshot();expect(cached.equals(forced)).toBe(true);expect(forcedFrame.calls).toBeGreaterThan(cachedFrame.calls);
 const moved=await page.evaluate(async()=>{const h=(window as any).edgeHarness,e=h.engine;e.scenePresentation.setShadowCaching(true);const b=[...e.solidBindings.values()][0] as any;b.group.position.x+=8;await h.frame();return h.frame()});
 const movedCached=await page.screenshot();
 await page.evaluate(async()=>{const h=(window as any).edgeHarness;h.engine.scenePresentation.setShadowCaching(false);await h.frame()});
 expect(movedCached.equals(await page.screenshot())).toBe(true);expect(movedCached.equals(cached)).toBe(false);
 fs.writeFileSync(path.join(output,'shadow-cache-verification.json'),JSON.stringify({cachedFrame,forcedFrame,moved,cachedMatchesForced:true,movedMatchesForced:true},null,2));
 const environment=await page.evaluate(()=>{const e=(window as any).edgeHarness.engine,gl=e.renderer.getContext(),d=gl.getExtension('WEBGL_debug_renderer_info');return {renderer:d?gl.getParameter(d.UNMASKED_RENDERER_WEBGL):gl.getParameter(gl.RENDERER),pixelRatio:e.renderer.getPixelRatio(),size:[1280,900]}});
 fs.writeFileSync(path.join(output,'viewport-measurements.json'),JSON.stringify({phase,environment,conditions:'serial samples; gl.finish and synchronous 1px readback after each renderer call; submit and completion wait separate; neither is isolated GPU time; RAF wall time inclusive',reports},null,2));
});

test('view-following lighting moves the real shadow and reuses the PMREM', async ({ page }, info) => {
 const errors:string[]=[];
 page.on('pageerror',error=>errors.push(error.message));
 page.on('console',message=>{if(/Shader Error|THREE.WebGLProgram/.test(message.text()))errors.push(message.text());});
 const file='fillet-balanced',payload=fs.readFileSync(path.join(fixtures,file+'.glb'));
 const digest=createHash('sha256').update(payload).digest('hex');
 await page.route('**/edge-fixtures/*.glb',route=>route.fulfill({body:payload,contentType:'model/gltf-binary'}));
 await page.goto('/browser/natural-edges.html');
 await page.waitForFunction(()=>Boolean((window as any).edgeHarness));
 await page.evaluate(args=>(window as any).edgeHarness.load(...args),[file,digest,payload.length,'#98a8b8',1,'natural',{materialDefaults:true}]);
 await page.evaluate(async () => {
  const {engine,frame}=(window as any).edgeHarness;
  const key=engine.lighting.children[1],update=key.shadow.updateMatrices.bind(key.shadow);
  engine.shadowPassCount=0;
  key.shadow.updateMatrices=(light:any)=>{engine.shadowPassCount++;update(light);};
  engine.setSceneSettings({...engine.sceneSettings,groundMode:'AUTO',shadows:true});
  await frame();
 });
 const sample=async(orientationDelta=0)=>page.evaluate(async orientationDelta=>{
  const {engine,frame}= (window as any).edgeHarness;
  const start=engine.shadowPassCount,stats=await frame(orientationDelta),key=engine.lighting.children[1];
  const direction=key.position.clone().sub(key.target.position).normalize();
  const floor=engine.environment.children.find((object:any)=>object.name==='场景地面');
  return {
   ...stats,shadowPasses:engine.shadowPassCount-start,direction:direction.toArray(),
   viewDirection:direction.clone().applyQuaternion(engine.camera.quaternion.clone().invert()).toArray(),
   ground:floor.position.toArray(),environment:engine.scene.environment.uuid,
   shadowMatrix:key.shadow.matrix.toArray(),floorVisible:floor.visible,
  };
 },orientationDelta);
 const world=await sample(.85);
 const fixed=await page.screenshot({path:info.outputPath('lighting-fixed.png')});
 const fixedTurn=await sample(.2);
 expect(fixedTurn.direction).toEqual(world.direction);
 expect(fixedTurn.shadowMatrix).toEqual(world.shadowMatrix);
 expect(fixedTurn.shadowPasses).toBe(0);
 await sample(-.2);
 await page.evaluate(()=>{
  const e=(window as any).edgeHarness.engine;
  e.setSceneSettings({...e.sceneSettings,lightingFrame:'VIEW'});
 });
 const view=await sample();
 const follow=await page.screenshot({path:info.outputPath('lighting-follow.png')});
 expect(fixed.equals(follow)).toBe(false);
 expect(view.environment).toBe(world.environment);
 expect(view.shadowMatrix).not.toEqual(world.shadowMatrix);
 expect(view.ground[2]).toBe(world.ground[2]);
 expect(view.floorVisible).toBe(true);
 const stationary=await sample();
 expect(stationary.shadowPasses).toBe(0);
 expect((await page.screenshot()).equals(follow)).toBe(true);
 const viewTurn=await sample(.2);
 expect(viewTurn.shadowPasses).toBe(1);
 expect(viewTurn.calls).toBeGreaterThan(stationary.calls);
 expect(viewTurn.viewDirection).toEqual(view.viewDirection.map((c:number)=>expect.closeTo(c,10)));
 expect(viewTurn.shadowMatrix).not.toEqual(view.shadowMatrix);
 await page.evaluate(()=>{
  const {engine,THREE}=(window as any).edgeHarness;
  engine.camera.position.add(new THREE.Vector3(2,0,0));
  engine.camera.zoom*=1.1;
  engine.camera.updateProjectionMatrix();
 });
 const panZoom=await sample();
 expect(panZoom.shadowPasses).toBe(0);
 expect(panZoom.shadowMatrix).toEqual(viewTurn.shadowMatrix);
 await page.evaluate(()=>{
  const e=(window as any).edgeHarness.engine;
  e.setSceneSettings({...e.sceneSettings,shadows:false});
 });
 const shadowDisabled=await sample(.2);
 expect(shadowDisabled.shadowPasses).toBe(0);
 expect(shadowDisabled.direction).not.toEqual(panZoom.direction);
 expect(shadowDisabled.environment).toBe(world.environment);
 expect(shadowDisabled.floorVisible).toBe(false);
 await page.screenshot({path:info.outputPath('lighting-follow-no-shadow.png')});

 // With only the environment contributing, turn the fixture and camera
 // together. Their projection is identical and the following PMREM should
 // produce the same lighting under a combined, non-axis-aligned rotation.
 await page.evaluate(async()=>{
  const {engine,frame}=(window as any).edgeHarness;
  engine.lighting.visible=false;
  engine.setDisplaySettings({points:false,planes:false,axes:false,coordinateSystems:false},{...engine.solidDisplay,edges:false});
  await frame();
 });
 const reflectionBefore=await page.screenshot();
 await page.evaluate(async()=>{
  const {engine,frame,THREE}=(window as any).edgeHarness;
  const binding=[...engine.solidBindings.values()][0] as any;
  const center=engine.navigation.target,rotation=new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(1,2,3).normalize(),.8);
  for(const object of [binding.group,engine.camera]){
   object.position.sub(center).applyQuaternion(rotation).add(center);
   object.quaternion.premultiply(rotation);
  }
  engine.refreshContentBounds();
  await frame();
 });
 const reflectionAfter=await page.screenshot();
 await page.evaluate(async()=>{
  const {engine,frame}=(window as any).edgeHarness;
  engine.setSceneSettings({...engine.sceneSettings,lightingFrame:'WORLD'});
  await frame();
 });
 const reflectionWorld=await page.screenshot();
 const reflectionError=await page.evaluate(async images=>{
  const buffers=await Promise.all(images.map(async source=>{
   const image=new Image();image.src=source;await image.decode();
   const canvas=document.createElement('canvas');canvas.width=image.width;canvas.height=image.height;
   const ctx=canvas.getContext('2d')!;ctx.drawImage(image,0,0);
   return ctx.getImageData(0,0,canvas.width,canvas.height).data;
  }));
  return buffers.slice(1).map(data=>{
   let error=0;
   for(let i=0;i<data.length;i+=4)for(let c=0;c<3;c++)error+=Math.abs(data[i+c]-buffers[0][i+c]);
   return error/(data.length*.75);
  });
 },[reflectionBefore,reflectionAfter,reflectionWorld].map(bytes=>'data:image/png;base64,'+bytes.toString('base64')));
 expect(reflectionError[1]).toBeGreaterThan(.5);
 expect(reflectionError[0]).toBeLessThan(reflectionError[1]*.1);
 fs.writeFileSync(info.outputPath('lighting-orbit.json'),JSON.stringify({
  digest,world,fixedTurn,view,stationary,viewTurn,panZoom,shadowDisabled,reflectionError,
 },null,2));
 expect(errors).toEqual([]);
});
