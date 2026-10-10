import {expect,test,type Page} from '@playwright/test';

// Browser contract fixtures replace API responses only in this test's page.
// No fixture is a numerical solver or an OCCT interference result.
async function fixture(page:Page,withDefinitions=false){
 await page.goto('/documents/mock-product-frame');
 await expect(page.locator('canvas')).toBeVisible();
 await page.evaluate(async seeded=>{
  const {api}=await import('/src/api/client.ts');
  const {queryClient}=await import('/src/app/providers.tsx');
  const documentId='mock-product-frame',originalGet=api.getDocument.bind(api),originalCommand=api.command.bind(api);
  const initial=await originalGet(documentId),instances=initial.product!.instances;
  const end=(instanceId:string)=>({instanceId,frame:{translation:[0,0,0],rotation:[0,0,0,1]},axis:{instanceId,kind:'AXIS',geometryId:'axis-system-default',axis:'Z'},plane:{instanceId,kind:'PLANE',geometryId:'datum-xy'},capturedX:[1,0,0]});
  let defs:any={mechanisms:[],drivers:[],studies:[],analyses:[],associations:[]};
  if(seeded){defs={...defs,mechanisms:[{id:'m',name:'测试机构',unitIds:instances.map(i=>i.id),joints:[{id:'g',name:'圆柱固定',kind:'GROUND',first:end(instances[0].id),zero:{value:0,unit:'deg'},direction:1},{id:'j',name:'轴接合',kind:'REVOLUTE',first:end(instances[0].id),second:end(instances[1].id),zero:{value:0,unit:'deg'},direction:1}]}],drivers:[{id:'d',name:'角度驱动',mechanismId:'m',jointId:'j'}],studies:[{id:'s',name:'旋转研究',mechanismId:'m',driverId:'d',start:{value:0,unit:'deg'},end:{value:360,unit:'deg'},durationSeconds:4,frames:5,budgetMs:60000,clearance:{value:0,unit:'mm'},checkDmu:false,includeSameUnit:false}]}}
  const state:any={saved:[],previews:[],requests:[],applied:[],inspected:[],jobs:[],blocked:false,release:undefined};
  (window as any).__motionFixture=state;
  let adopted:any;
  const decorate=(source:any)=>{
   const view=structuredClone(source);view.product.kinematics=structuredClone(defs);
   if(adopted){for(const i of view.product.instances)Object.assign(i,adopted[i.id]);for(const i of view.resolvedInstances??[])Object.assign(i,adopted[i.instancePath.segments[0].instanceId]);}
   const node=(kind:string,obj:any)=>({id:`application:${obj.id}`,kind,name:obj.name,entityId:obj.id,documentId,ownerDocumentId:documentId,versionId:view.document.versionId,subject:{documentId,entityKind:kind,entityId:obj.id},snapshot:{revisionId:view.document.versionId},capabilities:['EDIT','DELETE','RENAME']});
   view.structureTree.children=view.structureTree.children.filter((n:any)=>n.kind!=='APPLICATIONS');
   view.structureTree.children.push({id:'applications',kind:'APPLICATIONS',name:'Applications',documentId,versionId:view.document.versionId,children:[...defs.mechanisms.map((m:any)=>({...node('MECHANISM',m),children:[...m.joints.map((j:any)=>({...node('MECHANISM_JOINT',j),children:j.kind==='GROUND'?[]:[{...node('JOINT_EXPANSION',{id:j.id+'/axis',name:'同心'}),presentationRole:'INPUT_REFERENCE',capabilities:['EDIT']},{...node('JOINT_EXPANSION',{id:j.id+'/axial-location',name:'偏移'}),presentationRole:'INPUT_REFERENCE',capabilities:['EDIT']}]})),...defs.drivers.filter((d:any)=>d.mechanismId===m.id).map((d:any)=>node('MOTION_DRIVER',d)),...defs.studies.filter((s:any)=>s.mechanismId===m.id).map((s:any)=>node('MOTION_STUDY',s))]})),...defs.analyses.map((a:any)=>node('INTERFERENCE_ANALYSIS',a))]});
   for(let n=0;n<2;n++){
    const resolved=view.resolvedInstances.find((r:any)=>r.instancePath.segments[0].instanceId===instances[n].id);
    const path=structuredClone(resolved.instancePath);path.segments[0].ownerVersionId=view.document.versionId;
    view.structureTree.children.push({id:'supports-'+n,kind:'INSTANCE',name:'选择支持 '+n,documentId:instances[n].documentId,entityId:instances[n].id,instancePath:path,children:[
     {id:'axis-'+n,kind:'AXIS',name:'试验轴线 '+n,entityId:'axis-system-default',axis:'Z',documentId:instances[n].documentId,versionId:instances[n].versionId,instancePath:path},
     {id:'plane-'+n,kind:'PLANE',name:'试验平面 '+n,entityId:'datum-xy',plane:{origin:[0,0,0],normal:[0,0,1],xDirection:[1,0,0]},documentId:instances[n].documentId,versionId:instances[n].versionId,instancePath:path}]});
   }
   return view;
  };
  api.getDocument=async id=>id===documentId?decorate(await originalGet(id)):originalGet(id);
  api.command=async(id,input)=>{
   if(id!==documentId||!['SAVE_KINEMATICS','APPLY_MOTION_FRAME'].includes(String(input.type)))return originalCommand(id,input);
   if(input.type==='APPLY_MOTION_FRAME'&&state.applyBlocked)await new Promise<void>(resolve=>state.applyRelease=resolve);
   const current=await originalGet(id);if(input.versionId!==current.document.versionId)throw new Error('fixture CAS conflict');
   if(input.type==='SAVE_KINEMATICS'){defs=structuredClone(input.kinematics);state.saved.push(structuredClone(input));}
   else {state.applied.push(structuredClone(input));adopted=state.run.frames[input.motionApply!.frameIndex].unitPoses;}
   return decorate(await originalCommand(id,input));
  };
  api.listJobs=async()=>structuredClone(state.jobs);
  api.startMotionRun=async(id,request)=>{
   state.requests.push(structuredClone(request));const frozen=await api.getDocument(id),study=defs.studies[0];
   const frames=[0,90,180,270,360].map((angle,index)=>({timeSeconds:index,driverValue:angle*Math.PI/180,coordinates:{j:angle*Math.PI/180},kinematicValid:true,dmuConclusion:'NOT_CHECKED',unitPoses:Object.fromEntries(frozen.product!.instances.map((i,n)=>[i.id,{translation:n?[0,0,7]:[0,0,0],rotation:n?[0,0,Math.sin(angle*Math.PI/360),Math.cos(angle*Math.PI/360)]:[0,0,0,1]}]))}));
   state.run={schema:1,status:'COMPLETED',completed:true,elapsedMs:12,solveCalls:5,solverBuild:'browser-contract-fixture',frames,snapshot:{documentId:id,revisionId:frozen.document.versionId,digest:'fixture',view:frozen,mechanism:defs.mechanisms[0],study,currentOnly:false,geometryUnits:[]}};
   if(request.currentOnly){
    const gs=frozen.resolvedInstances!.slice(0,2).map((g,n)=>({id:'geometry-'+n,motionUnitId:g.instancePath.segments[0].instanceId,address:{instancePath:g.instancePath,bodyId:g.bodyId},geometryKey:g.geometryKey,geometryId:g.geometryId,relativePose:{translation:[0,0,0],rotation:[0,0,0,1]}}));
    state.run.snapshot={...state.run.snapshot,currentOnly:true,mechanism:{id:'',name:'',unitIds:null,joints:null},study:{id:'current',name:'当前姿态 DMU',start:{value:0,unit:''},end:{value:0,unit:''},frames:1,checkDmu:true},geometryUnits:gs};
    state.run.frames=[{timeSeconds:0,driverValue:0,coordinates:{},kinematicValid:false,dmuConclusion:'PASS',unitPoses:Object.fromEntries(frozen.product!.instances.map(i=>[i.id,{translation:i.translation,rotation:i.rotation??[0,0,0,1]}])),dmu:{complete:true,kernelBuild:'browser-contract-fixture',pairs:[{firstId:'geometry-0',secondId:'geometry-1',classification:'SEPARATED',distanceMm:10,commonVolumeMm3:0,complete:true,clearanceSatisfied:true}]}}];
   }
   const job={id:'fixture-run',type:'MOTION_STUDY',documentId:id,versionId:frozen.document.versionId,state:'SUCCEEDED',progress:100,createdAt:'fixture-time',payload:{studyName:study.name},resultObjectId:'fixture-artifact'};state.jobs=[job];return structuredClone(job) as any;
  };
  api.deleteMotionResult=async id=>{const job=state.jobs.find((j:any)=>j.id===id);state.jobs=state.jobs.filter((j:any)=>j.id!==id);state.deleted=id;return {...job,userVisible:false}};
  api.getMotionRun=async()=>{if(state.blocked)await new Promise<void>(resolve=>state.release=resolve);return structuredClone(state.run);};
  api.previewMechanism=async(id,req)=>{state.previews.push(structuredClone(req));if(state.previewBlocked)await new Promise<void>(resolve=>state.previewRelease=resolve);return {baseRevisionId:req.baseRevisionId,mechanism:req.mechanism,instancePoses:instances.map((i,n)=>({instanceId:i.id,translation:n?[0,0,7]:[0,0,0],rotation:[0,0,0,1]}))} as any};
  api.inspectAssemblySupports=async(id,refs)=>{state.inspected.push(structuredClone(refs));if(state.failInspection){state.failInspection=false;throw new Error('fixture temporary inspection failure')}return {supports:refs.map(reference=>({reference,descriptor:{Kind:reference.kind==='AXIS'?'AXIS':reference.kind==='PLANE'?'PLANE':reference.topologyId===99?'SPHERE':reference.derivedRole?'AXIS':'CYLINDER'}}))} as any;};

  api.motionJointProposals=async()=>[];
  api.planMotionApply=async(_id,version,request)=>{state.planRequest=structuredClone(request);return {baseRevisionId:version,digest:'fixture-plan',ready:true,items:[{role:'ground',constraintId:'fixed',action:'ADD'},{role:'axis',constraintId:'coaxial',action:'ADD'},{role:'axial-location',constraintId:'offset',action:'ADD'}],poseChanges:instances.map(i=>i.id),solverStatus:'CONVERGED',degreesOfFreedom:1};};
  // A changed definition requires a new Revision, just like the live contract.
  await api.updateDocument(documentId,initial.document.name,initial.document.description??'');
  await queryClient.invalidateQueries();
 },withDefinitions);
}
async function command(page:Page,name:string){
 await page.getByRole('button',{name:'搜索工具',exact:true}).click();
 await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);
 const result=page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp('^'+name+'$')})});
 await expect(result).toHaveCount(1);await expect(result).toBeEnabled();await result.click();
}


