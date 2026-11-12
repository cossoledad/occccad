import {expect,test,type Page} from '@playwright/test';
import fs from 'node:fs';
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
async function captureFeatureNotices(page:Page){
 await page.evaluate(()=>{const w=window as any;w.__featureNotices=[];w.__featureNoticeObserver=new MutationObserver(()=>{for(const n of document.querySelectorAll('.ant-notification-notice'))if(n.textContent&&!w.__featureNotices.includes(n.textContent))w.__featureNotices.push(n.textContent)});w.__featureNoticeObserver.observe(document.body,{subtree:true,childList:true,characterData:true})});
 return ()=>page.evaluate(()=>{const w=window as any;w.__featureNoticeObserver.disconnect();return w.__featureNotices as string[]});
}

test('Extracted faces sew and close from viewport as a whole skin while source faces stay visible',async({page},info)=>{
 await login(page);
 const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Viewport whole sewn skin');const id=v.document.id;
  v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:'EDIT_SKETCH',sketchId,operations:[{type:'ADD_RECTANGLE',first:{x:-10,y:-6},second:{x:10,y:6}}]});
  v=await api.command(id,{type:'CREATE_SOLID_FEATURE',generator:'LINEAR_EXTRUDE',sketchId,operation:'ADD',length:8});
  const body=v.part!.bodies![0],sourceVersionId=v.document.versionId,stage=v.part!.features.at(-1)!.id,faceCount=v.artifacts![body.geometryKey!]!.topology.faces;
  const extracted=[];
  for(let localId=1;localId<=faceCount;localId++){
   const selection=await api.bindTopologySelection(id,{sourceVersionId,geometryKey:body.geometryKey!,kind:'FACE',localId});
   v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name:`Extracted face ${localId}`,geometry:{kind:'EXTRACT',inputs:[{kind:'TOPOLOGY',selection:{selection,sourceVersionId,sourceFeatureId:stage}}]}}});extracted.push(v.part!.features.at(-1)!);
  }
  v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name:'Sewn closed skin',geometry:{kind:'SURFACE_SEW',surface:{tolerance:.01},inputs:extracted.map(f=>({kind:'RESULT',featureId:f.id,resultId:f.result!.id}))}}});
  return {id,shell:v.part!.features.at(-1)!.id,sourceBody:body.id,extracted:extracted.map(f=>f.id)};
 });
 await page.goto(`/documents/${seed.id}`);await observeViewport(page);await geometryTab(page);
 await command(page,'封闭曲面');let d=page.getByRole('dialog',{name:'封闭曲面',exact:true});await expect(d.getByRole('button',{name:/确\s*定/})).toBeDisabled();
 const p=await worldScreen(page,[0,0,8]);
 await expect.poll(()=>page.evaluate(({x,y})=>{const e=(window as any).__cameraEngine,r=e.renderer.domElement.getBoundingClientRect(),hit=e.hitTest(x-r.x,y-r.y);return {kind:hit?.kind,feature:hit?.result?.featureId}},p)).toEqual({kind:'feature',feature:seed.shell});
 const visible=await page.evaluate(()=>[...(window as any).__cameraEngine.solidBindings.values()].filter((b:any)=>b.group.visible).length);expect(visible).toBeGreaterThanOrEqual(8);
 await page.mouse.move(p.x,p.y);await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine;return {kind:e.preselected?.kind,feature:e.preselected?.result?.featureId,overlays:e.preselectedOverlays.length}})).toEqual({kind:'feature',feature:seed.shell,overlays:2});
 await page.screenshot({path:info.outputPath('whole-skin-viewport-hover.png')});await page.mouse.click(p.x,p.y);
 await ready(page,d);await expect(d).toContainText('Sewn closed skin');const notices=await captureFeatureNotices(page);
 await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);expect(await notices()).toEqual([]);
 let v=await document(page,seed.id);const closed=v.part.features.at(-1);expect(closed.type).toBe('CLOSE_SURFACE');expect(closed.geometry.inputs).toEqual([{kind:'RESULT',featureId:seed.shell,resultId:'result'}]);
 const created=v.part.bodies.find((b:any)=>b.createdByFeatureId===closed.id);expect(v.artifacts[created.geometryKey].volume).toBeCloseTo(1920,5);
 // Leaving whole-result selection restores normal sub-element commands.
 await command(page,'提取');d=page.getByRole('dialog',{name:'提取',exact:true});await clickWorld(page,[0,0,8]);await ready(page,d);
 await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);v=await document(page,seed.id);const extracted=v.part.features.at(-1);expect(extracted.geometry.inputs[0].kind).toBe('TOPOLOGY');expect(extracted.geometry.inputs[0].selection.selection.expectedType).toBe('FACE');expect(v.artifacts[extracted.result.geometryKey].topology.faces).toBe(1);
});

