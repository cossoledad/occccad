import {expect,test,type Page} from '@playwright/test';

test.setTimeout(180_000);

// Browser contract fixtures replace API responses only in this test's page.
// No fixture is a numerical solver or an OCCT interference result.
async function fixture(page:Page,withDefinitions=false){
 await page.goto('/documents/mock-product-frame');
 await expect(page.locator('canvas')).toBeVisible({timeout:60_000});
 await page.evaluate(async seeded=>{
  const {api}=await import('/src/api/client.ts');
  const {queryClient}=await import('/src/app/providers.tsx');
  const documentId='mock-product-frame',originalGet=api.getDocument.bind(api),originalCommand=api.command.bind(api);
  const initial=await originalGet(documentId),instances=initial.product!.instances;
  const end=(instanceId:string)=>({instanceId,frame:{translation:[0,0,0],rotation:[0,0,0,1]},axis:{instanceId,kind:'AXIS',geometryId:'axis-system-default',axis:'Z'},plane:{instanceId,kind:'PLANE',geometryId:'datum-xy'},capturedX:[1,0,0]});
  let defs:any={mechanisms:[],drivers:[],studies:[],associations:[]};
  if(seeded){defs={...defs,mechanisms:[{id:'m',name:'测试机构',unitIds:instances.map(i=>i.id),joints:[{id:'g',name:'圆柱固定',kind:'GROUND',first:end(instances[0].id),zero:{value:0,unit:'deg'},direction:1},{id:'j',name:'轴接合',kind:'REVOLUTE',first:end(instances[0].id),second:end(instances[1].id),zero:{value:0,unit:'deg'},direction:1}]}],drivers:[{id:'d',name:'角度驱动',mechanismId:'m',jointId:'j'}],laws:[{id:'law',name:'线性规律',mechanismId:'m',driverId:'d',start:{value:0,unit:'deg'},end:{value:360,unit:'deg'},durationSeconds:4}],studies:[{id:'s',name:'旋转研究',mechanismId:'m',lawId:'law',driverId:'d',start:{value:0,unit:'deg'},end:{value:360,unit:'deg'},durationSeconds:4,frames:5,budgetMs:60000}]}}
  for(const m of defs.mechanisms){m.poses=Object.fromEntries(instances.map((i,n)=>[i.id,{translation:n?[0,0,7]:[0,0,0],rotation:[0,0,0,1]}]));}
  const state:any={saved:[],previews:[],requests:[],applied:[],inspected:[],jobs:[],blocked:false,release:undefined};
  (window as any).__motionFixture=state;
  let adopted:any;
  const decorate=(source:any)=>{
   for(const m of defs.mechanisms)for(const j of m.joints)if(!j.constraints){
    const c=(id:string,name:string,kind:string,first:any,second?:any)=>({id,name,kind,first,second,evaluationStatus:'VERIFIED'});
    j.constraints=j.kind==='GROUND'?[c('relation-'+j.id+'-fixed','固定.1','FIX',{kind:'BODY',instanceId:j.first.instanceId})]:[c('relation-'+j.id+'-coincidence','重合.1','COINCIDENT',j.first.axis,j.second.axis),c('relation-'+j.id+'-offset','偏移.1','DISTANCE',j.first.plane,j.second.plane)];
   }
   const view=structuredClone(source);view.product.kinematics=structuredClone(defs);
   if(adopted){for(const i of view.product.instances)Object.assign(i,adopted[i.id]);for(const i of view.resolvedInstances??[])Object.assign(i,adopted[i.instancePath.segments[0].instanceId]);}
   const node=(kind:string,obj:any)=>({id:`application:${obj.id}`,kind,name:obj.name,entityId:obj.id,documentId,ownerDocumentId:documentId,versionId:view.document.versionId,subject:{documentId,entityKind:kind,entityId:obj.id},snapshot:{revisionId:view.document.versionId},capabilities:['EDIT','DELETE','RENAME']});
   view.structureTree.children=view.structureTree.children.filter((n:any)=>n.kind!=='APPLICATIONS');
   const project=(m:any,parent:string,reference=false)=>{
    const mn={...node(reference?'MECHANISM_REFERENCE':'MECHANISM',m),id:parent+'/mechanism:'+m.id,mechanismId:m.id,name:m.name+'，自由度=0',children:[] as any[]};
    const group=(key:string,name:string)=>({id:mn.id+'/'+key,kind:'KINEMATICS_GROUP',name,children:[] as any[]});
    const joints=group('joints','接合'),commands=group('commands','命令'),ground=group('ground','固定零件'),laws=group('laws','运动规律');
    const nameFor=(id:string)=>instances.find(i=>i.id===id)?.name??id;
    for(const j of m.joints){
     const suffix=' ('+nameFor(j.first.instanceId)+(j.second?','+nameFor(j.second.instanceId):'')+')';
     const jn={...node('MECHANISM_JOINT',j),id:mn.id+'/joint:'+j.id,mechanismId:m.id,name:j.name+suffix,children:[] as any[]};
     if(!j.constraints){const c=(id:string,name:string,kind:string,first:any,second?:any)=>({id,name,kind,first,second,evaluationStatus:'VERIFIED'});j.constraints=j.kind==='GROUND'?[c('relation-'+j.id+'-fixed','固定.1','FIX',{kind:'BODY',instanceId:j.first.instanceId})]:[c('relation-'+j.id+'-coincidence','重合.1','COINCIDENT',j.first.axis,j.second.axis),c('relation-'+j.id+'-offset','偏移.1','DISTANCE',j.first.plane,j.second.plane)];}
     jn.children=j.constraints.map((c:any)=>({...node('JOINT_EXPANSION',c),id:jn.id+'/relation:'+c.id,name:c.name+suffix,mechanismId:m.id,ownerEntityId:j.id,relationRole:c.kind==='DISTANCE'?'axial-location':c.kind==='FIX'?'ground':'axis',entityType:c.kind,evaluationStatus:c.evaluationStatus,presentationRole:'DEFINITION',capabilities:['EDIT']}));
     (j.kind==='GROUND'?ground:joints).children.push(jn);
    }
    commands.children=defs.drivers.filter((d:any)=>d.mechanismId===m.id).map((d:any)=>({...node('MOTION_DRIVER',d),id:mn.id+'/command:'+d.id,mechanismId:m.id}));
    laws.children=(defs.laws??[]).filter((l:any)=>l.mechanismId===m.id).map((l:any)=>({...node('MOTION_LAW',l),id:mn.id+'/law:'+l.id,mechanismId:m.id}));
    mn.children=[joints,commands,ground,laws];return mn;
   };
   view.structureTree.children.push({id:'applications',kind:'APPLICATIONS',name:'Applications',documentId,versionId:view.document.versionId,children:[{id:'mechanisms',kind:'KINEMATICS_GROUP',name:'机械装置',children:defs.mechanisms.map((m:any)=>project(m,'mechanisms'))},{id:'simulations',kind:'KINEMATICS_GROUP',name:'模拟',children:defs.studies.map((s:any)=>({...node('MOTION_STUDY',s),children:[project(defs.mechanisms.find((m:any)=>m.id===s.mechanismId),'simulation:'+s.id,true)]}))}]});

   for(let n=0;n<2;n++){
    const resolved=view.resolvedInstances.find((r:any)=>r.instancePath.segments[0].instanceId===instances[n].id);
    const path=structuredClone(resolved.instancePath);path.segments[0].ownerVersionId=view.document.versionId;
    const surface=(localId:number,name:string)=>({id:'surface-'+n+'-'+localId,kind:'PUBLICATION',name,entityId:'publication-'+n+'-'+localId,documentId:instances[n].documentId,sourceDocumentId:instances[n].documentId,versionId:instances[n].versionId,instancePath:path,publication:{id:'publication-'+n+'-'+localId,type:'SURFACE',compatibilityVersion:1,target:{bodyId:resolved.bodyId},resolution:{status:'CONNECTED',topologyKind:'FACE',localId,geometryKey:resolved.geometryKey,resolvedVersionId:instances[n].versionId}}});
    view.structureTree.children.push({id:'supports-'+n,kind:'INSTANCE',name:'选择支持 '+n,documentId:instances[n].documentId,entityId:instances[n].id,instancePath:path,children:[
     {id:'axis-'+n,kind:'AXIS',name:'试验轴线 '+n,entityId:'axis-system-default',axis:'Z',documentId:instances[n].documentId,versionId:instances[n].versionId,instancePath:path},
     surface(2,'试验圆柱面 '+n),surface(3,'试验定位面 '+n),
     {id:'plane-'+n,kind:'PLANE',name:'试验平面 '+n,entityId:'datum-xy',plane:{origin:[0,0,0],normal:[0,0,1],xDirection:[1,0,0]},documentId:instances[n].documentId,versionId:instances[n].versionId,instancePath:path}]});
   }
   return view;
  };
  api.getDocument=async id=>id===documentId?decorate(await originalGet(id)):originalGet(id);
  api.command=async(id,input)=>{
   if(id!==documentId||!['SAVE_KINEMATICS','APPLY_MOTION_FRAME','ACTIVATE_MECHANISM','EXIT_MECHANISM'].includes(String(input.type)))return originalCommand(id,input);
   if(input.type==='APPLY_MOTION_FRAME'&&state.applyBlocked)await new Promise<void>(resolve=>state.applyRelease=resolve);
   if(input.type==='SAVE_KINEMATICS'&&state.rejectNextSave){state.rejectNextSave=false;throw new Error('fixture rejected definition');}
   const current=await originalGet(id);if(input.versionId!==current.document.versionId)throw new Error('fixture CAS conflict');
   if(input.type==='SAVE_KINEMATICS'){defs=structuredClone(input.kinematics);for(const m of defs.mechanisms){m.poses??=Object.fromEntries(instances.map((i,n)=>[i.id,{translation:n?[0,0,7]:[0,0,0],rotation:[0,0,0,1]}]));}state.saved.push(structuredClone(input));}
   else if(input.type==='ACTIVATE_MECHANISM'){state.activations??=[];state.activations.push(structuredClone(input));}
   else if(input.type==='EXIT_MECHANISM'){state.exits??=[];state.exits.push(structuredClone(input));adopted=defs.mechanisms.find((m:any)=>m.id===input.mechanismApplication?.sourceMechanismId)?.poses??adopted;}
   else {state.applied.push(structuredClone(input));adopted=input.motionApply?.mechanismId?defs.mechanisms.find((m:any)=>m.id===input.motionApply.mechanismId).poses:state.run.frames[input.motionApply!.frameIndex].unitPoses;}
   return decorate(await originalCommand(id,input));
  };
  api.prepareMechanismApplication=async(_id,input)=>{
   const m=defs.mechanisms.find((m:any)=>m.id===input.mechanismId),source=defs.mechanisms.find((m:any)=>m.id===input.sourceMechanismId);
   const poses=m?.poses??source?.poses??adopted??Object.fromEntries(instances.map(i=>[i.id,{translation:i.translation,rotation:i.rotation??[0,0,0,1]}]));
   return {baseRevisionId:input.baseRevisionId,mechanism:m,instancePoses:instances.map(i=>({instanceId:i.id,...(poses[i.id]??{translation:i.translation,rotation:i.rotation??[0,0,0,1]})}))};
  };
  api.listJobs=async()=>structuredClone(state.jobs);
  api.startMotionRun=async(id,request)=>{
   state.requests.push(structuredClone(request));const frozen=await api.getDocument(id),study=defs.studies[0];
   const frames=[0,90,180,270,360].map((angle,index)=>({timeSeconds:index,driverValue:angle*Math.PI/180,coordinates:{j:angle*Math.PI/180},kinematicValid:true,unitPoses:Object.fromEntries(frozen.product!.instances.map((i,n)=>[i.id,{translation:n?[0,0,7]:[0,0,0],rotation:n?[0,0,Math.sin(angle*Math.PI/360),Math.cos(angle*Math.PI/360)]:[0,0,0,1]}]))}));
   state.run={schema:1,status:'COMPLETED',completed:true,elapsedMs:12,solveCalls:5,solverBuild:'browser-contract-fixture',frames,snapshot:{documentId:id,revisionId:frozen.document.versionId,digest:'fixture',view:frozen,mechanism:defs.mechanisms[0],study}};
   const job={id:'fixture-run',type:'MOTION_STUDY',documentId:id,versionId:frozen.document.versionId,state:state.holdJob?'RUNNING':'SUCCEEDED',progress:state.holdJob?20:100,canCancel:Boolean(state.holdJob),createdAt:'fixture-time',payload:{studyName:study.name},resultObjectId:state.holdJob?undefined:'fixture-artifact'};state.jobs=[job];return structuredClone(job) as any;
  };
  api.cancelJob=async id=>{state.canceled??=[];state.canceled.push(id);const job=state.jobs.find((j:any)=>j.id===id);Object.assign(job,{state:'CANCELED',canCancel:false});return structuredClone(job);};
  api.deleteMotionResult=async id=>{const job=state.jobs.find((j:any)=>j.id===id);state.jobs=state.jobs.filter((j:any)=>j.id!==id);state.deleted=id;return {...job,userVisible:false}};
  api.getMotionRun=async()=>{if(state.blocked)await new Promise<void>(resolve=>state.release=resolve);return structuredClone(state.run);};
  api.previewMechanism=async(id,req)=>{state.previews.push(structuredClone(req));if(state.previewBlocked)await new Promise<void>(resolve=>state.previewRelease=resolve);return {baseRevisionId:req.baseRevisionId,mechanism:{...req.mechanism,poses:Object.fromEntries(instances.map((i,n)=>[i.id,{translation:n?[0,0,7]:[0,0,0],rotation:[0,0,0,1]}]))},instancePoses:instances.map((i,n)=>({instanceId:i.id,translation:n?[0,0,7]:[0,0,0],rotation:[0,0,0,1]}))} as any};
  api.inspectAssemblySupports=async(id,refs)=>{state.inspected.push(structuredClone(refs));if(state.failInspection)throw new Error('fixture temporary inspection failure');return {supports:refs.map(reference=>({reference,descriptor:{Kind:reference.derivedRole?'AXIS':reference.kind==='AXIS'?'AXIS':reference.kind==='PLANE'||reference.topologyId===3?'PLANE':reference.topologyId===99?'SPHERE':'CYLINDER'}}))} as any;};

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
 if(existing){const name=await page.evaluate(()=>(window as any).__motionFixture.saved.at(-1)?.kinematics.mechanisms[0]?.name??'测试机构');const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill(name);await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText(name+'，自由度=0',{exact:true}).first().dblclick();await filter.fill('');}
 else {await command(page,'机构');await command(page,'新建机械装置');await page.getByRole('dialog',{name:'编辑机械装置',exact:true}).getByRole('button',{name:/^保\s*存$/}).click();await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.at(-1)?.kinematics.mechanisms.length)).toBe(1);}
 await expect(page.getByRole('tab',{name:'机构与 DMU',exact:true})).toBeVisible();
 await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toHaveCount(0);
}
async function start(page:Page){await command(page,'运行仿真');const dialog=page.getByRole('dialog',{name:'仿真与回放',exact:true});await expect(dialog).toBeVisible();await dialog.getByRole('button',{name:'计算仿真',exact:true}).click();}

