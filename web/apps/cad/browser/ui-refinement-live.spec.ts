import {searchTool as tool} from './command-helpers';
import {expect,test,type Page} from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
import {observeViewport} from './camera-helpers';
import {treeNavigator} from './tree-helpers';

const output=path.resolve('../../../build/agent-work/ui-optimization');
async function login(page:Page){expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBe(true);await page.goto('/')}

test('library navigation, branch folder loading, scoped tree actions and hover storage',async({page})=>{
 test.setTimeout(300_000);fs.mkdirSync(output,{recursive:true});const errors:string[]=[],folderReads:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore Vite application module
  const {restApi:api}=await import('/src/api.ts');const tag='UI refinement '+Date.now();
  const root=await api.createFolder(tag,'');const child=await api.createFolder(tag+' child','',root.id);const leaf=await api.createFolder(tag+' leaf','',child.id);
  let part=await api.createDocument('PART',tag);const id=part.document.id;
  part=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const sketch=part.part!.features[0].id;
  part=await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:-20,y:-15},second:{x:20,y:15}}]});
  part=await api.command(id,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length:20});
  return {id,name:tag,root,child,leaf,sketch,body:part.part!.bodies.find(body=>body.geometryKey)!.id};
 });
 let copyID:string|undefined;
 try{
  await page.reload();const card=page.locator('.document-card').filter({has:page.locator('.document-name',{hasText:seed.name})});await expect(card).toBeVisible();
  await card.getByRole('checkbox').check();await expect(card).toHaveClass(/selected/);expect(new URL(page.url()).pathname).toBe('/');await card.getByRole('checkbox').uncheck();
  page.on('request',request=>{const url=new URL(request.url());if(url.pathname==='/api/folders')folderReads.push(url.search)});
  await card.getByRole('button',{name:'文档操作 '+seed.name,exact:true}).click();await page.getByRole('menuitem',{name:/复制/}).click();
  const copy=page.getByRole('dialog',{name:'复制文档',exact:true});await expect(copy).toBeVisible();await expect(copy.locator('.folder-picker-list button').first()).toBeVisible();expect(folderReads.length).toBe(1);
  await copy.locator('.folder-picker-list button').filter({hasText:seed.root.name}).click();await expect(copy.locator('.folder-picker-list button').filter({hasText:seed.child.name})).toBeVisible();expect(folderReads.length).toBe(2);
  await copy.locator('.folder-picker-list button').filter({hasText:seed.child.name}).click();await expect(copy.locator('.folder-picker-list button').filter({hasText:seed.leaf.name})).toBeVisible();expect(folderReads.length).toBe(3);
  await copy.getByLabel('副本名称').fill(seed.name+' copy');await page.screenshot({path:path.join(output,'folder-picker.png')});await copy.getByRole('button',{name:/^复\s*制$/}).click();await expect(copy).not.toBeVisible();
  const copied=await page.evaluate(async(name)=>{const {restApi:api}=await import('/src/api.ts');return (await api.listDocuments({allFolders:true,query:name})).documents.find(document=>document.name===name)},seed.name+' copy');expect(copied?.folderId).toBe(seed.child.id);copyID=copied!.id;
  fs.writeFileSync(path.join(output,'folder-loading.json'),JSON.stringify({readsOnOpen:1,readsAfterTwoDescents:3,observedRequests:folderReads},null,2));
  await card.getByRole('button',{name:'文档操作 '+seed.name,exact:true}).click();await page.getByRole('menuitem',{name:/移动/}).click();
  const move=page.getByRole('dialog',{name:'移动文档',exact:true});await move.locator('.folder-picker-list button').filter({hasText:seed.root.name}).click();await move.getByRole('button',{name:/^移\s*动$/}).click();await expect(move).not.toBeVisible();
  expect((await(await page.request.get('/api/documents/'+seed.id)).json()).document.folderId).toBe(seed.root.id);
  await page.evaluate(async id=>{const {restApi:api}=await import('/src/api.ts');await api.moveDocument(id)},seed.id);await page.reload();await expect(card).toBeVisible();
  await page.getByRole('button',{name:'文件夹操作 '+seed.root.name,exact:true}).click();await page.getByRole('menuitem',{name:/编辑/}).click();const folderEditor=page.getByRole('dialog',{name:'编辑文件夹',exact:true});
  const renamed=seed.root.name+' renamed';await folderEditor.getByLabel('文件夹名称').fill(renamed);await folderEditor.getByRole('button',{name:/^保\s*存$/}).click();await expect(folderEditor).not.toBeVisible();
  await card.getByRole('button',{name:'文档操作 '+seed.name,exact:true}).click();await page.getByRole('menuitem',{name:/移动/}).click();const afterRename=page.getByRole('dialog',{name:'移动文档',exact:true});await afterRename.locator('.folder-picker-list button').filter({hasText:renamed}).click();await expect(afterRename.locator('.folder-picker-target')).toContainText(renamed);await afterRename.getByRole('button',{name:/^取\s*消$/}).click();


  await page.locator('[aria-label="显示方式"]').getByTitle('列表',{exact:true}).click();await expect(card).toHaveClass(/compact/);await page.screenshot({path:path.join(output,'library-list.png')});
  await card.getByRole('button',{name:'打开 '+seed.name,exact:true}).click();await expect(page).toHaveURL(new RegExp('/documents/'+seed.id));await observeViewport(page);
  await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.solidBindings.size)).toBe(1);const canvas=await page.locator('canvas').first().boundingBox();
  await expect(page.getByRole('button',{name:'搜索结构树',exact:true})).toHaveCount(0);await expect(page.getByRole('textbox',{name:'搜索结构树',exact:true})).toHaveCount(0);
  await treeNavigator(page)('Body.1',`/body:${seed.body}`);
  const body=page.locator(`[role=treeitem][data-tree-key$="/body:${seed.body}"]`);await expect(body).toBeVisible();await body.click();
  await body.getByRole('button',{name:/隐藏/}).click();await expect.poll(async()=>{const v=await(await page.request.get('/api/documents/'+seed.id)).json();return v.part.bodies.find((b:any)=>b.id===seed.body).visible}).toBe(false);
  await body.getByRole('button',{name:/显示/}).click();await expect.poll(async()=>{const v=await(await page.request.get('/api/documents/'+seed.id)).json();return v.part.bodies.find((b:any)=>b.id===seed.body).visible}).toBe(true);
  expect(await page.locator('canvas').first().boundingBox()).toEqual(canvas);
  await body.click({button:'right'});await expect(page.getByRole('menu')).toBeVisible();await page.keyboard.press('Escape');await expect(page.getByRole('menu')).not.toBeVisible();
  await page.evaluate(async()=>{const {useWorkbenchStore:store}=await import('/src/state/workbench-store.ts');const stats={pointerMoves:0,preselectionChanges:0,storageWrites:0};(window as any).hoverProbe=stats;store.getState().setSelections([]);store.subscribe((state,old)=>{if(state.preselection!==old.preselection)stats.preselectionChanges++});const set=Storage.prototype.setItem;Storage.prototype.setItem=function(key,value){if(key==='occccad.workbench')stats.storageWrites++;return set.call(this,key,value)}});
  for(let y=0;y<9;y++)for(let x=0;x<9;x++){await page.mouse.move(460+x*70,300+y*55);await page.evaluate(()=>(window as any).hoverProbe.pointerMoves++)}
  const hover=await page.evaluate(()=>(window as any).hoverProbe);expect(hover.preselectionChanges).toBeGreaterThan(0);expect(hover.storageWrites).toBe(0);fs.writeFileSync(path.join(output,'hover-storage.json'),JSON.stringify(hover,null,2));
  await page.screenshot({path:path.join(output,'workbench.png')});await tool(page,'拉伸');const extrude=page.getByRole('dialog',{name:/创建.*拉伸/});await expect(extrude).toBeVisible();
  await expect(extrude.getByText('材料',{exact:true})).toHaveCount(0);await expect(extrude.getByText('操作',{exact:true})).toBeVisible();await expect(extrude.getByText('更多参数',{exact:true})).toBeVisible();await expect(extrude.getByText('起始偏移',{exact:true})).not.toBeVisible();
  await extrude.getByText('切除',{exact:true}).click();await extrude.getByRole('combobox',{name:'范围',exact:true}).click();
  const extent=page.locator('.ant-select-dropdown:visible');for(const label of ['平面','曲面','下一面','贯穿'])await expect(extent.getByText(label,{exact:true})).toBeVisible();
  await expect(extent.getByText('到平面',{exact:true})).toHaveCount(0);await page.screenshot({path:path.join(output,'extent-options.png')});await extrude.getByRole('combobox',{name:'范围',exact:true}).press('Escape');
  await page.screenshot({path:path.join(output,'extrude.png')});await extrude.getByRole('button',{name:/取\s*消/}).click();
  await page.getByRole('tab',{name:'线框与曲面',exact:true}).click();
  const toolbar=page.getByRole('toolbar',{name:'工作台命令',exact:true});await expect(toolbar).not.toContainText('拉伸曲面');await expect(toolbar).not.toContainText('延长曲面');
  await tool(page,'拉伸','geometry.surface-extrude');const surface=page.getByRole('dialog',{name:'拉伸',exact:true});await expect(surface).toBeVisible();await surface.getByRole('button',{name:/取\s*消/}).click();
  await page.getByRole('button',{name:'搜索工具',exact:true}).click();await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill('拉伸');
  const solidResult=page.locator('.workbench-command-result[data-command-id="part.pad"]'),surfaceResult=page.locator('.workbench-command-result[data-command-id="geometry.surface-extrude"]');
  await expect(solidResult.locator('strong')).toHaveText('拉伸');await expect(surfaceResult.locator('strong')).toHaveText('拉伸');expect(await solidResult.locator('small').first().innerText()).not.toBe(await surfaceResult.locator('small').first().innerText());
  await page.screenshot({path:path.join(output,'contextual-command-names.png')});await page.keyboard.press('Escape');expect(errors).toEqual([]);
 }finally{
  const csrf=(await page.context().cookies()).find(cookie=>cookie.name==='occccad_csrf')!.value;
  for(const id of [copyID,seed.id].filter(Boolean)){expect((await page.request.delete('/api/documents/'+id,{headers:{'X-CSRF-Token':csrf}})).ok()).toBe(true);expect((await page.request.get('/api/documents/'+id)).status()).toBe(404)}
  expect((await page.request.delete('/api/folders/'+seed.root.id,{headers:{'X-CSRF-Token':csrf}})).ok()).toBe(true);
 }
});

