import {useEffect,useRef,useState} from "react";
import {useQuery,useQueryClient} from "@tanstack/react-query";
import {Alert,Button,Checkbox,Input,InputNumber,Popover,Select,Spin,Tooltip} from "antd";
import {CheckOutlined,CloseOutlined,CommentOutlined,PauseOutlined,SendOutlined,SettingOutlined,StopOutlined} from "@ant-design/icons";
import {randomUUID} from "../../utils/random-uuid";
import {automationApi,type AutomationTask,type TaskStep} from "./automation-api";
import {assistantApi,type AssistantRun,type AssistantSettings,type MessageInput,type RunEvent,type RunInput,type RunMode,type Thinking} from "./assistant-api";
import {phaseText} from "./task-model";
import {assistantFailure,conversationProgress,mergeMessagePage,mergeProgressPage,runActive,runStatusText,turnStatusText} from "./conversation-model";
import {scopeSummary} from "./frozen-scope";
import {messageOf,useModelingTask,type ModelingProps} from "./use-modeling-task";
import {ExportMenu,UsageDetails} from "./assistant-usage";
import {AssistantTrace} from "./assistant-trace";
import "./assistant-panel.css";

const tokenText=(value:number)=>value>=10000?`${Number((value/10000).toFixed(2))}万`:value.toLocaleString("zh-CN");
type Pending={runId?:string;input:RunInput|MessageInput};
function stepDetail(step:TaskStep):string{
 const a=step.arguments;
 if(step.operationId==="sketch.create")return `${a.plane} 平面`;
 if(step.operationId==="sketch.rectangle")return `${a.width} × ${a.height} ${a.unit}`;
 if(["part.fillet","part.chamfer"].includes(step.operationId))return `${step.operationId==="part.fillet"?"R":"距离 "}${a.size} ${a.unit}`;
 if(step.operationId==="part.extrude")return `${a.length} ${a.unit}`;
 if(step.operationId==="sketch.circle")return `直径 ${a.diameter} ${a.unit}`;
 if(step.operationId==="part.cut")return a.extent==="THROUGH_ALL"?"贯穿所有":`${a.length} ${a.unit}`;
 if(step.operationId==="parameter.set_literal")return `${a.value} ${a.unit}`;
 if(step.operationId==="parameter.set_expression")return String(a.expression);
 return "";
}
export function AssistantPanel(props:ModelingProps&{open:boolean;onClose:()=>void}){
 const {open}=props,client=useQueryClient();
 const [settingsOpen,setSettingsOpen]=useState(false),[apiKey,setApiKey]=useState(""),[keyBusy,setKeyBusy]=useState(false),[preferenceBusy,setPreferenceBusy]=useState(false);
 const [draft,setDraft]=useState(""),[sending,setSending]=useState(false),[error,setError]=useState<string>(),[optimistic,setOptimistic]=useState<Pending>(),[retry,setRetry]=useState<Pending>();
 const [newGoal,setNewGoal]=useState(false),[mode,setMode]=useState<RunMode>("AUTOMATIC"),[runCursor,setRunCursor]=useState<string>(),[historyBefore,setHistoryBefore]=useState<number>(),[stageBefore,setStageBefore]=useState<number>();
 const [eventsOpen,setEventsOpen]=useState(false);
 const model=useModelingTask(props,open),{run,task,step,busy}=model;
 const alive=useRef(true),end=useRef<HTMLDivElement>(null),followProgress=useRef(true);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false}},[]);
 const connection=useQuery({queryKey:["assistant-settings"],queryFn:assistantApi.settings,enabled:open,staleTime:30_000});
 const models=useQuery({queryKey:["assistant-models"],queryFn:assistantApi.models,enabled:open&&connection.data?.configured===true,staleTime:60_000});
 const runs=useQuery({queryKey:["assistant-runs",props.documentId,runCursor],queryFn:()=>assistantApi.runs(props.documentId,runCursor),enabled:open});
 useEffect(()=>{if(open&&!model.runId&&!newGoal&&runs.data?.items[0])model.select(runs.data.items[0].id)},[open,model.runId,newGoal,runs.data]);
 const messageKey=["assistant-messages",props.documentId,model.runId,historyBefore];
 const conversation=useQuery({queryKey:messageKey,queryFn:async()=>{
  const next=await assistantApi.messages(props.documentId,model.runId!,{before:historyBefore,limit:30});
  return historyBefore===undefined?mergeMessagePage(client.getQueryData(messageKey),next):next;
 },enabled:open&&Boolean(model.runId),refetchInterval:query=>historyBefore===undefined&&(run&&runActive(run)||query.state.data?.items.some(turn=>["RECEIVED","WAITING_BOUNDARY"].includes(turn.status)))?1000:false});
 const stages=useQuery({queryKey:["assistant-stages",props.documentId,model.runId,stageBefore],queryFn:()=>assistantApi.stages(props.documentId,model.runId!,{before:stageBefore,limit:20}),enabled:open&&Boolean(model.runId),refetchInterval:stageBefore===undefined&&run&&runActive(run)?1500:false});
 const progressKey=["assistant-progress",props.documentId,model.runId];
 const progress=useQuery({queryKey:progressKey,queryFn:async()=>{
  const previous=client.getQueryData<Awaited<ReturnType<typeof assistantApi.events>>>(progressKey);
  const next=await assistantApi.events(props.documentId,model.runId!,{after:previous?.nextAfter||undefined,limit:100});
  return mergeProgressPage(previous,next);
 },enabled:open&&Boolean(model.runId),refetchInterval:run&&["QUEUED","PLANNING","EXECUTING","WAITING_RETRY"].includes(run.status)?1000:false});
 useEffect(()=>{if(open&&model.runId)void client.invalidateQueries({queryKey:progressKey})},[open,model.runId,run?.eventSequence]);
 const operations=useQuery({queryKey:["automation-operations"],queryFn:automationApi.operations,enabled:open,staleTime:60_000});
 useEffect(()=>{setHistoryBefore(undefined);setStageBefore(undefined)},[model.runId]);
 useEffect(()=>{void client.invalidateQueries({queryKey:["assistant-messages",props.documentId,model.runId]})},[run?.version]);
 useEffect(()=>{if(historyBefore===undefined&&followProgress.current)end.current?.scrollIntoView({block:"nearest"})},[conversation.data?.nextAfter,progress.data?.nextAfter,run?.reply,optimistic,settingsOpen]);
 useEffect(()=>{if(!open)setApiKey("")},[open]);
 const saveKey=async()=>{
  setKeyBusy(true);setError(undefined);
  try{client.setQueryData(["assistant-settings"],await assistantApi.saveKey(apiKey));setApiKey("");setSettingsOpen(false);void client.invalidateQueries({queryKey:["assistant-models"]})}catch(cause){setError(messageOf(cause))}finally{setKeyBusy(false)}
 };
 const savePreferences=async(preferences:{model?:string;thinking?:Thinking;tokenBudget?:number|null})=>{
  setPreferenceBusy(true);setError(undefined);
  try{client.setQueryData(["assistant-settings"],await assistantApi.savePreferences(preferences))}catch(cause){setError(messageOf(cause))}finally{setPreferenceBusy(false)}
 };
 const send=async(previous?:Pending)=>{
  if(sending||!connection.data?.configured||!previous&&!draft.trim())return;
  setSending(true);setError(undefined);model.setError(undefined);setRetry(undefined);
  let pending=previous;
  try{
   if(!pending){
    const text=draft.trim(),scope=await props.captureScope();
    const input={requestId:randomUUID(),text,scope};
    pending=model.runId?{runId:model.runId,input}:{input:{...input,workingBodyId:scope.workingBodyId,mode,tokenBudget:connection.data.tokenBudget}};
   }
   followProgress.current=true;setOptimistic(pending);
   const result=pending.runId?(await assistantApi.send(props.documentId,pending.runId,pending.input)).run:await assistantApi.create(props.documentId,pending.input as RunInput);
   if(!alive.current)return;
   model.apply(result);if(model.runId!==result.id)model.select(result.id);
   setNewGoal(false);setDraft("");setHistoryBefore(undefined);
   void client.invalidateQueries({queryKey:["assistant-messages",props.documentId,result.id]});
  }catch(cause){if(alive.current){setError(messageOf(cause));if(pending)setRetry(pending)}}
  finally{if(alive.current){setSending(false);setOptimistic(undefined)}}
 };
 const taskList=()=>task&&<TaskSteps steps={task.steps} status={task.status} names={operations.data?.operations??[]} locate={props.locate}/>;
 if(!open)return null;
 const showConnection=settingsOpen||connection.data?.configured===false,turns=conversation.data?.items??[];
 const navigationRuns=run?[run,...(runs.data?.items??[]).filter(item=>item.id!==run.id)]:runs.data?.items??[];
 const optimisticVisible=optimistic&&!turns.some(turn=>turn.requestId===optimistic.input.requestId);
 const canControl=!busy;
 return <aside className="assistant-panel" aria-label="建模助手" onKeyDown={event=>event.stopPropagation()} onPointerDown={event=>event.stopPropagation()}>
  <header className="assistant-header"><div className="assistant-heading"><CommentOutlined/><strong>建模助手</strong><Tooltip title={connection.data?.configured?`已连接 ${connection.data.provider}`:"未连接"}><span aria-label={connection.data?.configured?"已连接":"未连接"} className={`assistant-connection ${connection.data?.configured?"is-connected":""}`}/></Tooltip></div><div><Tooltip title="连接设置"><Button type="text" size="small" aria-label="连接设置" icon={<SettingOutlined/>} onClick={()=>{setSettingsOpen(value=>!value);setApiKey("");setError(undefined)}}/></Tooltip><Button type="text" size="small" aria-label="收起建模助手" icon={<CloseOutlined/>} onClick={props.onClose}/></div></header>
  <div className="assistant-document" title={props.inOccurrence?"正在编辑共享零件定义":undefined}><span className="assistant-document-dot"/>{props.documentName}{props.readOnly&&<span>只读</span>}</div>
  {showConnection?<div className="assistant-connect">
   <div className="assistant-connect-icon"><CommentOutlined/></div><h3>{connection.data?.configured?"连接设置":"用对话开始建模"}</h3><p>连接 DeepSeek，直接描述目标与尺寸。</p>
   <label htmlFor="assistant-api-key">DeepSeek API Key</label><Input.Password id="assistant-api-key" aria-label="API Key" value={apiKey} autoComplete="off" placeholder={connection.data?.configured?"输入新的 API Key 以替换":"粘贴你的 API Key"} onChange={event=>setApiKey(event.target.value)} onPressEnter={()=>{if(apiKey)void saveKey()}}/>
   <small>密钥按账号加密保存在服务端，保存后不会回显。</small><Button type="primary" block loading={keyBusy} disabled={!apiKey.trim()} onClick={()=>void saveKey()}>保存并连接</Button><a href="https://platform.deepseek.com/api_keys" target="_blank" rel="noreferrer">获取 DeepSeek API Key ↗</a>
   {connection.data?.configured&&<div className="assistant-connection-actions"><Button size="small" onClick={()=>{setSettingsOpen(false);setApiKey("")}}>返回对话</Button><Button type="text" size="small" disabled={keyBusy} onClick={()=>void(async()=>{setKeyBusy(true);try{await assistantApi.deleteKey();await client.invalidateQueries({queryKey:["assistant-settings"]})}catch(cause){setError(messageOf(cause))}finally{setKeyBusy(false)}})()}>断开连接</Button></div>}
  </div>:<>
   <div className="assistant-run-navigation"><Select aria-label="建模目标" size="small" variant="borderless" value={model.runId} placeholder="新的建模目标" options={navigationRuns.map(item=>({value:item.id,label:`${runStatusText[item.status]} · ${item.goal}`}))} onChange={value=>{setNewGoal(false);model.select(value)}}/><Button size="small" type="text" onClick={()=>{setNewGoal(true);model.select(undefined)}}>新建目标</Button>{runs.data?.hasMore&&<Button type="text" size="small" onClick={()=>setRunCursor(runs.data!.nextCursor)}>更早目标</Button>}{runCursor&&<Button size="small" type="text" onClick={()=>setRunCursor(undefined)}>最近</Button>}</div>
   {run&&<div className="assistant-usage-header"><UsageDetails usage={run.usage} label="累计"/><ExportMenu documentId={props.documentId} runId={run.id}/></div>}
   <div className="assistant-thread" role="log" aria-label="建模对话" aria-live="polite" onScroll={event=>{const el=event.currentTarget;followProgress.current=el.scrollHeight-el.scrollTop-el.clientHeight<80}}>
    {(model.loading||conversation.isPending&&Boolean(model.runId))&&<div className="assistant-loading"><Spin size="small"/> 正在恢复任务</div>}
    {!model.runId&&!optimisticVisible&&<div className="assistant-welcome"><h3>想完成什么样的模型？</h3><p>描述目标和尺寸，也可以选中对象后提出要求。运行过程中可继续提问或修改。</p><button onClick={()=>setDraft("在 XY 平面建立 80×50 mm 矩形，拉伸 12 mm，再对全部边做 R2 圆角。")}>创建长方体并对全部边做 R2 圆角 <span>↗</span></button><button onClick={()=>setDraft("当前零件有哪些可以修改的参数？")}>查看当前零件的参数 <span>↗</span></button></div>}
    {conversation.data?.hasEarlier&&<Button className="assistant-history-page" size="small" type="link" onClick={()=>setHistoryBefore(turns[0]?.sequence)}>较早消息</Button>}{historyBefore!==undefined&&<Button size="small" type="link" onClick={()=>setHistoryBefore(undefined)}>返回最新消息</Button>}
    {turns.map(turn=><div className="assistant-exchange" key={turn.id}>
     <div className="assistant-message is-user"><span className="assistant-speaker" title={turnStatusText[turn.status]}>你</span><p>{turn.text}</p>{Boolean(turn.scope.selectionCount)&&<small className="assistant-scope-sent">{scopeSummary(turn.scope)}</small>}<div className="assistant-turn-accounting"><UsageDetails usage={turn.usage} label="本轮"/>{run&&<ExportMenu documentId={props.documentId} runId={run.id} turnId={turn.id}/>}</div></div>
     {(turn.kind==="QUERY"||turn.goalVersion!==run?.goalVersion)&&(turn.reply||turn.failure||["RECEIVED","WAITING_BOUNDARY"].includes(turn.status))&&<div className="assistant-message is-assistant"><span className="assistant-speaker"><CommentOutlined/> 建模助手</span>{turn.kind==="QUERY"&&!turn.reply&&<ProgressTimeline events={(progress.data?.items??[]).filter(event=>event.data?.inputId===turn.id&&Boolean(event.data?.message))}/>} {turn.reply&&<p>{turn.reply}</p>}{turn.failure&&<Alert type="error" title={assistantFailure(turn.failure)}/>} {turn.kind==="QUERY"&&["RECEIVED","WAITING_BOUNDARY"].includes(turn.status)&&<div className="assistant-thinking"><Spin size="small"/>正在查看…</div>}</div>}
    </div>)}
    {optimisticVisible&&<div className="assistant-message is-user"><span className="assistant-speaker">你 · 正在发送</span><p>{optimistic.input.text}</p><small>{scopeSummary(optimistic.input.scope)}</small></div>}
    {run&&<section className="assistant-run" aria-label="本次请求">
     <div className="assistant-speaker"><CommentOutlined/> 建模助手</div>
     <div className="assistant-run-title"><span data-run-status={run.status}>{["QUEUED","PLANNING","EXECUTING","WAITING_RETRY"].includes(run.status)&&<Spin size="small"/>} {runStatusText[run.status]}</span><div className="assistant-outcomes">{run.requestAppliedCount>0&&<Tooltip title={`本次请求已应用 ${run.requestAppliedCount} 个步骤；以真实提交回执为准`}><span data-outcome="applied">已应用</span></Tooltip>}{run.latestRevisionId&&run.latestRevisionId===run.presentedRevisionId&&<Tooltip title="视口已确认显示当前版本"><span data-outcome="presented">已显示</span></Tooltip>}</div></div>
     <ProgressTimeline events={run?conversationProgress(progress.data?.items??[],run):[]}/>
     {run.reply&&<p className="assistant-run-reply">{run.reply}</p>}
     {run.failure?<Alert type="warning" title={assistantFailure(run.failure)}/>:run.waitingReason&&run.waitingReason!==run.reply&&<p className="assistant-input-hint">{run.waitingReason}</p>}
     <div className="assistant-task-actions">
      {["PAUSED","WAITING_RETRY","BLOCKED"].includes(run.status)&&<Button size="small" type="primary" disabled={!canControl} onClick={()=>void model.control({action:"resume"})}>继续任务</Button>}
      {runActive(run)&&!["PAUSED","WAITING_BUDGET"].includes(run.status)&&<Button size="small" icon={<PauseOutlined/>} title="暂停后可手动接管建模" onClick={()=>void model.control({action:"pause"})}>暂停</Button>}
      {runActive(run)&&<Tooltip title="停止会保留已经提交的模型"><Button size="small" type="text" aria-label="停止建模" icon={<StopOutlined/>} onClick={()=>void model.control({action:"stop"})}/></Tooltip>}
      {run.status==="WAITING_CONFIRMATION"&&step?.phase==="PREVIEW_READY"&&<Button size="small" type="primary" disabled={!canControl||!props.enabled} onClick={()=>void model.confirm()}>确认这一步</Button>}
      {run.status==="WAITING_UI"&&step?.operationId.startsWith("ui.")&&step.phase==="PLANNED"&&<Button size="small" type="primary" disabled={!canControl||!props.enabled} onClick={()=>void model.openTool()}>打开工具</Button>}
      {run.status==="WAITING_UI"&&step&&["HANDED_OFF","UI_OPENED","UI_UNKNOWN"].includes(step.phase)&&<><span>工具操作待核对；不会自动重新打开</span><Button size="small" disabled={!canControl} onClick={()=>void model.control({action:"handoff_finish",taskId:task!.id,stepId:step.id,requestId:step.handoff?.domainRequestId,outcome:"COMPLETED"})}>核对操作结果</Button><Button size="small" disabled={!canControl} onClick={()=>void model.control({action:"handoff_finish",taskId:task!.id,stepId:step.id,outcome:"CANCELLED"})}>已取消操作</Button><Button size="small" disabled={!canControl} onClick={()=>void model.control({action:"handoff_finish",taskId:task!.id,stepId:step.id,outcome:"UNKNOWN"})}>结果待核对</Button></>}
     </div>
     {task&&<details className="assistant-task" open><summary>建模步骤</summary>{taskList()}</details>}
     {model.displayError&&<Alert type="warning" title={model.displayError}/>} {run.latestRevisionId&&run.latestRevisionId!==run.presentedRevisionId&&<Button type="link" size="small" onClick={()=>void model.retryDisplay()}>重新显示结果</Button>}
     {run.stageCount>0&&<details className="assistant-stage-history"><summary>步骤记录 · {run.stepCount}</summary>{stages.data?.hasEarlier&&<Button size="small" type="link" onClick={()=>setStageBefore(stages.data!.items[0]?.sequence)}>更早步骤</Button>}{stageBefore!==undefined&&<Button type="link" size="small" onClick={()=>setStageBefore(undefined)}>最近步骤</Button>}{stages.data?.items.filter(stage=>stage.taskId!==task?.id).map(stage=><PastTask key={stage.sequence} documentId={props.documentId} taskId={stage.taskId} sequence={stage.sequence} goalVersion={stage.goalVersion} names={operations.data?.operations??[]}/>)}{stages.isError&&<p>{stages.error.message}</p>}</details>}
     <details className="assistant-events" onToggle={event=>{if(event.target===event.currentTarget)setEventsOpen(event.currentTarget.open)}}><summary>调用详情 · {tokenText(run.usage?.total_tokens??0)} Token</summary>
      <div className="assistant-run-metrics">{run.modelCalls} 次模型调用 · {run.toolCalls} 次工具调用 · 累计已应用 {run.appliedCount} 步{run.tokenBudget!=null&&` · 预算 ${tokenText(run.tokenBudget)}`}{Boolean(run.usage?.unknownAttempts)&&` · ${run.usage.unknownAttempts} 次用量未知`}</div>

      {run.failure&&<p className="assistant-diagnostic">{run.failure.code} · {run.failure.message}</p>}
      <AssistantTrace documentId={props.documentId} runId={run.id} eventSequence={run.eventSequence} open={eventsOpen} active={["QUEUED","PLANNING","EXECUTING","WAITING_RETRY"].includes(run.status)}/>
     </details>
    </section>}<div ref={end}/>
   </div>
   <div className="assistant-composer">
    {connection.data?.configured&&<div className="assistant-model-controls">
     <Select virtual={false} size="small" variant="borderless" aria-label="AI 模型" popupMatchSelectWidth={220} value={connection.data.model} loading={models.isPending} options={models.data?.models.map(model=>({value:model.id,label:model.name??model.id}))} disabled={preferenceBusy||models.isError} onChange={value=>{const supported=models.data?.models.find(model=>model.id===value)?.effort?.supported_levels;void savePreferences({model:value,thinking:supported&&!supported.includes(connection.data!.thinking)?"off":connection.data!.thinking})}}/>
     <Tooltip title="修改只影响后续模型调用；正在处理的调用保持原设置"><Select virtual={false} size="small" variant="borderless" aria-label="思考度" value={connection.data.thinking} disabled={preferenceBusy} options={[{value:"off",label:"思考 · 关闭"},{value:"low",label:"思考 · 低"},{value:"high",label:"思考 · 高"},{value:"max",label:"思考 · 最高"}].map(option=>({...option,disabled:option.value!=="off"&&Boolean(models.data?.models.find(model=>model.id===connection.data!.model)?.effort&&!models.data.models.find(model=>model.id===connection.data!.model)!.effort!.supported_levels.includes(option.value))}))} onChange={value=>void savePreferences({thinking:value})}/></Tooltip>
     <BudgetControl settings={connection.data} run={run} busy={preferenceBusy||busy} saveDefault={savePreferences} saveRun={tokenBudget=>model.control({action:"budget",tokenBudget})}/>
    </div>}
    {props.inOccurrence&&<div className="assistant-input-hint">正在操作共享零件定义，修改会影响引用它的其他实例。</div>}
    {run?.status==="STOPPED"&&<div className="assistant-input-hint">此目标已停止，仍可查询；继续建模请新建目标。</div>}
    {(props.selectionCount>0||props.readOnly)&&<div className="assistant-scope" aria-label="发送目标摘要">{props.selectionCount?`已选 ${props.selectionSummary||`${props.selectionCount} 个对象`}`:"只读问答"}</div>}
    <Input.TextArea aria-label="建模需求" value={draft} onChange={event=>setDraft(event.target.value)} autoSize={{minRows:2,maxRows:5}} maxLength={2000} placeholder={run?"提问，或修改当前目标…":"描述目标、尺寸，或提出问题…"} onKeyDown={event=>{if(event.key==="Enter"&&!event.shiftKey&&!event.nativeEvent.isComposing){event.preventDefault();void send()}}}/>
    <div className="assistant-composer-actions"><Tooltip title="每一步先预览，确认后再应用"><Checkbox checked={(run?.mode??mode)==="CONFIRM"} disabled={busy||props.readOnly} onChange={event=>{const value:RunMode=event.target.checked?"CONFIRM":"AUTOMATIC";if(run)void model.control({action:"mode",mode:value});else setMode(value)}}>逐步确认</Checkbox></Tooltip><Button type="primary" size="small" aria-label="发送建模需求" icon={<SendOutlined/>} loading={sending} disabled={sending||!draft.trim()||!connection.data?.configured} onClick={()=>void send()}/></div>
   </div>
  </>}
  {(error||model.error||connection.error||conversation.error||runs.error)&&<div className="assistant-error"><Alert closable type="error" title={error??model.error??connection.error?.message??conversation.error?.message??runs.error?.message} onClose={()=>{setError(undefined);model.setError(undefined)}}/>{retry&&<Button size="small" disabled={sending} onClick={()=>void send(retry)}>恢复本次发送</Button>}</div>}
 </aside>;
}

