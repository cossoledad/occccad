import {test,expect,type Page} from '@playwright/test';
import {worldScreen,observeViewport} from './camera-helpers';
async function command(page:Page,name:string){await page.getByRole('button',{name:'搜索工具',exact:true}).click();await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);await page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp(`^${name}$`)})}).click()}
async function select(page:Page,label:string,text:string){await page.getByRole('combobox',{name:label,exact:true}).click();await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText(text,{exact:true}).click()}
async function current(page:Page,id:string){return (await page.request.get(`/api/documents/${id}`)).json()}
test('layout publications drive plate Hole through UI references and accepted update',async({page},info)=>{
 test.setTimeout(600_000);page.setDefaultTimeout(20_000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();await page.goto('/');
 const seed=await page.evaluate(async()=>{
  // @ts-ignore real Vite API
  const {restApi:api}=await import('/src/api.ts');
  let layout=await api.createDocument('PART','Layout UI acceptance');const l=layout.document.id;
  layout=await api.command(l,{type:'CREATE_PARAMETER',name:'pitch',value:120,unit:'mm'});const pitch=layout.part!.parameters!.find(p=>p.displayAlias==='pitch')!.parameterId;
  layout=await api.command(l,{type:'CREATE_SKETCH',plane:'XY'});const sketch=layout.part!.features.at(-1)!.id;
  for(let i=0;i<4;i++){const x=i%2?60:-60,y=i<2?-30:30;
   layout=await api.command(l,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_ENTITY',entity:{id:`source-${i}`,kind:'POINT',role:'CONSTRUCTION',point:{x,y}}},{type:'ADD_CONSTRAINT',parameterSource:'pitch/2',constraint:{id:`x-${i}`,kind:'HORIZONTAL_DISTANCE',value:60,unit:'mm',references:x<0?[{target:'ENTITY',entityId:`source-${i}`,subElement:'POINT'},{target:'SKETCH_ORIGIN',subElement:'POINT'}]:[{target:'SKETCH_ORIGIN',subElement:'POINT'},{target:'ENTITY',entityId:`source-${i}`,subElement:'POINT'}]}},{type:'ADD_CONSTRAINT',constraint:{id:`y-${i}`,kind:'VERTICAL_DISTANCE',value:30,unit:'mm',references:y<0?[{target:'ENTITY',entityId:`source-${i}`,subElement:'POINT'},{target:'SKETCH_ORIGIN',subElement:'POINT'}]:[{target:'SKETCH_ORIGIN',subElement:'POINT'},{target:'ENTITY',entityId:`source-${i}`,subElement:'POINT'}]}}]});
   layout=await api.command(l,{type:'CREATE_PUBLICATION',name:`Position.${i+1}`,publicationType:'POINT',targetKind:'SKETCH_ENTITY',sketchId:sketch,targetId:`source-${i}`});
  }
  layout=await api.command(l,{type:'CREATE_PUBLICATION',name:'Mounting plane',publicationType:'PLANE',targetKind:'PLANE',targetId:'datum-xy'});
  layout=await api.command(l,{type:'CREATE_PUBLICATION',name:'Hole spacing',publicationType:'PARAMETER',targetKind:'PARAMETER',targetId:pitch});
  layout=await api.command(l,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_ENTITY',entity:{id:'repair-point',kind:'POINT',role:'CONSTRUCTION',point:{x:0,y:0}}}]});
  layout=await api.command(l,{type:'CREATE_PUBLICATION',name:'Repair position',publicationType:'POINT',targetKind:'SKETCH_ENTITY',sketchId:sketch,targetId:'repair-point'});

  const plates=[];
  for(const name of ['Bottom UI','Cover UI']){let v=await api.createDocument('PART',name);const id=v.document.id;
   v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const profile=v.part!.features.at(-1)!.id;
   v=await api.command(id,{type:'EDIT_SKETCH',sketchId:profile,operations:[{type:'ADD_RECTANGLE',first:{x:-100,y:-70},second:{x:100,y:70}}]});
   v=await api.command(id,{type:'CREATE_SOLID_FEATURE',generator:'LINEAR_EXTRUDE',operation:'ADD',sketchId:profile,length:5});
   v=await api.command(id,{type:'CREATE_DATUM_PLANE',origin:[0,0,0],normal:[0,0,1],uDirection:[1,0,0]});const plane=v.part!.datumPlanes.at(-1)!;
   v=await api.command(id,{type:'CREATE_SKETCH',datumPlaneId:plane.id});const holeSketch=v.part!.features.at(-1)!;v=await api.command(id,{type:'CREATE_PARAMETER',name:'local_pitch',value:100,unit:'mm'});plates.push({id,plane,sketch:holeSketch});
  }
  let root=await api.createDocument('PRODUCT','Layout and plates UI');for(const id of [l,...plates.map(p=>p.id)])root=await api.command(root.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:id});
  const publications=layout.part!.publications!;await api.deletePublication(l,publications[0].id);
  return {root:root.document.id,layout:l,sourceSketch:sketch,pitch,plates,instances:root.product!.instances,publications};
 });

 // Create a native point Publication through the shared tree selection field.
 await page.goto(`/documents/${seed.layout}`);await expect(page.locator('canvas')).toBeVisible();
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill('Point.1');
 await command(page,'发布接口');const publisher=page.getByRole('dialog',{name:'发布接口',exact:true});
 await page.locator('[role=treeitem][data-tree-key$="entity:source-0"]').click();
 await publisher.getByPlaceholder('自动命名（可选）').fill('Position.1');await publisher.getByRole('button',{name:'发布当前选择',exact:true}).click();
 await expect.poll(async()=>{const v=await current(page,seed.layout);return v.part.publications.some((p:any)=>p.name==='Position.1')}).toBe(true);
 await publisher.getByRole('button',{name:/确\s*定/}).click();await expect(publisher).toHaveCount(0);
 await page.goto(`/documents/${seed.root}`);await expect(page.locator('canvas')).toBeVisible();await observeViewport(page);
 const points=[[-60,-30,0],[60,-30,0],[-60,30,0],[60,30,0]];
 for(let plate=0;plate<2;plate++){
  await page.getByRole('textbox',{name:'筛选模型结构'}).fill('');
  await page.locator(`[role=treeitem][data-tree-key$="instance:${seed.instances[plate+1].id}"]`).dblclick();
  await expect(page.getByRole('button',{name:'孔',exact:true})).toBeVisible();
  const bind=async(index:number,cancel=false)=>{
   const d=page.getByRole('dialog',{name:'引用设计信息与关联管理',exact:true});if(await d.count()===0)await command(page,'引用设计信息');
   if(index===0&&!cancel){
    await d.getByRole('button',{name:'选择发布接口',exact:true}).click();const p=await worldScreen(page,points[0]);await page.mouse.click(p.x,p.y);await expect(d.locator('.feature-pick-field').filter({hasText:'发布接口'})).toContainText('Position.1');
   }else{await d.getByRole('combobox',{name:'发布接口目录',exact:true}).click();await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText(new RegExp(seed.publications[index].name.replace('.','\\.'))).click()}
   if(index<4&&index>0&&!cancel)await expect(d.getByRole('combobox',{name:'本地对象',exact:true})).toHaveAttribute('aria-expanded','false');
   await select(page,'本地对象',index===4?seed.plates[plate].plane.name:index===5?'local_pitch':seed.plates[plate].sketch.name!);
   await expect(d.getByRole('button',{name:/确\s*定/})).toBeEnabled();
   if(cancel){const before=await current(page,seed.root),part=await current(page,seed.plates[plate].id);await d.getByRole('button',{name:/取\s*消/}).click();expect((await current(page,seed.root)).document.versionId).toBe(before.document.versionId);expect((await current(page,seed.plates[plate].id)).part.contextInputs??[]).toEqual(part.part.contextInputs??[]);return}
   if(index<3){const old=(await current(page,seed.root)).document.versionId;await d.getByRole('button',{name:'应用并继续引用',exact:true}).click();await expect.poll(async()=>(await current(page,seed.root)).document.versionId).not.toBe(old);await expect(d).toBeVisible();await expect(d.getByRole('button',{name:'应用并继续引用',exact:true})).toBeDisabled()}
   else{await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0)}
  };
  if(plate===0)await bind(0,true);await bind(4);for(let i=0;i<4;i++)await bind(i);await bind(5);
  await page.getByRole('textbox',{name:'筛选模型结构'}).fill(seed.plates[plate].sketch.name!);await page.locator(`[role=treeitem][data-tree-key$="sketch:${seed.plates[plate].sketch.id}"]`).dblclick();await page.getByRole('textbox',{name:'筛选模型结构'}).fill('');
  await expect(page.getByRole('button',{name:'退出草图',exact:true})).toBeVisible();
  for(let i=0;i<points.length;i++){await command(page,'点');const p=await worldScreen(page,points[i]);await page.mouse.move(p.x,p.y);await page.mouse.click(p.x,p.y);await expect.poll(async()=>{const v=await current(page,seed.plates[plate].id);return v.part.features.find((f:any)=>f.id===seed.plates[plate].sketch.id).sketch.entities.length}).toBe(i+1)}
  const base=await current(page,seed.plates[plate].id),sketch=base.part.features.find((f:any)=>f.id===seed.plates[plate].sketch.id).sketch;
  expect(sketch.constraints.filter((c:any)=>c.kind==='COINCIDENT'&&c.references.some((r:any)=>r.target==='EXTERNAL'))).toHaveLength(4);
  await page.getByRole('button',{name:'退出草图',exact:true}).click();
  for(const point of points){await command(page,'孔');const d=page.getByRole('dialog',{name:'创建 孔',exact:true});if(await d.getByRole('switch',{name:'反向',exact:true}).isChecked())await d.getByRole('switch',{name:'反向',exact:true}).click();
   const pick=d.getByRole('button',{name:'选择定位点',exact:true});if(await pick.getAttribute('aria-pressed')!=='true')await pick.click();
   let p=await worldScreen(page,point),rect=await d.boundingBox();if(rect&&p.x>=rect.x&&p.x<=rect.x+rect.width&&p.y>=rect.y&&p.y<=rect.y+rect.height){const header=d.locator('.cad-command-dialog-header');const box=await header.boundingBox();await page.mouse.move(box!.x+80,box!.y+15);await page.mouse.down();await page.mouse.move(box!.x+(p.x>700?-340:340),box!.y+15,{steps:5});await page.mouse.up();p=await worldScreen(page,point)}
   await page.mouse.click(p.x,p.y);await expect(d.getByRole('button',{name:/确\s*定/})).toBeEnabled();await page.screenshot({path:info.outputPath(`plate-${plate}-hole-preview.png`)});await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);
  }
 }
 const before=await current(page,seed.root);for(const plate of seed.plates)expect(before.contextVariants.find((v:any)=>v.baseDocumentId===plate.id).part.features.filter((f:any)=>f.type==='HOLE')).toHaveLength(4);
 await page.evaluate(async seed=>{const {restApi:api}=await import('/src/api.ts');await api.command(seed.layout,{type:'SET_PARAMETER_VALUE',parameterId:seed.pitch,value:140,unit:'mm'})},seed);
 await command(page,'引用设计信息');const dialog=page.getByRole('dialog',{name:'引用设计信息与关联管理',exact:true});await expect(dialog.getByRole('button',{name:'接受关联更新',exact:true})).toBeEnabled();await dialog.getByRole('button',{name:'接受关联更新',exact:true}).click();await expect(dialog).toHaveCount(0);
 for(const plate of seed.plates)await expect.poll(async()=>{const v=await current(page,seed.root);return v.contextVariants.find((p:any)=>p.baseDocumentId===plate.id).part.features.find((f:any)=>f.id===plate.sketch.id).sketch.entities.map((e:any)=>e.point.x).sort()}).toEqual([-70,-70,70,70].sort());
 const accepted=await current(page,seed.root),cover=accepted.contextVariants.find((v:any)=>v.baseDocumentId===seed.plates[1].id),hole=cover.part.features.find((f:any)=>f.type==='HOLE');
 const editHole=async()=>{await page.getByRole('textbox',{name:'筛选模型结构'}).fill(hole.name);await page.locator(`[role=treeitem][data-tree-key$="feature:${hole.id}"]`).dblclick();const d=page.getByRole('dialog',{name:'编辑 孔',exact:true});await expect(d.getByRole('button',{name:/确\s*定/})).toBeEnabled();return d};
 let edit=await editHole();await edit.getByRole('textbox',{name:'直径（mm）',exact:true}).fill('6');await expect(edit.getByRole('button',{name:/确\s*定/})).toBeEnabled();const cancelHead=(await current(page,seed.root)).document.versionId;await edit.getByRole('button',{name:/取\s*消/}).click();expect((await current(page,seed.root)).document.versionId).toBe(cancelHead);
 edit=await editHole();await edit.getByRole('textbox',{name:'直径（mm）',exact:true}).fill('6');await expect(edit.getByRole('button',{name:/确\s*定/})).toBeEnabled();await edit.getByRole('button',{name:/确\s*定/}).click();await expect(edit).toHaveCount(0);
 const diameter=async()=>{const v=await current(page,seed.root);return v.contextVariants.find((v:any)=>v.baseDocumentId===seed.plates[1].id).part.features.find((f:any)=>f.id===hole.id).diameter};await expect.poll(diameter).toBe(6);
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(diameter).toBe(5);await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(diameter).toBe(6);
 await page.reload();await expect(page.locator('canvas')).toBeVisible();await expect.poll(diameter).toBe(6);await page.screenshot({path:info.outputPath('accepted-layout-update.png')});
 // Failure does not remove accepted inputs. Pin both consumers, then advance the
 // source occurrence so its broken interface is actually unavailable to picks.
 await page.evaluate(async seed=>{const {restApi:api}=await import('/src/api.ts');await api.command(seed.layout,{type:'EDIT_SKETCH',sketchId:seed.sourceSketch,operations:[{type:'DELETE_ENTITIES',entityIds:['source-0']}]})},seed);
 const activate=async(index:number)=>{await page.getByRole('textbox',{name:'筛选模型结构'}).fill('');await page.locator(`[role=treeitem][data-tree-key$="instance:${seed.instances[index+1].id}"]`).dblclick()};
 for(const index of [1,0]){
  await activate(index);await command(page,'引用设计信息');const d=page.getByRole('dialog',{name:'引用设计信息与关联管理',exact:true});
  await d.getByRole('listitem').filter({hasText:'Position.1 →'}).getByRole('button',{name:/固\s*定/}).click();
  await expect(d.getByRole('button',{name:/确\s*定/})).toBeEnabled();await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);
 }
 await command(page,'引用设计信息');let repair=page.getByRole('dialog',{name:'引用设计信息与关联管理',exact:true});await expect(repair.getByRole('button',{name:'接受关联更新',exact:true})).toBeEnabled();await repair.getByRole('button',{name:'接受关联更新',exact:true}).click();await expect(repair).toHaveCount(0);
 // Esc cancels a reconnect pick while preserving the accepted binding.
 const beforeReconnectCancel=(await current(page,seed.root)).document.versionId;
 await command(page,'引用设计信息');repair=page.getByRole('dialog',{name:'引用设计信息与关联管理',exact:true});await repair.getByRole('listitem').filter({hasText:'Position.1 →'}).getByRole('button',{name:/选择/}).click();await page.keyboard.press('Escape');await expect(repair).toBeVisible();await expect(repair.getByRole('button',{name:/确\s*定/})).toBeEnabled();await repair.getByRole('button',{name:/取\s*消/}).click();await expect(repair).toHaveCount(0);expect((await current(page,seed.root)).document.versionId).toBe(beforeReconnectCancel);
 // Renaming a fixed binding uses its accepted snapshot, not the unavailable source.
 await command(page,'引用设计信息');repair=page.getByRole('dialog',{name:'引用设计信息与关联管理',exact:true});await repair.getByRole('listitem').filter({hasText:'Position.1 →'}).getByRole('button',{name:/编\s*辑/}).click();await repair.getByRole('textbox',{name:'关联名称',exact:true}).fill('Fixed first hole');await expect(repair.getByRole('button',{name:/确\s*定/})).toBeEnabled();await repair.getByRole('button',{name:/确\s*定/}).click();await expect(repair).toHaveCount(0);
 await activate(1);await command(page,'引用设计信息');repair=page.getByRole('dialog',{name:'引用设计信息与关联管理',exact:true});const row=repair.getByRole('listitem').filter({hasText:'Position.1 →'});await row.getByRole('button',{name:/选择/}).click();await repair.getByRole('combobox',{name:'发布接口目录',exact:true}).click();await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText(/Repair position/).click();await select(page,'关联更新方式','接受更新后采用');await expect(repair.getByRole('button',{name:/确\s*定/})).toBeEnabled();await repair.getByRole('button',{name:/确\s*定/}).click();await expect(repair).toHaveCount(0);
 await expect.poll(async()=>{const v=await current(page,seed.root);return v.contextVariants.find((v:any)=>v.baseDocumentId===seed.plates[1].id).part.features.find((f:any)=>f.id===seed.plates[1].sketch.id).sketch.entities.some((e:any)=>Math.abs(e.point.x)<1e-8&&Math.abs(e.point.y)<1e-8)}).toBe(true);
 edit=await editHole();await edit.getByRole('textbox',{name:'直径（mm）',exact:true}).fill('7');await expect(edit.getByRole('button',{name:/确\s*定/})).toBeEnabled();await edit.getByRole('button',{name:/确\s*定/}).click();await expect(edit).toHaveCount(0);await expect.poll(diameter).toBe(7);
 await expect(page.getByText('更新关联零件未完成',{exact:true})).toHaveCount(0);await page.screenshot({path:info.outputPath('reconnected-original-hole.png')});expect(errors).toEqual([]);
});

