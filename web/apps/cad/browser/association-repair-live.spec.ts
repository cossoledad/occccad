import {expect,test,type Page} from '@playwright/test';
import {worldScreen} from './camera-helpers';

async function current(page:Page,id:string){return (await page.request.get(`/api/documents/${id}`)).json();}
async function openRepair(page:Page){
 await page.getByRole('button',{name:'搜索工具',exact:true}).click();
 await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill('关联更新与修复');
 await page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:/^关联更新与修复$/})}).click();
 return page.getByRole('dialog',{name:'关联更新与修复',exact:true});
}

test('direct Part association update and viewport Reconnect preserve occurrences and accepted snapshots',async({page,context})=>{
 test.setTimeout(240_000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 expect(process.env.OCCCCAD_ADMIN_PASSWORD).toBeTruthy();
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();await page.goto('/');
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');
  let part=await api.createDocument('PART','关联修复验收零件');const pid=part.document.id;
  part=await api.command(pid,{type:'CREATE_SKETCH',plane:'XY'});const sketch=part.part!.features.at(-1)!.id;
  part=await api.command(pid,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:-10,y:-10},second:{x:10,y:10}}]});
  part=await api.command(pid,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length:10});const base=part.part!.features.at(-1)!;
  const key=part.part!.bodies.find(b=>b.id===base.bodyId)!.geometryKey!,artifact=part.artifacts[key];let top=0;
  for(let i=1;i<=Number(artifact.topology.faces);i++){const p=await api.getTopologyProperties(pid,key,'FACE',i,part.document.versionId);if(p.persistentSelection?.anchor.featureId===base.id&&p.persistentSelection.anchor.outputSlot.startsWith('END_CAP/')){top=i;break;}}
  if(!top)throw Error('Missing authoritative top cap');
  let product=await api.createDocument('PRODUCT','关联更新与修复验收');const id=product.document.id;
  for(let i=0;i<3;i++)product=await api.command(id,{type:'INSERT_INSTANCE',referencedDocumentId:pid});const ids=product.product!.instances.map(i=>i.id);
  for(let i=0;i<3;i++)product=await api.command(id,{type:'MOVE_INSTANCE',instanceId:ids[i],translation:[i*40,0,0],rotation:[0,0,0,1]});
  product=await api.command(id,{type:'SET_REFERENCE_MODE',instanceId:ids[2],referenceMode:'PINNED'});
  for(const instanceId of [ids[0],ids[2]])product=await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'FIX',fixMode:'SPACE',firstAssemblyRef:{instanceId,kind:'BODY'}});
  product=await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'COINCIDENT',directionRelation:'UNORIENTED',firstAssemblyRef:{instanceId:ids[0],kind:'FACE',geometryKey:key,topologyId:top},secondAssemblyRef:{instanceId:ids[1],kind:'FACE',geometryKey:key,topologyId:top}});
  return {id,pid,ids,sketch,base,edge:part.part!.features[0].sketch!.entities[0].id,pinned:part.document.versionId,mate:product.product!.constraints!.at(-1)!.id,fixes:product.product!.constraints!.slice(0,2).map(c=>c.id),version:product.document.versionId,poses:product.product!.instances.map(i=>({id:i.id,translation:i.translation,rotation:i.rotation}))};
 });
 const peer=await context.newPage();await peer.goto('/');
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();
 let drawer=await openRepair(page);await expect(drawer).toBeVisible();
 await expect(drawer.getByRole('button',{name:'关闭',exact:true})).toBeVisible();
 await drawer.getByRole('button',{name:'关闭',exact:true}).click();await expect(drawer).not.toBeVisible();drawer=await openRepair(page);
 await expect(drawer.locator('[data-reference-instance]')).toHaveCount(3);
 const row=(id:string)=>drawer.locator(`[data-reference-instance="${id}"]`);
 await expect(row(seed.ids[2])).toContainText('固定版本，不参与跟随更新');
 await row(seed.ids[1]).getByRole('button',{name:'定位实例',exact:true}).click();await expect(page.getByTestId('viewport-selection')).toContainText(seed.ids[1]);
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 const changed=await peer.evaluate(async seed=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');const p=await api.getDocument(seed.pid);
  const feature=p.part!.features.find(f=>f.id===seed.base.id)!;
  const find=(n:any):any=>n.entityId===feature.id?n:(n.children??[]).map(find).find(Boolean);
  return api.command(seed.pid,{type:'EDIT_FEATURE',targetId:feature.id,expectedFeatureDigest:find(p.structureTree)?.definitionDigest,feature:{...feature,length:14}});
 },seed);
 const verify=async(version:string,status:string)=>{
  const v=await current(page,seed.id);expect(v.product.instances.map((i:any)=>i.versionId)).toEqual([version,version,seed.pinned]);
  expect(v.product.instances.map((i:any)=>({id:i.id,translation:i.translation,rotation:i.rotation}))).toEqual(seed.poses);
  expect(v.product.constraints.find((c:any)=>c.id===seed.mate).evaluationStatus).toBe(status);
  for(const id of seed.fixes)expect(v.product.constraints.find((c:any)=>c.id===id).evaluationStatus).toBe('VERIFIED');
  return v;
 };
 await expect(async()=>{await verify(changed.document.versionId,'VERIFIED');}).toPass({timeout:30_000});
 await expect(row(seed.ids[0])).toContainText(changed.document.versionId);
 await expect(drawer.getByRole('button',{name:'重新计算',exact:true})).toBeEnabled();await drawer.getByRole('button',{name:'重新计算',exact:true}).click();
 await expect(drawer.getByRole('button',{name:'重新计算',exact:true})).toBeEnabled();await verify(changed.document.versionId,'VERIFIED');
 await expect(page.getByRole('dialog',{name:'约束定义',exact:true})).not.toBeVisible();
 const cut=await peer.evaluate(async seed=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');let v=await api.command(seed.pid,{type:'CREATE_DATUM_PLANE',origin:[0,0,7],normal:[0,0,1],uDirection:[1,0,0]});
  v=await api.command(seed.pid,{type:'CREATE_SKETCH',datumPlaneId:v.part!.datumPlanes.at(-1)!.id});const sketchId=v.part!.features.at(-1)!.id;
  v=await api.command(seed.pid,{type:'EDIT_SKETCH',sketchId,operations:[{type:'ADD_RECTANGLE',first:{x:-10,y:-10},second:{x:10,y:10}}]});
  return api.command(seed.pid,{type:'CREATE_SOLID_FEATURE',sketchId,generator:'LINEAR_EXTRUDE',operation:'REMOVE',length:7});
 },seed);
 await expect(async()=>{await verify(cut.document.versionId,'BROKEN');}).toPass({timeout:30_000});
 const brokenRow=()=>drawer.locator(`[data-repair-constraint="${seed.mate}"]`);
 await expect(brokenRow()).toContainText('NotConnected');await expect(brokenRow().getByRole('button',{name:'Reconnect',exact:true})).toBeEnabled();
 await brokenRow().getByRole('button',{name:'定位约束',exact:true}).click();await expect(page.getByTestId('viewport-selection')).toContainText(seed.mate);
 await page.screenshot({path:'../../../build/acceptance/association-repair-broken.png'});
 const brokenHead=(await current(page,seed.id)).document.versionId;
 const pickRepair=async()=>{
  await brokenRow().getByRole('button',{name:'Reconnect',exact:true}).click();
  const dialog=page.getByRole('dialog',{name:'约束定义',exact:true});await expect(dialog).toBeVisible();
  // Actual ray picking in the production viewport, not injected persistent refs.
  const header=await dialog.locator('header').boundingBox();if(!header)throw Error('Missing draggable header');
  await page.mouse.move(header.x+100,header.y+15);await page.mouse.down();await page.mouse.move(header.x+420,header.y+15,{steps:6});await page.mouse.up();
  const a=await worldScreen(page,[3,3,7]);await page.mouse.click(a.x,a.y);
  await expect(dialog.getByText('Reconnect 支持元素 1',{exact:true})).not.toBeVisible();
  await dialog.locator('.assembly-support-row').nth(1).getByRole('button',{name:'Reconnect',exact:true}).click();
  const b=await worldScreen(page,[36,-4,7]);expect(await page.evaluate(p=>document.elementFromPoint(p.x,p.y)?.tagName,b)).toBe('CANVAS');await page.mouse.click(b.x,b.y);
  await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeEnabled();
  await expect(dialog.locator('.assembly-constraint-status')).toContainText('Verified');return dialog;
 };
 let dialog=await pickRepair();await dialog.getByRole('button',{name:/取\s*消/}).click();expect((await current(page,seed.id)).document.versionId).toBe(brokenHead);
 drawer=await openRepair(page);await expect(brokenRow().getByRole('button',{name:'Reconnect',exact:true})).toBeEnabled();
 dialog=await pickRepair();await page.screenshot({path:'../../../build/acceptance/association-repair-preview.png'});await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).not.toBeVisible();
 await expect(async()=>{await verify(cut.document.versionId,'VERIFIED');}).toPass();
 const repaired=(await current(page,seed.id)).product.constraints.find((c:any)=>c.id===seed.mate);
 expect(repaired.first).toMatchObject({instanceId:seed.ids[0],kind:'FACE',resolution:{targetVersionId:cut.document.versionId,result:{supportingElementStatus:'CONNECTED'}}});
 expect(repaired.second).toMatchObject({instanceId:seed.ids[1],kind:'FACE',resolution:{targetVersionId:cut.document.versionId,result:{supportingElementStatus:'CONNECTED'}}});
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect(async()=>{await verify(cut.document.versionId,'BROKEN');}).toPass();
 await page.getByRole('button',{name:'重做',exact:true}).click();await expect(async()=>{await verify(cut.document.versionId,'VERIFIED');}).toPass();
 const accepted=(await current(page,seed.id)).document.versionId;
 await peer.evaluate(async seed=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');await api.command(seed.pid,{type:'EDIT_SKETCH',sketchId:seed.sketch,operations:[{type:'DELETE_ENTITIES',entityIds:[seed.edge]}]});
 },seed);
 drawer=await openRepair(page);await expect(drawer).toContainText('REFERENCE_PART_NOT_READY');
 expect((await current(page,seed.id)).document.versionId).toBe(accepted);await verify(cut.document.versionId,'VERIFIED');
 await drawer.getByRole('button',{name:'重试检查',exact:true}).click();await expect(drawer).toContainText('REFERENCE_PART_NOT_READY');expect((await current(page,seed.id)).document.versionId).toBe(accepted);
 const recovered=await peer.evaluate(async pid=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');return api.command(pid,{type:'UNDO'});
 },seed.pid);
 await expect(async()=>{await verify(recovered.document.versionId,'VERIFIED');}).toPass({timeout:30_000});
 await page.reload();drawer=await openRepair(page);await expect(drawer.locator('[data-reference-instance]')).toHaveCount(3);await expect(brokenRow()).toContainText('已满足');await expect(row(seed.ids[2])).toContainText(seed.pinned);
 await verify(recovered.document.versionId,'VERIFIED');expect(errors).toEqual([]);await peer.close();
});
