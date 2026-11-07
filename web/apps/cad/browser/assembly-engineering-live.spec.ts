import {treeNavigator} from "./tree-helpers";
import {expect,test,type Page} from '@playwright/test';
import {observeViewport} from './camera-helpers';
async function login(page:Page,email=process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password=process.env.OCCCCAD_ADMIN_PASSWORD){expect(password).toBeTruthy();const r=await page.request.post('/api/auth/login',{data:{email,password}});expect(r.ok()).toBeTruthy();await page.goto('/');}
async function current(page:Page,id:string){return (await page.request.get(`/api/documents/${id}`)).json();}
async function openMotion(page:Page){await page.getByRole('button',{name:'搜索工具',exact:true}).click();await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill('约束分析');await page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:/^约束分析$/})}).click();const dialog=page.getByRole('dialog',{name:/约束分析 ·/});await dialog.getByRole('tab',{name:'自由度',exact:true}).click();return dialog;}
async function observeGuides(page:Page){
 // Same production-module observation used by the neighboring motion fixture.
 await page.route('**/src/viewport/cad-viewport-engine.ts*',async route=>{const response=await route.fetch();await route.fulfill({response,body:await response.text()+`\nconst show=CadViewportEngine.prototype.showRemainingMotion;CadViewportEngine.prototype.showRemainingMotion=function(...args){window.__engineeringEngine=this;return show.apply(this,args)};`});});
}
const guides=(page:Page)=>page.evaluate(()=>(window as any).__engineeringEngine?.motionMarkers?.children.length??0);

test('assembly dependencies expand real constraints and checked formulas, locate objects and change activation',async({page},info)=>{
 test.setTimeout(180000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production API
  const {restApi:api}=await import('/src/api.ts');let part=await api.createDocument('PART','Dependency part');part=await api.command(part.document.id,{type:'CREATE_SKETCH',plane:'XY'});const sketch=part.part!.features.at(-1)!.id;
  await api.command(part.document.id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:-8,y:-6},second:{x:8,y:6}}]});await api.command(part.document.id,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length:5});
  let v=await api.createDocument('PRODUCT','装配依赖分析验收');const id=v.document.id;for(let n=0;n<3;n++)v=await api.command(id,{type:'INSERT_INSTANCE',referencedDocumentId:part.document.id});const ids=v.product!.instances.map(i=>i.id);
  v=await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'FIX',fixMode:'SPACE',firstAssemblyRef:{kind:'BODY',instanceId:ids[0]}});
  v=await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'DISTANCE',name:'安装偏移',value:12,distanceRelation:'SELECTED_PLANE_NORMAL_V1',firstAssemblyRef:{kind:'PLANE',instanceId:ids[0],geometryId:'datum-xy'},secondAssemblyRef:{kind:'PLANE',instanceId:ids[1],geometryId:'datum-xy'}});const constraint=v.product!.constraints!.at(-1)!.id;
  v=await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'FIX_TOGETHER',name:'支架组',groupMembers:[{instanceId:ids[1]},{instanceId:ids[2]}]});
  const previous=new Set(v.product!.parameters?.map(p=>p.parameterId));v=await api.command(id,{type:'CREATE_PARAMETER',name:'BaseLength',value:12,unit:'mm'});const base=v.product!.parameters!.find(p=>!previous.has(p.parameterId))!;
  const beforeDerived=new Set(v.product!.parameters?.map(p=>p.parameterId));v=await api.command(id,{type:'CREATE_PARAMETER',name:'DoubleLength',value:24,unit:'mm'});const derived=v.product!.parameters!.find(p=>!beforeDerived.has(p.parameterId))!;
  v=await api.command(id,{type:'SET_PARAMETER_EXPRESSION',parameterId:derived.parameterId,expression:`${base.key} * 2`});
  return {id,ids,constraint,base:base.parameterId,derived:derived.parameterId,model:v.product,revision:v.document.versionId};
 });
 await page.goto('/documents/'+seed.id);await observeViewport(page);await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.solidBindings.size)).toBe(3);
 await page.getByRole('button',{name:'依赖分析',exact:true}).click();const dialog=page.getByRole('dialog',{name:/依赖分析 ·/});await expect(dialog.getByLabel('装配依赖关系图')).toBeVisible();
 await dialog.getByRole('button',{name:'展开全部',exact:true}).click();const offset=()=>dialog.locator('.react-flow__node').filter({has:page.getByTitle('安装偏移',{exact:true})});await expect(offset()).toHaveCount(1);
 await offset().click();await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.selected[0]?.constraintId)).toBe(seed.constraint);
 await expect(dialog.getByLabel('公式与参数')).not.toBeChecked();await dialog.getByLabel('公式与参数').check();await dialog.getByRole('button',{name:'展开全部',exact:true}).click();await expect(dialog.getByTitle('公式 · DoubleLength',{exact:true})).toBeVisible();await expect(dialog.getByTitle('BaseLength',{exact:true})).toBeVisible();
 const graphViewport=dialog.locator('.react-flow__viewport'),beforeZoom=await graphViewport.getAttribute('style');await dialog.getByRole('button',{name:'放大关系图',exact:true}).click();await expect.poll(()=>graphViewport.getAttribute('style')).not.toBe(beforeZoom);await dialog.getByRole('button',{name:'适合窗口',exact:true}).click();
 const beforeDrag=await offset().getAttribute('style'),box=await offset().boundingBox();if(!box)throw Error('constraint node not rendered');await page.mouse.move(box.x+box.width/2,box.y+box.height/2);await page.mouse.down();await page.mouse.move(box.x+box.width/2+35,box.y+box.height/2+20,{steps:5});await page.mouse.up();await expect.poll(()=>offset().getAttribute('style')).not.toBe(beforeDrag);
 await page.screenshot({path:info.outputPath('dependencies-constraints-and-formulas.png')});
 let after=await current(page,seed.id);expect(after.document.versionId).toBe(seed.revision);expect(after.product).toEqual(seed.model);
 await offset().click({button:'right'});await page.getByRole('menuitem',{name:'停用',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.constraints.find((c:any)=>c.id===seed.constraint).suppressed).toBe(true);await expect(offset()).toContainText('停用');
 await dialog.getByLabel('包含停用').uncheck();await expect(offset()).toHaveCount(0);await dialog.getByLabel('包含停用').check();await dialog.getByRole('button',{name:'展开全部',exact:true}).click();await offset().click({button:'right'});await page.getByRole('menuitem',{name:'启用',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.constraints.find((c:any)=>c.id===seed.constraint).suppressed??false).toBe(false);
 const instanceName=(await current(page,seed.id)).product.instances[1].name;const component=dialog.locator('.react-flow__node').filter({has:page.getByTitle(instanceName,{exact:true})});await component.click({button:'right'});await page.getByRole('menuitem',{name:'设为根',exact:true}).click();await expect(dialog.getByLabel('分析根组件')).toHaveAttribute('aria-expanded','false');
 await dialog.getByRole('button',{name:'展开全部',exact:true}).click();await page.screenshot({path:info.outputPath('dependencies-component-root.png')});
 await offset().click({button:'right'});await page.getByRole('menuitem',{name:'编辑',exact:true}).click();await expect(dialog).not.toBeVisible();await expect(page.getByRole('dialog',{name:/约束定义|编辑约束|偏移/}).first()).toBeVisible();await page.keyboard.press('Escape');expect(errors).toEqual([]);
});

