import {expect,test,type Page} from '@playwright/test';
import fs from 'node:fs/promises';
import {observeViewport} from './camera-helpers';
test.use({actionTimeout:15_000});

async function login(page:Page){
 const r=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(r.ok()).toBeTruthy();await page.goto('/');
}
const metadata=`ISO-10303-21;
HEADER;
FILE_DESCRIPTION(('empty import acceptance'),'2;1');
FILE_NAME('metadata.step','2026-11-11T00:00:00',(''),(''),'occccad','occccad','');
FILE_SCHEMA(('CONFIG_CONTROL_DESIGN'));
ENDSEC;
DATA;
#1=APPLICATION_CONTEXT('configuration controlled 3d designs of mechanical parts and assemblies');
#2=APPLICATION_PROTOCOL_DEFINITION('international standard','config_control_design',1994,#1);
#3=PRODUCT_CONTEXT('',#1,'mechanical');
#4=PRODUCT('metadata','Metadata only','',(#3));
#5=PRODUCT_DEFINITION_FORMATION('','',#4);
#6=PRODUCT_DEFINITION_CONTEXT('part definition',#1,'design');
#7=PRODUCT_DEFINITION('design','',#5,#6);
ENDSEC;
END-ISO-10303-21;`;

test('exchange: unsupported-only file creates an empty Part and persistent completion warning',async({page},info)=>{
 await login(page);const name=`empty-exchange-${Date.now()}.step`,warnings='该零件没有可创建的几何，已创建空 Part 文档。';
 await page.getByRole('button',{name:'导入',exact:true}).click();const d=page.getByRole('dialog',{name:'导入文档',exact:true});
 await expect(d).not.toContainText('加载中');await d.locator('input[type=file]').setInputFiles({name,mimeType:'model/step',buffer:Buffer.from(metadata)});
 await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);
 const toast=page.locator('.ant-notification-notice').filter({hasText:'文档导入完成，有警告'}).filter({hasText:warnings}).filter({hasText:name});await expect(toast).toBeVisible({timeout:60_000});
 const job=await page.evaluate(async name=>{const{restApi:api}=await import('/src/api.ts');return(await api.listJobs()).find(j=>j.type==='EXCHANGE_IMPORT'&&j.payload.fileName===name)},name);
 expect(job?.state).toBe('SUCCEEDED');expect(job?.documentId).toBeTruthy();
 await toast.getByRole('button',{name:'打开文档',exact:true}).click();await expect(page.locator('canvas').first()).toBeVisible();
 const view=await(await page.request.get(`/api/documents/${job!.documentId}`)).json();expect(view.document.type).toBe('PART');expect(view.part.features).toHaveLength(0);
 await page.reload();await page.getByRole('button',{name:'打开消息中心'}).click();const item=page.locator('.activity-item').filter({hasText:name});await expect(item).toContainText(warnings);await expect(item).toContainText('已完成，有警告');await expect(item.getByRole('button',{name:/打开文档/})).toBeVisible();
 await page.screenshot({path:info.outputPath('empty-import-persistent-warning.png')});
 await page.request.delete(`/api/documents/${job!.documentId}`);
});

