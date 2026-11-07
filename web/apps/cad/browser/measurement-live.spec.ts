import {treeNavigator} from "./tree-helpers";
import {expect,test,type Page} from '@playwright/test';
import {worldScreen} from './camera-helpers';

async function openMeasure(page:Page,name:string){
 await page.getByRole('button',{name:'搜索工具',exact:true}).click();
 await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);
 await page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp(`^${name}$`)})}).click();
 const dialog=page.getByRole('dialog',{name,exact:true});
 if(['测量对象','测量之间','三点角度'].includes(name)){await expect(dialog.getByRole('checkbox',{name:'随文档保留',exact:true})).toBeChecked();await expect(dialog).not.toContainText('已保留的测量');await expect(dialog.getByRole('button',{name:'复制结果',exact:true})).toHaveCount(0)}
 return dialog;
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
 await treeNavigator(page)(seed.bodyName);
 await page.locator(`[role="treeitem"][data-tree-key$="/body:${seed.bodyId}"]`).click();
 await expect(dialog.getByLabel('测量结果')).toContainText('2000 mm³');await page.screenshot({path:info.outputPath('measurement-item-compact.png')});
 const options=dialog.locator('.measurement-pick-options');expect(await options.locator('.command-field').evaluateAll(fields=>Math.abs(fields[0].getBoundingClientRect().top-fields[1].getBoundingClientRect().top)<1)).toBeTruthy();
 await expect(page.locator('.measurement-label')).toHaveCount(1);
 await dialog.getByRole('textbox',{name:'测量名称'}).fill('Box geometry');await dialog.getByRole('checkbox',{name:'随文档保留'}).check();
 await dialog.getByRole('button',{name:/^确\s*定$/,exact:true}).click();await expect(dialog).toHaveCount(0);
 await expect.poll(async()=>(await current(seed.pid)).part.measurements?.length).toBe(1);
 await page.reload();await expect(page.locator('.measurement-label')).toHaveCount(1);
 const stored=(await current(seed.pid)).part.measurements[0];
 await treeNavigator(page)('Box geometry');await page.locator(`[role="treeitem"][data-tree-key$="/${stored.id}"]`).dblclick();
 dialog=page.getByRole('dialog',{name:'测量对象',exact:true});await expect(dialog).toBeVisible();await dialog.getByRole('button',{name:/^取\s*消$/,exact:true}).click();
 // The point filter temporarily reveals vertices even when the user's display
 // preferences hide them. All three picks still use exact Naming supports.
 await treeNavigator(page)('');
 dialog=await openMeasure(page,'三点角度');
 const title=dialog.locator('.cad-command-dialog-header');
 await expect(dialog).toBeVisible();{const box=(await title.boundingBox())!;await page.mouse.move(box.x+50,box.y+15);await page.mouse.down();await page.mouse.move(65,220,{steps:6});await page.mouse.up()}
 for(const [index,p] of [[20,0,10],[20,10,10],[20,10,0]].entries()){const xy=await worldScreen(page,p);expect(await page.evaluate(p=>document.elementFromPoint(p.x,p.y)?.tagName,xy)).toBe('CANVAS');await page.mouse.click(xy.x,xy.y);if(index<2)await expect(dialog.locator('[data-pick-active=true]')).toContainText(index===0?'角度顶点':'第三点')}
 await expect(dialog.getByLabel('测量结果')).toContainText('90°');await expect(page.locator('.measurement-overlay polyline')).toHaveCount(1);await page.screenshot({path:info.outputPath('exact-three-point-angle.png')});
 const reset=(await title.boundingBox())!;await page.mouse.move(reset.x+50,reset.y+15);await page.mouse.down();await page.mouse.move(700,230,{steps:6});await page.mouse.up();
 const temporaryHead=(await current(seed.pid)).document.versionId;await dialog.getByRole('checkbox',{name:'随文档保留',exact:true}).uncheck();await dialog.getByRole('button',{name:/^确\s*定$/}).click();expect((await current(seed.pid)).document.versionId).toBe(temporaryHead);
 // Face picks use the real raycaster. First pointerup retains the second field.
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();dialog=await openMeasure(page,'测量之间');
 {const header=dialog.locator('.cad-command-dialog-header'),box=(await header.boundingBox())!;await page.mouse.move(box.x+50,box.y+15);await page.mouse.down();await page.mouse.move(65,920,{steps:6});await page.mouse.up()}
 const first=await worldScreen(page,[10,5,10]);seed.version=(await current(seed.id)).document.versionId;await page.mouse.click(first.x,first.y);
 await expect(dialog.locator('[data-pick-active=true]')).toContainText('对象 2');
 const second=await worldScreen(page,[50,5,10]);expect(await page.evaluate(p=>document.elementFromPoint(p.x,p.y)?.tagName,second)).toBe('CANVAS');await page.mouse.click(second.x,second.y);
 await expect(dialog.getByLabel('测量结果')).toContainText('20 mm');await expect(page.locator('.measurement-label')).toHaveCount(1);
 expect((await current(seed.id)).document.versionId).toBe(seed.version);
 await dialog.getByRole('checkbox',{name:'显示坐标分量'}).check();await expect(page.locator('.measurement-component')).toHaveCount(3);
 {const header=dialog.locator('.cad-command-dialog-header'),box=(await header.boundingBox())!;await page.mouse.move(box.x+50,box.y+15);await page.mouse.down();await page.mouse.move(65,220,{steps:6});await page.mouse.up()}
 const label=page.locator('.measurement-label'),old=await label.getAttribute('transform'),rect=(await label.boundingBox())!;
 await page.mouse.move(rect.x+30,rect.y+12);await page.mouse.down();await page.mouse.move(rect.x+105,rect.y+65,{steps:8});await page.mouse.up();await expect(label).not.toHaveAttribute('transform',old!);
 const accepted=await label.getAttribute('transform'),drag=(await label.boundingBox())!;await page.mouse.move(drag.x+20,drag.y+12);await page.mouse.down();await page.mouse.move(drag.x+45,drag.y+35);await page.keyboard.press('Escape');await page.mouse.up();await expect(label).toHaveAttribute('transform',accepted!);await expect(dialog).toBeVisible();
 expect((await current(seed.id)).document.versionId).toBe(seed.version);
 await page.screenshot({path:info.outputPath('exact-distance-components.png')});
 await dialog.getByRole('textbox',{name:'测量名称'}).fill('Installation gap');await dialog.getByRole('checkbox',{name:'随文档保留'}).check();await dialog.getByRole('button',{name:/^确\s*定$/,exact:true}).click();await expect(dialog).toHaveCount(0);
 let saved=(await current(seed.id)).product.measurements[0];expect(saved.supports.every((s:any)=>s.persistentSelection&&!s.localId&&!s.geometryKey)).toBeTruthy();
 await page.reload();await expect(page.locator('.measurement-label')).toHaveCount(1);
 const retainedHead=(await current(seed.id)).document.versionId;
 const retainedLabel=page.locator('.measurement-label'),beforeLayout=await retainedLabel.getAttribute('transform'),retainedRect=(await retainedLabel.boundingBox())!;
 await page.mouse.move(retainedRect.x+30,retainedRect.y+12);await page.mouse.down();await page.mouse.move(retainedRect.x+95,retainedRect.y+48,{steps:7});await page.mouse.up();
 await expect(retainedLabel).not.toHaveAttribute('transform',beforeLayout!);
 const retainedTransform=await retainedLabel.getAttribute('transform');
 await expect.poll(async()=>{const result=await page.request.get(`/api/documents/${seed.id}/annotation-layouts`);return (await result.json())[saved.id]?.version??0}).toBe(1);
 expect((await current(seed.id)).document.versionId).toBe(retainedHead);
 await page.reload();await expect(page.locator('.measurement-label')).toHaveAttribute('transform',retainedTransform!);
 await page.evaluate(async s=>{

  // @ts-ignore production module
  const {restApi:api}=await import('/src/api.ts');const v=await api.getDocument(s.id);await api.command(s.id,{type:'MOVE_INSTANCE',versionId:v.document.versionId,instanceId:s.second,translation:[50,0,0],rotation:[0,0,0,1]});
 },seed);await page.reload();await expect(page.locator('.measurement-label')).toHaveCount(1);await expect(page.locator('.measurement-label')).toContainText('待更新');await expect.poll(async()=>(await current(seed.id)).product.measurements[0].status).toBe('STALE');
 await treeNavigator(page)('Installation gap');await page.locator(`[role="treeitem"][data-tree-key$="/${saved.id}"]`).dblclick();dialog=page.getByRole('dialog',{name:'测量之间',exact:true});await dialog.getByRole('button',{name:/^确\s*定$/,exact:true}).click();await expect(dialog).toHaveCount(0);
 await expect(page.locator('.measurement-label')).toHaveCount(1);saved=(await current(seed.id)).product.measurements[0];expect(saved.result.values.distanceMm).toBeCloseTo(30,6);
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect(page.locator('.measurement-label')).toContainText('待更新');await page.getByRole('button',{name:'重做',exact:true}).click();await expect(page.locator('.measurement-label')).toHaveCount(1);
 await page.screenshot({path:info.outputPath('retained-distance-updated.png')});
 // Hold a real Worker response and close the command before releasing it.
 const beforeCancel=(await current(seed.id)).document.versionId;
 let release!:()=>void,seen!:()=>void;const hold=new Promise<void>(r=>release=r),started=new Promise<void>(r=>seen=r);
 const pattern=`**/api/documents/${seed.id}/measure`;
 await page.route(pattern,async route=>{const response=await route.fetch();seen();await hold;await route.fulfill({response}).catch(()=>{})});
 dialog=await openMeasure(page,'测量之间');
 {const header=dialog.locator('.cad-command-dialog-header'),box=(await header.boundingBox())!;await page.mouse.move(box.x+50,box.y+15);await page.mouse.down();await page.mouse.move(700,230,{steps:6});await page.mouse.up()}
 for(const [index,p] of [[10,5,10],[10,0,5]].entries()){const xy=await worldScreen(page,p);await page.mouse.click(xy.x,xy.y);if(index===0)await expect(dialog.locator('[data-pick-active=true]')).toContainText('对象 2')}
 await expect(dialog.locator('[data-pick-active=true]')).toHaveCount(0);
 await Promise.race([started,new Promise((_,reject)=>setTimeout(()=>reject(Error('Measurement response not observed')),15000))]);
 await dialog.getByRole('button',{name:/^取\s*消$/,exact:true}).click();release();await page.unroute(pattern);await expect(dialog).toHaveCount(0);await expect(page.locator('.measurement-label')).toHaveCount(1);
 expect((await current(seed.id)).document.versionId).toBe(beforeCancel);
 // Default retention must also work across a chain's save/Revision boundary.
 dialog=await openMeasure(page,'测量之间');await dialog.getByRole('textbox',{name:'测量名称'}).fill('Chain segment');await dialog.getByRole('combobox',{name:'连续测量方式'}).click();await page.locator('.ant-select-dropdown:visible').getByText('链式 · 末项作为下一首项',{exact:true}).click();
 const members=await page.evaluate(async id=>{
  // @ts-ignore production API module
  const {restApi:api}=await import('/src/api.ts');const v=await api.getDocument(id),nodes:any[]=[];const visit=(n:any)=>{nodes.push(n);n.children?.forEach(visit)};visit(v.structureTree);return v.product!.instances.map(i=>nodes.find(n=>n.kind==='INSTANCE'&&n.entityId===i.id)?.id);
 },seed.id);expect(members.every(Boolean)).toBeTruthy();
 const pickMember=async(id:string)=>{await treeNavigator(page)('member',id);await page.locator(`[role=treeitem][data-tree-key=${JSON.stringify(id)}]`).click()};
 await pickMember(members[0]);await pickMember(members[1]);await expect(dialog.getByLabel('测量结果')).toContainText('30 mm');await dialog.getByRole('button',{name:'保留并继续',exact:true}).click();await expect(dialog.locator('[data-pick-active=true]')).toContainText('对象 2');await expect(dialog.getByRole('checkbox',{name:'随文档保留',exact:true})).toBeChecked();await expect(dialog).not.toContainText('已保留的测量');
 await pickMember(members[0]);await expect(dialog.getByLabel('测量结果')).toContainText('30 mm');await dialog.getByRole('button',{name:/^取\s*消$/,exact:true}).click();expect((await current(seed.id)).product.measurements).toHaveLength(2);expect(errors).toEqual([]);
});

