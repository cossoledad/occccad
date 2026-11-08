import {expect,test,type Page} from '@playwright/test';
import {worldScreen,observeViewport} from './camera-helpers';
import {treeNavigator} from './tree-helpers';

test.beforeEach(({page})=>{page.setDefaultTimeout(15_000)});

async function login(page:Page){
 if(!process.env.OCCCCAD_ADMIN_PASSWORD)throw new Error('Requires configured running application');
 const response=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(response.ok()).toBeTruthy();await page.goto('/');
}
async function command(page:Page,name:string){await page.getByRole('button',{name:'搜索工具',exact:true}).click();await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);await page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp(`^${name}$`)})}).click()}
async function document(page:Page,id:string){return (await page.request.get(`/api/documents/${id}`)).json()}
async function ready(page:Page,dialog:ReturnType<Page['getByRole']>){await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeEnabled();await expect(page.locator('[data-feature-preview]')).toHaveCount(1)}
async function clickWorld(page:Page,point:number[]){const p=await worldScreen(page,point);await page.mouse.click(p.x,p.y)}

async function selectTree(page:Page,id:string,name:string,kind='feature'){
 await treeNavigator(page)(name,`/${kind}:${id}`);
 await page.locator(`[role=treeitem][data-tree-key$="/${kind}:${id}"]`).getByText(name,{exact:true}).click();
}
async function geometryTab(page:Page){await page.getByRole('tab',{name:'线框与曲面',exact:true}).click()}
async function seedPoints(page:Page){return page.evaluate(async()=>{
 const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Geometry references acceptance');const id=v.document.id,points:Record<string,string>={};
 for(const [name,coordinates] of [['Start',[2,3,0]],['End',[32,3,0]],['Direction start',[2,3,5]],['Direction end',[2,3,15]],['Third',[12,13,8]],['Inserted',[22,8,4]]] as Array<[string,number[]]>){
  v=await api.command(id,{type:'CREATE_REFERENCE_POINT',name,referenceConstruction:{mode:'POINT_COORDINATES',coordinates}});points[name]=v.part!.datumPoints!.at(-1)!.id;
 }
 v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name:'Direction line',geometry:{kind:'LINE',curve:{points:[{id:'start',position:[0,0,0],reference:{kind:'DATUM',datum:{kind:'POINT',entityId:points['Direction start']}}},{id:'end',position:[0,0,0],reference:{kind:'DATUM',datum:{kind:'POINT',entityId:points['Direction end']}}}]},parameters:{direction:[0,0,0],origin:[0,0,0],distance:0,angle:0,start:0,end:0,tension:1}}}});
 return {id,points,direction:v.part!.features.at(-1)!.id};
})}

test('Geometry tabs: explicit picks, associative direction, history and reopen',async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));await login(page);const seed=await seedPoints(page),id=seed.id;
 await page.goto(`/documents/${id}`);await expect(page.locator('canvas')).toBeVisible();const initial=await document(page,id);
 await expect(page.getByRole('tab',{name:'实体',exact:true})).toBeVisible();await geometryTab(page);expect((await document(page,id)).document.versionId).toBe(initial.document.versionId);
 // A preexisting selection is never copied into a newly opened definition.
 await selectTree(page,seed.points.Start,'Start','point');await command(page,'直线');const line=page.getByRole('dialog',{name:'直线',exact:true});await expect(line.getByRole('button',{name:/确\s*定/})).toBeDisabled();await expect(line.getByText('未选择',{exact:true})).toHaveCount(2);
 await selectTree(page,seed.points.Start,'Start','point');await clickWorld(page,[32,3,0]);await ready(page,line);await expect(line).not.toContainText('更多');await page.screenshot({path:info.outputPath('line-references.png')});
 await line.getByRole('button',{name:/确\s*定/}).click();await expect(line).toHaveCount(0);const curve=(await document(page,id)).part.features.at(-1);expect(curve.geometry.curve.points.every((p:any)=>p.reference)).toBeTruthy();
 await command(page,'拉伸曲面');const sheet=page.getByRole('dialog',{name:'拉伸曲面',exact:true});await clickWorld(page,[17,3,0]);await expect(sheet.getByRole('button',{name:/确\s*定/})).toBeDisabled();await selectTree(page,seed.direction,'Direction line');await ready(page,sheet);await expect(sheet.getByRole('combobox',{name:'X',exact:true})).toHaveCount(0);await page.screenshot({path:info.outputPath('sheet-direction-reference.png')});
 await sheet.getByRole('button',{name:/确\s*定/}).click();await expect(sheet).toHaveCount(0);const surface=(await document(page,id)).part.features.at(-1);expect(surface.geometry.direction).toEqual({kind:'RESULT',featureId:seed.direction,resultId:'result'});
 await command(page,'加厚');const thick=page.getByRole('dialog',{name:'加厚',exact:true});await clickWorld(page,[17,3,10]);await ready(page,thick);await thick.getByRole('button',{name:/确\s*定/}).click();await expect(thick).toHaveCount(0);
 const volume=(v:any)=>Object.values(v.artifacts).reduce((n:number,a:any)=>n+(a.geometryDomain==='SOLID_SET'?a.volume:0),0);expect(volume(await document(page,id))).toBeCloseTo(1200,5);
 await selectTree(page,seed.points['Direction end'],'Direction end','point');await page.locator(`[role=treeitem][data-tree-key$="/point:${seed.points['Direction end']}"]`).getByText('Direction end',{exact:true}).dblclick();
 const edit=page.getByRole('dialog',{name:'编辑参考点',exact:true});await edit.getByRole('combobox',{name:'Y',exact:true}).fill('13');await edit.getByRole('combobox',{name:'Y',exact:true}).blur();await edit.getByRole('combobox',{name:'Z',exact:true}).fill('5');await edit.getByRole('combobox',{name:'Z',exact:true}).blur();await expect(edit.getByRole('button',{name:/确\s*定/})).toBeEnabled();await edit.getByRole('button',{name:/确\s*定/}).click();await expect(edit).toHaveCount(0);
 const bounds=async()=>{const v=await document(page,id),f=v.part.features.find((f:any)=>f.id===surface.id);return v.artifacts[f.result.geometryKey].bbox};
 await expect.poll(async()=>(await bounds()).max[1]).toBeCloseTo(23,4);expect(volume(await document(page,id))).toBeCloseTo(1200,5);
 const edited=(await document(page,id)).part.features.find((f:any)=>f.id===surface.id);expect(edited.geometry).toEqual(surface.geometry);
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await bounds()).max[2]).toBeCloseTo(20,4);await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>(await bounds()).max[1]).toBeCloseTo(23,4);
 await page.reload();await expect(page.locator('canvas')).toBeVisible();await expect.poll(async()=>(await bounds()).max[1]).toBeCloseTo(23,4);await page.screenshot({path:info.outputPath('geometry-reopened.png')});expect(errors).toEqual([]);
});

