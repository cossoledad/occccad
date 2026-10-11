import {forwardRef,useCallback,useEffect,useImperativeHandle,useRef,useState} from 'react';
import {useQuery,useQueryClient} from '@tanstack/react-query';
import {Alert,Button,Checkbox,Input,InputNumber,Select,Slider,Space,Table,Typography} from 'antd';
import {normalizeOperationFailure,useOperationFeedback} from '../../cad/command/operation-feedback';
import type {FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {createActor} from 'xstate';
import {assemblyPreviewMachine} from './assembly-preview-machine';
import {api} from '../../api/client';
import {queryKeys} from '../../app/query-keys';
import type {DocumentView,Job,SelectionItem,AssemblyGeometryRef} from '../../types';
import {memberPoses} from '../../cad/assembly/application-display';
import {randomUUID} from '../../utils/random-uuid';
import {mechanismSupportCandidate,type JointSupportRole} from '../../cad/assembly/mechanism-selection';
import {assemblyGeometryRef} from '../../cad/assembly/assembly-reference';
import {useCommandRegistry} from '../../cad/command/command-context';
import type {CommandOperation} from '../../cad/command/command-registry';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {ToolButton} from '../../cad/overlay/tool-button';
import {CadIcon} from '../../cad/overlay/cad-icons';
import {motionFrameAt,motionPlayback,quantityIn,type JointEndpoint,type KinematicsDefinitions,type MechanismJoint,type MotionPlayback,type MotionRun,type MotionStudy,type MotionApplyRequest,type MotionApplyPlan,type MotionDriver,type MotionLaw} from '../../cad/assembly/motion-study';

const identity=()=>({translation:[0,0,0] as [number,number,number],rotation:[0,0,0,1] as [number,number,number,number]});
const relationLabels:Record<string,string>={ground:'固定',axis:'轴线相合','axial-location':'轴向定位',orientation:'方向一致','angle-lock':'锁定角度',existing:'已有正式约束',solver:'正式装配求解'};
const actionLabels:Record<string,string>={ADD:'新增',REUSE:'复用',CONFLICT:'冲突'};
const done=(job:Job)=>['SUCCEEDED','FAILED','CANCELED'].includes(job.state);
const empty=():KinematicsDefinitions=>({mechanisms:[],studies:[],drivers:[],laws:[],associations:[]});
export type MotionPanelHandle={execute:(action:string,objectId?:string,operation?:CommandOperation)=>void|Promise<void>;closeCommand:()=>void;enabled:(action:string)=>boolean;adoptDefinition:(view:DocumentView)=>void;adoptProduct:(view:DocumentView)=>void};
type PreviewPose=import("../../cad/assembly/motion-study").MotionPose&{instanceId:string};
type Props={onMechanism:(m?:import("../../cad/assembly/motion-study").Mechanism)=>void;onStatus:(text:string)=>void;onSelectionSession:(session?:FeatureSelectionSession)=>void;onPreview:(poses?:PreviewPose[])=>void;view:DocumentView;active:boolean;selection:SelectionItem|null;canEdit:boolean;onSave:(k:KinematicsDefinitions)=>Promise<DocumentView>;onSwitch:(req:import("../../cad/assembly/motion-study").MechanismApplicationRequest)=>Promise<import("../../cad/assembly/motion-study").MechanismApplication&{view:DocumentView}>;onApply:(req:MotionApplyRequest)=>Promise<void>;onPlayback:(p?:MotionPlayback)=>void;onHighlight:(s:SelectionItem[])=>void;onClose:()=>void;onCommandState:(active:boolean)=>void};
export const MotionStudyPanel=forwardRef<MotionPanelHandle,Props>(function MotionStudyPanel({view,active,selection,canEdit,onSave,onSwitch,onApply,onPlayback,onHighlight,onClose,onCommandState,onStatus,onSelectionSession,onPreview,onMechanism},ref){
 const client=useQueryClient(),registry=useCommandRegistry(),feedback=useOperationFeedback();
 const [previewActor]=useState(()=>createActor(assemblyPreviewMachine));
 const [previewReady,setPreviewReady]=useState(false),[previewPending,setPreviewPending]=useState(false),[previewSuspended,setPreviewSuspended]=useState(false);
 const savingDefinition=useRef(false),ownedRevision=useRef<string|undefined>(undefined);
 const pickedSupports=useRef(new Map<JointSupportRole,SelectionItem>());
 const previewSequence=useRef(0),previewAbort=useRef<AbortController|undefined>(undefined),qualifiedJoint=useRef<MechanismJoint|undefined>(undefined),qualifiedInput=useRef<string|undefined>(undefined);
 const commandBaseline=useRef<import("../../cad/assembly/motion-study").Mechanism|undefined>(undefined);
 const supports=useRef(new Map<string,Awaited<ReturnType<typeof api.inspectAssemblySupports>>["supports"][number]|null>()),supportPending=useRef(new Map<string,Promise<Awaited<ReturnType<typeof api.inspectAssemblySupports>>["supports"][number]|null>>());
 const pickSessionEpoch=useRef(0);
 useEffect(()=>{previewActor.start();return()=>{previewActor.stop()}},[previewActor]);

 const [defs,setDefs]=useState<KinematicsDefinitions>(()=>structuredClone(view.product?.kinematics??empty()));
 const activeMechanismId=useRef<string|undefined>(undefined),acceptedMechanism=useRef<import("../../cad/assembly/motion-study").Mechanism|undefined>(undefined);
 const [mechanismId,setMechanismId]=useState(defs.mechanisms[0]?.id??''),[studyId,setStudyId]=useState(defs.studies[0]?.id??'');
 const [applicationQualified,setApplicationQualified]=useState(true);
 const [commandPending,setCommandPending]=useState(false),[pending,setPending]=useState(false),[error,setError]=useState<unknown>(),[jobId,setJobId]=useState<string>();
 const [run,setRun]=useState<MotionRun>(),[index,setIndex]=useState(0),[playing,setPlaying]=useState(false),[showing,setShowing]=useState(false);
 const [action,setAction]=useState<string>(),[objectId,setObjectId]=useState<string>(),[name,setName]=useState('');
 const [joint,setJoint]=useState<MechanismJoint>(),[pickRole,setPickRole]=useState<JointSupportRole>();
 const [angleCommand,setAngleCommand]=useState(false),[lawDraft,setLawDraft]=useState<MotionLaw>();
 const [driver,setDriver]=useState<MotionDriver>(),[studyDraft,setStudyDraft]=useState<MotionStudy>();
 const [plan,setPlan]=useState<MotionApplyPlan>(),[lockAngle,setLockAngle]=useState(false),[applyOpen,setApplyOpen]=useState(false);
 const foreground=useRef<string|undefined>(undefined),loaded=useRef<string|undefined>(undefined);
 const commandEpoch=useRef(0),commandOwner=useRef<CommandOperation|undefined>(undefined),applying=useRef(false);
 const revisionRef=useRef(view.document.versionId);revisionRef.current=view.document.versionId;
 const epoch=useRef(0),alive=useRef(true),activeRef=useRef(active);activeRef.current=active;
 const callbacks=useRef({onPlayback,onHighlight,onStatus,onSelectionSession,onPreview,onMechanism});callbacks.current={onPlayback,onHighlight,onStatus,onSelectionSession,onPreview,onMechanism};
 const selectedMechanism=defs.mechanisms.find(m=>m.id===mechanismId),study=defs.studies.find(s=>s.id===studyId&&s.mechanismId===mechanismId);
 useEffect(()=>{if(savingDefinition.current)return;callbacks.current.onMechanism(active&&activeMechanismId.current?(showing&&run?run.snapshot.mechanism:selectedMechanism):undefined)},[active,showing,run,selectedMechanism]);
 const units=view.product?.instances??[],geometries=view.resolvedInstances??[];
 const editEnabled=canEdit&&applicationQualified&&!pending&&!commandPending&&!showing;
 const jobs=useQuery({queryKey:['motion-jobs',view.document.id],queryFn:async()=> (await api.listJobs()).filter(j=>j.type==='MOTION_STUDY'&&j.documentId===view.document.id),refetchInterval:active&&jobId?1000:5000});
 const job=jobs.data?.find(j=>j.id===jobId);
 const closeCommand=(finishPlayback=false,forceEditing=false)=>{
  const editing=acceptedMechanism.current??selectedMechanism;
  const finishing=finishPlayback&&activeRef.current&&action==='results'&&showing;
  if(finishing){
   setPlaying(false);
   if(canEdit&&run?.snapshot.revisionId===revisionRef.current&&run.frames[index]?.kinematicValid&&editing?.id===run.snapshot.mechanism.id){
    const frameMechanism={...editing,poses:memberPoses(editing,run.frames[index].unitPoses)};
    const next={...defs,mechanisms:defs.mechanisms.map(v=>v.id===editing!.id?frameMechanism:v)};
    setCommandPending(true);savingDefinition.current=true;
    void onSave(next).then(updated=>{adopt(updated,frameMechanism.id);setShowing(false);setPreviewSuspended(false);callbacks.current.onPlayback();closeCommand(false,true)}).catch(e=>{setError(e);setShowing(true)}).finally(()=>{savingDefinition.current=false;setCommandPending(false)});
    return;
   }
   setShowing(false);setPreviewSuspended(false);callbacks.current.onMechanism(editing);callbacks.current.onPlayback();
  }
  pickedSupports.current.clear();qualifiedInput.current=undefined;commandEpoch.current++;commandOwner.current=undefined;
  setCommandPending(false);setAction(undefined);setPickRole(undefined);setError(undefined);previewAbort.current?.abort();previewSequence.current++;setPreviewPending(false);
  callbacks.current.onSelectionSession();callbacks.current.onPreview();
  callbacks.current.onMechanism(activeRef.current?(showing&&!finishing&&!forceEditing&&run?run.snapshot.mechanism:editing):undefined);
  setApplyOpen(false);setPlan(undefined);onCommandState(false);
 };
 const release=(exit=false)=>{
  foreground.current=undefined;epoch.current++;setPending(false);setPlaying(false);setShowing(false);previewAbort.current?.abort();previewSequence.current++;
  callbacks.current.onMechanism(exit?undefined:acceptedMechanism.current);callbacks.current.onPlayback();callbacks.current.onPreview();
  callbacks.current.onSelectionSession();callbacks.current.onStatus("");callbacks.current.onHighlight([]);
 };
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;epoch.current++;previewAbort.current?.abort();callbacks.current.onSelectionSession();callbacks.current.onPreview();callbacks.current.onStatus("");callbacks.current.onPlayback();callbacks.current.onHighlight([])}},[]);
 useEffect(()=>{if(!active){foreground.current=undefined;release(true);closeCommand(false,true)}},[active]);
 useEffect(()=>{
  foreground.current=undefined;
  if(!savingDefinition.current&&ownedRevision.current!==view.document.versionId)release();
  // An accepted apply refreshes Head before its command promise finishes. Keep
  // that owned invocation until it can return to assembly; explicit cancel still
  // disposes it. External revisions always release frozen viewport state.
  if(!savingDefinition.current&&ownedRevision.current!==view.document.versionId&&(!applying.current||!commandOwner.current?.current))closeCommand(false,true);
  setError(undefined);setPending(false);supports.current.clear();supportPending.current.clear();
  const next=structuredClone(view.product?.kinematics??empty());setDefs(next);
  const current=next.mechanisms.find(m=>m.id===activeMechanismId.current);
  if(active&&current&&!savingDefinition.current&&ownedRevision.current!==view.document.versionId){
   const revision=view.document.versionId,sequence=++previewSequence.current;
   savingDefinition.current=true;setCommandPending(true);setApplicationQualified(false);
   void onSwitch({baseRevisionId:revision,mechanismId:current.id,sourceMechanismIds:[],resume:true}).then(result=>{
    if(!alive.current||sequence!==previewSequence.current)return;
    adopt(result.view,current.id);setApplicationQualified(true);acceptedMechanism.current=result.mechanism;callbacks.current.onMechanism(result.mechanism);
   }).catch(e=>{if(sequence===previewSequence.current)setError(e)}).finally(()=>{if(sequence===previewSequence.current){savingDefinition.current=false;setCommandPending(false)}});
  }
  if(activeMechanismId.current&&!next.mechanisms.some(m=>m.id===activeMechanismId.current)){activeMechanismId.current=undefined;acceptedMechanism.current=undefined;release(true);closeCommand(false,true);setApplicationQualified(false);setError('当前机械装置已被删除，已返回最新装配设计');onClose()}
  setMechanismId(id=>next.mechanisms.some(m=>m.id===id)?id:next.mechanisms[0]?.id??'');setStudyId(id=>next.studies.some(s=>s.id===id)?id:next.studies[0]?.id??'');
  // Jobs survive panel/Revision changes. A result enters the viewport only after
  // an explicit load or the exact foreground run's completion in this session.
 },[view.document.id,view.document.versionId]);
 const adopt=(updated:DocumentView,id=activeMechanismId.current)=>{
  ownedRevision.current=updated.document.versionId;const next=structuredClone(updated.product?.kinematics??empty());setDefs(next);
  const m=next.mechanisms.find(m=>m.id===id);acceptedMechanism.current=m;
  callbacks.current.onMechanism(m);callbacks.current.onPreview();
 };
 const save=async(next:KinematicsDefinitions,activateId?:string)=>{const generation=commandEpoch.current;setCommandPending(true);setError(undefined);try{savingDefinition.current=true;const updated=await onSave(next);if(activateId){activeMechanismId.current=activateId;setMechanismId(activateId)}adopt(updated);if(generation===commandEpoch.current)closeCommand()}catch(e){if(generation===commandEpoch.current)setError(e)}finally{savingDefinition.current=false;if(generation===commandEpoch.current)setCommandPending(false)}};
 const enter=async(id?:string,resume=false)=>{
  if(commandPending||pending||action&&action!=='results')return;
  const existing=defs.mechanisms.find(m=>m.id===(id??defs.mechanisms[0]?.id));
  if(!existing){setMechanismId('');return}
  const sequence=++previewSequence.current;setCommandPending(true);savingDefinition.current=true;setError(undefined);
  try{
   const result=await onSwitch({baseRevisionId:revisionRef.current,mechanismId:existing.id,sourceMechanismId:activeMechanismId.current,resume});
   if(!alive.current||sequence!==previewSequence.current)return;
   activeMechanismId.current=existing.id;setMechanismId(existing.id);setStudyId(result.view.product?.kinematics?.studies.find(s=>s.mechanismId===existing.id)?.id??'');
   adopt(result.view,existing.id);acceptedMechanism.current=result.mechanism;callbacks.current.onMechanism(result.mechanism);callbacks.current.onPlayback();callbacks.current.onPreview();setPreviewSuspended(false);setApplicationQualified(true);
  }catch(e){setApplicationQualified(false);setError(e)}finally{savingDefinition.current=false;setCommandPending(false)}
 };
 const exit=async()=>{
  if(commandPending||action&&action!=='results')return;
  setCommandPending(true);savingDefinition.current=true;setError(undefined);foreground.current=undefined;epoch.current++;
  try{
   let base=revisionRef.current;
   if(canEdit&&showing&&run?.frames[index]?.kinematicValid){
    if(run.snapshot.revisionId!==base)throw new Error('历史回放请先关闭，或通过转换计划验证后采用');
    const m=acceptedMechanism.current;if(m&&m.id===run.snapshot.mechanism.id){
     const next={...defs,mechanisms:defs.mechanisms.map(v=>v.id===m.id?{...m,poses:memberPoses(m,run.frames[index].unitPoses)}:v)};
     const updated=await onSave(next);adopt(updated,m.id);base=updated.document.versionId;
    }
   }
   await onSwitch({baseRevisionId:base,sourceMechanismId:canEdit?activeMechanismId.current:undefined});activeMechanismId.current=undefined;acceptedMechanism.current=undefined;release(true);closeCommand(false,true);onClose()
  }catch(e){setError(e)}finally{savingDefinition.current=false;setCommandPending(false)}
 };
 const execute=(nextAction:string,id?:string,operation?:CommandOperation):void|Promise<void>=>{
  if(nextAction==='enter'||nextAction==='resume')return enter(id,nextAction==='resume');
  if(nextAction==='exit')return exit();
  if(nextAction==='cancel'&&job)return api.cancelJob(job.id).then(()=>client.invalidateQueries({queryKey:['motion-jobs',view.document.id]}));
  if(['play','pause','previous','next','reset'].includes(nextAction)){
   if(!run?.frames.length)return;setShowing(true);setPlaying(nextAction==='play');
   if(nextAction==='previous')setIndex(i=>Math.max(0,i-1));
   if(nextAction==='next')setIndex(i=>Math.min(run.frames.length-1,i+1));
   if(nextAction==='reset')setIndex(0);return;
  }
  if((nextAction==='edit'||nextAction==='edit-relation')&&id){const owner=defs.mechanisms.find(m=>m.id===id||m.joints.some(j=>j.id===id||j.constraints?.some(c=>c.id===id))||defs.drivers?.some(d=>d.id===id&&d.mechanismId===m.id)||defs.laws?.some(l=>l.id===id&&l.mechanismId===m.id)||defs.studies.some(s=>s.id===id&&s.mechanismId===m.id));if(owner&&owner.id!==activeMechanismId.current)return enter(owner.id).then(()=>{if(activeMechanismId.current===owner.id)execute(nextAction,id,operation)})}
  if(!['result','results','apply','run'].includes(nextAction))setPreviewSuspended(false);
  commandBaseline.current=structuredClone(acceptedMechanism.current??selectedMechanism);
  commandEpoch.current++;commandOwner.current=operation;setCommandPending(false);setError(undefined);setPlan(undefined);setPickRole(undefined);setObjectId(id);
  if(nextAction==='result'&&id){foreground.current=undefined;setJobId(id);setAction('results');onCommandState(true);const j=jobs.data?.find(j=>j.id===id);if(j?.resultObjectId)void load(id);return}
  let relationRole:string|undefined;
  if(nextAction==='edit-relation'){
   const owner=defs.mechanisms.flatMap(m=>m.joints).find(j=>j.constraints?.some(c=>c.id===id));
   const c=owner?.constraints?.find(c=>c.id===id);
   if(!owner||!c)return;relationRole=c.kind==='DISTANCE'?'axial-location':c.kind==='FIX'?'ground':'axis';id=owner.id;setObjectId(id);nextAction='edit';
  }
  if(nextAction==='edit'){
   const m=defs.mechanisms.find(m=>m.id===id),owner=defs.mechanisms.find(m=>m.joints.some(j=>j.id===id));
   if(m){setMechanismId(m.id);setName(m.name);nextAction='mechanism'}
   else if(owner){setMechanismId(owner.id);setJoint(structuredClone(owner.joints.find(j=>j.id===id)!));nextAction='joint';if(relationRole)setPickRole(relationRole==='axial-location'?'first.plane':relationRole==='axis'?'first.axis':undefined)}
   else if(defs.drivers?.some(d=>d.id===id)){const d=defs.drivers.find(d=>d.id===id)!;setMechanismId(d.mechanismId);setDriver(structuredClone(d));nextAction='driver'}
   else if(defs.laws?.some(l=>l.id===id)){const l=defs.laws.find(l=>l.id===id)!;setMechanismId(l.mechanismId);setLawDraft(structuredClone(l));nextAction='law'}
   else if(defs.studies.some(s=>s.id===id)){const s=defs.studies.find(s=>s.id===id)!;setMechanismId(s.mechanismId);setStudyDraft(structuredClone(s));setStudyId(id!);nextAction='study'}
  }
  if(['ground','revolute'].includes(nextAction)){
   if(!selectedMechanism){setError('请先创建或选择机构');return}
   setAngleCommand(false);
   const kind=nextAction==='ground'?'GROUND':nextAction.toUpperCase() as MechanismJoint['kind'];const selected=selection?.instanceId??units[0]?.id??'';
   const first:JointEndpoint={instanceId:selected,frame:identity()};setJoint({id:randomUUID(),name:(nextAction==='ground'?'固定零件':'旋转')+'.'+(selectedMechanism.joints.filter(j=>j.kind===kind).length+1),kind,first,second:kind==='GROUND'?undefined:{instanceId:units.find(v=>v.id!==selected)?.id??'',frame:identity()},zero:{value:0,unit:kind==='REVOLUTE'?'deg':'mm'},axialOffset:{value:0,unit:'mm'},direction:1});nextAction='joint';setPickRole(kind==='GROUND'?undefined:'first.axis');setObjectId(undefined)
  }
  if(nextAction==='driver'&&!id){const j=selectedMechanism?.joints.find(j=>j.kind==='REVOLUTE'&&!defs.drivers?.some(d=>d.mechanismId===mechanismId&&d.jointId===j.id));if(!j){setError('请先创建可驱动的接合');return}setDriver({id:randomUUID(),name:'命令.'+((defs.drivers??[]).filter(d=>d.mechanismId===mechanismId).length+1),mechanismId:mechanismId,jointId:j.id})}
  if(nextAction==='law'&&!id){const d=defs.drivers?.find(d=>d.mechanismId===mechanismId);if(!d){setError('请先保存角度命令');return}setLawDraft({id:randomUUID(),name:'规律.'+((defs.laws??[]).filter(l=>l.mechanismId===mechanismId).length+1),mechanismId,driverId:d.id,start:{value:0,unit:'deg'},end:{value:360,unit:'deg'},durationSeconds:5})}
  if(nextAction==='study'&&!id){const l=defs.laws?.find(l=>l.mechanismId===mechanismId);if(!l){setError('请先保存运动规律');return}setStudyDraft({id:randomUUID(),name:'模拟.'+(defs.studies.length+1),mechanismId,lawId:l.id,driverId:l.driverId,start:l.start,end:l.end,durationSeconds:l.durationSeconds,frames:73,budgetMs:60000})}
  if(nextAction==='mechanism'&&!id)setName('机械装置.'+(defs.mechanisms.length+1));
  if(nextAction==='apply'){if(!selectedMechanism||showing&&(!run||!jobId||!run.frames[index]?.kinematicValid)){setError('选择机械装置已确认位姿或合格仿真帧');return}setPlaying(false);setApplyOpen(true);setLockAngle(false)}
  setAction(nextAction);onCommandState(true)
 };
 const enabled=(command:string)=>{
  if(!active)return false;
  if(command==='exit')return !commandPending&&(!action||action==='results');
  if(command==='results')return !commandPending;
  if(pending||commandPending)return false;
  if(command==='play')return Boolean(run?.frames.length&&!playing);
  if(command==='pause')return playing;
  if(command==='previous'||command==='next'||command==='reset')return Boolean(run?.frames.length);
  if(command==='cancel')return Boolean(job?.canCancel);
  if(command==='apply')return Boolean(applicationQualified&&canEdit&&selectedMechanism&&(!showing||run&&frame?.kinematicValid));
  if(command==='run')return Boolean(applicationQualified&&!showing&&defs.studies.some(s=>s.mechanismId===mechanismId)&&(!job||done(job)));
  if(!editEnabled)return false;
  if(command==='mechanism')return true;
  if(!selectedMechanism||!activeMechanismId.current)return false;
  if(['ground','revolute'].includes(command))return units.length>0;
  if(command==='driver')return selectedMechanism.joints.some(j=>j.kind==='REVOLUTE'&&!defs.drivers?.some(d=>d.mechanismId===mechanismId&&d.jointId===j.id));
  if(command==='law')return Boolean(defs.drivers?.some(d=>d.mechanismId===mechanismId));
  if(command==='study')return Boolean(defs.laws?.some(l=>l.mechanismId===mechanismId));
  return true;
 };
 useImperativeHandle(ref,()=>({execute,closeCommand,enabled,adoptDefinition:updated=>adopt(updated),adoptProduct:updated=>{activeMechanismId.current=undefined;acceptedMechanism.current=undefined;release(true);closeCommand(false,true);adopt(updated)}}));
 useEffect(()=>onCommandState(Boolean(active&&(action||commandPending||pending))),[active,action,commandPending,pending,onCommandState]);
 useEffect(()=>{registry.notifyStateChanged()},[registry,active,pending,commandPending,canEdit,applicationQualified,showing,playing,run,index,study,selectedMechanism,job,defs]);
 const commandButton=(id:string)=>{const command=registry.declaration('dmu.'+id);return command?<ToolButton key={id} command={command.id} showLabel icon={<CadIcon name={command.iconKey}/>} tooltip={command.name} helpText={command.helpText} toolbarName="机构与 DMU"/>:null};
 // Exact support cache is scoped to this Product Revision. It gates both
 // ray hits and tree candidates through the existing FeatureSelectionSession.
 const supportKey=(item:SelectionItem,axis:boolean)=>JSON.stringify([axis,assemblyGeometryRef(item)]);
 const inspect=async(item:SelectionItem,axis:boolean)=>{
  const raw=assemblyGeometryRef(item);if(!raw)return null;
  let result=await api.inspectAssemblySupports(view.document.id,[raw],commandOwner.current?.signal);let support=result.supports[0];
  const derived=axis?({CYLINDER:'cylinder-axis',CIRCLE:'circle-axis',CONE:'cone-axis'} as Record<string,string>)[support?.descriptor?.Kind??'']:undefined;
  if(derived){result=await api.inspectAssemblySupports(view.document.id,[{...support.reference,derivedRole:derived}],commandOwner.current?.signal);support=result.supports[0]}
  return support?.descriptor?.Kind===(axis?'AXIS':'PLANE')?support:null;
 };
 const pick=async(role:NonNullable<typeof pickRole>,item:SelectionItem)=>{
  const axis=role.endsWith('axis'),generation=commandEpoch.current;
  try{const support=supports.current.get(supportKey(item,axis))??await inspect(item,axis);if(generation!==commandEpoch.current||!activeRef.current||!support)return;
   const [side,field]=role.split('.') as ['first'|'second','axis'|'plane'];
   pickedSupports.current.set(role,item);if(field==='axis'&&joint?.[side]?.instanceId!==support.reference.instanceId)pickedSupports.current.delete(`${side}.plane`);
   setJoint(j=>{if(!j)return j;const end=j[side];if(!end)return j;return {...j,first:{...j.first,capturedX:undefined},second:j.second?{...j.second,capturedX:undefined}:undefined,[side]:{...end,instanceId:support.reference.instanceId,[field]:support.reference,...(field==='axis'&&support.reference.instanceId!==end.instanceId?{plane:undefined}:{}),capturedX:undefined}}});
   const roles=['first.axis','second.axis','first.plane','second.plane'] as const;
   setPickRole(roles[roles.indexOf(role)+1]);setError(undefined)
  }catch(e){if(generation===commandEpoch.current){setError(e)}}
 };
 useEffect(()=>{
  if(!active||action!=='joint'||!joint){callbacks.current.onSelectionSession();return}
  if(!pickRole){callbacks.current.onSelectionSession({role:'geometry',documentId:view.document.id,versionId:view.document.versionId,selections:[...pickedSupports.current.values()],accept:()=>false,onPick:()=>{}});return()=>callbacks.current.onSelectionSession()}
  const axis=pickRole.endsWith('axis'),revision=view.document.versionId,generation=commandEpoch.current,sessionEpoch=++pickSessionEpoch.current;
  const candidate=(item:SelectionItem)=>mechanismSupportCandidate(view,joint,pickRole,item)&&supports.current.get(supportKey(item,axis))!==null;
  const qualify=(item:SelectionItem)=>{
   const key=supportKey(item,axis);if(supports.current.has(key))return Promise.resolve(supports.current.get(key)??null);
   const existing=supportPending.current.get(key);if(existing)return existing;
   const promise=inspect(item,axis).then(support=>{if(generation===commandEpoch.current&&revisionRef.current===revision&&activeRef.current)supports.current.set(key,support);return support}).catch(e=>{if(generation===commandEpoch.current){supports.current.delete(key);feedback(e,'几何支持')}return null}).finally(()=>{if(supportPending.current.get(key)===promise)supportPending.current.delete(key)});
   supportPending.current.set(key,promise);return promise;
  };
  const session:FeatureSelectionSession={role:axis?'axis':'plane',documentId:view.document.id,versionId:revision,selections:[...pickedSupports.current.values()],
   accept:item=>{
    if(!candidate(item))return false;
    const key=supportKey(item,axis);if(supports.current.has(key))return Boolean(supports.current.get(key));
    void qualify(item);return false;
   },pendingCandidate:candidate,resolvePick:item=>{void qualify(item).then(support=>{if(support&&sessionEpoch===pickSessionEpoch.current&&generation===commandEpoch.current&&activeRef.current&&revisionRef.current===revision)void pick(pickRole,item)})},onPick:item=>{void pick(pickRole,item)}};
  callbacks.current.onSelectionSession(session);
  return()=>{pickSessionEpoch.current++;callbacks.current.onSelectionSession()};
 },[active,action,pickRole,joint,view.document.versionId]);
 useEffect(()=>{
  const m=selectedMechanism;if(!active||showing||previewSuspended||!m||!activeMechanismId.current||savingDefinition.current||commandPending){setPreviewPending(false);return}
  const draft=action==='joint'?joint:undefined;
  if(draft&&draft.kind!=='GROUND'&&(!draft.first.axis||!draft.second?.axis)){setPreviewReady(false);return}
  const joints=draft?[...m.joints.filter(j=>j.id!==draft.id),draft]:m.joints;
  if(!joints.length){setPreviewReady(false);return}
  if(!draft&&m.poses){callbacks.current.onPreview();setPreviewReady(true);return}
  const generation=++previewSequence.current,abort=new AbortController();previewAbort.current?.abort();previewAbort.current=abort;
  setPreviewReady(false);setPreviewPending(true);setError(undefined);qualifiedJoint.current=undefined;previewActor.send({type:'REQUEST',sequence:generation});
  const timer=setTimeout(()=>{void api.previewMechanism(view.document.id,{requestId:randomUUID(),baseRevisionId:view.document.versionId,mechanism:{...m,poses:commandBaseline.current?.poses??m.poses,joints},draftJointId:draft?.id},abort.signal).then(result=>{
   if(abort.signal.aborted||generation!==previewSequence.current||!activeRef.current)return;
   previewActor.send({type:'RESOLVE',sequence:generation});setPreviewReady(true);setPreviewPending(false);qualifiedInput.current=JSON.stringify(draft);qualifiedJoint.current=result.mechanism.joints.find(j=>j.id===draft?.id);callbacks.current.onPreview(result.instancePoses);callbacks.current.onMechanism(result.mechanism);
  }).catch(e=>{if(abort.signal.aborted||generation!==previewSequence.current)return;previewActor.send({type:'REJECT',sequence:generation,error:String(e)});setPreviewPending(false);setError(e);})},100);
  return()=>{clearTimeout(timer);abort.abort()};
 },[active,showing,previewSuspended,action,joint,selectedMechanism,view.document.versionId,commandPending]);
 useEffect(()=>{if(error)feedback(error,'机构命令')},[error,feedback]);
 useEffect(()=>{if(active&&job?.errorMessage)feedback(new Error(job.errorMessage),'机构运行')},[active,job?.id,job?.errorMessage,feedback]);
 useEffect(()=>{callbacks.current.onStatus(!active?'':pickRole?`选择${pickRole.startsWith('first')?'第一':'第二'}${pickRole.endsWith('axis')?'轴线':'定位平面'}`:previewPending?'正在预览接合…':showing?`机构回放 · 帧 ${index+1}${run?.snapshot.revisionId!==view.document.versionId?' · 冻结旧版本':''}`:job&&!done(job)?`机构运行 · ${job.state} ${job.progress}%`:selectedMechanism?`机构编辑 · ${selectedMechanism.name}`:'机构编辑')},[active,pickRole,previewPending,showing,index,run,job,selectedMechanism]);
 const load=useCallback(async(id:string)=>{
  const generation=++epoch.current;setPending(true);setError(undefined);setPlaying(false);setShowing(false);setRun(undefined);setIndex(0);setPreviewSuspended(true);callbacks.current.onHighlight([]);
  try{const result=await api.getMotionRun(id);if(!alive.current||generation!==epoch.current||!activeRef.current)return;setRun(result);setIndex(0);setPlaying(false);setShowing(result.frames.length>0)}catch(e){if(generation===epoch.current)setError(e)}finally{if(generation===epoch.current)setPending(false)}
 },[]);
 useEffect(()=>{if(active&&job?.resultObjectId&&done(job)&&foreground.current===job.id&&loaded.current!==job.resultObjectId){loaded.current=job.resultObjectId;foreground.current=undefined;void load(job.id)}},[active,job,load]);
 useEffect(()=>{if(active&&showing&&run){const p=motionPlayback(run,index);if(p)callbacks.current.onPlayback(p);else{setError('冻结结果缺少完整帧／版本身份');setShowing(false)}}else callbacks.current.onPlayback()},[active,run,index,showing]);
 useEffect(()=>{if(!playing||!run||!showing||!active)return;const start=performance.now(),base=run.frames[index]?.timeSeconds??0,end=run.frames.at(-1)?.timeSeconds??0;const timer=setInterval(()=>{const time=base+(performance.now()-start)/1000;setIndex(motionFrameAt(run,time));if(time>=end)setPlaying(false)},25);return()=>clearInterval(timer)},[playing,run,showing,active]);
 const start=async()=>{
  if(!study)return;
  const generation=++epoch.current,ownerEpoch=commandEpoch.current;setPending(true);setError(undefined);setPlaying(false);setShowing(false);setRun(undefined);setIndex(0);setJobId(undefined);setPreviewSuspended(true);
  try{const j=await api.startMotionRun(view.document.id,{baseRevisionId:view.document.versionId,requestId:randomUUID(),studyId:study.id});if(!alive.current||generation!==epoch.current||ownerEpoch!==commandEpoch.current||!activeRef.current)return;setJobId(j.id);if(activeRef.current)foreground.current=j.id;loaded.current=undefined;await client.invalidateQueries({queryKey:['motion-jobs',view.document.id]});if(!alive.current||!activeRef.current||ownerEpoch!==commandEpoch.current)return;closeCommand();setAction('results');onCommandState(true)}catch(e){if(generation===epoch.current)setError(e)}finally{if(generation===epoch.current)setPending(false)}
 };
 const deleteResult=async()=>{
  if(!job||!done(job))return;
  const id=job.id,generation=epoch.current;setPending(true);
  try{await api.deleteMotionResult(id);if(generation===epoch.current&&activeRef.current&&jobId===id){loaded.current=undefined;setJobId(undefined);setRun(undefined);setIndex(0);setPreviewSuspended(true);release()}await Promise.all([client.invalidateQueries({queryKey:['motion-jobs',view.document.id]}),client.invalidateQueries({queryKey:queryKeys.jobs})])}catch(e){feedback(e,'机构运行')}finally{if(generation===epoch.current)setPending(false)}
 };
 const applyInput=():MotionApplyRequest=>showing&&run?{jobId:jobId!,frameIndex:index,lockAngle}:{mechanismId,frameIndex:0,lockAngle};
 const review=async()=>{const generation=++commandEpoch.current;setCommandPending(true);setError(undefined);try{const result=await api.planMotionApply(view.document.id,view.document.versionId,applyInput());if(alive.current&&activeRef.current&&generation===commandEpoch.current)setPlan(result)}catch(e){if(generation===commandEpoch.current)setError(e)}finally{if(generation===commandEpoch.current)setCommandPending(false)}};
 const apply=async()=>{
  if(!plan?.ready)return;setCommandPending(true);applying.current=true;
  const owner=commandOwner.current,generation=commandEpoch.current,revision=view.document.versionId;
  try{await onApply({...applyInput(),planDigest:plan.digest});if(alive.current&&activeRef.current&&(!owner||owner.current)){activeMechanismId.current=undefined;acceptedMechanism.current=undefined;release(true);closeCommand(false,true);onClose()}}
  catch(e){if(alive.current&&activeRef.current&&(!owner||owner.current)){if(revisionRef.current!==revision)closeCommand();setError(e);setPlan(undefined)}}
  finally{applying.current=false;if(generation===commandEpoch.current)setCommandPending(false)}
 };
 const frame=run?.frames[index];
 useEffect(()=>{commandEpoch.current++;setPlan(undefined)},[index,jobId]);
 const changeJoint=(patch:Partial<MechanismJoint>)=>setJoint(j=>j?{...j,...patch}:j);
 const saveJoint=async()=>{if(!joint||!selectedMechanism)return;const m={...selectedMechanism,poses:commandBaseline.current?.poses??selectedMechanism.poses,joints:[...selectedMechanism.joints.filter(j=>j.id!==joint.id),qualifiedJoint.current??joint]};m.unitIds=[...new Set([...m.unitIds,...m.joints.flatMap(j=>[j.first.instanceId,...(j.second?[j.second.instanceId]:[])])])];const drivers=angleCommand&&!defs.drivers?.some(d=>d.jointId===joint.id)?[...(defs.drivers??[]),{id:randomUUID(),name:'命令.'+((defs.drivers??[]).filter(d=>d.mechanismId===m.id).length+1),mechanismId:m.id,jointId:joint.id}]:defs.drivers;await save({...defs,drivers,mechanisms:[...defs.mechanisms.filter(v=>v.id!==m.id),m]})};
 const definitionValid=editEnabled&&(
  action==='mechanism'?Boolean(name.trim()):
  action==='joint'?Boolean(previewReady&&!previewPending&&qualifiedInput.current===JSON.stringify(joint)&&joint?.name&&joint.first.instanceId&&(joint.kind==='GROUND'||joint.second?.instanceId&&joint.first.axis&&joint.first.plane&&joint.second.axis&&joint.second.plane)):
  action==='driver'?Boolean(driver?.name&&driver.jointId):action==='law'?Boolean(lawDraft?.name&&lawDraft.driverId&&lawDraft.durationSeconds>0):action==='study'?Boolean(studyDraft?.name&&studyDraft.lawId):
  action==='run'?Boolean(study):action==='results'?true:false);
 const confirmDefinition=async()=>{
  if(action==='results'){closeCommand(true);return}
  if(!definitionValid)return;
  if(action==='run'){await start();return}
  if(action==='mechanism'){const id=objectId??randomUUID();await save({...defs,mechanisms:objectId?defs.mechanisms.map(m=>m.id===objectId?{...m,name}:m):[...defs.mechanisms,{id,name,unitIds:[],joints:[]}]},id)}
  if(action==='joint')await saveJoint();
  if(action==='driver'&&driver)await save({...defs,drivers:[...(defs.drivers??[]).filter(d=>d.id!==driver.id),driver]});
  if(action==='law'&&lawDraft)await save({...defs,laws:[...(defs.laws??[]).filter(l=>l.id!==lawDraft.id),lawDraft]});
  if(action==='study'&&studyDraft){setStudyId(studyDraft.id);await save({...defs,studies:[...defs.studies.filter(s=>s.id!==studyDraft.id),studyDraft]})}

 };
 const failure=error?normalizeOperationFailure(error):undefined;
 const failureNotice=failure?<Alert type="error" showIcon title={failure.message} description={<details><summary>诊断详情</summary><pre style={{whiteSpace:'pre-wrap'}}>{failure.details}</pre></details>}/>:null;
 return <>
  {active&&!action&&failureNotice}
  <CommandDialog id={'dmu.'+(action??'definition')} open={active&&Boolean(action)&&action!=='apply'} title={action==='results'?'仿真与回放':action==='joint'?joint?.name:action==='mechanism'?'编辑机械装置':registry.declaration('dmu.'+action)?.name??'编辑对象'} onClose={()=>closeCommand(action==='results')} onConfirm={confirmDefinition} confirmText={action==='results'?'关闭':action==='run'?'运行':'保存'} confirmLoading={commandPending||pending} confirmDisabled={action!=='results'&&!definitionValid} size={action==='results'?'L':'M'}><div className="motion-study-fields">

   {action==='results'&&<>  <Space wrap><span>机械装置</span><Select aria-label="当前机构" style={{width:210}} value={mechanismId||undefined} options={defs.mechanisms.map(m=>({value:m.id,label:m.name}))} onChange={id=>{void enter(id)}}/>
  {selectedMechanism&&<Typography.Text>{selectedMechanism.joints.length} 个接合 · {(defs.drivers??[]).filter(d=>d.mechanismId===mechanismId).length} 个命令</Typography.Text>}</Space>
  <Typography.Title level={5}>求解与分析</Typography.Title>
  <Space wrap><Select aria-label="当前仿真" style={{width:210}} placeholder="已保存仿真" value={studyId||undefined} options={defs.studies.filter(s=>s.mechanismId===mechanismId).map(s=>({value:s.id,label:s.name}))} onChange={setStudyId}/>{commandButton('run')}{job&&<span>{job.state} {job.progress}%</span>}{commandButton('cancel')}</Space>
  <Typography.Title level={5}>已保存运行与回放</Typography.Title>
  <Select aria-label="冻结运行结果" style={{width:'100%'}} disabled={pending} placeholder="选择冻结运行结果" value={jobId} options={jobs.data?.map(j=>({value:j.id,label:`${String(j.payload.studyName??'研究')} · ${j.state} · ${j.createdAt}`}))} onChange={id=>{foreground.current=undefined;setJobId(id);const j=jobs.data?.find(j=>j.id===id);if(j?.resultObjectId){loaded.current=j.resultObjectId;void load(id)}}}/>
  {job&&done(job)&&<Button disabled={pending} onClick={()=>void deleteResult()}>删除运行结果</Button>}
  {run&&<>
   {run.failure&&<Typography.Paragraph>{run.failure.code}：{run.failure.detail}；停止目标 t={run.failure.timeSeconds.toFixed(3)} s</Typography.Paragraph>}
   <Space wrap>{['play','pause','previous','next','reset','apply'].map(commandButton)}</Space>
   {run.frames.length>0&&<Slider min={0} max={Math.max(0,run.frames.length-1)} value={index} step={1} onChange={i=>{setPlaying(false);setShowing(true);setIndex(i);commandEpoch.current++;setPlan(undefined)}}/>}
   <Space>定位时间 s<InputNumber min={0} max={run.frames.at(-1)?.timeSeconds??0} value={frame?.timeSeconds??0} onChange={v=>{setPlaying(false);setShowing(true);setIndex(motionFrameAt(run,v??0));commandEpoch.current++;setPlan(undefined)}}/></Space>
   {frame&&<Typography.Paragraph>帧 {index+1} · t={frame.timeSeconds.toFixed(3)} s · 运动学 {frame.kinematicValid?'合格':'未检查'} · 驱动 {frame.driverValue.toFixed(6)} rad</Typography.Paragraph>}

  </>}
</>}
   {action==='run'&&<Select aria-label="当前仿真" style={{width:'100%'}} value={studyId||undefined} options={defs.studies.filter(s=>s.mechanismId===mechanismId).map(s=>({value:s.id,label:s.name}))} onChange={setStudyId}/>}
   {(action==='mechanism')&&<><Input aria-label="机构名称" value={name} onChange={e=>setName(e.target.value)}/></>}
   {action==='joint'&&joint&&<>
    <Input aria-label="接合名称" value={joint.name} onChange={e=>changeJoint({name:e.target.value})}/>
    {joint.kind==='GROUND'?<Select style={{width:'100%'}} value={joint.first.instanceId} options={units.map(v=>({value:v.id,label:v.name}))} onChange={instanceId=>changeJoint({first:{instanceId,frame:identity()}})}/>:<>
     {(['first','second'] as const).map(side=><fieldset key={side}><legend>{side==='first'?'第一端（运动）':'第二端（参考）'} {units.find(v=>v.id===joint[side]?.instanceId)?.name}</legend>{(['axis','plane'] as const).map(field=>{const role=`${side}.${field}` as NonNullable<typeof pickRole>;return <div key={field}><Button type={pickRole===role?'primary':'text'} aria-label={`${side==='first'?'第一':'第二'}${field==='axis'?'轴线':'定位平面'}`} onClick={()=>setPickRole(role)}>{field==='axis'?'轴线':'定位平面'}：{joint[side]?.[field]?'已绑定（点击替换）':'选择几何'}</Button></div>})}</fieldset>)}
     <Space wrap><span>轴向偏移 mm</span><InputNumber value={joint.axialOffset?.value??0} onChange={v=>changeJoint({axialOffset:{value:v??0,unit:'mm'}})}/><span>零位 {joint.zero.unit}</span><InputNumber value={joint.zero.value} onChange={v=>changeJoint({zero:{...joint.zero,value:v??0}})}/><Select value={joint.direction} options={[{value:1,label:'正向 +Z'},{value:-1,label:'反向 -Z'}]} onChange={direction=>changeJoint({direction})}/><InputNumber placeholder="下限（可空）" value={joint.lower?.value} onChange={v=>changeJoint({lower:v===null?undefined:{value:v,unit:joint.zero.unit}})}/><InputNumber placeholder="上限（可空）" value={joint.upper?.value} onChange={v=>changeJoint({upper:v===null?undefined:{value:v,unit:joint.zero.unit}})}/></Space>
     <Checkbox checked={angleCommand||Boolean(defs.drivers?.some(d=>d.jointId===joint.id))} disabled={Boolean(defs.drivers?.some(d=>d.jointId===joint.id))} onChange={e=>setAngleCommand(e.target.checked)}>角度命令</Checkbox>
    </>}


   </>}
   {action==='driver'&&driver&&<><Input value={driver.name} onChange={e=>setDriver({...driver,name:e.target.value})}/><Select style={{width:'100%'}} value={driver.jointId} options={selectedMechanism?.joints.filter(j=>j.kind==='REVOLUTE').map(j=>({value:j.id,label:j.name}))} onChange={jointId=>setDriver({...driver,jointId})}/></>}
   {action==='law'&&lawDraft&&<>
    <Input aria-label="运动规律名称" value={lawDraft.name} onChange={e=>setLawDraft({...lawDraft,name:e.target.value})}/>
    <Select aria-label="规律角度命令" style={{width:'100%'}} value={lawDraft.driverId} options={defs.drivers?.filter(d=>d.mechanismId===mechanismId).map(d=>({value:d.id,label:d.name}))} onChange={driverId=>setLawDraft({...lawDraft,driverId})}/>
    <Space wrap><span>起止 {lawDraft.start.unit}</span><InputNumber aria-label="规律起始角度" value={lawDraft.start.value} onChange={v=>setLawDraft({...lawDraft,start:{...lawDraft.start,value:v??0}})}/><InputNumber aria-label="规律结束角度" value={lawDraft.end.value} onChange={v=>setLawDraft({...lawDraft,end:{...lawDraft.end,value:v??0}})}/><span>时长 s</span><InputNumber min={.01} value={lawDraft.durationSeconds} onChange={v=>setLawDraft({...lawDraft,durationSeconds:v??5})}/></Space>
   </>}
   {action==='study'&&studyDraft&&<>
    <Input aria-label="模拟名称" value={studyDraft.name} onChange={e=>setStudyDraft({...studyDraft,name:e.target.value})}/>
    <Select aria-label="模拟运动规律" style={{width:'100%'}} value={studyDraft.lawId} options={defs.laws?.filter(l=>l.mechanismId===mechanismId).map(l=>({value:l.id,label:l.name}))} onChange={lawId=>setStudyDraft({...studyDraft,lawId})}/>
    <Space wrap><span>帧数</span><InputNumber min={2} max={500} precision={0} value={studyDraft.frames} onChange={v=>setStudyDraft({...studyDraft,frames:v??73})}/><span>预算 ms</span><InputNumber min={100} max={120000} value={studyDraft.budgetMs} onChange={v=>setStudyDraft({...studyDraft,budgetMs:v??60000})}/></Space>
   </>}


   {failureNotice}
  </div></CommandDialog>
 <CommandDialog id="dmu.apply" title="转换为产品约束" open={active&&applyOpen} size="L" onClose={()=>closeCommand(action==='results')} onConfirm={apply} confirmText="一次提交并返回装配" confirmDisabled={!plan?.ready||commandPending} confirmLoading={commandPending}>
  <Button loading={commandPending} onClick={()=>void review()}>生成／刷新转换计划</Button>
  <Typography.Paragraph>{showing?`仿真帧 ${index+1}`:"机械装置已确认位姿"}；默认保留旋转自由度。约束、正式位姿、发布映射一次可撤销提交。</Typography.Paragraph><Checkbox checked={lockAngle} onChange={e=>{setLockAngle(e.target.checked);commandEpoch.current++;setPlan(undefined)}}>锁定当前角度</Checkbox>
  {plan&&<><Typography.Paragraph>{plan.poseChanges.length} 个位姿变化 · 求解 {plan.solverStatus??'待解决冲突'} · DOF {plan.degreesOfFreedom}</Typography.Paragraph>{plan.poseChanges.length>0&&<ul>{plan.poseChanges.map(id=>{const unit=units.find(v=>v.id===id),target=frame?.unitPoses[id];return <li key={id}>{unit?.name??id}：位置 mm ({unit?.translation.map(v=>v.toFixed(3)).join(', ')}) → ({target?.translation.map(v=>v.toFixed(3)).join(', ')})；方向采用所选完整帧</li>})}</ul>}<Table size="small" pagination={false} rowKey={v=>JSON.stringify([v.jointId,v.constraintId,v.role,v.action])} dataSource={plan.items} columns={[{title:'关系',dataIndex:'role',render:(role:string,item)=>`${relationLabels[role]??role} · ${view.product?.constraints?.find(c=>c.id===item.constraintId)?.name??defs.mechanisms.flatMap(m=>m.joints).find(j=>j.id===item.jointId)?.name??item.constraintId??''}`},{title:'动作',dataIndex:'action',render:(value:string)=>actionLabels[value]??value},{title:'说明',dataIndex:'detail'},]}/></>}
 </CommandDialog></>;
});