test('read-only remaining motion refresh freezes metadata Revision, cancels stale markers and works for Viewer',async({page,browser},testInfo)=>{
 test.setTimeout(240_000);page.setDefaultTimeout(20000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await observeGuides(page);await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production module
  const {restApi:api}=await import('/src/api.ts');let part=await api.createDocument('PART','只读运动证据零件');const pid=part.document.id;
  part=await api.command(pid,{type:'CREATE_SKETCH',plane:'XY'});const sketch=part.part!.features.at(-1)!.id;
  part=await api.command(pid,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_ENTITY',entity:{id:'circle',kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius:6}}]});part=await api.command(pid,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length:12});
  let v=await api.createDocument('PRODUCT','只读剩余运动验收');const id=v.document.id;for(let n=0;n<5;n++)v=await api.command(id,{type:'INSERT_INSTANCE',referencedDocumentId:pid});const ids=v.product!.instances.map(i=>i.id);
  const positions=[[0,0,0],[0,0,20],[40,0,0],[50,0,0],[80,0,0]];for(let n=0;n<5;n++)v=await api.command(id,{type:'MOVE_INSTANCE',instanceId:ids[n],translation:positions[n],rotation:[0,0,0,1]});
  v=await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'FIX',fixMode:'SPACE',firstAssemblyRef:{kind:'BODY',instanceId:ids[0]}});const fixed=v.product!.constraints!.at(-1)!.id;
  const axis=(instanceId:string)=>({instanceId,kind:'AXIS',geometryId:'axis-system-default',axis:'Z'});
  v=await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'CONCENTRIC',firstAssemblyRef:axis(ids[0]),secondAssemblyRef:axis(ids[1])});const concentric=v.product!.constraints!.at(-1)!.id;
  v=await api.command(id,{type:'ADD_ASSEMBLY_CONSTRAINT',constraintKind:'FIX_TOGETHER',groupMembers:[{instanceId:ids[2]},{instanceId:ids[3]}]});
  v=await api.command(id,{type:'SET_DEFINITION_VISIBILITY',targetKind:'ASSEMBLY_CONSTRAINT',targetId:fixed,visible:false});
  const suffix=crypto.randomUUID(),email=`motion-viewer-${suffix}@example.test`,password=`Motion-${suffix}!`;
  const viewer=await api.adminCreateUser({email,displayName:'运动只读验收用户',password,platformRole:'MEMBER',status:'ACTIVE'});await api.share('documents',id,'USER',viewer.id,'VIEWER');await api.share('documents',pid,'USER',viewer.id,'VIEWER');
  return {id,pid,ids,fixed,concentric,version:v.document.versionId,model:v.product,email,password,viewerId:viewer.id};
 });
 const before=await current(page,seed.id);await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();let dialog=await openMotion(page);
 const state=()=>dialog.getByRole('status',{name:'剩余运动计算状态'});
 await expect(dialog.locator('tbody tr.ant-table-row').first()).toContainText('待计算');await expect(dialog.getByRole('button',{name:'刷新剩余运动',exact:true})).toBeEnabled();
 const refresh=async()=>{const revision=(await current(page,seed.id)).document.versionId;await expect(dialog).not.toContainText(revision);const response=page.waitForResponse(r=>r.url().includes('/assembly-engineering-evidence/refresh')&&r.request().method()==='POST');await dialog.getByRole('button',{name:/^(刷新剩余运动|重\s*试)$/}).click();const result=await response;expect(result.ok()).toBeTruthy();const evidence=await result.json();expect(evidence.revisionId).toBe(revision);expect(evidence.available).toBe(true);await expect(dialog.getByRole('button',{name:'刷新剩余运动',exact:true})).toBeEnabled();return evidence;};
 const calculated=await refresh();expect(calculated.components.filter((c:any)=>c.solved).flatMap((c:any)=>c.bodyIds)).toHaveLength(5);await expect(dialog.locator('tbody tr.ant-table-row')).toHaveCount(5);await expect(dialog).not.toContainText('分析完成');
 const names=before.product.instances.map((i:any)=>i.name);const shaft=()=>dialog.locator('tbody tr.ant-table-row').filter({hasText:names[1]});await shaft().click();await expect(dialog).toContainText('沿轴平移并绕该轴旋转');await expect.poll(()=>guides(page)).toBeGreaterThan(0);
 const rotationButton=dialog.getByRole('button',{name:'转动 1',exact:true}),translationButton=dialog.getByRole('button',{name:'平移 1',exact:true});
 await rotationButton.click();await expect(rotationButton).toHaveAttribute('aria-pressed','true');await expect(dialog.getByLabel('转动 1详情')).toContainText('轴上点');
 await translationButton.click();await expect(translationButton).toHaveAttribute('aria-pressed','true');expect(await page.evaluate(()=>(window as any).__engineeringEngine.motionPresentation.activeDirections)).toEqual(['rotation:0','translation:0']);
 await page.screenshot({path:testInfo.outputPath('accepted-pose-motion-directions.png')});
 await rotationButton.click();await expect(rotationButton).toHaveAttribute('aria-pressed','false');

 let after=await current(page,seed.id);expect(after.document.versionId).toBe(before.document.versionId);expect(after.product).toEqual(before.product);expect([after.document.canUndo,after.document.canRedo]).toEqual([before.document.canUndo,before.document.canRedo]);
 const peer=await page.context().newPage();await peer.goto('/');
 const edited=await peer.evaluate(async s=>{
  // @ts-ignore existing formal Preview/Commit chain, independent of read-only analysis
  const {restApi:api}=await import('/src/api.ts');const input={type:'EDIT_ASSEMBLY_CONSTRAINT',targetId:s.fixed,fixMode:'SPACE',fixedPose:{translation:[5,0,0],rotation:[0,0,0,1]}};
  const preview=await api.previewCommand(s.id,input);return api.command(s.id,{...input,previewId:preview.previewId});
 },seed);
 await expect(dialog).not.toContainText(edited.document.versionId);await refresh();const afterEditedAnalysis=await current(page,seed.id);expect(afterEditedAnalysis.product).toEqual(edited.product);expect(afterEditedAnalysis.document.versionId).toBe(edited.document.versionId);
 // Delay the actual Router/Worker response, never fabricate numerical evidence.
 let release!:()=>void,seen!:()=>void;let hold=new Promise<void>(r=>release=r),started=new Promise<void>(r=>seen=r);
 const pattern='**/assembly-engineering-evidence/refresh*';
 await page.route(pattern,async route=>{const response=await route.fetch();seen();await hold;await route.fulfill({response}).catch(()=>{});});
 await dialog.getByRole('button',{name:'刷新剩余运动',exact:true}).click();await Promise.race([started,new Promise((_,reject)=>setTimeout(()=>reject(Error('Refresh response not observed')),15000))]);await expect.poll(()=>guides(page)).toBe(0);await dialog.getByRole('button',{name:'取消计算',exact:true}).click();release();await expect(state()).toContainText('已取消');await expect.poll(()=>guides(page)).toBe(0);await page.unroute(pattern);
 await refresh();await shaft().click();await expect.poll(()=>guides(page)).toBeGreaterThan(0);
 await page.route(pattern,route=>route.abort('failed'));await dialog.getByRole('button',{name:'刷新剩余运动',exact:true}).click();await expect(state()).toContainText('计算失败');await expect.poll(()=>guides(page)).toBe(0);await page.unroute(pattern);await refresh();
 // Closing a running analysis must leave a retryable state when reopened.
 hold=new Promise<void>(r=>release=r);started=new Promise<void>(r=>seen=r);
 await page.route(pattern,async route=>{const response=await route.fetch();seen();await hold;await route.fulfill({response}).catch(()=>{});});
 await dialog.getByRole('button',{name:'刷新剩余运动',exact:true}).click();await started;await dialog.getByRole('button',{name:'关闭',exact:true}).click();release();dialog=await openMotion(page);await expect(state()).toContainText('已取消');await expect.poll(()=>guides(page)).toBe(0);await page.unroute(pattern);await refresh();
 hold=new Promise<void>(r=>release=r);started=new Promise<void>(r=>seen=r);
 await page.route(pattern,async route=>{const response=await route.fetch();seen();await hold;await route.fulfill({response}).catch(()=>{});});
 await dialog.getByRole('button',{name:'刷新剩余运动',exact:true}).click();await Promise.race([started,new Promise((_,reject)=>setTimeout(()=>reject(Error('Refresh response not observed')),15000))]);
 const changed=await peer.evaluate(async s=>{
  // @ts-ignore production module
  const {restApi:api}=await import('/src/api.ts');return api.command(s.id,{type:'SET_DEFINITION_VISIBILITY',targetKind:'ASSEMBLY_CONSTRAINT',targetId:s.fixed,visible:true});
 },seed);
 await expect(dialog).not.toContainText(changed.document.versionId);release();await expect(dialog.locator('tbody tr.ant-table-row').first()).toContainText('待计算');await expect.poll(()=>guides(page)).toBe(0);await page.unroute(pattern);await refresh();
 const stale=await page.evaluate(async s=>{
  // @ts-ignore production API adapter supplies normal CSRF/session headers
  const {restApi:api}=await import('/src/api.ts');try{await api.refreshAssemblyEngineeringEvidence(s.id,s.version);return "accepted"}catch(error){return String(error)}
 },seed);expect(stale).toContain('ASSEMBLY_EVIDENCE_STALE');
 await dialog.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('button',{name:'撤销',exact:true}).click();dialog=await openMotion(page);await refresh();await dialog.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('button',{name:'重做',exact:true}).click();dialog=await openMotion(page);await refresh();
 await page.reload();dialog=await openMotion(page);await expect(shaft()).toContainText('可运动');await shaft().click();await expect.poll(()=>guides(page)).toBeGreaterThan(0);
 // A genuine Viewer can compute without invoking any Domain Command.
 const viewerContext=await browser.newContext({baseURL:'http://127.0.0.1:5175',viewport:{width:1440,height:1000}});const viewer=await viewerContext.newPage();await observeGuides(viewer);await login(viewer,seed.email,seed.password);await viewer.goto(`/documents/${seed.id}`);const vd=await openMotion(viewer);await expect(vd.getByRole('button',{name:'刷新剩余运动',exact:true})).toBeEnabled();
 const priorViewer=await current(viewer,seed.id);expect(priorViewer.document.permission).toBe('VIEWER');const viewerResponse=viewer.waitForResponse(r=>r.url().includes('/assembly-engineering-evidence/refresh')&&r.request().method()==='POST');await vd.getByRole('button',{name:'刷新剩余运动',exact:true}).click();expect((await viewerResponse).ok()).toBeTruthy();await expect(vd.locator('tbody tr.ant-table-row')).toHaveCount(5);
 await vd.getByRole('button',{name:'关闭',exact:true}).click();await viewer.getByRole('button',{name:'依赖分析',exact:true}).click();const dependencyViewer=viewer.getByRole('dialog',{name:/依赖分析 ·/});await dependencyViewer.getByRole('button',{name:'展开全部',exact:true}).click();const fixedLabel=priorViewer.product.constraints.find((c:any)=>c.id===seed.fixed).name;await dependencyViewer.locator('.react-flow__node').filter({has:viewer.getByTitle(fixedLabel,{exact:true})}).click({button:'right'});await expect(viewer.getByRole('menuitem',{name:'停用',exact:true})).toHaveAttribute('aria-disabled','true');await viewer.keyboard.press('Escape');
 const postViewer=await current(viewer,seed.id);expect(postViewer.product).toEqual(priorViewer.product);expect(postViewer.document.versionId).toBe(priorViewer.document.versionId);await viewerContext.close();
 expect(errors).toEqual([]);await peer.close();
});

