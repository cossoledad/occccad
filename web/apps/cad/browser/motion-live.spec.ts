import {expect,test,type Page} from '@playwright/test';
import {writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';

test.use({actionTimeout:30_000});

async function command(page:Page,name:string){
 await page.getByRole('button',{name:'搜索工具',exact:true}).click();
 await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);
 const item=page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp('^'+name+'$')})});
 await expect(item).toBeEnabled();await item.click();
}
async function current(page:Page,id:string){const r=await page.request.get('/api/documents/'+id);expect(r.ok()).toBeTruthy();return r.json();}

async function viewportEngine(page:Page){
 await page.evaluate(async()=>{
  if((window as any).__dmuEngine)return;
  // Vite can retain a timestamped module URL after HMR even in a fresh page.
  // Observe the prototypes actually loaded by this Workbench, including an
  // earlier live engine; importing only the plain URL can create another class.
  const paths=new Set(['/src/viewport/cad-viewport-engine.ts',...performance.getEntriesByType('resource').map(entry=>new URL(entry.name)).filter(url=>url.pathname==='/src/viewport/cad-viewport-engine.ts').map(url=>url.pathname+url.search)]);
  for(const path of paths){
   const {CadViewportEngine}=await import(path),prototype=CadViewportEngine.prototype;
   if(prototype.__dmuObserved)continue;prototype.__dmuObserved=true;
   const pick=prototype.hitTest;
   prototype.hitTest=function(...args:any[]){(window as any).__dmuEngine=this;return pick.apply(this,args)};
  }
 });
 const box=await page.locator('canvas').boundingBox();if(!box)throw new Error('canvas missing');
 // The recorder can cover either side of the canvas. Find actual exposed DOM
 // canvas pixels instead of sending pointer events into a floating dialog.
 await expect.poll(async()=>{
  const point=await page.evaluate(()=>{const canvas=document.querySelector('canvas');if(!canvas)return;const r=canvas.getBoundingClientRect();for(let y=40;y<r.height-20;y+=40)for(let x=40;x<r.width-20;x+=40)if(document.elementFromPoint(r.x+x,r.y+y)===canvas&&document.elementFromPoint(r.x+x+2,r.y+y+2)===canvas)return {x:r.x+x,y:r.y+y}});
  if(point){await page.mouse.move(point.x,point.y);await page.mouse.move(point.x+2,point.y+2)}
  return page.evaluate(()=>Boolean((window as any).__dmuEngine));
 },{timeout:60_000}).toBeTruthy();return box;
}
async function pickVisibleInstance(page:Page,id:string){
 const box=await viewportEngine(page),dialogs=await page.getByRole('dialog').all();const blocked=[];
 for(const dialog of dialogs){const rect=await dialog.boundingBox();if(rect)blocked.push(rect)}
 const point=await page.evaluate(({id,box,blocked})=>{
  const engine=(window as any).__dmuEngine;
  for(let y=40;y<box.height-20;y+=12)for(let x=40;x<box.width-20;x+=12){
   if(blocked.some((b:any)=>box.x+x>=b.x&&box.x+x<=b.x+b.width&&box.y+y>=b.y&&box.y+y<=b.y+b.height))continue;
   const hit=engine.hitTest(x,y);if(hit?.instanceId===id)return {x,y};
  }
 },{id,box,blocked});if(!point)throw new Error('No visible selectable instance '+id);await page.mouse.click(box.x+point.x,box.y+point.y);
}

async function dragAttachedComponent(page:Page,distance=8){
 const box=await viewportEngine(page);await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__dmuEngine;return {attached:e.moveManipulator.isAttached(),tool:e.activeToolID}})).toMatchObject({attached:true,tool:'assembly.move'});
 const drag=await page.evaluate(({width,height,distance})=>{
  const e=(window as any).__dmuEngine,m=e.moveManipulator;m.object.updateWorldMatrix(true,true);
  const handle=m.handles.find((h:any)=>h.operation==='translate'&&h.axis==='X');
  const p=handle.pick.position.clone().multiplyScalar(1.6);m.object.localToWorld(p);const start=p.clone().project(e.camera);
  const end=p.clone().addScaledVector(handle.pick.position.clone().normalize().applyQuaternion(m.object.quaternion),distance).project(e.camera);
  const locks=(window as any).__dmuToolLocks=[];
  (window as any).__dmuToolObserver?.disconnect();
  (window as any).__dmuToolObserver=new MutationObserver(()=>{for(const name of ['移动组件','旋转接合'])if(document.querySelector<HTMLButtonElement>(`button[aria-label="${name}"]`)?.disabled)locks.push(name)});
  (window as any).__dmuToolObserver.observe(document.documentElement,{subtree:true,attributes:true,attributeFilter:['disabled']});
  return {x:(start.x+1)*width/2,y:(1-start.y)*height/2,toX:(end.x+1)*width/2,toY:(1-end.y)*height/2};
 },{width:box.width,height:box.height,distance});
 await page.mouse.move(box.x+drag.x,box.y+drag.y);await page.mouse.down();await page.mouse.move(box.x+drag.toX,box.y+drag.toY,{steps:8});await page.mouse.up();
 await expect.poll(()=>page.evaluate(()=>(window as any).__dmuEngine.moveInteraction.state),{timeout:60_000}).toBe('committed').catch(async error=>{console.log('MOVE_TRACE',await page.evaluate(()=>({events:(window as any).__dmuMoveEvents,state:(window as any).__dmuEngine.moveInteraction.state,reason:(window as any).__dmuEngine.moveInteraction.reason,selected:(window as any).__dmuEngine.selected})));throw error});
 await expect.poll(()=>page.evaluate(()=>(window as any).__dmuEngine.moveManipulator.isAttached())).toBeTruthy();
 const locks=await page.evaluate(()=>{(window as any).__dmuToolObserver.disconnect();return (window as any).__dmuToolLocks});expect(locks).toEqual([]);
}

async function seedProduct(page:Page,defineMechanism=true,publishSupports=false){
 return page.evaluate(async ({defineMechanism,publishSupports})=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');
  const parts=[];
  for(const [name,radius,length] of [['DMU轴',4,30],['DMU转子',12,4]] as const){
   let p=await api.createDocument('PART',name);
   p=await api.command(p.document.id,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=p.part!.features.at(-1)!.id;
   p=await api.command(p.document.id,{type:'EDIT_SKETCH',sketchId,operations:[{type:'ADD_ENTITY',entity:{id:crypto.randomUUID(),kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius}}]});
   p=await api.command(p.document.id,{type:'PAD_SKETCH',sketchId,length,operation:'ADD'});
   if(publishSupports){
    const body=p.part!.bodies.find(b=>b.id===p.part!.activeBodyId)!,artifact=p.artifacts![body.geometryKey!];
    const faces=[];
    for(let i=1;i<=Number(artifact.topology.faces);i++)faces.push(await api.getTopologyProperties(p.document.id,artifact.geometryKey,'FACE',i,p.document.versionId));
    const cylinder=faces.find(f=>f.geometryType==='CYLINDER');
    const plane=faces.find(f=>f.geometryType==='PLANE'&&f.bbox&&Math.abs(f.bbox.min[2])<1e-6&&Math.abs(f.bbox.max[2])<1e-6);
    if(!cylinder||!plane)throw new Error('Expected real cylinder and z=0 plane faces: '+JSON.stringify(faces.map(f=>({type:f.geometryType,bbox:f.bbox,properties:f.properties}))));
    for(const [name,face] of [['旋转圆柱面',cylinder],['旋转定位面',plane]] as const)p=await api.createPublication(p.document.id,{name,publicationType:'SURFACE',targetKind:'FACE',geometryKey:artifact.geometryKey,topologyId:face.localId,versionId:p.document.versionId});
   }
   parts.push(p.document.id);
  }
  let v=await api.createDocument('PRODUCT','DMU位姿与转换验收');const id=v.document.id;
  for(const part of parts)v=await api.command(id,{type:'INSERT_INSTANCE',referencedDocumentId:part});
  const [shaft,rotor]=v.product!.instances.map(i=>i.id);
  v=await api.command(id,{type:'MOVE_INSTANCE',instanceId:rotor,translation:[22,15,7],rotation:[0,0,0,1]});
  const endpoint=(instanceId:string)=>({instanceId,frame:{translation:[0,0,0] as [number,number,number],rotation:[0,0,0,1] as [number,number,number,number]},axis:{instanceId,kind:'AXIS' as const,geometryId:'axis-system-default',axis:'Z' as const},plane:{instanceId,kind:'PLANE' as const,geometryId:'datum-xy'},capturedX:[1,0,0] as [number,number,number]});
  const mid=crypto.randomUUID(),jid=crypto.randomUUID();
  if(defineMechanism)v=await api.command(id,{type:'SAVE_KINEMATICS',versionId:v.document.versionId,kinematics:{mechanisms:[{id:mid,name:'机械装置.1',unitIds:[shaft,rotor],joints:[{id:crypto.randomUUID(),name:'固定零件.1',kind:'GROUND',first:endpoint(shaft)},{id:jid,name:'旋转.1',kind:'REVOLUTE',first:endpoint(shaft),second:endpoint(rotor),coordinates:[{id:crypto.randomUUID(),role:'AXIAL_ROTATION',dimension:'ANGLE',zero:{value:0,unit:'deg'},direction:1}],axialOffset:{value:7,unit:'mm'}}]}],drivers:[],studies:[],associations:[]}});
  return {id,shaft,rotor,mid,jid,parts,version:v.document.versionId};
 },{defineMechanism,publishSupports});
}

// Production API, native Worker and real renderer. The trace records poses when
// Three.js renders the scene, including writes between React effects.
test('edited Part tab returns to its referencing Product without a sketch context crash',async({page})=>{
 test.setTimeout(240_000);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();
 await page.goto('/');
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');const suffix=crypto.randomUUID().slice(0,8);
  let p=await api.createDocument('PART','Part切换-'+suffix);p=await api.command(p.document.id,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=p.part!.features.at(-1)!.id;
  p=await api.command(p.document.id,{type:'EDIT_SKETCH',sketchId,operations:[{type:'ADD_ENTITY',entity:{id:crypto.randomUUID(),kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius:10}}]});
  p=await api.command(p.document.id,{type:'PAD_SKETCH',sketchId,length:10,operation:'ADD'});
  let product=await api.createDocument('PRODUCT','Product切换-'+suffix);product=await api.command(product.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:p.document.id});
  await api.openDocument(product.document.id);
  const find=(node:any):any=>node.entityId===sketchId?node:(node.children??[]).map(find).find(Boolean);
  return {partId:p.document.id,partName:p.document.name,productId:product.document.id,productName:product.document.name,sketchId,sketchName:find(p.structureTree).name};
 });
 await page.goto('/documents/'+seed.partId);await expect(page.locator('canvas')).toBeVisible();
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill(seed.sketchName);const sketchRow=page.getByRole('treeitem').filter({hasText:seed.sketchName}).first();await expect(sketchRow).toBeVisible();await sketchRow.dblclick();await expect(page.getByRole('button',{name:'退出草图',exact:true})).toBeVisible();
 const changed=await page.evaluate(async seed=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');const p=await api.command(seed.partId,{type:'EDIT_SKETCH',sketchId:seed.sketchId,operations:[{type:'ADD_ENTITY',entity:{id:crypto.randomUUID(),kind:'POINT',role:'CONSTRUCTION',point:{x:20,y:20}}}]});return p.document.versionId;
 },seed);
 await page.locator('.document-tab-switch').filter({hasText:seed.productName}).click();
 await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();await expect(page.locator('canvas')).toBeVisible();
 await expect.poll(async()=>((await current(page,seed.productId)).product.instances[0].resolvedVersionId),{timeout:60_000}).toBe(changed);
 await page.locator('.document-tab-switch').filter({hasText:seed.partName}).click();await expect(page.getByRole('button',{name:'退出草图',exact:true})).toBeVisible();expect(errors).toEqual([]);
});