test('settings stay compact and admin editing has a single drawer level',async({page})=>{
 await login(page);const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 await page.getByRole('button',{name:'选项',exact:true}).click();const options=page.getByRole('dialog',{name:'选项',exact:true});await expect(options).toBeVisible();expect((await options.boundingBox())!.width).toBeLessThan(700);
 await options.getByRole('menuitem',{name:'单位',exact:true}).click();await expect(options.getByRole('combobox',{name:'默认长度单位',exact:true})).toBeVisible();await page.screenshot({path:path.join(output,'options-units.png')});await options.getByRole('button',{name:'关闭',exact:true}).click();
 await page.getByRole('button',{name:'管理',exact:true}).click();await page.getByRole('textbox',{name:'搜索账号',exact:true}).fill('ui-no-such-user');await expect(page.getByText('没有找到账号',{exact:true})).toBeVisible();
 await page.getByRole('button',{name:'添加账号',exact:true}).click();await expect(page.getByLabel('显示名称')).toBeVisible();expect(await page.locator('.ant-modal-wrap:visible').count()).toBe(0);await page.screenshot({path:path.join(output,'admin-create.png')});
 await page.getByRole('button',{name:'返回用户列表',exact:true}).click();await expect(page.getByRole('textbox',{name:'搜索账号',exact:true})).toHaveValue('ui-no-such-user');expect(errors).toEqual([]);
});