async function clashCommand(page:Page,name:string){await page.getByRole('button',{name:'搜索工具',exact:true}).click();await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);await page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp('^'+name+'$')})}).click();}
async function clashSeed(page:Page,motion=false){return page.evaluate(async motion=>{
 // @ts-ignore production Vite module
 const {restApi:api}=await import('/src/api.ts');let p=await api.createDocument('PART','Clash验收块');
 p=await api.command(p.document.id,{type:'CREATE_SKETCH',plane:'XY'});const sid=p.part!.features.at(-1)!.id;
 p=await api.command(p.document.id,{type:'EDIT_SKETCH',sketchId:sid,operations:[{type:'ADD_RECTANGLE',first:{x:-5,y:-5},second:{x:5,y:5}}]});
 p=await api.command(p.document.id,{type:'CREATE_SOLID_FEATURE',sketchId:sid,generator:'LINEAR_EXTRUDE',operation:'ADD',length:10});
 let v=await api.createDocument('PRODUCT',motion?'Clash运动停止验收':'Clash局部图形验收');
 for(let i=0;i<3;i++)v=await api.command(v.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:p.document.id});
 const [ground,moving,environment]=v.product!.instances.map(i=>i.id);
 for(const [instanceId,translation] of [[moving,motion?[0,0,20]:[8,0,0]],[environment,motion?[0,0,35]:[26,0,0]]] as const)v=await api.command(v.document.id,{type:'MOVE_INSTANCE',instanceId,translation,rotation:[0,0,0,1]});
 const mid=crypto.randomUUID();
 if(motion){const end=(instanceId:string)=>({instanceId,frame:{translation:[0,0,0],rotation:[0,0,0,1]},capturedX:[1,0,0],axis:{instanceId,kind:'AXIS',geometryId:'axis-system-default',axis:'Z'},plane:{instanceId,kind:'PLANE',geometryId:'datum-yz'}});
 v=await api.command(v.document.id,{type:'SAVE_KINEMATICS',versionId:v.document.versionId,kinematics:{mechanisms:[{id:mid,name:'滑块机构.1',unitIds:[ground,moving],joints:[{id:crypto.randomUUID(),name:'固定件.1',kind:'GROUND',first:end(ground)},{id:'slider',name:'棱形.1',kind:'PRISMATIC',first:end(ground),second:end(moving),orientationAngle:{value:0,unit:'deg'},coordinates:[{id:'travel',role:'AXIAL_TRANSLATION',dimension:'LENGTH',zero:{value:0,unit:'mm'},direction:1,lower:{value:-50,unit:'mm'},upper:{value:50,unit:'mm'}}]}]}],drivers:[{id:'driver',name:'行程.1',mechanismId:mid,jointId:'slider',coordinateId:'travel'}],studies:[],associations:[]}})}
 return {id:v.document.id,mid,ground,moving,environment,version:v.document.versionId};
 },motion)}
async function observeClash(page:Page){await page.route('**/src/viewport/cad-viewport-engine.ts*',async route=>{const response=await route.fetch();await route.fulfill({response,body:await response.text()+`\nconst clash=CadViewportEngine.prototype.showStaticInterference;CadViewportEngine.prototype.showStaticInterference=function(...args){window.__clashEngine=this;return clash.apply(this,args)};`})})}
async function clickClashInstance(page:Page,id:string){const canvas=page.locator('canvas'),box=await canvas.boundingBox();if(!box)throw Error('canvas missing');const point=await page.evaluate(({id,box})=>{const e=(window as any).__clashEngine;for(let y=20;y<box.height-20;y+=6)for(let x=20;x<box.width-20;x+=6){if(document.elementFromPoint(box.x+x,box.y+y)!==document.querySelector('canvas'))continue;const hit=e.hitTest(x,y);if(hit?.instanceId===id)return {x:box.x+x,y:box.y+y}}}, {id,box});if(!point)throw Error('No exposed selectable pixels for component');await page.mouse.click(point.x,point.y)}

