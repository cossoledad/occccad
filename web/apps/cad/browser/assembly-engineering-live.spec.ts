import {expect,test,type Page} from '@playwright/test';
async function login(page:Page,email=process.env.OCCCCAD_ADMIN_EMAIL??'admin@occccad.local',password=process.env.OCCCCAD_ADMIN_PASSWORD){expect(password).toBeTruthy();const r=await page.request.post('/api/auth/login',{data:{email,password}});expect(r.ok()).toBeTruthy();await page.goto('/');}
async function current(page:Page,id:string){return (await page.request.get(`/api/documents/${id}`)).json();}
async function openMotion(page:Page){await page.getByRole('button',{name:'搜索工具',exact:true}).click();await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill('装配约束分析');await page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:/^装配约束分析$/})}).click();const dialog=page.getByRole('dialog',{name:/装配状态 ·/});await dialog.getByRole('tab',{name:'剩余运动',exact:true}).click();return dialog;}
async function observeGuides(page:Page){
 // Same production-module observation used by the neighboring motion fixture.
 await page.route('**/src/viewport/cad-viewport-engine.ts*',async route=>{const response=await route.fetch();await route.fulfill({response,body:await response.text()+`\nconst show=CadViewportEngine.prototype.showRemainingMotion;CadViewportEngine.prototype.showRemainingMotion=function(...args){window.__engineeringEngine=this;return show.apply(this,args)};`});});
}
const guides=(page:Page)=>page.evaluate(()=>(window as any).__engineeringEngine?.motionMarkers?.children.length??0);

test('read-only remaining motion refresh freezes metadata Revision, cancels stale markers and works for Viewer',async({page,browser},testInfo)=>{
 test.setTimeout(240_000);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await observeGuides(page);await login(page);
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