test('admin form submits create, update and reset through the real API',async({page})=>{
 await login(page);const name='UI account '+Date.now(),email=`ui-${Date.now()}@ui-refinement.invalid`;let userID:string|undefined;
 try{
  await page.getByRole('button',{name:'管理',exact:true}).click();await page.getByRole('button',{name:'添加账号',exact:true}).click();
  await page.getByLabel('显示名称').fill(name);await page.getByLabel('电子邮箱').fill(email);await page.getByLabel('初始密码').fill('Ui-refinement-password-2026');
  const created=page.waitForResponse(response=>new URL(response.url()).pathname==='/api/admin/users'&&response.request().method()==='POST');
  await page.locator('.admin-editor-actions').getByRole('button',{name:/^创\s*建$/}).click();const response=await created;expect(response.ok()).toBe(true);userID=(await response.json()).id;
  await page.getByRole('textbox',{name:'搜索账号',exact:true}).fill(email);let row=page.locator('.admin-drawer tbody tr').filter({hasText:email});await expect(row).toBeVisible();
  await row.getByRole('button',{name:/^编\s*辑$/}).click();await page.getByLabel('显示名称').fill(name+' edited');
  await page.locator('.admin-editor-actions').getByRole('button',{name:/^保\s*存$/}).click();await expect(row).toContainText(name+' edited');
  await row.getByRole('button',{name:/账号操作/}).click();await expect(page.getByRole('menuitem',{name:'编辑账号',exact:true})).toHaveCount(0);await page.getByRole('menuitem',{name:/重置密码/}).click();
  await page.getByLabel('新密码').fill('Ui-refinement-reset-2026');const reset=page.waitForResponse(response=>response.url().endsWith('/reset-password')&&response.request().method()==='POST');
  await page.locator('.admin-editor-actions').getByRole('button',{name:'重置密码',exact:true}).click();expect((await reset).ok()).toBe(true);await expect(row).toBeVisible();
  expect(await page.locator('.ant-modal-wrap:visible').count()).toBe(0);
 }finally{
  if(!userID){const users=await(await page.request.get('/api/admin/users?q='+encodeURIComponent(email))).json();userID=users.users?.find((user:any)=>user.email===email)?.id}
  if(userID){const csrf=(await page.context().cookies()).find(cookie=>cookie.name==='occccad_csrf')!.value;expect((await page.request.delete('/api/admin/users/'+userID,{headers:{'X-CSRF-Token':csrf}})).ok()).toBe(true)}
 }
});

