import {expect,test} from '@playwright/test';
import {observeViewport} from './camera-helpers';
import {treeNavigator} from './tree-helpers';

test.beforeEach(async({page})=>{
 page.setDefaultTimeout(20_000);
 expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();
 await page.goto('/');
});

test('constraint supports use available width and disabled or broken constraints edit through their object',async({page},info)=>{
 const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');
  let part=await api.createDocument('PART','Part.1');
  part=await api.command(part.document.id,{type:'CREATE_PUBLICATION',name:'Face.4',publicationType:'PLANE',targetKind:'PLANE',targetId:'datum-xy'});
  let root=await api.createDocument('PRODUCT','Constraint cleanup '+Date.now());const id=root.document.id;
  for(let i=0;i<2;i++)root=await api.command(id,{type:'INSERT_INSTANCE',referencedDocumentId:part.document.id});
  const refs=[];
  for(const instance of root.product!.instances){const session=await api.getProductDesignSession(id,instance.id);refs.push({instanceId:instance.id,kind:'PLANE',publicationRef:{scope:'OCCURRENCE',ownerPath:session.activeInstancePath,publicationId:part.part!.publications![0].id,expectedType:'PLANE',compatibilityVersion:'1.0.0'}})}
  root=await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'DISTANCE',firstAssemblyRef:refs[0],secondAssemblyRef:refs[1],value:10,distanceRelation:'SELECTED_PLANE_NORMAL_V1'});
  return {id,constraint:root.product!.constraints![0].id,name:root.product!.constraints![0].name,instance:root.product!.instances[0].id};
 });
 await page.goto('/documents/'+seed.id);await treeNavigator(page)(seed.name,`/constraint:${seed.constraint}`);
 const row=page.locator(`[role=treeitem][data-tree-key$="/constraint:${seed.constraint}"]`);
 await row.dblclick();const dialog=page.getByRole('dialog',{name:'约束定义',exact:true});
 await expect(dialog).toBeVisible();
 const owner=dialog.locator('.assembly-publication-owner').first();
 await expect(owner).toContainText('Part.1');
 expect(await owner.evaluate(el=>el.scrollWidth<=el.clientWidth+1)).toBe(true);
 const widths=await dialog.locator('.feature-pick-field').first().evaluate(el=>({field:el.getBoundingClientRect().width,control:el.querySelector('.command-field-control')!.getBoundingClientRect().width}));
 expect(widths.control).toBeGreaterThan(widths.field*.65);
 await page.screenshot({path:info.outputPath('support-owner-full-width.png')});
 await dialog.getByRole('button',{name:/取\s*消/}).click();
 await row.click({button:'right'});await page.getByRole('menuitem',{name:'对象',exact:true}).hover();
 await page.getByRole('menuitem',{name:'抑制',exact:true}).click();
 await expect(row).not.toContainText('[停用]');
 await expect.poll(async()=>{const v=await (await page.request.get('/api/documents/'+seed.id)).json();return v.product.constraints[0].suppressed}).toBe(true);
 await page.evaluate(async seed=>{
  const {restApi:api}=await import('/src/api.ts');const replacement=await api.createDocument('PART','Unmatched replacement');
  const v=await api.getDocument(seed.id);await api.command(seed.id,{type:'REPLACE_INSTANCE',versionId:v.document.versionId,instanceId:seed.instance,referencedDocumentId:replacement.document.id,replacementVersionId:replacement.document.versionId});
 },seed);
 await page.reload();await treeNavigator(page)(seed.name,`/constraint:${seed.constraint}`);
 await row.click({button:'right'});await page.getByRole('menuitem',{name:'对象',exact:true}).hover();
 await expect(page.getByRole('menuitem',{name:/Reconnect|更换支撑/})).toHaveCount(0);
 await page.keyboard.press('Escape');await page.keyboard.press('Escape');await row.dblclick();await expect(dialog).toBeVisible();
 await dialog.getByRole('button',{name:/取\s*消/}).click();
 await page.getByRole('button',{name:'搜索工具',exact:true}).click();await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill('关联更新');
 await expect(page.locator('.workbench-command-result')).toHaveCount(0);
});

