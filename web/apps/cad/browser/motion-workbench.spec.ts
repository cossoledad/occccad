import {viewportProbe,expectViewport} from "./camera-helpers";
import {expect,test,type Page} from '@playwright/test';

test.setTimeout(180_000);

// Browser contract fixtures replace API responses only in this test's page.
// No fixture is a numerical solver or an OCCT interference result.
async function fixture(page:Page,withDefinitions=false,duration=4){
 await page.goto('/documents/mock-product-frame');
 await expect(page.locator('canvas')).toBeVisible({timeout:60_000});
 await page.evaluate(async ({seeded,duration})=>{
  const {api}=await import('/src/api/client.ts');
  const {queryClient}=await import('/src/app/providers.tsx');
  const documentId='mock-product-frame',originalGet=api.getDocument.bind(api),originalCommand=api.command.bind(api);
  const initial=await originalGet(documentId),instances=initial.product!.instances;
  const end=(instanceId:string)=>({instanceId,frame:{translation:[0,0,0],rotation:[0,0,0,1]},axis:{instanceId,kind:'AXIS',geometryId:'axis-system-default',axis:'Z'},plane:{instanceId,kind:'PLANE',geometryId:'datum-xy'},capturedX:[1,0,0]});
  let defs:any={mechanisms:[],drivers:[],studies:[],associations:[]};
  if(seeded){defs={...defs,
   mechanisms:[{id:'m',name:'测试机构',unitIds:instances.map(i=>i.id),joints:[
    {id:'g',name:'圆柱固定',kind:'GROUND',first:end(instances[0].id)},
    {id:'j',name:'轴接合',kind:'REVOLUTE',first:end(instances[0].id),second:end(instances[1].id),coordinates:[{id:'q',role:'AXIAL_ROTATION',dimension:'ANGLE',zero:{value:0,unit:'deg'},direction:1,lower:{value:-360,unit:'deg'},upper:{value:720,unit:'deg'}}]}]}],
   drivers:[{id:'d',name:'角度驱动',mechanismId:'m',jointId:'j',coordinateId:'q',reference:{value:0,unit:'deg'},referenceDigest:'fixture-coordinate'}],
   studies:[{id:'s',name:'旋转研究',mechanismId:'m',activeDriverIds:['d'],durationSeconds:duration,frames:5,budgetMs:60000,sourceRevisionId:initial.document.versionId,basisDigest:'fixture-basis',geometryDigest:'fixture-geometry',references:{d:{value:0,unit:'deg'}},keyframes:[{id:'key-start',timeSeconds:0,values:{d:{value:0,unit:'deg'}}},{id:'key-end',timeSeconds:duration,values:{d:{value:360,unit:'deg'}}}]}]}}

  for(const m of defs.mechanisms){m.poses=Object.fromEntries(instances.map((i,n)=>[i.id,{translation:n?[0,0,7]:[0,0,0],rotation:[0,0,0,1]}]));m.coordinates={q:0};}
  for(const s of defs.studies){const m=defs.mechanisms.find((m:any)=>m.id===s.mechanismId);s.initialPoses=structuredClone(m.poses);s.initialCoordinates=structuredClone(m.coordinates);}
  const state:any={saved:[],previews:[],requests:[],applied:[],inspected:[],jobs:[],blocked:false,release:undefined};
  (window as any).__motionFixture=state;
  let adopted:any;
  const decorate=(source:any)=>{
   for(const m of defs.mechanisms)for(const j of m.joints)if(!j.constraints){
    const c=(id:string,name:string,kind:string,first:any,second?:any)=>({id,name,kind,first,second,relationRole:kind==='DISTANCE'?'axial-location':kind==='FIX'?'ground':'axis',evaluationStatus:'VERIFIED'});
    j.constraints=j.kind==='GROUND'?[c('relation-'+j.id+'-fixed','固定.1','FIX',{kind:'BODY',instanceId:j.first.instanceId})]:[c('relation-'+j.id+'-coincidence','重合.1','COINCIDENT',j.first.axis,j.second.axis),c('relation-'+j.id+'-offset','偏移.1','DISTANCE',j.first.plane,j.second.plane)];
   }
   const view=structuredClone(source);view.product.kinematics=structuredClone(defs);if(state.permission)view.document.permission=state.permission;
   if(adopted){for(const i of view.product.instances)Object.assign(i,adopted[i.id]);for(const i of view.resolvedInstances??[])Object.assign(i,adopted[i.instancePath.segments[0].instanceId]);}
   const node=(kind:string,obj:any)=>({id:`application:${obj.id}`,kind,name:obj.name,entityId:obj.id,documentId,ownerDocumentId:documentId,versionId:view.document.versionId,subject:{documentId,entityKind:kind,entityId:obj.id},snapshot:{revisionId:view.document.versionId},capabilities:['EDIT','DELETE','RENAME']});
   view.structureTree.children=view.structureTree.children.filter((n:any)=>n.kind!=='APPLICATIONS');
   const project=(m:any,parent:string,reference=false)=>{
    const mn={...node(reference?'MECHANISM_REFERENCE':'MECHANISM',m),id:parent+'/mechanism:'+m.id,mechanismId:m.id,name:m.name+'，自由度=0',children:[] as any[]};
    const group=(key:string,name:string)=>({id:mn.id+'/'+key,kind:'KINEMATICS_GROUP',name,children:[] as any[]});
    const joints=group('joints','接合'),commands=group('commands','命令'),ground=group('ground','固定零件');
    const nameFor=(id:string)=>instances.find(i=>i.id===id)?.name??id;
    for(const j of m.joints){
     const suffix=' ('+nameFor(j.first.instanceId)+(j.second?','+nameFor(j.second.instanceId):'')+')';
     const jn={...node('MECHANISM_JOINT',j),id:mn.id+'/joint:'+j.id,mechanismId:m.id,name:j.name+suffix,children:[] as any[]};
     if(!j.constraints){const c=(id:string,name:string,kind:string,first:any,second?:any)=>({id,name,kind,first,second,relationRole:kind==='DISTANCE'?'axial-location':kind==='FIX'?'ground':'axis',evaluationStatus:'VERIFIED'});j.constraints=j.kind==='GROUND'?[c('relation-'+j.id+'-fixed','固定.1','FIX',{kind:'BODY',instanceId:j.first.instanceId})]:[c('relation-'+j.id+'-coincidence','重合.1','COINCIDENT',j.first.axis,j.second.axis),c('relation-'+j.id+'-offset','偏移.1','DISTANCE',j.first.plane,j.second.plane)];}
     jn.children=j.constraints.map((c:any)=>({...node('JOINT_EXPANSION',c),id:jn.id+'/relation:'+c.id,name:c.name+suffix,mechanismId:m.id,ownerEntityId:j.id,relationRole:c.kind==='DISTANCE'?'axial-location':c.kind==='FIX'?'ground':'axis',entityType:c.kind,evaluationStatus:c.evaluationStatus,presentationRole:'DEFINITION',capabilities:['EDIT']}));
     (j.kind==='GROUND'?ground:joints).children.push(jn);
    }
    commands.children=defs.drivers.filter((d:any)=>d.mechanismId===m.id).map((d:any)=>({...node('MOTION_DRIVER',d),id:mn.id+'/command:'+d.id,mechanismId:m.id}));
    mn.children=[joints,commands,ground];return mn;
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
   else {state.applied.push(structuredClone(input));adopted=input.motionApply?.mechanismId?defs.mechanisms.find((m:any)=>m.id===input.motionApply.mechanismId).poses:state.run.frames.find((f:any)=>f.timeSeconds===input.motionApply!.timeSeconds)!.unitPoses;}
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
   const frames=[0,90,180,270,360].map((angle,index)=>({timeSeconds:index*study.durationSeconds/4,commandValues:{d:angle*Math.PI/180},coordinates:{q:angle*Math.PI/180},kinematicValid:true,unitPoses:Object.fromEntries(frozen.product!.instances.map((i,n)=>[i.id,{translation:n?[0,0,7]:[0,0,0],rotation:n?[0,0,Math.sin(angle*Math.PI/360),Math.cos(angle*Math.PI/360)]:[0,0,0,1]}]))}));
   state.run={schema:2,status:'COMPLETED',completed:true,elapsedMs:12,solveCalls:5,solverBuild:'browser-contract-fixture',frames,snapshot:{documentId:id,revisionId:frozen.document.versionId,digest:'fixture',view:frozen,mechanism:defs.mechanisms[0],study}};
   const job={id:'fixture-run-'+state.requests.length,type:'MOTION_STUDY',documentId:id,versionId:frozen.document.versionId,state:state.holdJob?'RUNNING':'SUCCEEDED',progress:state.holdJob?20:100,canCancel:Boolean(state.holdJob),createdAt:new Date(1700000000000+state.requests.length*1000).toISOString(),payload:{studyId:study.id,studyName:study.name},resultObjectId:state.holdJob?undefined:'fixture-artifact'};state.jobs.push(job);state.runs??={};state.runs[job.id]=structuredClone(state.run);return structuredClone(job) as any;
  };
  api.cancelJob=async id=>{state.canceled??=[];state.canceled.push(id);const job=state.jobs.find((j:any)=>j.id===id);Object.assign(job,{state:'CANCELED',canCancel:false});return structuredClone(job);};
  api.deleteMotionResult=async id=>{const job=state.jobs.find((j:any)=>j.id===id);state.jobs=state.jobs.filter((j:any)=>j.id!==id);state.deleted=id;return {...job,userVisible:false}};
  api.getMotionRun=async id=>{if(state.blocked)await new Promise<void>(resolve=>state.release=resolve);return structuredClone(state.runs?.[id]??state.run);};
  api.prepareMotionPose=async(id,baseRevisionId,request)=>{
   state.poseRequest=structuredClone(request);const run=state.runs[request.jobId!],frame=run.frames.find((f:any)=>f.timeSeconds===request.timeSeconds);
   if(!frame)throw new Error('fixture requires a sampled frame');
   const kinematics=structuredClone(defs),m=kinematics.mechanisms.find((m:any)=>m.id===run.snapshot.mechanism.id);m.poses=frame.unitPoses;m.coordinates=frame.coordinates;
   return {type:'SAVE_KINEMATICS',versionId:baseRevisionId,requestId:'fixture-adopt',kinematics};
  };
  api.beginMechanismSession=async(_id,input)=>{
   const m=defs.mechanisms.find((m:any)=>m.id===input.mechanismId);
   return {sessionId:'fixture-session',baseRevisionId:input.baseRevisionId,inputDigest:'fixture-input',sequence:0,accepted:true,qualified:true,solveMs:0,basisDigest:'fixture-basis',geometryDigest:'fixture-geometry',initialPoses:structuredClone(m.poses),activeDriverIds:['d'],drivers:structuredClone(defs.drivers),study:structuredClone(defs.studies.find((s:any)=>s.id===input.studyId)),frame:{timeSeconds:0,commandValues:{d:0},coordinates:{q:0},unitPoses:structuredClone(m.poses),kinematicValid:true}};
  };
  api.cancelMechanismSession=async(_id,sessionId)=>{(state.closedSessions??=[]).push(sessionId);};
  api.previewMechanism=async(id,req)=>{state.previews.push(structuredClone(req));if(state.previewBlocked)await new Promise<void>(resolve=>state.previewRelease=resolve);return {baseRevisionId:req.baseRevisionId,mechanism:{...req.mechanism,poses:Object.fromEntries(instances.map((i,n)=>[i.id,{translation:n?[0,0,7]:[0,0,0],rotation:[0,0,0,1]}]))},instancePoses:instances.map((i,n)=>({instanceId:i.id,translation:n?[0,0,7]:[0,0,0],rotation:[0,0,0,1]}))} as any};
  api.inspectAssemblySupports=async(id,refs)=>{state.inspected.push(structuredClone(refs));if(state.failInspection)throw new Error('fixture temporary inspection failure');return {supports:refs.map(reference=>({reference,descriptor:{Kind:reference.derivedRole?'AXIS':reference.kind==='AXIS'?'AXIS':reference.kind==='PLANE'||reference.topologyId===3?'PLANE':reference.topologyId===99?'SPHERE':'CYLINDER'}}))} as any;};

  api.planMotionApply=async(_id,version,request)=>{state.planRequest=structuredClone(request);return {baseRevisionId:version,digest:'fixture-plan',ready:true,items:[{role:'ground',constraintId:'fixed',action:'ADD'},{role:'axis',constraintId:'coaxial',action:'ADD'},{role:'axial-location',constraintId:'offset',action:'ADD'}],poseChanges:instances.map(i=>i.id),solverStatus:'CONVERGED',degreesOfFreedom:1};};
  // A changed definition requires a new Revision, just like the live contract.
  await api.updateDocument(documentId,initial.document.name,initial.document.description??'');
  await queryClient.invalidateQueries();
 },{seeded:withDefinitions,duration});
}
async function command(page:Page,name:string){
 await page.getByRole('button',{name:'搜索工具',exact:true}).click();
 await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);
 const result=page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp('^'+name+'$')})});
 await expect(result).toHaveCount(1);await expect(result).toBeEnabled();await result.click();
}


