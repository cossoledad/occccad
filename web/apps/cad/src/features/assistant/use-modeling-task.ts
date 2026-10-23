import {useEffect,useRef,useState} from "react";
import {useQuery,useQueryClient} from "@tanstack/react-query";
import type {CommandPreview} from "../../types";
import {automationApi,type EntityRef} from "./automation-api";
import {assistantApi,type AssistantRun,type FrozenScope,type RunControl} from "./assistant-api";
import {newerRun,presentedAcknowledgment,runActive} from "./conversation-model";
import {handoffDispatchAllowed,type AssistantToolHandoff} from "./workbench-handoff";

export type ModelingProps={documentId:string;documentName:string;documentType?:string;workingBodyId?:string;enabled:boolean;readOnly?:boolean;selectionCount:number;selectionSummary?:string;inOccurrence?:boolean;captureScope:()=>Promise<FrozenScope>;displayed?:{documentId:string;revisionId:string};prepare:()=>Promise<void>;refresh:()=>Promise<void>;preview:(value:CommandPreview|undefined)=>void;executeCommand?:(id:string,targetId?:string,handoff?:AssistantToolHandoff)=>Promise<void>;locate:(ref:EntityRef)=>void};
export const messageOf=(cause:unknown)=>cause instanceof Error?cause.message:String(cause);