test('Curve preview remains visible inside a solid and surface confirmation has no transient failure',async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 await login(page);
 const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Curve preview and commit lifecycle');const id=v.document.id;
  v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=v.part!.features.at(-1)!.id;
  await api.command(id,{type:'EDIT_SKETCH',sketchId,operations:[{type:'ADD_RECTANGLE',first:{x:-15,y:-15},second:{x:15,y:15}}]});
  v=await api.command(id,{type:'CREATE_SOLID_FEATURE',generator:'LINEAR_EXTRUDE',sketchId,operation:'ADD',length:20});
  const points=[];for(const [i,coordinates] of [[0,[3,0,10]],[1,[12,0,10]]] as const){v=await api.command(id,{type:'CREATE_REFERENCE_POINT',name:`Preview point ${i}`,referenceConstruction:{mode:'POINT_COORDINATES',coordinates}});points.push(v.part!.datumPoints!.at(-1)!)}
  return {id,points};
 });
 await page.goto(`/documents/${seed.id}`);await observeViewport(page);await geometryTab(page);
 await command(page,'直线');let d=page.getByRole('dialog',{name:'直线',exact:true});
 for(const p of seed.points)await selectTree(page,p.id,p.name,'point');
 await ready(page,d);await observeViewport(page);
 const preview=await page.evaluate(()=>{const e=(window as any).__cameraEngine,items:any[]=[];e.commandPreview.traverse((o:any)=>{if(o.material)items.push({name:o.name,depthTest:o.material.depthTest,width:o.material.linewidth,depthWrite:o.material.depthWrite})});return items});
 expect(preview.some(p=>p.name==='feature-preview-curve'&&p.width>=2&&!p.depthTest&&!p.depthWrite)).toBe(true);
 const shown=await page.locator('canvas').screenshot();await page.screenshot({path:info.outputPath('curve-preview-through-solid.png')});
 await page.evaluate(()=>{const e=(window as any).__cameraEngine;e.commandPreview.visible=false;e.invalidate()});
 const hidden=await page.locator('canvas').screenshot();
 const pixels=await page.evaluate(async({a,b})=>{
  const read=async(s:string)=>{const img=new Image();img.src='data:image/png;base64,'+s;await img.decode();const c=document.createElement('canvas');c.width=img.width;c.height=img.height;const ctx=c.getContext('2d')!;ctx.drawImage(img,0,0);return ctx.getImageData(0,0,c.width,c.height).data};
  const x=await read(a),y=await read(b);let changed=0;for(let i=0;i<x.length;i+=4)if(Math.abs(x[i]-y[i])+Math.abs(x[i+1]-y[i+1])+Math.abs(x[i+2]-y[i+2])>40)changed++;return changed;
 },{a:shown.toString('base64'),b:hidden.toString('base64')});expect(pixels).toBeGreaterThan(30);
 await page.evaluate(()=>{const e=(window as any).__cameraEngine;e.commandPreview.visible=true;e.invalidate()});
 const notices=await captureFeatureNotices(page);
 await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);
 const v=await document(page,seed.id),line=v.part.features.at(-1);await selectTree(page,line.id,line.name);
 await command(page,'旋转曲面');d=page.getByRole('dialog',{name:'旋转曲面',exact:true});
 await treeNavigator(page)('Y Axis','/origin/axis:axis-system-default/y');
 await page.locator('[role=treeitem][data-tree-key$="/origin/axis:axis-system-default/y"]').click();
 await ready(page,d);await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);
 expect((await document(page,seed.id)).part.features.at(-1).result.domain).toBe('SHEET_SET');
 expect(await notices()).toEqual([]);expect(errors).toEqual([]);
});
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
 // A compatible preselection seeds the first role; no sample geometry is invented.
 await selectTree(page,seed.points.Start,'Start','point');await command(page,'直线');const line=page.getByRole('dialog',{name:'直线',exact:true});await expect(line.getByRole('button',{name:/确\s*定/})).toBeDisabled();await expect(line.getByText('未选择',{exact:true})).toHaveCount(1);await expect(line).toContainText('Start');
 await clickWorld(page,[32,3,0]);await ready(page,line);await expect(line).not.toContainText('更多');await page.screenshot({path:info.outputPath('line-references.png')});
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
 await expect(rows).toHaveCount(1);await expect(page.locator('[data-feature-preview]')).toHaveCount(0);await expect(spline.getByRole('button',{name:/确\s*定/})).toBeDisabled();
 await expect(rows).toHaveCount(1);await clickWorld(page,[32,3,0]);await selectTree(page,seed.points.Third,'Third','point');await expect(rows).toHaveCount(3);await ready(page,spline);
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
 await command(page,'扫掠');const dialog=page.getByRole('dialog',{name:/创建.*扫掠/});await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeDisabled();await expect(dialog.getByRole('button',{name:'移除轮廓',exact:true})).toHaveCount(1);await page.locator(`[role=treeitem][data-tree-key$="/sketch:${seed.sketch}"]`).click();await dialog.getByRole('button',{name:'选择路径曲线',exact:true}).click();
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

