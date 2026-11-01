import {expect,test,type Page} from '@playwright/test';
import {worldScreen} from './camera-helpers';

async function openMeasure(page:Page,name:string){
 await page.getByRole('button',{name:'搜索工具',exact:true}).click();
 await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);
 await page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp(`^${name}$`)})}).click();
 return page.getByRole('dialog',{name,exact:true});
}
test('Part and Product exact measurement, paired picking, annotation drag and retained history',async({page},info)=>{
 test.setTimeout(240_000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();await page.goto('/');
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production module
  const {restApi:api}=await import('/src/api.ts');
  // @ts-ignore production preferences
  const {useUIPreferences}=await import('/src/state/ui-preferences.ts');useUIPreferences.getState().setSolidDisplay({mode:'shaded',edges:false,vertices:false});
  let part=await api.createDocument('PART','Measurement browser box'),pid=part.document.id;
  part=await api.command(pid,{type:'CREATE_SKETCH',plane:'XY'});const sid=part.part!.features.at(-1)!.id;
  part=await api.command(pid,{type:'EDIT_SKETCH',sketchId:sid,operations:[{type:'ADD_RECTANGLE',first:{x:0,y:0},second:{x:20,y:10}}]});
  part=await api.command(pid,{type:'CREATE_SOLID_FEATURE',sketchId:sid,generator:'LINEAR_EXTRUDE',operation:'ADD',length:10});const body=part.part!.bodies.find(b=>b.geometryKey)!;
  let root=await api.createDocument('PRODUCT','Measurement browser assembly');for(let i=0;i<2;i++)root=await api.command(root.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:pid});
  root=await api.command(root.document.id,{type:'MOVE_INSTANCE',instanceId:root.product!.instances[1].id,translation:[40,0,0],rotation:[0,0,0,1]});
  return {pid,bodyId:body.id,bodyName:body.name,id:root.document.id,version:root.document.versionId,second:root.product!.instances[1].id};
 });
 const current=async(id:string)=>(await page.request.get(`/api/documents/${id}`)).json();
 await page.goto(`/documents/${seed.pid}`);await expect(page.locator('canvas')).toBeVisible();let dialog=await openMeasure(page,'测量对象');
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill(seed.bodyName);
 await page.locator(`[role="treeitem"][data-tree-key$="/body:${seed.bodyId}"]`).click();
 await expect(dialog.getByLabel('测量结果')).toContainText('2000 mm³');
 await expect(page.locator('.measurement-label')).toHaveCount(1);
 await dialog.getByRole('textbox',{name:'测量名称'}).fill('Box geometry');await dialog.getByRole('checkbox',{name:'随文档保留'}).check();
 await dialog.getByRole('button',{name:'保留测量',exact:true}).click();await expect(dialog).toHaveCount(0);
 await expect.poll(async()=>(await current(seed.pid)).part.measurements?.length).toBe(1);
 await page.reload();await expect(page.locator('.measurement-label')).toHaveCount(1);
 const stored=(await current(seed.pid)).part.measurements[0];
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill('Box geometry');await page.locator(`[role="treeitem"][data-tree-key$="/${stored.id}"]`).dblclick();
 dialog=page.getByRole('dialog',{name:'测量对象',exact:true});await expect(dialog).toBeVisible();await dialog.getByRole('button',{name:'关闭',exact:true}).click();
 // The point filter temporarily reveals vertices even when the user's display
 // preferences hide them. All three picks still use exact Naming supports.
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill('');
 dialog=await openMeasure(page,'三点角度');
 const title=dialog.locator('.cad-command-dialog-header');
 await expect(dialog).toBeVisible();{const box=(await title.boundingBox())!;await page.mouse.move(box.x+50,box.y+15);await page.mouse.down();await page.mouse.move(65,220,{steps:6});await page.mouse.up()}
 for(const [index,p] of [[20,0,10],[20,10,10],[20,10,0]].entries()){const xy=await worldScreen(page,p);await page.mouse.click(xy.x,xy.y);if(index<2)await expect(dialog.locator('[data-pick-active=true]')).toContainText(index===0?'角度顶点':'第三点')}
 await expect(dialog.getByLabel('测量结果')).toContainText('90°');await expect(page.locator('.measurement-overlay polyline')).toHaveCount(1);await page.screenshot({path:info.outputPath('exact-three-point-angle.png')});
 const reset=(await title.boundingBox())!;await page.mouse.move(reset.x+50,reset.y+15);await page.mouse.down();await page.mouse.move(700,230,{steps:6});await page.mouse.up();
 await dialog.getByRole('button',{name:/完\s*成/}).click();
 // Face picks use the real raycaster. First pointerup retains the second field.
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();dialog=await openMeasure(page,'测量之间');
 const first=await worldScreen(page,[10,5,10]);seed.version=(await current(seed.id)).document.versionId;await page.mouse.click(first.x,first.y);
 await expect(dialog.locator('[data-pick-active=true]')).toContainText('对象 2');
 const second=await worldScreen(page,[50,5,10]);await page.mouse.click(second.x,second.y);
 await expect(dialog.getByLabel('测量结果')).toContainText('20 mm');await expect(page.locator('.measurement-label')).toHaveCount(1);
 expect((await current(seed.id)).document.versionId).toBe(seed.version);
 await dialog.getByRole('checkbox',{name:'显示坐标分量'}).check();await expect(page.locator('.measurement-component')).toHaveCount(3);
 const label=page.locator('.measurement-label'),old=await label.getAttribute('transform'),rect=(await label.boundingBox())!;
 await page.mouse.move(rect.x+30,rect.y+12);await page.mouse.down();await page.mouse.move(rect.x+105,rect.y+65,{steps:8});await page.mouse.up();await expect(label).not.toHaveAttribute('transform',old!);
 const accepted=await label.getAttribute('transform'),drag=(await label.boundingBox())!;await page.mouse.move(drag.x+20,drag.y+12);await page.mouse.down();await page.mouse.move(drag.x+45,drag.y+35);await page.keyboard.press('Escape');await page.mouse.up();await expect(label).toHaveAttribute('transform',accepted!);await expect(dialog).toBeVisible();
 expect((await current(seed.id)).document.versionId).toBe(seed.version);
 await page.screenshot({path:info.outputPath('exact-distance-components.png')});
 await dialog.getByRole('textbox',{name:'测量名称'}).fill('Installation gap');await dialog.getByRole('checkbox',{name:'随文档保留'}).check();await dialog.getByRole('button',{name:'保留测量',exact:true}).click();await expect(dialog).toHaveCount(0);
 let saved=(await current(seed.id)).product.measurements[0];expect(saved.supports.every((s:any)=>s.persistentSelection&&!s.localId&&!s.geometryKey)).toBeTruthy();
 await page.reload();await expect(page.locator('.measurement-label')).toHaveCount(1);
 await page.evaluate(async s=>{
  // @ts-ignore production module
  const {restApi:api}=await import('/src/api.ts');const v=await api.getDocument(s.id);await api.command(s.id,{type:'MOVE_INSTANCE',versionId:v.document.versionId,instanceId:s.second,translation:[50,0,0],rotation:[0,0,0,1]});
 },seed);await page.reload();await expect(page.locator('.measurement-label')).toHaveCount(0);await expect.poll(async()=>(await current(seed.id)).product.measurements[0].status).toBe('STALE');
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill('Installation gap');await page.locator(`[role="treeitem"][data-tree-key$="/${saved.id}"]`).dblclick();dialog=page.getByRole('dialog',{name:'测量之间',exact:true});await dialog.getByRole('button',{name:'更新测量',exact:true}).click();await expect(dialog).toHaveCount(0);
 await expect(page.locator('.measurement-label')).toHaveCount(1);saved=(await current(seed.id)).product.measurements[0];expect(saved.result.values.distanceMm).toBeCloseTo(30,6);
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect(page.locator('.measurement-label')).toHaveCount(0);await page.getByRole('button',{name:'重做',exact:true}).click();await expect(page.locator('.measurement-label')).toHaveCount(1);
 await page.screenshot({path:info.outputPath('retained-distance-updated.png')});
 // Hold a real Worker response and close the command before releasing it.
 const beforeCancel=(await current(seed.id)).document.versionId;
 let release!:()=>void,seen!:()=>void;const hold=new Promise<void>(r=>release=r),started=new Promise<void>(r=>seen=r);
 const pattern=`**/api/documents/${seed.id}/measure`;
 await page.route(pattern,async route=>{const response=await route.fetch();seen();await hold;await route.fulfill({response}).catch(()=>{})});
 dialog=await openMeasure(page,'测量之间');
 for(const [index,p] of [[10,5,10],[10,0,5]].entries()){const xy=await worldScreen(page,p);await page.mouse.click(xy.x,xy.y);if(index===0)await expect(dialog.locator('[data-pick-active=true]')).toContainText('对象 2')}
 await expect(dialog.locator('[data-pick-active=true]')).toHaveCount(0);
 await Promise.race([started,new Promise((_,reject)=>setTimeout(()=>reject(Error('Measurement response not observed')),15000))]);
 await dialog.getByRole('button',{name:'关闭',exact:true}).click();release();await page.unroute(pattern);await expect(dialog).toHaveCount(0);await expect(page.locator('.measurement-label')).toHaveCount(1);
 expect((await current(seed.id)).document.versionId).toBe(beforeCancel);expect(errors).toEqual([]);
});