test('entering DMU does not create and explicitly confirmed empty mechanisms remain',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page);
 await command(page,'机构');await expect(page.getByRole('tab',{name:'机构与 DMU',exact:true})).toBeVisible();
 expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);
 await command(page,'新建机械装置');const dialog=page.getByRole('dialog',{name:'编辑机械装置',exact:true});
 await page.keyboard.press('Escape');expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);
 await command(page,'新建机械装置');await dialog.getByRole('button',{name:/^保\s*存$/}).click();
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(1);
 await command(page,'退出机构');expect(await page.evaluate(()=>(window as any).__motionFixture.saved.at(-1).kinematics.mechanisms.length)).toBe(1);
 await enter(page,true);await expect(page.getByRole('dialog')).toHaveCount(0);expect(errors).toEqual([]);
});

test('filtered tree picks advance roles and preview axes before locating planes',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);await command(page,'旋转接合');
 const dialog=page.getByRole('dialog',{name:/^旋转\./});await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:/^保\s*存$/})).toBeDisabled();
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});
 // A plane is never accepted by the axial role.
 await filter.fill('试验平面 0');await page.getByText('试验平面 0',{exact:true}).click();await expect(dialog.getByRole('status')).toContainText('已选择 0/4');
 for(const label of ['试验圆柱面 0','试验圆柱面 1','试验定位面 0','试验定位面 1']){
  await filter.fill(label);const item=page.getByText(label,{exact:true});
  const role=label.includes('圆柱')?'轴线':'定位平面';const before=['试验圆柱面 0','试验圆柱面 1','试验定位面 0','试验定位面 1'].indexOf(label);
  if(label==='试验圆柱面 0'){
   await page.evaluate(()=>(window as any).__motionFixture.failInspection=true);await item.click();
   await expect(page.locator('.cad-operation-notification')).toContainText('几何支持未完成');
   await expect(dialog.getByRole('status')).toContainText(`已选择 ${before}/4`);
   await page.evaluate(()=>(window as any).__motionFixture.failInspection=false);
  }
  await item.hover();await item.click();await expect(dialog.getByRole('status')).toContainText(`已选择 ${before+1}/4`);
  await expect(page.getByTestId('viewport-feature-selection')).toHaveAttribute('data-count',String(before+1));
  if(label==='试验圆柱面 1')await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.previews.some((p:any)=>p.draftJointId&&!p.mechanism.joints.at(-1).first.plane))).toBe(true);
 }
 await expect(dialog.getByRole('button',{name:/^保\s*存$/})).toBeEnabled();
 await dialog.getByRole('button',{name:'清除第一轴线',exact:true}).click();await expect(dialog.getByRole('status')).toContainText('已选择 2/4');
 await expect(dialog.getByRole('button',{name:/^保\s*存$/})).toBeDisabled();
 await filter.fill('试验圆柱面 0');await page.getByText('试验圆柱面 0',{exact:true}).click();await expect(dialog.getByRole('button',{name:'第一定位平面',exact:true})).toHaveClass(/ant-btn-primary/);
 await filter.fill('试验定位面 0');await page.getByText('试验定位面 0',{exact:true}).click();await expect(dialog.getByRole('status')).toContainText('已选择 4/4');
 await expect(dialog.getByRole('button',{name:/^保\s*存$/})).toBeEnabled();await dialog.getByRole('button',{name:/^保\s*存$/}).click();
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(1);
 const joint=await page.evaluate(()=>(window as any).__motionFixture.saved[0].kinematics.mechanisms[0].joints.at(-1));expect(joint.first.instanceId).not.toBe(joint.second.instanceId);expect(joint.first.axis.derivedRole).toBe('cylinder-axis');expect(joint.first.plane.topologyId).toBe(3);expect(errors).toEqual([]);
});