test('Engineering surfaces: empty dialogs, viewport picks, ordered loft, sewing and close',async({page},info)=>{
 test.setTimeout(300_000);const errors:string[]=[];
 page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(/Shader Error|VALIDATE_STATUS|THREE.WebGLProgram/.test(m.text()))errors.push(m.text())});
 await login(page);
 const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Engineering surfaces C acceptance');const id=v.document.id;
  const p={direction:[0,0,1],origin:[0,0,0],distance:20,angle:0,start:.5,end:1,tension:1};
  const create=async(name:string,geometry:any)=>{v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name,geometry:{parameters:{...p},...geometry}}});return v.part!.features.at(-1)!};
  const profile=await create('C Profile',{kind:'LINE',curve:{points:[{id:'a',position:[0,0,0]},{id:'b',position:[30,0,0]}]}});
  const rail=await create('C Rail',{kind:'LINE',curve:{points:[{id:'a',position:[0,0,0]},{id:'b',position:[0,0,20]}]}});
  v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY',sketchOwnerKind:'GEOMETRICAL_SET'});const sketch=v.part!.features.at(-1)!;
  v=await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch.id,operations:[{type:'ADD_RECTANGLE',first:{x:40,y:0},second:{x:50,y:10}}]});
  const ids=v.part!.features.find(f=>f.id===sketch.id)!.sketch!.entities.map(e=>e.id);
  const bottom=await create('C Bottom',{kind:'EXTRACT',inputs:[{kind:'SKETCH',featureId:sketch.id,entityIds:ids}]});
  const top=await create('C Top',{kind:'TRANSLATE',inputs:[{kind:'RESULT',featureId:bottom.id,resultId:'result'}],direction:{kind:'RESULT',featureId:rail.id,resultId:'result'},parameters:{...p,distance:20}});
  const cutLine=await create('C Cut line',{kind:'LINE',curve:{points:[{id:'a',position:[15,-10,0]},{id:'b',position:[15,10,0]}]}});
  const cutter=await create('C Cutter',{kind:'SURFACE_EXTRUDE',inputs:[{kind:'RESULT',featureId:cutLine.id,resultId:'result'}],direction:{kind:'RESULT',featureId:rail.id,resultId:'result'}});
  v=await api.command(id,{type:'CREATE_REFERENCE_POINT',name:'C Keep point',referenceConstruction:{mode:'POINT_COORDINATES',coordinates:[5,0,5]}});const keep=v.part!.datumPoints!.at(-1)!;
  const point=await create('C Curve point',{kind:'POINT_ON_CURVE',inputs:[{kind:'RESULT',featureId:profile.id,resultId:'result'}]});
  return {id,profile,rail,bottom,top,cutter,keep,point};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();await geometryTab(page);
 const notices=await captureFeatureNotices(page),pick=async(f:any)=>selectTree(page,f.id,f.name);
 const open=async(name:string)=>{const before=(await document(page,seed.id)).document.versionId;await command(page,name);const d=page.getByRole('dialog',{name,exact:true});await expect(d.getByRole('button',{name:/确\s*定/})).toBeDisabled();await expect(d).not.toContainText('更多');expect((await document(page,seed.id)).document.versionId).toBe(before);return d};
 const confirm=async(d:ReturnType<Page['getByRole']>)=>{await ready(page,d);await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);const v=await document(page,seed.id),f=v.part.features.at(-1);expect(f.evaluationStatus).toBeFalsy();return f};
 let d=await open('曲面扫掠');await clickWorld(page,[10,0,0]);await pick(seed.rail);const sweep=await confirm(d);
 d=await open('曲面放样');await pick(seed.bottom);await expect(d.getByRole('button',{name:/确\s*定/})).toBeDisabled();await pick(seed.top);await ready(page,d);
 await d.getByRole('button',{name:'上移截面 2',exact:true}).click();await ready(page,d);await d.getByRole('button',{name:'下移截面 1',exact:true}).click();await ready(page,d);await page.screenshot({path:info.outputPath('engineering-loft-list.png')});const walls=await confirm(d);
 const caps=[];for(const source of [seed.bottom,seed.top]){d=await open('填充曲面');await pick(source);caps.push(await confirm(d))}
 d=await open('连接曲面');for(const f of [walls,...caps])await pick(f);const join=await confirm(d);expect(join.geometry.inputs).toHaveLength(3);
 d=await open('缝合曲面');for(const f of [walls,...caps])await pick(f);const shell=await confirm(d);
 d=await open('封闭曲面');await pick(shell);const closed=await confirm(d);expect(closed.type).toBe('CLOSE_SURFACE');
 let current=await document(page,seed.id);expect(Object.values(current.artifacts).some((a:any)=>a.geometryDomain==='SOLID_SET'&&Math.abs(a.volume-2000)<1e-4)).toBe(true);
 d=await open('延长曲面');await pick(sweep);await clickWorld(page,[10,0,20]);await page.screenshot({path:info.outputPath('engineering-extension-boundary.png')});const extended=await confirm(d);expect(extended.geometry.inputs[1].kind).toBe('TOPOLOGY');
 d=await open('偏移曲面');await pick(sweep);const offset=await confirm(d);expect(offset.geometry.parameters.distance).toBe(2);
 d=await open('裁剪曲面');await pick(sweep);await pick(seed.cutter);await selectTree(page,seed.keep.id,seed.keep.name,'point');await ready(page,d);await page.screenshot({path:info.outputPath('engineering-trim-keep-point.png')});const trim=await confirm(d);expect(trim.geometry.keepPoint.kind).toBe('DATUM');
 // Both standalone points and datum points have an outlined cross; body corners
 // retain round markers, and engineering wires remain visible with edges off.
 await observeViewport(page);const display=await page.evaluate(()=>{
  const e=(window as any).__cameraEngine;e.selectMany([],true);e.preselect(null,true);const data:any[]=[];
  for(const b of e.solidBindings.values()){const points=b.group.children.filter((o:any)=>o.isPoints);data.push({domain:b.artifact.geometryDomain,pointShapes:points.map((o:any)=>o.material.uniforms.uMarkerShape.value),color:b.mesh.material.color?.getHexString(),cast:b.mesh.castShadow,receive:b.mesh.receiveShadow,side:b.mesh.material.side,wire:b.group.children.find((o:any)=>o.userData.cadWideEdges)?.material.linewidth})}
  const reference:any[]=[];e.scene.traverse((o:any)=>{if(o.userData.kind==='reference-point')reference.push(o.material.uniforms.uMarkerShape.value)});return {data,reference};
 });
 expect(display.reference.every(v=>v===3)).toBe(true);expect(display.data.filter(v=>v.domain==='POINT_SET').every(v=>v.pointShapes.includes(3))).toBe(true);
 expect(display.data.filter(v=>v.domain==='SHEET_SET').every(v=>v.cast&&v.receive&&v.side===2)).toBe(true);expect(display.data.filter(v=>v.domain==='WIRE_SET').every(v=>v.wire>=2)).toBe(true);
 await page.screenshot({path:info.outputPath('engineering-surfaces-display.png')});
 expect(await notices()).toEqual([]);
 const before=(await document(page,seed.id)).document.versionId;d=await open('封闭曲面');await pick(sweep);await expect(page.locator('.ant-notification')).toBeVisible();await expect(d.getByRole('button',{name:/确\s*定/})).toBeDisabled();await d.getByRole('button',{name:/取\s*消/}).click();expect((await document(page,seed.id)).document.versionId).toBe(before);
 await page.reload();await expect(page.locator('canvas')).toBeVisible();current=await document(page,seed.id);expect(current.part.features.find((f:any)=>f.id===trim.id).result).toBeTruthy();expect(errors).toEqual([]);
});

