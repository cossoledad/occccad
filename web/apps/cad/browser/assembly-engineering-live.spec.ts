import {expect,test,type Page} from '@playwright/test';
async function login(page:Page,email=process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password=process.env.OCCCCAD_ADMIN_PASSWORD){expect(password).toBeTruthy();const r=await page.request.post('/api/auth/login',{data:{email,password}});expect(r.ok()).toBeTruthy();await page.goto('/');}
async function current(page:Page,id:string){return (await page.request.get(`/api/documents/${id}`)).json();}
async function openMotion(page:Page){await page.getByRole('button',{name:'搜索工具',exact:true}).click();await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill('约束分析');await page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:/^约束分析$/})}).click();const dialog=page.getByRole('dialog',{name:/装配状态 ·/});await dialog.getByRole('tab',{name:'剩余运动',exact:true}).click();return dialog;}
async function observeGuides(page:Page){
 // Same production-module observation used by the neighboring motion fixture.
 await page.route('**/src/viewport/cad-viewport-engine.ts*',async route=>{const response=await route.fetch();await route.fulfill({response,body:await response.text()+`\nconst show=CadViewportEngine.prototype.showRemainingMotion;CadViewportEngine.prototype.showRemainingMotion=function(...args){window.__engineeringEngine=this;return show.apply(this,args)};`});});
}
const guides=(page:Page)=>page.evaluate(()=>(window as any).__engineeringEngine?.motionMarkers?.children.length??0);

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
 await expect(state()).toContainText('待计算');await expect(dialog.getByRole('button',{name:'刷新剩余运动',exact:true})).toBeEnabled();
 const refresh=async()=>{const revision=(await current(page,seed.id)).document.versionId;await expect(dialog).not.toContainText(revision);await dialog.getByRole('button',{name:/^(刷新剩余运动|重\s*试)$/}).click();await expect(state()).toContainText('分析完成');};
 await refresh();await expect(state()).toContainText('覆盖 5 / 5');await expect(state()).toContainText('整体 gauge 6');
 const names=before.product.instances.map((i:any)=>i.name);const shaft=()=>dialog.locator('tbody tr').filter({hasText:names[1]});await shaft().click();await expect(dialog).toContainText('沿轴平移并绕该轴旋转');await expect.poll(()=>guides(page)).toBeGreaterThan(0);
 await page.screenshot({path:testInfo.outputPath('accepted-pose-motion-directions.png')});
 let after=await current(page,seed.id);expect(after.document.versionId).toBe(before.document.versionId);expect(after.product).toEqual(before.product);expect([after.document.canUndo,after.document.canRedo]).toEqual([before.document.canUndo,before.document.canRedo]);
 const peer=await page.context().newPage();await peer.goto('/');
 const edited=await peer.evaluate(async s=>{
  // @ts-ignore existing formal Preview/Commit chain, independent of read-only analysis
  const {restApi:api}=await import('/src/api.ts');const input={type:'EDIT_ASSEMBLY_CONSTRAINT',targetId:s.fixed,fixMode:'SPACE',fixedPose:{translation:[5,0,0],rotation:[0,0,0,1]}};
  const preview=await api.previewCommand(s.id,input);return api.command(s.id,{...input,previewId:preview.previewId});
 },seed);
 await expect(state()).toContainText(edited.document.versionId);await refresh();const afterEditedAnalysis=await current(page,seed.id);expect(afterEditedAnalysis.product).toEqual(edited.product);expect(afterEditedAnalysis.document.versionId).toBe(edited.document.versionId);
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
 await expect(state()).toContainText(changed.document.versionId);release();await expect(state()).toContainText('待计算');await expect.poll(()=>guides(page)).toBe(0);await page.unroute(pattern);await refresh();
 const stale=await page.evaluate(async s=>{
  // @ts-ignore production API adapter supplies normal CSRF/session headers
  const {restApi:api}=await import('/src/api.ts');try{await api.refreshAssemblyEngineeringEvidence(s.id,s.version);return "accepted"}catch(error){return String(error)}
 },seed);expect(stale).toContain('ASSEMBLY_EVIDENCE_STALE');
 await dialog.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('button',{name:'撤销',exact:true}).click();dialog=await openMotion(page);await refresh();await dialog.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('button',{name:'重做',exact:true}).click();dialog=await openMotion(page);await refresh();
 await page.reload();dialog=await openMotion(page);await expect(state()).toContainText('分析完成');await shaft().click();await expect.poll(()=>guides(page)).toBeGreaterThan(0);
 // A genuine Viewer can compute without invoking any Domain Command.
 const viewerContext=await browser.newContext({baseURL:'http://127.0.0.1:5175',viewport:{width:1440,height:1000}});const viewer=await viewerContext.newPage();await observeGuides(viewer);await login(viewer,seed.email,seed.password);await viewer.goto(`/documents/${seed.id}`);const vd=await openMotion(viewer);await expect(vd.getByRole('button',{name:'刷新剩余运动',exact:true})).toBeEnabled();
 const priorViewer=await current(viewer,seed.id);expect(priorViewer.document.permission).toBe('VIEWER');await vd.getByRole('button',{name:'刷新剩余运动',exact:true}).click();await expect(vd.getByRole('status',{name:'剩余运动计算状态'})).toContainText('分析完成');const postViewer=await current(viewer,seed.id);expect(postViewer.product).toEqual(priorViewer.product);expect(postViewer.document.versionId).toBe(priorViewer.document.versionId);await viewerContext.close();
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