test('mechanism drafts, refresh, exit and independent conversion retain accepted poses',async({page},testInfo)=>{
 test.setTimeout(300_000);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const frameLog:any[]=[];
 await page.addInitScript(()=>{(window as any).__dmuFrames=[];(window as any).__dmuStage=sessionStorage.getItem('dmu-acceptance-stage')??'initial'});
 // Instrument the real module before its first engine is constructed, including
 // after reload. No API or numerical result is replaced by this trace hook.
 await page.route('**/src/viewport/cad-viewport-engine.ts*',async route=>{
  const response=await route.fetch();const source=await response.text();
  await route.fulfill({response,body:source+`
const dmuCommit=CadViewportEngine.prototype.commitApplicationDisplay;
CadViewportEngine.prototype.commitApplicationDisplay=function(){
 if(!this.renderer.__dmuTrace){
  const render=this.renderer.render.bind(this.renderer);this.renderer.__dmuTrace=true;
  this.renderer.render=(scene,camera)=>{if(scene===this.scene)window.__dmuFrames.push({stage:window.__dmuStage,poses:Object.fromEntries([...this.instanceGroups].map(([id,g])=>[id,{translation:g.position.toArray(),rotation:g.quaternion.toArray()}]))});return render(scene,camera)};
 }
 return dmuCommit.call(this);
};`});
 });
 if(!process.env.OCCCCAD_ADMIN_PASSWORD)throw new Error('Configured development login required');
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();
 await page.goto('/');
 const seed=await seedProduct(page);
 await page.goto('/documents/'+seed.id);await expect(page.locator('canvas')).toBeVisible({timeout:60_000});
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill('机械装置.1');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText(/^机械装置\.1，/).first().click();await command(page,'机构');await page.getByRole('textbox',{name:'筛选模型结构'}).fill('');await expect(page.locator('[data-application-id="'+seed.mid+'"]')).toBeVisible();await expect(page.getByRole('button',{name:'旋转接合',exact:true})).toBeEnabled();
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});
 const edit=async()=>{await filter.fill('偏移.1');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText(/^偏移\.1 \(/).first().dblclick();const d=page.getByRole('dialog',{name:'旋转.1',exact:true});await expect(d).toBeVisible();return d};
 const phase=async(stage:string)=>page.evaluate(s=>{(window as any).__dmuStage=s;sessionStorage.setItem('dmu-acceptance-stage',s)},stage);
 const collect=async()=>{frameLog.push(...await page.evaluate(()=>{const frames=(window as any).__dmuFrames;(window as any).__dmuFrames=[];return frames}));};
 const visibleZ=async(z:number)=>expect.poll(()=>page.evaluate(id=>{const f=(window as any).__dmuFrames.at(-1);return f?.poses[id]?.translation[2]},seed.rotor)).toBeCloseTo(z,5);
 await phase('cancel');let d=await edit();await d.getByRole('spinbutton').first().fill('15');await visibleZ(15);await page.keyboard.press('Escape');await visibleZ(7);
 await phase('confirm');d=await edit();await d.getByRole('spinbutton').first().fill('11');await visibleZ(11);await d.getByRole('button',{name:/^确\s*定$/}).click();await expect(d).toHaveCount(0);await visibleZ(11);
 let v=await current(page,seed.id);expect(v.product.instances.find((i:any)=>i.id===seed.rotor).translation).toEqual([22,15,7]);expect(v.product.kinematics.mechanisms[0].poses[seed.rotor].translation[2]).toBeCloseTo(11,5);
 await filter.fill('');const tree=page.getByRole('complementary',{name:'模型结构',exact:true});
 for(const label of ['Applications','机械装置',/^机械装置\.1，/,'接合',/^旋转\.1 \(/,'固定零件',/^固定零件\.1 \(/]){
  const row=tree.getByRole('treeitem').filter({has:page.getByText(label,{exact:true})}).first();
  if(await row.getAttribute('aria-expanded')==='false')await row.getByRole('button',{name:'展开',exact:true}).click();
 }
 await tree.getByText(/^机械装置\.1，/).click();await page.screenshot({path:'../../../build/acceptance/dmu-mechanism.png'});
 await phase('refresh');await collect();
 await page.reload();await expect(page.locator('[data-application-id="'+seed.mid+'"]')).toBeVisible({timeout:60_000});
 await visibleZ(11);await collect();await phase('exit');
 await command(page,'退出机构');await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();
 v=await current(page,seed.id);expect(v.product.constraints??[]).toHaveLength(0);expect(v.product.instances.find((i:any)=>i.id===seed.rotor).translation[2]).toBeCloseTo(11,5);
 await collect();await phase('reopen');await page.reload();await expect(page.locator('canvas')).toBeVisible({timeout:60_000});await visibleZ(11);v=await current(page,seed.id);expect(v.product.instances.find((i:any)=>i.id===seed.rotor).translation[2]).toBeCloseTo(11,5);
 await phase('conversion');
 await filter.fill('机械装置.1');await tree.getByText(/^机械装置\.1，/).first().click();await command(page,'机构');await filter.fill('');await command(page,'转换为产品约束');const apply=page.getByRole('dialog',{name:'转换为产品约束',exact:true});
 await apply.getByRole('button',{name:'生成／刷新转换计划'}).click();await expect(apply.getByText(/自由度 1/)).toBeVisible();await apply.getByRole('button',{name:/^确\s*定$/}).click();await expect(apply).toHaveCount(0);
 v=await current(page,seed.id);expect(v.product.constraints).toHaveLength(3);expect(v.product.kinematics.drivers??[]).toHaveLength(0);expect(v.product.kinematics.studies??[]).toHaveLength(0);
 const source=v.product.kinematics.mechanisms[0].joints.flatMap((j:any)=>j.constraints.map((c:any)=>c.id));for(const c of v.product.constraints)expect(source).not.toContain(c.id);
 await command(page,'撤销');v=await current(page,seed.id);expect(v.product.constraints??[]).toHaveLength(0);
 await command(page,'重做');v=await current(page,seed.id);expect(v.product.constraints).toHaveLength(3);expect(errors).toEqual([]);
 await collect();expect(frameLog.length).toBeGreaterThan(3);
 for(const f of frameLog.filter(f=>['cancel','confirm','refresh','exit','reopen','conversion'].includes(f.stage)&&f.poses[seed.rotor])){
  expect(Math.hypot(f.poses[seed.rotor].translation[0],f.poses[seed.rotor].translation[1]),f.stage).toBeLessThan(1e-5);
  if(['refresh','exit','reopen','conversion'].includes(f.stage))expect(f.poses[seed.rotor].translation[2],f.stage).toBeCloseTo(11,5);
 }
 const trace=JSON.stringify({documentId:seed.id,mechanismId:seed.mid,rotorId:seed.rotor,frames:frameLog},null,2);
 await testInfo.attach('application-render-frames',{body:trace,contentType:'application/json'});await writeFile(resolve('../../../build/acceptance/dmu-render-frames.json'),trace);
});

test('new mechanisms bind real tree supports through inspection, preview and commit',async({page})=>{
 test.setTimeout(240_000);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();
 await page.goto('/');const seed=await seedProduct(page,false);
 await page.goto('/documents/'+seed.id);await expect(page.locator('canvas')).toBeVisible({timeout:60_000});
 await command(page,'机构');await expect(page.getByRole('button',{name:'固定件',exact:true})).toBeEnabled();
 await command(page,'固定件');await pickVisibleInstance(page,seed.shaft);const ground=page.getByRole('dialog',{name:'固定零件.1',exact:true});
 await expect(ground.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await ground.getByRole('button',{name:/^确\s*定$/}).click();await expect(ground).toHaveCount(0);
 await command(page,'旋转接合');const joint=page.getByRole('dialog',{name:'旋转.1',exact:true});
 const filter=page.getByRole('textbox',{name:'筛选模型结构'}),tree=page.getByRole('complementary',{name:'模型结构',exact:true});
 for(const [label,index,nextRole] of [['Z Axis',0,'第二轴线'],['Z Axis',1,'第一定位平面'],['XY Plane',0,'第二定位平面'],['XY Plane',1,undefined]] as const){
  await filter.fill(label);const support=tree.getByText(label,{exact:true});await expect(support).toHaveCount(2);await support.nth(index).click();
  if(nextRole)await expect(joint.getByRole('button',{name:nextRole,exact:true})).toHaveClass(/ant-btn-primary/);
 }
 await joint.getByRole('spinbutton').first().fill('9');await expect(joint.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await joint.getByRole('button',{name:/^确\s*定$/}).click();await expect(joint).toHaveCount(0);
 let v=await current(page,seed.id);const m=v.product.kinematics.mechanisms[0],r=m.joints.find((j:any)=>j.kind==='REVOLUTE');
 expect(v.product.constraints??[]).toHaveLength(0);expect(v.product.instances.find((i:any)=>i.id===seed.rotor).translation).toEqual([22,15,7]);
 expect(m.poses[seed.rotor].translation[2]).toBeCloseTo(9,5);expect(m.evaluation).toMatchObject({relativeDof:1,gaugeDof:0,driverCount:0});
 expect(r.constraints).toHaveLength(2);
 for(const end of [r.first,r.second])for(const ref of [end.axis,end.plane]){expect(ref.instancePath.rootDocumentId).toBe(seed.id);expect(ref.instancePath.segments[0].instanceId).toBe(end.instanceId);expect(ref.geometryKey??'').toBe('');}
 await filter.fill('偏移.1');await expect(tree.getByText(/^偏移\.1 \(/)).toBeVisible();await page.screenshot({path:'../../../build/acceptance/dmu-created-by-ui.png'});
 await command(page,'退出机构');await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();
 v=await current(page,seed.id);expect(v.product.instances.find((i:any)=>i.id===seed.rotor).translation[2]).toBeCloseTo(9,5);expect(v.product.constraints??[]).toHaveLength(0);expect(errors).toEqual([]);
});


// Real curved/planar face references and the one-dialog computation/replay flow.
test('cylindrical supports, command trial and recorded simulation share qualified poses',async({page})=>{
 test.setTimeout(360_000);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();
 await page.goto('/');const seed=await seedProduct(page,false,true);
 await page.goto('/documents/'+seed.id);await expect(page.locator('canvas')).toBeVisible({timeout:60_000});
 await command(page,'机构');await expect(page.getByRole('button',{name:'固定件',exact:true})).toBeEnabled();
 await command(page,'固定件');await pickVisibleInstance(page,seed.shaft);await page.getByRole('dialog',{name:'固定零件.1',exact:true}).getByRole('button',{name:/^确\s*定$/}).click();
 await command(page,'旋转接合');const joint=page.getByRole('dialog',{name:'旋转.1',exact:true});
 const filter=page.getByRole('textbox',{name:'筛选模型结构'}),tree=page.getByRole('complementary',{name:'模型结构',exact:true});
 for(const [label,index] of [['旋转圆柱面',0],['旋转圆柱面',1],['旋转定位面',0],['旋转定位面',1]] as const){
  await filter.fill(label);const supports=tree.getByText(label,{exact:true});await expect(supports).toHaveCount(2);await supports.nth(index).click();
 }
 await expect(joint.locator('.feature-pick-field').filter({hasText:'第一轴线'})).toContainText('圆柱面轴线');
 await expect(joint.locator('.feature-pick-field').filter({hasText:'第二轴线'})).toContainText('圆柱面轴线');
 await joint.getByRole('spinbutton',{name:'轴向偏移 mm',exact:true}).fill('9');await joint.getByRole('combobox',{name:'平面方向',exact:true}).click();await page.keyboard.press('ArrowDown');await page.keyboard.press('Enter');await joint.getByRole('checkbox',{name:'角度驱动',exact:true}).check();
 await expect(joint.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await page.screenshot({path:'../../../build/acceptance/dmu-revolute-dialog.png'});
 await joint.getByRole('button',{name:/^确\s*定$/}).click();await expect(joint).toHaveCount(0);
 let v=await current(page,seed.id);const m=v.product.kinematics.mechanisms[0],revolute=m.joints.find((j:any)=>j.kind==='REVOLUTE');
 expect(revolute.axialDirection).toBe('OPPOSITE');expect(revolute.constraints.find((c:any)=>c.kind==='DISTANCE').directionRelation).toBe('OPPOSITE');expect(revolute.constraints.find((c:any)=>c.kind==='COINCIDENT').directionRelation).toBe('OPPOSITE');expect(revolute.constraints).toHaveLength(2);for(const end of [revolute.first,revolute.second]){expect(end.axis.derivedRole).toBe('cylinder-axis');expect(end.axis.publicationRef).toBeTruthy();expect(end.plane.publicationRef).toBeTruthy();expect(end.axis.persistentSelection).toBeTruthy();}
 expect(v.product.constraints??[]).toHaveLength(0);
 await filter.fill('');
 const baseline=await current(page,seed.id);
 await command(page,'使用命令进行模拟');const trial=page.getByRole('dialog',{name:'使用命令进行模拟',exact:true});
 await expect(trial.getByRole('spinbutton',{name:'命令角度'})).toBeEnabled();
 await trial.getByRole('spinbutton',{name:'命令角度'}).fill('10');
 await expect.poll(async()=>Number(await trial.getByRole('spinbutton',{name:'命令角度'}).inputValue())).toBeCloseTo(10,2);
 expect((await current(page,seed.id)).document.versionId).toBe(baseline.document.versionId);
 await trial.getByRole('button',{name:/^取\s*消$/}).click();await expect(trial).toHaveCount(0);
 expect((await current(page,seed.id)).product.kinematics).toEqual(baseline.product.kinematics);
 await command(page,'使用命令进行模拟');await expect(trial.getByRole('spinbutton',{name:'命令角度'})).toBeEnabled();
 await command(page,'移动组件');await pickVisibleInstance(page,seed.rotor);
 const canvas=await viewportEngine(page);await expect.poll(()=>page.evaluate(()=>(window as any).__dmuEngine.moveManipulator.isAttached())).toBeTruthy();
 const arc=await page.evaluate(({width,height})=>{
  const e=(window as any).__dmuEngine,m=e.moveManipulator,handle=m.handles.find((h:any)=>h.operation==='rotate'&&h.axis==='Z');
  m.object.updateWorldMatrix(true,true);const origin=m.object.position.clone(),p=m.object.localToWorld(handle.pick.position.clone()),axis=origin.clone().set(0,0,1).applyQuaternion(m.object.quaternion);
  return Array.from({length:9},(_,i)=>{const point=p.clone().sub(origin).applyAxisAngle(axis,i*20*Math.PI/180/8).add(origin).project(e.camera);return {x:(point.x+1)*width/2,y:(1-point.y)*height/2}});
 },{width:canvas.width,height:canvas.height});
 await page.mouse.move(canvas.x+arc[0].x,canvas.y+arc[0].y);await page.mouse.down();for(const point of arc.slice(1))await page.mouse.move(canvas.x+point.x,canvas.y+point.y);await page.mouse.up();
 await expect.poll(()=>page.evaluate(()=>(window as any).__dmuEngine.moveInteraction.state),{timeout:60_000}).toBe('committed');
 await expect.poll(()=>page.evaluate(()=>(window as any).__dmuEngine.moveManipulator.isAttached())).toBeTruthy();
 await expect.poll(async()=>Math.abs(Number(await trial.getByRole('spinbutton',{name:'命令角度'}).inputValue()))).toBeGreaterThan(.1);
 expect((await current(page,seed.id)).document.versionId).toBe(baseline.document.versionId);
 await trial.getByRole('button',{name:/^取\s*消$/}).click();await expect(page.getByRole('dialog')).toHaveCount(0);
 await command(page,'模拟');const recorder=page.getByRole('dialog',{name:'模拟',exact:true});
 await expect(page.getByRole('dialog')).toHaveCount(2);await expect(trial.getByRole('spinbutton',{name:'命令角度'})).toBeEnabled();
 await trial.getByRole('spinbutton',{name:'命令角度'}).fill('20');
 await expect(recorder.locator('tbody tr[data-row-key]')).toHaveCount(2);
 await recorder.getByRole('checkbox',{name:'自动插入',exact:true}).uncheck();
 await recorder.getByRole('spinbutton',{name:'关键帧时间'}).fill('20');
 await trial.getByRole('spinbutton',{name:'命令角度'}).fill('60');await expect(recorder.getByRole('button',{name:'插入',exact:true})).toBeEnabled();await recorder.getByRole('button',{name:'插入',exact:true}).click();await expect(recorder.locator('tbody tr[data-row-key]')).toHaveCount(3);
 expect((await current(page,seed.id)).document.versionId).toBe(baseline.document.versionId);
 await page.evaluate(()=>{
  const engine=(window as any).__dmuEngine,render=engine.renderer.render.bind(engine.renderer);(window as any).__nativeMotionFrames=[];
  engine.renderer.render=(scene:any,camera:any)=>{if(scene===engine.scene&&engine.motionDisplay?.clock)(window as any).__nativeMotionFrames.push(Object.fromEntries([...engine.instanceGroups].map(([id,g]:any)=>[id,{translation:g.position.toArray(),rotation:g.quaternion.toArray()}])));return render(scene,camera)};
 });
 await recorder.getByRole('button',{name:'播放',exact:true}).click();
 await expect(recorder.getByRole('button',{name:'暂停',exact:true})).toBeVisible({timeout:60_000});
 await expect.poll(()=>page.evaluate(()=>((window as any).__nativeMotionFrames??[]).length)).toBeGreaterThan(2);
 await expect.poll(async()=>Number(await recorder.getByRole('spinbutton',{name:'模拟定位时间'}).inputValue()),{timeout:60_000}).toBeGreaterThan(.2);
 const continuous=await page.evaluate(()=>((window as any).__nativeMotionFrames??[]));expect(continuous.length).toBeGreaterThan(2);
 expect(new Set(continuous.map((f:any)=>JSON.stringify(f[seed.rotor].rotation.map((v:number)=>v.toFixed(5))))).size).toBeGreaterThan(2);
 await recorder.getByRole('button',{name:'暂停',exact:true}).click();await expect(recorder.getByRole('button',{name:/^取\s*消$/})).toBeEnabled();
 await recorder.getByRole('button',{name:/^取\s*消$/}).click();await expect(page.getByRole('dialog')).toHaveCount(0);
 expect((await current(page,seed.id)).document.versionId).toBe(baseline.document.versionId);
 await command(page,'模拟');await expect(trial.getByRole('spinbutton',{name:'命令角度'})).toBeEnabled();
 await trial.getByRole('spinbutton',{name:'命令角度'}).fill('20');await expect(recorder.locator('tbody tr[data-row-key]')).toHaveCount(2);
 await recorder.getByRole('checkbox',{name:'自动插入',exact:true}).uncheck();
 await recorder.getByRole('spinbutton',{name:'关键帧时间'}).fill('1.1');
 await trial.getByRole('spinbutton',{name:'命令角度'}).fill('40');await expect(recorder.getByRole('button',{name:'插入',exact:true})).toBeEnabled();await recorder.getByRole('button',{name:'插入',exact:true}).click();await expect(recorder.locator('tbody tr[data-row-key]')).toHaveCount(3);
 await page.screenshot({path:'../../../build/acceptance/dmu-recorded-simulation.png'});
 await recorder.getByRole('button',{name:/^确\s*定$/}).click();await expect(page.getByRole('dialog')).toHaveCount(0);
 const recorded=await current(page,seed.id);expect(recorded.product.kinematics.studies).toHaveLength(1);expect(recorded.product.kinematics.studies[0].keyframes).toHaveLength(3);
 expect(recorded.product.instances).toEqual(baseline.product.instances);expect(recorded.product.kinematics).not.toHaveProperty('laws');
 await command(page,'撤销');v=await current(page,seed.id);expect(v.product.kinematics.studies).toHaveLength(0);expect(v.product.kinematics.mechanisms[0].poses).toEqual(baseline.product.kinematics.mechanisms[0].poses);
 await command(page,'重做');v=await current(page,seed.id);expect(v.product.kinematics.studies[0].keyframes).toEqual(recorded.product.kinematics.studies[0].keyframes);
 await filter.fill('模拟.1');await tree.getByText('模拟.1',{exact:true}).dblclick();await expect(page.getByRole('dialog')).toHaveCount(2);
 await expect.poll(async()=>Number(await trial.getByRole('spinbutton',{name:'命令角度'}).inputValue())).toBeCloseTo(0,2);
 await recorder.getByRole('button',{name:/^取\s*消$/}).click();await expect(page.getByRole('dialog')).toHaveCount(0);expect((await current(page,seed.id)).document.versionId).toBe(v.document.versionId);
 await filter.fill('结果.');await expect(tree.getByText('结果.1 · 完成',{exact:true})).toBeVisible({timeout:120_000});
 await tree.getByText('结果.1 · 完成',{exact:true}).dblclick();const simulation=page.getByRole('dialog',{name:'仿真与回放',exact:true});await expect(simulation).toBeVisible();
 await expect(simulation.getByRole('button',{name:'删除',exact:true})).toHaveCount(0);
 await simulation.getByRole('spinbutton',{name:'定位时间 s'}).fill('0.37');await expect(simulation.getByRole('spinbutton',{name:'定位时间 s'})).toHaveValue('0.37');
 await command(page,'转换为产品约束');const conversion=page.getByRole('dialog',{name:'转换为产品约束',exact:true});expect((await conversion.boundingBox())!.width).toBeLessThanOrEqual(520);
 await conversion.getByRole('button',{name:'生成／刷新转换计划'}).click();await expect(conversion.getByText(/自由度 1/)).toBeVisible();await conversion.getByRole('button',{name:/^取\s*消$/}).click();
 await expect(simulation.getByRole('spinbutton',{name:'定位时间 s'})).toHaveValue('0.37');await page.keyboard.press('Escape');
 const beforeDelete=await current(page,seed.id);await tree.getByText('结果.1 · 完成',{exact:true}).click({button:'right'});await page.getByRole('menuitem',{name:'删除',exact:true}).click();await expect(tree.getByText('结果.1 · 完成',{exact:true})).toHaveCount(0);
 expect((await current(page,seed.id)).document.versionId).toBe(beforeDelete.document.versionId);await filter.fill('');
 await command(page,'旋转接合');await expect(page.getByRole('dialog',{name:'旋转.2',exact:true})).toBeVisible();await page.keyboard.press('Escape');expect(errors).toEqual([]);
});

test('viewport instance picks and mechanism manipulation preserve Product constraints and poses',async({page})=>{
 test.setTimeout(360_000);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();
 await page.goto('/');const seed=await seedProduct(page,false);await page.goto('/documents/'+seed.id);await expect(page.locator('canvas')).toBeVisible({timeout:60_000});
 await command(page,'多实例化');const pattern=page.getByRole('dialog',{name:'多实例化',exact:true});await pickVisibleInstance(page,seed.shaft);await expect(pattern.getByRole('button',{name:'源组件',exact:true})).toContainText('DMU轴');
 await pattern.getByRole('button',{name:/^确\s*定$/}).click();await expect(pattern).toHaveCount(0);let v=await current(page,seed.id);expect(v.product.instances).toHaveLength(3);
 await page.evaluate(async ({id,rotor})=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'FIX',fixMode:'SPACE',firstAssemblyRef:{kind:'BODY',instanceId:rotor}});
 },seed);await page.reload();await expect(page.locator('canvas')).toBeVisible();v=await current(page,seed.id);const original=v.product.instances.find((i:any)=>i.id===seed.rotor),constraints=JSON.stringify(v.product.constraints);
 await command(page,'机构');await expect(page.getByRole('button',{name:'移动组件',exact:true})).toBeEnabled();await command(page,'移动组件');await pickVisibleInstance(page,seed.rotor);
 await dragAttachedComponent(page);
 await expect.poll(async()=>{const currentView=await current(page,seed.id);const p=currentView.product.kinematics.mechanisms[0]?.poses?.[seed.rotor]?.translation;return p?Math.hypot(...p.map((v:number,i:number)=>v-original.translation[i])):0},{timeout:60_000}).toBeGreaterThan(1);
 v=await current(page,seed.id);const m=v.product.kinematics.mechanisms[0];expect(v.product.instances.find((i:any)=>i.id===seed.rotor).translation).toEqual(original.translation);expect(JSON.stringify(v.product.constraints)).toBe(constraints);expect(m.unitIds).toContain(seed.rotor);
 expect(m.evaluation).toBeTruthy();const moved=m.poses[seed.rotor].translation;await page.screenshot({path:'../../../build/acceptance/dmu-mechanism-move.png'});await command(page,'撤销');await expect.poll(async()=>((await current(page,seed.id)).product.kinematics.mechanisms[0].unitIds)).not.toContain(seed.rotor);await expect(page.getByRole('button',{name:'重做',exact:true})).toBeEnabled();await command(page,'重做');await expect.poll(async()=>((await current(page,seed.id)).product.kinematics.mechanisms[0].unitIds)).toContain(seed.rotor);v=await current(page,seed.id);for(let i=0;i<3;i++)expect(v.product.kinematics.mechanisms[0].poses[seed.rotor].translation[i]).toBeCloseTo(moved[i],6);
 await command(page,'移动组件');await pickVisibleInstance(page,seed.rotor);
 for(let round=0;round<2;round++){
  const before=await current(page,seed.id);await dragAttachedComponent(page);
  v=await current(page,seed.id);expect(v.document.versionId).not.toBe(before.document.versionId);
  expect(v.product.instances).toEqual(before.product.instances);expect(JSON.stringify(v.product.constraints)).toBe(constraints);
 }
 await command(page,'退出机构');await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();v=await current(page,seed.id);expect(v.product.instances.find((i:any)=>i.id===seed.rotor).translation).toEqual(original.translation);expect(v.product.kinematics.mechanisms).toHaveLength(1);expect(errors).toEqual([]);
});

test('recording playback preserves body world poses across play and exact seeks',async({page})=>{
 test.setTimeout(300_000);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();
 await page.goto('/');const seed=await seedProduct(page);
 await page.evaluate(async seed=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');const v=await api.getDocument(seed.id),k=structuredClone(v.product!.kinematics!);
  const joint=k.mechanisms[0].joints.find(j=>j.id===seed.jid)!;joint.coordinates![0].lower={value:-180,unit:'deg'};joint.coordinates![0].upper={value:180,unit:'deg'};
  k.drivers=[{id:crypto.randomUUID(),name:'命令.1',mechanismId:seed.mid,jointId:seed.jid,coordinateId:joint.coordinates![0].id}];
  await api.command(seed.id,{type:'SAVE_KINEMATICS',versionId:v.document.versionId,kinematics:k});
 },seed);
 await page.goto('/documents/'+seed.id+'?application=mechanism&mechanismId='+seed.mid);await expect(page.getByRole('button',{name:'模拟',exact:true})).toBeEnabled({timeout:60_000});
 const baseline=await current(page,seed.id);expect(baseline.product.instances.find((i:any)=>i.id===seed.rotor).translation).not.toEqual(baseline.product.kinematics.mechanisms[0].poses[seed.rotor].translation);
 await command(page,'模拟');const recorder=page.getByRole('dialog',{name:'模拟',exact:true}),trial=page.getByRole('dialog',{name:'使用命令进行模拟',exact:true});
 await expect(trial.getByRole('spinbutton',{name:'命令角度'})).toBeEnabled();await recorder.getByRole('checkbox',{name:'自动插入',exact:true}).uncheck();
 for(const [time,angle,count] of [[4,20,2],[8,45,3]]){
  await recorder.getByRole('spinbutton',{name:'关键帧时间'}).fill(String(time));await trial.getByRole('spinbutton',{name:'命令角度'}).fill(String(angle));
  await expect(recorder.getByRole('button',{name:'插入',exact:true})).toBeEnabled();await recorder.getByRole('button',{name:'插入',exact:true}).click();await expect(recorder.locator('tbody tr[data-row-key]')).toHaveCount(count);
 }
 await viewportEngine(page);
 const scene=()=>page.evaluate(()=>{
  const e=(window as any).__dmuEngine;e.scene.updateMatrixWorld(true);
  return [...e.solidBindings.values()].map((b:any)=>({id:b.context.instanceId,root:e.instanceGroups.get(b.context.instanceId).position.toArray(),position:b.mesh.getWorldPosition(b.mesh.position.clone()).toArray(),rotation:b.mesh.getWorldQuaternion(b.mesh.quaternion.clone()).toArray(),rootRotation:e.instanceGroups.get(b.context.instanceId).quaternion.toArray()}));
 });
 const qualified=(bodies:any[])=>{
  expect(bodies).toHaveLength(2);
  for(const b of bodies){for(let i=0;i<3;i++){expect(b.position[i]).toBeCloseTo(b.root[i],6);expect(b.position[i]).toBeCloseTo(baseline.product.kinematics.mechanisms[0].poses[b.id].translation[i],6)}expect(Math.abs(b.rotation.reduce((sum:number,v:number,i:number)=>sum+v*b.rootRotation[i],0))).toBeCloseTo(1,7)}
 };
 for(const name of ['起点','终点']){await recorder.getByRole('button',{name,exact:true}).click();await expect(recorder.getByRole('button',{name:'播放',exact:true})).toBeEnabled();qualified(await scene())}
 await recorder.getByRole('spinbutton',{name:'模拟定位时间'}).fill('2.37');await expect.poll(async()=>Number(await recorder.getByRole('spinbutton',{name:'模拟定位时间'}).inputValue())).toBeCloseTo(2.37,2);qualified(await scene());
 await recorder.getByRole('button',{name:'终点',exact:true}).click();await expect(recorder.getByRole('button',{name:'播放',exact:true})).toBeEnabled();
 await page.evaluate(()=>{
  const e=(window as any).__dmuEngine,render=e.renderer.render.bind(e.renderer);(window as any).__dmuBodyFrames=[];
  e.renderer.render=(scene:any,camera:any)=>{if(scene===e.scene&&e.motionDisplay?.clock&&(window as any).__dmuBodyFrames.length<60){e.scene.updateMatrixWorld(true);(window as any).__dmuBodyFrames.push([...e.solidBindings.values()].map((b:any)=>({id:b.context.instanceId,root:e.instanceGroups.get(b.context.instanceId).position.toArray(),position:b.mesh.getWorldPosition(b.mesh.position.clone()).toArray(),rotation:b.mesh.getWorldQuaternion(b.mesh.quaternion.clone()).toArray(),rootRotation:e.instanceGroups.get(b.context.instanceId).quaternion.toArray()})))}return render(scene,camera)};
 });
 await recorder.getByRole('button',{name:'播放',exact:true}).click();await expect(recorder.getByRole('button',{name:'暂停',exact:true})).toBeVisible({timeout:60_000});await expect.poll(()=>page.evaluate(()=>(window as any).__dmuBodyFrames.length)).toBeGreaterThan(3);
 const frames=await page.evaluate(()=>(window as any).__dmuBodyFrames);for(const frame of frames)qualified(frame);
 expect(new Set(frames.map((f:any)=>JSON.stringify(f.find((b:any)=>b.id===seed.rotor).rotation.map((v:number)=>v.toFixed(5))))).size).toBeGreaterThan(2);
 await recorder.getByRole('button',{name:'暂停',exact:true}).click();await expect(recorder.getByRole('button',{name:/^取\s*消$/})).toBeEnabled();qualified(await scene());
 await page.screenshot({path:'../../../build/acceptance/dmu-recording-playback-world-poses.png'});
 await recorder.getByRole('button',{name:/^取\s*消$/}).click();await expect(recorder).toHaveCount(0);expect((await current(page,seed.id)).document.versionId).toBe(baseline.document.versionId);qualified(await scene());expect(errors).toEqual([]);
});

test('joint limits own command creation and zero preserves recorded positions',async({page})=>{
 test.setTimeout(480_000);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();
 await page.goto('/');const seed=await seedProduct(page);
 await page.goto('/documents/'+seed.id+'?application=mechanism&mechanismId='+seed.mid);await expect(page.getByRole('button',{name:'移动组件',exact:true})).toBeEnabled({timeout:60_000});
 await expect(page.getByRole('button',{name:'角度命令',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'运动规律',exact:true})).toHaveCount(0);
 const filter=page.getByRole('textbox',{name:'筛选模型结构'}),tree=page.getByRole('complementary',{name:'模型结构',exact:true});
 const joint=page.getByRole('dialog',{name:'旋转.1',exact:true}),openJoint=async()=>{await filter.fill('旋转.1');await tree.getByText(/^旋转\.1 \(/).first().dblclick();await expect(joint).toBeVisible()};
 await openJoint();await expect(joint.getByRole('checkbox',{name:'角度驱动',exact:true})).not.toBeChecked();await expect(joint.getByRole('spinbutton',{name:'角度下限 °',exact:true})).toBeDisabled();
 await joint.getByRole('checkbox',{name:'角度驱动',exact:true}).check();await expect(joint.getByRole('checkbox',{name:'启用角度下限',exact:true})).toBeChecked();await expect(joint.getByRole('checkbox',{name:'启用角度上限',exact:true})).toBeDisabled();
 await joint.getByRole('spinbutton',{name:'角度下限 °',exact:true}).fill('-20');await joint.getByRole('spinbutton',{name:'角度上限 °',exact:true}).fill('50');
 await expect(joint.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await joint.getByRole('button',{name:/^确\s*定$/}).click();await expect(joint).toHaveCount(0);
 let view=await current(page,seed.id);const oldDriver=view.product.kinematics.drivers[0].id,stops=view.product.kinematics.mechanisms[0].joints.find((j:any)=>j.id===seed.jid);
 expect(view.product.kinematics.drivers).toHaveLength(1);expect(view.product.kinematics.drivers[0]).not.toHaveProperty('minimum');expect(view.product.kinematics).not.toHaveProperty('laws');
 await openJoint();await joint.getByRole('checkbox',{name:'角度驱动',exact:true}).uncheck();await expect(joint.getByRole('checkbox',{name:'启用角度下限',exact:true})).toBeEnabled();
 await joint.getByRole('button',{name:/^确\s*定$/}).click();await expect(joint).toHaveCount(0);view=await current(page,seed.id);expect(view.product.kinematics.drivers??[]).toHaveLength(0);
 const undriven=view.product.kinematics.mechanisms[0].joints.find((j:any)=>j.id===seed.jid);expect(undriven.coordinates[0].lower).toEqual(stops.coordinates[0].lower);expect(undriven.coordinates[0].upper).toEqual(stops.coordinates[0].upper);
 await openJoint();await joint.getByRole('checkbox',{name:'角度驱动',exact:true}).check();await joint.getByRole('button',{name:/^确\s*定$/}).click();await expect(joint).toHaveCount(0);view=await current(page,seed.id);expect(view.product.kinematics.drivers[0].id).not.toBe(oldDriver);
 await filter.fill('');await command(page,'模拟');const recorder=page.getByRole('dialog',{name:'模拟',exact:true}),trial=page.getByRole('dialog',{name:'使用命令进行模拟',exact:true});
 await expect(trial.getByRole('spinbutton',{name:'命令角度'})).toBeEnabled();await trial.getByRole('spinbutton',{name:'命令角度'}).fill('20');await expect(recorder.locator('tbody tr[data-row-key]')).toHaveCount(2);
 await trial.getByRole('spinbutton',{name:'命令角度'}).fill('10');await expect(recorder.locator('tbody tr[data-row-key]')).toHaveCount(3);await recorder.getByRole('button',{name:/^确\s*定$/}).click();await expect(recorder).toHaveCount(0);
 const saved=await current(page,seed.id),beforePoses=saved.product.kinematics.mechanisms[0].poses;
 await viewportEngine(page);const rendered=()=>page.evaluate(()=>Object.fromEntries([...(window as any).__dmuEngine.instanceGroups].map(([id,g]:any)=>[id,{translation:g.position.toArray(),rotation:g.quaternion.toArray()}])));
 const beforeRender=await rendered();await filter.fill('命令.1');await tree.getByText(/^命令\.1 \(/).first().dblclick();const editor=page.getByRole('dialog',{name:'命令.1',exact:true});
 await expect(editor.getByRole('spinbutton',{name:'命令当前值',exact:true})).toHaveValue('10.00');await expect(editor.getByRole('combobox')).toHaveCount(0);await editor.getByRole('button',{name:'置零',exact:true}).click();await expect(editor.getByRole('spinbutton',{name:'命令当前值',exact:true})).toHaveValue('0.00');
 expect(await rendered()).toEqual(beforeRender);await editor.getByRole('button',{name:/^确\s*定$/}).click();await expect(editor).toHaveCount(0);
 const zeroed=await current(page,seed.id);expect(zeroed.product.kinematics.studies).toEqual(saved.product.kinematics.studies);
 for(const [id,pose] of Object.entries(beforePoses) as any)for(let i=0;i<3;i++)expect(zeroed.product.kinematics.mechanisms[0].poses[id].translation[i]).toBeCloseTo(pose.translation[i],6);
 await filter.fill('模拟.1');await tree.getByText('模拟.1',{exact:true}).dblclick();await expect(trial.getByRole('spinbutton',{name:'命令角度'})).toBeEnabled();await expect.poll(async()=>Number(await trial.getByRole('spinbutton',{name:'命令角度'}).inputValue())).toBeCloseTo(-10,2);
 await expect(trial.getByText('-30.00 °',{exact:true})).toBeVisible();await expect(trial.getByText('40.00 °',{exact:true})).toBeVisible();
 await trial.getByRole('spinbutton',{name:'命令角度'}).fill('60');await expect(trial.getByText('该位置未通过求解。',{exact:true})).toBeVisible();await expect.poll(async()=>Number(await trial.getByRole('spinbutton',{name:'命令角度'}).inputValue())).toBeCloseTo(-10,2);
 expect((await current(page,seed.id)).document.versionId).toBe(zeroed.document.versionId);await recorder.getByRole('button',{name:/^取\s*消$/}).click();await expect(recorder).toHaveCount(0);
 await openJoint();await expect(joint.getByRole('checkbox',{name:'角度驱动',exact:true})).toBeDisabled();await expect(joint.getByText('命令被模拟引用',{exact:true})).toBeVisible();await expect(joint.getByRole('spinbutton',{name:'角度下限 °',exact:true})).toHaveValue('-30.00');await expect(joint.getByRole('spinbutton',{name:'角度上限 °',exact:true})).toHaveValue('40.00');
 await page.screenshot({path:'../../../build/acceptance/dmu-joint-owned-limits.png'});await joint.getByRole('button',{name:/^取\s*消$/}).click();expect(errors).toEqual([]);
});

test('cylindrical joint authors one angular and linear recording and pauses exactly for manipulation',async({page})=>{
 test.setTimeout(360_000);
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();
 await page.goto('/');const seed=await seedProduct(page,false,true);
 await page.goto('/documents/'+seed.id);await command(page,'机构');await command(page,'固定件');await pickVisibleInstance(page,seed.shaft);
 await page.getByRole('dialog',{name:'固定零件.1',exact:true}).getByRole('button',{name:/^确\s*定$/}).click();
 await command(page,'圆柱接合');const joint=page.getByRole('dialog',{name:'圆柱.1',exact:true}),filter=page.getByRole('textbox',{name:'筛选模型结构'}),tree=page.getByRole('complementary',{name:'模型结构',exact:true});
 await filter.fill('旋转圆柱面');const cylinders=tree.getByText('旋转圆柱面',{exact:true});await expect(cylinders).toHaveCount(2);
 await cylinders.nth(0).click();await cylinders.nth(1).click();
 await expect(joint.getByRole('button',{name:'选择第一定位平面',exact:true})).toHaveCount(0);
 await joint.getByRole('checkbox',{name:'角度驱动',exact:true}).check();await joint.getByRole('checkbox',{name:'长度驱动',exact:true}).check();
 await expect(joint.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await joint.getByRole('button',{name:/^确\s*定$/}).click();await expect(joint).toHaveCount(0);await filter.fill('');
 let v=await current(page,seed.id);const mechanism=v.product.kinematics.mechanisms[0],cylinder=mechanism.joints.find((j:any)=>j.kind==='CYLINDRICAL');
 expect(cylinder.constraints).toHaveLength(1);expect(cylinder.coordinates).toHaveLength(2);expect(v.product.kinematics.drivers).toHaveLength(2);expect(mechanism.evaluation.relativeDof).toBe(2);expect(v.product.constraints??[]).toHaveLength(0);
 await command(page,'模拟');const trial=page.getByRole('dialog',{name:'使用命令进行模拟',exact:true}),recorder=page.getByRole('dialog',{name:'模拟',exact:true});
 const angle=trial.getByRole('spinbutton',{name:'命令.1 角度',exact:true}),length=trial.getByRole('spinbutton',{name:'命令.2 长度',exact:true});
 await expect(angle).toBeEnabled();await expect(length).toBeEnabled();await expect(recorder.locator('tbody tr[data-row-key]')).toHaveCount(1);
 await recorder.getByRole('checkbox',{name:'自动插入',exact:true}).uncheck();
 for(const [time,angular,linear,count] of [[2,30,15,2],[20,60,5,3]]){
  await recorder.getByRole('spinbutton',{name:'关键帧时间'}).fill(String(time));await angle.fill(String(angular));await length.fill(String(linear));
  await expect(recorder.getByRole('button',{name:'插入',exact:true})).toBeEnabled();await recorder.getByRole('button',{name:'插入',exact:true}).click();await expect(recorder.locator('tbody tr[data-row-key]')).toHaveCount(count);
 }
 await recorder.getByRole('spinbutton',{name:'模拟定位时间'}).fill('1');await expect.poll(async()=>Number(await angle.inputValue())).toBeCloseTo(15,2);await expect.poll(async()=>Number(await length.inputValue())).toBeCloseTo(7.5,2);
 await viewportEngine(page);
 const world=()=>page.evaluate(id=>{const e=(window as any).__dmuEngine;e.scene.updateMatrixWorld(true);const root=e.instanceGroups.get(id),binding=[...e.solidBindings.values()].find((b:any)=>b.context.instanceId===id) as any;return {root:root.position.toArray(),body:binding.mesh.getWorldPosition(binding.mesh.position.clone()).toArray(),rotation:root.quaternion.toArray()}},seed.rotor);
 const expected=await page.evaluate(({mechanism,cylinder,rotor})=>{const pose=mechanism.poses[rotor],frame=cylinder.first.frame,q=frame.rotation,[x,y,z,w]=q;const direction=[2*(x*z+w*y),2*(y*z-w*x),1-2*(x*x+y*y)];return pose.translation.map((value:number,index:number)=>value+direction[index]*7.5*cylinder.coordinates.find((c:any)=>c.dimension==='LENGTH').direction)},{mechanism,cylinder,rotor:seed.rotor});
 let pose=await world();for(let i=0;i<3;i++){expect(pose.root[i]).toBeCloseTo(expected[i],5);expect(pose.body[i]).toBeCloseTo(pose.root[i],5)}
 await recorder.getByRole('button',{name:'播放',exact:true}).click();await expect(recorder.getByRole('button',{name:'暂停',exact:true})).toBeEnabled();
 await command(page,'移动组件');await expect(recorder.getByRole('button',{name:'播放',exact:true})).toBeEnabled();
 const at=Number(await recorder.getByRole('spinbutton',{name:'模拟定位时间'}).inputValue());expect(at).toBeGreaterThanOrEqual(1);expect(at).toBeLessThan(20);
 pose=await world();for(let i=0;i<3;i++)expect(pose.body[i]).toBeCloseTo(pose.root[i],5);
 await expect.poll(()=>page.evaluate(()=>(window as any).__dmuEngine.activeToolID)).toBe('assembly.move');
 await page.screenshot({path:'../../../build/acceptance/dmu-cylindrical-vector-recording.png'});
 await recorder.getByRole('button',{name:/^确\s*定$/}).click();await expect(recorder).toHaveCount(0);
 v=await current(page,seed.id);const study=v.product.kinematics.studies[0];expect(study.activeDriverIds).toHaveLength(2);expect(study.keyframes).toHaveLength(3);
 for(const key of study.keyframes)expect(Object.keys(key.values).sort()).toEqual([...study.activeDriverIds].sort());
 expect(errors).toEqual([]);
});

for(const definition of [
 {kind:'PRISMATIC',command:'棱形接合',title:'棱形.1',supports:['Z Axis','YZ Plane'],dof:1,count:2},
 {kind:'PLANAR',command:'平面接合',title:'平面.1',supports:['XY Plane'],dof:3,count:1},
 {kind:'SPHERICAL',command:'球面接合',title:'球面.1',supports:['Absolute Axis System'],dof:3,count:1},
 {kind:'RIGID',command:'刚性接合',title:'刚性.1',supports:[],dof:0,count:1},
])test(`${definition.kind} joint is created from actual supports and retains its physical freedom`,async({page})=>{
 test.setTimeout(240_000);
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();
 await page.goto('/');const seed=await seedProduct(page,false);
 await page.goto('/documents/'+seed.id);await command(page,'机构');await command(page,'固定件');await pickVisibleInstance(page,seed.shaft);
 await page.getByRole('dialog',{name:'固定零件.1',exact:true}).getByRole('button',{name:/^确\s*定$/}).click();
 await command(page,definition.command);const dialog=page.getByRole('dialog',{name:definition.title,exact:true}),filter=page.getByRole('textbox',{name:'筛选模型结构'}),tree=page.getByRole('complementary',{name:'模型结构',exact:true});
 if(definition.kind==='RIGID'){await pickVisibleInstance(page,seed.shaft);await pickVisibleInstance(page,seed.rotor)}
 else for(const label of definition.supports){await filter.fill(label);const supports=tree.getByText(label,{exact:true});await expect(supports).toHaveCount(2);await supports.nth(0).click();await supports.nth(1).click()}
 if(definition.kind==='PRISMATIC'){await dialog.getByRole('checkbox',{name:'长度驱动',exact:true}).check();await expect(dialog.getByRole('checkbox',{name:'角度驱动',exact:true})).toHaveCount(0)}
 if(definition.kind==='PLANAR'){await dialog.getByRole('spinbutton',{name:'平面偏移 mm',exact:true}).fill('3');await dialog.getByRole('combobox',{name:'平面方向',exact:true}).click();await page.keyboard.press('ArrowDown');await page.keyboard.press('Enter')}
 await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await page.screenshot({path:`../../../build/acceptance/dmu-${definition.kind.toLowerCase()}-joint.png`});
 await dialog.getByRole('button',{name:/^确\s*定$/}).click();await expect(dialog).toHaveCount(0);await filter.fill('');
 let view=await current(page,seed.id);const mechanism=view.product.kinematics.mechanisms[0],joint=mechanism.joints.find((j:any)=>j.kind===definition.kind);
 expect(joint.constraints).toHaveLength(definition.count);expect(mechanism.evaluation).toMatchObject({relativeDof:definition.dof,gaugeDof:0});expect(view.product.constraints??[]).toHaveLength(0);
 if(definition.kind==='RIGID'){
  expect(joint.constraints[0].groupMembers).toHaveLength(2);expect(mechanism.poses[seed.rotor].translation).toEqual([22,15,7]);
  const references:any[]=[];const visit=(node:any)=>{if(node.kind==='ASSEMBLY_GROUP_MEMBER_REFERENCE'&&node.mechanismId===mechanism.id)references.push(node);for(const child of node.children??[])visit(child)};visit(view.structureTree);expect(references).toHaveLength(2);
  await filter.fill(references[0].name);await tree.getByText(references[0].name,{exact:true}).dblclick();await expect(dialog).toBeVisible();await dialog.getByRole('button',{name:/^取\s*消$/}).click();await expect(dialog).toHaveCount(0);await filter.fill('');expect((await current(page,seed.id)).document.versionId).toBe(view.document.versionId);
 }
 if(definition.kind==='SPHERICAL'){expect(joint.first.point.derivedRole).toBe('frame-origin');expect(joint.second.point.derivedRole).toBe('frame-origin');expect(mechanism.poses[seed.rotor].translation.every((v:number)=>Math.abs(v)<1e-5)).toBeTruthy()}
 if(definition.kind==='PRISMATIC'){
  await command(page,'使用命令进行模拟');const trial=page.getByRole('dialog',{name:'使用命令进行模拟',exact:true}),input=trial.getByRole('spinbutton',{name:'命令长度',exact:true});await expect(input).toBeEnabled();await input.fill('10');await expect.poll(async()=>Number(await input.inputValue())).toBeCloseTo(10,2);await trial.getByRole('button',{name:/^确\s*定$/}).click();await expect(trial).toHaveCount(0);
  view=await current(page,seed.id);expect(view.product.kinematics.mechanisms[0].poses[seed.rotor].translation[2]).toBeCloseTo(mechanism.poses[seed.rotor].translation[2]+10,4);
 }
 await command(page,'退出机构');view=await current(page,seed.id);expect(view.product.constraints??[]).toHaveLength(0);expect(errors).toEqual([]);
});

test('Product Fix Together continuously picks viewport members and moves the complete rigid group',async({page})=>{
 test.setTimeout(300_000);
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();
 await page.goto('/');const seed=await seedProduct(page,false),initial=await current(page,seed.id);
 const third=await page.evaluate(async({id,part})=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');
  const inserted=await api.command(id,{type:'INSERT_INSTANCE',referencedDocumentId:part}),instance=inserted.product!.instances.at(-1)!.id;
  await api.command(id,{type:'MOVE_INSTANCE',instanceId:instance,translation:[0,-35,0],rotation:[0,0,0,1]});return instance;
 },{id:seed.id,part:initial.product.instances[0].documentId});await page.goto('/documents/'+seed.id);
 await command(page,'固联组');const dialog=page.getByRole('dialog').filter({has:page.getByLabel('固联组成员')});await expect(dialog).toBeVisible();
 await expect(dialog.getByRole('combobox')).toHaveCount(0);
 const header=await dialog.locator('.cad-command-dialog-header').boundingBox();if(!header)throw new Error('Missing group dialog header');
 await page.mouse.move(header.x+100,header.y+20);await page.mouse.down();await page.mouse.move(header.x+100,900,{steps:8});await page.mouse.up();
 await page.keyboard.press('Escape');await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:'选择成员集',exact:true})).toHaveAttribute('aria-pressed','false');await dialog.getByRole('button',{name:'选择成员集',exact:true}).click();
 await pickVisibleInstance(page,seed.shaft);await pickVisibleInstance(page,seed.rotor);await pickVisibleInstance(page,third);
 await expect(dialog.getByLabel('固联组成员').getByRole('button',{name:/^移除成员 /})).toHaveCount(3);await dialog.getByLabel('固联组成员').getByRole('button',{name:/^移除成员 /}).last().click();await expect(dialog.getByLabel('固联组成员').getByRole('button',{name:/^移除成员 /})).toHaveCount(2);await dialog.getByRole('button',{name:'选择成员集',exact:true}).click();await pickVisibleInstance(page,third);await expect(dialog.getByLabel('固联组成员').getByRole('button',{name:/^移除成员 /})).toHaveCount(3);await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await dialog.getByRole('button',{name:/^确\s*定$/}).click();await expect(dialog).toHaveCount(0);
 const before=await current(page,seed.id),group=before.product.constraints.find((c:any)=>c.kind==='FIX_TOGETHER');expect(group.name).toBe('固联组.1');expect(group.groupMembers).toHaveLength(3);expect(group.groupRelations).toHaveLength(3);
 const references:any[]=[];const visit=(node:any)=>{if(node.kind==='ASSEMBLY_GROUP_MEMBER_REFERENCE')references.push(node);for(const child of node.children??[])visit(child)};visit(before.structureTree);expect(references).toHaveLength(3);expect(references.every(node=>!node.capabilities.includes('DELETE'))).toBeTruthy();
 const filter=page.getByRole('textbox',{name:'筛选模型结构'}),tree=page.getByRole('complementary',{name:'模型结构',exact:true}),reference=references.find(node=>node.entityId===seed.rotor);
 await filter.fill(reference.name);await tree.getByText(reference.name,{exact:true}).dblclick();await expect(dialog).toBeVisible();await expect(dialog.getByLabel('固联组成员').getByRole('button',{name:/^移除成员 /})).toHaveCount(3);await dialog.getByRole('button',{name:/^取\s*消$/}).click();await expect(dialog).toHaveCount(0);await filter.fill('');expect((await current(page,seed.id)).document.versionId).toBe(before.document.versionId);
 await command(page,'移动组件');await pickVisibleInstance(page,seed.rotor);await dragAttachedComponent(page,4);
 const after=await current(page,seed.id);const baseline=before.product.instances.find((i:any)=>i.id===seed.shaft),moved=after.product.instances.find((i:any)=>i.id===seed.shaft);expect(moved.translation).not.toEqual(baseline.translation);
 expect(after.product.constraints[0].groupRelations).toEqual(group.groupRelations);await page.screenshot({path:'../../../build/acceptance/product-fix-together-viewport.png'});expect(errors).toEqual([]);
});

test('reported closed four-bar saves successive viewport drags and retains diagnostic references',async({page,context})=>{
 test.setTimeout(360_000);
 const sourceId=process.env.OCCCCAD_TEST_DMU_REPORTED_DOCUMENT;
 test.skip(!sourceId,'requires the retained reported mechanism');
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();
 await page.route('**/src/viewport/cad-viewport-engine.ts*',async route=>{
  const response=await route.fetch();await route.fulfill({response,body:await response.text()+`
const dmuInvalidate=CadViewportEngine.prototype.invalidate;
CadViewportEngine.prototype.invalidate=function(...args){window.__dmuEngine=this;return dmuInvalidate.apply(this,args)};
`});
 });
 await page.goto('/');
 const seed=await page.evaluate(async sourceId=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');const source=await api.getDocument(sourceId);
  let clone=await api.createDocument('PRODUCT','闭环拖动回归-'+crypto.randomUUID().slice(0,8));
  const ids=new Map<string,string>([[sourceId,clone.document.id]]);
  for(const instance of source.product!.instances){
   clone=await api.command(clone.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:instance.documentId,translation:instance.translation,rotation:instance.rotation});
   ids.set(instance.id,clone.product!.instances.at(-1)!.id);
   clone=await api.command(clone.document.id,{type:'RENAME_INSTANCE',instanceId:ids.get(instance.id),name:instance.name});
  }
  const k=source.product!.kinematics!;
  for(const item of [...k.mechanisms,...k.drivers,...k.studies])ids.set(item.id,crypto.randomUUID());
  for(const mechanism of k.mechanisms)for(const joint of mechanism.joints){
   ids.set(joint.id,crypto.randomUUID());for(const coordinate of joint.coordinates??[])ids.set(coordinate.id,crypto.randomUUID());for(const constraint of joint.constraints){
    const id=crypto.randomUUID();ids.set(constraint.id,id);
    const parameter=constraint.quantityParameter;if(parameter)ids.set(parameter.parameterId,parameter.parameterId.replace(constraint.id,id));
   }
  }
  const remap=(value:any):any=>{
   if(typeof value==='string')return ids.get(value)??value;
   if(Array.isArray(value))return value.map(remap);
   if(!value||typeof value!=='object')return value;
   const next:any=Object.fromEntries(Object.entries(value).map(([key,item])=>[ids.get(key)??key,remap(item)]));
   if(next.ownerDocumentId===clone.document.id&&'ownerVersionId' in next)next.ownerVersionId=clone.document.versionId;
   return next;
  };
  clone=await api.command(clone.document.id,{type:'SAVE_KINEMATICS',versionId:clone.document.versionId,kinematics:(()=>{const copied=remap(k);copied.studies=[];for(const driver of copied.drivers){delete driver.reference;delete driver.referenceDigest;const m=copied.mechanisms.find((m:any)=>m.id===driver.mechanismId),j=m.joints.find((j:any)=>j.id===driver.jointId);const c=j.coordinates.find((c:any)=>c.id===driver.coordinateId);c.lower??={value:-720,unit:'deg'};c.upper??={value:720,unit:'deg'};}return copied})()});
  const mechanismId=ids.get('ef90c89c-6306-4150-a36e-4d09f5dd698f')!;
  return {id:clone.document.id,mechanismId,crank:ids.get('instance-4e6bf784f7063a53be7a')!,sourceHead:source.document.versionId};
 },sourceId!);
 const url='/documents/'+seed.id+'?application=mechanism&mechanismId='+seed.mechanismId;
 await page.goto(url);await expect(page.getByRole('button',{name:'移动组件',exact:true})).toBeEnabled({timeout:60_000});
 const original=await current(page,seed.id);
 await command(page,'使用命令进行模拟');const trial=page.getByRole('dialog',{name:'使用命令进行模拟',exact:true});
 await trial.getByRole('combobox',{name:'运动命令'}).click();await page.getByRole('option',{name:'命令.3',exact:true}).click();
 await expect(trial.getByRole('spinbutton',{name:'命令角度'})).toBeEnabled();
 await expect.poll(async()=>Number(await trial.getByRole('spinbutton',{name:'命令角度'}).inputValue())).toBeCloseTo(0,2);
 const state=()=>page.evaluate(()=>Object.fromEntries([...(window as any).__dmuEngine.instanceGroups].map(([id,g]:any)=>[id,{translation:g.position.toArray(),rotation:g.quaternion.toArray()}])));
 let positions=await state();const mechanism=original.product.kinematics.mechanisms.find((m:any)=>m.id===seed.mechanismId);
 for(const id of mechanism.unitIds)for(let i=0;i<3;i++)expect(positions[id].translation[i]).toBeCloseTo(mechanism.poses[id].translation[i],5);
 await trial.getByRole('spinbutton',{name:'命令角度'}).fill('5');await expect(trial.getByRole('combobox',{name:'运动命令'})).toBeEnabled();await expect.poll(async()=>Number(await trial.getByRole('spinbutton',{name:'命令角度'}).inputValue()),{timeout:60_000}).toBeCloseTo(5,2);
 const lastGood=await state();
 await trial.getByRole('combobox',{name:'运动命令'}).click();await page.getByRole('option',{name:'命令.2',exact:true}).click();
 await expect(trial.getByRole('spinbutton',{name:'命令角度'})).toBeEnabled();positions=await state();for(const id of mechanism.unitIds)for(let i=0;i<3;i++)expect(positions[id].translation[i]).toBeCloseTo(lastGood[id].translation[i],5);
 await trial.getByRole('combobox',{name:'运动命令'}).click();await page.getByRole('option',{name:'命令.3',exact:true}).click();await expect.poll(async()=>Number(await trial.getByRole('spinbutton',{name:'命令角度'}).inputValue())).toBeCloseTo(5,2);
 await trial.getByRole('spinbutton',{name:'命令角度'}).fill('60');
 await expect(trial.getByText('该位置未通过求解。',{exact:true})).toBeVisible({timeout:60_000});
 await expect.poll(async()=>Number(await trial.getByRole('spinbutton',{name:'命令角度'}).inputValue())).toBeCloseTo(5,2);
 positions=await state();for(const id of mechanism.unitIds)for(let i=0;i<3;i++)expect(positions[id].translation[i]).toBeCloseTo(lastGood[id].translation[i],5);
 await trial.getByRole('spinbutton',{name:'命令角度'}).fill('-5');await expect(trial.getByRole('combobox',{name:'运动命令'})).toBeEnabled();await expect.poll(async()=>Number(await trial.getByRole('spinbutton',{name:'命令角度'}).inputValue()),{timeout:60_000}).toBeCloseTo(-5,2);
 await trial.getByRole('button',{name:/^取\s*消$/}).click();expect((await current(page,seed.id)).document.versionId).toBe(original.document.versionId);
 await command(page,'移动组件');await pickVisibleInstance(page,seed.crank);
 for(const distance of [-8,8,-8]){
  const before=await current(page,seed.id);await dragAttachedComponent(page,distance);
  const saved=await current(page,seed.id);expect(saved.document.versionId).not.toBe(before.document.versionId);expect(saved.product.instances).toEqual(before.product.instances);
  await page.waitForTimeout(800);
  const displayed=await page.evaluate(()=>Object.fromEntries([...(window as any).__dmuEngine.instanceGroups].map(([id,g]:any)=>[id,{translation:g.position.toArray(),rotation:g.quaternion.toArray()}])));
  const mechanism=saved.product.kinematics.mechanisms.find((m:any)=>m.id===seed.mechanismId);
  for(const id of mechanism.unitIds)for(let i=0;i<3;i++)expect(displayed[id].translation[i]).toBeCloseTo(mechanism.poses[id].translation[i],6);
  await expect(page.locator('.cad-operation-notification')).toHaveCount(0);
 }
 const accepted=await current(page,seed.id);await page.screenshot({path:'../../../build/acceptance/dmu-closed-fourbar-move.png'});
 await page.reload();await expect(page.getByRole('button',{name:'移动组件',exact:true})).toBeEnabled({timeout:60_000});
 const restored=await current(page,seed.id);expect(restored.product.kinematics.mechanisms).toEqual(accepted.product.kinematics.mechanisms);
 await page.evaluate(async id=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');
  try{await api.beginAssemblyInteraction(id,{baseRevisionId:'stale',instanceId:'missing',localGrabPoint:[0,0,0],frameRotation:[0,0,0,1]})}catch(error){(window as any).__dmuEngine.callbacks.operationFailed(error)}
 },seed.id);
 await page.getByRole('button',{name:'查看诊断',exact:true}).click();const diagnostic=page.getByRole('dialog',{name:'技术诊断',exact:true});await expect(diagnostic).toBeVisible();
 await context.grantPermissions(['clipboard-read','clipboard-write']);await diagnostic.getByRole('button',{name:'复制诊断',exact:true}).click();await expect(diagnostic.getByRole('button',{name:'已复制',exact:true})).toBeVisible();
 const reference=await page.evaluate(()=>navigator.clipboard.readText());expect(reference).toMatch(/^CAD_DIAGNOSTIC [^ ]+ VALIDATION_FAILED$/);
 const [,identity]=reference.split(' ');const response=await page.request.get('/api/documents/'+identity.split('/')[0]+'/diagnostics/'+identity.split('/')[1]);expect(response.ok()).toBeTruthy();
 expect((await response.json()).interactionBegin.instanceId).toBe('missing');
 expect((await current(page,sourceId!)).document.versionId).toBe(seed.sourceHead);expect(errors).toEqual([]);
});