test('Engineering surface display: curved sheets, point glyphs, dark theme and real thumbnails',async({browser},info)=>{
 const context=await browser.newContext({baseURL:'http://127.0.0.1:5175',viewport:{width:1440,height:1000},deviceScaleFactor:2});
 try{
  const page=await context.newPage(),errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(/Shader Error|VALIDATE_STATUS|THREE.WebGLProgram/.test(m.text()))errors.push(m.text())});await login(page);
  const seed=await page.evaluate(async()=>{
   const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Engineering curved sheet and points');const id=v.document.id;
   const parameters={direction:[0,0,1],origin:[0,0,0],distance:10,angle:0,start:.5,end:1,tension:1};
   const create=async(name:string,geometry:any)=>{v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name,geometry:{parameters:{...parameters},...geometry}}});const f=v.part!.features.at(-1)!;if(!f.result)throw new Error(f.diagnostic??'Missing geometry');return f};
   const circles=[];for(const [z,radius] of [[0,5],[10,10],[20,5]])circles.push(await create('Section '+z,{kind:'CIRCLE',curve:{points:[{id:'center',position:[0,0,z]}],normal:[0,0,1],xDirection:[1,0,0],radius}}));
   const sheet=await create('Curved surface',{kind:'SURFACE_LOFT',inputs:circles.map(f=>({kind:'RESULT',featureId:f.id,resultId:'result'})),surface:{ruled:false,tolerance:.000001}});
   for(const f of circles)v=await api.command(id,{type:'SET_DEFINITION_VISIBILITY',targetKind:'FEATURE',targetId:f.id,visible:false});
   const curve=await create('Engineering curve',{kind:'LINE',curve:{points:[{id:'a',position:[18,-8,10]},{id:'b',position:[18,8,10]}]}});
   const point=await create('Independent point',{kind:'POINT_ON_CURVE',inputs:[{kind:'RESULT',featureId:curve.id,resultId:'result'}]});
   v=await api.command(id,{type:'CREATE_REFERENCE_POINT',name:'Reference cross',referenceConstruction:{mode:'POINT_COORDINATES',coordinates:[-18,0,10]}});
   return {id,sheet:sheet.id,curve:curve.id,point:point.id,revision:v.document.versionId};
  });
  await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();await geometryTab(page);await observeViewport(page);
  await expect.poll(()=>page.evaluate(()=>[...(window as any).__cameraEngine.solidBindings.values()].some((b:any)=>b.artifact.geometryDomain==='SHEET_SET'))).toBe(true);
  await treeNavigator(page)('Curved surface',`/feature:${seed.sheet}`);
  const sheetRow=page.locator(`[role=treeitem][data-tree-key$="/feature:${seed.sheet}"]`);
  await sheetRow.hover();
  await expect.poll(()=>page.evaluate(id=>{const e=(window as any).__cameraEngine,b=[...e.solidBindings.values()].find((v:any)=>v.context.result?.featureId===id);return e.preselectedOverlays.length>0&&e.preselectedOverlays.every((o:any)=>o.parent===b.group)},seed.sheet)).toBe(true);
  await sheetRow.getByText('Curved surface',{exact:true}).click();await page.mouse.move(1100,250);
  await expect.poll(()=>page.evaluate(id=>{const e=(window as any).__cameraEngine,b=[...e.solidBindings.values()].find((v:any)=>v.context.result?.featureId===id);return e.selectedOverlays.length>0&&e.selectedOverlays.every((o:any)=>o.parent===b.group)},seed.sheet)).toBe(true);
  await expect(page.locator('.property-list')).toContainText('曲面放样');await expect(page.locator('.property-list')).toContainText('曲面集');
  await page.screenshot({path:info.outputPath('engineering-surface-tree-selection.png')});
  await selectTree(page,seed.curve,'Engineering curve');
  const row=page.locator(`[role=treeitem][data-tree-key$="/feature:${seed.curve}"]`);
  await row.click({button:'right'});await page.getByRole('menuitem',{name:'隐藏',exact:true}).click();
  await expect.poll(()=>page.evaluate(id=>[...(window as any).__cameraEngine.solidBindings.values()].find((b:any)=>b.context.result?.featureId===id)?.group.visible,seed.curve)).toBe(false);
  await row.click({button:'right'});await page.getByRole('menuitem',{name:'显示',exact:true}).click();
  await expect.poll(()=>page.evaluate(id=>[...(window as any).__cameraEngine.solidBindings.values()].find((b:any)=>b.context.result?.featureId===id)?.group.visible,seed.curve)).toBe(true);
  const stableRevision=(await document(page,seed.id)).document.versionId;await page.mouse.click(1100,250);
  const normal=await page.evaluate(()=>{const e=(window as any).__cameraEngine;e.setDisplaySettings(e.referenceVisibility,{...e.solidDisplay,vertices:false,edges:false});const sheet=[...e.solidBindings.values()].find((b:any)=>b.artifact.geometryDomain==='SHEET_SET'),wire=[...e.solidBindings.values()].find((b:any)=>b.artifact.geometryDomain==='WIRE_SET'),point=[...e.solidBindings.values()].find((b:any)=>b.artifact.geometryDomain==='POINT_SET');return {shadows:e.renderer.shadowMap.enabled,bounds:!e.groundBounds.isEmpty(),roughness:sheet.mesh.material.roughness,metalness:sheet.mesh.material.metalness,wireVisible:wire.group.children.find((o:any)=>o.userData.cadWideEdges).visible,wireColor:wire.group.children.find((o:any)=>o.userData.cadWideEdges).material.color.getHexString(),pointShape:point.group.children.find((o:any)=>o.isPoints).material.uniforms.uMarkerShape.value,pointColor:point.group.children.find((o:any)=>o.isPoints).material.uniforms.uColor.value.getHexString()}});
  expect(normal).toMatchObject({shadows:true,bounds:true,roughness:.78,metalness:0,wireVisible:true,wireColor:'6745a3',pointShape:3,pointColor:'ffffff'});
  await page.mouse.move(1430,990);await page.screenshot({path:info.outputPath('engineering-curved-light-dpr2.png')});
  await clickWorld(page,[18,0,10]);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.selected[0]?.kind)).toBe('vertex');await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.selectedOverlays.at(-1)?.material.uniforms.uMarkerShape.value)).toBe(3);
  await page.getByRole('button',{name:'选项',exact:true}).click();const options=page.getByRole('dialog',{name:'选项',exact:true});await options.getByRole('menuitem',{name:'场景',exact:true}).click();await options.getByRole('button',{name:'深色工程',exact:true}).click();await options.getByRole('button',{name:'关闭',exact:true}).click();
  await expect(page.locator('.workbench-content')).not.toHaveClass(/light-scene/);
  await page.evaluate(()=>{const e=(window as any).__cameraEngine;e.selectMany([],true);e.preselect(null,true);e.setSceneSettings({...e.sceneSettings,projection:'ORTHOGRAPHIC'});e.camera.position.sub(e.navigation.target).applyAxisAngle(e.camera.up,Math.PI).add(e.navigation.target);e.camera.lookAt(e.navigation.target);e.navigation.syncCamera(false);e.invalidate()});
  await page.mouse.move(1430,990);await page.screenshot({path:info.outputPath('engineering-curved-dark-dpr2.png')});
  const dark=await page.evaluate(()=>{const e=(window as any).__cameraEngine;return {dpr:e.renderer.getPixelRatio(),wire:[...e.solidBindings.values()].find((b:any)=>b.artifact.geometryDomain==='WIRE_SET').group.children.find((o:any)=>o.userData.cadWideEdges).material.color.getHexString()}});expect(dark).toEqual({dpr:2,wire:'a99bff'});
  await expect.poll(async()=>{const r=await page.request.get(`/api/documents/${seed.id}/preview`);expect(r.ok()).toBe(true);return r.headers()['x-occccad-thumbnail-state']},{timeout:30_000}).toBe('ready');
  const thumbnail=await page.request.get(`/api/documents/${seed.id}/preview`);fs.writeFileSync(info.outputPath('engineering-curved-thumbnail.png'),await thumbnail.body());
  expect((await document(page,seed.id)).document.versionId).toBe(stableRevision);expect(errors).toEqual([]);
 }finally{await context.close()}
});

