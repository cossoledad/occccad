import {confirmCommandName} from "./command-labels";
import {expect,test,type Page,type Locator} from '@playwright/test';
import {treeNavigator} from './tree-helpers';
import {observeViewport} from './camera-helpers';

test.beforeEach(({page})=>page.setDefaultTimeout(15_000));

async function login(page:Page){
 const r=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});
 expect(r.ok()).toBeTruthy();await page.goto('/');
}
async function current(page:Page,id:string){return (await page.request.get(`/api/documents/${id}`)).json()}
async function tool(page:Page,name:string){await page.getByRole('button',{name:'搜索工具',exact:true}).click();await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);await page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp(`^${name}$`)})}).click()}
async function row(page:Page,id:string,name:string,kind='feature'){
 await treeNavigator(page)(name,`/${kind}:${id}`);
 return page.locator(`[role=treeitem][data-tree-key$="/${kind}:${id}"]`).getByText(name,{exact:true});
}
async function choose(page:Page,id:string,name:string,kind='feature'){await (await row(page,id,name,kind)).click()}
async function save(dialog:Locator){await expect(dialog.getByRole('button',{name:confirmCommandName})).toBeEnabled();await dialog.getByRole('button',{name:confirmCommandName}).click();await expect(dialog).toHaveCount(0)}
async function seed(page:Page){return page.evaluate(async()=>{
 // @ts-ignore Vite source module, real authenticated API.
 const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Part object delivery acceptance');const id=v.document.id;
 const parameters={direction:[0,0,1],origin:[0,0,0],distance:20,angle:360,start:.5,end:.8,tension:1};
 const line=async(name:string,x:number)=>{v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name,geometry:{kind:'LINE',curve:{points:[{id:'a',position:[0,0,0]},{id:'b',position:[x,0,0]}]},parameters}}});return v.part!.features.at(-1)!};
 const input=await line('Curve input',40),alternate=await line('Alternate input',30);
 v=await api.command(id,{type:'CREATE_DATUM_AXIS',name:'Extrusion direction',referenceConstruction:{mode:'LINE_COORDINATES',coordinates:[0,0,0],direction:[0,0,1]}});const axis=v.part!.datumAxes!.at(-1)!;
 v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name:'Sheet result',geometry:{kind:'SURFACE_EXTRUDE',inputs:[{kind:'RESULT',featureId:input.id,resultId:'result'}],direction:{kind:'DATUM',datum:{kind:'AXIS',entityId:axis.id}},parameters}}});const sheet=v.part!.features.at(-1)!;
 v=await api.command(id,{type:'CREATE_SOLID_FEATURE',feature:{id:'',type:'THICKEN',name:'Material result',operation:'NEW_BODY',geometry:{kind:'THICKEN',inputs:[{kind:'RESULT',featureId:sheet.id,resultId:'result'}],parameters:{...parameters,distance:2}}}});const solid=v.part!.features.at(-1)!;
 v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name:'Revolution profile',geometry:{kind:'LINE',curve:{points:[{id:'a',position:[10,0,0]},{id:'b',position:[10,0,20]}]},parameters}}});const generatrix=v.part!.features.at(-1)!;
 const points=[];for(const [name,coordinates] of [['Point A',[0,10,0]],['Point B',[20,10,5]],['Point C',[40,10,0]]] as const){v=await api.command(id,{type:'CREATE_REFERENCE_POINT',name,referenceConstruction:{mode:'POINT_COORDINATES',coordinates}});points.push(v.part!.datumPoints!.at(-1)!)}
 v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name:'Fill boundary',geometry:{kind:'CIRCLE',curve:{points:[{id:'center',position:[0,0,0],reference:{kind:'DATUM',datum:{kind:'POINT',entityId:points[0].id}}}],support:{kind:'PLANE',planeId:'datum-xy'},radius:5},parameters}}});const circle=v.part!.features.at(-1)!;
 v=await api.command(id,{type:'CREATE_GEOMETRICAL_SET',name:'Destination'});const destination=v.part!.geometricalSets!.at(-1)!;
 return {id,input,alternate,axis,sheet,solid,generatrix,points,circle,destination,container:v.part!.geometricalSets!.find(s=>s.id===input.geometricalSetId)!};
})}