test('fixed constraint create and edit close after authoritative commits',async({page})=>{
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();
 await page.goto('/');const seed=await seedProduct(page,false);await page.goto('/documents/'+seed.id);
 await expect(page.locator('canvas')).toBeVisible();
 await page.locator(`[role=treeitem][data-tree-key*="instance:${seed.shaft}"]`).first().click();
 await command(page,'固定');const dialog=page.getByRole('dialog',{name:'约束定义',exact:true});
 await expect(dialog).toBeVisible();
 await dialog.getByRole('spinbutton',{name:'X（mm）',exact:true}).fill('7.654');
 await dialog.getByRole('spinbutton',{name:'X（mm）',exact:true}).press('Tab');
 await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await dialog.getByRole('button',{name:/^确\s*定$/}).click();await expect(dialog).toHaveCount(0);
 const saved=await current(page,seed.id),fixed=saved.product.constraints.find((c:any)=>c.kind==='FIX');
 expect(fixed).toBeTruthy();expect(fixed.fixedPose.translation[0]).toBeCloseTo(7.654,8);
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill(fixed.name);
 const row=page.getByRole('treeitem').filter({hasText:fixed.name}).last();
 await row.dblclick();await expect(dialog).toBeVisible();
 await expect(dialog.getByRole('spinbutton',{name:'X（mm）',exact:true})).toHaveValue('7.65');
 await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await dialog.getByRole('spinbutton',{name:'X（mm）',exact:true}).press('Enter');
 await expect(dialog).toHaveCount(0);
 expect((await current(page,seed.id)).product.constraints.find((c:any)=>c.id===fixed.id).fixedPose.translation[0]).toBeCloseTo(7.654,8);
 await row.dblclick();await expect(dialog).toBeVisible();
 const beforeClear=(await current(page,seed.id)).document.versionId;
 await dialog.getByRole('button',{name:'选择支撑 1',exact:true}).click();await page.keyboard.press('Escape');await expect(dialog).toBeVisible();
 await dialog.getByRole('button',{name:'移除支撑 1',exact:true}).click();await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeDisabled();
 await dialog.getByRole('button',{name:/^取\s*消$/}).click();await expect(dialog).toHaveCount(0);expect((await current(page,seed.id)).document.versionId).toBe(beforeClear);
 await row.dblclick();await expect(dialog.getByRole('button',{name:'移除支撑 1',exact:true})).toBeVisible();
 await dialog.getByRole('spinbutton',{name:'X（mm）',exact:true}).fill('9.876');
 await dialog.getByRole('spinbutton',{name:'X（mm）',exact:true}).press('Tab');
 await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await dialog.getByRole('button',{name:/^确\s*定$/}).click();await expect(dialog).toHaveCount(0);
 const edited=await current(page,seed.id);expect(edited.product.constraints.find((c:any)=>c.id===fixed.id).fixedPose.translation[0]).toBeCloseTo(9.876,8);
 expect(edited.document.versionId).not.toBe(saved.document.versionId);expect(errors).toEqual([]);
});

