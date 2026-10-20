import {useEffect,useRef,useState} from "react";
import {useQuery,useQueryClient} from "@tanstack/react-query";
import {Alert,Button,Checkbox,InputNumber,Space,Typography} from "antd";
import {RobotOutlined,CloseOutlined} from "@ant-design/icons";
import type {CommandPreview} from "../../types";
import {automationApi,type AutomationTask,type EntityRef} from "./automation-api";
import {displayAcknowledgment,newerTask,pendingStep,phaseText,rectangularExtrudePlan} from "./task-model";
import "./assistant-panel.css";

type Props={documentId:string;documentName:string;workingBodyId?:string;enabled:boolean;displayed?:{documentId:string;revisionId:string};prepare:()=>Promise<void>;refresh:()=>Promise<void>;preview:(value:CommandPreview|undefined)=>void;locate:(ref:EntityRef)=>void};
export function AssistantPanel(props:Props){
  const client=useQueryClient(),[open,setOpen]=useState(false),[taskId,setTaskId]=useState<string>(),[busy,setBusy]=useState(false),[automatic,setAutomatic]=useState(false),[running,setRunning]=useState(false),[error,setError]=useState<string>();
  const [width,setWidth]=useState<number|null>(80),[height,setHeight]=useState<number|null>(50),[length,setLength]=useState<number|null>(12);
  const current=useRef(props);current.current=props;
  const alive=useRef(true),activeWork=useRef<Promise<void>|undefined>(undefined),creation=useRef<ReturnType<typeof rectangularExtrudePlan>|undefined>(undefined),requestedDisplay=useRef<string|undefined>(undefined);
  const key=["automation-task",props.documentId,taskId];
  const taskQuery=useQuery({queryKey:key,queryFn:async()=>newerTask(client.getQueryData<AutomationTask>(key),await automationApi.task(props.documentId,taskId!)),enabled:open&&Boolean(taskId),refetchInterval:query=>query.state.data?.status==="ACTIVE"?1500:false});
  const operations=useQuery({queryKey:["automation-operations"],queryFn:automationApi.operations,enabled:open,staleTime:60_000});
  const task=taskQuery.data,step=task?pendingStep(task):undefined;
  const taskRef=useRef(task);taskRef.current=task;
  const apply=(value:AutomationTask)=>{if(!alive.current||value.documentId!==current.current.documentId)return;client.setQueryData<AutomationTask>(["automation-task",value.documentId,value.id],previous=>newerTask(previous,value));};
  const work=(action:()=>Promise<void>)=>{
    if(activeWork.current)return activeWork.current;
    setBusy(true);setError(undefined);
    const pending=action().catch(async cause=>{if(alive.current){setRunning(false);setError(cause instanceof Error?cause.message:String(cause));const value=taskRef.current;if(value)try{apply(await automationApi.task(value.documentId,value.id))}catch{/* Preserve the original error and request identity. */}}}).finally(()=>{activeWork.current=undefined;if(alive.current)setBusy(false)});
    activeWork.current=pending;return pending;
  };
  useEffect(()=>{alive.current=true;return()=>{alive.current=false;current.current.preview(undefined)}},[]);
  useEffect(()=>{setTaskId(undefined);setRunning(false);creation.current=undefined;setError(undefined)},[props.documentId]);
  useEffect(()=>{if(!open||taskId)return;let disposed=false;void automationApi.tasks(props.documentId).then(result=>{if(!disposed&&result.tasks[0])setTaskId(result.tasks[0].id)}).catch(cause=>{if(!disposed)setError(String(cause))});return()=>{disposed=true}},[open,taskId,props.documentId]);
  useEffect(()=>{const value=step?.phase==="PREVIEW_READY"?step.preview:undefined;props.preview(value);return()=>props.preview(undefined)},[step?.id,step?.phase,step?.preview?.previewId]);
  const advance=()=>work(async()=>{
    const value=taskRef.current;if(!value)return;await current.current.prepare();const next=await automationApi.advance(value.documentId,value);apply(next);
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
    // Give the committed render acknowledgment and controls an event boundary.
    const frame=requestAnimationFrame(()=>void advance());return()=>cancelAnimationFrame(frame);
  },[running,automatic,busy,props.enabled,open,task?.version]);
  const control=async(action:"pause"|"resume"|"stop")=>{
    setRunning(false);await activeWork.current;
    return work(async()=>{const value=taskRef.current;if(!value)return;const fresh=await automationApi.task(value.documentId,value.id);const context=action==="resume"?await automationApi.context(value.documentId):undefined;apply(await automationApi.control(value.documentId,fresh,action,undefined,undefined,context?{workspaceId:context.snapshot.workspaceId,revisionId:context.snapshot.revisionId,sequence:context.snapshot.sequence}:undefined));if(!alive.current)return;if(action==="resume"){await current.current.refresh();setRunning(automatic)}else current.current.preview(undefined)});
  };
  useEffect(()=>{const pause=()=>{if(document.hidden){setRunning(false);if(taskRef.current?.status==="ACTIVE")void control("pause")}};document.addEventListener("visibilitychange",pause);return()=>document.removeEventListener("visibilitychange",pause)},[]);
  const create=()=>work(async()=>{
    await current.current.prepare();if(!creation.current){const context=await automationApi.context(props.documentId);creation.current=rectangularExtrudePlan({...context,workingBodyId:current.current.workingBodyId??context.workingBodyId},width!,height!,length!)}
    const value=await automationApi.create(props.documentId,creation.current);if(!alive.current)return;apply(value);setTaskId(value.id);creation.current=undefined;setRunning(automatic);
  });
  const close=()=>{setRunning(false);setOpen(false);props.preview(undefined);if(task?.status==="ACTIVE")void control("pause")};
  if(!open)return <Button className="assistant-launcher" aria-label="建模助手" icon={<RobotOutlined/>} onClick={()=>setOpen(true)}>建模助手</Button>;
  return <aside className="assistant-panel" aria-label="建模助手" onKeyDown={event=>event.stopPropagation()}>
    <header><strong>建模助手</strong><Button type="text" size="small" aria-label="收起建模助手" icon={<CloseOutlined/>} onClick={close}/></header>
    <Typography.Text type="secondary">{props.documentName} · AI 尚未连接，可直接运行建模流程</Typography.Text>
    {!props.enabled&&<Alert type="info" title="请结束当前编辑后，在独立零件中运行流程"/>}
    <Space wrap><label>宽度 mm <InputNumber aria-label="流程宽度" value={width} min={0.001} max={1e6} onChange={setWidth}/></label><label>高度 mm <InputNumber aria-label="流程高度" value={height} min={0.001} max={1e6} onChange={setHeight}/></label><label>拉伸 mm <InputNumber aria-label="流程拉伸长度" value={length} min={0.001} max={1e6} onChange={setLength}/></label></Space>
    <Checkbox checked={automatic} onChange={event=>{setAutomatic(event.target.checked);if(!event.target.checked)setRunning(false)}}>自动逐步执行</Checkbox>
    <Button type="primary" disabled={!props.enabled||busy||task?.status==="ACTIVE"||!width||!height||!length} onClick={()=>void create()} loading={busy&&!task}>创建矩形拉伸流程</Button>
    {task&&<><Typography.Paragraph className="assistant-goal">{task.goal}</Typography.Paragraph><ol className="assistant-steps">{task.steps.map(item=><li key={item.id} data-phase={item.phase}><div><strong>{operations.data?.operations.find(op=>op.id===item.operationId)?.name??item.operationId}</strong><span>{phaseText[item.phase]}</span></div>{item.result?.output&&<Button type="link" size="small" onClick={()=>props.locate(item.result!.output!)}>定位</Button>}{item.failure&&<Alert type="error" title={item.failure.message}/>}</li>)}</ol>
      <Space wrap>
        {step?.phase==="COMMITTED"&&<Button disabled={busy} onClick={()=>void work(async()=>{requestedDisplay.current=undefined;await current.current.refresh()})}>重新显示结果</Button>}
        {task.status==="ACTIVE"&&<><Button disabled={!props.enabled||busy||step?.phase==="COMMITTED"} loading={busy} onClick={()=>{setRunning(automatic);void advance()}}>{step?.phase==="PREVIEW_READY"?"确认这一步":"执行下一步"}</Button><Button onClick={()=>void control("pause")}>暂停 / 接管</Button><Button onClick={()=>void control("stop")}>停止</Button></>}
        {task.status==="PAUSED"&&<Button disabled={!props.enabled||busy} onClick={()=>void control("resume")}>基于当前状态继续</Button>}
        {task.status==="COMPLETED"&&<Typography.Text type="success">流程完成</Typography.Text>}{task.status==="STOPPED"&&<Typography.Text>流程已停止，已提交步骤保留</Typography.Text>}
      </Space></>}
    {error&&<Alert type="error" title={error}/>} {taskQuery.error&&<Alert type="error" title={taskQuery.error.message}/>}
  </aside>;
}
