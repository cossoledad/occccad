import {expect,test,type Page} from '@playwright/test';
import {writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';

async function command(page:Page,name:string){
 await page.getByRole('button',{name:'搜索工具',exact:true}).click();
 await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);
 const item=page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp('^'+name+'$')})});
 await expect(item).toBeEnabled();await item.click();
}
async function current(page:Page,id:string){const r=await page.request.get('/api/documents/'+id);expect(r.ok()).toBeTruthy();return r.json();}

async function seedProduct(page:Page,defineMechanism=true){
 return page.evaluate(async defineMechanism=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');
  const parts=[];
  for(const [name,radius,length] of [['DMU轴',4,30],['DMU转子',12,4]] as const){
   let p=await api.createDocument('PART',name);
   p=await api.command(p.document.id,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=p.part!.features.at(-1)!.id;
   p=await api.command(p.document.id,{type:'EDIT_SKETCH',sketchId,operations:[{type:'ADD_ENTITY',entity:{id:crypto.randomUUID(),kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius}}]});
   p=await api.command(p.document.id,{type:'PAD_SKETCH',sketchId,length,operation:'ADD'});parts.push(p.document.id);
  }
  let v=await api.createDocument('PRODUCT','DMU位姿与转换验收');const id=v.document.id;
  for(const part of parts)v=await api.command(id,{type:'INSERT_INSTANCE',referencedDocumentId:part});
  const [shaft,rotor]=v.product!.instances.map(i=>i.id);
  v=await api.command(id,{type:'MOVE_INSTANCE',instanceId:rotor,translation:[22,15,7],rotation:[0,0,0,1]});
  const endpoint=(instanceId:string)=>({instanceId,frame:{translation:[0,0,0] as [number,number,number],rotation:[0,0,0,1] as [number,number,number,number]},axis:{instanceId,kind:'AXIS' as const,geometryId:'axis-system-default',axis:'Z' as const},plane:{instanceId,kind:'PLANE' as const,geometryId:'datum-xy'},capturedX:[1,0,0] as [number,number,number]});
  const mid=crypto.randomUUID(),jid=crypto.randomUUID();
  if(defineMechanism)v=await api.command(id,{type:'SAVE_KINEMATICS',versionId:v.document.versionId,kinematics:{mechanisms:[{id:mid,name:'机械装置.1',unitIds:[shaft,rotor],joints:[{id:crypto.randomUUID(),name:'固定零件.1',kind:'GROUND',first:endpoint(shaft),zero:{value:0,unit:'deg'},direction:1},{id:jid,name:'旋转.1',kind:'REVOLUTE',first:endpoint(shaft),second:endpoint(rotor),zero:{value:0,unit:'deg'},axialOffset:{value:7,unit:'mm'},direction:1}]}],drivers:[],laws:[],studies:[],associations:[]}});
  return {id,shaft,rotor,mid,jid,version:v.document.versionId};
 },defineMechanism);
}