test('Part lifecycle: impact, missing input, repair and compensation in the workbench',async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await login(page);const s=await seed(page);
 await page.goto(`/documents/${s.id}`);await expect(page.locator('canvas')).toBeVisible();
 const before=await current(page,s.id);
 await (await row(page,s.input.id,s.input.name!)).click({button:'right'});await page.getByRole('menuitem',{name:/^删除/}).click();
 const del=page.getByRole('dialog',{name:'删除对象',exact:true});await expect(del).toContainText('Curve input');await del.getByRole('tab',{name:/保留并更新/}).click();await expect(del).toContainText('Sheet result');await expect(del).toContainText(s.solid.name!);await page.screenshot({path:info.outputPath('delete-impact.png')});
 await del.getByRole('checkbox',{name:'同时删除依赖特征',exact:true}).check();await del.getByRole('tab',{name:/删除/}).click();await expect(del).toContainText(s.solid.name!);await del.getByRole('button',{name:/取\s*消/}).click();expect((await current(page,s.id)).document.versionId).toBe(before.document.versionId);
 await (await row(page,s.input.id,s.input.name!)).click({button:'right'});await page.getByRole('menuitem',{name:/^删除/}).click();await save(del);
 let v=await current(page,s.id);expect(v.part.features.find((f:any)=>f.id===s.sheet.id).evaluationStatus).toBe('FAILED');expect(v.part.features.find((f:any)=>f.id===s.solid.id).evaluationStatus).toBe('BLOCKED');
 await (await row(page,s.sheet.id,s.sheet.name!)).dblclick();const edit=page.getByRole('dialog',{name:'编辑拉伸曲面',exact:true});await expect(edit).toContainText('已删除');await expect(edit.getByRole('button',{name:confirmCommandName})).toBeDisabled();
 await choose(page,s.alternate.id,s.alternate.name!);await expect(edit).toContainText('Alternate input');await save(edit);
 v=await current(page,s.id);expect(v.part.features.find((f:any)=>f.id===s.solid.id).evaluationStatus).toBeFalsy();
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await current(page,s.id)).part.features.find((f:any)=>f.id===s.sheet.id).evaluationStatus).toBe('FAILED');
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await current(page,s.id)).part.features.some((f:any)=>f.id===s.input.id)).toBe(true);
 await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>(await current(page,s.id)).part.features.some((f:any)=>f.id===s.input.id)).toBe(false);
 await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>(await current(page,s.id)).part.features.find((f:any)=>f.id===s.solid.id).evaluationStatus).toBeFalsy();
 await page.reload();await expect(page.locator('canvas')).toBeVisible();await page.screenshot({path:info.outputPath('repaired-part.png')});expect(errors).toEqual([]);
});

