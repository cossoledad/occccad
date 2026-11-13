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
