import {expect,test,type Page} from '@playwright/test';
import {treeNavigator} from './tree-helpers';

async function current(page:Page,id:string){return (await page.request.get(`/api/documents/${id}`)).json()}
async function tool(page:Page,name:string){
 await page.getByRole('button',{name:'搜索工具',exact:true}).click();
 await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);
 await page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp(`^${name}$`)})}).click();
}

test('simplified workbench tree, shared quantity input and context replacement',async({page},info)=>{
 test.setTimeout(240_000);page.setDefaultTimeout(20_000);const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();await page.goto('/');
 const seed=await page.evaluate(async()=>{
  // @ts-ignore real Vite application module
  const {restApi:api}=await import('/src/api.ts');const tag=Date.now();
  const cube=async(name:string,length:number)=>{let v=await api.createDocument('PART',name+' '+tag);v=await api.command(v.document.id,{type:'CREATE_SKETCH',plane:'XY'});const sketch=v.part!.features[0].id;v=await api.command(v.document.id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:0,y:0},second:{x:10,y:10}}]});v=await api.command(v.document.id,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length});return v};
  const part=await cube('UX original',10),replacement=await cube('UX chosen replacement',20);
  let root=await api.createDocument('PRODUCT','UX assembly '+tag);const id=root.document.id;
  for(let i=0;i<2;i++)root=await api.command(id,{type:'INSERT_INSTANCE',referencedDocumentId:part.document.id});
  const ids=root.product!.instances.map(i=>i.id);
  root=await api.command(id,{type:'MOVE_INSTANCE',instanceId:ids[1],translation:[40,0,0],rotation:[0,0,0,1]});
  root=await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'DISTANCE',firstAssemblyRef:{instanceId:ids[0],kind:'PLANE',geometryId:'datum-xy'},secondAssemblyRef:{instanceId:ids[1],kind:'PLANE',geometryId:'datum-xy'},distanceRelation:'SELECTED_PLANE_NORMAL_V1',value:10});
  const constraint=root.product!.constraints![0];
  root=await api.command(id,{type:'CREATE_PARAMETER',name:'Allowance',value:3,unit:'mm'});
  const supports=ids.map(instanceId=>({kind:'BODY' as const,bodyId:part.part!.bodies.find(b=>b.geometryKey)!.id,mode:'GEOMETRY' as const,instancePath:{rootDocumentId:id,canonical:instanceId,display:instanceId,segments:[{ownerDocumentId:id,ownerVersionId:root.document.versionId,instanceId,instanceName:root.product!.instances.find(i=>i.id===instanceId)!.name,referencedDocumentId:part.document.id,resolvedVersionId:part.document.versionId}]}}));
  root=await api.command(id,{type:'SAVE_MEASUREMENT',versionId:root.document.versionId,measurement:{id:'',name:'UX clearance',kind:'BETWEEN',supports,visible:true,showComponents:false}});
  return {id,pid:part.document.id,bodyId:part.part!.bodies.find(b=>b.geometryKey)!.id,first:ids[0],second:ids[1],firstName:root.product!.instances[0].name,measurement:root.product!.measurements![0].id,constraint:constraint.id,constraintName:constraint.name,replacement:replacement.document.id,replacementName:replacement.document.name};
 });
 await page.goto('/documents/'+seed.id);await expect(page.locator('canvas')).toBeVisible();
 await expect(page.getByRole('textbox',{name:/筛选模型结构/})).toHaveCount(0);
 await expect(page.getByRole('button',{name:'产品版本',exact:true})).toHaveCount(0);
 await expect(page.getByRole('button',{name:'替换组件',exact:true})).toHaveCount(0);
 await expect(page.getByRole('button',{name:'机构分析',exact:true})).toHaveCount(0);
 const view=await current(page,seed.id),rootChildren=view.structureTree.children;
 expect(rootChildren.at(-1).kind).toBe('PARAMETER_SET');expect(rootChildren.at(-1).name).toBe('参数');expect(rootChildren.findIndex((n:any)=>n.kind==='APPLICATIONS')).toBeLessThan(rootChildren.length-1);
 await expect(page.locator('.measurement-retained')).toHaveCount(1);
 await treeNavigator(page)('测量');const group=page.locator('[role=treeitem][data-tree-key$="/measurements"]');
 await group.click({button:'right'});await page.getByRole('menuitem',{name:'隐藏',exact:true}).click();await expect(page.locator('.measurement-retained')).toHaveCount(0);
 await group.click({button:'right'});await page.getByRole('menuitem',{name:'显示',exact:true}).click();await expect(page.locator('.measurement-retained')).toHaveCount(1);
 const first=page.locator(`[role=treeitem][data-tree-key$="/instance:${seed.first}"]`);
 await first.click({button:'right'});await page.getByRole('menuitem',{name:'属性…',exact:true}).click();const properties=page.getByRole('dialog',{name:/^属性 ·/});await properties.getByRole('tab',{name:'信息',exact:true}).click();await expect(properties.getByRole('button',{name:'加载最新属性（保留修改）',exact:true})).toBeVisible();await properties.getByRole('button',{name:'关闭',exact:true}).click();
 if(await first.getAttribute('aria-expanded')==='true')await first.locator('.specification-tree-junction.branch').click();
 await tool(page,'参数与公式');const parameters=page.getByRole('dialog',{name:'参数与公式',exact:true});await parameters.getByRole('combobox',{name:'参数所属文档',exact:true}).click();await page.locator('.ant-select-dropdown:visible').getByText(seed.firstName,{exact:false}).first().click();await expect(first).toHaveAttribute('aria-expanded','true');await parameters.getByRole('button',{name:'关闭',exact:true}).click();
 await page.locator(`[role=treeitem][data-tree-key="document:${seed.id}"]`).dblclick();
 await treeNavigator(page)(seed.constraintName);await page.locator(`[role=treeitem][data-tree-key$="/constraint:${seed.constraint}"]`).dblclick();const constraint=page.getByRole('dialog',{name:'约束定义',exact:true});
 await expect(constraint.getByRole('textbox',{name:'数值或公式',exact:true})).toHaveCount(1);await expect(constraint.getByRole('textbox',{name:'数值或公式',exact:true})).toHaveValue('10');await expect(constraint.getByRole('textbox',{name:'长度表达式（空白使用数值）'})).toHaveCount(0);
 await constraint.getByRole('textbox',{name:'数值或公式',exact:true}).fill('10 mm + 5 mm');await constraint.getByRole('textbox',{name:'数值或公式',exact:true}).blur();await constraint.getByText('参数选项',{exact:true}).click();await constraint.getByRole('combobox',{name:/^模式\s*:/}).click();await page.locator('.ant-select-dropdown:visible').getByText('参考测量',{exact:true}).click();await expect(constraint.getByRole('button',{name:/确\s*定/})).toBeEnabled();await page.screenshot({path:info.outputPath('compact-constraint.png')});await constraint.getByRole('button',{name:/确\s*定/}).click();await expect(constraint).toHaveCount(0);
 await expect.poll(async()=>(await current(page,seed.id)).product.constraints.find((c:any)=>c.id===seed.constraint).quantityParameter.source.expression.sourceText).toBe('(10 mm + 5 mm)');
 expect((await current(page,seed.id)).product.constraints.find((c:any)=>c.id===seed.constraint).measuredValue).toBeDefined();
 await first.click({button:'right'});await page.getByRole('menuitem',{name:'对象',exact:true}).hover();await page.getByRole('menuitem',{name:'替换组件',exact:true}).click();const replacement=page.getByRole('dialog',{name:/^替换组件 ·/});await expect(replacement.getByRole('combobox',{name:'新组件'})).toHaveCount(0);await replacement.getByRole('button',{name:'全部文档',exact:true}).click();await replacement.getByRole('textbox',{name:'检索插入文档',exact:true}).fill(seed.replacementName);await replacement.getByRole('button',{name:'选择 '+seed.replacementName,exact:true}).click();await replacement.getByRole('button',{name:/^替\s*换$/}).click();await expect(replacement).toHaveCount(0);
 await expect.poll(async()=>(await current(page,seed.id)).product.instances.find((i:any)=>i.id===seed.first).documentId).toBe(seed.replacement);
 const replaced=await current(page,seed.id);expect(replaced.product.constraints.find((c:any)=>c.id===seed.constraint).evaluationStatus).toBe('BROKEN');expect(replaced.product.constraints.find((c:any)=>c.id===seed.constraint).measuredValue).toBeUndefined();expect(replaced.product.instances.find((i:any)=>i.id===seed.second).documentId).toBe(seed.pid);
 await treeNavigator(page)(seed.constraintName);await expect(page.locator(`[role=treeitem][data-tree-key$="/constraint:${seed.constraint}"] .tree-status-badge`)).toBeVisible();await page.screenshot({path:info.outputPath('replacement-broken-relations.png')});expect(errors).toEqual([]);
});