test('finite solid edge and repeated occurrence edits keep independent complete views',async({page},info)=>{
 test.setTimeout(300_000);page.setDefaultTimeout(20_000);
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();await page.goto('/');
 const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');
  let source=await api.createDocument('PART','Finite edge source'),target=await api.createDocument('PART','Repeated receiver');
  for(const v of [source,target]){let p=await api.command(v.document.id,{type:'CREATE_SKETCH',plane:'XY'});const sketch=p.part!.features[0].id;p=await api.command(v.document.id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:-20,y:-10},second:{x:20,y:10}}]});p=await api.command(v.document.id,{type:'CREATE_SOLID_FEATURE',generator:'LINEAR_EXTRUDE',operation:'ADD',sketchId:sketch,length:v===source?5:3});if(v===source)source=p;else target=p}
  const body=source.part!.bodies[0],artifact=source.artifacts![body.geometryKey!];let edge=0;
  for(let i=1;i<=Number((artifact.topology as any).edges);i++){const p=await api.getTopologyProperties(source.document.id,body.geometryKey!,'EDGE',i,source.document.versionId);if(p.geometryType==='LINE'&&(p.properties.direction as number[])[2]===0){edge=i;break}}
  if(!edge)throw new Error('finite horizontal edge missing');
  source=await api.createPublication(source.document.id,{name:'Straight edge',publicationType:'CURVE',targetKind:'EDGE',geometryKey:body.geometryKey!,topologyId:edge,versionId:source.document.versionId});
  source=await api.createPublication(source.document.id,{name:'Mount XY',publicationType:'PLANE',targetKind:'PLANE',targetId:'datum-xy'});
  source=await api.command(source.document.id,{type:'CREATE_DATUM_PLANE',origin:[0,0,5],normal:[0,0,1],uDirection:[1,0,0]});source=await api.createPublication(source.document.id,{name:'Mount upper',publicationType:'PLANE',targetKind:'PLANE',targetId:source.part!.datumPlanes.at(-1)!.id});
  source=await api.createPublication(source.document.id,{name:'Depth five',publicationType:'PARAMETER',targetKind:'PARAMETER',targetId:source.part!.parameters!.find(p=>p.propertySlot==='length')!.parameterId});
  source=await api.command(source.document.id,{type:'CREATE_PARAMETER',name:'deep',value:8,unit:'mm'});source=await api.createPublication(source.document.id,{name:'Depth eight',publicationType:'PARAMETER',targetKind:'PARAMETER',targetId:source.part!.parameters!.find(p=>p.displayAlias==='deep')!.parameterId});
  target=await api.command(target.document.id,{type:'CREATE_DATUM_PLANE',origin:[0,0,2],normal:[0,0,1],uDirection:[1,0,0]});const plane=target.part!.datumPlanes.at(-1)!;target=await api.command(target.document.id,{type:'CREATE_SKETCH',datumPlaneId:plane.id});const receiver=target.part!.features.at(-1)!;
  let root=await api.createDocument('PRODUCT','Repeated context UI');for(const id of [source.document.id,target.document.id,target.document.id])root=await api.command(root.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:id});
  const ids=root.product!.instances.map(i=>i.id),parameter=target.part!.parameters!.find(p=>p.propertySlot==='length')!;let planeInput='',parameterInput='';
  for(let index=0;index<2;index++)for(const [kind,targetId,pub,input] of [['DATUM',plane.id,index?2:1,planeInput],['PARAMETER',parameter.parameterId,index?4:3,parameterInput]] as const){const a=await api.getProductDesignSession(root.document.id,ids[0]),b=await api.getProductDesignSession(root.document.id,ids[index+1]);root=await api.createProductContextBinding(root.document.id,{owningInstancePath:b.activeInstancePath!,sourceInstancePath:a.activeInstancePath!,publicationId:source.part!.publications![pub].id,publicationType:source.part!.publications![pub].type,targetKind:kind,targetId,contextInputId:input||undefined});target=await api.getDocument(target.document.id);const port=target.part!.contextInputs!.find(p=>p.target.kind===kind&&p.target.targetId===targetId)!;if(kind==='DATUM')planeInput=port.id;else parameterInput=port.id}
  return {root:root.document.id,target:target.document.id,instances:ids,instanceNames:root.product!.instances.map(i=>i.name),plane:plane.id,receiver:receiver.id,receiverName:receiver.name!,feature:target.part!.features[1].id,featureName:target.part!.features[1].name!};
 });
 await page.goto(`/documents/${seed.root}`);await expect(page.locator('canvas')).toBeVisible();await observeViewport(page);
 const activate=async(index:number)=>{await page.getByRole('textbox',{name:'筛选模型结构'}).fill('');await page.locator(`[role=treeitem][data-tree-key$="instance:${seed.instances[index+1]}"]`).dblclick()};
 const edit=async(instance?:string)=>{await page.getByRole('textbox',{name:'筛选模型结构'}).fill(instance?seed.instanceNames[seed.instances.indexOf(instance)]:seed.featureName);await page.locator(`[role=treeitem]${instance?`[data-tree-key*="instance:${instance}"]`:''}[data-tree-key$="pad:${seed.feature}"]`).dblclick();return page.getByRole('dialog',{name:'编辑 拉伸',exact:true})};
 await activate(0);await command(page,'引用设计信息');const association=page.getByRole('dialog',{name:'引用设计信息与关联管理',exact:true});await association.getByRole('combobox',{name:'发布接口目录',exact:true}).click();await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText(/Straight edge/).click();await select(page,'本地对象',seed.receiverName);await expect(association.getByRole('button',{name:/确\s*定/})).toBeEnabled();await association.getByRole('button',{name:/确\s*定/}).click();await expect(association).toHaveCount(0);
 const accepted=await current(page,seed.root),first=accepted.contextVariants.find((v:any)=>v.owningInstancePath.canonical===seed.instances[1]);expect(first.part.features.find((f:any)=>f.id===seed.receiver).sketch.externalGeometry[0].snapshot.kind).toBe('LINE');
 let d=await edit(seed.instances[1]);await expect(d.getByRole('textbox',{name:'长度（mm）',exact:true})).toBeDisabled();await expect(d.getByRole('textbox',{name:'长度（mm）',exact:true})).toHaveValue('5');await d.getByRole('switch').first().click();await expect(d.getByRole('button',{name:/确\s*定/})).toBeEnabled();await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);
 await activate(1);d=await edit(seed.instances[2]);await expect(d.getByRole('textbox',{name:'长度（mm）',exact:true})).toHaveValue('8');await expect(d.getByRole('textbox',{name:'长度（mm）',exact:true})).toBeDisabled();await d.getByRole('button',{name:/取\s*消/}).click();
 // The existing document-tab action preserves the same QueryClient cache.
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill('');await page.locator(`[role=treeitem][data-tree-key$="instance:${seed.instances[2]}"]`).click({button:'right'});await page.getByRole('menuitem',{name:'在新标签页中打开',exact:true}).click();await expect(page).toHaveURL(new RegExp(`/documents/${seed.target}`));
 d=await edit();await expect(d.getByRole('textbox',{name:'长度（mm）',exact:true})).toBeEnabled();await expect(d.getByRole('textbox',{name:'长度（mm）',exact:true})).toHaveValue('3');await d.getByRole('button',{name:/取\s*消/}).click();
 await observeViewport(page);await expect.poll(()=>page.evaluate(id=>{const e=(window as any).__cameraEngine;return e?.view?.document.id===id?(e.view.datumPlanes.find((p:any)=>p.id===e.view.part.datumPlanes.at(-1).id)?.origin[2]):undefined},seed.target)).toBe(2);
 await page.screenshot({path:info.outputPath('independent-default-after-occurrence-edits.png')});
});