test('clash exact local graphics, compact panel, temporary isolate and late cancellation',async({page},testInfo)=>{
 test.setTimeout(240_000);page.setDefaultTimeout(20000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await observeClash(page);await login(page);const seed=await clashSeed(page);await page.goto('/documents/'+seed.id);
 await clashCommand(page,'碰撞');let dialog=page.getByRole('dialog',{name:'碰撞',exact:true});
 const before=await current(page,seed.id),history=await (await page.request.get(`/api/documents/${seed.id}/history`)).json();
 await dialog.getByRole('button',{name:/^检\s*查$/}).click();await expect(dialog.getByRole('status',{name:'干涉检查状态'})).toContainText('存在需要检查');
 const row=dialog.locator('tbody tr.ant-table-row').filter({hasText:'干涉'});await expect(row).toHaveCount(1);let detailRelease!:()=>void,detailSeen!:()=>void;const detailHold=new Promise<void>(r=>detailRelease=r),detailStarted=new Promise<void>(r=>detailSeen=r);
 await page.route(`**/api/documents/${seed.id}/static-interference`,async route=>{const response=await route.fetch();if(route.request().postDataJSON().detailPair){detailSeen();await detailHold}await route.fulfill({response}).catch(()=>{})});
 await row.click();await detailStarted;await dialog.getByRole('checkbox',{name:'只看这一对',exact:true}).check();detailRelease();
 await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__clashEngine;return e?.staticInterferenceReport?.pairs.some((p:any)=>p.overlapTriangles?.length>0&&p.sectionPoints?.length>0)})).toBeTruthy();
 await dialog.getByRole('checkbox',{name:'只看这一对',exact:true}).check();await expect.poll(()=>page.evaluate(()=>[...(window as any).__clashEngine.solidBindings.values()].filter((b:any)=>b.group.visible).length)).toBe(2);
 expect(await page.evaluate(()=>[...(window as any).__clashEngine.interferenceRoleGroups].every((g:any)=>{let matches=true;g.traverse((o:any)=>{if(o.isMesh&&o.material?.userData?.cadMaterial==='surface')matches&&=o.material.color.getHex()===o.material.userData.baseColor});return matches}))).toBeTruthy();
 await expect(dialog.getByRole('checkbox',{name:'剖切',exact:true})).toHaveCount(0);await expect(dialog.getByRole('button',{name:'查看几何',exact:true})).toHaveCount(0);await expect(dialog).toHaveAttribute('data-panel-size','M');
 await page.unroute(`**/api/documents/${seed.id}/static-interference`);
 await testInfo.attach('clash-section',{body:await page.screenshot({path:testInfo.outputPath('clash-view.png')}),contentType:'image/png'});
 await dialog.getByRole('checkbox',{name:'只看这一对',exact:true}).uncheck();await expect.poll(()=>page.evaluate(()=>[...(window as any).__clashEngine.solidBindings.values()].filter((b:any)=>b.group.visible).length)).toBe(3);
 await dialog.getByRole('combobox',{name:'检查范围'}).click();await page.locator('.ant-select-dropdown:visible').getByText('所选零件与其余零件',{exact:true}).click();
 // Pick actual canvas pixels through the production hit tester/session.
 await clickClashInstance(page,seed.ground);await dialog.getByRole('button',{name:/^检\s*查$/}).click();await expect(dialog.getByRole('status',{name:'干涉检查状态'})).toContainText('存在需要检查');await expect(dialog.getByText('2 / 2 对已完成')).toBeVisible();
 await dialog.getByRole('button',{name:'关闭',exact:true}).click();await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.staticInterferenceMarkers?.children.length??0)).toBe(0);
 const after=await current(page,seed.id);expect(after.product).toEqual(before.product);expect(after.document.versionId).toBe(before.document.versionId);expect(await (await page.request.get(`/api/documents/${seed.id}/history`)).json()).toEqual(history);
 await page.setViewportSize({width:1100,height:720});await clashCommand(page,'碰撞');dialog=page.getByRole('dialog',{name:'碰撞',exact:true});const bounds=await dialog.boundingBox();expect(bounds!.x).toBeGreaterThanOrEqual(0);expect(bounds!.x+bounds!.width).toBeLessThanOrEqual(1100);
 let release!:()=>void;const hold=new Promise<void>(r=>release=r);let observed!:()=>void;const started=new Promise<void>(r=>observed=r);
 await page.route(`**/api/documents/${seed.id}/static-interference`,async route=>{const response=await route.fetch();observed();await hold;await route.fulfill({response}).catch(()=>{})});
 await dialog.getByRole('button',{name:/^检\s*查$/}).click();await started;await dialog.getByRole('button',{name:'取消计算',exact:true}).click();release();await expect(dialog.getByRole('status',{name:'干涉检查状态'})).toHaveText('已取消');await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.staticInterferenceMarkers?.children.length??0)).toBe(0);await page.unroute(`**/api/documents/${seed.id}/static-interference`);
 await dialog.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.instances.find((i:any)=>i.id===seed.environment).translation).toEqual([0,0,0]);await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product).toEqual(before.product);
 await page.reload();expect((await current(page,seed.id)).product).toEqual(before.product);expect(errors).toEqual([]);
 await testInfo.attach('clash-document',{body:JSON.stringify(seed),contentType:'application/json'});
});

test('mechanism clash stop retains safe geometry and prompt permits the checked collision without history',async({page},testInfo)=>{
 test.setTimeout(240_000);page.setDefaultTimeout(20000);await observeClash(page);await login(page);const seed=await clashSeed(page,true);await page.goto(`/documents/${seed.id}?application=mechanism&mechanismId=${seed.mid}`);
 await expect(page.getByRole('button',{name:'使用命令进行模拟',exact:true})).toBeEnabled({timeout:60_000});await clashCommand(page,'使用命令进行模拟');const dialog=page.getByRole('dialog',{name:'使用命令进行模拟',exact:true});const before=await current(page,seed.id);
 await dialog.getByRole('combobox',{name:'运动干涉检测'}).click();await page.locator('.ant-select-dropdown:visible').getByText('干涉时停止',{exact:true}).click();await expect(dialog.getByText('当前采样位置无干涉',{exact:true})).toBeVisible();
 const input=dialog.getByRole('spinbutton',{name:'命令长度',exact:true});await input.fill('-20');await expect(dialog.getByText('检测到干涉或间隙不足，目标未到达',{exact:true})).toBeVisible();
 const stopped=Number(await input.inputValue());expect(stopped).toBeLessThan(0);expect(stopped).toBeGreaterThanOrEqual(-10.00001);
 await dialog.getByRole('combobox',{name:'运动干涉检测'}).click();await page.locator('.ant-select-dropdown:visible').getByText('提示',{exact:true}).click();await input.fill('-20');await expect(dialog.getByText('检测到干涉或间隙不足',{exact:true})).toBeVisible();await expect.poll(async()=>Number(await input.inputValue())).toBeCloseTo(-20,2);
 await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__clashEngine;return e.staticInterferenceReport?.pairs.some((p:any)=>p.overlapTriangles?.length>0)&&e.staticInterferenceMarkers?.children.some((c:any)=>c.geometry?.type==='WireframeGeometry')})).toBeTruthy();
 await testInfo.attach('motion-clash-prompt',{body:await page.screenshot({path:testInfo.outputPath('clash-view.png')}),contentType:'image/png'});
 await dialog.getByRole('button',{name:/^取\s*消$/}).click();await expect(dialog).toHaveCount(0);await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.staticInterferenceMarkers?.children.length??0)).toBe(0);
 const after=await current(page,seed.id);expect(after.product).toEqual(before.product);expect(after.document.versionId).toBe(before.document.versionId);await page.reload();expect((await current(page,seed.id)).product).toEqual(before.product);
});

test('clash checked recording playback stops at a qualified sample and keeps its time',async({page})=>{
 test.setTimeout(180_000);page.setDefaultTimeout(20000);await observeClash(page);await login(page);const seed=await clashSeed(page,true);await page.goto(`/documents/${seed.id}?application=mechanism&mechanismId=${seed.mid}`);
 await expect(page.getByRole('button',{name:'模拟',exact:true})).toBeEnabled({timeout:60_000});await clashCommand(page,'模拟');
 const trial=page.getByRole('dialog',{name:'使用命令进行模拟',exact:true}),recorder=page.getByRole('dialog',{name:'模拟',exact:true}),input=trial.getByRole('spinbutton',{name:'命令长度',exact:true});
 await expect(input).toBeEnabled();await recorder.getByRole('checkbox',{name:'自动插入',exact:true}).uncheck();
 await input.fill('35');await expect.poll(async()=>Number(await input.inputValue())).toBeCloseTo(35,2);await expect(recorder.getByRole('button',{name:'插入',exact:true})).toBeEnabled();
 await recorder.getByRole('spinbutton',{name:'关键帧时间',exact:true}).fill('1');await recorder.getByRole('button',{name:'插入',exact:true}).click();await expect(recorder.locator('tbody tr.ant-table-row')).toHaveCount(2);
 await recorder.getByRole('button',{name:'起点',exact:true}).click();await expect.poll(async()=>Number(await input.inputValue())).toBeCloseTo(0,2);
 await trial.getByRole('combobox',{name:'运动干涉检测'}).click();await page.locator('.ant-select-dropdown:visible').getByText('干涉时停止',{exact:true}).click();await expect(trial.getByText('当前采样位置无干涉',{exact:true})).toBeVisible();
 await recorder.getByRole('button',{name:'播放',exact:true}).click();await expect(trial.getByText('检测到干涉或间隙不足，目标未到达',{exact:true})).toBeVisible();await expect(recorder.getByRole('button',{name:'播放',exact:true})).toBeEnabled();
 const position=Number(await input.inputValue()),time=Number(await recorder.getByRole('spinbutton',{name:'模拟定位时间',exact:true}).inputValue());expect(position).toBeGreaterThan(0);expect(position).toBeLessThanOrEqual(5.00001);expect(time).toBeCloseTo(position/35,2);
 await recorder.getByRole('button',{name:/^取\s*消$/}).click();await expect(trial).toHaveCount(0);
 const currentView=await current(page,seed.id);expect(currentView.product.kinematics.studies??[]).toHaveLength(0);
});