test('Part properties, working collection, move and dissolve retain exact geometry',async({page},info)=>{
 await login(page);const s=await seed(page);await page.goto(`/documents/${s.id}`);await expect(page.locator('canvas')).toBeVisible();
 await choose(page,s.container.id,s.container.name,'geometrical-set');await page.keyboard.press('Alt+Enter');const properties=page.getByRole('dialog',{name:/^属性 ·/});await expect(properties).toBeVisible();await properties.getByRole('textbox',{name:'颜色 HEX',exact:true}).fill('#25bd8d');await properties.getByRole('spinbutton',{name:'线宽（px）',exact:true}).fill('3');await save(properties);
 let v=await current(page,s.id);expect(v.resultAppearances[s.input.id].color).toBe('#25bd8d');expect(v.part.features.find((f:any)=>f.id===s.input.id).result.geometryKey).toBe(s.input.result!.geometryKey);
 await choose(page,s.points[0].id,s.points[0].name,'point');await page.keyboard.press('Alt+Enter');await properties.getByRole('combobox',{name:'点符号',exact:true}).click();await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText('十字',{exact:true}).click();await properties.getByRole('spinbutton',{name:'点大小（px）',exact:true}).fill('17');await save(properties);
 v=await current(page,s.id);expect(v.objectAppearances['REFERENCE_POINT:'+s.points[0].id].pointSymbol).toBe('PLUS');
 await (await row(page,s.destination.id,s.destination.name,'geometrical-set')).click({button:'right'});await page.getByRole('menuitem',{name:'对象',exact:true}).hover();await page.getByRole('menuitem',{name:'设为工作集合',exact:true}).click();await expect(page.locator(`[data-tree-key$="/geometrical-set:${s.destination.id}"]`)).toHaveAttribute('data-working-set','true');
 await tool(page,'参考点');const point=page.getByRole('dialog',{name:'创建参考点',exact:true});await save(point);v=await current(page,s.id);expect(v.part.datumPoints.at(-1).geometricalSetId).toBe(s.destination.id);
 await (await row(page,s.alternate.id,s.alternate.name!)).click({button:'right'});await page.getByRole('menuitem',{name:'对象',exact:true}).hover();await page.getByRole('menuitem',{name:'移入集合…',exact:true}).click();const move=page.getByRole('dialog',{name:'移入几何图形集',exact:true});await save(move);v=await current(page,s.id);expect(v.part.features.find((f:any)=>f.id===s.alternate.id).geometricalSetId).toBe(s.destination.id);
 await (await row(page,s.destination.id,s.destination.name,'geometrical-set')).dblclick();const set=page.getByRole('dialog',{name:'编辑几何图形集',exact:true});await set.getByRole('button',{name:'解散集合…',exact:true}).click();const dissolve=page.getByRole('dialog',{name:'解散几何图形集',exact:true});await dissolve.getByRole('button',{name:'选择目标集合',exact:true}).click();await choose(page,s.container.id,s.container.name,'geometrical-set');await save(dissolve);
 v=await current(page,s.id);expect(v.part.geometricalSets.some((set:any)=>set.id===s.destination.id)).toBe(false);expect(v.part.features.find((f:any)=>f.id===s.alternate.id).result.geometryKey).toBe(s.alternate.result!.geometryKey);await page.screenshot({path:info.outputPath('collection-properties.png')});
});

test('Multiple point preselection and associative surface revolution use compact pick forms',async({page},info)=>{
 await login(page);const s=await seed(page);await page.goto(`/documents/${s.id}`);await expect(page.locator('canvas')).toBeVisible();
 await choose(page,s.points[0].id,s.points[0].name,'point');await (await row(page,s.points[1].id,s.points[1].name,'point')).click({modifiers:['Control']});await tool(page,'样条');const spline=page.getByRole('dialog',{name:'样条',exact:true});await expect(spline.locator('.feature-pick-item')).toHaveCount(2);await expect(spline).toContainText('Point A');await expect(spline).toContainText('Point B');await save(spline);
 // The picked direction line supplies the rotation axis; angle stays scalar.
 await choose(page,s.generatrix.id,s.generatrix.name!);await tool(page,'旋转曲面');const revolve=page.getByRole('dialog',{name:'旋转曲面',exact:true});await expect(revolve).toContainText('Revolution profile');await expect(revolve.getByRole('button',{name:confirmCommandName})).toBeDisabled();await choose(page,s.axis.id,s.axis.name,'datum-axis');
 await save(revolve);const revolved=(await current(page,s.id)).part.features.at(-1);expect(revolved.geometry.axis.datum.entityId).toBe(s.axis.id);expect(revolved.result.domain).toBe('SHEET_SET');
 await choose(page,s.circle.id,s.circle.name!);await tool(page,'填充曲面');const fill=page.getByRole('dialog',{name:'填充曲面',exact:true});await expect(fill).toContainText('Fill boundary');await save(fill);expect((await current(page,s.id)).part.features.at(-1).geometry.inputs[0].featureId).toBe(s.circle.id);
 // Quality diagnostics are read only and dispose their transient overlay.
 await choose(page,s.sheet.id,s.sheet.name!);const before=(await current(page,s.id)).document.versionId;await tool(page,'曲率分析');const quality=page.getByRole('dialog',{name:'曲率分析',exact:true});await expect(quality.getByRole('region',{name:'曲率分析结果',exact:true})).toBeVisible();await quality.getByText('自由边',{exact:true}).click();await expect(quality).toContainText('4 条自由边');await observeViewport(page);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.curvatureOverlay.root.children.length)).toBeGreaterThan(0);await page.screenshot({path:info.outputPath('free-boundaries.png')});await quality.getByText('法向',{exact:true}).click();await page.screenshot({path:info.outputPath('surface-normals.png')});await quality.getByRole('button',{name:/完\s*成/}).click();await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.curvatureOverlay.root.children.length)).toBe(0);expect((await current(page,s.id)).document.versionId).toBe(before);
});