async function enter(page:Page,existing=false){
 if(existing){const name=await page.evaluate(()=>(window as any).__motionFixture.saved.at(-1)?.kinematics.mechanisms[0]?.name??'测试机构');const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill(name);const row=page.getByRole('treeitem').filter({has:page.getByText(name+'，自由度=0',{exact:true})}).first();await expect(row).toHaveAttribute('data-action-enabled','true');await row.dblclick();await filter.fill('');}
 else {await command(page,'机构');await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.at(-1)?.kinematics.mechanisms.length)).toBe(1);}
 await expect(page.getByRole('tab',{name:'机构与 DMU',exact:true})).toBeVisible();
 await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toHaveCount(0);
}
async function openResult(page:Page){
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill('结果.1');
 const row=page.getByRole('treeitem').filter({hasText:/^结果\.1 ·/});await expect(row).toHaveAttribute('data-action-enabled','true');await row.dblclick();await filter.fill('');
 await expect(page.getByRole('dialog',{name:'仿真与回放',exact:true})).toBeVisible();
}
async function start(page:Page){
 // Seed an immutable background result at the API boundary; playback starts from its tree entry.
 await page.evaluate(async()=>{const {api}=await import('/src/api/client.ts');const {queryClient}=await import('/src/app/providers.tsx');const id='mock-product-frame',view=await api.getDocument(id);await api.startMotionRun(id,{baseRevisionId:view.document.versionId,studyId:'s',requestId:crypto.randomUUID()});await queryClient.invalidateQueries({queryKey:['motion-jobs',id]});});
 await openResult(page);
}

