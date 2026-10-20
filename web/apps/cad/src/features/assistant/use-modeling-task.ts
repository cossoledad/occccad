import {useEffect,useRef,useState} from "react";
import {useQuery,useQueryClient} from "@tanstack/react-query";
import type {CommandPreview} from "../../types";
import {automationApi,type AutomationTask,type EntityRef} from "./automation-api";
import {displayAcknowledgment,newerTask,pendingStep} from "./task-model";

export type ModelingProps={documentId:string;documentName:string;documentType?:string;workingBodyId?:string;enabled:boolean;displayed?:{documentId:string;revisionId:string};prepare:()=>Promise<void>;refresh:()=>Promise<void>;preview:(value:CommandPreview|undefined)=>void;executeCommand?:(id:string,targetId?:string)=>Promise<void>;locate:(ref:EntityRef)=>void};
export const messageOf=(cause:unknown)=>cause instanceof Error?cause.message:String(cause);

export function useModelingTask(props:ModelingProps,open:boolean){
  const client=useQueryClient();
  const [taskId,setTaskId]=useState<string>(),[busy,setBusy]=useState(false),[automatic,setAutomatic]=useState(true),[running,setRunning]=useState(false),[error,setError]=useState<string>();
  const current=useRef(props);current.current=props;
  const alive=useRef(true),activeWork=useRef<Promise<void>|undefined>(undefined),requestedDisplay=useRef<string|undefined>(undefined);
  const controls=useRef<Promise<void>>(Promise.resolve());
  const key=["automation-task",props.documentId,taskId];
  const query=useQuery({queryKey:key,queryFn:async()=>newerTask(client.getQueryData<AutomationTask>(key),await automationApi.task(props.documentId,taskId!)),enabled:open&&Boolean(taskId),refetchInterval:q=>q.state.data?.status==="ACTIVE"?1500:false});
  const task=query.data,step=task?pendingStep(task):undefined;
  const taskRef=useRef(task);taskRef.current=task;
  const apply=(value:AutomationTask)=>{
    if(!alive.current||value.documentId!==current.current.documentId)return;
    client.setQueryData<AutomationTask>(["automation-task",value.documentId,value.id],previous=>newerTask(previous,value));
  };
  const work=(action:()=>Promise<void>)=>{
    if(activeWork.current)return activeWork.current;
    setBusy(true);setError(undefined);
    const pending=action().catch(async cause=>{
      if(alive.current){setRunning(false);setError(messageOf(cause));const value=taskRef.current;
        if(value)try{apply(await automationApi.task(value.documentId,value.id))}catch{/* Preserve the original failure. */}}
    }).finally(()=>{activeWork.current=undefined;if(alive.current)setBusy(false)});
    activeWork.current=pending;return pending;
  };
  useEffect(()=>{alive.current=true;return()=>{alive.current=false;current.current.preview(undefined)}},[]);
  useEffect(()=>{
    const value=open&&step?.phase==="PREVIEW_READY"?step.preview:undefined;
    props.preview(value);return()=>props.preview(undefined);
  },[open,step?.id,step?.phase,step?.preview?.previewId]);
  const advance=()=>work(async()=>{
    const value=taskRef.current;if(!value)return;
    const step=pendingStep(value);
    if(step?.operationId.startsWith("ui.")){
      setRunning(false);
      if(!current.current.executeCommand)throw new Error("当前界面不能打开这个工具");
      // Journal the handoff before dispatch. Lost acknowledgments never replay
      // a potentially mutating UI command; a reload only restores its record.
      const handed=await automationApi.control(value.documentId,value,"handoff",step.id);apply(handed);
      try {await current.current.executeCommand(step.operationId.slice(3),typeof step.arguments.targetId==="string"?step.arguments.targetId:undefined)}
      catch(cause){apply(await automationApi.control(value.documentId,handed,"ui_failed",step.id));throw cause}
      apply(await automationApi.control(value.documentId,handed,"ui_opened",step.id));
      return;
    }
    await current.current.prepare();const next=await automationApi.advance(value.documentId,value);apply(next);
    if(alive.current&&pendingStep(next)?.phase==="COMMITTED"){current.current.preview(undefined);await current.current.refresh()}
  });
  useEffect(()=>{
    const revision=step?.result?.receipt.revisionId;
    if(!open||busy||step?.phase!=="COMMITTED"||!revision||displayAcknowledgment(task!,props.displayed)||requestedDisplay.current===revision)return;
    requestedDisplay.current=revision;
    void work(async()=>{current.current.preview(undefined);await current.current.refresh()});
  },[open,task?.version,busy,props.displayed?.revisionId]);
  useEffect(()=>{
    if(!task||busy)return;const ack=displayAcknowledgment(task,props.displayed);if(!ack)return;
    void work(async()=>{apply(await automationApi.control(task.documentId,task,"displayed",ack.stepId,ack.revisionId))});
  },[task?.version,props.displayed?.documentId,props.displayed?.revisionId,busy]);
  useEffect(()=>{
    if(!running||!automatic||busy||!props.enabled||!open||document.hidden||task?.status!=="ACTIVE"||!step||!["PLANNED","PREVIEW_READY"].includes(step.phase))return;
    const frame=requestAnimationFrame(()=>void advance());return()=>cancelAnimationFrame(frame);
  },[running,automatic,busy,props.enabled,open,task?.version]);
  const control=(action:"pause"|"resume"|"stop")=>{
    setRunning(false);
    // Preserve every control intent, including Stop queued while Pause is
    // waiting for an in-flight commit/display acknowledgment.
    const queued=controls.current.then(async()=>{
      while(activeWork.current)await activeWork.current;
      return work(async()=>{
      const value=taskRef.current;if(!value)return;
      const fresh=await automationApi.task(value.documentId,value.id),context=action==="resume"?await automationApi.context(value.documentId):undefined;
      apply(await automationApi.control(value.documentId,fresh,action,undefined,undefined,context?{workspaceId:context.snapshot.workspaceId,revisionId:context.snapshot.revisionId,sequence:context.snapshot.sequence}:undefined));
      if(!alive.current)return;
      if(action==="resume"){await current.current.refresh();setRunning(automatic)}else current.current.preview(undefined);
      });
    });
    controls.current=queued.catch(()=>undefined);
    return queued;
  };
  useEffect(()=>{
    const pause=()=>{if(document.hidden){setRunning(false);if(taskRef.current?.status==="ACTIVE")void control("pause")}};
    document.addEventListener("visibilitychange",pause);return()=>document.removeEventListener("visibilitychange",pause);
  },[]);
  return {taskId,setTaskId,task,step,busy,loading:Boolean(taskId)&&query.isPending,automatic,setAutomatic,running,setRunning,error:error??query.error?.message,setError,advance,control,
    select:(id:string,start=false)=>{setTaskId(id);setRunning(start);requestedDisplay.current=undefined},
    close:()=>{setRunning(false);props.preview(undefined);if(taskRef.current?.status==="ACTIVE")void control("pause")},
    retryDisplay:()=>work(async()=>{requestedDisplay.current=undefined;await current.current.refresh()})};
}
