import {expect,test} from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {edgeFixtures as fixtures,prepareEdgeFixtures} from './edge-fixtures';
const output=path.resolve('../../../build/agent-work/natural-edges/comparison');
test.beforeAll(async()=>{test.setTimeout(180_000);await prepareEdgeFixtures()});

// Decode browser screenshots with Canvas: no image processing dependency, and
// comparisons include the actual MSAA, depth and display-colour pipeline.
async function compare(page:any,first:Buffer,second:Buffer,region?:number[]){
 return page.evaluate(async({a,b,region}:any)=>{
  async function pixels(url:string){const image=new Image();image.src=url;await image.decode();const c=document.createElement('canvas');c.width=image.width;c.height=image.height;const x=c.getContext('2d')!;x.drawImage(image,0,0);return {data:x.getImageData(0,0,c.width,c.height).data,width:c.width,height:c.height}}
  const a1=await pixels(a),b1=await pixels(b);let changed=0,total=0,maxDelta=0,brightened=0,ratioError=0;
  const [left,top,right,bottom]=region??[0,0,a1.width,a1.height];
  for(let y=Math.max(0,Math.floor(top));y<Math.min(a1.height,Math.ceil(bottom));y++)for(let x=Math.max(0,Math.floor(left));x<Math.min(a1.width,Math.ceil(right));x++){
   const i=(y*a1.width+x)*4,d=[0,1,2].map(k=>b1.data[i+k]-a1.data[i+k]);
   const delta=Math.max(...d.map(Math.abs));total++;maxDelta=Math.max(maxDelta,delta);if(delta)changed++;
   if(d.some(v=>v>1))brightened++;
   const ratios=[0,1,2].filter(k=>a1.data[i+k]>30).map(k=>b1.data[i+k]/a1.data[i+k]);
   if(ratios.length>1)ratioError=Math.max(ratioError,Math.max(...ratios)-Math.min(...ratios));
  }
  return {changed,total,maxDelta,brightened,ratioError};
 },{a:`data:image/png;base64,${first.toString('base64')}`,b:`data:image/png;base64,${second.toString('base64')}`,region});
}

async function load(page:any,name:string,color='#aebac7',count=1,mode='natural'){
 const data=fs.readFileSync(path.join(fixtures,name+'.glb')),digest=createHash('sha256').update(data).digest('hex');
 return {...await page.evaluate(async(args:any)=>(window as any).edgeHarness.load(...args),[name,digest,data.length,color,count,mode]),digest};
}

async function setup(page:any){
 const errors:string[]=[];page.on('pageerror',(error:Error)=>errors.push(error.message));
 page.on('console',(message:any)=>{if(/THREE.WebGLProgram|Shader Error|VALIDATE_STATUS/.test(message.text()))errors.push(message.text())});
 await page.route('**/edge-fixtures/*.glb',async(route:any)=>{
  const file=path.basename(new URL(route.request().url()).pathname);expect(file).toMatch(/^[a-z]+\.glb$/);
  await route.fulfill({body:fs.readFileSync(path.join(fixtures,file)),contentType:'model/gltf-binary'});
 });
 await page.goto('/browser/natural-edges.html');await page.waitForFunction(()=>Boolean((window as any).edgeHarness));return errors;
}