test('Spline starts empty and supports ordered point picks, insertion, replacement and cancel',async({page},info)=>{
 await login(page);const seed=await seedPoints(page);await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();await geometryTab(page);
 await selectTree(page,seed.points.Start,'Start','point');const before=await document(page,seed.id);await command(page,'样条');const spline=page.getByRole('dialog',{name:'样条',exact:true});const rows=spline.getByRole('list',{name:'点列列表'}).getByRole('listitem');
 await expect(rows).toHaveCount(0);await expect(page.locator('[data-feature-preview]')).toHaveCount(0);await expect(spline.getByRole('button',{name:/确\s*定/})).toBeDisabled();
 await selectTree(page,seed.points.Start,'Start','point');await expect(rows).toHaveCount(1);await clickWorld(page,[32,3,0]);await selectTree(page,seed.points.Third,'Third','point');await expect(rows).toHaveCount(3);await ready(page,spline);
 await spline.getByRole('button',{name:'在点 2 前插入',exact:true}).click();await selectTree(page,seed.points.Inserted,'Inserted','point');await expect(rows).toHaveCount(4);await expect(rows.nth(1)).toContainText('Inserted');await ready(page,spline);
 await spline.getByRole('button',{name:'移除点 2',exact:true}).click();await expect(rows).toHaveCount(3);await spline.getByRole('button',{name:'选择点 3',exact:true}).click();await selectTree(page,seed.points.Inserted,'Inserted','point');await expect(rows.nth(2)).toContainText('Inserted');await ready(page,spline);
 await spline.getByText('控制点',{exact:true}).click();await ready(page,spline);await page.screenshot({path:info.outputPath('spline-point-list.png')});await spline.getByRole('button',{name:/取\s*消/}).click();await expect(spline).toHaveCount(0);expect((await document(page,seed.id)).document.versionId).toBe(before.document.versionId);
 // A geometrically invalid direction is reported through the application outlet.
 await command(page,'拉伸曲面');const surface=page.getByRole('dialog',{name:'拉伸曲面',exact:true});await selectTree(page,seed.direction,'Direction line');await selectTree(page,seed.direction,'Direction line');await expect(surface.getByRole('button',{name:/确\s*定/})).toBeDisabled();await expect(page.locator('.ant-notification')).toBeVisible();await page.screenshot({path:info.outputPath('geometry-global-error.png')});await surface.getByRole('button',{name:/取\s*消/}).click();expect((await document(page,seed.id)).document.versionId).toBe(before.document.versionId);
});