test('tree edit enters an existing mechanism application without leaving a phantom form',async({page})=>{
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));await fixture(page,true);await enter(page,true);
 await expect(page.getByRole('dialog')).toHaveCount(0);
 await command(page,'旋转接合');await expect(page.getByRole('dialog',{name:/旋转/})).toBeVisible();await page.keyboard.press('Escape');
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill('测试机构');
 const row=page.getByRole('treeitem').filter({hasText:'测试机构，自由度=0'}).last();await row.focus();await page.keyboard.press('Enter');
 await expect(page.getByRole('dialog')).toHaveCount(0);await filter.fill('');
 await command(page,'旋转接合');await expect(page.getByRole('dialog',{name:/旋转/})).toBeVisible();await page.keyboard.press('Escape');
 expect(errors).toEqual([]);
});

test('joint toolbar is flat and prismatic plane direction is saved independently',async({page})=>{
 test.setTimeout(60_000);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page);await enter(page);
 const definition=page.getByLabel('机械装置定义',{exact:true});
 for(const name of ['旋转接合','棱形接合','圆柱接合','平面接合','刚性接合','球面接合'])await expect(definition.getByRole('button',{name,exact:true})).toBeVisible();
 await expect(definition.locator('[aria-haspopup="menu"]')).toHaveCount(0);
 await command(page,'棱形接合');const dialog=page.getByRole('dialog',{name:/^棱形\./});
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});
 for(const label of ['试验轴线 0','试验轴线 1','试验定位面 0','试验定位面 1']){await filter.fill(label);await page.getByText(label,{exact:true}).click();}
 await dialog.getByRole('combobox',{name:'平面方向',exact:true}).click();await page.locator('.ant-select-item-option-content').getByText('相对',{exact:true}).click();
 await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await dialog.getByRole('button',{name:/^确\s*定$/}).click();await expect(dialog).toHaveCount(0);
 const joint=await page.evaluate(()=>(window as any).__motionFixture.saved.at(-1).kinematics.mechanisms[0].joints.at(-1));
 expect(joint.kind).toBe('PRISMATIC');expect(joint.axialDirection).toBe('OPPOSITE');expect(joint.axisDirection??'SAME').toBe('SAME');expect(joint.orientationAngle.value).toBe(0);
 expect(errors).toEqual([]);
});