test('batch imports use the Vite realtime connection and one completion notice',async({page})=>{
 test.setTimeout(180_000);const frames:string[]=[];page.on('websocket',socket=>{if(socket.url().includes('/api/realtime'))socket.on('framereceived',frame=>{frames.push(String(frame.payload))})});await login(page);
 const tag='ui-batch-'+Date.now(),names=[tag+'-fillet.brep',tag+'-shallow.brep'];
 try{
  await page.getByRole('button',{name:'导入',exact:true}).click();const dialog=page.getByRole('dialog',{name:'导入文档',exact:true});
  await dialog.locator('input[type=file]').setInputFiles(names.map((name,index)=>({name,mimeType:'application/octet-stream',buffer:fs.readFileSync(path.resolve('../../../build/agent-work/visual-studio/fixtures/'+(index?'shallow':'fillet')+'.brep'))})));
  await dialog.getByRole('button',{name:/^导\s*入$/}).click();await expect(dialog).not.toBeVisible();await expect(page.locator('.exchange-completion-notice')).toHaveCount(1,{timeout:90_000});await expect(page.locator('.exchange-completion-notice')).toContainText('导入完成 · 2/2');
  expect(frames.filter(frame=>frame.includes('job.state.changed.v1')).length).toBeGreaterThan(0);
  await page.getByRole('button',{name:'打开消息中心',exact:true}).click();for(const name of names)await expect(page.locator('.activity-item').filter({hasText:name})).toContainText('已完成');
  const queries=await page.evaluate(async()=>{const {queryClient}=await import('/src/app/providers.tsx');return {jobs:queryClient.getQueryCache().findAll({queryKey:['jobs']}).length,motion:queryClient.getQueryCache().findAll({queryKey:['motion-jobs']}).length}});expect(queries).toEqual({jobs:1,motion:0});
  await page.screenshot({path:path.join(output,'batch-activity.png')});fs.writeFileSync(path.join(output,'batch-activity.json'),JSON.stringify({jobs:2,notices:1,realtimeJobFrames:frames.filter(frame=>frame.includes('job.state.changed.v1')).length,queries},null,2));
 }finally{
  const jobs=(await(await page.request.get('/api/jobs')).json()).jobs.filter((job:any)=>names.includes(job.fileName??job.payload?.fileName));const csrf=(await page.context().cookies()).find(cookie=>cookie.name==='occccad_csrf')!.value;
  for(const job of jobs)if(job.documentId)expect((await page.request.delete('/api/documents/'+job.documentId,{headers:{'X-CSRF-Token':csrf}})).ok()).toBe(true);
 }
});

test('immutable surface resources share GPU uploads and survive sibling removal',async({page})=>{
 test.setTimeout(240_000);await login(page);
 const fixture=path.resolve('../../../build/agent-work/visual-studio/fixtures/fillet-balanced.glb');expect(fs.existsSync(fixture),'Generate the SurfaceDisplay native fixture first').toBe(true);
 const reports=await page.evaluate(async(base64)=>{
  // @ts-ignore test-only Vite module
  const {measureSurfaceResources}=await import('/browser/surface-resources.ts');const {decodeMeshGLB}=await import('/src/cad/visual/mesh-glb.ts');const data=decodeMeshGLB(Uint8Array.from(atob(base64),c=>c.charCodeAt(0)).buffer).mesh;
  return [measureSurfaceResources(data,8,false),measureSurfaceResources(data,8,true),measureSurfaceResources(data,32,false),measureSurfaceResources(data,32,true)];
 },fs.readFileSync(fixture).toString('base64'));
 for(let i=0;i<reports.length;i+=2){const baseline=reports[i],shared=reports[i+1];expect(shared.initial.image).toBe(baseline.initial.image);expect(shared.remaining.image).toBe(baseline.remaining.image);expect(shared.remaining.glError).toBe(0);expect(shared.remaining.additionalUploads).toBe(0);expect(shared.initial.drawCalls).toBe(baseline.initial.drawCalls);expect(shared.initial.uploadedBytes*shared.count).toBe(baseline.initial.uploadedBytes);expect(shared.released.geometries).toBe(0);
  for(const report of [baseline,shared]){fs.writeFileSync(path.join(output,`surfaces-${report.count}-${report.shared?'shared':'baseline'}.png`),Buffer.from(report.initial.image.split(',')[1],'base64'));delete (report.initial as any).image;delete (report.remaining as any).image;}}
 fs.writeFileSync(path.join(output,'surface-resources.json'),JSON.stringify(reports,null,2));
});