test('saved spatial analyses: wire clash, exact sections, distance bands and explicit update history',async({page},testInfo)=>{
 test.setTimeout(240_000);page.setDefaultTimeout(20_000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await observeClash(page);await login(page);const seed=await clashSeed(page);await page.goto('/documents/'+seed.id);
 const before=await current(page,seed.id);
 await clashCommand(page,'碰撞');let clash=page.getByRole('dialog',{name:'碰撞',exact:true});await clash.getByRole('button',{name:/^检\s*查$/}).click();await expect(clash.getByRole('status',{name:'干涉检查状态'})).toContainText('存在需要检查');
 await clash.locator('tbody tr.ant-table-row').filter({hasText:'干涉'}).click();await expect.poll(()=>page.evaluate(()=>{const g=(window as any).__clashEngine?.staticInterferenceMarkers;return g?.children.some((c:any)=>c.geometry?.type==='WireframeGeometry')})).toBeTruthy();
 // Role and overlap evidence use wires; the original engineering materials remain intact.
 expect(await page.evaluate(()=>(window as any).__clashEngine.staticInterferenceMarkers.children.some((c:any)=>c.isMesh&&!c.isLineSegments2))).toBeFalsy();
 await clash.getByRole('textbox',{name:'分析名称'}).fill('安装碰撞');await clash.getByRole('button',{name:/^确\s*定$/}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses?.length).toBe(1);await expect(clash).toHaveCount(0);
 await clashCommand(page,'切割');let section=page.getByRole('dialog',{name:'切割',exact:true});await section.getByRole('textbox',{name:'分析名称'}).fill('安装截面');await section.getByRole('spinbutton',{name:'位置 Z',exact:true}).fill('5');await expect(section.getByRole('switch',{name:'截面视图',exact:true})).toBeVisible();await expect(section.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await expect(section.locator('.cad-command-dialog-footer button')).toHaveCount(2);await expect(section.getByRole('button',{name:/^分\s*析$/})).toHaveCount(0);await expect(section.getByRole('slider',{name:'显示精度',exact:true})).toBeVisible();await expect(section.getByRole('spinbutton',{name:'切割宽',exact:true})).toHaveCount(0);await expect(section.getByRole('spinbutton',{name:'切割高',exact:true})).toHaveCount(0);await expect(section).not.toContainText('分析完成');
 {await section.getByRole('switch',{name:'截面视图',exact:true}).click();const sheet=page.getByRole('dialog',{name:'截面视图',exact:true});const svg=sheet.getByRole('img',{name:'剖切二维视图'});await expect(svg).toBeVisible();const initialBox=await svg.getAttribute('viewBox');await svg.hover();await page.mouse.wheel(0,-200);await expect(svg).not.toHaveAttribute('viewBox',initialBox!);await page.screenshot({path:testInfo.outputPath('section-independent-view.png')});
 const windowBefore=await sheet.boundingBox();
 const path=svg.locator('g').first(),pathBefore=(await path.boundingBox())!,canvasBox=(await svg.boundingBox())!;
 const panStart={x:canvasBox.x+canvasBox.width/2,y:canvasBox.y+canvasBox.height/2};await page.mouse.move(panStart.x,panStart.y);await page.mouse.down({button:'middle'});await page.mouse.move(panStart.x+50,panStart.y+30,{steps:6});await page.mouse.up({button:'middle'});
 const pathAfter=(await path.boundingBox())!;expect(pathAfter.x-pathBefore.x).toBeCloseTo(50,0);expect(pathAfter.y-pathBefore.y).toBeCloseTo(30,0);const zoomed=await svg.getAttribute('viewBox');
 await page.mouse.move(panStart.x,panStart.y);await page.mouse.down({button:'middle'});await page.mouse.move(panStart.x+20,panStart.y+15);await page.keyboard.press('Escape');await page.mouse.up({button:'middle'});await expect(svg).toHaveAttribute('viewBox',zoomed!);await expect(sheet).toBeVisible();
 await section.getByRole('spinbutton',{name:'位置 Z',exact:true}).fill('10000');await section.getByRole('spinbutton',{name:'位置 Z',exact:true}).press('Tab');await expect(section.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await expect(sheet).toBeVisible();await expect(section.getByRole('switch',{name:'截面视图',exact:true})).toBeChecked();
 expect(await svg.locator('path').evaluateAll(paths=>paths.every(p=>!p.getAttribute('d')))).toBeTruthy();expect(await sheet.boundingBox()).toEqual(windowBefore);expect(await svg.getAttribute('viewBox')).toBe(zoomed);
 await page.screenshot({path:testInfo.outputPath('section-empty-persistent-window.png')});
 await page.setViewportSize({width:1000,height:720});await expect.poll(async()=>{const small=(await sheet.boundingBox())!;return small.width<=720&&small.height<=468&&small.x+small.width<=1000&&small.y+small.height<=720}).toBeTruthy();await page.setViewportSize({width:1440,height:1000});
 await section.getByRole('spinbutton',{name:'位置 Z',exact:true}).fill('5');await section.getByRole('spinbutton',{name:'位置 Z',exact:true}).press('Tab');await expect(section.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await expect.poll(()=>svg.locator('path').evaluateAll(paths=>paths.some(p=>!!p.getAttribute('d')))).toBeTruthy();await expect(svg).toHaveAttribute('viewBox',initialBox!);
 await sheet.getByRole('button',{name:'关闭',exact:true}).click();await expect(section.getByRole('switch',{name:'截面视图',exact:true})).not.toBeChecked();}
 await expect(section.locator('tbody tr.ant-table-row')).toHaveCount(3);await expect(section.locator('tbody tr.ant-table-row').first()).toContainText('100');
 await section.getByRole('combobox',{name:'三维切除',exact:true}).click();await page.locator('.ant-select-dropdown:visible').getByText('内侧',{exact:true}).click();await expect(section.getByRole('switch',{name:'截面视图',exact:true})).toBeVisible();await expect(section.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await expect.poll(()=>page.evaluate(()=>[...(window as any).__clashEngine.solidBindings.values()].filter((b:any)=>{const materials=Array.isArray(b.mesh.material)?b.mesh.material:[b.mesh.material];return materials.every((m:any)=>m.clippingPlanes?.length===1)}).length)).toBe(3);
 await testInfo.attach('section-clipped',{body:await page.screenshot({path:testInfo.outputPath('spatial-section.png')}),contentType:'image/png'});
 await section.getByRole('button',{name:/^确\s*定$/}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses?.length).toBe(2);await expect(section).toHaveCount(0);
 await expect.poll(()=>page.evaluate(()=>[...(window as any).__clashEngine.solidBindings.values()].filter((b:any)=>b.group.visible).length)).toBe(3);
 await clashCommand(page,'距离和区域分析');let distance=page.getByRole('dialog',{name:'距离和区域分析',exact:true});await distance.getByRole('textbox',{name:'分析名称'}).fill('安装距离');await distance.getByRole('button',{name:/^分\s*析$/}).click();await expect(distance.getByRole('button',{name:/^保\s*存$/})).toBeEnabled();await distance.getByRole('button',{name:/^保\s*存$/}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses?.length).toBe(3);await distance.getByRole('button',{name:'关闭',exact:true}).click();
 await clashCommand(page,'距离和区域分析');distance=page.getByRole('dialog',{name:'距离和区域分析',exact:true});await distance.getByRole('combobox',{name:'分析方式',exact:true}).click();await page.locator('.ant-select-dropdown:visible').getByText('距离区域',{exact:true}).click();
 await distance.getByRole('button',{name:'选择第一组',exact:true}).click();await clickClashInstance(page,seed.ground);await expect(distance.getByRole('list',{name:'第一组列表'})).toContainText('Clash验收块.1');
 await distance.getByRole('button',{name:'选择第二组',exact:true}).click();await clickClashInstance(page,seed.moving);await expect(distance.getByRole('list',{name:'第二组列表'})).toContainText('Clash验收块.2');await distance.getByRole('button',{name:'选择第二组',exact:true}).click();
 await distance.getByRole('spinbutton',{name:'最大距离',exact:true}).fill('5');await distance.getByRole('button',{name:/^分\s*析$/}).click();await expect(distance.getByRole('button',{name:/^保\s*存$/})).toBeEnabled();await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.staticInterferenceMarkers?.children.some((c:any)=>c.geometry?.type==='WireframeGeometry'))).toBeTruthy();
 await testInfo.attach('distance-band',{body:await page.screenshot({path:testInfo.outputPath('spatial-band.png')}),contentType:'image/png'});await distance.getByRole('button',{name:'关闭',exact:true}).click();
 const accepted=await current(page,seed.id);expect(accepted.product.instances).toEqual(before.product.instances);expect(accepted.product.spatialAnalyses.every((a:any)=>a.status==='CURRENT')).toBeTruthy();
 // A single tree selection also loads exact overlap graphics, without an editor.
 await page.reload();await treeNavigator(page)('安装碰撞');await page.getByText('安装碰撞',{exact:true}).click();await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine?.staticInterferenceMarkers?.children.some((c:any)=>c.geometry?.type==='WireframeGeometry'))).toBeTruthy();
 // Applications tree nodes survive reopen, and their editor restores evidence.
 await page.reload();await treeNavigator(page)('安装截面');await page.getByText('安装截面',{exact:true}).dblclick();section=page.getByRole('dialog',{name:'切割',exact:true});await expect(section.getByRole('switch',{name:'截面视图',exact:true})).toBeVisible();await expect(section.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await expect(section.getByRole('spinbutton',{name:'切割宽',exact:true})).toHaveCount(0);await expect(section.getByRole('spinbutton',{name:'切割高',exact:true})).toHaveCount(0);await expect(section).not.toContainText('分析完成');
 {await section.getByRole('switch',{name:'截面视图',exact:true}).click();const sheet=page.getByRole('dialog',{name:'截面视图',exact:true});const svg=sheet.getByRole('img',{name:'剖切二维视图'});await expect(svg).toBeVisible();const initialBox=await svg.getAttribute('viewBox');await svg.hover();await page.mouse.wheel(0,-200);await expect(svg).not.toHaveAttribute('viewBox',initialBox!);await page.screenshot({path:testInfo.outputPath('section-independent-view.png')});await sheet.getByRole('button',{name:'关闭',exact:true}).click();}
 await expect(section.locator('tbody tr.ant-table-row')).toHaveCount(3);await section.getByRole('button',{name:/^取\s*消$/,exact:true}).click();
 await page.evaluate(async({id,moving})=>{const {restApi:api}=await import('/src/api.ts');await api.command(id,{type:'MOVE_INSTANCE',instanceId:moving,translation:[12,0,0],rotation:[0,0,0,1]})},{id:seed.id,moving:seed.moving});await page.reload();await treeNavigator(page)('安装截面');await page.getByText('安装截面',{exact:true}).dblclick();section=page.getByRole('dialog',{name:'切割',exact:true});await expect(section.getByRole('button',{name:/^分\s*析$/})).toHaveCount(0);
 const moved=await current(page,seed.id);expect(moved.product.spatialAnalyses.find((a:any)=>a.name==='安装截面').status).toBe('STALE');await expect(section.getByRole('switch',{name:'截面视图',exact:true})).toBeVisible();await expect(section.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await section.getByRole('button',{name:/^确\s*定$/}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses.find((a:any)=>a.name==='安装截面').status).toBe('CURRENT');await expect(section).toHaveCount(0);
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses.find((a:any)=>a.name==='安装截面').status).toBe('STALE');await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses.find((a:any)=>a.name==='安装截面').status).toBe('CURRENT');expect((await current(page,seed.id)).product.instances).toEqual(moved.product.instances);
 // Update the entire saved category through its actual structure-tree action.
 await treeNavigator(page)('空间分析');await page.getByText('空间分析',{exact:true}).filter({hasNot:page.locator('button')}).last().click({button:'right'});
 await page.getByRole('menuitem',{name:'对象',exact:true}).hover();await page.getByRole('menuitem',{name:'更新全部分析',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses.every((a:any)=>a.status==='CURRENT')).toBeTruthy();expect((await current(page,seed.id)).product.instances).toEqual(moved.product.instances);
 expect(errors).toEqual([]);await testInfo.attach('saved-spatial-document',{body:JSON.stringify({id:seed.id}),contentType:'application/json'});
});

test('spatial commands open and save without native crypto.randomUUID',async({page})=>{
 test.setTimeout(180_000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await login(page);const seed=await clashSeed(page);
 // Match HTTP LAN capability availability before any application module loads.
 await page.addInitScript(()=>Object.defineProperty(globalThis.crypto,'randomUUID',{configurable:true,value:undefined}));
 await page.goto('/documents/'+seed.id);expect(await page.evaluate(()=>typeof crypto.randomUUID)).toBe('undefined');expect(await page.evaluate(()=>typeof crypto.getRandomValues)).toBe('function');
 const before=await current(page,seed.id);
 await clashCommand(page,'切割');let section=page.getByRole('dialog',{name:'切割',exact:true});await expect(section).toBeVisible();await expect(page.getByRole('button',{name:'切割',exact:true})).toHaveAttribute('aria-pressed','true');await section.getByRole('button',{name:/^取\s*消$/,exact:true}).click();await expect(page.getByRole('button',{name:'切割',exact:true})).toHaveAttribute('aria-pressed','false');expect((await current(page,seed.id)).document.versionId).toBe(before.document.versionId);
 for(const name of ['切割','距离和区域分析','碰撞']){
  await clashCommand(page,name);const dialog=page.getByRole('dialog',{name,exact:true});await expect(dialog).toBeVisible();
  if(name==='切割')await dialog.getByRole('spinbutton',{name:'位置 Z',exact:true}).fill('5');
  if(name!=='切割')await dialog.getByRole('button',{name:name==='碰撞'?/^检\s*查$/:/^分\s*析$/}).click();const confirm=dialog.getByRole('button',{name:name==='距离和区域分析'?/^保\s*存$/:/^确\s*定$/});await expect(confirm).toBeEnabled();
  const count=(await current(page,seed.id)).product.spatialAnalyses?.length??0;await confirm.click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses?.length).toBe(count+1);if(name!=='距离和区域分析')await expect(dialog).toHaveCount(0);else await dialog.getByRole('button',{name:'关闭',exact:true}).click();await expect(page.getByRole('button',{name,exact:true})).toHaveAttribute('aria-pressed','false');
 }
 const saved=await current(page,seed.id);expect(saved.product.spatialAnalyses).toHaveLength(3);expect(new Set(saved.product.spatialAnalyses.map((a:any)=>a.id)).size).toBe(3);for(const a of saved.product.spatialAnalyses)expect(a.id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);expect(saved.product.instances).toEqual(before.product.instances);expect(errors).toEqual([]);
});

test('section handles move rotate resize and position exact targets without writing until save',async({page},testInfo)=>{
 test.setTimeout(240_000);page.setDefaultTimeout(20_000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await observeClash(page);await login(page);const seed=await clashSeed(page);await page.goto('/documents/'+seed.id);const before=await current(page,seed.id);
 let analysisRequests=0;page.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith('/spatial-analysis'))analysisRequests++});
 await clashCommand(page,'切割');let dialog=page.getByRole('dialog',{name:'切割',exact:true});await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine?.sectionManipulator?.root.visible)).toBeTruthy();
 await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await page.waitForTimeout(260);expect(analysisRequests).toBe(1);
 const z=dialog.getByRole('spinbutton',{name:'位置 Z',exact:true}),initialZ=Number(await z.inputValue());
 const handle=async(operation:string,axis?:string)=>page.evaluate(({operation,axis})=>{const e=(window as any).__clashEngine,m=e.sectionManipulator.gizmo,canvas=e.renderer.domElement,b=canvas.getBoundingClientRect();m.updateScale(e.camera,{cssWidth:b.width,cssHeight:b.height,devicePixelRatio:1});m.root.updateMatrixWorld(true);for(const h of m.handles){if(h.operation!==operation||axis&&h.axis!==axis)continue;const p=h.pick.getWorldPosition(e.camera.position.clone().set(0,0,0)).project(e.camera),x=(p.x+1)*b.width/2,y=(1-p.y)*b.height/2;if(document.elementFromPoint(b.x+x,b.y+y)===canvas&&m.pick(x,y,e.camera,canvas)===h){const origin=m.object.getWorldPosition(e.camera.position.clone().set(0,0,0)),axisVector=e.camera.position.clone().set(h.axis==='X'?1:0,h.axis==='Y'?1:0,h.axis==='Z'?1:0).applyQuaternion(m.frameQuaternion()),world=h.pick.getWorldPosition(e.camera.position.clone().set(0,0,0));const end=world.sub(origin).applyAxisAngle(axisVector,.35).add(origin).project(e.camera);return {x:b.x+x,y:b.y+y,axis:h.axis,endX:b.x+(end.x+1)*b.width/2,endY:b.y+(1-end.y)*b.height/2}}}throw Error('No exposed section handle: '+operation+' '+axis)}, {operation,axis});
 const h=await handle('translate','Z');await page.mouse.move(h.x,h.y);await page.mouse.down();await page.mouse.move(h.x,h.y-36,{steps:8});await expect.poll(async()=>Math.abs(Number(await z.inputValue())-initialZ)).toBeGreaterThan(.2);await page.keyboard.press('Escape');await page.mouse.up();await expect.poll(async()=>Math.abs(Number(await z.inputValue())-initialZ)).toBeLessThan(1e-8);await expect(dialog).toBeVisible();
 const grabPlane=async()=>page.evaluate(()=>{const e=(window as any).__clashEngine,m=e.sectionManipulator,d=m.definition(),canvas=e.renderer.domElement,b=canvas.getBoundingClientRect(),V=e.camera.position.constructor,Q=e.camera.quaternion.constructor,q=new Q().fromArray(d.frame.rotation),origin=new V().fromArray(d.frame.translation);for(const u of [-.3,-.2,.2,.3])for(const v of [-.3,.3]){const p=new V(u*d.size[0],v*d.size[1],0).applyQuaternion(q).add(origin),screen=p.clone().project(e.camera),x=(screen.x+1)*b.width/2,y=(1-screen.y)*b.height/2;if(document.elementFromPoint(b.x+x,b.y+y)!==canvas||m.gizmo.pick(x,y,e.camera,canvas)||m.directHit(x,y)?.axis!==undefined)continue;const projected=(delta:any)=>{const s=p.clone().add(delta.applyQuaternion(q)).project(e.camera);return {x:b.x+(s.x+1)*b.width/2,y:b.y+(1-s.y)*b.height/2}};return {x:b.x+x,y:b.y+y,w:projected(new V(0,0,2)),uv:projected(new V(2,1,0))}}throw Error('No exposed primary plane')});
 const face=await grabPlane();await page.mouse.move(face.x,face.y);await page.mouse.down();await page.mouse.move(face.w.x,face.w.y,{steps:6});await expect.poll(async()=>Number(await z.inputValue())).toBeCloseTo(initialZ+2,5);await page.keyboard.press('Escape');await page.mouse.up();await expect.poll(async()=>Number(await z.inputValue())).toBeCloseTo(initialZ,5);
 const uv=await grabPlane();await page.mouse.move(uv.x,uv.y);await page.mouse.down({button:'left'});await page.mouse.down({button:'middle'});await page.mouse.move(uv.uv.x,uv.uv.y,{steps:6});await page.mouse.up({button:'middle'});await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.sectionManipulator.isDragging())).toBeTruthy();await page.mouse.up({button:'left'});await expect.poll(async()=>Number(await z.inputValue())).toBeCloseTo(initialZ,5);await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.sectionManipulator.definition().frame.translation[1])).toBeCloseTo(1,5);
 await dialog.getByRole('button',{name:'复位位置',exact:true}).click();
 await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();const requestsBeforeDrag=analysisRequests;
 const t=await handle('translate','Z');await page.mouse.move(t.x,t.y);await page.mouse.down();await page.mouse.move(t.x,t.y-20,{steps:6});await expect.poll(()=>analysisRequests).toBeGreaterThan(requestsBeforeDrag);await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.staticInterferenceMarkers?.children.length??0)).toBeGreaterThan(0);await page.mouse.up();await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await page.waitForTimeout(260);expect(analysisRequests).toBeGreaterThan(requestsBeforeDrag);expect((await current(page,seed.id)).document.versionId).toBe(before.document.versionId);
 await dialog.getByRole('button',{name:'复位位置',exact:true}).click();await expect.poll(async()=>Number(await z.inputValue())).toBeCloseTo(initialZ,8);
 const r=await handle('rotate');await page.mouse.move(r.x,r.y);await page.mouse.down();await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.sectionManipulator.gizmo.drag?.handle.operation)).toBe('rotate');await page.mouse.move(r.endX,r.endY,{steps:8});await page.mouse.up();await expect.poll(()=>page.evaluate(()=>{const q=(window as any).__clashEngine.sectionManipulator.definition().frame.rotation;return Math.abs(q[0])+Math.abs(q[1])+Math.abs(q[2])})).toBeGreaterThan(.01);
 await dialog.getByRole('button',{name:'复位位置',exact:true}).click();
 // Grab an exposed plane edge rather than the numeric size editor.
 const edge=await page.evaluate(()=>{const e=(window as any).__clashEngine,m=e.sectionManipulator,d=m.definition(),canvas=e.renderer.domElement,b=canvas.getBoundingClientRect(),THREE=(e.camera.position.constructor as any),Q=(e.camera.quaternion.constructor as any);for(const i of [0,1])for(const side of [-1,1]){const p=new THREE(i===0?side*d.size[0]/2:0,i===1?side*d.size[1]/2:0,0).applyQuaternion(new Q().fromArray(d.frame.rotation)).add(new THREE().fromArray(d.frame.translation)),v=p.clone().project(e.camera),x=(v.x+1)*b.width/2,y=(1-v.y)*b.height/2;if(document.elementFromPoint(b.x+x,b.y+y)===canvas){const hit=m.directHit(x,y);if(hit?.axis===i)return {x:b.x+x,y:b.y+y,index:i,size:d.size[i]}}}throw Error('no exposed section edge')});
 await page.mouse.move(edge.x,edge.y);await page.mouse.down();await page.mouse.move(edge.x-35,edge.y-15,{steps:8});await page.mouse.up();await expect.poll(()=>page.evaluate(i=>(window as any).__clashEngine.sectionManipulator.definition().size[i],edge.index)).not.toBeCloseTo(edge.size,2);
 await dialog.getByRole('button',{name:'选择几何定位',exact:true}).click();await clickClashInstance(page,seed.ground);await expect(dialog.getByRole('button',{name:'选择几何定位',exact:true})).toHaveAttribute('aria-pressed','false');await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await dialog.getByRole('switch',{name:'截面视图',exact:true}).click();const sectionWindow=page.getByRole('dialog',{name:'截面视图',exact:true});await expect(sectionWindow.getByRole('img',{name:'剖切二维视图',exact:true})).toBeVisible();await sectionWindow.getByRole('button',{name:'关闭',exact:true}).click();
 // A real topology vertex is resolved authoritatively; point placement keeps the current normal.
 const rotationBeforePoint=await page.evaluate(()=>(window as any).__clashEngine.sectionManipulator.definition().frame.rotation);
 await dialog.getByRole('button',{name:'选择几何定位',exact:true}).click();
 const pickedPoint=await page.evaluate(()=>{const e=(window as any).__clashEngine;for(const binding of e.selectionIndex.picks){if(!binding.root.isPoints)continue;const point=binding.root.geometry.attributes.position;if(!point?.count)continue;const selection=binding.resolve({object:binding.root,index:0,point:e.camera.position.clone().set(point.getX(0),point.getY(0),point.getZ(0)).applyMatrix4(binding.root.matrixWorld)});if(selection?.kind==='vertex'&&e.featureSelection.accept(selection)){e.featureSelection.onPick(selection);return selection}}throw Error('no accepted vertex binding')});
 await expect(dialog.getByRole('button',{name:'选择几何定位',exact:true})).toHaveAttribute('aria-pressed','false');await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();expect(await page.evaluate(()=>(window as any).__clashEngine.sectionManipulator.definition().frame.rotation)).toEqual(rotationBeforePoint);expect(pickedPoint.topologyId).toBeDefined();
 // A late support measurement must not restore a cancelled replacement.
 const acceptedFrame=await page.evaluate(()=>(window as any).__clashEngine.sectionManipulator.definition().frame);
 let targetRelease!:()=>void,targetSeen!:()=>void;const targetHold=new Promise<void>(r=>targetRelease=r),targetStarted=new Promise<void>(r=>targetSeen=r);
 await page.route(`**/api/documents/${seed.id}/assembly-supports/inspect`,async route=>{const response=await route.fetch();targetSeen();await targetHold;await route.fulfill({response}).catch(()=>{})});
 await dialog.getByRole('button',{name:'选择几何定位',exact:true}).click();await clickClashInstance(page,seed.moving);await targetStarted;await page.keyboard.press('Escape');targetRelease();await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:'选择几何定位',exact:true})).toHaveAttribute('aria-pressed','false');await page.waitForTimeout(100);expect(await page.evaluate(()=>(window as any).__clashEngine.sectionManipulator.definition().frame)).toEqual(acceptedFrame);await page.unroute(`**/api/documents/${seed.id}/assembly-supports/inspect`);
 await dialog.getByRole('combobox',{name:'三维切除',exact:true}).click();await page.locator('.ant-select-dropdown:visible').getByText('内侧',{exact:true}).click();await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.sectionMaterials.size)).toBe(3);await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();

 await expect(dialog.getByRole('switch',{name:'截面视图',exact:true})).toBeVisible();
 await testInfo.attach('section-manipulator',{body:await page.screenshot({path:testInfo.outputPath('section-manipulator.png')}),contentType:'image/png'});
 const draft=await page.evaluate(()=>(window as any).__clashEngine.sectionManipulator.definition());await dialog.getByRole('button',{name:/^确\s*定$/}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses?.length).toBe(1);const saved=await current(page,seed.id),analysis=saved.product.spatialAnalyses[0];expect(analysis.frame).toEqual(draft.frame);expect(analysis.size).toEqual(draft.size);expect(saved.product.instances).toEqual(before.product.instances);
 await expect(dialog).toHaveCount(0);await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.sectionManipulator.root.visible)).toBeFalsy();expect(await page.evaluate(()=>({materials:(window as any).__clashEngine.sectionMaterials.size,edges:(window as any).__clashEngine.sectionEdgeVisibility.size}))).toEqual({materials:0,edges:0});await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses?.length??0).toBe(0);await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses?.[0]?.id).toBe(analysis.id);await page.reload();await treeNavigator(page)(analysis.name);await page.getByText(analysis.name,{exact:true}).dblclick();dialog=page.getByRole('dialog',{name:'切割',exact:true});await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.sectionManipulator.definition().frame)).toEqual(analysis.frame);await dialog.getByRole('button',{name:/^取\s*消$/,exact:true}).click();expect(errors).toEqual([]);
});