test('mechanism entry creates without selection, edits selected mechanisms and discards untouched empties',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);
 await command(page,'机构');await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.at(-1)?.kinematics.mechanisms.length)).toBe(2);
 await expect(page.getByRole('dialog')).toHaveCount(0);await command(page,'退出机构');
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.at(-1)?.kinematics.mechanisms.length)).toBe(1);
 const filter=page.getByRole('textbox',{name:'筛选模型结构'}),tree=page.getByRole('complementary',{name:'模型结构',exact:true});
 await filter.fill('测试机构');await tree.getByText('测试机构，自由度=0',{exact:true}).first().click();await command(page,'机构');
 await expect(page.locator('[data-application-id="m"]')).toBeVisible();
 expect(await page.evaluate(()=>(window as any).__motionFixture.saved.at(-1).kinematics.mechanisms.length)).toBe(1);
 await filter.fill('');await command(page,'退出机构');
 await command(page,'机构');await command(page,'旋转接合');await page.keyboard.press('Escape');await command(page,'退出机构');
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.at(-1)?.kinematics.mechanisms.length)).toBe(1);expect(errors).toEqual([]);
});

test('filtered tree picks advance roles and preview axes before locating planes',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);await command(page,'旋转接合');
 const dialog=page.getByRole('dialog',{name:/^旋转\./});await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeDisabled();
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});
 // A plane is never accepted by the axial role.
 await filter.fill('试验平面 0');await page.getByText('试验平面 0',{exact:true}).click();await expect(dialog.locator('.feature-pick-field').filter({hasText:'第一轴线'})).toContainText('未选择');
 for(const label of ['试验圆柱面 0','试验圆柱面 1','试验定位面 0','试验定位面 1']){
  await filter.fill(label);const item=page.getByText(label,{exact:true});
  const role=label.includes('圆柱')?'轴线':'定位平面';const before=['试验圆柱面 0','试验圆柱面 1','试验定位面 0','试验定位面 1'].indexOf(label);
  if(label==='试验圆柱面 0'){
   await page.evaluate(()=>(window as any).__motionFixture.failInspection=true);await item.click();
   await expect(page.locator('.cad-operation-notification')).toContainText('几何支持未完成');
   await expect(dialog.locator('.feature-pick-field').filter({hasText:'第一轴线'})).toContainText('未选择');
   await page.evaluate(()=>(window as any).__motionFixture.failInspection=false);
  }
  await item.hover();await item.click();await expectViewport(viewportProbe(page,"viewport-feature-selection")).toHaveAttribute('data-count',String(before+1));
  await expectViewport(viewportProbe(page,"viewport-feature-selection")).toHaveAttribute('data-count',String(before+1));
  if(label==='试验圆柱面 1')await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.previews.some((p:any)=>p.draftJointId&&!p.mechanism.joints.at(-1).first.plane))).toBe(true);
 }
 await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await dialog.getByRole('button',{name:'移除第一轴线',exact:true}).click();await expect(dialog.locator('.feature-pick-field').filter({hasText:'第一定位平面'})).toContainText('先选轴线');
 await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeDisabled();
 await filter.fill('试验圆柱面 0');await page.getByText('试验圆柱面 0',{exact:true}).click();await expect(dialog.getByRole('button',{name:'选择第一定位平面',exact:true})).toHaveAttribute('aria-pressed','true');
 await filter.fill('试验定位面 0');await page.getByText('试验定位面 0',{exact:true}).click();await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();
 await expect(dialog.getByRole('button',{name:/^确\s*定$/})).toBeEnabled();await dialog.getByRole('button',{name:/^确\s*定$/}).click();await expect(dialog).toHaveCount(0);
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(1);
 const joint=await page.evaluate(()=>(window as any).__motionFixture.saved[0].kinematics.mechanisms[0].joints.at(-1));expect(joint.first.instanceId).not.toBe(joint.second.instanceId);expect(joint.first.axis.derivedRole).toBe('cylinder-axis');expect(joint.first.plane.topologyId).toBe(3);expect(errors).toEqual([]);
});