test('dialog replay uses whole frames and publishes the selected frame',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);await start(page);
 const result=page.getByRole('dialog',{name:'仿真与回放',exact:true});await expect(result.getByText(/帧 1\/5 · t=0.000/)).toBeVisible();
 await expect(page.locator('.workbench-status').getByText(/机构回放 · 帧 1/)).toBeVisible();await expect(page.locator('.motion-study-activity')).toHaveCount(0);
 await expect(result.getByRole('button',{name:'恢复正式姿态',exact:true})).toHaveCount(0);
 await result.getByRole('button',{name:'下一步',exact:true}).click();await expect(result.getByText(/帧 2\/5/)).toBeVisible();
 await result.getByRole('button',{name:'计算仿真',exact:true}).click();await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.requests.length)).toBe(2);
 await expect(result.getByText(/帧 1\/5/)).toBeVisible();expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(1);
 await page.evaluate(()=>(window as any).__motionFixture.rejectNextSave=true);await result.getByRole('button',{name:'关闭并返回机构编辑',exact:true}).click();
 await expect(result.getByText('操作未完成，请查看诊断详情后重试。',{exact:true})).toBeVisible();await expect(result.getByText(/帧 1\/5/)).toBeVisible();
 expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(1);
 await page.keyboard.press('Escape');await expect(page.locator('.workbench-status')).not.toContainText('机构回放');
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(2);
 await command(page,'旋转接合');await expect(page.getByRole('dialog',{name:/^旋转\./})).toBeVisible();await page.keyboard.press('Escape');
 await command(page,'已保存结果');await expect(result).toBeVisible();
 await expect(page.getByRole('dialog')).toHaveCount(1);await expect(page.getByRole('button',{name:'播放',exact:true})).toHaveCount(1);
 await result.getByRole('button',{name:'终点',exact:true}).click();await expect(result.getByText(/帧 5\/5/)).toBeVisible();
 await result.getByRole('button',{name:'起点',exact:true}).click();await expect(result.getByText(/帧 1\/5/)).toBeVisible();
 await result.getByRole('button',{name:'下一步',exact:true}).click();await expect(result.getByText(/帧 2\/5 · t=1.000/)).toBeVisible();
 await result.getByRole('button',{name:'转换为产品约束',exact:true}).click();const apply=page.getByRole('dialog',{name:'转换为产品约束',exact:true});await expect(apply).toBeVisible();
 await apply.getByRole('button',{name:'生成／刷新转换计划'}).click();await expect(apply.getByText(/DOF 1/)).toBeVisible();await page.keyboard.press('Escape');
 await expect(result).toBeVisible();await expect(page.getByRole('dialog')).toHaveCount(1);await expect(result.getByText(/帧 2\/5/)).toBeVisible();
 await expect(page.locator('.workbench-status').getByText(/机构回放 · 帧 2/)).toBeVisible();
 await result.getByRole('button',{name:'转换为产品约束',exact:true}).click();await apply.getByRole('button',{name:'生成／刷新转换计划'}).click();await apply.getByRole('button',{name:'一次提交并返回装配'}).click();
 await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();expect(await page.evaluate(()=>(window as any).__motionFixture.applied[0].motionApply)).toMatchObject({frameIndex:1,lockAngle:false,planDigest:'fixture-plan'});expect(errors).toEqual([]);
});

