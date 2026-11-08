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
 const pick=async(f:any)=>selectTree(page,f.id,f.name);
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
  const normal=await page.evaluate(()=>{const e=(window as any).__cameraEngine;e.setDisplaySettings(e.referenceVisibility,{...e.solidDisplay,vertices:false,edges:false});const sheet=[...e.solidBindings.values()].find((b:any)=>b.artifact.geometryDomain==='SHEET_SET'),wire=[...e.solidBindings.values()].find((b:any)=>b.artifact.geometryDomain==='WIRE_SET'),point=[...e.solidBindings.values()].find((b:any)=>b.artifact.geometryDomain==='POINT_SET');return {shadows:e.renderer.shadowMap.enabled,bounds:!e.groundBounds.isEmpty(),roughness:sheet.mesh.material.roughness,metalness:sheet.mesh.material.metalness,wireVisible:wire.group.children.find((o:any)=>o.userData.cadWideEdges).visible,wireColor:wire.group.children.find((o:any)=>o.userData.cadWideEdges).material.color.getHexString(),pointShape:point.group.children.find((o:any)=>o.isPoints).material.uniforms.uMarkerShape.value}});
  expect(normal).toMatchObject({shadows:true,bounds:true,roughness:.78,metalness:0,wireVisible:true,wireColor:'6745a3',pointShape:3});
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