test('saved mechanism analysis uses the command dialog, automatic physical evidence and viewport motion guides',async({page,browser},testInfo)=>{
 test.setTimeout(240_000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 await page.route('**/src/viewport/cad-viewport-engine.ts*',async route=>{const r=await route.fetch();await route.fulfill({response:r,body:await r.text()+`\nconst analysisShow=CadViewportEngine.prototype.showMechanismAnalysis;CadViewportEngine.prototype.showMechanismAnalysis=function(...args){window.__dmuEngine=this;return analysisShow.apply(this,args)};`})});
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();await page.goto('/');
 const seed=await seedProduct(page);await page.goto('/documents/'+seed.id);await expect(page.locator('canvas')).toBeVisible();
 let before=await current(page,seed.id);const history=await page.request.get(`/api/documents/${seed.id}/history`).then(r=>r.json());
 let requests=0;page.on('request',r=>{if(r.url().endsWith('/mechanism-analysis'))requests++});
 await command(page,'机构分析');const panel=page.getByRole('dialog',{name:'机构分析',exact:true});await expect(panel.getByRole('status',{name:'机构分析状态'})).toContainText('分析完成');await expect(panel).toContainText('相对自由度 1');expect(requests).toBe(1);
 await expect(panel.getByRole('button',{name:'导出 JSON'})).toHaveCount(0);await expect(panel).not.toContainText(before.document.versionId);
 await panel.getByRole('button',{name:'显示接合',exact:true}).click();await expect(panel).toContainText('视口位置与分析时不同');expect(await page.evaluate(()=>(window as any).__dmuEngine.analysisMechanism)).toBeUndefined();
 // Shared drag, position memory, Esc and clamping contract.
 const header=panel.locator('.cad-command-dialog-header'),box=await header.boundingBox();if(!box)throw new Error('header');await page.mouse.move(box.x+100,box.y+20);await page.mouse.down();await page.mouse.move(box.x+20,box.y+90,{steps:8});await page.mouse.up();const moved=await panel.boundingBox();
 await panel.getByRole('button',{name:'关闭',exact:true}).click();await command(page,'机构分析');await expect(panel).toBeVisible();await expect(panel).toContainText('相对自由度 1');expect(requests).toBe(1);const reopened=await panel.boundingBox();expect(reopened!.x).toBeCloseTo(moved!.x,0);expect(reopened!.y).toBeCloseTo(moved!.y,0);
 await page.setViewportSize({width:1024,height:768});await expect.poll(async()=>{const r=await panel.boundingBox();return !!r&&r.x>=0&&r.y>=0&&r.x+r.width<=1025&&r.y+r.height<=769}).toBeTruthy();await page.keyboard.press('Escape');await expect(panel).toHaveCount(0);await page.setViewportSize({width:1440,height:1000});
 expect((await current(page,seed.id)).product).toEqual(before.product);expect(await page.request.get(`/api/documents/${seed.id}/history`).then(r=>r.json())).toEqual(history);
 await command(page,'机构分析');
 const alternatives=await page.evaluate(async seed=>{const {restApi:api}=await import('/src/api.ts');let v=await api.command(seed.id,{type:'INSERT_INSTANCE',referencedDocumentId:seed.parts[0]});const other=v.product!.instances.at(-1)!.id;const k=structuredClone(v.product!.kinematics!);const end=(instanceId:string)=>({instanceId,frame:{translation:[0,0,0] as [number,number,number],rotation:[0,0,0,1] as [number,number,number,number]},axis:{instanceId,kind:'AXIS' as const,geometryId:'axis-system-default',axis:'Z'},plane:{instanceId,kind:'PLANE' as const,geometryId:'datum-xy'}});const mid=crypto.randomUUID(),jid=crypto.randomUUID(),rid=crypto.randomUUID();const coordinates=[{id:crypto.randomUUID(),role:'AXIAL_ROTATION' as const,dimension:'ANGLE' as const,direction:1 as const,zero:{value:0,unit:'deg' as const},lower:{value:-180,unit:'deg' as const},upper:{value:180,unit:'deg' as const}},{id:crypto.randomUUID(),role:'AXIAL_TRANSLATION' as const,dimension:'LENGTH' as const,direction:1 as const,zero:{value:0,unit:'mm' as const},lower:{value:-20,unit:'mm' as const},upper:{value:20,unit:'mm' as const}}];k.mechanisms.push({id:mid,name:'双命令圆柱',unitIds:[seed.shaft,other],joints:[{id:crypto.randomUUID(),name:'圆柱 Ground',kind:'GROUND',first:end(seed.shaft)},{id:jid,name:'圆柱双坐标',kind:'CYLINDRICAL',first:end(seed.shaft),second:end(other),coordinates}]},{id:rid,name:'未接地固联',unitIds:[seed.shaft,other],joints:[{id:crypto.randomUUID(),name:'重复零件刚性',kind:'RIGID',first:end(seed.shaft),second:end(other)}]});k.drivers??=[];for(const c of coordinates)k.drivers.push({id:crypto.randomUUID(),name:c.dimension==='ANGLE'?'旋转命令':'移动命令',mechanismId:mid,jointId:jid,coordinateId:c.id});v=await api.command(seed.id,{type:'SAVE_KINEMATICS',versionId:v.document.versionId,kinematics:k});return {mid,rid,other,revision:v.document.versionId}},seed);
 await expect.poll(async()=>((await current(page,seed.id)).document.versionId)).toBe(alternatives.revision);await expect(panel.getByRole('status',{name:'机构分析状态'})).toContainText('分析完成');
 const selectMechanism=async(name:string)=>{await panel.getByRole('combobox',{name:'分析机构'}).click();await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({hasText:name}).click();await expect(panel.getByRole('status',{name:'机构分析状态'})).toContainText('分析完成')};
 const unchanged=await current(page,seed.id);await selectMechanism('双命令圆柱');await expect(panel).toContainText('相对自由度 2');await expect(panel).toContainText('DMU轴.1');await expect(panel).toContainText('DMU轴.2');await panel.getByRole('button').filter({hasText:'DMU轴.2'}).click();await expect.poll(()=>page.evaluate(()=>((window as any).__dmuEngine.selected??[]).map((s:any)=>s.instanceId))).toContain(alternatives.other);await expect.poll(()=>page.evaluate(()=>(window as any).__dmuEngine.motionMarkers?.children.filter((c:any)=>c.name==='analysis-rotation-arc').length??0)).toBe(1);await expect.poll(()=>page.evaluate(()=>(window as any).__dmuEngine.motionMarkers?.children.filter((c:any)=>c.type==='ArrowHelper').length??0)).toBe(2);await panel.locator('tbody tr').filter({hasText:'圆柱双坐标'}).click();await expect(panel).toContainText('旋转命令');await expect(panel).toContainText('移动命令');await selectMechanism('未接地固联');await expect(panel).toContainText('相对自由度 0');await expect(panel).toContainText('整体还可移动');expect((await current(page,seed.id)).product).toEqual(unchanged.product);await selectMechanism('机械装置.1');
 let release:()=>void=()=>{},started:()=>void=()=>{};const held=new Promise<void>(r=>release=r),ready=new Promise<void>(r=>started=r);await page.route(`**/api/documents/${seed.id}/mechanism-analysis`,async route=>{const r=await route.fetch();started();await held;await route.fulfill({response:r})});
 await panel.getByRole('button',{name:'重新分析',exact:true}).click();await ready;await panel.getByRole('button',{name:'取消计算'}).click();release();await expect(panel).toContainText('已取消');await expect(panel).not.toContainText('相对自由度 1');await page.unroute(`**/api/documents/${seed.id}/mechanism-analysis`);
 await page.route(`**/api/documents/${seed.id}/mechanism-analysis`,route=>route.abort('failed'));await panel.getByRole('button',{name:'重新分析'}).click();await expect(panel).toContainText('分析未完成');await page.unroute(`**/api/documents/${seed.id}/mechanism-analysis`);await panel.getByRole('button',{name:/重\s*试/}).click();await expect(panel).toContainText('相对自由度 1');await panel.getByRole('button',{name:'关闭',exact:true}).click();
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill('机械装置.1');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText(/^机械装置\.1，/).first().click();await command(page,'机构');await page.getByRole('textbox',{name:'筛选模型结构'}).fill('');await expect(page.locator(`[data-application-id="${seed.mid}"]`)).toBeVisible();before=await current(page,seed.id);
 await command(page,'机构分析');await expect(panel).toContainText('相对自由度 1');await panel.getByRole('button').filter({hasText:'绕图示轴旋转'}).click();await expect.poll(()=>page.evaluate(()=>(window as any).__dmuEngine.motionMarkers?.children.filter((c:any)=>c.name==='analysis-rotation-arc').length??0)).toBe(1);const visualHeader=await panel.locator('.cad-command-dialog-header').boundingBox();if(!visualHeader)throw new Error('Missing analysis header');await page.mouse.move(visualHeader.x+90,visualHeader.y+20);await page.mouse.down();await page.mouse.move(visualHeader.x+90,195,{steps:8});await page.mouse.up();const visibleArc=await page.evaluate(()=>{const e=(window as any).__dmuEngine,g=e.motionMarkers.children.find((c:any)=>c.name==='analysis-rotation-arc');g.updateWorldMatrix(true,true);const bounds=e.renderer.domElement.getBoundingClientRect(),a=g.position.clone().set(0,0,0),points=Array.from({length:48},(_,i)=>{const p=a.clone().set(Math.cos(i*Math.PI/24),Math.sin(i*Math.PI/24),0).applyMatrix4(g.matrixWorld).project(e.camera);return [p.x*bounds.width/2,p.y*bounds.height/2]});return {visible:g.visible,diameter:Math.max(Math.max(...points.map(p=>p[0]))-Math.min(...points.map(p=>p[0])),Math.max(...points.map(p=>p[1]))-Math.min(...points.map(p=>p[1]))),anchor:g.position.toArray(),scale:g.scale.toArray()}});expect(visibleArc.visible).toBeTruthy();expect(visibleArc.diameter).toBeGreaterThan(50);await testInfo.attach('rotation-guide',{body:JSON.stringify(visibleArc),contentType:'application/json'});await page.screenshot({path:testInfo.outputPath('engineering-mechanism-analysis.png')});expect((await current(page,seed.id)).product).toEqual(before.product);expect((await current(page,seed.id)).document.versionId).toBe(before.document.versionId);await panel.getByRole('button',{name:'关闭',exact:true}).click();await command(page,'退出机构');
 const viewerSeed=await page.evaluate(async seed=>{const {restApi:api}=await import('/src/api.ts');const email=`mechanism-viewer-${crypto.randomUUID()}@occccad.local`,password='MechanismViewer-123!';const u=await api.adminCreateUser({email,displayName:'机构分析只读',password,platformRole:'MEMBER',status:'ACTIVE'});for(const id of [seed.id,...seed.parts])await api.share('documents',id,'USER',u.id,'VIEWER');return {email,password}},seed);
 const context=await browser.newContext({baseURL:'http://127.0.0.1:5175',viewport:{width:1440,height:1000}}),viewer=await context.newPage();expect((await viewer.request.post('/api/auth/login',{data:viewerSeed})).ok()).toBeTruthy();await viewer.goto('/documents/'+seed.id);await command(viewer,'机构分析');const vp=viewer.getByRole('dialog',{name:'机构分析',exact:true});await vp.getByRole('combobox',{name:'分析机构'}).click();await viewer.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({hasText:'机械装置.1'}).click();await expect(vp).toContainText('相对自由度 1');await expect(vp.getByRole('button',{name:'修复支撑'}).last()).toBeDisabled();await context.close();expect(errors).toEqual([]);
});

test('invalid mechanism supports remain inspectable and repair through real viewport picks without activation',async({page},testInfo)=>{
 test.setTimeout(240_000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();await page.goto('/');const seed=await seedProduct(page);
 const supportSeed=await page.evaluate(async s=>{
  const {restApi:api}=await import('/src/api.ts');let p=await api.getDocument(s.parts[0]);const sketch=p.part!.features.find(f=>f.type==='SKETCH')!.id,entityId=crypto.randomUUID();p=await api.command(p.document.id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_ENTITY',entity:{id:entityId,kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius:1}}]});
  let v=await api.command(s.id,{type:'UPDATE_REFERENCES'});const key=p.part!.bodies.find(b=>b.id===p.part!.activeBodyId)!.geometryKey!,faces=[];for(let i=1;i<=p.artifacts![key].topology.faces;i++)faces.push(await api.getTopologyProperties(p.document.id,key,'FACE',i,p.document.versionId));
  const cylinders=faces.filter(f=>f.geometryType==='CYLINDER');if(cylinders.length!==2)throw new Error('Expected hollow shaft '+JSON.stringify(faces));
  const inner=cylinders.sort((a,b)=>(a.bbox!.max[0]-a.bbox!.min[0])-(b.bbox!.max[0]-b.bbox!.min[0]))[0];
  const support=await api.inspectAssemblySupports(s.id,[{instanceId:s.shaft,kind:'FACE',geometryKey:key,topologyId:inner.localId,derivedRole:'cylinder-axis'}]);if(support.supports[0].status!=='RESOLVED')throw new Error(JSON.stringify(support));
  const k=structuredClone(v.product!.kinematics!);k.mechanisms[0].joints.find(j=>j.id===s.jid)!.first.axis=support.supports[0].reference;v=await api.command(s.id,{type:'SAVE_KINEMATICS',versionId:v.document.versionId,kinematics:k});
  p=await api.command(p.document.id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'DELETE_ENTITIES',entityIds:[entityId]}]});v=await api.command(s.id,{type:'UPDATE_REFERENCES'});return {sketch,entityId,version:v.document.versionId};
 },seed);
 await page.goto('/documents/'+seed.id);await expect(page.locator('canvas')).toBeVisible();const before=await current(page,seed.id);
 await command(page,'机构分析');const panel=page.getByRole('dialog',{name:'机构分析',exact:true});await expect(panel).toContainText('部分结果未知');const row=panel.locator('tbody tr').filter({hasText:'旋转.1'});await expect(row).toContainText('支撑需要修复');await panel.getByRole('button',{name:'重新分析'}).click();await expect(panel).toContainText('部分结果未知');await row.getByRole('button',{name:/^定位 /}).click();
 // Existing activation correctly rejects this definition; repair must bypass activation.
 const activation=await page.evaluate(async s=>{const {restApi:api}=await import('/src/api.ts');try{await api.prepareMechanismApplication(s.id,{baseRevisionId:(await api.getDocument(s.id)).document.versionId,mechanismId:s.mid});return 'accepted'}catch(e){return String(e)}},seed);expect(activation).not.toBe('accepted');
 await row.getByRole('button',{name:'修复支撑'}).click();const dialog=page.getByRole('dialog',{name:'旋转.1',exact:true});await expect(dialog).toBeVisible();await dialog.getByRole('button',{name:/取\s*消/}).click();expect((await current(page,seed.id)).document.versionId).toBe(before.document.versionId);
 await command(page,'机构分析');await expect(panel).toContainText('部分结果未知');await panel.locator('tbody tr').filter({hasText:'旋转.1'}).getByRole('button',{name:'修复支撑'}).click();await expect(dialog).toBeVisible();
 // Select the axis role in the original JointFields, then click an actual outer cylinder face.
 const role=dialog.getByRole('button',{name:'选择第一轴线',exact:true});
 await role.click();await viewportEngine(page);
 const box=await page.locator('canvas').boundingBox();if(!box)throw new Error('canvas');
 let point:{x:number;y:number}|undefined;await expect.poll(async()=>{point=await page.evaluate(async({id,documentId,box})=>{const e=(window as any).__dmuEngine,{restApi:api}=await import('/src/api.ts');const checked=new Set();for(let y=60;y<box.height-20;y+=6)for(let x=40;x<box.width-20;x+=6){if(document.elementFromPoint(box.x+x,box.y+y)!==document.querySelector('canvas'))continue;const h=e.hitTest(x,y);if(h?.instanceId===id&&h.kind==='face'&&!checked.has(h.topologyId)){checked.add(h.topologyId);const result=await api.inspectAssemblySupports(documentId,[{instanceId:id,kind:'FACE',geometryKey:h.geometryKey,topologyId:h.topologyId}]);if(result.supports[0]?.descriptor?.Kind==='CYLINDER')return {x,y}}}},{id:seed.shaft,documentId:seed.id,box});return Boolean(point)},{timeout:30_000}).toBeTruthy();if(!point)throw new Error('No shaft face pixel');await page.mouse.click(box.x+point.x,box.y+point.y);
 await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeEnabled({timeout:60_000});expect((await current(page,seed.id)).document.versionId).toBe(before.document.versionId);await page.screenshot({path:testInfo.outputPath('invalid-mechanism-support-preview.png')});await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 let restored=await current(page,seed.id);expect(restored.product.kinematics.mechanisms[0].id).toBe(seed.mid);expect(restored.product.kinematics.mechanisms[0].joints.find((j:any)=>j.id===seed.jid).first.axis.persistentSelection).toBeTruthy();expect(restored.product.instances).toEqual(before.product.instances);
 await command(page,'机构分析');await expect(panel).toContainText('分析完成');await panel.getByRole('button',{name:'关闭',exact:true}).click();await command(page,'撤销');await command(page,'机构分析');await expect(panel).toContainText('支撑需要修复');await panel.getByRole('button',{name:'关闭',exact:true}).click();await command(page,'重做');await page.reload();await command(page,'机构分析');await expect(panel).toContainText('分析完成');expect(errors).toEqual([]);expect(supportSeed.version).toBeTruthy();
});