test('dialog replay uses whole frames and publishes the selected frame',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);await start(page);
 const result=page.getByRole('dialog',{name:'仿真与回放',exact:true});await expect(result.getByText(/1\/5 · 0.00/)).toBeVisible();
 await expect(page.locator('.workbench-status').getByText(/机构回放 · 帧 1/)).toBeVisible();await expect(page.locator('.motion-study-activity')).toHaveCount(0);
 await expect(result.getByRole('button',{name:'恢复正式姿态',exact:true})).toHaveCount(0);
 await result.getByRole('button',{name:'下一步',exact:true}).click();await expect(result.getByText(/2\/5/)).toBeVisible();
 await expect(result.getByRole('button',{name:'计算仿真',exact:true})).toHaveCount(0);
 expect(await page.evaluate(()=>(window as any).__motionFixture.requests.length)).toBe(1);
 expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);
 await page.evaluate(()=>(window as any).__motionFixture.rejectNextSave=true);await result.getByRole('button',{name:/^确\s*定$/,exact:true}).click();
 await expect(result.getByText('操作未完成，请查看诊断详情后重试。',{exact:true})).toBeVisible();await expect(result.getByText(/2\/5/)).toBeVisible();
 expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);
 expect(await page.evaluate(()=>(window as any).__motionFixture.poseRequest)).toMatchObject({jobId:'fixture-run-1',timeSeconds:1});
 await page.keyboard.press('Escape');await expect(page.locator('.workbench-status')).not.toContainText('机构回放');
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);
 await command(page,'旋转接合');await expect(page.getByRole('dialog',{name:/^旋转\./})).toBeVisible();await page.keyboard.press('Escape');
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill('结果.1');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText(/^结果\.1 ·/).dblclick();await expect(result).toBeVisible();
 await expect(page.getByRole('dialog')).toHaveCount(1);await expect(page.getByRole('button',{name:'播放',exact:true})).toHaveCount(1);
 await result.getByRole('button',{name:'终点',exact:true}).click();await expect(result.getByText(/5\/5/)).toBeVisible();
 await result.getByRole('button',{name:'起点',exact:true}).click();await expect(result.getByText(/1\/5/)).toBeVisible();
 await result.getByRole('button',{name:'下一步',exact:true}).click();await expect(result.getByText(/2\/5 · 1.00/)).toBeVisible();
 await command(page,'转换为产品约束');const apply=page.getByRole('dialog',{name:'转换为产品约束',exact:true});await expect(apply).toBeVisible();
 await apply.getByRole('button',{name:'生成／刷新转换计划'}).click();await expect(apply.getByText(/自由度 1/)).toBeVisible();await page.keyboard.press('Escape');
 await expect(result).toBeVisible();await expect(page.getByRole('dialog')).toHaveCount(1);await expect(result.getByText(/2\/5/)).toBeVisible();
 await expect(page.locator('.workbench-status').getByText(/机构回放 · 帧 2/)).toBeVisible();
 await command(page,'转换为产品约束');await apply.getByRole('button',{name:'生成／刷新转换计划'}).click();await apply.getByRole('button',{name:/^确\s*定$/}).click();
 await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();expect(await page.evaluate(()=>(window as any).__motionFixture.applied[0].motionApply)).toMatchObject({frameIndex:0,timeSeconds:1,lockCoordinateIds:[],planDigest:'fixture-plan'});expect(errors).toEqual([]);
});