test('exchange: export dialog selects hidden objects and STEP schema through real Jobs',async({page},info)=>{
 await login(page);
 const seed=await page.evaluate(async()=>{
  const{restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART',`Exchange options ${Date.now()}`);const id=v.document.id;
  for(let n=0;n<2;n++){
   if(n)v=await api.command(id,{type:'CREATE_BODY'});const body=v.part!.activeBodyId;
   v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY',bodyId:body});const sketch=v.part!.features.at(-1)!.id;
   v=await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:n*30,y:0},second:{x:n*30+10,y:10}}]});
   v=await api.command(id,{type:'CREATE_SOLID_FEATURE',bodyId:body,sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length:10});
   if(n)v=await api.command(id,{type:'SET_BODY_VISIBILITY',bodyId:body,visible:false});
  }
  return{id,name:v.document.name};
 });
 await page.reload();const card=page.locator('.document-card').filter({hasText:seed.name});await card.locator('.card-menu').click();await page.getByRole('menuitem',{name:/导出/}).click();
 let dialog=page.getByRole('dialog',{name:`导出“${seed.name}”`,exact:true});await expect(dialog.getByRole('checkbox',{name:'不导出隐藏对象'})).toBeChecked();await dialog.getByRole('combobox').click();await page.getByText('AP214（兼容）',{exact:true}).click();
 let sent=page.waitForRequest(r=>r.method()==='POST'&&r.url().endsWith('/api/exchange/exports'));await dialog.getByRole('button',{name:/确\s*定/}).click();expect((await sent).postDataJSON()).toMatchObject({documentId:seed.id,excludeHidden:true,stepSchema:'AP214IS'});
 let complete=page.locator('.ant-notification-notice').filter({hasText:'文档导出完成'}).filter({hasText:seed.name});await expect(complete).toBeVisible({timeout:60_000});let download=page.waitForEvent('download');await complete.getByRole('button',{name:'下载文件'}).click();let file=await download;let path=info.outputPath('visible.step');await file.saveAs(path);let text=await fs.readFile(path,'utf8');expect(text).toContain('AUTOMOTIVE_DESIGN');expect(text.match(/\bMANIFOLD_SOLID_BREP\s*\(/g)).toHaveLength(1);
 await complete.locator('.ant-notification-notice-close').click();await card.locator('.card-menu').click();await page.getByRole('menuitem',{name:/导出/}).click();dialog=page.getByRole('dialog',{name:`导出“${seed.name}”`,exact:true});await dialog.getByRole('checkbox',{name:'不导出隐藏对象'}).uncheck();await expect(dialog).toContainText('重新打开 STEP 后这些对象可能显示');
 sent=page.waitForRequest(r=>r.method()==='POST'&&r.url().endsWith('/api/exchange/exports'));await dialog.getByRole('button',{name:/确\s*定/}).click();expect((await sent).postDataJSON()).toMatchObject({excludeHidden:false,stepSchema:'AP214IS'});
 complete=page.locator('.ant-notification-notice').filter({hasText:'文档导出完成，有警告'}).filter({hasText:seed.name});await expect(complete).toContainText('当前 STEP 转换器不能完整保留对象的隐藏状态',{timeout:60_000});download=page.waitForEvent('download');await complete.getByRole('button',{name:'下载文件'}).click();file=await download;path=info.outputPath('all.step');await file.saveAs(path);text=await fs.readFile(path,'utf8');expect(text.match(/\bMANIFOLD_SOLID_BREP\s*\(/g)).toHaveLength(2);
 await page.screenshot({path:info.outputPath('export-all-warning.png')});await page.request.delete(`/api/documents/${seed.id}`);
});

test('exchange: Product displays independent geometry results at each occurrence',async({page},info)=>{
 await login(page);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const seed=await page.evaluate(async()=>{
  const{restApi:api}=await import('/src/api.ts');let part=await api.createDocument('PART',`Exchange wire ${Date.now()}`);const id=part.document.id;
  part=await api.command(id,{type:'CREATE_GEOMETRY_FEATURE',feature:{id:'',type:'GEOMETRY',name:'Exchange line',geometry:{kind:'LINE',curve:{points:[{id:'start',position:[0,0,0]},{id:'end',position:[50,0,0]}]},parameters:{direction:[0,0,0],origin:[0,0,0],distance:0,angle:0,start:0,end:1,tension:1}}}});
  let product=await api.createDocument('PRODUCT',`Exchange geometry assembly ${Date.now()}`);for(let n=0;n<2;n++){product=await api.command(product.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:id});if(n)product=await api.command(product.document.id,{type:'MOVE_INSTANCE',instanceId:product.product!.instances.at(-1)!.id,translation:[0,30,0],rotation:[0,0,0,1]})}
  return{part:id,id:product.document.id,feature:part.part!.features.at(-1)!.id};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas').first()).toBeVisible();await observeViewport(page);
 await expect.poll(()=>page.evaluate(feature=>[...(window as any).__cameraEngine.solidBindings.values()].filter((b:any)=>b.context.result?.featureId===feature&&b.group.visible).length,seed.feature)).toBe(2);
 const bindings=await page.evaluate(feature=>[...(window as any).__cameraEngine.solidBindings.values()].filter((b:any)=>b.context.result?.featureId===feature).map((b:any)=>({path:b.context.instancePath.canonical,revision:b.context.versionId,body:b.context.bodyId,result:b.context.result})),seed.feature);expect(new Set(bindings.map(b=>b.path)).size).toBe(2);expect(bindings.every(b=>b.revision&&b.result.resultId&&!b.body)).toBe(true);expect(errors).toEqual([]);
 await page.screenshot({path:info.outputPath('product-independent-curves.png')});await page.request.delete(`/api/documents/${seed.id}`);await page.request.delete(`/api/documents/${seed.part}`);
});