async function enter(page:Page,existing=false){
 if(existing){await page.getByRole('textbox',{name:'筛选模型结构'}).fill('测试机构');await page.getByText('测试机构',{exact:true}).dblclick();await page.getByRole('textbox',{name:'筛选模型结构'}).fill('');}
 else {await command(page,'机构');await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.at(-1)?.kinematics.mechanisms.length)).toBe(1);}
 await expect(page.getByRole('tab',{name:'机构与 DMU',exact:true})).toBeVisible();
 await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toHaveCount(0);
}
async function start(page:Page){await command(page,'运行仿真');const dialog=page.getByRole('dialog',{name:'运行仿真',exact:true});await expect(dialog).toBeVisible();await dialog.getByRole('button',{name:/^运\s*行$/}).click();}

test('entered mechanism session creates its tree node and discards only unedited entries',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page);
 const height=(await page.getByRole('region',{name:'三维视口'}).boundingBox())!.height;
 await enter(page);await expect(page.locator('.motion-study-activity')).toHaveCount(0);await expect(page.getByRole('dialog')).toHaveCount(0);
 expect((await page.getByRole('region',{name:'三维视口'}).boundingBox())!.height).toBe(height);
 await command(page,'退出机构');await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.at(-1).kinematics.mechanisms.length)).toBe(0);
 await enter(page);await command(page,'固定件');const fixed=page.getByRole('dialog',{name:'固定件',exact:true});await expect(fixed).toBeVisible();await page.keyboard.press('Escape');
 await command(page,'退出机构');await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.at(-1).kinematics.mechanisms.length)).toBe(0);
 await enter(page);await command(page,'固定件');await expect(fixed.getByRole('button',{name:/^保\s*存$/})).toBeEnabled();await fixed.getByRole('button',{name:/^保\s*存$/}).click();
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.at(-1).kinematics.mechanisms[0]?.joints.length)).toBe(1);
 await command(page,'退出机构');await page.getByRole('textbox',{name:'筛选模型结构'}).fill('机构 1');await page.getByText('机构 1',{exact:true}).dblclick();
 await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toHaveCount(0);await expect(page.getByRole('dialog')).toHaveCount(0);
 expect((await page.getByRole('region',{name:'三维视口'}).boundingBox())!.height).toBe(height);expect(errors).toEqual([]);
});