test('simulation dialog owns calculation cancellation while close preserves the background job',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);
 await page.evaluate(()=>(window as any).__motionFixture.holdJob=true);await start(page);
 const dialog=page.getByRole('dialog',{name:'仿真与回放',exact:true});await expect(dialog.getByRole('button',{name:'停止计算',exact:true})).toBeEnabled();
 await dialog.getByRole('button',{name:/^确\s*定$/,exact:true}).click();await expect(dialog).toHaveCount(0);
 expect(await page.evaluate(()=>(window as any).__motionFixture.jobs[0].state)).toBe('RUNNING');expect(await page.evaluate(()=>(window as any).__motionFixture.canceled??[])).toEqual([]);
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill('结果.1');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText(/^结果\.1 ·/).dblclick();await dialog.getByRole('button',{name:'停止计算',exact:true}).click();await expect(dialog.getByRole('button',{name:'停止计算',exact:true})).toHaveCount(0);
 expect(await page.evaluate(()=>(window as any).__motionFixture.canceled)).toEqual(['fixture-run-1']);await page.keyboard.press('Escape');expect(errors).toEqual([]);
});

test('late results and preview responses cannot reopen an exited session',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);
 await page.evaluate(()=>(window as any).__motionFixture.blocked=true);await start(page);
 await expect.poll(()=>page.evaluate(()=>typeof (window as any).__motionFixture.release)).toBe('function');await page.keyboard.press('Escape');await command(page,'退出机构');await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();await page.evaluate(()=>(window as any).__motionFixture.release());
 await enter(page,true);await expect(page.locator('.workbench-status').getByText(/机构回放/)).toHaveCount(0);
 await page.evaluate(()=>(window as any).__motionFixture.previewBlocked=true);await command(page,'固定件');await page.getByRole('textbox',{name:'筛选模型结构'}).fill('试验圆柱面 0');await page.getByText('试验圆柱面 0',{exact:true}).click();await expect.poll(()=>page.evaluate(()=>typeof (window as any).__motionFixture.previewRelease)).toBe('function');
 await page.keyboard.press('Escape');await command(page,'退出机构');await page.evaluate(()=>(window as any).__motionFixture.previewRelease());await expect(page.getByRole('dialog')).toHaveCount(0);await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);expect(errors).toEqual([]);
});


