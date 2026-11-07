import {expect,test} from '@playwright/test';
import fs from 'node:fs/promises';
import {observeViewport} from './camera-helpers';

test('solid and face highlights retain material appearance and use spatial contour lines',async({page},info)=>{
 test.setTimeout(120000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(/Shader Error|VALIDATE_STATUS|THREE.WebGLProgram/.test(m.text()))errors.push(m.text())});expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();await page.goto('/');
 const id=await page.evaluate(async()=>{
  // @ts-ignore Vite production API
  const {restApi:api}=await import('/src/api.ts');let p=await api.createDocument('PART','Contour ring');p=await api.command(p.document.id,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=p.part!.features.at(-1)!.id;
  await api.command(p.document.id,{type:'EDIT_SKETCH',sketchId,operations:[{type:'ADD_RECTANGLE',first:{x:-10,y:-8},second:{x:10,y:8}},{type:'ADD_ENTITY',entity:{id:'bore',kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius:5}}]});await api.command(p.document.id,{type:'CREATE_SOLID_FEATURE',sketchId,generator:'LINEAR_EXTRUDE',operation:'ADD',length:10});
  let v=await api.createDocument('PRODUCT','Contour and coincident faces');for(const x of [0,8]){v=await api.command(v.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:p.document.id});v=await api.command(v.document.id,{type:'MOVE_INSTANCE',instanceId:v.product!.instances.at(-1)!.id,translation:[x,0,0],rotation:[0,0,0,1]})}return v.document.id;
 });
 await page.goto('/documents/'+id);await observeViewport(page);await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine;return e.solidBindings.size===2&&!e.viewTransition.active})).toBeTruthy();
 await page.evaluate(async()=>{
  // @ts-ignore Vite production preferences
  const {useUIPreferences}=await import('/src/state/ui-preferences.ts');useUIPreferences.getState().setSolidDisplay({mode:'shaded',edges:false,vertices:false});
 });await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.solidDisplay.edges)).toBe(false);
 const baseline=await page.evaluate(()=>{const e=(window as any).__cameraEngine;e.preselect(null,true);e.selectMany([],true);return [...e.solidBindings.values()].flatMap((b:any)=>(Array.isArray(b.mesh.material)?b.mesh.material:[b.mesh.material]).map((m:any)=>({color:m.color.getHex(),emissive:m.emissive.getHex(),opacity:m.opacity}))) });
 await page.screenshot({path:info.outputPath('neutral-coplanar-parts.png')});
 const selected=await page.evaluate(()=>{const e=(window as any).__cameraEngine,b=[...e.solidBindings.values()][0] as any;e.selectMany([b.group.userData],true);e.invalidate();return {bodyId:b.context.bodyId,occurrence:b.context.occurrencePath}});
 await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.selectedOverlays.length)).toBeGreaterThan(0);
 const appearance=()=>page.evaluate(()=>[...(window as any).__cameraEngine.solidBindings.values()].flatMap((b:any)=>(Array.isArray(b.mesh.material)?b.mesh.material:[b.mesh.material]).map((m:any)=>({color:m.color.getHex(),emissive:m.emissive.getHex(),opacity:m.opacity}))));expect(await appearance()).toEqual(baseline);
 const contours=await page.evaluate(()=>{const e=(window as any).__cameraEngine,values:any[]=[];e.selectedOverlays.forEach((root:any)=>root.traverse((o:any)=>{if(o.material)values.push({line:!!o.material.isLineMaterial,depthTest:o.material.depthTest,depthWrite:o.material.depthWrite,owner:root.parent.userData.occurrencePath})}));return values});expect(contours.length).toBeGreaterThan(0);expect(contours.every(c=>c.line&&c.depthTest&&!c.depthWrite&&c.owner===selected.occurrence)).toBe(true);
 await page.screenshot({path:info.outputPath('body-contours-without-edges.png')});
 await page.evaluate(()=>{const e=(window as any).__cameraEngine,b=[...e.solidBindings.values()][1] as any;const face=b.artifact.mesh.faceIds[0];e.selectMany([],true);e.preselect({...b.context,kind:'face',id:'contour-face',topologyId:face},true);e.invalidate()});
 await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.preselectedOverlays.length)).toBeGreaterThan(0);expect(await appearance()).toEqual(baseline);
 const face=await page.evaluate(()=>{const e=(window as any).__cameraEngine;return {selection:e.preselected,lines:e.preselectedOverlays.map((o:any)=>({line:!!o.material?.isLineMaterial,contour:o.material?.userData.cadSilhouette,adjacency:!!o.geometry?.attributes.instanceEdgeNormalA}))}});expect(face.lines.every((v:any)=>v.line&&v.contour&&v.adjacency)).toBe(true);
 await page.screenshot({path:info.outputPath('face-hover-contours.png')});expect(errors).toEqual([]);
});

