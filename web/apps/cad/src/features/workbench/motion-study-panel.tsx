import {useCallback,useEffect,useRef,useState} from 'react';
import {useQuery,useQueryClient} from '@tanstack/react-query';
import {Alert,Button,Checkbox,Drawer,Input,InputNumber,Select,Slider,Space,Table,Typography} from 'antd';
import {api,isMockMode} from '../../api/client';
import type {DocumentView,Job,SelectionItem} from '../../types';
import {randomUUID} from '../../utils/random-uuid';
import {motionFrameAt,motionPlayback,motionProblemFrames,quantityIn,type JointEndpoint,type KinematicsDefinitions,type MechanismJoint,type MotionPlayback,type MotionRun,type MotionStudy} from '../../cad/assembly/motion-study';

const identity=()=>({translation:[0,0,0] as [number,number,number],rotation:[0,0,0,1] as [number,number,number,number]});
const addressKey=(v:{instancePath:{canonical:string};bodyId:string})=>JSON.stringify([v.instancePath.canonical,v.bodyId]);
const done=(job:Job)=>['SUCCEEDED','FAILED','CANCELED'].includes(job.state);
function Endpoint({value,units,onChange,onError}:{value:JointEndpoint;units:{id:string;name:string}[];onChange:(v:JointEndpoint)=>void;onError:(s:string)=>void}){
 const vector=(raw:string,length:3|4,field:'translation'|'rotation')=>{const values=raw.split(',').map(Number);if(values.length!==length||values.some(v=>!Number.isFinite(v))){onError('坐标格式为逗号分隔的有限数值');return}if(field==='rotation'&&Math.abs(values.reduce((s,v)=>s+v*v,0)-1)>1e-8){onError('四元数必须归一化，顺序为 x,y,z,w');return}onChange({...value,frame:{...value.frame,[field]:values}})};
 return <Space orientation="vertical" size={3}>
  <Select aria-label="刚体运动单元" style={{width:180}} value={value.instanceId} options={units.map(u=>({value:u.id,label:u.name}))} onChange={instanceId=>onChange({...value,instanceId})}/>
  <Input key={'t'+value.frame.translation.join(',')} aria-label="局部原点 mm" addonBefore="原点 mm" defaultValue={value.frame.translation.join(',')} onBlur={e=>vector(e.target.value,3,'translation')}/>
  <Input key={'r'+value.frame.rotation.join(',')} aria-label="局部旋转四元数" addonBefore="x,y,z,w" defaultValue={value.frame.rotation.join(',')} onBlur={e=>vector(e.target.value,4,'rotation')}/>
 </Space>;
}