test('clash exact local graphics, viewport scope, temporary isolate/section and late cancellation',async({page},testInfo)=>{
 test.setTimeout(240_000);page.setDefaultTimeout(20000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await observeClash(page);await login(page);const seed=await clashSeed(page);await page.goto('/documents/'+seed.id);
 await clashCommand(page,'碰撞');let dialog=page.getByRole('dialog',{name:'碰撞',exact:true});
 const before=await current(page,seed.id),history=await (await page.request.get(`/api/documents/${seed.id}/history`)).json();
 await dialog.getByRole('button',{name:/^检\s*查$/}).click();await expect(dialog.getByRole('status',{name:'干涉检查状态'})).toContainText('发现需要检查');
 const row=dialog.locator('tbody tr').filter({hasText:'干涉'});await expect(row).toHaveCount(1);let detailRelease!:()=>void,detailSeen!:()=>void;const detailHold=new Promise<void>(r=>detailRelease=r),detailStarted=new Promise<void>(r=>detailSeen=r);
 await page.route(`**/api/documents/${seed.id}/static-interference`,async route=>{const response=await route.fetch();if(route.request().postDataJSON().detailPair){detailSeen();await detailHold}await route.fulfill({response}).catch(()=>{})});
 await row.click();await detailStarted;await dialog.getByRole('checkbox',{name:'只看这一对',exact:true}).check();detailRelease();
 await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__clashEngine;return e?.staticInterferenceReport?.pairs.some((p:any)=>p.overlapTriangles?.length>0&&p.sectionPoints?.length>0)})).toBeTruthy();
 await dialog.getByRole('checkbox',{name:'只看这一对',exact:true}).check();await expect.poll(()=>page.evaluate(()=>[...(window as any).__clashEngine.solidBindings.values()].filter((b:any)=>b.group.visible).length)).toBe(2);
 await dialog.getByRole('checkbox',{name:'剖切',exact:true}).check();await expect.poll(()=>page.evaluate(()=>[...(window as any).__clashEngine.solidBindings.values()].filter((b:any)=>b.group.visible).length)).toBe(0);
 await page.unroute(`**/api/documents/${seed.id}/static-interference`);
 await dialog.getByRole('button',{name:'查看几何',exact:true}).click();await expect(dialog).toHaveAttribute('data-panel-size','S');
 await testInfo.attach('clash-section',{body:await page.screenshot({path:testInfo.outputPath('clash-view.png')}),contentType:'image/png'});
 await dialog.getByRole('button',{name:'返回列表',exact:true}).click();
 await dialog.getByRole('checkbox',{name:'剖切',exact:true}).uncheck();await dialog.getByRole('checkbox',{name:'只看这一对',exact:true}).uncheck();await expect.poll(()=>page.evaluate(()=>[...(window as any).__clashEngine.solidBindings.values()].filter((b:any)=>b.group.visible).length)).toBe(3);
 await dialog.getByRole('combobox',{name:'检查范围'}).click();await page.locator('.ant-select-dropdown:visible').getByText('所选零件与其余零件',{exact:true}).click();
 // Pick actual canvas pixels through the production hit tester/session.
 await clickClashInstance(page,seed.ground);await dialog.getByRole('button',{name:/^检\s*查$/}).click();await expect(dialog.getByRole('status',{name:'干涉检查状态'})).toContainText('发现需要检查');await expect(dialog.getByText('2 / 2 对已完成')).toBeVisible();
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
 await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.staticInterferenceMarkers?.children.length??0)).toBeGreaterThan(0);
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
 await recorder.getByRole('spinbutton',{name:'关键帧时间',exact:true}).fill('1');await recorder.getByRole('button',{name:'插入',exact:true}).click();await expect(recorder.locator('tbody tr')).toHaveCount(2);
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
 await clashCommand(page,'碰撞');let clash=page.getByRole('dialog',{name:'碰撞',exact:true});await clash.getByRole('button',{name:/^检\s*查$/}).click();await expect(clash.getByRole('status',{name:'干涉检查状态'})).toContainText('发现需要检查');
 await clash.locator('tbody tr').filter({hasText:'干涉'}).click();await expect.poll(()=>page.evaluate(()=>{const g=(window as any).__clashEngine?.staticInterferenceMarkers;return g?.children.some((c:any)=>c.geometry?.type==='WireframeGeometry')})).toBeTruthy();
 // Role and overlap evidence use wires; the original engineering materials remain intact.
 expect(await page.evaluate(()=>(window as any).__clashEngine.staticInterferenceMarkers.children.some((c:any)=>c.isMesh&&!c.isLineSegments2))).toBeFalsy();
 await clash.getByRole('textbox',{name:'分析名称'}).fill('安装碰撞');await clash.getByRole('button',{name:/^保\s*存$/}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses?.length).toBe(1);await clash.getByRole('button',{name:'关闭',exact:true}).click();
 await clashCommand(page,'切割');let section=page.getByRole('dialog',{name:'切割',exact:true});await section.getByRole('textbox',{name:'分析名称'}).fill('安装截面');await section.getByRole('spinbutton',{name:'位置 Z',exact:true}).fill('5');await section.getByRole('button',{name:/^分\s*析$/}).click();await expect(section.getByRole('status').filter({hasText:'分析完成'})).toBeVisible();
 await expect(section.locator('tbody tr')).toHaveCount(3);await expect(section.locator('tbody tr').first()).toContainText('100');
 await section.getByRole('combobox',{name:'三维切除',exact:true}).click();await page.locator('.ant-select-dropdown:visible').getByText('保留正侧 / 内部',{exact:true}).click();await section.getByRole('button',{name:/^分\s*析$/}).click();await expect(section.getByRole('status').filter({hasText:'分析完成'})).toBeVisible();
 await expect.poll(()=>page.evaluate(()=>[...(window as any).__clashEngine.solidBindings.values()].filter((b:any)=>b.group.visible).length)).toBe(0);
 await testInfo.attach('section-clipped',{body:await page.screenshot({path:testInfo.outputPath('spatial-section.png')}),contentType:'image/png'});
 await section.getByRole('button',{name:/^保\s*存$/}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses?.length).toBe(2);await section.getByRole('button',{name:'关闭',exact:true}).click();
 await expect.poll(()=>page.evaluate(()=>[...(window as any).__clashEngine.solidBindings.values()].filter((b:any)=>b.group.visible).length)).toBe(3);
 await clashCommand(page,'距离和区域分析');let distance=page.getByRole('dialog',{name:'距离和区域分析',exact:true});await distance.getByRole('textbox',{name:'分析名称'}).fill('安装距离');await distance.getByRole('button',{name:/^分\s*析$/}).click();await expect(distance.getByRole('status').filter({hasText:'分析完成'})).toBeVisible();await distance.getByRole('button',{name:/^保\s*存$/}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses?.length).toBe(3);await distance.getByRole('button',{name:'关闭',exact:true}).click();
 await clashCommand(page,'距离和区域分析');distance=page.getByRole('dialog',{name:'距离和区域分析',exact:true});await distance.getByRole('combobox',{name:'分析方式',exact:true}).click();await page.locator('.ant-select-dropdown:visible').getByText('距离区域',{exact:true}).click();
 await distance.getByRole('button',{name:'选择第一组',exact:true}).click();await clickClashInstance(page,seed.ground);await expect(distance.getByRole('list',{name:'第一组列表'})).toContainText('Clash验收块.1');
 await distance.getByRole('button',{name:'选择第二组',exact:true}).click();await clickClashInstance(page,seed.moving);await expect(distance.getByRole('list',{name:'第二组列表'})).toContainText('Clash验收块.2');await distance.getByRole('button',{name:'选择第二组',exact:true}).click();
 await distance.getByRole('spinbutton',{name:'最大距离',exact:true}).fill('5');await distance.getByRole('button',{name:/^分\s*析$/}).click();await expect(distance.getByRole('status').filter({hasText:'分析完成'})).toBeVisible();await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine.staticInterferenceMarkers?.children.some((c:any)=>c.geometry?.type==='WireframeGeometry'))).toBeTruthy();
 await testInfo.attach('distance-band',{body:await page.screenshot({path:testInfo.outputPath('spatial-band.png')}),contentType:'image/png'});await distance.getByRole('button',{name:'关闭',exact:true}).click();
 const accepted=await current(page,seed.id);expect(accepted.product.instances).toEqual(before.product.instances);expect(accepted.product.spatialAnalyses.every((a:any)=>a.status==='CURRENT')).toBeTruthy();
 // Applications tree nodes survive reopen, and their editor restores evidence.
 await page.reload();await page.getByRole('textbox',{name:'筛选模型结构',exact:true}).fill('安装截面');await page.getByText('安装截面',{exact:true}).dblclick();section=page.getByRole('dialog',{name:'切割',exact:true});await expect(section.getByRole('status').filter({hasText:'分析完成'})).toBeVisible();await expect(section.locator('tbody tr')).toHaveCount(3);await section.getByRole('button',{name:'关闭',exact:true}).click();
 await page.evaluate(async({id,moving})=>{const {restApi:api}=await import('/src/api.ts');await api.command(id,{type:'MOVE_INSTANCE',instanceId:moving,translation:[12,0,0],rotation:[0,0,0,1]})},{id:seed.id,moving:seed.moving});await page.reload();await page.getByRole('textbox',{name:'筛选模型结构',exact:true}).fill('安装截面');await page.getByText('安装截面',{exact:true}).dblclick();section=page.getByRole('dialog',{name:'切割',exact:true});await expect(section.getByText('待更新',{exact:true})).toBeVisible();
 await expect.poll(()=>page.evaluate(()=>(window as any).__clashEngine?.staticInterferenceMarkers?.children.length??0)).toBe(0);
 const moved=await current(page,seed.id);await section.getByRole('button',{name:/^分\s*析$/}).click();await expect(section.getByRole('status').filter({hasText:'分析完成'})).toBeVisible();await section.getByRole('button',{name:/^保\s*存$/}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses.find((a:any)=>a.name==='安装截面').status).toBe('CURRENT');await section.getByRole('button',{name:'关闭',exact:true}).click();
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses.find((a:any)=>a.name==='安装截面').status).toBe('STALE');await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses.find((a:any)=>a.name==='安装截面').status).toBe('CURRENT');expect((await current(page,seed.id)).product.instances).toEqual(moved.product.instances);
 // Update the entire saved category through its actual structure-tree action.
 await page.getByRole('textbox',{name:'筛选模型结构',exact:true}).fill('空间分析');await page.getByText('空间分析',{exact:true}).filter({hasNot:page.locator('button')}).last().click({button:'right'});
 await page.getByRole('menuitem',{name:'对象',exact:true}).hover();await page.getByRole('menuitem',{name:'更新全部分析',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses.every((a:any)=>a.status==='CURRENT')).toBeTruthy();expect((await current(page,seed.id)).product.instances).toEqual(moved.product.instances);
 expect(errors).toEqual([]);await testInfo.attach('saved-spatial-document',{body:JSON.stringify({id:seed.id}),contentType:'application/json'});
});