test('filtered tree picks advance roles and preview axes before locating planes',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);await command(page,'旋转接合');
 const dialog=page.getByRole('dialog',{name:'旋转接合',exact:true});await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:/^保\s*存$/})).toBeDisabled();
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});
 // A plane is never accepted by the axial role.
 await filter.fill('试验平面 0');await page.getByText('试验平面 0',{exact:true}).click();await expect(dialog.getByText('已绑定（点击替换）',{exact:false})).toHaveCount(0);
 for(const label of ['试验轴线 0','试验轴线 1','试验平面 0','试验平面 1']){
  await filter.fill(label);const item=page.getByText(label,{exact:true});await item.hover();
  const role=label.includes('轴线')?'轴线':'定位平面';const before=await dialog.getByText('已绑定（点击替换）',{exact:false}).count();
  if(label==='试验轴线 0'){
   await page.evaluate(()=>(window as any).__motionFixture.failInspection=true);await item.click();
   await expect(page.locator('.cad-operation-notification')).toContainText('几何支持未完成');
   await expect(dialog.getByText('已绑定（点击替换）',{exact:false})).toHaveCount(before);
  }
  await item.click();await expect(dialog.getByText('已绑定（点击替换）',{exact:false})).toHaveCount(before+1);
  await expect(page.getByTestId('viewport-feature-selection')).toHaveAttribute('data-count',String(before+1));
  if(label==='试验轴线 1')await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.previews.some((p:any)=>p.draftJointId&&!p.mechanism.joints.at(-1).first.plane))).toBe(true);
 }
 await expect(dialog.getByRole('button',{name:/^保\s*存$/})).toBeEnabled();await dialog.getByRole('button',{name:/^保\s*存$/}).click();
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(1);
 const joint=await page.evaluate(()=>(window as any).__motionFixture.saved[0].kinematics.mechanisms[0].joints.at(-1));expect(joint.first.instanceId).not.toBe(joint.second.instanceId);expect(joint.first.plane.geometryId).toBe('datum-xy');expect(errors).toEqual([]);
});

