import {forwardRef,useCallback,useEffect,useImperativeHandle,useRef,useState} from 'react';
import {useQuery,useQueryClient} from '@tanstack/react-query';
import {Button,Checkbox,Input,InputNumber,Select,Slider,Space,Table,Typography} from 'antd';
import {useOperationFeedback} from '../../cad/command/operation-feedback';
import type {FeatureSelectionSession} from '../../cad/interaction/feature-selection';
import {createActor} from 'xstate';
import {assemblyPreviewMachine} from './assembly-preview-machine';
import {api} from '../../api/client';
import {queryKeys} from '../../app/query-keys';
import type {DocumentView,Job,SelectionItem,AssemblyGeometryRef} from '../../types';
import {randomUUID} from '../../utils/random-uuid';
import {mechanismSupportCandidate,type JointSupportRole} from '../../cad/assembly/mechanism-selection';
import {assemblyGeometryRef} from '../../cad/assembly/assembly-reference';
import {useCommandRegistry} from '../../cad/command/command-context';
import type {CommandOperation} from '../../cad/command/command-registry';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {ToolButton} from '../../cad/overlay/tool-button';
import {CadIcon} from '../../cad/overlay/cad-icons';
import {motionFrameAt,motionPlayback,motionProblemFrames,quantityIn,type JointEndpoint,type KinematicsDefinitions,type MechanismJoint,type MotionPlayback,type MotionRun,type MotionStudy,type MotionApplyRequest,type MotionApplyPlan,type InterferenceAnalysis,type MotionDriver} from '../../cad/assembly/motion-study';

