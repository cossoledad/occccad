import {expect,test,type Page} from '@playwright/test';
import {observeViewport} from './camera-helpers';
import {treeNavigator} from './tree-helpers';

type Node={id:string;name:string;kind:string;entityId?:string;instancePath?:{canonical:string};children?:Node[]};
const row=(page:Page,key:string)=>page.locator(`[role=treeitem][data-tree-key=${JSON.stringify(key)}]`);
async function clearHover(page:Page){
 const point=await page.evaluate(()=>{const e=(window as any).__cameraEngine,canvas=document.querySelector('canvas')!,box=canvas.getBoundingClientRect();for(let y=24;y<box.height-24;y+=20)for(let x=24;x<box.width-24;x+=20)if(document.elementFromPoint(box.x+x,box.y+y)===canvas&&!e.hitTest(x,y))return {x:box.x+x,y:box.y+y}});
 if(!point)throw Error('no empty viewport point');await page.mouse.move(point.x,point.y);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.preselected)).toBeNull();
}
async function visibleHint(page:Page,key:string){
 const target=row(page,key);await expect(target).toHaveClass(/selected-descendant/);await expect(target).not.toHaveClass(/\bhighlighted\b/);await expect(target).toHaveAttribute('aria-selected','false');
 await expect(page.locator('[role=treeitem].selected-descendant')).toHaveCount(1);
 // Check actual painted feedback after removing hover; a class alone missed
 // the regression where a later workbench rule made the hint transparent.
 const paint=await target.evaluate(n=>{const c=getComputedStyle(n);return {background:c.backgroundColor,shadow:c.boxShadow}});
 expect(paint.background,'selection association must stay visible without hover').not.toBe('rgba(0, 0, 0, 0)');expect(paint.shadow).not.toBe('none');
}
async function facePoint(page:Page,path:string,featureId:string){
 const box=await page.locator('canvas').boundingBox();if(!box)throw Error('viewport unavailable');
 const point=await page.evaluate(({box,path,featureId})=>{
  const e=(window as any).__cameraEngine;
  for(let y=24;y<box.height-24;y+=8)for(let x=24;x<box.width-24;x+=8){
   if(document.elementFromPoint(box.x+x,box.y+y)!==document.querySelector('canvas'))continue;
   const hit=e.hitTest(x,y);if(hit?.kind==='face'&&(hit.occurrencePath??hit.instancePath?.canonical)===path&&hit.associatedFeatureIds?.includes(featureId))return {x:box.x+x,y:box.y+y};
  }
 },{box,path,featureId});if(!point)throw Error('no visible face with the expected occurrence and feature');return point;
}

test('Product viewport clicks keep visible feature and collapsed ancestor feedback',async({page},info)=>{
 test.setTimeout(240_000);page.setDefaultTimeout(20_000);
 expect((await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();await page.goto('/');
 const seed=await page.evaluate(async()=>{
  // @ts-ignore Vite application module; all geometry is created by the real API/Worker.
  const {restApi:api}=await import('/src/api.ts');
  let part=await api.createDocument('PART','Tree selection part');part=await api.command(part.document.id,{type:'CREATE_SKETCH',plane:'XY'});const sketch=part.part!.features.at(-1)!.id;
  part=await api.command(part.document.id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:-18,y:-15},second:{x:18,y:15}}]});
  part=await api.command(part.document.id,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length:20});const feature=part.part!.features.at(-1)!.id;
  let root=await api.createDocument('PRODUCT','Tree selection assembly');for(let i=0;i<2;i++)root=await api.command(root.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:part.document.id});const ids=root.product!.instances.map((i:any)=>i.id);
  root=await api.command(root.document.id,{type:'MOVE_INSTANCE',instanceId:ids[1],translation:[70,0,0],rotation:[0,0,0,1]});
  let outer=await api.createDocument('PRODUCT','Tree selection nested assembly');outer=await api.command(outer.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:root.document.id});
  return {feature,ids,scenes:[{id:root.document.id,prefix:'',revision:root.document.versionId},{id:outer.document.id,prefix:outer.product!.instances[0].id+'/',revision:outer.document.versionId}]};
 });
 for(const scene of seed.scenes){
  await page.goto('/documents/'+scene.id);await observeViewport(page);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.solidBindings.size)).toBe(2);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.viewTransition.active)).toBe(false);
  if(scene.prefix){await page.getByRole('button',{name:'选项',exact:true}).click();const options=page.getByRole('dialog',{name:'选项',exact:true});await options.getByRole('menuitem',{name:'场景',exact:true}).click();await options.getByRole('button',{name:'深色工程',exact:true}).click();await options.getByRole('button',{name:'关闭',exact:true}).click();await expect(page.locator('.workbench-content')).not.toHaveClass(/light-scene/)}
  const view=await (await page.request.get('/api/documents/'+scene.id)).json(),paths=seed.ids.map((id:string)=>scene.prefix+id);
  const find=(node:Node,path:string,parents:Node[]=[]):Node[]|undefined=>{if(node.entityId===seed.feature&&node.instancePath?.canonical===path)return [...parents,node];for(const child of node.children??[]){const found=find(child,path,[...parents,node]);if(found)return found}};
  const chain=find(view.structureTree,paths[0]);if(!chain)throw Error('feature tree path missing');const feature=chain.at(-1)!,top=chain.find(n=>n.kind==='INSTANCE')!;
  if(await row(page,top.id).getAttribute('aria-expanded')==='true')await row(page,top.id).getByRole('button',{name:'折叠',exact:true}).click();
  const point=await facePoint(page,paths[0],seed.feature);await page.mouse.click(point.x,point.y);await clearHover(page);
  await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.selected.map((s:any)=>[s.kind,s.occurrencePath]))).toEqual([['face',paths[0]]]);
  await visibleHint(page,top.id);await expect(row(page,top.id)).toHaveAttribute('aria-expanded','false');
  await treeNavigator(page)(feature.name,feature.id);await clearHover(page);await visibleHint(page,feature.id);
  await page.mouse.move(point.x,point.y);await expect(row(page,feature.id)).toHaveClass(/highlighted/);await clearHover(page);await visibleHint(page,feature.id);
  for(const parent of [...chain.slice(1,-1)].reverse()){
   const target=row(page,parent.id);if(await target.count()&&await target.getAttribute('aria-expanded')==='true'){
    await target.getByRole('button',{name:'折叠',exact:true}).click();await clearHover(page);await visibleHint(page,parent.id);await expect(target).toHaveAttribute('aria-expanded','false');
   }
  }
  await page.screenshot({path:info.outputPath(scene.prefix?'nested-collapsed-selection.png':'collapsed-selection.png')});
  await treeNavigator(page)(feature.name,feature.id);await clearHover(page);await visibleHint(page,feature.id);
  const other=await facePoint(page,paths[1],seed.feature);await page.mouse.click(other.x,other.y);await clearHover(page);
  await expect(row(page,feature.id)).not.toHaveClass(/selected-descendant/);
  const otherChain=find(view.structureTree,paths[1]);if(!otherChain)throw Error('second occurrence tree path missing');
  // Open the other branch to inspect its associated feature, preserving the
  // actual face selection and keeping the first occurrence unhighlighted.
  await treeNavigator(page)(otherChain.at(-1)!.name,otherChain.at(-1)!.id);await clearHover(page);await visibleHint(page,otherChain.at(-1)!.id);
  await page.screenshot({path:info.outputPath(scene.prefix?'nested-expanded-selection.png':'expanded-selection.png')});
  expect((await (await page.request.get('/api/documents/'+scene.id)).json()).document.versionId).toBe(scene.revision);
 }
});