test('mechanism exit verifies cylinder contact and reports failures without changing the workbench layout',async({page},testInfo)=>{
 test.setTimeout(180_000);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();
 await page.goto('/');const seed=await seedProduct(page);
 const version=await page.evaluate(async s=>{
  const {restApi:api}=await import('/src/api.ts');const refs=[];
  for(const [index,partId] of s.parts.entries()){
   const p=await api.getDocument(partId),body=p.part!.bodies.find(b=>b.geometryKey)!,key=body.geometryKey!;
   let cylinder;
   for(let i=1;i<=p.artifacts![key].topology.faces;i++){const face=await api.getTopologyProperties(partId,key,'FACE',i,p.document.versionId);if(face.geometryType==='CYLINDER'){cylinder=face;break}}
   if(!cylinder)throw new Error('Missing exact cylinder');
   const inspected=await api.inspectAssemblySupports(s.id,[{instanceId:index===0?s.shaft:s.rotor,kind:'FACE',geometryKey:key,topologyId:cylinder.localId}]);
   if(inspected.supports[0].status!=='RESOLVED')throw new Error('Cylinder support not resolved');refs.push(inspected.supports[0].reference);
  }
  const v=await api.command(s.id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'CONTACT',contactKind:'LINE',contactSide:'INTERNAL',firstAssemblyRef:refs[0],secondAssemblyRef:refs[1]});
  if(v.product!.constraints[0].evaluationStatus!=='VERIFIED')throw new Error('Contact not accepted');return v.document.versionId;
 },seed);
 await page.goto(`/documents/${seed.id}?application=mechanism&mechanismId=${seed.mid}&applicationRevision=${version}&mechanismSources=${seed.mid}`);
 await expect(page.getByRole('button',{name:'退出机构',exact:true})).toBeEnabled();await expect(page.locator('canvas')).toBeVisible();
 const before=await current(page,seed.id),history=await page.request.get(`/api/documents/${seed.id}/history`).then(r=>r.json());
 const stage=page.getByRole('region',{name:'三维视口',exact:true}),bounds=await stage.boundingBox();
 // Fault injection at the existing transport boundary; successful retry below
 // uses the real Router, native solver and independent final-pose witness.
 const endpoint=`**/api/documents/${seed.id}/mechanism-application`;
 await page.route(endpoint,async route=>{if(!route.request().postDataJSON().mechanismId)await route.fulfill({status:400,json:{code:'EXECUTION_FAILURE',error:'injected mechanism exit failure',retryable:true}});else await route.continue()});
 await command(page,'退出机构');const notice=page.locator('.ant-notification-bottomRight .cad-operation-notification');await expect(notice).toContainText('机构操作未完成');
 await expect(page.locator('.cad-workbench > .ant-alert')).toHaveCount(0);
 expect(await stage.boundingBox()).toEqual(bounds);expect((await current(page,seed.id)).document.versionId).toBe(before.document.versionId);expect(await page.request.get(`/api/documents/${seed.id}/history`).then(r=>r.json())).toEqual(history);
 await page.screenshot({path:testInfo.outputPath('mechanism-exit-failure-notification.png')});await page.unroute(endpoint);
 await command(page,'退出机构');await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();expect(new URL(page.url()).searchParams.has('application')).toBe(false);
 const after=await current(page,seed.id);expect(after.product.kinematics).toEqual(before.product.kinematics);expect(after.product.constraints.map((c:any)=>[c.id,c.evaluationStatus])).toEqual(before.product.constraints.map((c:any)=>[c.id,c.evaluationStatus]));
 const [a,b]=after.product.instances,q=a.rotation,axis=[2*(q[0]*q[2]+q[3]*q[1]),2*(q[1]*q[2]-q[3]*q[0]),1-2*(q[0]*q[0]+q[1]*q[1])],delta=b.translation.map((v:number,i:number)=>v-a.translation[i]),axial=delta.reduce((sum:number,v:number,i:number)=>sum+v*axis[i],0);
 expect(Math.hypot(...delta.map((v:number,i:number)=>v-axial*axis[i]))).toBeCloseTo(8,5);
 expect(after.document.versionId).not.toBe(before.document.versionId);
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.instances).toEqual(before.product.instances);
 await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.instances).toEqual(after.product.instances);
 await page.reload();await expect(page.locator('canvas')).toBeVisible();expect((await current(page,seed.id)).product.instances).toEqual(after.product.instances);await expect(page.locator('.cad-workbench > .ant-alert')).toHaveCount(0);expect(errors).toEqual([]);
});