test('Shape control D: empty Blend, supported G2, exact curvature and disposable graphics',async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(/Shader Error|VALIDATE_STATUS|THREE.WebGLProgram/.test(m.text()))errors.push(m.text())});await login(page);
 const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Shape control D acceptance');const id=v.document.id,p={direction:[0,0,0],origin:[0,0,0],distance:0,angle:0,start:0,end:1,tension:1};
  const create=async(name:string,kind:string,curve:any)=>{v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name,geometry:{kind,curve,parameters:p}}});return v.part!.features.at(-1)!};
  const a=await create('D Boundary A','LINE',{points:[{id:'a',position:[0,0,0]},{id:'b',position:[30,0,0]}]});
  const b=await create('D Boundary B','LINE',{points:[{id:'a',position:[0,20,8]},{id:'b',position:[30,20,8]}]});
  const circle=await create('D Circle','CIRCLE',{points:[{id:'center',position:[-25,0,0]}],normal:[0,0,1],xDirection:[1,0,0],radius:10});return {id,a,b,circle};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();await geometryTab(page);await selectTree(page,seed.a.id,seed.a.name!);await command(page,'桥接曲面');let d=page.getByRole('dialog',{name:'桥接曲面',exact:true});
 await expect(d.getByRole('button',{name:/确\s*定/})).toBeDisabled();await expect(d.getByText('未选择',{exact:true})).toHaveCount(1);await expect(d).toContainText(seed.a.name!);await selectTree(page,seed.b.id,seed.b.name!);await ready(page,d);
 const boundaryList=d.getByRole('listbox',{name:'边界设置',exact:true});await boundaryList.getByRole('option').first().click();const boundary=boundaryList.locator('..').locator('.command-item-detail');await boundary.getByText('G2',{exact:true}).click();await expect(d.getByRole('button',{name:/确\s*定/})).toBeDisabled();await boundary.getByRole('button',{name:'选择支撑',exact:true}).click();await page.getByRole('treeitem',{name:'XY Plane',exact:true}).click();await ready(page,d);await expect(d).not.toContainText('更多');await page.screenshot({path:info.outputPath('shape-control-blend.png')});
 await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);const v=await document(page,seed.id),blend=v.part.features.at(-1);expect(blend.geometry.kind).toBe('SURFACE_BLEND');expect(blend.geometry.surface.boundaries[0].support.kind).toBe('PLANE');const revision=v.document.versionId;
 await command(page,'曲率分析');d=page.getByRole('dialog',{name:'曲率分析',exact:true});await selectTree(page,blend.id,blend.name);await expect(d.getByRole('region',{name:'曲率分析结果'})).toBeVisible();await expect(d).toContainText('256 个有效采样');await observeViewport(page);
 await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.curvatureOverlay.root.children.length)).toBe(1);await d.getByText('密',{exact:true}).click();await expect(d).toContainText('1024 个有效采样');await page.screenshot({path:info.outputPath('surface-curvature-samples.png')});await d.getByText('高斯',{exact:true}).click();await expect(d).toContainText('1/mm²');await d.getByRole('button',{name:/完\s*成/}).click();
 await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.curvatureOverlay.root.children.length)).toBe(0);expect((await document(page,seed.id)).document.versionId).toBe(revision);
 await command(page,'曲率分析');d=page.getByRole('dialog',{name:'曲率分析',exact:true});await selectTree(page,seed.circle.id,seed.circle.name!);await expect(d.getByRole('region',{name:'曲率分析结果'})).toBeVisible();await expect(d).toContainText('0.1 1/mm');await page.screenshot({path:info.outputPath('curve-curvature-comb.png')});
 await d.getByRole('button',{name:/完\s*成/}).click();await page.reload();await expect(page.locator('canvas')).toBeVisible();await observeViewport(page);expect((await document(page,seed.id)).document.versionId).toBe(revision);expect(errors).toEqual([]);
});