test('parts receive each other shadows without a floor and viewing ground remains stable during motion',async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');
  const box=async(name:string,size:number,height:number)=>{let v=await api.createDocument('PART',name);v=await api.command(v.document.id,{type:'CREATE_SKETCH',plane:'XY'});const s=v.part!.features[0].id;v=await api.command(v.document.id,{type:'EDIT_SKETCH',sketchId:s,operations:[{type:'ADD_RECTANGLE',first:{x:0,y:0},second:{x:size,y:size}}]});return api.command(v.document.id,{type:'CREATE_SOLID_FEATURE',sketchId:s,generator:'LINEAR_EXTRUDE',operation:'ADD',length:height})};
  const receiver=await box('Shadow receiver',100,5),blocker=await box('Shadow blocker',15,20);
  let root=await api.createDocument('PRODUCT','Interpart shadow '+Date.now());const id=root.document.id;
  for(const part of [receiver,blocker])root=await api.command(id,{type:'INSERT_INSTANCE',referencedDocumentId:part.document.id});
  root=await api.command(id,{type:'MOVE_INSTANCE',instanceId:root.product!.instances[1].id,translation:[35,35,20],rotation:[0,0,0,1]});
  return {id,blocker:root.product!.instances[1].id};
 });
 await page.goto('/documents/'+seed.id);await observeViewport(page);
 await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.solidBindings.size)).toBe(2);
 const result=await page.evaluate(async()=>{
  const e=(window as any).__cameraEngine;
  const module=await (await fetch('/src/cad/rendering/scene-presentation.ts')).text(),path=module.match(/from\s+["']([^"']*three\.js[^"']*)["']/)?.[1];
  if(!path)throw Error('Three.js module URL missing');const T=await import(path);
  e.viewTransition.cancel();e.navigation.target.set(50,50,0);e.camera.position.set(50,50,230);e.camera.up.set(0,1,0);e.camera.lookAt(50,50,0);e.navigation.syncCamera(false);
  e.helpers.visible=false;e.sketchContext.visible=false;
  const settings={...e.sceneSettings,groundMode:'OFF',worldGrid:false,shadows:true,shadowOpacity:1,ambient:.35,fill:.25,reflection:.3};
  e.setSceneSettings(settings);
  const width=512,height=512,target=new T.WebGLRenderTarget(width,height),pixels=()=>{const data=new Uint8Array(width*height*4);e.renderer.setRenderTarget(target);e.renderer.render(e.scene,e.camera);e.renderer.readRenderTargetPixels(target,0,0,width,height,data);e.renderer.setRenderTarget(null);return data};
  const on=pixels();e.setSceneSettings({...settings,shadows:false});const off=pixels();
  // Project only the top of the receiving slab, excluding the block itself.
  let count=0,maxDifference=0,maxClearDifference=0;const samples=[];
  for(let y=50;y<=85;y+=2)for(let x=50;x<=75;x+=2){const p=new T.Vector3(x,y,5).project(e.camera),px=Math.floor((p.x+1)*width/2),py=Math.floor((p.y+1)*height/2);if(px<0||py<0||px>=width||py>=height)continue;const i=(py*width+px)*4,d=(off[i]+off[i+1]+off[i+2]-on[i]-on[i+1]-on[i+2])/3;if(d>3){count++;samples.push([x,y,d])}maxDifference=Math.max(maxDifference,d)}
  for(let y=10;y<=25;y++)for(let x=10;x<=25;x++){const p=new T.Vector3(x,y,5).project(e.camera),i=(Math.floor((p.y+1)*height/2)*width+Math.floor((p.x+1)*width/2))*4;maxClearDifference=Math.max(maxClearDifference,...[0,1,2].map(c=>Math.abs(off[i+c]-on[i+c])))}
  target.dispose();e.setSceneSettings({...settings,shadowOpacity:.6});e.invalidate();
  return {count,maxDifference,maxClearDifference,samples,flags:[...e.solidBindings.values()].map((b:any)=>({cast:b.mesh.castShadow,receive:b.mesh.receiveShadow})),floorVisible:e.environment.children[0].visible};
 });
 expect(result.flags).toEqual([{cast:true,receive:true},{cast:true,receive:true}]);expect(result.floorVisible).toBe(false);
 expect(result.count).toBeGreaterThan(15);expect(result.maxDifference).toBeGreaterThan(8);
 expect(result.maxClearDifference,'unoccluded planar faces must not show shadow acne').toBeLessThanOrEqual(2);
 await info.attach('receiver-shadow-samples',{body:JSON.stringify(result,null,2),contentType:'application/json'});
 await page.screenshot({path:info.outputPath('interpart-shadow-without-floor.png')});
 const stable=await page.evaluate(id=>{
  const e=(window as any).__cameraEngine;e.setSceneSettings({...e.sceneSettings,groundMode:'AUTO',grid:true});
  const snapshot=()=>[e.groundHeight,...e.groundFootprint.center.toArray(),e.groundFootprint.halfSize];
  const before=snapshot(),binding=[...e.solidBindings.values()].find((b:any)=>b.group.userData.occurrencePath===id) as any;
  const group=binding?.group??e.instanceGroups.get(id);if(!group)throw Error('blocker occurrence missing');
  group.position.addScalar(500);group.updateMatrixWorld(true);e.refreshSceneGround();const after=snapshot();
  group.visible=false;e.refreshSceneGround();const hidden=snapshot();group.visible=true;group.position.addScalar(-500);group.updateMatrixWorld(true);e.refreshSceneGround();e.invalidate();
  return {before,after,hidden};
 },seed.blocker);
 expect(stable.after).toEqual(stable.before);expect(stable.hidden).toEqual(stable.before);expect(stable.before[3]).toBeGreaterThanOrEqual(10000);
 await page.screenshot({path:info.outputPath('stable-large-ground.png')});expect(errors).toEqual([]);
});