test('simulation dialog owns calculation cancellation while close preserves the background job',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);
 await page.evaluate(()=>(window as any).__motionFixture.holdJob=true);await start(page);
 const dialog=page.getByRole('dialog',{name:'仿真与回放',exact:true});await expect(dialog.getByRole('button',{name:'停止计算',exact:true})).toBeEnabled();
 await dialog.getByRole('button',{name:'关闭并返回机构编辑',exact:true}).click();await expect(dialog).toHaveCount(0);
 expect(await page.evaluate(()=>(window as any).__motionFixture.jobs[0].state)).toBe('RUNNING');expect(await page.evaluate(()=>(window as any).__motionFixture.canceled??[])).toEqual([]);
 await command(page,'已保存结果');await dialog.getByRole('button',{name:'停止计算',exact:true}).click();await expect(dialog.getByRole('button',{name:'停止计算',exact:true})).toHaveCount(0);
 expect(await page.evaluate(()=>(window as any).__motionFixture.canceled)).toEqual(['fixture-run']);await page.keyboard.press('Escape');expect(errors).toEqual([]);
});

test('late results and preview responses cannot reopen an exited session',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);
 await page.evaluate(()=>(window as any).__motionFixture.blocked=true);await start(page);
 await expect.poll(()=>page.evaluate(()=>typeof (window as any).__motionFixture.release)).toBe('function');await page.keyboard.press('Escape');await command(page,'退出机构');await page.evaluate(()=>(window as any).__motionFixture.release());
 await enter(page,true);await expect(page.locator('.workbench-status').getByText(/机构回放/)).toHaveCount(0);
 await page.evaluate(()=>(window as any).__motionFixture.previewBlocked=true);await command(page,'固定件');await expect.poll(()=>page.evaluate(()=>typeof (window as any).__motionFixture.previewRelease)).toBe('function');
 await page.keyboard.press('Escape');await command(page,'退出机构');await page.evaluate(()=>(window as any).__motionFixture.previewRelease());await expect(page.getByRole('dialog')).toHaveCount(0);await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);expect(errors).toEqual([]);
});