test('Shape control D: associative spine, orientation guide and manual point correspondence',async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Spine and correspondence D acceptance');const id=v.document.id,p={direction:[0,0,0],origin:[0,0,0],distance:0,angle:0,start:0,end:1,tension:1};
  const line=async(name:string,a:number[],b:number[])=>{v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name,geometry:{kind:'LINE',curve:{points:[{id:'a',position:a},{id:'b',position:b}]},parameters:p}}});return v.part!.features.at(-1)!};
  const a=await line('D Section A',[0,0,0],[30,0,0]),b=await line('D Section B',[0,0,30],[40,0,30]),spine=await line('D Spine',[0,0,0],[0,0,30]),guide=await line('D Orientation guide',[30,0,0],[40,0,30]);
  const points=[];for(const [name,coordinates] of [['D Station A',[0,0,0]],['D Station B',[0,0,30]],['D Coupling A',[12,0,0]],['D Coupling B',[25,0,30]]] as Array<[string,number[]]>){v=await api.command(id,{type:'CREATE_REFERENCE_POINT',name,referenceConstruction:{mode:'POINT_COORDINATES',coordinates}});points.push(v.part!.datumPoints!.at(-1)!)}return {id,a,b,spine,guide,points};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();await geometryTab(page);await command(page,'曲面放样');const d=page.getByRole('dialog',{name:'曲面放样',exact:true});await selectTree(page,seed.a.id,seed.a.name!);await selectTree(page,seed.b.id,seed.b.name!);await ready(page,d);
 await d.getByRole('button',{name:'选择脊线',exact:true}).click();await selectTree(page,seed.spine.id,seed.spine.name!);await expect(d.getByRole('button',{name:/确\s*定/})).toBeDisabled();for(const point of seed.points.slice(0,2))await selectTree(page,point.id,point.name!,'point');await ready(page,d);
 await d.getByRole('button',{name:'选择导向曲线',exact:true}).click();await selectTree(page,seed.guide.id,seed.guide.name!);await ready(page,d);await d.getByRole('button',{name:'添加对应点',exact:true}).click();await expect(d.getByRole('button',{name:/确\s*定/})).toBeDisabled();await d.getByRole('listbox',{name:'截面设置',exact:true}).getByRole('option').filter({hasText:'D Section A'}).click();await d.getByRole('button',{name:'选择对应点 1',exact:true}).click();for(const point of seed.points.slice(2))await selectTree(page,point.id,point.name!,'point');await ready(page,d);await page.screenshot({path:info.outputPath('spine-manual-correspondence.png')});
 await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);const v=await document(page,seed.id),feature=v.part.features.at(-1),controls=feature.geometry.surface.sections;expect(feature.geometry.surface.spine.featureId).toBe(seed.spine.id);expect(feature.geometry.surface.guide.featureId).toBe(seed.guide.id);expect(controls[0].couplings[0].id).toBe(controls[1].couplings[0].id);expect(controls.every((c:any)=>c.stationPoint?.datum&&c.couplings[0].point?.datum)).toBeTruthy();
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await document(page,seed.id)).part.features.some((f:any)=>f.id===feature.id)).toBeFalsy();await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>(await document(page,seed.id)).part.features.some((f:any)=>f.id===feature.id)).toBeTruthy();await page.reload();await expect(page.locator('canvas')).toBeVisible();expect((await document(page,seed.id)).part.features.at(-1).geometry.surface.sections).toEqual(controls);expect(errors).toEqual([]);
});

