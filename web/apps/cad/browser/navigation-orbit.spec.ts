import {expect,test,type Page} from '@playwright/test';
import {observeViewport} from './camera-helpers';

/** Find actual visible topology in the mounted GLB display. The oracle reads
 * the hit topology's coordinates; it does not call the orbit-target adapter. */
async function candidate(page:Page,kind:'face'|'edge'|'vertex'){
 await observeViewport(page);
 return page.evaluate(kind=>{
  const e=(window as any).__cameraEngine,canvas=e.renderer.domElement,rect=canvas.getBoundingClientRect(),V=e.camera.position.constructor;
  for(const binding of e.solidBindings.values()){
   const mesh=binding.artifact.mesh,world=(p:number[])=>new V(...p).applyMatrix4(binding.mesh.matrixWorld);
   const vertex=(i:number)=>mesh.buffers?Array.from(mesh.buffers.positions.subarray(i*3,i*3+3)) as number[]:mesh.vertices[i];
   const triangle=(i:number)=>mesh.buffers?Array.from(mesh.buffers.indices.subarray(i*3,i*3+3)) as number[]:mesh.triangles[i];
   const faces=mesh.buffers?.faceIds??mesh.faceIds;
   const points:number[][]=kind==='vertex'?mesh.topologyVertices.map((v:any)=>v.point):kind==='edge'?mesh.edges.flatMap((edge:any)=>edge.points.slice(1).flatMap((b:number[],i:number)=>[.25,.5,.75].map(t=>b.map((v,j)=>v*t+edge.points[i][j]*(1-t)))))
    :Array.from(faces,(_:unknown,i:number)=>triangle(i).map(vertex).reduce((sum:number[],p:number[])=>sum.map((v,j)=>v+p[j]/3),[0,0,0]));
   for(const p of points){
    const projected=world(p).project(e.camera),x=(projected.x+1)*rect.width/2,y=(1-projected.y)*rect.height/2;
    if(x<330||x>rect.width-50||y<40||y>rect.height-50||Math.abs(projected.z)>1)continue;
    const hit=e.hitTest(x,y);
    if(hit?.kind!==kind||hit.bodyId!==binding.context.bodyId||(hit.occurrencePath??'')!==(binding.context.occurrencePath??''))continue;
    const coordinates=kind==='vertex'?[mesh.topologyVertices.find((v:any)=>v.localId===hit.topologyId).point]
     :kind==='edge'?mesh.edges.filter((edge:any)=>edge.localId===hit.topologyId).flatMap((edge:any)=>edge.points)
     :Array.from(faces).flatMap((id,i)=>id===hit.topologyId?triangle(i).map(vertex):[]);
    const values=coordinates.map(world),expected=[0,1,2].map(i=>(Math.min(...values.map((v:any)=>v.getComponent(i)))+Math.max(...values.map((v:any)=>v.getComponent(i))))/2);
    return {x:rect.x+x,y:rect.y+y,expected,kind,occurrence:hit.occurrencePath??''};
   }
  }
  throw new Error(`No visible ${kind} candidate`);
 },kind);
}
async function pivot(page:Page){return page.evaluate(()=>(window as any).__cameraEngine.navigation.target.toArray() as number[])}
async function expectPivot(page:Page,point:number[]){expect(await pivot(page)).toEqual(point.map(v=>expect.closeTo(v,7)))}

for(const kind of ['face','edge','vertex'] as const)test(`default orbit locks a visible ${kind} without altering selection`,async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 await page.goto('/documents/mock-part-bracket');await observeViewport(page);
 if(kind==='vertex'){
  await page.getByRole('button',{name:'选项',exact:true}).click();const dialog=page.getByRole('dialog',{name:'选项',exact:true});
  await dialog.getByRole('menuitem',{name:'显示',exact:true}).click();await dialog.getByRole('checkbox',{name:'显示交点',exact:true}).check();await dialog.getByRole('button',{name:'关闭',exact:true}).click();
 }
 const target=await candidate(page,kind);
 await page.evaluate(()=>{const e=(window as any).__cameraEngine,original=e.navigationPicker.pickOrbit.bind(e.navigationPicker);(window as any).orbitPicks=0;e.navigationPicker.pickOrbit=(...args:unknown[])=>{(window as any).orbitPicks++;return original(...args)}});
 const before=await page.evaluate(()=>{const e=(window as any).__cameraEngine;return JSON.stringify([e.selected,e.view.document.versionId])});
 await page.mouse.move(target.x,target.y);await page.mouse.down({button:'right'});await expectPivot(page,target.expected);
 const fixed=await page.evaluate(point=>{const e=(window as any).__cameraEngine;return e.navigation.target.clone().set(...point).project(e.camera).toArray().slice(0,2)},target.expected);
 await page.mouse.move(target.x+75,target.y+45,{steps:8});await expectPivot(page,target.expected);
 const screen=await page.evaluate(point=>{const e=(window as any).__cameraEngine;return e.navigation.target.clone().set(...point).project(e.camera).toArray().slice(0,2)},target.expected);
 expect(Math.max(...screen.map((v:number,i:number)=>Math.abs(v-fixed[i])))).toBeLessThan(1e-8);
 expect(await page.evaluate(()=>(window as any).orbitPicks)).toBe(1);
 await page.mouse.up({button:'right'});
 expect(await page.evaluate(()=>{const e=(window as any).__cameraEngine;return JSON.stringify([e.selected,e.view.document.versionId])})).toBe(before);
 await page.screenshot({path:info.outputPath(`default-${kind}-pivot.png`)});expect(errors).toEqual([]);
});

test('default blank-start orbit acquires once, and assembly faces use occurrence coordinates',async({page},info)=>{
 await page.goto('/documents/mock-product-frame');await observeViewport(page);
 const target=await candidate(page,'face');expect(target.occurrence).not.toBe('');
 const blank=await page.evaluate(()=>{const e=(window as any).__cameraEngine,r=e.renderer.domElement.getBoundingClientRect();
  for(const [x,y] of [[r.width-50,50],[r.width-60,r.height-60],[350,50]])if(!e.navigationPicker.pickOrbit(x,y))return {x:r.x+x,y:r.y+y};throw new Error('No blank area');});
 await page.mouse.move(blank.x,blank.y);await page.mouse.down({button:'right'});
 const camera=await page.evaluate(()=>(window as any).__cameraEngine.camera.position.toArray());
 await page.mouse.move(target.x,target.y);await expectPivot(page,target.expected);
 expect(await page.evaluate(()=>(window as any).__cameraEngine.camera.position.toArray())).toEqual(camera);
 await page.mouse.move(target.x+55,target.y+30,{steps:6});await expectPivot(page,target.expected);await page.mouse.up({button:'right'});
 await info.attach('occurrence-target',{body:JSON.stringify(target,null,2),contentType:'application/json'});
 await page.screenshot({path:info.outputPath('assembly-acquired-pivot.png')});
});