test('Reference elements preview, tree picking, owned parameters and Product parameter scope',async({page},info)=>{
 test.setTimeout(90000);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();await page.goto('/');
 const id=await page.evaluate(async()=>{
  // @ts-ignore production module
  const {restApi:api}=await import('/src/api.ts');return (await api.createDocument('PART',`Reference browser ${Date.now()}`)).document.id;
 });
 await page.goto(`/documents/${id}`);await expect(page.locator('canvas')).toBeVisible();
 await expect(page.locator('[role=treeitem]').filter({hasText:'XY Plane'})).toBeVisible();await expect(page.locator('[role=treeitem]').filter({hasText:/^Origin$/})).toHaveCount(0);
 const createPoint=async(name:string,z:string)=>{await openMeasure(page,'参考点');const dialog=page.getByRole('dialog',{name:'创建参考点',exact:true});await dialog.getByRole('textbox',{name:'名称',exact:true}).fill(name);await dialog.getByRole('textbox',{name:'Z（mm）',exact:true}).fill(z);await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeEnabled();await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0)};
 await createPoint('Anchor A','0');await createPoint('Anchor B','10');
 const current=async()=>(await page.request.get(`/api/documents/${id}`)).json();
 let view=await current();const [a,b]=view.part.datumPoints;
 await expect(page.locator(`[role=treeitem][data-tree-key$="/point:${b.id}"]`)).toBeVisible();
 await openMeasure(page,'参考线');let dialog=page.getByRole('dialog',{name:'创建参考线',exact:true});
 await dialog.getByRole('button',{name:'选择点 1',exact:true}).click();await page.locator(`[role=treeitem][data-tree-key$="/point:${a.id}"]`).click();
 await dialog.getByRole('button',{name:'选择点 2',exact:true}).click();const pointScreen=await worldScreen(page,[0,0,10]);await page.mouse.click(pointScreen.x,pointScreen.y);
 await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeEnabled();await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 view=await current();expect(view.part.datumAxes[0].direction).toEqual([0,0,1]);expect(view.part.geometricalSets).toHaveLength(1);
 await page.locator(`[role=treeitem][data-tree-key$="/point:${b.id}"]`).dblclick();dialog=page.getByRole('dialog',{name:'编辑参考点',exact:true});await expect(dialog).toBeVisible();await dialog.getByRole('textbox',{name:'Z（mm）',exact:true}).fill('20');await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 await page.reload();await expect(page.locator('canvas')).toBeVisible();view=await current();expect(view.part.datumPoints[1].position[2]).toBe(20);
 await page.screenshot({path:info.outputPath('reference-elements.png')});const partName=view.document.name;
 const productID=await page.evaluate(async id=>{
  // @ts-ignore production module
  const {restApi:api}=await import('/src/api.ts');const part=await api.getDocument(id),parameter=part.part!.parameters!.find(p=>p.propertySlot==='reference:z'&&p.evaluatedValue?.siValue===.02)!;await api.createPublication(id,{publicationType:'PARAMETER',targetKind:'PARAMETER',targetId:parameter.parameterId,name:'Anchor height'});let product=await api.createDocument('PRODUCT','Global parameters browser');product=await api.command(product.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:id});return product.document.id;
 },id);
 await page.goto(`/documents/${productID}`);await expect(page.locator('canvas')).toBeVisible();await openMeasure(page,'参数与公式');dialog=page.getByRole('dialog',{name:'参数与公式',exact:true});await expect(dialog).toBeVisible();await expect(dialog.getByRole('combobox',{name:'参数所属文档'})).toBeVisible();
 await dialog.getByPlaceholder('别名（可选）').fill('ProductWidth');await dialog.getByRole('button',{name:'新建参数',exact:true}).click();await expect(dialog.getByRole('region',{name:'参数列表'})).toContainText('ProductWidth');
 await page.screenshot({path:info.outputPath('product-parameters.png')});
 await dialog.getByRole('combobox',{name:'参数所属文档'}).click();await page.locator('.ant-select-dropdown .ant-select-item-option').filter({hasText:partName}).click();
 await expect(dialog.getByRole('region',{name:'参数列表'})).toContainText('Anchor B');
 await dialog.getByRole('combobox',{name:'参数所属文档'}).click();await page.locator('.ant-select-dropdown .ant-select-item-option').filter({hasText:'Global parameters browser'}).click();
 await dialog.getByRole('button',{name:/引\s*用/}).click();dialog=page.getByRole('dialog',{name:'引用发布参数',exact:true});await expect(dialog).toBeVisible();
 await dialog.getByRole('textbox',{name:'搜索文档',exact:true}).fill(partName);
 await dialog.getByRole('combobox',{name:'来源文档',exact:true}).click();await page.locator('.ant-select-dropdown .ant-select-item-option').filter({hasText:partName}).click();
 await dialog.getByRole('combobox',{name:'发布参数',exact:true}).click();await page.locator('.ant-select-dropdown .ant-select-item-option').filter({hasText:'Anchor height'}).click();
 await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeEnabled();await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 await expect.poll(async()=>((await (await page.request.get(`/api/documents/${productID}`)).json()).product.parameters[0].evaluatedValue.siValue)).toBe(.02);
 expect(errors).toEqual([]);
});