test('dialog replay uses whole frames and publishes the selected frame',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);await start(page);
 const result=page.getByRole('dialog',{name:'仿真与回放',exact:true});await expect(result.getByText(/帧 1 · t=0.000/)).toBeVisible();
 await expect(page.locator('.workbench-status').getByText(/机构回放 · 帧 1/)).toBeVisible();await expect(page.locator('.motion-study-activity')).toHaveCount(0);
 const previewsBeforeRestore=await page.evaluate(()=>(window as any).__motionFixture.previews.length);
 await result.getByRole('button',{name:'恢复正式姿态',exact:true}).click();await expect(page.locator('.workbench-status')).not.toContainText('机构回放');
 // Closing and reopening results must not resume an alignment request after an
 // explicit restore. The selected frozen frame remains available for playback.
 await page.keyboard.press('Escape');await command(page,'仿真与回放');await expect(result).toBeVisible();
 expect(await page.evaluate(()=>(window as any).__motionFixture.previews.length)).toBe(previewsBeforeRestore);
 await result.getByRole('button',{name:'单步',exact:true}).click();await expect(result.getByText(/帧 2 · t=1.000/)).toBeVisible();
 await result.getByRole('button',{name:'应用到装配',exact:true}).click();const apply=page.getByRole('dialog',{name:'应用到装配：连接关系与所选帧姿态',exact:true});await expect(apply).toBeVisible();
 await apply.getByRole('button',{name:'生成／刷新转换计划'}).click();await expect(apply.getByText(/DOF 1/)).toBeVisible();await page.keyboard.press('Escape');
 await expect(page.locator('.workbench-status').getByText(/机构回放 · 帧 2/)).toBeVisible();
 await command(page,'应用到装配');await apply.getByRole('button',{name:'生成／刷新转换计划'}).click();await apply.getByRole('button',{name:'一次提交并返回装配'}).click();
 await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();expect(await page.evaluate(()=>(window as any).__motionFixture.applied[0].motionApply)).toMatchObject({frameIndex:1,lockAngle:false,planDigest:'fixture-plan'});expect(errors).toEqual([]);
});