function ProgressTimeline({events}:{events:RunEvent[]}){
 const [expanded,setExpanded]=useState(false);
 if(!events.length)return null;
 return <div className="assistant-progress">{events.length>5&&<button type="button" className="assistant-progress-toggle" aria-expanded={expanded} onClick={()=>setExpanded(value=>!value)}>{expanded?"收起此前进展":"此前进展"}</button>}<ol>{(expanded?events:events.slice(-5)).map(event=><li key={event.sequence} className={event.type==="ASSISTANT_MESSAGE"?"is-explanation":"is-activity"} title={new Date(event.at).toLocaleTimeString()}><span className="assistant-progress-marker">{event.type==="CAD_APPLIED"?<CheckOutlined/>:"·"}</span><p>{event.data?.message}</p></li>)}</ol></div>;
}
function TaskSteps({steps,status,names,locate}:{steps:TaskStep[];status?:AutomationTask["status"];names:{id:string;name:string}[];locate?:ModelingProps["locate"]}){
 return <ol className="assistant-steps">{steps.map((item,index)=>{const cancelled=status==="STOPPED"&&!item.result&&["PLANNED","PREPARING","PREVIEW_READY"].includes(item.phase);return <li key={item.id} data-phase={cancelled?"CANCELLED":item.phase}><span className="assistant-step-index">{["COMMITTED","DISPLAYED","APPLIED"].includes(item.phase)?<CheckOutlined/>:index+1}</span><div className="assistant-step-body"><div><strong>{names.find(op=>op.id===item.operationId)?.name??item.operationId}</strong><span title={item.failure?assistantFailure(item.failure):phaseText[item.phase]}>{cancelled?"已取消":["COMMITTED","APPLIED","SUPERSEDED"].includes(item.phase)?"已应用":item.phase==="DISPLAYED"?"已显示":phaseText[item.phase]}</span></div><small>{stepDetail(item)}</small></div>{item.result?.output&&locate&&<Button type="text" size="small" onClick={()=>locate(item.result!.output!)}>定位</Button>}</li>})}</ol>;
}
function PastTask({documentId,taskId,sequence,goalVersion,names}:{documentId:string;taskId:string;sequence:number;goalVersion:number;names:{id:string;name:string}[]}){
 const [expanded,setExpanded]=useState(false);
 const query=useQuery({queryKey:["automation-task",documentId,taskId],queryFn:()=>automationApi.task(documentId,taskId),enabled:expanded,staleTime:30_000});
 return <details className="assistant-task" onToggle={event=>setExpanded(event.currentTarget.open)}><summary title={`请求版本 ${goalVersion}`}>步骤组 {sequence}{query.data?.status==="STOPPED"?" · 已停止":""}</summary>{expanded&&(query.isPending?<p>正在读取…</p>:query.data&&<TaskSteps steps={query.data.steps} status={query.data.status} names={names}/>)}{query.error&&<Alert type="error" title={query.error.message}/>}</details>;
}
function BudgetControl({settings,run,busy,saveDefault,saveRun}:{settings:AssistantSettings;run?:AssistantRun;busy:boolean;saveDefault:(value:{tokenBudget:number|null})=>Promise<void>;saveRun:(value:number|null)=>Promise<void>}){
 const [open,setOpen]=useState(false),[value,setValue]=useState<number|null>(null);
 const current=run?run.tokenBudget:settings.tokenBudget;
 return <Popover trigger="click" open={open} onOpenChange={next=>{setOpen(next);if(next)setValue(current)}} title="累计 Token 预算（可选）" content={<div className="assistant-budget-editor"><Checkbox checked={value!==null} onChange={event=>setValue(event.target.checked?Math.max(100000,(run?.usage.total_tokens??0)+100000):null)}>设置累计预算</Checkbox>{value!==null&&<InputNumber aria-label="累计 Token 预算" min={1} max={Number.MAX_SAFE_INTEGER} precision={0} step={10000} value={value} onChange={next=>setValue(typeof next==="number"?next:null)}/>}<p>达到预算后保存进度，调整后继续。用量由供应商在调用结束后报告，末次调用可能超额。</p>{run&&<Button type="primary" size="small" disabled={busy} onClick={()=>void saveRun(value).then(()=>setOpen(false))}>应用到当前任务</Button>}<Button size="small" disabled={busy} onClick={()=>void saveDefault({tokenBudget:value}).then(()=>setOpen(false))}>保存为新任务默认</Button></div>}><Button type="text" size="small" aria-label="累计预算设置" title={current==null?"无累计上限":`预算 ${tokenText(current)} Token`}>{current==null?"预算":`预算 ${tokenText(current)}`}</Button></Popover>;
}