// This bridge observes the durable server runner. Closing or unmounting it
// clears visual previews only; there is deliberately no browser advance loop.
export function useModelingTask(props:ModelingProps,open:boolean){
 const client=useQueryClient(),[runId,setRunId]=useState<string>(),[busy,setBusy]=useState(false),[error,setError]=useState<string>(),[displayError,setDisplayError]=useState<string>();
 const current=useRef(props);current.current=props;
 const alive=useRef(true),work=useRef<Promise<void>|undefined>(undefined),controlQueue=useRef(Promise.resolve()),pendingCount=useRef(0),requestedDisplay=useRef<string|undefined>(undefined),acknowledged=useRef<string|undefined>(undefined);
 const key=["assistant-run",props.documentId,runId];
 const query=useQuery({queryKey:key,queryFn:async()=>newerRun(client.getQueryData<AssistantRun>(key),await assistantApi.run(props.documentId,runId!)),enabled:open&&Boolean(runId),refetchInterval:q=>q.state.data&&runActive(q.state.data)?1000:false});
 const run=query.data,runRef=useRef(run);runRef.current=run;
 const taskQuery=useQuery({queryKey:["automation-task",props.documentId,run?.activeTaskId],queryFn:()=>automationApi.task(props.documentId,run!.activeTaskId!),enabled:open&&Boolean(run?.activeTaskId),refetchInterval:run&&runActive(run)?1000:false});
 const task=taskQuery.data,step=task?.steps.find(value=>["PLANNED","PREPARING","PREVIEW_READY","COMMITTING","HANDED_OFF","UI_OPENED","UI_UNKNOWN","UI_FAILED"].includes(value.phase));
 const apply=(next:AssistantRun)=>{
  if(!alive.current||next.documentId!==current.current.documentId)return;
  client.setQueryData<AssistantRun>(["assistant-run",next.documentId,next.id],previous=>newerRun(previous,next));
  void client.invalidateQueries({queryKey:["assistant-runs",next.documentId]});
 };
 const tracked=(operation:()=>Promise<void>)=>{
  pendingCount.current++;setBusy(true);setError(undefined);
  return operation().catch(cause=>{if(alive.current){setError(messageOf(cause));void query.refetch();void taskQuery.refetch()}}).finally(()=>{pendingCount.current--;if(alive.current)setBusy(pendingCount.current>0)});
 };
 const act=(operation:()=>Promise<void>)=>{
  if(work.current)return work.current;
  const pending=tracked(operation).finally(()=>{work.current=undefined});work.current=pending;return pending;
 };
 const control=(input:Omit<RunControl,"expectedVersion"|"epoch">)=>{
  // Explicit controls are never discarded behind an in-flight UI open. In
  // particular Stop may retire that claim before dispatch reaches the registry.
  const queued=controlQueue.current.then(()=>tracked(async()=>{
  const value=runRef.current;if(!value)return;
  // General controls apply to this Run; confirmation stays tied to the exact
  // epoch/candidate that the user saw, so it never adopts a newer preview.
  const fresh=input.action==="confirm"?value:await assistantApi.run(value.documentId,value.id);
  const task=input.action==="handoff_finish"&&input.taskId?await automationApi.task(fresh.documentId,input.taskId):undefined;
  const handoff=task?.steps.find(step=>step.id===input.stepId)?.handoff;
  apply(await assistantApi.control(fresh.documentId,fresh.id,{...input,taskVersion:task?.version??input.taskVersion,handoffId:handoff?.id??input.handoffId,expectedVersion:fresh.version,epoch:fresh.epoch}));
  void taskQuery.refetch();
  }));controlQueue.current=queued.catch(()=>undefined);return queued;
 };
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;current.current.preview(undefined)}},[]);
 useEffect(()=>{
  const value=open&&run?.mode==="CONFIRM"&&run.status==="WAITING_CONFIRMATION"&&step?.phase==="PREVIEW_READY"?step.preview:undefined;
  props.preview(value);return()=>props.preview(undefined);
 },[open,run?.epoch,run?.status,run?.mode,step?.id,step?.phase,step?.preview?.previewId]);
 const refreshDisplay=async()=>{
  setDisplayError(undefined);
  try{await current.current.refresh()}catch(cause){if(alive.current)setDisplayError(`模型已应用，视图加载失败：${messageOf(cause)}`)}
 };
 useEffect(()=>{
  if(!open||!run?.latestRevisionId)return;
  const identity=`${run.documentId}:${run.latestRevisionId}`;
  if(props.displayed?.documentId===run.documentId&&props.displayed.revisionId===run.latestRevisionId||requestedDisplay.current===identity)return;
  requestedDisplay.current=identity;void refreshDisplay();
 },[open,run?.documentId,run?.latestRevisionId,props.displayed?.documentId,props.displayed?.revisionId]);
 useEffect(()=>{
  if(!open||!run)return;const revisionId=presentedAcknowledgment(run,props.displayed);if(!revisionId)return;
  const identity=`${run.id}:${revisionId}`;if(acknowledged.current===identity)return;
  acknowledged.current=identity;
  void assistantApi.control(run.documentId,run.id,{action:"presented",expectedVersion:run.version,epoch:run.epoch,revisionId}).then(apply).catch(()=>{acknowledged.current=undefined;void query.refetch()});
 },[open,run?.version,props.displayed?.documentId,props.displayed?.revisionId]);
 const confirm=()=>{
  if(!run||!task||step?.phase!=="PREVIEW_READY"||!step.preview?.previewId)return Promise.resolve();
  return control({action:"confirm",taskId:task.id,taskVersion:task.version,stepId:step.id,previewId:step.preview.previewId});
 };
 const openTool=()=>act(async()=>{
  const value=runRef.current;if(!value||!task||!step?.operationId.startsWith("ui.")||step.phase!=="PLANNED")return;
  if(!current.current.executeCommand)throw new Error("当前界面无法打开这个工具");
  const next=await assistantApi.control(value.documentId,value.id,{action:"handoff_start",expectedVersion:value.version,epoch:value.epoch,taskId:task.id,taskVersion:task.version,stepId:step.id});apply(next);
  let outcome:"UI_OPENED"|"FAILED"="UI_OPENED";
  try{
   const persisted=await automationApi.task(value.documentId,task.id),claimed=persisted.steps.find(item=>item.id===step.id),handoff=claimed?.handoff;
   if(claimed?.phase!=="HANDED_OFF"||!handoff||handoff.planEpoch!==next.epoch)throw new Error("工具接管已记录，打开状态需要核对；请勿重复打开");
   await current.current.prepare();
   const bridge:AssistantToolHandoff={runId:next.id,taskId:task.id,stepId:step.id,epoch:next.epoch,handoffId:handoff.id,domainRequestId:handoff.domainRequestId,documentId:value.documentId,baselineRevisionId:handoff.baseline.revisionId,uiCommand:handoff.uiCommand};
   const live=await assistantApi.run(value.documentId,value.id);
   if(!handoffDispatchAllowed(live,bridge))throw new Error("任务或接管目标已变化，这次工具没有打开");
   await current.current.executeCommand(step.operationId.slice(3),typeof step.arguments.targetId==="string"?step.arguments.targetId:undefined,bridge);
  }catch(cause){outcome="FAILED";setError(messageOf(cause))}
  // A lost response leaves the durable claim in place. Polling/reload never
  // dispatches it again, and UI_OPENED does not certify a geometry result.
  const fresh=await assistantApi.run(value.documentId,value.id);
  const openedTask=await automationApi.task(value.documentId,task.id);
  apply(await assistantApi.control(value.documentId,value.id,{action:"handoff_finish",expectedVersion:fresh.version,epoch:next.epoch,taskId:task.id,taskVersion:openedTask.version,stepId:step.id,handoffId:openedTask.steps.find(item=>item.id===step.id)?.handoff?.id,outcome}));
  void taskQuery.refetch();
 });
 return {runId,run,task,step,busy,loading:Boolean(runId)&&query.isPending,error:error??query.error?.message??taskQuery.error?.message,displayError,setError,apply,control,confirm,openTool,
  select:(id?:string)=>{setRunId(id);requestedDisplay.current=undefined;acknowledged.current=undefined;setError(undefined);setDisplayError(undefined)},
  retryDisplay:refreshDisplay};
}