// Production API, native Worker and real renderer. The trace records poses when
// Three.js renders the scene, including writes between React effects.
test('mechanism drafts, refresh, exit and independent conversion retain accepted poses',async({page},testInfo)=>{
 test.setTimeout(300_000);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const frameLog:any[]=[];
 await page.addInitScript(()=>{(window as any).__dmuFrames=[];(window as any).__dmuStage=sessionStorage.getItem('dmu-acceptance-stage')??'initial'});
 // Instrument the real module before its first engine is constructed, including
 // after reload. No API or numerical result is replaced by this trace hook.
 await page.route('**/src/viewport/cad-viewport-engine.ts*',async route=>{
  const response=await route.fetch();const source=await response.text();
  await route.fulfill({response,body:source+`
const dmuCommit=CadViewportEngine.prototype.commitApplicationDisplay;
CadViewportEngine.prototype.commitApplicationDisplay=function(){
 if(!this.renderer.__dmuTrace){
  const render=this.renderer.render.bind(this.renderer);this.renderer.__dmuTrace=true;
  this.renderer.render=(scene,camera)=>{if(scene===this.scene)window.__dmuFrames.push({stage:window.__dmuStage,poses:Object.fromEntries([...this.instanceGroups].map(([id,g])=>[id,{translation:g.position.toArray(),rotation:g.quaternion.toArray()}]))});return render(scene,camera)};
 }
 return dmuCommit.call(this);
};`});
 });
 if(!process.env.OCCCCAD_ADMIN_PASSWORD)throw new Error('Configured development login required');
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();
 await page.goto('/');
 const seed=await seedProduct(page);
 await page.goto('/documents/'+seed.id);await expect(page.locator('canvas')).toBeVisible({timeout:60_000});
 await command(page,'机构');await expect(page.locator('[data-application-id="'+seed.mid+'"]')).toBeVisible();await expect(page.getByRole('button',{name:'旋转接合',exact:true})).toBeEnabled();
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});
 const edit=async()=>{await filter.fill('偏移.1');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText(/^偏移\.1 \(/).first().dblclick();const d=page.getByRole('dialog',{name:'旋转.1',exact:true});await expect(d).toBeVisible();return d};
 const phase=async(stage:string)=>page.evaluate(s=>{(window as any).__dmuStage=s;sessionStorage.setItem('dmu-acceptance-stage',s)},stage);
 const collect=async()=>{frameLog.push(...await page.evaluate(()=>{const frames=(window as any).__dmuFrames;(window as any).__dmuFrames=[];return frames}));};
 const visibleZ=async(z:number)=>expect.poll(()=>page.evaluate(id=>{const f=(window as any).__dmuFrames.at(-1);return f?.poses[id]?.translation[2]},seed.rotor)).toBeCloseTo(z,5);
 await phase('cancel');let d=await edit();await d.getByRole('spinbutton').first().fill('15');await visibleZ(15);await page.keyboard.press('Escape');await visibleZ(7);
 await phase('confirm');d=await edit();await d.getByRole('spinbutton').first().fill('11');await visibleZ(11);await d.getByRole('button',{name:/^保\s*存$/}).click();await expect(d).toHaveCount(0);await visibleZ(11);
 let v=await current(page,seed.id);expect(v.product.instances.find((i:any)=>i.id===seed.rotor).translation).toEqual([22,15,7]);expect(v.product.kinematics.mechanisms[0].poses[seed.rotor].translation[2]).toBeCloseTo(11,5);
 await filter.fill('');const tree=page.getByRole('complementary',{name:'模型结构',exact:true});
 for(const label of ['Applications','机械装置',/^机械装置\.1，/,'接合',/^旋转\.1 \(/,'固定零件',/^固定零件\.1 \(/]){
  const row=tree.getByRole('treeitem').filter({has:page.getByText(label,{exact:true})}).first();
  if(await row.getAttribute('aria-expanded')==='false')await row.getByRole('button',{name:'展开',exact:true}).click();
 }
 await tree.getByText(/^机械装置\.1，/).click();await page.screenshot({path:'../../../build/acceptance/dmu-mechanism.png'});
 await phase('refresh');await collect();
 await page.reload();await expect(page.locator('[data-application-id="'+seed.mid+'"]')).toBeVisible({timeout:60_000});
 await visibleZ(11);await collect();await phase('exit');
 await command(page,'退出机构');await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();
 v=await current(page,seed.id);expect(v.product.constraints??[]).toHaveLength(0);expect(v.product.instances.find((i:any)=>i.id===seed.rotor).translation[2]).toBeCloseTo(11,5);
 await collect();await phase('reopen');await page.reload();await expect(page.locator('canvas')).toBeVisible({timeout:60_000});await visibleZ(11);v=await current(page,seed.id);expect(v.product.instances.find((i:any)=>i.id===seed.rotor).translation[2]).toBeCloseTo(11,5);
 await phase('conversion');
 await command(page,'机构');await command(page,'转换为产品约束');const apply=page.getByRole('dialog',{name:'转换为产品约束',exact:true});
 await apply.getByRole('button',{name:'生成／刷新转换计划'}).click();await expect(apply.getByText(/DOF 1/)).toBeVisible();await apply.getByRole('button',{name:'一次提交并返回装配'}).click();await expect(apply).toHaveCount(0);
 v=await current(page,seed.id);expect(v.product.constraints).toHaveLength(3);expect(v.product.kinematics.drivers??[]).toHaveLength(0);expect(v.product.kinematics.studies??[]).toHaveLength(0);
 const source=v.product.kinematics.mechanisms[0].joints.flatMap((j:any)=>j.constraints.map((c:any)=>c.id));for(const c of v.product.constraints)expect(source).not.toContain(c.id);
 await command(page,'撤销');v=await current(page,seed.id);expect(v.product.constraints??[]).toHaveLength(0);
 await command(page,'重做');v=await current(page,seed.id);expect(v.product.constraints).toHaveLength(3);expect(errors).toEqual([]);
 await collect();expect(frameLog.length).toBeGreaterThan(3);
 for(const f of frameLog.filter(f=>['cancel','confirm','refresh','exit','reopen','conversion'].includes(f.stage)&&f.poses[seed.rotor])){
  expect(Math.hypot(f.poses[seed.rotor].translation[0],f.poses[seed.rotor].translation[1]),f.stage).toBeLessThan(1e-5);
  if(['refresh','exit','reopen','conversion'].includes(f.stage))expect(f.poses[seed.rotor].translation[2],f.stage).toBeCloseTo(11,5);
 }
 const trace=JSON.stringify({documentId:seed.id,mechanismId:seed.mid,rotorId:seed.rotor,frames:frameLog},null,2);
 await testInfo.attach('application-render-frames',{body:trace,contentType:'application/json'});await writeFile(resolve('../../../build/acceptance/dmu-render-frames.json'),trace);
});

test('new mechanisms bind real tree supports through inspection, preview and commit',async({page})=>{
 test.setTimeout(240_000);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const login=await page.request.post('/api/auth/login',{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();
 await page.goto('/');const seed=await seedProduct(page,false);
 await page.goto('/documents/'+seed.id);await expect(page.locator('canvas')).toBeVisible({timeout:60_000});
 await command(page,'机构');await command(page,'新建机械装置');const created=page.getByRole('dialog',{name:'编辑机械装置',exact:true});
 await created.getByRole('button',{name:/^保\s*存$/}).click();await expect(created).toHaveCount(0);
 await command(page,'固定件');const ground=page.getByRole('dialog',{name:'固定零件.1',exact:true});
 await expect(ground.getByRole('button',{name:/^保\s*存$/})).toBeEnabled();await ground.getByRole('button',{name:/^保\s*存$/}).click();await expect(ground).toHaveCount(0);
 await command(page,'旋转接合');const joint=page.getByRole('dialog',{name:'旋转.1',exact:true});
 const filter=page.getByRole('textbox',{name:'筛选模型结构'}),tree=page.getByRole('complementary',{name:'模型结构',exact:true});
 for(const [label,index,nextRole] of [['Z Axis',0,'第二轴线'],['Z Axis',1,'第一定位平面'],['XY Plane',0,'第二定位平面'],['XY Plane',1,undefined]] as const){
  await filter.fill(label);const support=tree.getByText(label,{exact:true});await expect(support).toHaveCount(2);await support.nth(index).click();
  if(nextRole)await expect(joint.getByRole('button',{name:nextRole,exact:true})).toHaveClass(/ant-btn-primary/);
 }
 await joint.getByRole('spinbutton').first().fill('9');await expect(joint.getByRole('button',{name:/^保\s*存$/})).toBeEnabled();await joint.getByRole('button',{name:/^保\s*存$/}).click();await expect(joint).toHaveCount(0);
 let v=await current(page,seed.id);const m=v.product.kinematics.mechanisms[0],r=m.joints.find((j:any)=>j.kind==='REVOLUTE');
 expect(v.product.constraints??[]).toHaveLength(0);expect(v.product.instances.find((i:any)=>i.id===seed.rotor).translation).toEqual([22,15,7]);
 expect(m.poses[seed.rotor].translation[2]).toBeCloseTo(9,5);expect(m.evaluation).toMatchObject({relativeDof:1,gaugeDof:0,drivenDof:1,driverCount:0});
 expect(r.constraints).toHaveLength(2);
 for(const end of [r.first,r.second])for(const ref of [end.axis,end.plane]){expect(ref.instancePath.rootDocumentId).toBe(seed.id);expect(ref.instancePath.segments[0].instanceId).toBe(end.instanceId);expect(ref.geometryKey??'').toBe('');}
 await filter.fill('偏移.1');await expect(tree.getByText(/^偏移\.1 \(/)).toBeVisible();await page.screenshot({path:'../../../build/acceptance/dmu-created-by-ui.png'});
 await command(page,'退出机构');await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();
 v=await current(page,seed.id);expect(v.product.instances.find((i:any)=>i.id===seed.rotor).translation[2]).toBeCloseTo(9,5);expect(v.product.constraints??[]).toHaveLength(0);expect(errors).toEqual([]);
});
