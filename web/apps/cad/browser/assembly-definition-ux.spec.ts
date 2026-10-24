import {viewportProbe,expectViewport} from "./camera-helpers";
import {expect,test,type Page} from '@playwright/test';
import {mkdir} from 'node:fs/promises';

async function command(page:Page,name:string){
 await page.getByRole('button',{name:'搜索工具',exact:true}).click();
 await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);
 await page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp('^'+name+'$')})}).click();
}
test.beforeAll(async()=>{await mkdir('../../../build/acceptance',{recursive:true});});

test('fixed create and edit share compact parameters and cancel closes on the first click',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('/documents/mock-product-frame');await expect(page.locator('canvas')).toBeVisible();
 await page.evaluate(async()=>{
  const {api}=await import('/src/api/client.ts');const {queryClient}=await import('/src/app/providers.tsx');
  await api.command('mock-product-frame',{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'FIX',firstAssemblyRef:{instanceId:'mock-instance-a',kind:'BODY'},fixedPose:{translation:[1.234567,2,3],rotation:[0,0,0,1]}});
  await queryClient.invalidateQueries();
 });
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill('FIX');
 const fixed=page.getByRole('treeitem').filter({hasText:/#FIX\./});
 const dialog=page.getByRole('dialog',{name:'约束定义',exact:true});
 for(let i=0;i<2;i++){
  await fixed.dblclick();await expect(dialog).toHaveCount(1);
  await expect(dialog.getByRole('switch',{name:'在空间中固定'})).toBeChecked();
  await expect(dialog.getByRole('spinbutton',{name:'X（mm）',exact:true})).toHaveValue('1.23');
  await expect(dialog.getByRole('spinbutton')).toHaveCount(6);
  const rows=dialog.locator('.assembly-pose-fields');await expect(rows).toHaveCount(2);
  expect((await rows.first().boundingBox())!.height).toBeLessThan(100);
  if(i===0)await page.screenshot({path:'../../../build/acceptance/fix-definition.png'});
  await dialog.getByRole('spinbutton',{name:'X（mm）',exact:true}).fill('9.87654');
  await dialog.getByRole('button',{name:/^取\s*消$/}).click();await expect(dialog).toHaveCount(0);
 }
 await filter.fill('');
 await page.evaluate(async()=>{const {useWorkbenchStore}=await import('/src/state/workbench-store.ts');useWorkbenchStore.getState().setSelection({kind:'instance',id:'mock-instance-b',instanceId:'mock-instance-b',occurrencePath:'mock-instance-b'});});
 await command(page,'固定');await expect(dialog).toHaveCount(1);
 await expect(dialog.getByRole('spinbutton')).toHaveCount(6);
 await dialog.getByRole('switch',{name:'在空间中固定'}).click();
 await expect(dialog.getByRole('switch',{name:'在空间中固定'})).not.toBeChecked();
 await dialog.getByRole('button',{name:/^取\s*消$/}).click();await expect(dialog).toHaveCount(0);
 expect(errors).toEqual([]);
});

test('group members use existing group identity when selecting an overlapping component',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('/documents/mock-product-frame');await expect(page.locator('canvas')).toBeVisible();
 await page.evaluate(async()=>{
  const {api}=await import('/src/api/client.ts');const {queryClient}=await import('/src/app/providers.tsx');
  const original=api.getDocument;
  api.getDocument=async id=>{
   const view=structuredClone(await original(id));if(id!=='mock-product-frame')return view;
   const first={instanceId:'mock-instance-a',kind:'BODY' as const};
   view.product!.instances.push({...view.product!.instances[1],id:'mock-instance-c',name:'P3'});
   view.product!.constraints=[
    {id:'g1',name:'固联组.1',kind:'FIX_TOGETHER',first,groupMembers:[{instanceId:'mock-instance-a'},{instanceId:'mock-instance-b'}],evaluationStatus:'VERIFIED'},
    {id:'g2',name:'固联组.2',kind:'FIX_TOGETHER',first,groupMembers:[{instanceId:'mock-instance-b'},{instanceId:'mock-instance-c'}],evaluationStatus:'VERIFIED'}
   ];
   view.structureTree!.children!.push(...view.product!.constraints.map(c=>({id:'ux/'+c.id,kind:'ASSEMBLY_CONSTRAINT' as const,name:c.name!,entityId:c.id,entityType:c.kind,documentId:id,versionId:view.document.versionId,capabilities:['EDIT' as const]})));
   return view;
  };await queryClient.invalidateQueries();
 });
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill('固联组.2');
 await page.getByRole('treeitem').filter({hasText:'固联组.2'}).dblclick();
 const dialog=page.getByRole('dialog',{name:'约束定义',exact:true}),list=dialog.getByLabel('固联组成员');
 await expect(dialog.getByText('固联组名称',{exact:true})).toHaveCount(0);
 await expect(list.getByText('固联组.1',{exact:true})).toBeVisible();
 await expect(list.locator('.feature-pick-detail').filter({hasText:/有效$/})).toHaveCount(2);
 await expect(list.getByRole('button',{name:/^定位成员 /})).toHaveCount(2);
 await expect(list.getByRole('button',{name:/^选择成员 /})).toHaveCount(2);
 await list.getByRole('listitem').filter({hasText:'固联组.1'}).getByRole('button',{name:/^移除成员 /}).click();
 await filter.fill('');
 // Feed the same tree selection route used by component rows.
 const instance=page.locator('[role=treeitem][data-tree-key*="instance:mock-instance-b"]').first();
 await list.getByRole('button',{name:'选择成员集',exact:true}).click();await instance.click();await expect(list.getByText('固联组.1',{exact:true})).toBeVisible();
 await expect.poll(async()=>Number(await viewportProbe(page,"viewport-selection").getAttribute('data-highlighted'))).toBeGreaterThanOrEqual(3);
 await page.screenshot({path:'../../../build/acceptance/fix-together-members.png'});
 await list.getByRole('button',{name:/^选择成员 /}).first().click();await instance.click();
 await expect(list.getByRole('listitem')).toHaveCount(1);
 await expect(list.getByText('固联组.1',{exact:true})).toBeVisible();
 await dialog.getByRole('button',{name:/^取\s*消$/}).click();await expect(dialog).toHaveCount(0);
 expect(errors).toEqual([]);
});

