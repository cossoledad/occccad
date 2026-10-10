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
  const state:any={saved:[],requests:[],applied:[],inspected:[],jobs:[],blocked:false,release:undefined};
  (window as any).__motionFixture=state;
  let adopted:any;
  const decorate=(source:any)=>{
   const view=structuredClone(source);view.product.kinematics=structuredClone(defs);
   if(adopted){for(const i of view.product.instances)Object.assign(i,adopted[i.id]);for(const i of view.resolvedInstances??[])Object.assign(i,adopted[i.instancePath.segments[0].instanceId]);}
   const node=(kind:string,obj:any)=>({id:`application:${obj.id}`,kind,name:obj.name,entityId:obj.id,documentId,ownerDocumentId:documentId,versionId:view.document.versionId,subject:{documentId,entityKind:kind,entityId:obj.id},snapshot:{revisionId:view.document.versionId},capabilities:['EDIT','DELETE','RENAME']});
   view.structureTree.children=view.structureTree.children.filter((n:any)=>n.kind!=='APPLICATIONS');
   view.structureTree.children.push({id:'applications',kind:'APPLICATIONS',name:'Applications',documentId,versionId:view.document.versionId,children:[...defs.mechanisms.map((m:any)=>({...node('MECHANISM',m),children:[...m.joints.map((j:any)=>node('MECHANISM_JOINT',j)),...defs.drivers.filter((d:any)=>d.mechanismId===m.id).map((d:any)=>node('MOTION_DRIVER',d)),...defs.studies.filter((s:any)=>s.mechanismId===m.id).map((s:any)=>node('MOTION_STUDY',s))]})),...defs.analyses.map((a:any)=>node('INTERFERENCE_ANALYSIS',a))]});
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
   const job={id:'fixture-run',type:'MOTION_STUDY',documentId:id,versionId:frozen.document.versionId,state:'SUCCEEDED',progress:100,createdAt:'fixture-time',payload:{studyName:study.name},resultObjectId:'fixture-artifact'};state.jobs=[job];return structuredClone(job) as any;
  };
  api.getMotionRun=async()=>{if(state.blocked)await new Promise<void>(resolve=>state.release=resolve);return structuredClone(state.run);};
  api.inspectAssemblySupports=async(id,refs)=>{state.inspected.push(structuredClone(refs));return {supports:refs.map(reference=>({reference,descriptor:{Kind:reference.kind==='AXIS'?'AXIS':'PLANE'}}))} as any;};
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

test('one configured tab deck, registry search and cancelable definition commands',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page);
 await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toHaveAttribute('aria-selected','true');
 await expect(page.locator('.workbench-section-tabs')).toHaveCount(1);
 await expect(page.locator('.ant-drawer')).toHaveCount(0);
 await page.getByRole('button',{name:'搜索工具',exact:true}).click();
 await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill('新建机构');
 await expect(page.locator('.workbench-command-result')).toHaveCount(0);await page.keyboard.press('Escape');
 await page.getByRole('tab',{name:'机构与 DMU',exact:true}).click();
 const activity=page.getByRole('region',{name:'机构运行与回放'});await expect(activity).toBeVisible();
 await command(page,'新建机构');
 const dialog=page.getByRole('dialog',{name:'新建机构',exact:true});await expect(dialog).toHaveClass(/cad-command-dialog/);
 await dialog.getByRole('textbox',{name:'机构名称'}).fill('取消的机构');await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);
 expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);
 await command(page,'新建机构');await dialog.getByRole('textbox',{name:'机构名称'}).fill('螺旋桨机构');await dialog.getByRole('button',{name:/^保\s*存$/}).click();await expect(dialog).toHaveCount(0);
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill('螺旋桨机构');
 const mechanism=page.getByRole('treeitem').filter({hasText:'螺旋桨机构'});await expect(mechanism).toBeVisible();await mechanism.dblclick();
 const edit=page.getByRole('dialog',{name:'编辑机构',exact:true});await expect(edit).toBeVisible();await edit.getByRole('textbox',{name:'机构名称'}).fill('编辑后的机构');await edit.getByRole('button',{name:/^保\s*存$/}).click();await expect(edit).toHaveCount(0);await filter.fill('编辑后的机构');await expect(page.getByRole('treeitem').filter({hasText:'编辑后的机构'})).toBeVisible();
 await command(page,'装配关联');const association=page.getByRole('dialog',{name:'装配关联',exact:true});
 await association.getByRole('checkbox').check();await page.keyboard.press('Escape');
 expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(2);
 await command(page,'新建机构');await page.getByRole('tab',{name:'装配设计',exact:true}).click();
 await expect(dialog).toHaveCount(0);await expect(activity).toHaveCount(0);
 expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(2);expect(errors).toEqual([]);
});