test('entered definitions use standard rename and deletion commands',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page);await enter(page);
 await command(page,'固定件');await page.getByRole('textbox',{name:'筛选模型结构'}).fill('试验圆柱面 0');await page.getByText('试验圆柱面 0',{exact:true}).click();await page.getByRole('dialog',{name:/^固定零件\./}).getByRole('button',{name:/^确\s*定$/}).click();
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionFixture.saved.length),{timeout:30_000}).toBe(2);
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill('固定零件.1 (');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText(/^固定零件\.1 \(/).click({button:'right'});
 await page.getByRole('menuitem',{name:'属性…',exact:true}).click();const rename=page.getByRole('dialog',{name:/^属性 ·/});
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
 for(const name of ['旋转接合','棱形接合','圆柱接合','平面接合','刚性接合','球面接合','使用命令进行模拟','模拟'])await expect(page.getByRole('button',{name,exact:true})).toBeEnabled();
 for(const name of ['移动接合','干涉检查','恢复正式姿态','机构示例','单坐标试动','装配关联','播放','暂停','上一步','单步','复位','取消后台任务']){
  await page.getByRole('button',{name:'搜索工具',exact:true}).click();await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);
  await expect(page.locator('.workbench-command-result').filter({has:page.locator('strong',{hasText:new RegExp('^'+name+'$')})})).toHaveCount(0);await page.keyboard.press('Escape');
 }
 await command(page,'转换为产品约束');const plan=page.getByRole('dialog',{name:'转换为产品约束',exact:true});await plan.getByRole('button',{name:'生成／刷新转换计划'}).click();
 await expect(plan).toContainText('机械装置已确认位姿');await expect(plan.locator('li').filter({hasText:'→ ()'})).toHaveCount(0);await page.keyboard.press('Escape');
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill('旋转研究');await page.getByRole('complementary',{name:'模型结构',exact:true}).getByText('旋转研究',{exact:true}).dblclick();
 await expect(page.getByRole('dialog',{name:'模拟',exact:true})).toBeVisible();await expect(page.getByRole('dialog',{name:'使用命令进行模拟',exact:true}).getByRole('spinbutton',{name:'命令角度'})).toBeEnabled();await expect(page.getByRole('dialog')).toHaveCount(2);await page.keyboard.press('Escape');await expect(page.getByRole('dialog')).toHaveCount(0);expect(errors).toEqual([]);
});


test('joint relation children edit their owner through the shared tree command',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill('重合.1');const coincidence=page.getByRole('treeitem').filter({hasText:/重合\.1 \(/}).first();await expect(coincidence).toHaveAttribute('data-action-enabled','true');await coincidence.dblclick();
 const dialog=page.getByRole('dialog',{name:'轴接合',exact:true});await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:'选择第一轴线',exact:true})).toHaveAttribute('aria-pressed','true');await page.keyboard.press('Escape');await expect(dialog).toBeVisible();await page.keyboard.press('Escape');
 await filter.fill('偏移.1');const offset=page.getByRole('treeitem').filter({hasText:/偏移\.1 \(/}).first();await expect(offset).toHaveAttribute('data-action-enabled','true');await offset.dblclick();await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:'选择第一定位平面',exact:true})).toHaveAttribute('aria-pressed','true');await page.keyboard.press('Escape');await expect(dialog).toBeVisible();await page.keyboard.press('Escape');
 expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);expect(errors).toEqual([]);
});

test('failed automatic mechanism creation preserves the active application',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await fixture(page,true);await enter(page,true);
 await page.evaluate(()=>(window as any).__motionFixture.rejectNextSave=true);await command(page,'新建机械装置');
 await expect(page.locator('.cad-operation-notification')).toContainText('机械装置');
 await expect(page.locator('[data-application-id="m"]')).toBeVisible();expect(await page.evaluate(()=>(window as any).__motionFixture.saved.length)).toBe(0);
 await command(page,'退出机构');await expect(page.getByRole('tab',{name:'装配设计',exact:true})).toBeVisible();
 expect(await page.evaluate(()=>(window as any).__motionFixture.exits.at(-1).mechanismApplication.sourceMechanismId)).toBe('m');expect(errors).toEqual([]);
});