test('datum references stay compact and axis preview matches the saved reference',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('/documents/mock-part-bracket');await expect(page.locator('canvas')).toBeVisible();
 // Fixed geometry fixture: this case exercises editor lifecycle and WebGL
 // presentation; authoritative datum evaluation is covered by Worker tests.
 await page.evaluate(async()=>{
  const {api}=await import('/src/api/client.ts');const preview=api.previewCommand;
  api.previewCommand=async(id,input,signal)=>input.type==='CREATE_DATUM_AXIS'?{previewId:'axis-preview',baseVersionId:(await api.getDocument(id)).document.versionId,baseSequence:0,modelHash:'axis-ui-fixture',referenceGeometry:{datumPlanes:[],axisSystems:[],datumAxes:[{id:'axis-preview',name:String(input.name),origin:input.origin as [number,number,number],direction:input.direction as [number,number,number]}]}}:preview(id,input,signal);
 });
 await command(page,'基准轴');const dialog=page.getByRole('dialog',{name:'创建基准轴',exact:true});
 await expect(dialog.locator('.feature-pick-field')).toHaveCount(3);
 const heights=await Promise.all((await dialog.locator('.feature-pick-field').all()).map(async row=>(await row.boundingBox())!.height));expect(Math.max(...heights)).toBeLessThanOrEqual(76);expect(Math.max(...heights)-Math.min(...heights)).toBeLessThanOrEqual(1);
 await page.screenshot({path:'../../../build/acceptance/datum-reference-fields.png'});
 await dialog.getByText('自由坐标',{exact:true}).click();
 await dialog.getByRole('spinbutton',{name:'原点（mm） X',exact:true}).fill('12.34567');
 await dialog.getByRole('spinbutton',{name:'原点（mm） X',exact:true}).press('Tab');
 await expect(dialog.getByRole('spinbutton',{name:'原点（mm） X',exact:true})).toHaveValue('12.35');
 await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await page.screenshot({path:'../../../build/acceptance/datum-axis-preview.png'});
 await dialog.getByRole('button',{name:/^确\s*定$/}).click();await expect(dialog).toHaveCount(0);
 await page.screenshot({path:'../../../build/acceptance/datum-axis-saved.png'});
 expect(errors).toEqual([]);
});