test('entered definitions use standard rename and deletion commands',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page);await enter(page);
 await command(page,'固定件');await page.getByRole('dialog',{name:/^固定零件\./}).getByRole('button',{name:/^保\s*存$/}).click();
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.length),{timeout:30_000}).toBe(2);
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill('固定零件.1 (');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText(/^固定零件\.1 \(/).click({button:'right'});
 await page.getByRole('menuitem',{name:'重命名',exact:true}).click();const rename=page.getByRole('dialog',{name:'重命名建模对象',exact:true});
 await expect(rename.getByRole('textbox',{name:/名称$/})).toHaveValue('固定零件.1');await rename.getByRole('textbox',{name:/名称$/}).fill('圆柱固定');
 await rename.getByRole('button',{name:/^确\s*定$/}).click();await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(3);
 await filter.fill('圆柱固定');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText(/^圆柱固定 \(/).dblclick();
 await expect(page.getByRole('dialog',{name:'圆柱固定',exact:true})).toBeVisible();await page.keyboard.press('Escape');
 await filter.fill('机械装置.1');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText('机械装置.1，自由度=0',{exact:true}).click({button:'right'});await page.getByRole('menuitem',{name:'删除',exact:true}).click();
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.at(-1).kinematics.mechanisms.length)).toBe(0);
 await expect(page.locator('.workbench-status')).toContainText('机构编辑');await expect(page.locator('.workbench-status')).not.toContainText('机械装置.1');expect(errors).toEqual([]);
});