test('Solid sweep selects an exact spatial curve in the viewport',async({page},info)=>{
 await login(page);const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Spatial curve sweep acceptance');const id=v.document.id;
  v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const sketch=v.part!.features.at(-1)!;
  v=await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch.id,operations:[{type:'ADD_ENTITY',entity:{id:'disk',kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius:1}}]});
  v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name:'Spatial rail',geometry:{kind:'SPLINE',curve:{points:[{id:'a',position:[0,0,0]},{id:'b',position:[0,0,10]},{id:'c',position:[10,10,20]},{id:'d',position:[10,20,30]}],degree:3,knots:[0,1],multiplicities:[4,4],weights:[1,1,1,1],normal:[0,0,1],xDirection:[1,0,0]},parameters:{direction:[0,0,0],origin:[0,0,0],distance:0,angle:0,start:0,end:0,tension:1}}}});
  return {id,sketch:sketch.id,sketchName:sketch.name};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();await treeNavigator(page)(seed.sketchName??'Sketch',`/sketch:${seed.sketch}`);await page.locator(`[role=treeitem][data-tree-key$="/sketch:${seed.sketch}"]`).click();
 await command(page,'扫掠');const dialog=page.getByRole('dialog',{name:/创建.*扫掠/});await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeDisabled();await expect(dialog.getByRole('button',{name:'移除轮廓',exact:true})).toHaveCount(0);await page.locator(`[role=treeitem][data-tree-key$="/sketch:${seed.sketch}"]`).click();await dialog.getByRole('button',{name:'选择路径曲线',exact:true}).click();
 await clickWorld(page,[5,6.25,15]);await ready(page,dialog);await expect(dialog).toContainText('Spatial rail');await page.screenshot({path:info.outputPath('spatial-curve-sweep-preview.png')});
 await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);const v=await document(page,seed.id),sweep=v.part.features.at(-1);expect(sweep.type).toBe('SWEEP');expect(sweep.sweep.path.geometry.kind).toBe('TOPOLOGY');expect(sweep.sweep.path.geometry.selection.selection.sourceResult).toBeTruthy();expect(Object.values(v.artifacts).some((a:any)=>a.geometryDomain==='SOLID_SET'&&a.volume>30*Math.PI)).toBeTruthy();
});

test('Associative curve tools preserve input roles and display point results',async({page},info)=>{
 await login(page);const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');let view=await api.createDocument('PART','Associative curve acceptance');const id=view.document.id;
  view=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name:'Base rail',geometry:{kind:'LINE',curve:{points:[{id:'a',position:[0,0,0]},{id:'b',position:[30,0,0]}],normal:[0,0,1],xDirection:[1,0,0]},parameters:{direction:[0,0,1],origin:[0,0,0],distance:20,angle:0,start:.5,end:.8,tension:1}}}});
  return {id,curve:view.part!.features.at(-1)!.id};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();await geometryTab(page);
 const pickTree=async(id:string,name:string)=>{await treeNavigator(page)(name,`/feature:${id}`);await page.locator(`[role=treeitem][data-tree-key$="/feature:${id}"]`).getByText(name,{exact:true}).click()};
 await command(page,'曲线上点');let dialog=page.getByRole('dialog',{name:'曲线上点',exact:true});await clickWorld(page,[12,0,0]);await ready(page,dialog);await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 expect((await document(page,seed.id)).part.features.at(-1).result.domain).toBe('POINT_SET');await observeViewport(page);await expect.poll(()=>page.evaluate(()=>[...(window as any).__cameraEngine.solidBindings.values()].filter((b:any)=>b.artifact.geometryDomain==='POINT_SET').length)).toBe(1);
 await pickTree(seed.curve,'Base rail');await command(page,'分割');dialog=page.getByRole('dialog',{name:'分割',exact:true});await pickTree(seed.curve,'Base rail');await ready(page,dialog);await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);const split=(await document(page,seed.id)).part.features.at(-1);
 await pickTree(split.id,split.name);await command(page,'复合曲线');dialog=page.getByRole('dialog',{name:'复合曲线',exact:true});await pickTree(split.id,split.name);await ready(page,dialog);await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);expect((await document(page,seed.id)).part.features.at(-1).geometry.kind).toBe('COMPOSITE');
 await pickTree(seed.curve,'Base rail');await command(page,'偏移曲线');dialog=page.getByRole('dialog',{name:'偏移曲线',exact:true});await pickTree(seed.curve,'Base rail');await page.getByRole('treeitem',{name:'XY Plane',exact:true}).click();await ready(page,dialog);
 await dialog.getByRole('button',{name:'移除曲线',exact:true}).click();await expect(dialog).toContainText('XY Plane');await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeDisabled();await dialog.getByRole('button',{name:'选择曲线',exact:true}).click();await pickTree(seed.curve,'Base rail');await ready(page,dialog);await page.screenshot({path:info.outputPath('curve-input-roles.png')});await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const offset=(await document(page,seed.id)).part.features.at(-1);expect(offset.geometry.inputs[0].kind).toBe('RESULT');expect(offset.geometry.inputs[1].kind).toBe('PLANE');expect(offset.result.domain).toBe('WIRE_SET');
});