test('geometry picks enter joint drafts through the shared selection and command dialog',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);
 await page.getByRole('tab',{name:'机构与 DMU',exact:true}).click();await command(page,'旋转接合');
 const dialog=page.getByRole('dialog',{name:'旋转接合',exact:true});await expect(dialog).toBeVisible();
 await expect(dialog.getByRole('button',{name:/^保\s*存$/})).toBeDisabled();
 for(const side of [0,1])for(const axis of [true,false]){
  // This feeds the same formal SelectionItem used by viewport/tree picking;
  // exact support replies are isolated fixtures, not mock geometry authority.
  await page.evaluate(async({side,axis})=>{
   const {api}=await import('/src/api/client.ts');const {useWorkbenchStore}=await import('/src/state/workbench-store.ts');
   const view=await api.getDocument('mock-product-frame'),i=view.product!.instances[side],resolved=view.resolvedInstances!.find(r=>r.instancePath.segments[0].instanceId===i.id)!;
   useWorkbenchStore.getState().setSelections([{kind:axis?'axis':'plane',id:axis?'axis-system-default':'datum-xy',entityId:axis?'axis-system-default':'datum-xy',axis:axis?'Z':undefined,instanceId:i.id,instancePath:resolved.instancePath,documentId:i.documentId,versionId:i.versionId}]);
  },{side,axis});
  await dialog.getByRole('button',{name:axis?'拾取轴':'拾取定位平面',exact:true}).nth(side).click();
  await expect(dialog.getByText('已绑定',{exact:false})).toHaveCount(side*2+(axis?1:2));
 }
 await dialog.getByRole('button',{name:/^保\s*存$/}).click();await expect(dialog).toHaveCount(0);
 const joint=await page.evaluate(()=>(window as any).__motionFixture.saved.at(-1).kinematics.mechanisms[0].joints.at(-1));
 expect(joint.first.instanceId).not.toBe(joint.second.instanceId);expect(joint.first.axis.instancePath.canonical).not.toBe(joint.second.axis.instancePath.canonical);expect(joint.first.plane.geometryId).toBe('datum-xy');expect(errors).toEqual([]);
});