test('native CAD GLBs show topology-aware edges under fixed materials, cameras and lights',async({page})=>{
 fs.mkdirSync(output,{recursive:true});const errors=await setup(page);
 const reports=[];
 for(const [name,color,count] of [['cube','#fafafa',1],['fillet','#fafafa',1],['cylinder','#819db4',1],['sphere','#fafafa',1],['coplanar','#aebac7',1],['varying','#aebac7',1],['open','#819db4',1],['cube','#151b24',1],['fillet','#151b24',1],['fillet','#db553b',1],['fillet','#2877cf',1],['cube','#aebac7',16],['cube','#aebac7',64]] as const){
  const key=`${name}-${color.slice(1)}-${count}`,screens:Partial<Record<string,Buffer>>={};
  await load(page,name,color,count,'uniform');
  for(const mode of ['uniform','basic','natural'] as const){
   const loaded=await load(page,name,color,count,mode);
   expect(loaded.bindings).toBe(count);expect(loaded.profileSegments).toBeGreaterThan(0);
   screens[mode]=await page.screenshot({path:path.join(output,`${key}-${mode}.png`)});
   const frames=[];for(let i=0;i<4;i++)frames.push(await page.evaluate(async()=>(window as any).edgeHarness.frame()));
   reports.push({key,mode,color,count,...loaded,frames});expect(errors).toEqual([]);
  }
  const identical=await compare(page,screens.basic!,screens.natural!);
  expect(identical.changed,`${key}: optimizations preserve every rendered pixel`).toBe(0);
  if(name==='fillet'&&count===1){
   await page.evaluate(async()=>{const {engine,frame}=(window as any).edgeHarness;engine.setDisplaySettings({points:false,planes:false,axes:false,coordinateSystems:false},{mode:'shaded',edges:false,vertices:false,edgeStyle:'natural'});await frame()});
   const unlined=await page.screenshot();
   const point=await page.evaluate(()=>{const {engine:e,THREE}=(window as any).edgeHarness,p=new THREE.Vector3(10,3,20).project(e.camera);return [(p.x+1)*640,(1-p.y)*450]});
   const blend=await compare(page,unlined,screens.natural!,[point[0]-6,point[1]-6,point[0]+6,point[1]+6]);
   expect(blend.changed,'the smooth join has a faint stroke').toBeGreaterThan(0);
   expect(blend.brightened,'smooth ink darkens the lit surface').toBe(0);
   expect(blend.ratioError,'RGB ratios preserve surface hue to pixel quantization').toBeLessThan(.045);
   reports.at(-1)!.blend=blend;
  }
 }
 const environment=await page.evaluate(()=>{const gl=(window as any).edgeHarness.engine.renderer.getContext(),debug=gl.getExtension('WEBGL_debug_renderer_info');return {renderer:debug?gl.getParameter(debug.UNMASKED_RENDERER_WEBGL):gl.getParameter(gl.RENDERER),vendor:debug?gl.getParameter(debug.UNMASKED_VENDOR_WEBGL):gl.getParameter(gl.VENDOR),userAgent:navigator.userAgent,hardwareConcurrency:navigator.hardwareConcurrency,devicePixelRatio:devicePixelRatio}});
 fs.writeFileSync(path.join(output,'measurements.json'),JSON.stringify({renderer:'Chromium SwiftShader',browser:page.context().browser()?.version(),environment,viewport:[1280,900],reports},null,2));
 expect(errors).toEqual([]);
});