test('late results and preview responses cannot reopen an exited session',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);
 await page.evaluate(()=>(window as any).__motionFixture.blocked=true);await start(page);
 await expect.poll(()=>page.evaluate(()=>typeof (window as any).__motionFixture.release)).toBe('function');await command(page,'退出机构');await page.evaluate(()=>(window as any).__motionFixture.release());
 await enter(page,true);await expect(page.locator('.workbench-status').getByText(/机构回放/)).toHaveCount(0);
 await page.evaluate(()=>(window as any).__motionFixture.previewBlocked=true);await command(page,'固定件');await expect.poll(()=>page.evaluate(()=>typeof (window as any).__motionFixture.previewRelease)).toBe('function');
 await command(page,'退出机构');await page.evaluate(()=>(window as any).__motionFixture.previewRelease());await expect(page.getByRole('dialog')).toHaveCount(0);await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);expect(errors).toEqual([]);
});


test('entered definitions use standard rename and deletion commands',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page);await enter(page);
 await command(page,'固定件');await page.getByRole('dialog',{name:'固定件',exact:true}).getByRole('button',{name:/^保\s*存$/}).click();
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(2);
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill('固定件');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText('固定件',{exact:true}).click({button:'right'});
 await page.getByRole('menuitem',{name:'重命名',exact:true}).click();const rename=page.getByRole('dialog',{name:'重命名建模对象',exact:true});
 await expect(rename.getByRole('textbox',{name:/名称$/})).toHaveValue('固定件');await rename.getByRole('textbox',{name:/名称$/}).fill('圆柱固定');
 await rename.getByRole('button',{name:/^确\s*定$/}).click();await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(3);
 await filter.fill('圆柱固定');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText('圆柱固定',{exact:true}).dblclick();
 await expect(page.getByRole('dialog',{name:'圆柱固定',exact:true})).toBeVisible();await page.keyboard.press('Escape');
 await filter.fill('机构 1');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText('机构 1',{exact:true}).click({button:'right'});await page.getByRole('menuitem',{name:'删除',exact:true}).click();
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.at(-1).kinematics.mechanisms.length)).toBe(0);
 await expect(page.locator('.workbench-status')).toContainText('机构编辑');await expect(page.locator('.workbench-status')).not.toContainText('机构 1');expect(errors).toEqual([]);
});


test('static interference results reopen without a mechanism and delete without tree ghosts',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);
 await command(page,'干涉检查');const definition=page.getByRole('dialog',{name:'干涉检查',exact:true});await definition.getByRole('button',{name:/^保\s*存$/}).click();
 await command(page,'运行干涉检查');const check=page.getByRole('dialog',{name:'运行干涉检查',exact:true});await check.getByRole('button',{name:/^检\s*查$/}).click();
 const result=page.getByRole('dialog',{name:'仿真与回放',exact:true});await expect(result).toContainText('运动学 未检查');await expect(result).toContainText('SEPARATED');await expect(result).not.toContainText('驱动 0.000000');
 await expect(page.locator('canvas')).toBeVisible();await page.keyboard.press('Escape');await command(page,'仿真与回放');await expect(result).toContainText('SEPARATED');
 await result.getByRole('button',{name:'删除运行结果',exact:true}).click();await expect(result).not.toContainText('SEPARATED');await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.jobs.length)).toBe(0);
 await page.keyboard.press('Escape');await command(page,'退出机构');await enter(page,true);await command(page,'仿真与回放');await expect(result).not.toContainText('SEPARATED');await expect(page.locator('canvas')).toBeVisible();expect(errors).toEqual([]);
});


test('joint relation children edit their owner through the shared tree command',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill('同心');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText('同心',{exact:true}).dblclick();
 const dialog=page.getByRole('dialog',{name:'轴接合',exact:true});await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:'第一轴线',exact:true})).toHaveClass(/ant-btn-primary/);await page.keyboard.press('Escape');
 await filter.fill('偏移');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText('偏移',{exact:true}).dblclick();await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:'第一定位平面',exact:true})).toHaveClass(/ant-btn-primary/);await page.keyboard.press('Escape');
 expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);expect(errors).toEqual([]);
});
