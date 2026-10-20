import {useEffect,useRef,useState} from "react";
import {useQuery,useQueryClient} from "@tanstack/react-query";
import {Alert,Button,Checkbox,Input,Spin,Tooltip} from "antd";
import {CheckOutlined,CloseOutlined,CommentOutlined,PauseOutlined,SendOutlined,SettingOutlined,StopOutlined} from "@ant-design/icons";
import {randomUUID} from "../../utils/random-uuid";
import {automationApi,type TaskStep} from "./automation-api";
import {assistantApi,type AssistantTurn,type Conversation,type TurnInput} from "./assistant-api";
import {phaseText} from "./task-model";
import {latestPlan,mergeConversation} from "./conversation-model";
import {messageOf,useModelingTask,type ModelingProps} from "./use-modeling-task";
import "./assistant-panel.css";

function stepDetail(step:TaskStep):string{
  const a=step.arguments;
  if(step.operationId==="sketch.create")return `${a.plane} 平面`;
  if(step.operationId==="sketch.rectangle")return `${a.width} × ${a.height} ${a.unit}`;
  if(step.operationId==="part.extrude")return `${a.length} ${a.unit}`;
  if(step.operationId==="parameter.set_literal")return `${a.value} ${a.unit}`;
  if(step.operationId==="parameter.set_expression")return String(a.expression);
  return "";
}
export function AssistantPanel(props:ModelingProps){
  const client=useQueryClient();
  const [open,setOpen]=useState(false),[settingsOpen,setSettingsOpen]=useState(false),[apiKey,setApiKey]=useState("");
  const [keyBusy,setKeyBusy]=useState(false),[draft,setDraft]=useState(""),[sending,setSending]=useState(false),[error,setError]=useState<string>();
  const [optimistic,setOptimistic]=useState<TurnInput>(),[retry,setRetry]=useState<TurnInput>();
  const model=useModelingTask(props,open),{task,step,busy}=model;
  const alive=useRef(true),visible=useRef(open),request=useRef<AbortController|undefined>(undefined),end=useRef<HTMLDivElement>(null);
  visible.current=open;
  const conversationKey=["assistant-conversation",props.documentId];
  const connection=useQuery({queryKey:["assistant-settings"],queryFn:assistantApi.settings,enabled:open,staleTime:30_000});
  const conversation=useQuery({queryKey:conversationKey,queryFn:async()=>mergeConversation(client.getQueryData<Conversation>(conversationKey),await assistantApi.conversation(props.documentId)),enabled:open,
    refetchInterval:q=>sending||q.state.data?.turns.some(turn=>turn.status==="THINKING")?1000:false});
  const operations=useQuery({queryKey:["automation-operations"],queryFn:automationApi.operations,enabled:open,staleTime:60_000});
  const thinking=conversation.data?.turns.find(turn=>turn.status==="THINKING"),unfinished=task?.status==="ACTIVE"||task?.status==="PAUSED";
  useEffect(()=>{alive.current=true;return()=>{alive.current=false;request.current?.abort()}},[]);
  useEffect(()=>{
    if(!open||sending)return;
    const latest=latestPlan(conversation.data?.turns??[]);
    if(latest){if(latest!==model.taskId)model.select(latest);return}
    if(model.taskId)return;
    let disposed=false;
    void automationApi.tasks(props.documentId).then(result=>{if(!disposed&&result.tasks[0])model.select(result.tasks[0].id)}).catch(cause=>{if(!disposed)setError(messageOf(cause))});
    return()=>{disposed=true};
  },[open,model.taskId,conversation.data,props.documentId,sending]);
  useEffect(()=>{end.current?.scrollIntoView({block:"nearest",behavior:"smooth"})},[conversation.data?.turns.length,optimistic,sending,task?.version,settingsOpen]);
  useEffect(()=>{const cancel=()=>{if(document.hidden)request.current?.abort()};document.addEventListener("visibilitychange",cancel);return()=>document.removeEventListener("visibilitychange",cancel)},[]);
  const cancelChat=async()=>{
    model.setRunning(false);request.current?.abort();setOptimistic(undefined);setRetry(undefined);
    if(thinking)try{await assistantApi.cancel(props.documentId,thinking.id)}catch(cause){if(alive.current)setError(messageOf(cause))}
    await client.invalidateQueries({queryKey:conversationKey});
  };
  const rememberTurn=(turn:AssistantTurn)=>client.setQueryData<Conversation>(["assistant-conversation",turn.documentId],previous=>mergeConversation(previous,{hasEarlier:false,turns:[turn]}));
  const send=async(previous?:TurnInput)=>{
    if(sending||thinking||unfinished||model.loading||!props.enabled||!connection.data?.configured)return;
    const input=previous??{requestId:randomUUID(),text:draft.trim(),workingBodyId:props.workingBodyId};
    if(!input.text)return;
    setSending(true);setError(undefined);model.setError(undefined);setOptimistic(input);setDraft("");setRetry(undefined);
    const controller=new AbortController();request.current=controller;
    try{
      await props.prepare();
      const turn=await assistantApi.send(props.documentId,input,controller.signal);
      if(!alive.current)return;
      rememberTurn(turn);
      // Recovery/polling never starts mutations; only this successful explicit send does.
      if(turn.taskId&&turn.status==="COMPLETED")model.select(turn.taskId,!controller.signal.aborted&&visible.current&&model.automatic&&!document.hidden);
    }catch(cause){if(alive.current&&!controller.signal.aborted){setError(messageOf(cause));setRetry(input)}}
    finally{request.current=undefined;if(alive.current){setSending(false);setOptimistic(undefined);void client.invalidateQueries({queryKey:conversationKey})}}
  };
  const saveKey=async()=>{
    setKeyBusy(true);setError(undefined);
    try{client.setQueryData(["assistant-settings"],await assistantApi.saveKey(apiKey));setApiKey("");setSettingsOpen(false)}
    catch(cause){setError(messageOf(cause))}finally{setKeyBusy(false)}
  };
  const close=()=>{model.close();setOpen(false);setApiKey("");request.current?.abort()};
  const renderTask=()=>task&&<section className="assistant-task" aria-label="建模步骤">
    <details open><summary><span>建模步骤</span><span>{task.steps.filter(item=>item.phase==="DISPLAYED").length} / {task.steps.length}</span></summary>
      <ol className="assistant-steps">{task.steps.map((item,index)=><li key={item.id} data-phase={item.phase}>
        <span className="assistant-step-index">{item.phase==="DISPLAYED"?<CheckOutlined/>:index+1}</span>
        <div className="assistant-step-body"><div><strong>{operations.data?.operations.find(op=>op.id===item.operationId)?.name??"建模操作"}</strong><span>{phaseText[item.phase]}</span></div><small>{stepDetail(item)}</small>{item.failure&&<Alert type="error" title={item.failure.message}/>}</div>
        {item.result?.output&&<Button type="text" size="small" onClick={()=>props.locate(item.result!.output!)}>定位</Button>}
      </li>)}</ol>
    </details>
    <div className="assistant-task-actions">
      {step?.phase==="COMMITTED"&&<Button size="small" disabled={busy} onClick={()=>void model.retryDisplay()}>重新显示结果</Button>}
      {task.status==="ACTIVE"&&<>
        {(!model.running||!model.automatic)&&<Button size="small" type="primary" disabled={!props.enabled||busy||step?.phase==="COMMITTED"} loading={busy} onClick={()=>{model.setRunning(model.automatic);void model.advance()}}>{step?.phase==="PREVIEW_READY"?"确认这一步":model.automatic?"开始执行":"预览下一步"}</Button>}
        <Button size="small" aria-label="暂停 / 接管" icon={<PauseOutlined/>} onClick={()=>void model.control("pause")}>暂停 / 接管</Button>
        <Tooltip title="停止后保留已完成的建模步骤"><Button size="small" type="text" aria-label="停止建模" icon={<StopOutlined/>} onClick={()=>void model.control("stop")}/></Tooltip>
      </>}
      {task.status==="PAUSED"&&<><Button size="small" type="primary" disabled={!props.enabled||busy} onClick={()=>void model.control("resume")}>基于当前状态继续</Button><Button size="small" onClick={()=>void model.control("stop")}>停止建模</Button></>}
      {task.status==="COMPLETED"&&<span className="assistant-task-complete"><CheckOutlined/> 已完成，可以继续提出修改</span>}
      {task.status==="STOPPED"&&<span>已停止，已完成的步骤已保留</span>}
    </div>
  </section>;
  if(!open)return <Button className="assistant-launcher" aria-label="建模助手" icon={<CommentOutlined/>} onClick={()=>setOpen(true)}>AI 建模</Button>;
  const showConnection=settingsOpen||connection.data?.configured===false,turns=conversation.data?.turns??[];
  const optimisticVisible=optimistic&&!turns.some(turn=>turn.requestId===optimistic.requestId);
  return <aside className="assistant-panel" aria-label="建模助手" onKeyDown={event=>event.stopPropagation()} onPointerDown={event=>event.stopPropagation()}>
    <header className="assistant-header"><div className="assistant-heading"><CommentOutlined/><strong>建模助手</strong><span className={`assistant-connection ${connection.data?.configured?"is-connected":""}`}>{connection.data?.configured?"DeepSeek":"未连接"}</span></div><div><Tooltip title="连接设置"><Button type="text" size="small" aria-label="连接设置" icon={<SettingOutlined/>} onClick={()=>{setSettingsOpen(value=>!value);setApiKey("");setError(undefined)}}/></Tooltip><Button type="text" size="small" aria-label="收起建模助手" icon={<CloseOutlined/>} onClick={close}/></div></header>
    <div className="assistant-document"><span className="assistant-document-dot"/>{props.documentName}<span>当前零件</span></div>
    {showConnection?<div className="assistant-connect">
      <div className="assistant-connect-icon"><CommentOutlined/></div><h3>{connection.data?.configured?"连接设置":"用对话开始建模"}</h3><p>连接 DeepSeek，描述你想创建的零件，助手会在画布上逐步完成。</p>
      <label htmlFor="assistant-api-key">DeepSeek API Key</label><Input.Password id="assistant-api-key" aria-label="API Key" value={apiKey} autoComplete="off" placeholder={connection.data?.configured?"输入新的 API Key 以替换":"粘贴你的 API Key"} onChange={event=>setApiKey(event.target.value)} onPressEnter={()=>{if(apiKey)void saveKey()}}/>
      <small>密钥按账号加密保存在服务端，保存后不会回显。</small><Button type="primary" block loading={keyBusy} disabled={!apiKey.trim()||sending} onClick={()=>void saveKey()}>保存并连接</Button><a href="https://platform.deepseek.com/api_keys" target="_blank" rel="noreferrer">获取 DeepSeek API Key ↗</a>
      {connection.data?.configured&&<div className="assistant-connection-actions"><Button size="small" onClick={()=>{setSettingsOpen(false);setApiKey("")}}>返回对话</Button><Button type="text" size="small" disabled={keyBusy||sending} onClick={()=>void(async()=>{setKeyBusy(true);try{await assistantApi.deleteKey();await client.invalidateQueries({queryKey:["assistant-settings"]})}catch(cause){setError(messageOf(cause))}finally{setKeyBusy(false)}})()}>断开连接</Button></div>}
    </div>:<>
      <div className="assistant-thread" role="log" aria-label="建模对话" aria-live="polite">
        {conversation.isPending&&<div className="assistant-loading"><Spin size="small"/> 正在读取对话</div>}
        {!conversation.isPending&&turns.length===0&&!optimisticVisible&&<div className="assistant-welcome"><h3>想做一个什么样的零件？</h3><p>告诉我形状和尺寸。我会先理解需求，再把建模过程展示给你。</p><button onClick={()=>setDraft("在 XY 平面创建一个 80 × 50 mm 的矩形，并拉伸 12 mm")}>创建 80 × 50 × 12 mm 的长方体 <span>↗</span></button><button onClick={()=>setDraft("当前零件有哪些可以修改的参数？")}>查看当前零件的参数 <span>↗</span></button><small>当前支持矩形草图、拉伸及参数修改。</small></div>}
        {conversation.data?.hasEarlier&&<div className="assistant-history-note">显示最近 30 次对话；建模步骤保存在文档中</div>}
        {turns.map(turn=><div className="assistant-exchange" key={turn.id}>
          <div className="assistant-message is-user"><span className="assistant-speaker">你</span><p>{turn.text}</p></div>
          <div className="assistant-message is-assistant"><span className="assistant-speaker"><CommentOutlined/> 建模助手</span>
            {turn.status==="THINKING"?<div className="assistant-thinking"><Spin size="small"/> 正在理解需求、检查零件…</div>:<>
              {turn.reply&&<p>{turn.reply}</p>}{turn.failure&&<Alert type="error" title={turn.failure.message}/>}
              {turn.status==="CANCELLED"&&<p className="assistant-muted">已停止本次回复。</p>}{turn.status==="INTERRUPTED"&&<p className="assistant-muted">上次连接中断，未自动执行新的建模步骤。可以重新描述需求。</p>}
            </>}
            {turn.taskId&&(turn.taskId===model.taskId?renderTask():<PastTask documentId={props.documentId} taskId={turn.taskId}/>)}
          </div>
        </div>)}
        {optimisticVisible&&<div className="assistant-exchange"><div className="assistant-message is-user"><span className="assistant-speaker">你</span><p>{optimistic.text}</p></div><div className="assistant-message is-assistant"><span className="assistant-speaker"><CommentOutlined/> 建模助手</span><div className="assistant-thinking"><Spin size="small"/> 正在理解需求、检查零件…</div></div></div>}
        {task&&!turns.some(turn=>turn.taskId===model.taskId)&&<div className="assistant-message is-assistant"><p>{task.goal}</p>{renderTask()}</div>}<div ref={end}/>
      </div>
      <div className="assistant-composer">
        {!props.enabled&&<div className="assistant-input-hint">请先结束当前编辑，再继续对话建模</div>}
        {unfinished&&<div className="assistant-input-hint">{task?.status==="PAUSED"?"建模已暂停，可接管修改后继续，或停止并提出新需求":"正在完成上方步骤，可以随时暂停或接管"}</div>}
        <Input.TextArea aria-label="建模需求" value={draft} onChange={event=>setDraft(event.target.value)} autoSize={{minRows:2,maxRows:5}} maxLength={600} placeholder="描述形状、尺寸，或提出修改…" disabled={sending||Boolean(thinking)} onKeyDown={event=>{if(event.key==="Enter"&&!event.shiftKey&&!event.nativeEvent.isComposing){event.preventDefault();void send()}}}/>
        <div className="assistant-composer-actions"><Checkbox checked={!model.automatic} onChange={event=>{model.setAutomatic(!event.target.checked);if(event.target.checked)model.setRunning(false)}}>逐步确认</Checkbox><span>Enter 发送</span>{sending||thinking?<Button size="small" aria-label="停止回复" icon={<StopOutlined/>} onClick={()=>void cancelChat()}/>:<Button type="primary" size="small" aria-label="发送建模需求" icon={<SendOutlined/>} disabled={!draft.trim()||!props.enabled||Boolean(unfinished)||model.loading||!connection.data?.configured} onClick={()=>void send()}/>}</div>
      </div>
    </>}
    {(error||model.error||connection.error||conversation.error)&&<div className="assistant-error"><Alert closable type="error" title={error??model.error??connection.error?.message??conversation.error?.message} onClose={()=>{setError(undefined);model.setError(undefined)}}/>{retry&&<Button size="small" disabled={sending||Boolean(thinking)||Boolean(unfinished)} onClick={()=>void send(retry)}>重试本次请求</Button>}</div>}
  </aside>;
}

function PastTask({documentId,taskId}:{documentId:string;taskId:string}){
  const [expanded,setExpanded]=useState(false);
  const query=useQuery({queryKey:["automation-task",documentId,taskId],queryFn:()=>automationApi.task(documentId,taskId),enabled:expanded,staleTime:30_000});
  return <details className="assistant-task" onToggle={event=>setExpanded(event.currentTarget.open)}><summary>查看此前建模步骤</summary>
    {query.isPending?<p className="assistant-history-note">正在读取步骤…</p>:query.data&&<ol className="assistant-steps">{query.data.steps.map((item,index)=><li key={item.id} data-phase={item.phase}><span className="assistant-step-index">{index+1}</span><div className="assistant-step-body"><div><strong>{item.operationId}</strong><span>{phaseText[item.phase]}</span></div><small>{stepDetail(item)}</small></div></li>)}</ol>}
    {query.error&&<Alert type="error" title={query.error.message}/>}
  </details>;
}