test('basic mechanism command catalog excludes retired functions and simulation remains editable',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);
 for(const name of ['移动接合','刚性接合','干涉检查','恢复正式姿态','机构示例','单坐标试动','装配关联','播放','暂停','上一步','单步','复位','取消后台任务']){
  await page.getByRole('button',{name:'搜索工具',exact:true}).click();await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);
  await expect(page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp('^'+name+'$')})})).toHaveCount(0);await page.keyboard.press('Escape');
 }
 await command(page,'转换为产品约束');const plan=page.getByRole('dialog',{name:'转换为产品约束',exact:true});await plan.getByRole('button',{name:'生成／刷新转换计划'}).click();
 await expect(plan).toContainText('机构已确认完整位姿');await expect(plan.locator('li').filter({hasText:'→ ()'})).toHaveCount(0);await page.keyboard.press('Escape');
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill('旋转研究');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText('旋转研究',{exact:true}).dblclick();
 await expect(page.getByRole('dialog',{name:'仿真定义',exact:true})).toBeVisible();await page.keyboard.press('Escape');expect(errors).toEqual([]);
});


test('joint relation children edit their owner through the shared tree command',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill('重合.1');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText(/^重合\.1 \(/).first().dblclick();
 const dialog=page.getByRole('dialog',{name:'轴接合',exact:true});await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:'第一轴线',exact:true})).toHaveClass(/ant-btn-primary/);await page.keyboard.press('Escape');
 await filter.fill('偏移.1');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText(/^偏移\.1 \(/).first().dblclick();await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:'第一定位平面',exact:true})).toHaveClass(/ant-btn-primary/);await page.keyboard.press('Escape');
 expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);expect(errors).toEqual([]);
});

test('failed new mechanism confirmation preserves the active application',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);
 await page.evaluate(()=>(window as any).__motionFixture.rejectNextSave=true);
 await command(page,'新建机械装置');const dialog=page.getByRole('dialog',{name:'编辑机械装置',exact:true});
 await dialog.getByRole('button',{name:/^保\s*存$/}).click();await expect(dialog.getByText('操作未完成，请查看诊断详情后重试。',{exact:true})).toBeVisible();
 await dialog.getByText('诊断详情',{exact:true}).click();await expect(dialog.getByText('fixture rejected definition',{exact:true})).toBeVisible();
 await expect(page.locator('[data-application-id="m"]')).toBeVisible();expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);
 await page.keyboard.press('Escape');await command(page,'退出机构');await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();
 expect(await page.evaluate(()=>(window as any).__motionFixture.exits.at(-1).mechanismApplication.sourceMechanismId)).toBe('m');expect(errors).toEqual([]);
});