test('frozen whole-frame playback, apply command and application exit',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);
 await page.getByRole('tab',{name:'机构与 DMU',exact:true}).click();await command(page,'运行仿真');
 const activity=page.getByRole('region',{name:'机构运行与回放'});await expect(activity.getByText(/帧 1 · t=0.000/)).toBeVisible();
 await expect(page.getByText('机构回放：冻结版本临时姿态',{exact:true})).toBeVisible();
 await activity.getByRole('button',{name:'单步',exact:true}).click();await expect(activity.getByText(/帧 2 · t=1.000/)).toBeVisible();
 await activity.getByRole('button',{name:'应用到装配',exact:true}).click();
 const apply=page.getByRole('dialog',{name:'应用到装配：连接关系与所选帧姿态',exact:true});await expect(apply).toHaveClass(/cad-command-dialog/);
 await expect(apply.getByRole('checkbox',{name:'锁定当前角度'})).not.toBeChecked();
 await apply.getByRole('button',{name:'生成／刷新转换计划',exact:true}).click();await expect(apply.getByText(/DOF 1/)).toBeVisible();
 await page.keyboard.press('Escape');await expect(apply).toHaveCount(0);await expect(page.getByText('机构回放：冻结版本临时姿态',{exact:true})).toBeVisible();
 await activity.getByRole('button',{name:'应用到装配',exact:true}).click();await apply.getByRole('button',{name:'生成／刷新转换计划',exact:true}).click();await apply.getByRole('button',{name:'一次提交并返回装配',exact:true}).click();
 await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toHaveAttribute('aria-selected','true');await expect(activity).toHaveCount(0);await expect(page.getByText('机构回放：冻结版本临时姿态',{exact:true})).toHaveCount(0);
 const applied=await page.evaluate(()=>(window as any).__motionFixture.applied);expect(applied).toHaveLength(1);expect(applied[0].motionApply).toMatchObject({frameIndex:1,lockAngle:false,planDigest:'fixture-plan'});expect(errors).toEqual([]);
});

test('a late run response cannot reacquire the viewport after leaving the application',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);
 await page.evaluate(()=>(window as any).__motionFixture.blocked=true);
 await page.getByRole('tab',{name:'机构与 DMU',exact:true}).click();await command(page,'运行仿真');
 await expect.poll(()=>page.evaluate(()=>typeof (window as any).__motionFixture.release)).toBe('function');
 await page.getByRole('tab',{name:'装配设计',exact:true}).click();await page.evaluate(()=>(window as any).__motionFixture.release());
 await expect(page.getByText('机构回放：冻结版本临时姿态',{exact:true})).toHaveCount(0);
 await page.getByRole('tab',{name:'机构与 DMU',exact:true}).click();await expect(page.getByText('机构回放：冻结版本临时姿态',{exact:true})).toHaveCount(0);await expect(page.getByRole('region',{name:'机构运行与回放'}).getByText(/帧 1 ·/)).toHaveCount(0);expect(errors).toEqual([]);
});


test('canceling an apply display prevents its late completion from switching application tabs',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);
 await page.getByRole('tab',{name:'机构与 DMU',exact:true}).click();await command(page,'运行仿真');
 const activity=page.getByRole('region',{name:'机构运行与回放'});await expect(activity.getByText(/帧 1 · t=0.000/)).toBeVisible();
 await activity.getByRole('button',{name:'应用到装配',exact:true}).click();const apply=page.getByRole('dialog',{name:'应用到装配：连接关系与所选帧姿态',exact:true});
 await apply.getByRole('button',{name:'生成／刷新转换计划',exact:true}).click();await expect(apply.getByText(/DOF 1/)).toBeVisible();
 await page.evaluate(()=>(window as any).__motionFixture.applyBlocked=true);
 await apply.getByRole('button',{name:'一次提交并返回装配',exact:true}).click();await expect.poll(()=>page.evaluate(()=>typeof (window as any).__motionFixture.applyRelease)).toBe('function');
 await page.getByRole('tab',{name:'装配设计',exact:true}).click();await expect(apply).toHaveCount(0);
 await page.getByRole('tab',{name:'机构与 DMU',exact:true}).click();await page.evaluate(()=>(window as any).__motionFixture.applyRelease());
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.applied.length)).toBe(1);
 await expect(page.getByText('机构回放：冻结版本临时姿态',{exact:true})).toHaveCount(0);
 await expect(page.getByRole('tab',{name:'机构与 DMU',exact:true})).toHaveAttribute('aria-selected','true');
 // The issued domain transaction can finish; its disposed UI invocation cannot
 // claim ownership of the user's new application session.
 await command(page,'新建机构');await expect(page.getByRole('dialog',{name:'新建机构',exact:true})).toBeVisible();expect(errors).toEqual([]);
});