test('replacement context bindings remain readable after source or receiver changes',async({page})=>{
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();await page.goto('/');
 const outcomes=await page.evaluate(async()=>{
  // @ts-ignore real Vite application module
  const {restApi:api}=await import('/src/api.ts');const outcomes=[];
  for(const role of ['source','receiver']){
   let source=await api.createDocument('PART','Binding source '+role),receiver=await api.createDocument('PART','Binding receiver '+role);const replacement=await api.createDocument('PART','Explicit replacement '+role);
   source=await api.command(source.document.id,{type:'CREATE_PARAMETER',name:'SourceValue',value:10,unit:'mm'});
   source=await api.command(source.document.id,{type:'CREATE_PUBLICATION',name:'Source value',publicationType:'PARAMETER',targetKind:'PARAMETER',targetId:source.part!.parameters![0].parameterId});
   receiver=await api.command(receiver.document.id,{type:'CREATE_PARAMETER',name:'ReceiverValue',value:3,unit:'mm'});
   let root=await api.createDocument('PRODUCT','Broken input '+role);const id=root.document.id;
   for(const child of [source,receiver])root=await api.command(id,{type:'INSERT_INSTANCE',referencedDocumentId:child.document.id});
   const path=(index:number)=>{const i=root.product!.instances[index];return {rootDocumentId:id,canonical:i.id,display:i.name,segments:[{ownerDocumentId:id,ownerVersionId:root.document.versionId,instanceId:i.id,instanceName:i.name,referencedDocumentId:i.documentId,resolvedVersionId:i.versionId}]}};
   root=await api.command(id,{type:'CREATE_PRODUCT_CONTEXT_BINDING',expectedRootRevisionId:root.document.versionId,owningInstancePath:path(1),sourceInstancePath:path(0),publicationId:source.part!.publications![0].id,publicationType:'PARAMETER',targetKind:'PARAMETER',targetId:receiver.part!.parameters![0].parameterId,referenceMode:'FOLLOW_WORKSPACE_WITH_ACCEPT'});
   const binding=root.product!.contextBindings![0],before=JSON.stringify([binding.sourceInstancePath,binding.owningInstancePath,binding.accepted.sourceRevisionId]);
   root=await api.command(id,{type:'REPLACE_INSTANCE',versionId:root.document.versionId,instanceId:root.product!.instances[role==='source'?0:1].id,referencedDocumentId:replacement.document.id,replacementVersionId:replacement.document.versionId});
   const reopened=await api.getDocument(id),after=reopened.product!.contextBindings![0],plan=await api.getProductUpdatePlan(id);
   root=await api.command(id,{type:'UNDO'});const restored=root.product!.contextBindings![0];
   root=await api.command(id,{type:'REDO'});
   outcomes.push({role,status:after.accepted.status,canAccept:plan.canAccept,currency:plan.entries.find(e=>e.bindingId===binding.id)?.currency,keptIdentity:before===JSON.stringify([after.sourceInstancePath,after.owningInstancePath,after.accepted.sourceRevisionId]),restored:restored.accepted.status,redone:root.product!.contextBindings![0].accepted.status});
  }
  return outcomes;
 });
 expect(outcomes).toEqual(['source','receiver'].map(role=>({role,status:'BROKEN',canAccept:false,currency:'UPDATE_BLOCKED',keptIdentity:true,restored:'CONNECTED',redone:'BROKEN'})));
});