test('spatial commands open and save without native crypto.randomUUID',async({page})=>{
 test.setTimeout(180_000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await login(page);const seed=await clashSeed(page);
 // Match HTTP LAN capability availability before any application module loads.
 await page.addInitScript(()=>Object.defineProperty(globalThis.crypto,'randomUUID',{configurable:true,value:undefined}));
 await page.goto('/documents/'+seed.id);expect(await page.evaluate(()=>typeof crypto.randomUUID)).toBe('undefined');expect(await page.evaluate(()=>typeof crypto.getRandomValues)).toBe('function');
 const before=await current(page,seed.id);
 await clashCommand(page,'切割');let section=page.getByRole('dialog',{name:'切割',exact:true});await expect(section).toBeVisible();await section.getByRole('button',{name:'关闭',exact:true}).click();expect((await current(page,seed.id)).document.versionId).toBe(before.document.versionId);
 for(const name of ['切割','距离和区域分析','碰撞']){
  await clashCommand(page,name);const dialog=page.getByRole('dialog',{name,exact:true});await expect(dialog).toBeVisible();
  if(name==='切割')await dialog.getByRole('spinbutton',{name:'位置 Z',exact:true}).fill('5');
  await dialog.getByRole('button',{name:name==='碰撞'?/^检\s*查$/:/^分\s*析$/}).click();await expect(dialog.getByRole('button',{name:/^保\s*存$/})).toBeEnabled();
  const count=(await current(page,seed.id)).product.spatialAnalyses?.length??0;await dialog.getByRole('button',{name:/^保\s*存$/}).click();await expect.poll(async()=>(await current(page,seed.id)).product.spatialAnalyses?.length).toBe(count+1);await dialog.getByRole('button',{name:'关闭',exact:true}).click();
 }
 const saved=await current(page,seed.id);expect(saved.product.spatialAnalyses).toHaveLength(3);expect(new Set(saved.product.spatialAnalyses.map((a:any)=>a.id)).size).toBe(3);for(const a of saved.product.spatialAnalyses)expect(a.id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);expect(saved.product.instances).toEqual(before.product.instances);expect(errors).toEqual([]);
});