test('camera, scale, occlusion, DPR and exact-edge interactions remain stable',async({page})=>{
 const errors=await setup(page),report:any={};
 await load(page,'cube');
 const before=await page.screenshot();await page.evaluate(async()=>(window as any).edgeHarness.frame());
 expect((await compare(page,before,await page.screenshot())).changed,'an unchanged view has no temporal flicker').toBe(0);
 await page.evaluate(async()=>{
  const {engine:e,frame,THREE}=(window as any).edgeHarness,b:any=[...e.solidBindings.values()][0],wide=b.group.children.find((o:any)=>o.userData.cadWideEdges);
  const {buildEdgeBuffers,edgeLineGeometry}=await import('/src/cad/rendering/natural-edges.ts');
  // Three 0.179's clone deduplicates interleaved arrays by ArrayBuffer, losing
  // distinct GLB view ranges. Build an owned copy for fault injection instead.
  wide.userData.acceptanceOriginal=wide.geometry;wide.geometry=edgeLineGeometry(buildEdgeBuffers(b.artifact.mesh,false));
  const count=wide.geometry.getAttribute('instanceEdgeKind').count;
  wide.geometry.setAttribute('instanceEdgeKind',new THREE.InstancedBufferAttribute(new Float32Array(count).fill(4),1));await frame();
 });
 expect((await compare(page,before,await page.screenshot())).changed,'a false seam tag cannot suppress a right-angle edge').toBe(0);
 await page.evaluate(async()=>{
  const {engine:e,frame,THREE}=(window as any).edgeHarness,b:any=[...e.solidBindings.values()][0],wide=b.group.children.find((o:any)=>o.userData.cadWideEdges),count=wide.geometry.getAttribute('instanceEdgeKind').count;
  for(const side of ['A','B'])for(const end of ['Start','End'])wide.geometry.setAttribute(`instanceEdge${side}${end}`,new THREE.InstancedBufferAttribute(new Float32Array(count*3),3));await frame();
 });
 expect((await compare(page,before,await page.screenshot())).changed,'a missing normal cannot make a tagged edge disappear').toBe(0);
 await page.evaluate(async()=>{const {engine:e,frame}=(window as any).edgeHarness,b:any=[...e.solidBindings.values()][0],wide=b.group.children.find((o:any)=>o.userData.cadWideEdges);wide.geometry.dispose();wide.geometry=wide.userData.acceptanceOriginal;delete wide.userData.acceptanceOriginal;await frame()});
 await page.evaluate(async()=>{const {engine:e,frame}=(window as any).edgeHarness;e.setDisplaySettings({points:false,planes:false,axes:false,coordinateSystems:false},{mode:'shaded',edges:false,vertices:false});await frame()});
 const unlined=await page.screenshot();
 const hidden=await page.evaluate(()=>{const {engine:e,THREE}=(window as any).edgeHarness,p=new THREE.Vector3(0,20,10).project(e.camera);return [(p.x+1)*640,(1-p.y)*450]});
 expect((await compare(page,unlined,before,[hidden[0]-2,hidden[1]-2,hidden[0]+2,hidden[1]+2])).changed,'a rear edge remains occluded').toBe(0);
 await load(page,'varying');
 report.motion=await page.evaluate(async()=>{
  const {engine:e,frame}=(window as any).edgeHarness;
  const {edgePreparationStats}=await import('/src/cad/rendering/natural-edges.ts');edgePreparationStats(true);
  const frames=[];for(let i=0;i<32;i++)frames.push(await frame(.009));
  return {preparation:edgePreparationStats(),frames};
 });expect(report.motion.preparation.builds,'camera motion does not resample static edge evidence').toBe(0);
 await page.screenshot({path:path.join(output,'varying-orbit.png')});
 for(const name of ['small','large','rotated','mirrored']){await load(page,name);await page.screenshot({path:path.join(output,`${name}-natural.png`)});}
 await load(page,'fillet');
 report.zoom=await page.evaluate(async()=>{
  const {engine:e,frame}=(window as any).edgeHarness,{edgePreparationStats}=await import('/src/cad/rendering/natural-edges.ts');edgePreparationStats(true);
  const frames=[];
  for(const zoom of [.5,.75,1,1.002,1.25,2]){e.camera.zoom=zoom;e.camera.updateProjectionMatrix();e.updateCameraClipping();frames.push({zoom,...await frame()})}
  return {preparation:edgePreparationStats(),frames};
 });expect(report.zoom.preparation.builds,'zoom does not rebuild static edge evidence').toBe(0);
 await page.screenshot({path:path.join(output,'fillet-zoom-2.png')});
 await page.evaluate(async()=>{const {engine:e,frame}=(window as any).edgeHarness;e.camera.zoom=1;e.camera.updateProjectionMatrix();e.updateCameraClipping();await frame()});
 for(const projection of ['PERSPECTIVE','ORTHOGRAPHIC']){
  await page.evaluate(async projection=>{const {engine:e,frame}=(window as any).edgeHarness;e.setSceneSettings({...e.sceneSettings,projection});await frame()},projection);
  for(const dpr of [1,2]){
   report[`${projection}-${dpr}`]=await page.evaluate(async dpr=>{const {engine:e,frame}=(window as any).edgeHarness;e.renderer.setPixelRatio(dpr);await frame();const w=[...e.solidBindings.values()][0] as any;const m=w.group.children.find((o:any)=>o.userData.cadWideEdges).material,box=e.renderer.domElement.getBoundingClientRect();return {pixelRatio:e.renderer.getPixelRatio(),width:m.linewidth,resolution:m.resolution.toArray(),cssSize:[box.width,box.height]}},dpr);
   expect(report[`${projection}-${dpr}`].width).toBe(1.35);
   expect(report[`${projection}-${dpr}`].resolution).toEqual([1280,900]);
   expect(report[`${projection}-${dpr}`].cssSize).toEqual([1280,900]);
   await page.screenshot({path:path.join(output,`fillet-${projection.toLowerCase()}-dpr${dpr}.png`)});
  }
 }
 await load(page,'varying');
 report.nonuniform=await page.evaluate(async()=>{
  const {engine:e,frame,THREE}=(window as any).edgeHarness,{edgePreparationStats}=await import('/src/cad/rendering/natural-edges.ts');edgePreparationStats(true);
  const b:any=[...e.solidBindings.values()][0],wide=b.group.children.find((o:any)=>o.userData.cadWideEdges),geometry=wide.geometry;
  e.setSceneSettings({...e.sceneSettings,projection:'PERSPECTIVE'});b.group.scale.set(.5,2,.5);e.content.updateMatrixWorld(true);e.refreshContentBounds();const bounds=e.visibleContentBounds(),center=bounds.getCenter(new THREE.Vector3()),radius=bounds.getSize(new THREE.Vector3()).length();
  e.navigation.target.copy(center);e.camera.position.copy(center).add(new THREE.Vector3(1,-1.5,1).normalize().multiplyScalar(radius*1.8));e.camera.lookAt(center);e.camera.updateMatrixWorld(true);e.navigation.syncCamera(false);e.updateCameraClipping();await frame();
  return {scale:b.group.scale.toArray(),sameGeometry:geometry===wide.geometry,preparation:edgePreparationStats(),camera:{position:e.camera.position.toArray(),target:e.navigation.target.toArray(),center:center.toArray(),projected:center.clone().project(e.camera).toArray(),near:e.camera.near,far:e.camera.far,zoom:e.camera.zoom,kind:e.camera.type},bounds:[bounds.min.toArray(),bounds.max.toArray()]};
 });expect(report.nonuniform.sameGeometry).toBe(true);expect(report.nonuniform.preparation.builds).toBe(0);
 const scaled=await page.screenshot({path:path.join(output,'varying-nonuniform.png')});
 await page.evaluate(async()=>{const {engine:e,frame}=(window as any).edgeHarness;[...e.solidBindings.values()].forEach((b:any)=>b.group.visible=false);await frame()});
 const foreground=await compare(page,await page.screenshot(),scaled);
 fs.writeFileSync(path.join(output,'transform-probe.json'),JSON.stringify({nonuniform:report.nonuniform,foreground},null,2));
 expect(foreground.changed,'the transformed model is actually visible, not just present in a binding').toBeGreaterThan(10_000);
 report.nonuniform.foregroundPixels=foreground.changed;
 await page.evaluate(()=>(window as any).edgeHarness.engine.renderer.setPixelRatio(1));
 await load(page,'sphere');
 const normal=await page.screenshot();
 for(const state of ['hover','selected']){
  await page.evaluate(async state=>{
   const {engine:e,frame}=(window as any).edgeHarness,b:any=[...e.solidBindings.values()][0],p=b.artifact.mesh.edgeProfile;
   let id=0;for(let i=0;i<p.metadata.length;i+=4)if((p.metadata[i+3]&255)===4){id=p.metadata[i];break}
   const selection={...b.context,kind:'edge',topologyId:id,id:`root:${b.artifact.geometryKey}:edge:${id}`};
   state==='hover'?e.preselect(selection):e.select(selection,false);await frame();
  },state);
  const capture=await page.screenshot({path:path.join(output,`sphere-${state}.png`)});
  expect((await compare(page,normal,capture)).changed,'even a suppressed periodic seam has a clear exact-edge highlight').toBeGreaterThan(10);
  await page.evaluate(async()=>{const {engine:e,frame}=(window as any).edgeHarness;e.preselect(null);e.select(null,false);await frame()});
  expect((await compare(page,normal,await page.screenshot())).changed,'highlight cancellation restores the base view').toBe(0);
 }
 fs.writeFileSync(path.join(output,'stability.json'),JSON.stringify(report,null,2));expect(errors).toEqual([]);
});