type Props={view:DocumentView;canEdit:boolean;onSave:(k:KinematicsDefinitions)=>Promise<void>;onPlayback:(p?:MotionPlayback)=>void;onHighlight:(s:SelectionItem[])=>void;onDemo:()=>Promise<void>;onClose:()=>void};
export function MotionStudyPanel({view,canEdit,onSave,onPlayback,onHighlight,onDemo,onClose}:Props){
 const client=useQueryClient();
 const [defs,setDefs]=useState<KinematicsDefinitions>(()=>structuredClone(view.product?.kinematics??{mechanisms:[],studies:[]}));
 const [mechanismId,setMechanismId]=useState(defs.mechanisms[0]?.id??'');
 const [studyId,setStudyId]=useState(defs.studies[0]?.id??'');
 const [pending,setPending]=useState(false),[error,setError]=useState<string>(),[jobId,setJobId]=useState<string>();
 const [run,setRun]=useState<MotionRun>(),[index,setIndex]=useState(0),[playing,setPlaying]=useState(false),[showing,setShowing]=useState(false);
 const [gap,setGap]=useState(0),[scope,setScope]=useState<string[]>([]),[sameUnit,setSameUnit]=useState(false);
 const epoch=useRef(0),currentJob=useRef<string|undefined>(undefined),alive=useRef(true);
 const playbackCallback=useRef(onPlayback);playbackCallback.current=onPlayback;
 const highlightCallback=useRef(onHighlight);highlightCallback.current=onHighlight;
 const selectedMechanism=defs.mechanisms.find(m=>m.id===mechanismId);
 const study=defs.studies.find(s=>s.id===studyId&&s.mechanismId===mechanismId);
 const units=view.product?.instances??[];
 const geometries=view.resolvedInstances??[];
 const scopeAddresses=geometries.filter(g=>scope.includes(addressKey(g))).map(g=>({instancePath:g.instancePath,bodyId:g.bodyId}));
 const jobs=useQuery({queryKey:['motion-jobs',view.document.id],queryFn:async()=> (await api.listJobs()).filter(j=>j.type==='MOTION_STUDY'&&j.documentId===view.document.id),refetchInterval:jobId?1000:5000});
 const job=jobs.data?.find(j=>j.id===jobId);
 const dirty=JSON.stringify(defs)!==JSON.stringify(view.product?.kinematics??{mechanisms:[],studies:[]});
 const editEnabled=canEdit&&!pending&&!showing;
 const close=useCallback(()=>{epoch.current++;alive.current=false;if(currentJob.current)void api.cancelJob(currentJob.current).catch(()=>{});setPlaying(false);playbackCallback.current();onHighlight([]);onClose()},[onClose,onHighlight]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;epoch.current++;if(currentJob.current)void api.cancelJob(currentJob.current).catch(()=>{});playbackCallback.current();highlightCallback.current([])}},[]);
 useEffect(()=>{
  epoch.current++;setPending(false);setError(undefined);setPlaying(false);setShowing(false);setRun(undefined);playbackCallback.current();highlightCallback.current([]);
  if(currentJob.current)void api.cancelJob(currentJob.current).catch(()=>{});currentJob.current=undefined;setJobId(undefined);
  const next=structuredClone(view.product?.kinematics??{mechanisms:[],studies:[]});setDefs(next);
  setMechanismId(id=>next.mechanisms.some(m=>m.id===id)?id:next.mechanisms[0]?.id??'');setStudyId(id=>next.studies.some(s=>s.id===id)?id:next.studies[0]?.id??'');
 },[view.document.id,view.document.versionId]);
 useEffect(()=>{if(job&&done(job)&&currentJob.current===job.id)currentJob.current=undefined},[job]);
 const load=useCallback(async(id:string)=>{
  const generation=++epoch.current;setPending(true);setError(undefined);highlightCallback.current([]);
  try{const result=await api.getMotionRun(id);if(!alive.current||generation!==epoch.current)return;setRun(result);setIndex(0);setPlaying(false);setShowing(result.frames.length>0)}catch(e){if(generation===epoch.current)setError(String(e))}finally{if(generation===epoch.current)setPending(false)}
 },[]);
 const loadedJob=useRef<string|undefined>(undefined);
 useEffect(()=>{if(job?.resultObjectId&&done(job)&&loadedJob.current!==job.id+"/"+job.resultObjectId){loadedJob.current=job.id+"/"+job.resultObjectId;void load(job.id)}},[job,load]);
 useEffect(()=>{if(showing&&run){const p=motionPlayback(run,index);if(p)playbackCallback.current(p);else{setError('冻结结果缺少完整帧／版本身份');setShowing(false)}}else playbackCallback.current()},[run,index,showing]);
 useEffect(()=>{if(!playing||!run||!showing)return;const start=performance.now(),base=run.frames[index]?.timeSeconds??0;const end=run.frames.at(-1)?.timeSeconds??0;const timer=setInterval(()=>{const time=base+(performance.now()-start)/1000;setIndex(motionFrameAt(run,time));if(time>=end)setPlaying(false)},25);return()=>clearInterval(timer)},[playing,run,showing]);
 const changeJoint=(id:string,patch:Partial<MechanismJoint>)=>setDefs(k=>({...k,
  mechanisms:k.mechanisms.map(m=>m.id===mechanismId?{...m,joints:m.joints.map(j=>j.id===id?{...j,...patch,...(patch.kind?{lower:undefined,upper:undefined}:{})}:j)}:m),
  studies:k.studies.map(s=>s.mechanismId===mechanismId&&s.driverJointId===id&&(patch.kind==='REVOLUTE'||patch.kind==='PRISMATIC')
   ?{...s,start:{value:0,unit:patch.kind==='REVOLUTE'?'deg':'mm'},end:{value:90,unit:patch.kind==='REVOLUTE'?'deg':'mm'},replaceDriverConstraintId:undefined}:s)}));
 const changeStudy=(patch:Partial<MotionStudy>)=>setDefs(k=>({...k,studies:k.studies.map(s=>s.id===studyId?{...s,...patch}:s)}));
 const addMechanism=()=>{if(units.length<2){setError('请先插入至少两个运动单元');return}const id=randomUUID(),groundId=randomUUID(),driveId=randomUUID(),sid=randomUUID();
  const m={id,name:'机构 '+(defs.mechanisms.length+1),unitIds:[units[0].id,units[1].id],joints:[{id:groundId,name:'Ground',kind:'GROUND' as const,first:{instanceId:units[0].id,frame:identity()},zero:{value:0,unit:'mm' as const},direction:1},{id:driveId,name:'Revolute',kind:'REVOLUTE' as const,first:{instanceId:units[0].id,frame:identity()},second:{instanceId:units[1].id,frame:identity()},zero:{value:0,unit:'deg' as const},direction:1}]};
  const s:MotionStudy={id:sid,name:'线性研究',mechanismId:id,driverJointId:driveId,start:{value:0,unit:'deg'},end:{value:90,unit:'deg'},durationSeconds:3,frames:31,budgetMs:30000,checkDmu:true,includeSameUnit:false,clearance:{value:0,unit:'mm'}};
  setDefs(k=>({...k,mechanisms:[...k.mechanisms,m],studies:[...k.studies,s]}));setMechanismId(id);setStudyId(sid);
 };
 const addJoint=()=>{if(!selectedMechanism||units.length<2)return;const j:MechanismJoint={id:randomUUID(),name:'Revolute',kind:'REVOLUTE',first:{instanceId:units[0].id,frame:identity()},second:{instanceId:units[1].id,frame:identity()},zero:{value:0,unit:'deg'},direction:1};setDefs(k=>({...k,mechanisms:k.mechanisms.map(m=>m.id===mechanismId?{...m,joints:[...m.joints,j]}:m)}))};
 const save=async()=>{setPending(true);setError(undefined);try{const next={...defs,mechanisms:defs.mechanisms.map(m=>({...m,unitIds:[...new Set(m.joints.flatMap(j=>[j.first.instanceId,...(j.second?[j.second.instanceId]:[])]))]}))};await onSave(next);setDefs(next)}catch(e){setError(String(e))}finally{setPending(false)}};
 const start=async(currentOnly:boolean)=>{const generation=++epoch.current;setPending(true);setError(undefined);setPlaying(false);setShowing(false);highlightCallback.current([]);
  try{const j=await api.startMotionRun(view.document.id,{baseRevisionId:view.document.versionId,requestId:randomUUID(),studyId:study?.id,currentOnly,clearance:{value:gap,unit:'mm'},scope:scopeAddresses,includeSameUnit:sameUnit});if(!alive.current||generation!==epoch.current){void api.cancelJob(j.id).catch(()=>{});return}currentJob.current=j.id;setJobId(j.id);loadedJob.current=undefined;await client.invalidateQueries({queryKey:['motion-jobs',view.document.id]})}catch(e){if(generation===epoch.current)setError(String(e))}finally{if(generation===epoch.current)setPending(false)}
 };
 const highlight=(a:string,b:string)=>{if(!run)return;const selected=run.snapshot.geometryUnits.filter(g=>g.id===a||g.id===b).flatMap(g=>{const r=run.snapshot.view.resolvedInstances?.find(v=>addressKey(v)===addressKey(g.address));if(!r)return [];return [{kind:'body' as const,id:`${r.occurrencePath}:body:${r.bodyId}`,documentId:r.documentId,versionId:r.instancePath.segments.at(-1)?.resolvedVersionId,bodyId:r.bodyId,geometryKey:r.geometryKey,occurrencePath:r.occurrencePath,instancePath:r.instancePath,treeNodeId:r.bodyTreeNodeId,instanceId:g.motionUnitId}]});onHighlight(selected)};
 const frame=run?.frames[index];
 const label=(id:string)=>{const g=run?.snapshot.geometryUnits.find(g=>g.id===id);return g?`${g.address.instancePath.canonical} / Body ${g.address.bodyId}`:id};
 return <Drawer title="机构与基础 DMU" open onClose={close} mask={false} size={700} styles={{body:{padding:16}}}>
  {isMockMode&&<Alert type="warning" title="请使用 API 模式连接实际后端"/>}
  {error&&<Alert closable onClose={()=>setError(undefined)} type="error" title={error}/>}
  {!units.length&&<Button disabled={!editEnabled||isMockMode} loading={pending} onClick={()=>{setPending(true);void onDemo().catch(e=>setError(String(e))).finally(()=>setPending(false))}}>在空 Product 创建四杆闭环演示</Button>}
  <Typography.Paragraph>局部坐标系的 Z 为关节正轴、X 为角度零位方向。运动单元是所属 Product 的直接子实例；子 Product 内部冻结。保留所有已有硬约束，仅允许明确替换同一驱动坐标。</Typography.Paragraph>
  <Space wrap><Select style={{width:210}} value={mechanismId||undefined} placeholder="选择机构" options={defs.mechanisms.map(m=>({label:m.name,value:m.id}))} onChange={id=>{setMechanismId(id);setStudyId(defs.studies.find(s=>s.mechanismId===id)?.id??'')}}/>
   <Button disabled={!editEnabled} onClick={addMechanism}>新建机构</Button><Button disabled={!editEnabled||!selectedMechanism} onClick={addJoint}>添加关节</Button>
   <Button loading={pending} disabled={!editEnabled||!dirty} onClick={()=>void save()}>保存定义</Button>
  </Space>
  {selectedMechanism&&<>
   <Input aria-label="机构名称" disabled={!editEnabled} value={selectedMechanism.name} onChange={e=>setDefs(k=>({...k,mechanisms:k.mechanisms.map(m=>m.id===mechanismId?{...m,name:e.target.value}:m)}))}/>
   <div style={{opacity:editEnabled?1:.65,pointerEvents:editEnabled?'auto':'none'}}>
   {selectedMechanism.joints.map(j=><fieldset key={j.id} style={{margin:'12px 0',padding:8}}><legend>{j.name}</legend>
    <Space wrap><Input aria-label="关节名称" style={{width:140}} value={j.name} onChange={e=>changeJoint(j.id,{name:e.target.value})}/>
     <Select value={j.kind} style={{width:130}} options={['GROUND','RIGID','REVOLUTE','PRISMATIC'].map(value=>({value,label:value}))} onChange={kind=>changeJoint(j.id,{kind,second:kind==='GROUND'?undefined:j.second??{instanceId:units.find(u=>u.id!==j.first.instanceId)?.id??'',frame:identity()},zero:{value:0,unit:kind==='REVOLUTE'?'deg':'mm'}})}/>
     <Button danger size="small" onClick={()=>setDefs(k=>({...k,mechanisms:k.mechanisms.map(m=>m.id===mechanismId?{...m,joints:m.joints.filter(v=>v.id!==j.id)}:m)}))}>删除</Button></Space>
    <Space align="start"><div>第一端<Endpoint value={j.first} units={units} onChange={first=>changeJoint(j.id,{first})} onError={setError}/></div>
     {j.second&&<div>第二端<Endpoint value={j.second} units={units} onChange={second=>changeJoint(j.id,{second})} onError={setError}/></div>}</Space>
    {(j.kind==='REVOLUTE'||j.kind==='PRISMATIC')&&<Space wrap>
     <span>零位 {j.kind==='REVOLUTE'?'deg':'mm'}</span><InputNumber value={quantityIn(j.zero,j.kind==='REVOLUTE'?'deg':'mm')} onChange={value=>changeJoint(j.id,{zero:{value:value??0,unit:j.kind==='REVOLUTE'?'deg':'mm'}})}/>
     <Select value={j.direction} options={[{value:1,label:'正向 +Z'},{value:-1,label:'反向 -Z'}]} onChange={direction=>changeJoint(j.id,{direction})}/>
     <InputNumber placeholder="下限（可空）" value={j.lower?quantityIn(j.lower,j.kind==='REVOLUTE'?'deg':'mm'):undefined} onChange={value=>changeJoint(j.id,{lower:value===null?undefined:{value,unit:j.kind==='REVOLUTE'?'deg':'mm'}})}/>
     <InputNumber placeholder="上限（可空）" value={j.upper?quantityIn(j.upper,j.kind==='REVOLUTE'?'deg':'mm'):undefined} onChange={value=>changeJoint(j.id,{upper:value===null?undefined:{value,unit:j.kind==='REVOLUTE'?'deg':'mm'}})}/>
    </Space>}
   </fieldset>)}
   </div>
   <Select style={{width:220}} value={studyId||undefined} options={defs.studies.filter(s=>s.mechanismId===mechanismId).map(s=>({value:s.id,label:s.name}))} onChange={setStudyId}/>
   <Button disabled={!editEnabled} onClick={()=>{const template=defs.studies.find(s=>s.mechanismId===mechanismId);if(!template)return;const s={...structuredClone(template),id:randomUUID(),name:'新研究'};setDefs(k=>({...k,studies:[...k.studies,s]}));setStudyId(s.id)}}>复制研究</Button>
  </>}
  {study&&<fieldset disabled={!editEnabled} style={{pointerEvents:editEnabled?"auto":"none",margin:'12px 0',padding:8}}><legend>单坐标线性规律</legend>
   <Input aria-label="研究名称" value={study.name} onChange={e=>changeStudy({name:e.target.value})}/>
   <Space wrap><span>驱动关节</span><Select style={{width:180}} value={study.driverJointId} options={selectedMechanism?.joints.filter(j=>j.kind==='REVOLUTE'||j.kind==='PRISMATIC').map(j=>({value:j.id,label:j.name}))} onChange={id=>{const j=selectedMechanism!.joints.find(j=>j.id===id)!;const unit=j.kind==='REVOLUTE'?'deg':'mm';changeStudy({driverJointId:id,start:{value:0,unit},end:{value:90,unit}})}}/>
    <span>起止 {study.start.unit}</span><InputNumber value={study.start.value} onChange={value=>changeStudy({start:{...study.start,value:value??0}})}/><InputNumber value={quantityIn(study.end,study.start.unit)} onChange={value=>changeStudy({end:{unit:study.start.unit,value:value??0}})}/>
    <span>时长 s</span><InputNumber min={.001} max={3600} value={study.durationSeconds} onChange={v=>changeStudy({durationSeconds:v??3})}/>
    <span>离散帧</span><InputNumber min={2} max={500} precision={0} value={study.frames} onChange={v=>changeStudy({frames:v??31})}/>
    <span>预算 ms</span><InputNumber min={100} max={120000} value={study.budgetMs} onChange={v=>changeStudy({budgetMs:v??30000})}/>
    <Select style={{width:260}} allowClear placeholder="明确替换同坐标装配约束（可空）" value={study.replaceDriverConstraintId} options={view.product?.constraints?.filter(c=>!c.suppressed&&c.mode!=='MEASURED').map(c=>({value:c.id,label:c.name??c.id}))} onChange={replaceDriverConstraintId=>changeStudy({replaceDriverConstraintId})}/>
    <Checkbox checked={study.checkDmu} onChange={e=>changeStudy({checkDmu:e.target.checked})}>逐采样帧检查 DMU</Checkbox>
    <Checkbox checked={study.includeSameUnit} onChange={e=>changeStudy({includeSameUnit:e.target.checked})}>包含同刚体内 Body 对</Checkbox>
    <span>间隙 mm</span><InputNumber min={0} value={quantityIn(study.clearance,'mm')} onChange={v=>changeStudy({clearance:{value:v??0,unit:'mm'}})}/>
   </Space>
   <Select mode="multiple" style={{width:'100%'}} placeholder="研究 DMU 范围（空为全部）" value={(study.scope??[]).map(addressKey)} options={geometries.map(g=>({value:addressKey(g),label:`${g.name} / ${g.occurrencePath} / ${g.bodyId}`}))} onChange={values=>changeStudy({scope:geometries.filter(g=>values.includes(addressKey(g))).map(g=>({instancePath:g.instancePath,bodyId:g.bodyId}))})}/>
  </fieldset>}
  <Space wrap><Button type="primary" disabled={isMockMode||dirty||!study||pending||Boolean(job&&!done(job))} onClick={()=>void start(false)}>运行已保存研究</Button>
   {job&&<span>{job.state} {job.progress}%</span>}{job?.canCancel&&<Button onClick={()=>void api.cancelJob(job.id).then(()=>client.invalidateQueries({queryKey:['motion-jobs',view.document.id]}))}>取消运行</Button>}
  </Space>
  <Typography.Title level={5}>当前装配姿态 DMU</Typography.Title>
  <Select mode="multiple" style={{width:'100%'}} placeholder="选择 occurrence + Body 范围（空为全部）" value={scope} options={geometries.map(g=>({value:addressKey(g),label:`${g.name} / ${g.occurrencePath} / ${g.bodyId}`}))} onChange={setScope}/>
  <Space wrap><span>间隙 mm</span><InputNumber min={0} value={gap} onChange={v=>setGap(v??0)}/><Checkbox checked={sameUnit} onChange={e=>setSameUnit(e.target.checked)}>同刚体内 Body 对</Checkbox><Button disabled={isMockMode||pending||showing||Boolean(job&&!done(job))} onClick={()=>void start(true)}>检查当前姿态</Button></Space>
  <Typography.Title level={5}>已保存运行与回放</Typography.Title>
  <Select style={{width:'100%'}} disabled={pending} placeholder="选择冻结运行结果" value={jobId} options={jobs.data?.map(j=>({value:j.id,label:`${String(j.payload.studyName??'研究')} · ${j.state} · ${j.createdAt}`}))} onChange={id=>{if(currentJob.current&&currentJob.current!==id){void api.cancelJob(currentJob.current).catch(()=>{});currentJob.current=undefined}setJobId(id);const j=jobs.data?.find(j=>j.id===id);if(j?.resultObjectId){loadedJob.current=id+"/"+j.resultObjectId;void load(id)}}}/>
  {job?.errorMessage&&<Alert type="error" title={job.errorMessage}/>}
  {run&&<>
   <Alert type={run.completed?'info':'warning'} title={`${run.status} · ${run.frames.length} 帧 · ${run.elapsedMs.toFixed(0)} ms`} description={run.snapshot.revisionId!==view.document.versionId?'旧版本结果：显式回放冻结版本，未覆盖当前模型。':'整帧临时显示；退出后恢复正式装配姿态。'}/>
   {run.failure&&<Alert type="warning" title={run.failure.code} description={`${run.failure.detail}；停止目标 t=${run.failure.timeSeconds.toFixed(3)} s`}/>}
   <Space wrap><Button disabled={!run.frames.length} onClick={()=>{setShowing(true);setPlaying(p=>!p)}}>{playing?'暂停':'播放'}</Button>
    <Button disabled={!run.frames.length} onClick={()=>{setPlaying(false);setShowing(true);setIndex(i=>Math.max(0,i-1))}}>上一步</Button>
    <Button disabled={!run.frames.length} onClick={()=>{setPlaying(false);setShowing(true);setIndex(i=>Math.min(run.frames.length-1,i+1))}}>单步</Button>
    <Button onClick={()=>{setPlaying(false);setShowing(true);setIndex(0)}}>复位研究</Button>
    <Button onClick={()=>{setPlaying(false);setShowing(false);onHighlight([])}}>恢复正式姿态</Button>
   </Space>
   {run.frames.length>0&&<Slider min={0} max={Math.max(0,run.frames.length-1)} value={index} step={1} onChange={i=>{setPlaying(false);setShowing(true);setIndex(i)}}/>}
   <Space>定位时间 s<InputNumber min={0} max={run.frames.at(-1)?.timeSeconds??0} value={frame?.timeSeconds??0} onChange={v=>{setPlaying(false);setShowing(true);setIndex(motionFrameAt(run,v??0))}}/></Space>
   {frame&&<Typography.Paragraph>帧 {index+1} · t={frame.timeSeconds.toFixed(3)} s · 运动学 {frame.kinematicValid?'合格':'未检查'} · DMU {frame.dmuConclusion} · 驱动 {frame.driverValue.toFixed(6)} {['deg','rad'].includes(run.snapshot.study.start.unit)?'rad':'mm'}</Typography.Paragraph>}
   <Space wrap>{motionProblemFrames(run).map(i=><Button key={i} size="small" onClick={()=>{setPlaying(false);setShowing(true);setIndex(i)}}>问题帧 {i+1}</Button>)}</Space>
   <Table size="small" pagination={{pageSize:8}} rowKey={p=>p.firstId+p.secondId} dataSource={frame?.dmu?.pairs??[]} onRow={p=>({onClick:()=>{setShowing(true);highlight(p.firstId,p.secondId)}})} columns={[
    {title:'实例 + Body',dataIndex:'firstId',render:label},{title:'实例 + Body',dataIndex:'secondId',render:label},
    {title:'分类',dataIndex:'classification'}, {title:'距离 mm',dataIndex:'distanceMm',render:(v:number,p)=>p.complete?v.toFixed(6):'未完成'},
    {title:'间隙',render:(_,p)=>!p.complete?'无法判定':p.clearanceSatisfied?'满足':'不满足'}, {title:'诊断',dataIndex:'diagnostic'}]}/>
   <Typography.Text type="secondary">离散帧精确检查，不代表帧间连续无碰撞。点击结果高亮完整实例与 Body。</Typography.Text>
  </>}
 </Drawer>;
}
