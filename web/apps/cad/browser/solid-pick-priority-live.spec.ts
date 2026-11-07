import {expect,test} from '@playwright/test';
import fs from 'node:fs/promises';
import {observeViewport} from './camera-helpers';

test('solid topology wins over coincident visible sketches, with compact vertices and readable curves',async({browser},info)=>{
 test.setTimeout(240000);let fixture:any;const evidence:any[]=[];
 for(const dpr of [1,2]){
  const context=await browser.newContext({baseURL:'http://127.0.0.1:5175',viewport:{width:1440,height:1000},deviceScaleFactor:dpr});
  try{
   const page=await context.newPage(),errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(/Shader Error|VALIDATE_STATUS|THREE.WebGLProgram/.test(m.text()))errors.push(m.text())});
   expect((await context.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();await page.goto('/');
   if(!fixture)fixture=await page.evaluate(async()=>{
    // @ts-ignore Vite application module
    const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Visible profile and solid edges');const part=v.document.id;
    v=await api.command(part,{type:'CREATE_SKETCH',plane:'XY'});const sketch=v.part!.features.at(-1)!.id;
    await api.command(part,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:-20,y:-16},second:{x:20,y:16}},{type:'ADD_ENTITY',entity:{id:'hole',kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius:6}},{type:'ADD_ENTITY',entity:{id:'corner-point',kind:'POINT',role:'CONSTRUCTION',point:{x:-20,y:-16}}}]});
    await api.command(part,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length:12});
    v=await api.command(part,{type:'SET_DEFINITION_VISIBILITY',targetKind:'SKETCH',targetId:sketch,visible:true});
    let p=await api.createDocument('PRODUCT','Coincident sketch selection');p=await api.command(p.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:part});return {part,product:p.document.id,sketch,revision:v.document.versionId};
   });
   for(const mode of ['part','product'])for(const projection of ['PERSPECTIVE','ORTHOGRAPHIC']){
    await page.goto('/documents/'+fixture[mode]);await observeViewport(page);await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine;return e.solidBindings.size===1&&!e.viewTransition.active})).toBeTruthy();
    await page.evaluate(async(projection:string)=>{const e=(window as any).__cameraEngine;e.viewTransition.cancel();e.selectMany([],true);e.preselect(null,true);e.setDisplaySettings(e.referenceVisibility,{mode:'shaded',edges:true,vertices:false});e.setSceneSettings({...e.sceneSettings,projection,grid:false,worldGrid:false});e.navigation.target.set(0,0,6);e.camera.position.set(55,-70,-65);e.camera.up.set(0,0,1);e.camera.lookAt(e.navigation.target);e.navigation.syncCamera(false);e.updateCameraClipping();e.invalidate();await new Promise(requestAnimationFrame)},projection);
    const samples=await page.evaluate(()=>{
     const e=(window as any).__cameraEngine,c=e.renderer.domElement,r=c.getBoundingClientRect(),binding=[...e.solidBindings.values()][0] as any;
     return [[0,-16,0],[-20,-16,0],[0,6,0]].map(point=>{const p=e.navigation.target.clone().fromArray(point).project(e.camera),x=(p.x+1)*r.width/2,y=(1-p.y)*r.height/2,hit=e.hitTest(x,y);const all=e.selectionIndex.pickAll(e.raycaster,()=>true,32);return {active:e.activeSketchID,feature:e.featureSelection?.role,x:r.x+x,y:r.y+y,hit,candidates:all.map((h:any)=>h.selection),owner:binding.context}});
    });
    await fs.writeFile(info.outputPath(`${mode}-${projection}-candidates-dpr${dpr}.json`),JSON.stringify(samples,null,2));
    expect(samples[0].hit.kind).toBe('edge');expect(samples[1].hit.kind).toBe('vertex');expect(samples[2].hit.kind).toBe('edge');
    for(const sample of samples){expect(sample.candidates.some((s:any)=>s.kind==='visual'&&s.featureId===fixture.sketch)).toBe(true);expect(sample.hit.occurrencePath).toBe(sample.owner.occurrencePath);expect(sample.hit.geometryKey).toBe(sample.owner.geometryKey)}
    const style=await page.evaluate(()=>{const e=(window as any).__cameraEngine,b=[...e.solidBindings.values()][0] as any;return {edges:b.group.children.filter((o:any)=>o.userData.cadWideEdges).map((o:any)=>({width:o.material.linewidth,aa:o.material.alphaToCoverage,toneMapped:o.material.toneMapped})),points:b.group.children.filter((o:any)=>o.isPoints).map((o:any)=>({visible:o.material.visible,size:o.material.userData.cssPointSize}))}});
    expect(style.edges).toEqual([{width:1.35,aa:true,toneMapped:false}]);expect(style.points.every((p:any)=>!p.visible&&p.size===4.5)).toBe(true);
    await page.mouse.move(1430,990);await page.screenshot({path:info.outputPath(`${mode}-${projection}-edges-dpr${dpr}.png`)});
    await page.mouse.move(samples[1].x,samples[1].y);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.preselected?.kind)).toBe('vertex');
    const hover=await page.evaluate(()=>{const o=(window as any).__cameraEngine.preselectedOverlays.at(-1);return {size:o.material.userData.cssPointSize,pixels:o.material.uniforms.uPointSize.value,shape:o.material.uniforms.uMarkerShape.value}});expect(hover).toEqual({size:5,pixels:5*dpr,shape:2});
    await page.screenshot({path:info.outputPath(`${mode}-${projection}-vertex-hover-dpr${dpr}.png`)});await page.mouse.click(samples[1].x,samples[1].y);await page.mouse.move(1430,990);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.selected[0]?.kind)).toBe('vertex');expect(await page.evaluate(()=>(window as any).__cameraEngine.selectedOverlays.at(-1).material.userData.cssPointSize)).toBe(6);
    // The same overlap remains selectable by a command that explicitly needs a profile.
    const profile=await page.evaluate(({sketch,x,y}:any)=>{const e=(window as any).__cameraEngine,b=[...e.solidBindings.values()][0] as any,r=e.renderer.domElement.getBoundingClientRect();e.setFeatureSelection({role:'profile',documentId:b.context.documentId,versionId:b.context.versionId,occurrencePath:b.context.occurrencePath,sketchIds:[sketch],selections:[],onPick(){}});const hit=e.hitTest(x-r.x,y-r.y);e.setFeatureSelection();return hit},{sketch:fixture.sketch,x:samples[0].x,y:samples[0].y});expect(profile.kind).toBe('sketch');expect(profile.id).toBe(fixture.sketch);
    if(mode==='part'){
     const active=await page.evaluate(async(sketch:string)=>{const e=(window as any).__cameraEngine;e.beginSketch(sketch,'XY',true);e.invalidate();await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);return {active:e.activeSketchID,points:(()=>{const p:any[]=[];e.scene.traverse((o:any)=>{if(o.material?.uniforms?.uMarkerShape?.value===1)p.push({depth:o.material.depthTest,size:o.material.userData.cssPointSize})});return p})()}},fixture.sketch);
     expect(active.active).toBe(fixture.sketch);expect(active.points.some((p:any)=>!p.depth&&p.size>=5)).toBe(true);await page.screenshot({path:info.outputPath(`sketch-edit-${projection}-dpr${dpr}.png`)});await page.evaluate(()=>{const e=(window as any).__cameraEngine;e.endSketch();e.viewTransition.cancel()});
    }
    evidence.push({mode,projection,dpr,samples,style,hover,profile});
   }
   expect(errors).toEqual([]);
  }finally{await context.close()}
 }
 await fs.writeFile(info.outputPath('selection-and-style.json'),JSON.stringify(evidence,null,2));
});