test.describe('Compact engineering forms at DPR 2',()=>{
 test.use({viewport:{width:1024,height:720},deviceScaleFactor:2});
 test('Loft section details preserve ordered inputs and shared correspondence edits',async({page},info)=>{
  await login(page);const s=await page.evaluate(async()=>{
   // @ts-ignore Vite module, real API and Worker.
   const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Loft form acceptance');const id=v.document.id,sections=[];
   for(const z of [0,10,20]){v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name:'Section '+z,geometry:{kind:'CIRCLE',curve:{points:[{id:'center',position:[0,0,z]}],normal:[0,0,1],xDirection:[1,0,0],radius:10+z/2},parameters:{direction:[0,0,0],origin:[0,0,0],distance:20,angle:0,start:0,end:0,tension:1}}}});sections.push(v.part!.features.at(-1)!)}
   return {id,sections,revision:v.document.versionId};
  });
  await page.goto(`/documents/${s.id}`);await expect(page.locator('canvas')).toBeVisible();await choose(page,s.sections[0].id,s.sections[0].name!);await tool(page,'曲面放样');const loft=page.getByRole('dialog',{name:'曲面放样',exact:true});
  for(const section of s.sections.slice(1))await choose(page,section.id,section.name!);
  const confirm=loft.getByRole('button',{name:confirmCommandName}),list=loft.getByRole('listbox',{name:'截面设置',exact:true});await expect(confirm).toBeEnabled();await expect(list.getByRole('option')).toHaveCount(3);await list.getByRole('option').nth(2).click();await expect(list.getByRole('option').nth(2)).toHaveAttribute('aria-selected','true');
  await loft.getByRole('button',{name:'添加对应点',exact:true}).click();await expect(confirm).toBeDisabled();await expect(loft.getByRole('button',{name:'移除对应 1',exact:true})).toBeVisible();await loft.getByRole('button',{name:'移除对应 1',exact:true}).click();await expect(confirm).toBeEnabled();
  const box=await loft.boundingBox();expect(box!.x).toBeGreaterThanOrEqual(0);expect(box!.y).toBeGreaterThanOrEqual(0);expect(box!.x+box!.width).toBeLessThanOrEqual(1024);expect(box!.y+box!.height).toBeLessThanOrEqual(720);expect(await loft.evaluate(el=>el.scrollWidth<=el.clientWidth+1)).toBe(true);
  await page.screenshot({path:info.outputPath('loft-details-dpr2.png')});await loft.getByRole('button',{name:/取\s*消/}).click();expect((await current(page,s.id)).document.versionId).toBe(s.revision);
 });
});