test('Shape control D: exact border guide follows an upstream rail',async({page},info)=>{
 await login(page);const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Border guide D acceptance');const id=v.document.id,p={direction:[0,0,0],origin:[0,0,0],distance:0,angle:0,start:0,end:1,tension:1};
  const create=async(name:string,kind:string,curve:any)=>{v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name,geometry:{kind,curve,parameters:p}}});return v.part!.features.at(-1)!};
  const profile=await create('D Linear profile','LINE',{points:[{id:'a',position:[0,0,0]},{id:'b',position:[10,0,0]}]}),spine=await create('D Straight spine','LINE',{points:[{id:'a',position:[0,0,0]},{id:'b',position:[0,0,20]}]});
  const guide=await create('D Border rail','SPLINE',{points:[{id:'a',position:[10,0,0]},{id:'b',position:[15,4,5]},{id:'c',position:[8,4,15]},{id:'d',position:[20,0,20]}],degree:3,knots:[0,1],multiplicities:[4,4],weights:[1,1,1,1]});return {id,profile,spine,guide};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();await geometryTab(page);await command(page,'曲面扫掠');const d=page.getByRole('dialog',{name:'曲面扫掠',exact:true});await selectTree(page,seed.profile.id,seed.profile.name!);await selectTree(page,seed.spine.id,seed.spine.name!);await ready(page,d);
 await d.getByRole('button',{name:'选择导向曲线',exact:true}).click();await selectTree(page,seed.guide.id,seed.guide.name!);await ready(page,d);await d.getByText('贴合边界',{exact:true}).click();await ready(page,d);await page.screenshot({path:info.outputPath('exact-border-guide.png')});await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);
 const before=await document(page,seed.id),feature=before.part.features.at(-1);expect(feature.geometry.surface.frameMode).toBe('GUIDE_BORDER');expect(feature.geometry.surface.guide.featureId).toBe(seed.guide.id);
 await page.evaluate(async({id,guide})=>{const {restApi:api}=await import('/src/api.ts');const view=await api.getDocument(id),f=view.part!.features.find(f=>f.id===guide)!,find=(n:any):any=>n.entityId===guide?n:(n.children??[]).map(find).find(Boolean);const node=find(view.structureTree);f.geometry!.curve!.points[1].position[0]=18;await api.command(id,{type:'EDIT_FEATURE',targetId:guide,expectedFeatureDigest:node.definitionDigest,feature:f})},{id:seed.id,guide:seed.guide.id});
 await expect.poll(async()=>(await document(page,seed.id)).part.features.at(-1).result.geometryKey).not.toBe(feature.result.geometryKey);await page.reload();await expect(page.locator('canvas')).toBeVisible();const after=await document(page,seed.id);expect(after.part.features.at(-1).evaluationStatus).toBeFalsy();expect(after.part.features.at(-1).geometry.surface.guide.featureId).toBe(seed.guide.id);
});