test('continuous playback uses two-decimal time, cancel restores poses, and tree deletion keeps the model',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 await page.route('**/src/viewport/cad-viewport-engine.ts*',async route=>{
  const response=await route.fetch();
  await route.fulfill({response,body:await response.text()+`
const motionRender=CadViewportEngine.prototype.render;
CadViewportEngine.prototype.render=function(...args){window.__motionRenders=(window.__motionRenders||0)+1;return motionRender.apply(this,args)};
const motionCommit=CadViewportEngine.prototype.commitApplicationDisplay;
CadViewportEngine.prototype.commitApplicationDisplay=function(){
 if(!this.renderer.__motionTrace){const render=this.renderer.render.bind(this.renderer);this.renderer.__motionTrace=true;
 this.renderer.render=(scene,camera)=>{if(scene===this.scene){window.__motionLastFrame=Object.fromEntries([...this.instanceGroups].map(([id,g])=>[id,{translation:g.position.toArray(),rotation:g.quaternion.toArray()}]));if(this.motionDisplay?.clock)(window.__motionFrames??=[]).push([...this.instanceGroups].map(([id,g])=>({id,rotation:g.quaternion.toArray()})))}return render(scene,camera)};}
 return motionCommit.call(this);
};`});
 });
 await fixture(page,true,20);await enter(page,true);await start(page);
 const dialog=page.getByRole('dialog',{name:'仿真与回放',exact:true}),time=dialog.getByRole('spinbutton',{name:'定位时间 s',exact:true});
 await expect(time).toHaveValue('0.00');
 await expect(dialog.getByRole('button',{name:'删除运行结果',exact:true})).toHaveCount(0);
 await expect(dialog.getByRole('button',{name:'转换为产品约束',exact:true})).toHaveCount(0);
 const read=()=>page.evaluate(async()=>{const {api}=await import('/src/api/client.ts');return api.getDocument('mock-product-frame')});
 const before=await read(),renders=await page.evaluate(()=>(window as any).__motionRenders);
 await dialog.getByRole('button',{name:'播放',exact:true}).click();
 await expect.poll(async()=>Number(await time.inputValue())).toBeGreaterThan(.15);
 await expect(time).toHaveValue(/^\d+\.\d{2}$/);
 const frames=await page.evaluate(()=>(window as any).__motionFrames??[]);
 expect(frames.length).toBeGreaterThan(2);
 expect(frames.some((frame:any[])=>frame.some(p=>Math.abs(p.rotation[2])>.01&&Math.abs(p.rotation[2])<.7))).toBeTruthy();
 expect(await page.evaluate(()=>(window as any).__motionRenders)).toBe(renders);
 await dialog.getByRole('button',{name:'暂停',exact:true}).click();
 await dialog.getByRole('button',{name:'终点',exact:true}).click();await expect(time).toHaveValue('20.00');await dialog.getByRole('button',{name:'上一步',exact:true}).click();await expect(time).toHaveValue('15.00');
 await dialog.getByRole('button',{name:/^取\s*消$/}).click();await expect(dialog).toHaveCount(0);
 await expect.poll(()=>page.evaluate(()=>(window as any).__motionLastFrame)).toEqual(before.product!.kinematics!.mechanisms[0].poses);
 const after=await read();expect(after.document.versionId).toBe(before.document.versionId);expect(after.product!.kinematics).toEqual(before.product!.kinematics);
 expect(await page.evaluate(()=>(window as any).__motionFixture.saved)).toHaveLength(0);
 await page.evaluate(async()=>{(window as any).__motionFixture.permission='VIEWER';const {queryClient}=await import('/src/app/providers.tsx');await queryClient.invalidateQueries()});
 const tree=page.getByRole('complementary',{name:'模型结构',exact:true});await page.getByRole('textbox',{name:'筛选模型结构'}).fill('结果.1');
 await tree.getByText('结果.1 · 完成',{exact:true}).click({button:'right'});await page.getByRole('menuitem',{name:'删除',exact:true}).click();
 await expect(tree.getByText('结果.1 · 完成',{exact:true})).toHaveCount(0);
 expect(await page.evaluate(()=>(window as any).__motionFixture.deleted)).toBe('fixture-run-1');
 const deleted=await read();expect(deleted.document.versionId).toBe(before.document.versionId);expect(deleted.product!.kinematics).toEqual(before.product!.kinematics);
 expect(errors).toEqual([]);
});