test('section cap shader preserves a real bore for both cut sides at DPR 1 and 2',async({page},testInfo)=>{
 test.setTimeout(180_000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',e=>{if(e.type()==='error'&&/Shader|WebGLProgram/.test(e.text()))errors.push(e.text())});await observeClash(page);await login(page);
 const id=await page.evaluate(async()=>{
  // @ts-ignore production API module served by Vite
  const {restApi:api}=await import('/src/api.ts');let part=await api.createDocument('PART','截面孔洞验收');const pid=part.document.id;part=await api.command(pid,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=part.part!.features.at(-1)!.id;
  part=await api.command(pid,{type:'EDIT_SKETCH',sketchId,operations:[6,3].map((radius,i)=>({type:'ADD_ENTITY',entity:{id:`ring-${i}`,kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius}}))});await api.command(pid,{type:'CREATE_SOLID_FEATURE',sketchId,generator:'LINEAR_EXTRUDE',operation:'ADD',length:10});await api.command(pid,{type:'CREATE_REFERENCE_POINT',name:'切面定位点',referenceConstruction:{mode:'POINT_COORDINATES',coordinates:[0,0,5]}});let product=await api.createDocument('PRODUCT','切口斜线验收');product=await api.command(product.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:pid});return product.document.id;
 });
 await page.goto('/documents/'+id);await clashCommand(page,'切割');const dialog=page.getByRole('dialog',{name:'切割',exact:true});await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await dialog.getByRole('combobox',{name:'三维切除',exact:true}).click();await page.locator('.ant-select-dropdown:visible').getByText('内侧',{exact:true}).click();await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.sectionCaps.root.children.filter((c:any)=>c.material?.userData.sectionHatch).length)).toBe(1);
 await dialog.getByRole('button',{name:'选择几何定位',exact:true}).click();await treeNavigator(page)('切面定位点');await page.getByText('切面定位点',{exact:true}).click();await expect(dialog.getByRole('button',{name:'选择几何定位',exact:true})).toHaveAttribute('aria-pressed','false');await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();expect(await page.evaluate(()=>(window as any).__clashEngine.sectionManipulator.definition().frame)).toEqual({translation:[0,0,5],rotation:[0,0,0,1]});
 // Render the actual accepted holed Body through the production cap helper.
 // An isolated draw makes background, bore and material samples deterministic.
 const samples=await page.evaluate(async()=>{
  // @ts-ignore production rendering module
  const {SectionCaps}=await import('/src/cad/rendering/section-caps.ts');
  // @ts-ignore production projection helper
  const {convertProjection}=await import('/src/cad/navigation/projection-camera.ts');const e=(window as any).__clashEngine,renderer=e.renderer,source=[...e.solidBindings.values()][0].mesh,scene=new e.scene.constructor(),caps=new SectionCaps();let camera=e.camera.clone();camera.position.set(0,0,-40);camera.up.set(0,1,0);camera.lookAt(0,0,5);camera.aspect=1;camera.updateProjectionMatrix();camera.updateMatrixWorld(true);scene.add(caps.root);
  const size=renderer.getSize(e.camera.position.clone().set(0,0,0)),ratio=renderer.getPixelRatio(),clear=renderer.getClearColor((Array.isArray(source.material)?source.material[0]:source.material).color.clone()),alpha=renderer.getClearAlpha(),out:any[]=[];renderer.setClearColor(0,1);
  try{for(const dpr of [1,2])for(const projection of ['PERSPECTIVE','ORTHOGRAPHIC'])for(const sectionType of ['PLANE','SLICE','BOX'])for(const mode of ['KEEP_INSIDE','KEEP_OUTSIDE']){
   camera=convertProjection(camera,camera.position.clone().set(0,0,5),projection,1);
   renderer.setPixelRatio(dpr);renderer.setSize(320,320,false);const d={...e.sectionManipulator.definition(),size:[16,16,4],sectionType,cutMode:mode};caps.update(d,[source]);if(!caps.root.parent)scene.add(caps.root);renderer.clear(true,true,true);renderer.render(scene,camera);const gl=renderer.getContext();
   const sample=(x:number,y:number)=>{const p=camera.position.clone().set(x,y,5).project(camera),pixel=new Uint8Array(4);gl.readPixels(Math.floor((p.x+1)*160*dpr),Math.floor((p.y+1)*160*dpr),1,1,gl.RGBA,gl.UNSIGNED_BYTE,pixel);return [...pixel].slice(0,3)};
   out.push({dpr,mode,sectionType,projection,hole:sample(0,0),ring:sample(4.5,0),outside:sample(9,0),stencil:gl.getContextAttributes().stencil,drawCalls:renderer.info.render.calls});
  }}finally{caps.clear();renderer.setPixelRatio(ratio);renderer.setSize(size.x,size.y,false);renderer.setClearColor(clear,alpha);e.invalidate()}return out;
 });
 for(const sample of samples){expect(sample.stencil).toBeTruthy();expect(sample.hole).toEqual([0,0,0]);expect(sample.outside).toEqual([0,0,0]);expect(Math.max(...sample.ring)).toBeGreaterThan(40);expect(sample.drawCalls).toBe(sample.sectionType==='BOX'?18:sample.sectionType==='SLICE'?6:3)}
 await testInfo.attach('cap-raster-samples',{body:JSON.stringify(samples,null,2),contentType:'application/json'});await page.screenshot({path:testInfo.outputPath('section-bore-hatch.png')});await dialog.getByRole('combobox',{name:'三维切除',exact:true}).click();await page.locator('.ant-select-dropdown:visible').getByText('外侧',{exact:true}).click();await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await page.screenshot({path:testInfo.outputPath('section-bore-outside.png')});await dialog.getByRole('button',{name:/^取\s*消$/,exact:true}).click();expect(errors).toEqual([]);
});