test('Shape control D: cancelling a pending topology pick drops its late result',async({page})=>{
 await login(page);const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','Curvature pick cancellation acceptance');const id=v.document.id;
  v=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name:'Cancellation circle',geometry:{kind:'CIRCLE',curve:{points:[{id:'center',position:[0,0,0]}],normal:[0,0,1],xDirection:[1,0,0],radius:10},parameters:{direction:[0,0,0],origin:[0,0,0],distance:0,angle:0,start:0,end:1,tension:1}}}});return {id,circle:v.part!.features.at(-1)!};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();await geometryTab(page);await command(page,'曲率分析');const d=page.getByRole('dialog',{name:'曲率分析',exact:true});await d.getByText('曲线',{exact:true}).click();
 let release!:()=>void,entered=false;const gate=new Promise<void>(resolve=>release=resolve),kinds:string[]=[];
 page.on('request',request=>{if(request.url().endsWith('/shape-quality'))kinds.push(request.postDataJSON().input.kind)});
 await page.route('**/persistent-selections/bind',async route=>{entered=true;await gate;await route.continue().catch(()=>{})});
 try{
  await clickWorld(page,[10,0,0]);await expect.poll(()=>entered).toBeTruthy();await expect(d.getByRole('status')).toBeVisible();await page.keyboard.press('Escape');await expect(d).toBeVisible();await expect(d.getByRole('status')).toHaveCount(0);
  await d.getByRole('button',{name:'选择对象',exact:true}).click();await selectTree(page,seed.circle.id,seed.circle.name!);await expect(d.getByRole('region',{name:'曲率分析结果'})).toBeVisible();await expect(d.getByRole('status')).toHaveCount(0);expect(kinds).toEqual(['RESULT']);
  const response=page.waitForResponse(r=>r.url().endsWith('/persistent-selections/bind'));release();expect((await response).ok()).toBeTruthy();await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));expect(kinds).toEqual(['RESULT']);
  await d.getByRole('button',{name:/完\s*成/}).click();
 }finally{release();await page.unroute('**/persistent-selections/bind')}
});