const identity=()=>({translation:[0,0,0] as [number,number,number],rotation:[0,0,0,1] as [number,number,number,number]});
const addressKey=(v:{instancePath:{canonical:string};bodyId:string})=>JSON.stringify([v.instancePath.canonical,v.bodyId]);
const relationLabels:Record<string,string>={ground:'固定',axis:'轴线相合','axial-location':'轴向定位',orientation:'方向一致','angle-lock':'锁定角度',existing:'已有正式约束',solver:'正式装配求解'};
const actionLabels:Record<string,string>={ADD:'新增',REUSE:'复用',UPDATE:'更新',CONFLICT:'冲突',REPLACE:'替换',SUPPRESS:'停用'};
const done=(job:Job)=>['SUCCEEDED','FAILED','CANCELED'].includes(job.state);
const empty=():KinematicsDefinitions=>({mechanisms:[],studies:[],drivers:[],analyses:[],associations:[]});
export type MotionPanelHandle={execute:(action:string,objectId?:string,operation?:CommandOperation)=>void|Promise<void>;closeCommand:()=>void;enabled:(action:string)=>boolean};
type PreviewPose=import("../../cad/assembly/motion-study").MotionPose&{instanceId:string};
type Props={onMechanism:(m?:import("../../cad/assembly/motion-study").Mechanism)=>void;onStatus:(text:string)=>void;onSelectionSession:(session?:FeatureSelectionSession)=>void;onPreview:(poses?:PreviewPose[])=>void;view:DocumentView;active:boolean;selection:SelectionItem|null;canEdit:boolean;onSave:(k:KinematicsDefinitions)=>Promise<void>;onApply:(req:MotionApplyRequest)=>Promise<void>;onPlayback:(p?:MotionPlayback)=>void;onHighlight:(s:SelectionItem[])=>void;onClose:()=>void;onCommandState:(active:boolean)=>void};
export const MotionStudyPanel=forwardRef<MotionPanelHandle,Props>(function MotionStudyPanel({view,active,selection,canEdit,onSave,onApply,onPlayback,onHighlight,onClose,onCommandState,onStatus,onSelectionSession,onPreview,onMechanism},ref){
 const client=useQueryClient(),registry=useCommandRegistry(),feedback=useOperationFeedback();
 const [previewActor]=useState(()=>createActor(assemblyPreviewMachine));
 const [previewReady,setPreviewReady]=useState(false),[previewPending,setPreviewPending]=useState(false),[previewSuspended,setPreviewSuspended]=useState(false);
 const pickedSupports=useRef(new Map<JointSupportRole,SelectionItem>());
 const previewSequence=useRef(0),previewAbort=useRef<AbortController|undefined>(undefined),qualifiedJoint=useRef<MechanismJoint|undefined>(undefined),qualifiedInput=useRef<string|undefined>(undefined);
 const acceptedPoses=useRef<PreviewPose[]|undefined>(undefined);
 const supports=useRef(new Map<string,Awaited<ReturnType<typeof api.inspectAssemblySupports>>["supports"][number]|null>()),supportPending=useRef(new Map<string,Promise<Awaited<ReturnType<typeof api.inspectAssemblySupports>>["supports"][number]|null>>());
 const pickSessionEpoch=useRef(0);
 const newEntry=useRef<{id:string;documentId:string;revisionId?:string;ready?:Promise<void>} | undefined>(undefined);
 const discardEmptyEntry=async()=>{
  const entry=newEntry.current;if(!entry)return;newEntry.current=undefined;
  await entry.ready;
  const latest=client.getQueryData<DocumentView>(queryKeys.document(entry.documentId));
  const k=latest?.product?.kinematics,m=k?.mechanisms.find(m=>m.id===entry.id);
  if(!latest||latest.document.versionId!==entry.revisionId||!m||m.joints.length||k?.drivers?.some(d=>d.mechanismId===m.id)||k?.studies.some(s=>s.mechanismId===m.id))return;
  try{const updated=await api.command(entry.documentId,{type:'SAVE_KINEMATICS',versionId:latest.document.versionId,kinematics:{...k!,mechanisms:k!.mechanisms.filter(m=>m.id!==entry.id)}});client.setQueryData(queryKeys.document(entry.documentId),updated);await client.invalidateQueries({queryKey:queryKeys.document(entry.documentId)})}catch(e){feedback(e,'机构命令')}
 };
 const [draftMechanism,setDraftMechanism]=useState<import("../../cad/assembly/motion-study").Mechanism>();
 useEffect(()=>{previewActor.start();return()=>{previewActor.stop()}},[previewActor]);

 const [defs,setDefs]=useState<KinematicsDefinitions>(()=>structuredClone(view.product?.kinematics??empty()));
 const [mechanismId,setMechanismId]=useState(defs.mechanisms[0]?.id??''),[studyId,setStudyId]=useState(defs.studies[0]?.id??''),[analysisId,setAnalysisId]=useState('');
 const [commandPending,setCommandPending]=useState(false),[pending,setPending]=useState(false),[error,setError]=useState<unknown>(),[jobId,setJobId]=useState<string>();
 const [run,setRun]=useState<MotionRun>(),[index,setIndex]=useState(0),[playing,setPlaying]=useState(false),[showing,setShowing]=useState(false);
 const [action,setAction]=useState<string>(),[objectId,setObjectId]=useState<string>(),[name,setName]=useState('');
 const [joint,setJoint]=useState<MechanismJoint>(),[pickRole,setPickRole]=useState<JointSupportRole>();
 const [driver,setDriver]=useState<MotionDriver>(),[studyDraft,setStudyDraft]=useState<MotionStudy>(),[analysisDraft,setAnalysisDraft]=useState<InterferenceAnalysis>();
 const [trialDriverId,setTrialDriverId]=useState('');
 const [trial,setTrial]=useState(0),[proposals,setProposals]=useState<MechanismJoint[]>([]),[imports,setImports]=useState<string[]>([]),[supplemental,setSupplemental]=useState<string[]>([]),[detach,setDetach]=useState(false);
 const [plan,setPlan]=useState<MotionApplyPlan>(),[lockAngle,setLockAngle]=useState(false),[resolutions,setResolutions]=useState<Record<string,string>>({}),[applyOpen,setApplyOpen]=useState(false);
 const foreground=useRef<string|undefined>(undefined),loaded=useRef<string|undefined>(undefined);
 const commandEpoch=useRef(0),commandOwner=useRef<CommandOperation|undefined>(undefined),applying=useRef(false);
 const revisionRef=useRef(view.document.versionId);revisionRef.current=view.document.versionId;
 const epoch=useRef(0),alive=useRef(true),activeRef=useRef(active);activeRef.current=active;
 const callbacks=useRef({onPlayback,onHighlight,onStatus,onSelectionSession,onPreview,onMechanism});callbacks.current={onPlayback,onHighlight,onStatus,onSelectionSession,onPreview,onMechanism};
 const selectedMechanism=defs.mechanisms.find(m=>m.id===mechanismId)??draftMechanism,study=defs.studies.find(s=>s.id===studyId&&s.mechanismId===mechanismId),analysis=defs.analyses?.find(a=>a.id===analysisId);
 useEffect(()=>{callbacks.current.onMechanism(active?(showing&&run?(run.snapshot.currentOnly?undefined:run.snapshot.mechanism):selectedMechanism):undefined)},[active,showing,run,selectedMechanism]);
 const units=view.product?.instances??[],geometries=view.resolvedInstances??[];
 const editEnabled=canEdit&&!pending&&!commandPending&&!showing;
 const jobs=useQuery({queryKey:['motion-jobs',view.document.id],queryFn:async()=> (await api.listJobs()).filter(j=>j.type==='MOTION_STUDY'&&j.documentId===view.document.id),refetchInterval:active&&jobId?1000:5000});
 const job=jobs.data?.find(j=>j.id===jobId);
 const closeCommand=()=>{pickedSupports.current.clear();qualifiedInput.current=undefined;commandEpoch.current++;commandOwner.current=undefined;setCommandPending(false);setAction(undefined);setPickRole(undefined);previewAbort.current?.abort();previewSequence.current++;setPreviewPending(false);callbacks.current.onSelectionSession();callbacks.current.onPreview(showing?undefined:acceptedPoses.current);callbacks.current.onMechanism(activeRef.current?(showing&&run?(run.snapshot.currentOnly?undefined:run.snapshot.mechanism):selectedMechanism):undefined);setApplyOpen(false);setPlan(undefined);onCommandState(false)};
 const release=()=>{foreground.current=undefined;epoch.current++;setPending(false);setPlaying(false);setShowing(false);acceptedPoses.current=undefined;previewAbort.current?.abort();previewSequence.current++;callbacks.current.onPreview();callbacks.current.onMechanism();callbacks.current.onSelectionSession();callbacks.current.onStatus("");callbacks.current.onPlayback();callbacks.current.onHighlight([])};
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;void discardEmptyEntry();epoch.current++;previewAbort.current?.abort();callbacks.current.onSelectionSession();callbacks.current.onPreview();callbacks.current.onStatus("");callbacks.current.onPlayback();callbacks.current.onHighlight([])}},[]);
 useEffect(()=>{if(!active){foreground.current=undefined;release();closeCommand();setDraftMechanism(undefined);void discardEmptyEntry()}},[active]);
 useEffect(()=>{
  foreground.current=undefined;release();
  // An accepted apply refreshes Head before its command promise finishes. Keep
  // that owned invocation until it can return to assembly; explicit cancel still
  // disposes it. External revisions always release frozen viewport state.
  if(!applying.current||!commandOwner.current?.current)closeCommand();
  setError(undefined);setPending(false);supports.current.clear();supportPending.current.clear();
  const next=structuredClone(view.product?.kinematics??empty());setDefs(next);
  setDraftMechanism(draft=>draft&&next.mechanisms.some(m=>m.id===draft.id)?undefined:draft);
  setMechanismId(id=>next.mechanisms.some(m=>m.id===id)?id:next.mechanisms[0]?.id??'');setStudyId(id=>next.studies.some(s=>s.id===id)?id:next.studies[0]?.id??'');
  // Jobs survive panel/Revision changes. A result enters the viewport only after
  // an explicit load or the exact foreground run's completion in this session.
 },[view.document.id,view.document.versionId]);
 const save=async(next:KinematicsDefinitions)=>{const generation=commandEpoch.current;setCommandPending(true);setError(undefined);try{if(draftMechanism&&!next.mechanisms.some(m=>m.id===draftMechanism.id))next={...next,mechanisms:[...next.mechanisms,draftMechanism]};await onSave(next);if(generation===commandEpoch.current)closeCommand()}catch(e){if(generation===commandEpoch.current)setError(e)}finally{if(generation===commandEpoch.current)setCommandPending(false)}};
 const enter=async(id?:string)=>{
  await discardEmptyEntry();release();closeCommand();setPreviewSuspended(false);
  const existing=defs.mechanisms.find(m=>m.id===id);
  if(existing){setDraftMechanism(undefined);setMechanismId(existing.id);setStudyId(defs.studies.find(s=>s.mechanismId===id)?.id??'');return}
  if(!canEdit)return;
  const m={id:randomUUID(),name:'机构 '+(defs.mechanisms.length+1),unitIds:[],joints:[]};
  const entry:{id:string;documentId:string;revisionId?:string;ready?:Promise<void>}={id:m.id,documentId:view.document.id};newEntry.current=entry;
  setDraftMechanism(m);setMechanismId(m.id);setStudyId('');setCommandPending(true);
  entry.ready=onSave({...defs,mechanisms:[...defs.mechanisms,m]}).then(()=>{entry.revisionId=client.getQueryData<DocumentView>(queryKeys.document(entry.documentId))?.document.versionId}).catch(e=>{if(newEntry.current===entry)newEntry.current=undefined;setDraftMechanism(undefined);feedback(e,'机构命令')}).finally(()=>setCommandPending(false));
  await entry.ready;
 };
 const execute=(nextAction:string,id?:string,operation?:CommandOperation):void|Promise<void>=>{
  if(nextAction==='enter')return enter(id)
  if(nextAction==='exit'){release();closeCommand();setDraftMechanism(undefined);return discardEmptyEntry().then(onClose)}
  if(nextAction==='cancel'&&job)return api.cancelJob(job.id).then(()=>client.invalidateQueries({queryKey:['motion-jobs',view.document.id]}));
  if(nextAction==='restore'){setPreviewSuspended(true);release();return}
  if(['play','pause','previous','next','reset'].includes(nextAction)){
   if(!run?.frames.length)return;setShowing(true);setPlaying(nextAction==='play');
   if(nextAction==='previous')setIndex(i=>Math.max(0,i-1));
   if(nextAction==='next')setIndex(i=>Math.min(run.frames.length-1,i+1));
   if(nextAction==='reset')setIndex(0);return;
  }
  if(!['result','results','apply','run','check'].includes(nextAction))setPreviewSuspended(false);
  commandEpoch.current++;commandOwner.current=operation;setCommandPending(false);setError(undefined);setPlan(undefined);setPickRole(undefined);setObjectId(id);
  if(nextAction==='result'&&id){foreground.current=undefined;setJobId(id);setAction('results');onCommandState(true);const j=jobs.data?.find(j=>j.id===id);if(j?.resultObjectId)void load(id);return}
  let relationRole:string|undefined;
  if(nextAction==='edit-relation'){
   const owner=defs.mechanisms.flatMap(m=>m.joints).find(j=>id?.startsWith(j.id+'/'));
   if(!owner)return;relationRole=id!.slice(owner.id.length+1);id=owner.id;setObjectId(id);nextAction='edit';
  }
  if(nextAction==='edit'){
   const m=defs.mechanisms.find(m=>m.id===id),owner=defs.mechanisms.find(m=>m.joints.some(j=>j.id===id));
   if(m){setMechanismId(m.id);setName(m.name);nextAction='mechanism'}
   else if(owner){setMechanismId(owner.id);setJoint(structuredClone(owner.joints.find(j=>j.id===id)!));nextAction='joint';if(relationRole)setPickRole(relationRole==='axial-location'?'first.plane':'first.axis')}
   else if(defs.drivers?.some(d=>d.id===id)){const d=defs.drivers.find(d=>d.id===id)!;setMechanismId(d.mechanismId);setDriver(structuredClone(d));nextAction='driver'}
   else if(defs.studies.some(s=>s.id===id)){const s=defs.studies.find(s=>s.id===id)!;setMechanismId(s.mechanismId);setStudyDraft(structuredClone(s));setStudyId(id!);nextAction='study'}
   else if(defs.analyses?.some(a=>a.id===id)){setAnalysisDraft(structuredClone(defs.analyses.find(a=>a.id===id)!));setAnalysisId(id!);nextAction='interference'}
  }
  if(['ground','revolute','prismatic','rigid'].includes(nextAction)){
   if(!selectedMechanism){setError('请先创建或选择机构');return}
   const kind=nextAction==='ground'?'GROUND':nextAction.toUpperCase() as MechanismJoint['kind'];const selected=selection?.instanceId??units[0]?.id??'';
   const first:JointEndpoint={instanceId:selected,frame:identity()};setJoint({id:randomUUID(),name:registry.declaration('dmu.'+nextAction)?.name??'接合',kind,first,second:kind==='GROUND'?undefined:{instanceId:units.find(v=>v.id!==selected)?.id??'',frame:identity()},zero:{value:0,unit:kind==='REVOLUTE'?'deg':'mm'},axialOffset:{value:0,unit:'mm'},direction:1});nextAction='joint';setPickRole(kind==='GROUND'?undefined:'first.axis');setObjectId(undefined)
  }
  if(nextAction==='driver'&&!id){const j=selectedMechanism?.joints.find(j=>j.kind==='REVOLUTE'||j.kind==='PRISMATIC');if(!j){setError('请先创建可驱动的接合');return}setDriver({id:randomUUID(),name:'驱动',mechanismId:mechanismId,jointId:j.id})}
  if(nextAction==='study'&&!id){const d=defs.drivers?.find(d=>d.mechanismId===mechanismId);if(!d){setError('请先保存独立驱动');return}const j=selectedMechanism!.joints.find(j=>j.id===d.jointId)!;const unit=j.kind==='REVOLUTE'?'deg':'mm';setStudyDraft({id:randomUUID(),name:'线性仿真',mechanismId,driverId:d.id,driverJointId:'',start:{value:0,unit},end:{value:unit==='deg'?360:30,unit},durationSeconds:5,frames:73,budgetMs:60000,checkDmu:false,includeSameUnit:false,clearance:{value:0,unit:'mm'}})}
  if(nextAction==='trial'){const d=defs.drivers?.find(d=>d.mechanismId===mechanismId);if(!d){setError('请先保存独立驱动');return}setTrialDriverId(d.id)}
  if(nextAction==='interference'&&!id)setAnalysisDraft({id:randomUUID(),name:'干涉分析',clearance:{value:0,unit:'mm'},includeSameUnit:false});
  if(nextAction==='import'){setImports([]);setSupplemental(selectedMechanism?.supplementalConstraintIds??[]);setDetach(false);const generation=commandEpoch.current;void api.motionJointProposals(view.document.id).then(result=>{if(generation===commandEpoch.current&&(!operation||operation.current))setProposals(result)}).catch(e=>{if(generation===commandEpoch.current)setError(e)})}
  if(nextAction==='apply'){if(!run||!jobId||!run.frames[index]?.kinematicValid){setError('先加载并选中合格的机构帧');return}setPlaying(false);setApplyOpen(true);setLockAngle(false);setResolutions({})}
  setAction(nextAction);onCommandState(true)
 };
 const enabled=(command:string)=>{
  if(!active)return false;
  if(command==='exit'||command==='results')return true;
  if(pending||commandPending)return false;
  if(command==='play')return Boolean(run?.frames.length&&!playing);
  if(command==='pause')return playing;
  if(command==='previous'||command==='next'||command==='reset')return Boolean(run?.frames.length);
  if(command==='restore')return showing;
  if(command==='cancel')return Boolean(job?.canCancel);
  if(command==='apply')return Boolean(canEdit&&run&&!run.snapshot.currentOnly&&frame?.kinematicValid);
  if(command==='run')return Boolean(!showing&&defs.studies.some(s=>s.mechanismId===mechanismId)&&(!job||done(job)));
  if(command==='check')return Boolean(!showing&&defs.analyses?.length&&(!job||done(job)));
  if(!editEnabled)return false;
  if(command==='interference')return true;
  if(!selectedMechanism)return false;
  if(['ground','revolute','prismatic','rigid'].includes(command))return units.length>0;
  if(command==='driver')return selectedMechanism.joints.some(j=>['REVOLUTE','PRISMATIC'].includes(j.kind));
  if(command==='study'||command==='trial')return Boolean(defs.drivers?.some(d=>d.mechanismId===mechanismId));
  return true;
 };
 useImperativeHandle(ref,()=>({execute,closeCommand,enabled}));
 useEffect(()=>{registry.notifyStateChanged()},[registry,active,pending,commandPending,canEdit,showing,playing,run,index,study,analysis,selectedMechanism,job,defs]);
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
  const m=selectedMechanism;if(!active||showing||previewSuspended||!m){setPreviewPending(false);return}
  const draft=action==='joint'?joint:undefined;
  if(draft&&draft.kind!=='GROUND'&&(!draft.first.axis||!draft.second?.axis)){setPreviewReady(false);return}
  const joints=draft?[...m.joints.filter(j=>j.id!==draft.id),draft]:m.joints;
  if(!joints.length){setPreviewReady(false);return}
  const generation=++previewSequence.current,abort=new AbortController();previewAbort.current?.abort();previewAbort.current=abort;
  setPreviewReady(false);setPreviewPending(true);qualifiedJoint.current=undefined;callbacks.current.onPreview(acceptedPoses.current);previewActor.send({type:'REQUEST',sequence:generation});
  const timer=setTimeout(()=>{void api.previewMechanism(view.document.id,{requestId:randomUUID(),baseRevisionId:view.document.versionId,mechanism:{...m,joints},draftJointId:draft?.id},abort.signal).then(result=>{
   if(abort.signal.aborted||generation!==previewSequence.current||!activeRef.current)return;
   previewActor.send({type:'RESOLVE',sequence:generation});setPreviewReady(true);setPreviewPending(false);qualifiedInput.current=JSON.stringify(draft);qualifiedJoint.current=result.mechanism.joints.find(j=>j.id===draft?.id);callbacks.current.onPreview(result.instancePoses);callbacks.current.onMechanism(result.mechanism);
   if(!draft)acceptedPoses.current=result.instancePoses;
  }).catch(e=>{if(abort.signal.aborted||generation!==previewSequence.current)return;previewActor.send({type:'REJECT',sequence:generation,error:String(e)});setPreviewPending(false);setError(e);callbacks.current.onPreview(acceptedPoses.current)})},100);
  return()=>{clearTimeout(timer);abort.abort()};
 },[active,showing,previewSuspended,action,joint,selectedMechanism,view.document.versionId]);
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
 const start=async(currentOnly:boolean,analysisID?:string,trialValue?:number,trialDriver?:string)=>{
  const generation=++epoch.current,ownerEpoch=commandEpoch.current;setPending(true);setError(undefined);setPlaying(false);setShowing(false);setRun(undefined);setIndex(0);setJobId(undefined);setPreviewSuspended(true);
  try{const j=await api.startMotionRun(view.document.id,{baseRevisionId:view.document.versionId,requestId:randomUUID(),studyId:trialDriver?undefined:study?.id,driverId:trialDriver,analysisId:analysisID,currentOnly,trial:trialValue===undefined?undefined:{value:trialValue,unit:trialDriver?selectedMechanism?.joints.find(j=>j.id===defs.drivers?.find(d=>d.id===trialDriver)?.jointId)?.zero.unit??'deg':study?.start.unit??'deg'},clearance:{value:0,unit:'mm'}});if(!alive.current||generation!==epoch.current||ownerEpoch!==commandEpoch.current||!activeRef.current)return;setJobId(j.id);if(activeRef.current)foreground.current=j.id;loaded.current=undefined;await client.invalidateQueries({queryKey:['motion-jobs',view.document.id]});if(!alive.current||!activeRef.current||ownerEpoch!==commandEpoch.current)return;closeCommand();setAction('results');onCommandState(true)}catch(e){if(generation===epoch.current)setError(e)}finally{if(generation===epoch.current)setPending(false)}
 };
 const deleteResult=async()=>{
  if(!job||!done(job))return;
  const id=job.id,generation=epoch.current;setPending(true);
  try{await api.deleteMotionResult(id);if(generation===epoch.current&&activeRef.current&&jobId===id){loaded.current=undefined;setJobId(undefined);setRun(undefined);setIndex(0);setPreviewSuspended(true);release()}await Promise.all([client.invalidateQueries({queryKey:['motion-jobs',view.document.id]}),client.invalidateQueries({queryKey:queryKeys.jobs})])}catch(e){feedback(e,'机构运行')}finally{if(generation===epoch.current)setPending(false)}
 };
 const applyInput=():MotionApplyRequest=>({jobId:jobId!,frameIndex:index,lockAngle,resolutions});
 const review=async()=>{const generation=++commandEpoch.current;setCommandPending(true);setError(undefined);try{const result=await api.planMotionApply(view.document.id,view.document.versionId,applyInput());if(alive.current&&activeRef.current&&generation===commandEpoch.current)setPlan(result)}catch(e){if(generation===commandEpoch.current)setError(e)}finally{if(generation===commandEpoch.current)setCommandPending(false)}};
 const apply=async()=>{
  if(!plan?.ready)return;setCommandPending(true);applying.current=true;
  const owner=commandOwner.current,generation=commandEpoch.current,revision=view.document.versionId;
  try{await onApply({...applyInput(),planDigest:plan.digest});if(alive.current&&activeRef.current&&(!owner||owner.current)){release();closeCommand();onClose()}}
  catch(e){if(alive.current&&activeRef.current&&(!owner||owner.current)){if(revisionRef.current!==revision)closeCommand();setError(e);setPlan(undefined)}}
  finally{applying.current=false;if(generation===commandEpoch.current)setCommandPending(false)}
 };
 const highlight=(a:string,b:string)=>{const s=run?.snapshot;const items=s?.geometryUnits.filter(g=>g.id===a||g.id===b).map(g=>({kind:'body' as const,id:`${g.address.instancePath.canonical}:body:${g.address.bodyId}`,bodyId:g.address.bodyId,instancePath:g.address.instancePath,occurrencePath:g.address.instancePath.canonical,rootDocumentId:s.documentId,instanceId:g.address.instancePath.segments[0]?.instanceId,documentId:g.address.instancePath.segments.at(-1)?.referencedDocumentId,versionId:g.address.instancePath.segments.at(-1)?.resolvedVersionId,geometryKey:g.geometryKey}));onHighlight(items??[])};
 const label=(id:string)=>{const g=run?.snapshot.geometryUnits.find(g=>g.id===id);return g?`${g.address.instancePath.display} / ${g.address.bodyId}`:id};
 const frame=run?.frames[index];
 useEffect(()=>{commandEpoch.current++;setPlan(undefined)},[index,jobId]);
 const changeJoint=(patch:Partial<MechanismJoint>)=>setJoint(j=>j?{...j,...patch}:j);
 const saveJoint=async()=>{if(!joint||!selectedMechanism)return;const m={...selectedMechanism,joints:[...selectedMechanism.joints.filter(j=>j.id!==joint.id),qualifiedJoint.current??joint]};m.unitIds=[...new Set(m.joints.flatMap(j=>[j.first.instanceId,...(j.second?[j.second.instanceId]:[])]))];await save({...defs,mechanisms:[...defs.mechanisms.filter(v=>v.id!==m.id),m]})};
 const definitionValid=editEnabled&&(
  action==='mechanism'?Boolean(name.trim()):
  action==='joint'?Boolean(previewReady&&!previewPending&&qualifiedInput.current===JSON.stringify(joint)&&joint?.name&&joint.first.instanceId&&(joint.kind==='GROUND'||joint.second?.instanceId&&joint.first.axis&&joint.first.plane&&joint.second.axis&&joint.second.plane)):
  action==='driver'?Boolean(driver?.name&&driver.jointId):action==='study'?Boolean(studyDraft?.name&&studyDraft.driverId):
  action==='run'?Boolean(study):action==='check'?Boolean(analysis):action==='results'?true:action==='trial'?Boolean(trialDriverId):action==='interference'?Boolean(analysisDraft?.name):action==='import'?Boolean(selectedMechanism):false);
 const confirmDefinition=async()=>{
  if(action==='results'){closeCommand();return}
  if(!definitionValid)return;
  if(action==='run'){await start(false);return}
  if(action==='check'){await start(!analysis?.studyId,analysisId);return}
  if(action==='mechanism'){const id=objectId??randomUUID();setMechanismId(id);await save({...defs,mechanisms:objectId?defs.mechanisms.map(m=>m.id===objectId?{...m,name}:m):[...defs.mechanisms,{id,name,unitIds:[],joints:[]}]})}
  if(action==='joint')await saveJoint();
  if(action==='driver'&&driver)await save({...defs,drivers:[...(defs.drivers??[]).filter(d=>d.id!==driver.id),driver]});
  if(action==='study'&&studyDraft){setStudyId(studyDraft.id);await save({...defs,studies:[...defs.studies.filter(s=>s.id!==studyDraft.id),studyDraft]})}
  if(action==='trial')await start(false,undefined,trial,trialDriverId);
  if(action==='interference'&&analysisDraft){setAnalysisId(analysisDraft.id);await save({...defs,analyses:[...(defs.analyses??[]).filter(a=>a.id!==analysisDraft.id),analysisDraft]})}
  if(action==='import'&&selectedMechanism){
   const joints=[...selectedMechanism.joints];for(const j of proposals.filter(j=>imports.includes(j.id))){const existing=joints.findIndex(old=>old.sources?.some(src=>j.sources?.some(s=>s.constraintId===src.constraintId)));if(existing>=0)joints[existing]={...j,id:joints[existing].id};else joints.push(j)}
   const m={...selectedMechanism,joints,supplementalConstraintIds:supplemental,unitIds:[...new Set(joints.flatMap(j=>[j.first.instanceId,...(j.second?[j.second.instanceId]:[])]))]};
   await save({...defs,mechanisms:[...defs.mechanisms.filter(v=>v.id!==m.id),m],associations:detach?(defs.associations??[]).filter(a=>a.mechanismId!==mechanismId):defs.associations});
  }
 };
 return <>
  <CommandDialog id={'dmu.'+(action??'definition')} open={active&&Boolean(action)&&action!=='apply'} title={action==='results'?'仿真与回放':action==='joint'?joint?.name:action==='mechanism'?'编辑机构':registry.declaration('dmu.'+action)?.name??'编辑对象'} onClose={closeCommand} onConfirm={confirmDefinition} confirmText={action==='results'?'关闭':action==='run'?'运行':action==='check'?'检查':action==='trial'?'求解指定坐标':'保存'} confirmLoading={commandPending||pending} confirmDisabled={action!=='results'&&!definitionValid} size={action==='results'?'L':'M'}><div className="motion-study-fields">

   {action==='results'&&<>  <Space wrap><span>机构</span><Select aria-label="当前机构" style={{width:210}} value={mechanismId||undefined} options={[...defs.mechanisms,...(draftMechanism?[draftMechanism]:[])].map(m=>({value:m.id,label:m.name}))} onChange={id=>{closeCommand();setMechanismId(id);setStudyId(defs.studies.find(s=>s.mechanismId===id)?.id??'')}}/>
  {selectedMechanism&&<Typography.Text>{selectedMechanism.joints.length} 个接合 · {(defs.drivers??[]).filter(d=>d.mechanismId===mechanismId).length} 个驱动</Typography.Text>}</Space>
  <Typography.Title level={5}>求解与分析</Typography.Title>
  <Space wrap><Select aria-label="当前仿真" style={{width:210}} placeholder="已保存仿真" value={studyId||undefined} options={defs.studies.filter(s=>s.mechanismId===mechanismId).map(s=>({value:s.id,label:s.name}))} onChange={setStudyId}/>{commandButton('run')}<Select aria-label="当前干涉分析" allowClear style={{width:210}} placeholder="已保存干涉分析" value={analysisId||undefined} options={defs.analyses?.map(a=>({value:a.id,label:a.name}))} onChange={setAnalysisId}/>{commandButton('check')}{job&&<span>{job.state} {job.progress}%</span>}{commandButton('cancel')}</Space>
  <Typography.Title level={5}>已保存运行与回放</Typography.Title>
  <Select aria-label="冻结运行结果" style={{width:'100%'}} disabled={pending} placeholder="选择冻结运行结果" value={jobId} options={jobs.data?.map(j=>({value:j.id,label:`${String(j.payload.studyName??'研究')} · ${j.state} · ${j.createdAt}`}))} onChange={id=>{foreground.current=undefined;setJobId(id);const j=jobs.data?.find(j=>j.id===id);if(j?.resultObjectId){loaded.current=j.resultObjectId;void load(id)}}}/>
  {job&&done(job)&&<Button disabled={pending} onClick={()=>void deleteResult()}>删除运行结果</Button>}
  {run&&<>
   {run.failure&&<Typography.Paragraph>{run.failure.code}：{run.failure.detail}；停止目标 t={run.failure.timeSeconds.toFixed(3)} s</Typography.Paragraph>}
   <Space wrap>{['play','pause','previous','next','reset','restore','apply'].map(commandButton)}</Space>
   {run.frames.length>0&&<Slider min={0} max={Math.max(0,run.frames.length-1)} value={index} step={1} onChange={i=>{setPlaying(false);setShowing(true);setIndex(i);commandEpoch.current++;setPlan(undefined)}}/>}
   <Space>定位时间 s<InputNumber min={0} max={run.frames.at(-1)?.timeSeconds??0} value={frame?.timeSeconds??0} onChange={v=>{setPlaying(false);setShowing(true);setIndex(motionFrameAt(run,v??0));commandEpoch.current++;setPlan(undefined)}}/></Space>
   {frame&&<Typography.Paragraph>帧 {index+1} · t={frame.timeSeconds.toFixed(3)} s · 运动学 {frame.kinematicValid?'合格':'未检查'} · DMU {frame.dmuConclusion} {!run.snapshot.currentOnly&&<> · 驱动 {frame.driverValue.toFixed(6)} {['deg','rad'].includes(run.snapshot.study.start.unit)?'rad':'mm'}</>}</Typography.Paragraph>}
   <Space wrap>{motionProblemFrames(run).map(i=><Button key={i} size="small" onClick={()=>{setPlaying(false);setShowing(true);setIndex(i)}}>问题帧 {i+1}</Button>)}</Space>
   <Table size="small" pagination={{pageSize:8}} rowKey={p=>p.firstId+p.secondId} dataSource={frame?.dmu?.pairs??[]} onRow={p=>({onClick:()=>{setShowing(true);highlight(p.firstId,p.secondId)}})} columns={[{title:'实例 + Body',dataIndex:'firstId',render:label},{title:'实例 + Body',dataIndex:'secondId',render:label},{title:'分类',dataIndex:'classification'},{title:'距离 mm',dataIndex:'distanceMm',render:(v:number,p)=>p.complete?v.toFixed(6):'未完成'},{title:'间隙',render:(_,p)=>!p.complete?'无法判定':p.clearanceSatisfied?'满足':'不满足'},{title:'诊断',dataIndex:'diagnostic'}]}/><Typography.Text type="secondary">离散帧精确检查，不代表帧间连续无碰撞。</Typography.Text>
  </>}
</>}
   {action==='run'&&<Select aria-label="当前仿真" style={{width:'100%'}} value={studyId||undefined} options={defs.studies.filter(s=>s.mechanismId===mechanismId).map(s=>({value:s.id,label:s.name}))} onChange={setStudyId}/>}
   {action==='check'&&<Select aria-label="当前干涉分析" style={{width:'100%'}} value={analysisId||undefined} options={defs.analyses?.map(a=>({value:a.id,label:a.name}))} onChange={setAnalysisId}/>}
   {(action==='mechanism')&&<><Input aria-label="机构名称" value={name} onChange={e=>setName(e.target.value)}/></>}
   {action==='joint'&&joint&&<>
    <Input aria-label="接合名称" value={joint.name} onChange={e=>changeJoint({name:e.target.value})}/>
    {joint.kind==='GROUND'?<Select style={{width:'100%'}} value={joint.first.instanceId} options={units.map(v=>({value:v.id,label:v.name}))} onChange={instanceId=>changeJoint({first:{instanceId,frame:identity()}})}/>:<>
     {(['first','second'] as const).map(side=><fieldset key={side}><legend>{side==='first'?'第一端（运动）':'第二端（参考）'} {units.find(v=>v.id===joint[side]?.instanceId)?.name}</legend>{(['axis','plane'] as const).map(field=>{const role=`${side}.${field}` as NonNullable<typeof pickRole>;return <div key={field}><Button type={pickRole===role?'primary':'text'} aria-label={`${side==='first'?'第一':'第二'}${field==='axis'?'轴线':'定位平面'}`} onClick={()=>setPickRole(role)}>{field==='axis'?'轴线':'定位平面'}：{joint[side]?.[field]?'已绑定（点击替换）':'选择几何'}</Button></div>})}</fieldset>)}
     <Space wrap><span>轴向偏移 mm</span><InputNumber value={joint.axialOffset?.value??0} onChange={v=>changeJoint({axialOffset:{value:v??0,unit:'mm'}})}/><span>零位 {joint.zero.unit}</span><InputNumber value={joint.zero.value} onChange={v=>changeJoint({zero:{...joint.zero,value:v??0}})}/><Select value={joint.direction} options={[{value:1,label:'正向 +Z'},{value:-1,label:'反向 -Z'}]} onChange={direction=>changeJoint({direction})}/><InputNumber placeholder="下限（可空）" value={joint.lower?.value} onChange={v=>changeJoint({lower:v===null?undefined:{value:v,unit:joint.zero.unit}})}/><InputNumber placeholder="上限（可空）" value={joint.upper?.value} onChange={v=>changeJoint({upper:v===null?undefined:{value:v,unit:joint.zero.unit}})}/></Space>
    </>}
    {(joint.sources?.length??0)>0&&<><Typography.Paragraph>来源：{joint.sources!.map(v=>v.constraintId).join(', ')}。来源变化不自动改写接合；可在装配关联中重新提案并确认。</Typography.Paragraph><Button onClick={()=>changeJoint({sources:[]})}>解除来源关联</Button></>}

   </>}
   {action==='driver'&&driver&&<><Input value={driver.name} onChange={e=>setDriver({...driver,name:e.target.value})}/><Select style={{width:'100%'}} value={driver.jointId} options={selectedMechanism?.joints.filter(j=>j.kind==='REVOLUTE'||j.kind==='PRISMATIC').map(j=>({value:j.id,label:j.name}))} onChange={jointId=>setDriver({...driver,jointId})}/></>}
   {action==='study'&&studyDraft&&<>
    <Input value={studyDraft.name} onChange={e=>setStudyDraft({...studyDraft,name:e.target.value})}/><Select style={{width:'100%'}} value={studyDraft.driverId} options={defs.drivers?.filter(d=>d.mechanismId===studyDraft.mechanismId).map(d=>({value:d.id,label:d.name}))} onChange={driverId=>{const d=defs.drivers!.find(d=>d.id===driverId)!,j=selectedMechanism!.joints.find(j=>j.id===d.jointId)!,unit=j.kind==='REVOLUTE'?'deg':'mm';setStudyDraft({...studyDraft,driverId,driverJointId:'',start:{value:0,unit},end:{value:unit==='deg'?360:30,unit}})}}/>
    <Space wrap><span>起止 {studyDraft.start.unit}</span><InputNumber value={studyDraft.start.value} onChange={v=>setStudyDraft({...studyDraft,start:{...studyDraft.start,value:v??0}})}/><InputNumber value={studyDraft.end.value} onChange={v=>setStudyDraft({...studyDraft,end:{...studyDraft.end,value:v??0}})}/><span>时长 s</span><InputNumber min={.01} value={studyDraft.durationSeconds} onChange={v=>setStudyDraft({...studyDraft,durationSeconds:v??5})}/><span>帧数</span><InputNumber min={2} max={500} precision={0} value={studyDraft.frames} onChange={v=>setStudyDraft({...studyDraft,frames:v??73})}/><span>预算 ms</span><InputNumber min={100} max={120000} value={studyDraft.budgetMs} onChange={v=>setStudyDraft({...studyDraft,budgetMs:v??60000})}/></Space>
   </>}
   {action==='trial'&&<><Select style={{width:220}} value={trialDriverId} options={defs.drivers?.filter(d=>d.mechanismId===mechanismId).map(d=>({value:d.id,label:d.name}))} onChange={setTrialDriverId}/><InputNumber value={trial} onChange={v=>setTrial(v??0)}/><span>{selectedMechanism?.joints.find(j=>j.id===defs.drivers?.find(d=>d.id===trialDriverId)?.jointId)?.zero.unit??'deg'}</span></>}
   {action==='interference'&&analysisDraft&&<>
    <Input value={analysisDraft.name} onChange={e=>setAnalysisDraft({...analysisDraft,name:e.target.value})}/><Select allowClear style={{width:'100%'}} placeholder="当前正式姿态（不关联仿真）" value={analysisDraft.studyId} options={defs.studies.map(s=>({value:s.id,label:s.name}))} onChange={studyId=>setAnalysisDraft({...analysisDraft,studyId})}/><Select mode="multiple" style={{width:'100%'}} placeholder="范围：完整 occurrence + Body；空为全部" value={analysisDraft.scope?.map(addressKey)??[]} options={geometries.map(g=>({value:addressKey(g),label:`${g.name} / ${g.occurrencePath} / ${g.bodyId}`}))} onChange={values=>setAnalysisDraft({...analysisDraft,scope:geometries.filter(g=>values.includes(addressKey(g))).map(g=>({instancePath:g.instancePath,bodyId:g.bodyId}))})}/><Space><span>间隙 mm</span><InputNumber min={0} value={quantityIn(analysisDraft.clearance,'mm')} onChange={v=>setAnalysisDraft({...analysisDraft,clearance:{value:v??0,unit:'mm'}})}/><Checkbox checked={analysisDraft.includeSameUnit} onChange={e=>setAnalysisDraft({...analysisDraft,includeSameUnit:e.target.checked})}>同刚体 Body 对</Checkbox></Space>
   </>}
   {action==='import'&&<>
    <Typography.Paragraph>只导入确认的接合提案。来源为只读引用；已有产品约束不会再次进入机构方程。</Typography.Paragraph><Select mode="multiple" style={{width:'100%'}} placeholder="选择接合提案" value={imports} options={proposals.map(j=>({value:j.id,label:`${j.name} (${j.kind})`}))} onChange={setImports}/>
    <Typography.Paragraph>明确采用补充装配关系：</Typography.Paragraph><Select mode="multiple" style={{width:'100%'}} value={supplemental} options={view.product?.constraints?.filter(c=>!c.suppressed&&!selectedMechanism?.joints.some(j=>j.sources?.some(s=>s.constraintId===c.id))).map(c=>({value:c.id,label:c.name??c.id}))} onChange={setSupplemental}/><Checkbox checked={detach} onChange={e=>setDetach(e.target.checked)}>解除本机构发布关联（保留产品约束）</Checkbox>
   </>}
  </div></CommandDialog>
 <CommandDialog id="dmu.apply" title="应用到装配：连接关系与所选帧姿态" open={active&&applyOpen} size="L" onClose={closeCommand} onConfirm={apply} confirmText="一次提交并返回装配" confirmDisabled={!plan?.ready||commandPending} confirmLoading={commandPending}>
  <Button loading={commandPending} onClick={()=>void review()}>生成／刷新转换计划</Button>
  <Typography.Paragraph>帧 {index+1}；默认保留旋转自由度。约束、正式位姿、发布映射一次可撤销提交。</Typography.Paragraph><Checkbox checked={lockAngle} onChange={e=>{setLockAngle(e.target.checked);commandEpoch.current++;setPlan(undefined)}}>锁定当前角度</Checkbox>
  {plan&&<><Typography.Paragraph>{plan.poseChanges.length} 个位姿变化 · 求解 {plan.solverStatus??'待解决冲突'} · DOF {plan.degreesOfFreedom}</Typography.Paragraph>{plan.poseChanges.length>0&&<ul>{plan.poseChanges.map(id=>{const unit=units.find(v=>v.id===id),target=frame?.unitPoses[id];return <li key={id}>{unit?.name??id}：位置 mm ({unit?.translation.map(v=>v.toFixed(3)).join(', ')}) → ({target?.translation.map(v=>v.toFixed(3)).join(', ')})；方向采用所选完整帧</li>})}</ul>}<Table size="small" pagination={false} rowKey={v=>JSON.stringify([v.jointId,v.constraintId,v.role,v.action])} dataSource={plan.items} columns={[{title:'关系',dataIndex:'role',render:(role:string,item)=>`${relationLabels[role]??role} · ${view.product?.constraints?.find(c=>c.id===item.constraintId)?.name??defs.mechanisms.flatMap(m=>m.joints).find(j=>j.id===item.jointId)?.name??item.constraintId??''}`},{title:'动作',dataIndex:'action',render:(value:string)=>actionLabels[value]??value},{title:'说明',dataIndex:'detail'},{title:'冲突处理',render:(_,item)=>item.action==='CONFLICT'&&item.constraintId?<Select allowClear style={{width:150}} value={resolutions[item.constraintId]} options={[...(item.role==='existing'?[{value:'SUPPRESS',label:'明确停用'}]:[]),{value:'REPLACE',label:'采用接合替换'}]} onChange={value=>{setResolutions(r=>{const next={...r};if(value)next[item.constraintId]=value;else delete next[item.constraintId];return next});setPlan(p=>p?{...p,ready:false}:undefined)}}/>:null}]}/></>}
 </CommandDialog></>;
});