test('the real decode Worker transfers edge evidence without cloning its typed arrays',async({page})=>{
 const errors=await setup(page);
 const result=await page.evaluate(async()=>{
  const {VisualDecoder}=await import('/src/cad/visual/visual-decoder.ts');
  const original=await(await fetch('/edge-fixtures/fillet.glb')).arrayBuffer(),v=new DataView(original),jsonLength=v.getUint32(12,true),padding=512*1024;
  // JSON whitespace crosses the production 512 KiB Worker threshold while
  // preserving every original OCCT position, accessor and topology locator.
  const padded=new ArrayBuffer(original.byteLength+padding),bytes=new Uint8Array(padded),header=new DataView(padded);
  bytes.set(new Uint8Array(original,0,20+jsonLength));bytes.fill(32,20+jsonLength,20+jsonLength+padding);
  bytes.set(new Uint8Array(original,20+jsonLength),20+jsonLength+padding);header.setUint32(8,padded.byteLength,true);header.setUint32(12,jsonLength+padding,true);
  const decoder=new VisualDecoder(),loaded=await decoder.decode(padded,true,new AbortController().signal);
  const p=loaded.decoded.mesh.edgeProfile!;
  const result={transferred:padded.byteLength===0,segments:p.metadata.length/4,sameA:p.normalA.buffer===loaded.buffer,sameB:p.normalB.buffer===loaded.buffer,samePositions:p.positions.buffer===loaded.buffer,source:p.source,contours:Boolean((loaded.decoded.mesh as any).contourIncidence)};
  decoder.dispose();return result;
 });
 expect(result).toEqual({transferred:true,segments:672,sameA:true,sameB:true,samePositions:true,source:'OCCT_EDGE_USE_V1',contours:true});expect(errors).toEqual([]);
});