test('opaque coplanar ties are stable without swallowing a real gap or glass',async({page},info)=>{
 test.setTimeout(120000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(/Shader Error|VALIDATE_STATUS|THREE.WebGLProgram/.test(m.text()))errors.push(m.text())});await page.goto('/');
 const result=await page.evaluate(async()=>{
  // @ts-ignore Vite production modules
  const source=await (await fetch('/src/cad/rendering/cad-material-factory.ts')).text(),url=source.match(/from\s+["']([^"']*three\.js[^"']*)["']/)?.[1];if(!url)throw Error('Three module URL missing');const T=await import(url);
  // @ts-ignore Vite production modules
  const {CadMaterialFactory}=await import('/src/cad/rendering/cad-material-factory.ts');
  // @ts-ignore Vite production modules
  const {CadShaderLibrary}=await import('/src/cad/rendering/shader/cad-shader-library.ts');
  // @ts-ignore Vite production modules
  const {stableCadOpaqueSort}=await import('/src/cad/rendering/surface-depth-pass.ts');
  const renderer=new T.WebGLRenderer({antialias:true,preserveDrawingBuffer:true});renderer.setSize(800,600);renderer.setClearColor(0x233346);renderer.setOpaqueSort(stableCadOpaqueSort);document.body.append(renderer.domElement);
  const factory=new CadMaterialFactory(new CadShaderLibrary()),scene=new T.Scene();scene.add(new T.AmbientLight(0xffffff,2));let camera=new T.PerspectiveCamera(35,4/3,.2,400),target=new T.Vector3(400,-300,100);
  const first=new T.Mesh(new T.PlaneGeometry(12,12,1,1),factory.surface(0xca4538)),second=new T.Mesh(new T.PlaneGeometry(12,12,7,9),factory.surface(0x31975d));first.userData.cadSurfaceKey='occurrence:A';second.userData.cadSurfaceKey='occurrence:B';first.rotation.set(.3,.2,.1);first.position.copy(target);first.updateMatrixWorld(true);second.geometry.applyMatrix4(first.matrixWorld);scene.add(first,second);
  const normal=new T.Vector3(0,0,1).applyQuaternion(first.quaternion),glass=new T.Mesh(new T.PlaneGeometry(12,12),factory.surface(0xffffff,{opacity:.25}));glass.position.copy(target).addScaledVector(normal,1);glass.quaternion.copy(first.quaternion);glass.visible=false;scene.add(glass);
  const gl=renderer.getContext(),samples=[];for(let x=-3;x<=3;x+=.25)for(let y=-3;y<=3;y+=.25)samples.push(new T.Vector3(x,y,0).applyMatrix4(first.matrixWorld));const cases:any[]=[];
  for(const [name,gap,showGlass,dpr,orthographic] of [['coplanar',0,false,1,false],['behind',-.01,false,1,false],['in-front',.01,false,1,false],['glass',0,true,1,false],['coplanar-dpr2',0,false,2,false],['coplanar-orthographic',0,false,1,true],['coplanar-orthographic-dpr2',0,false,2,true],['behind-orthographic',-.01,false,1,true],['in-front-orthographic',.01,false,1,true]] as const){
   renderer.setPixelRatio(dpr);renderer.setSize(800,600);camera=orthographic?new T.OrthographicCamera(-12,12,9,-9,.2,400):new T.PerspectiveCamera(35,4/3,.2,400);const width=800*dpr,height=600*dpr,pixels=new Uint8Array(width*height*4);
   second.position.copy(normal).multiplyScalar(gap);glass.visible=showGlass;let wrong=0,image='';
   for(let i=0;i<25;i++){
    const offset=new T.Vector3(3,-10,25).applyAxisAngle(new T.Vector3(0,1,0),(i-12)*.004);camera.position.copy(target).add(offset);camera.lookAt(target);camera.updateMatrixWorld(true);scene.children.reverse();factory.surfaceDepth.render(renderer,scene,camera);renderer.setRenderTarget(null);renderer.clear();renderer.render(scene,camera);gl.readPixels(0,0,width,height,gl.RGBA,gl.UNSIGNED_BYTE,pixels);
    for(const point of samples){const p=point.clone().project(camera),n=(Math.round((p.y+1)*height/2)*width+Math.round((p.x+1)*width/2))*4;const green=pixels[n+1]>pixels[n]+10;if(green!==(gap>=0))wrong++;}if(i===12)image=renderer.domElement.toDataURL();
   }
   cases.push({name,wrong,samples:samples.length*25,image});
  }
  const syncPixel=new Uint8Array(4);const timings:{mode:string;ms:number;calls:number;triangles:number}[]=[];renderer.setPixelRatio(1);renderer.setSize(800,600);glass.visible=false;second.position.set(0,0,0);renderer.info.autoReset=false;
  for(let i=0;i<32;i++)for(const enabled of (i%2?[true,false]:[false,true])){
   gl.finish();renderer.info.reset();const start=performance.now();
   if(enabled)factory.surfaceDepth.render(renderer,scene,camera);else factory.surfaceDepth.opaqueUniforms.uCadOpaqueResolve.value=false;
   renderer.setRenderTarget(null);renderer.clear();renderer.render(scene,camera);gl.readPixels(400,300,1,1,gl.RGBA,gl.UNSIGNED_BYTE,syncPixel);
   if(i>=8)timings.push({mode:enabled?'resolved':'colour-only',ms:performance.now()-start,calls:renderer.info.render.calls,triangles:renderer.info.render.triangles});
  }
  const debug=gl.getExtension('WEBGL_debug_renderer_info');const stats={timings,timing:'800x600, 2 planes, alternating order, 8 warmups + 24 paired samples, synchronized pixel read; colour-only skips the depth pass and resolve but retains the shader; not production FPS',renderer:debug?gl.getParameter(debug.UNMASKED_RENDERER_WEBGL):gl.getParameter(gl.RENDERER),depthBits:gl.getParameter(gl.DEPTH_BITS)};factory.surfaceDepth.dispose();renderer.dispose();[first,second,glass].forEach(o=>{o.geometry.dispose();o.material.dispose()});renderer.domElement.remove();return {cases,stats};
 });
 for(const c of result.cases){await fs.writeFile(info.outputPath(`depth-${c.name}.png`),Buffer.from(c.image.split(',')[1],'base64'));delete c.image;}await fs.writeFile(info.outputPath('coplanar-depth.json'),JSON.stringify(result,null,2));for(const c of result.cases)expect(c.wrong,`${c.name} must have one consistent visible material`).toBe(0);expect(errors).toEqual([]);
});
