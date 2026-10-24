import {useEffect,useState} from "react";
import {useQuery,useQueryClient} from "@tanstack/react-query";
import {Button,Select,Spin} from "antd";
import {ApiError} from "../../api";
import {assistantApi,type ModelCallSummary,type ModelCallTrace,type RuntimeStamp} from "./assistant-api";
import {cadCheckLabel} from "./conversation-model";
import {UsageDetails} from "./assistant-usage";
import {ToolPayload as Payload} from "./tool-payload";

const toolNames:Record<string,string>={cad_context:"读取当前模型",cad_query_model:"查询模型对象",cad_query_topology:"查询实体边面",cad_search_operations:"查找建模工具",cad_describe_operation:"读取工具参数",cad_describe_type:"读取参数类型",cad_create_task:"提交建模步骤",cad_verify_part:"检查指定几何",cad_finish:"结束本次请求",cad_wait_for_input:"请求补充信息",cad_query_occurrence:"查询实例几何",cad_mate_candidates:"查找装配候选",cad_verify_assembly:"检查装配",cad_get_selection:"读取选中对象",cad_get_task:"查询步骤结果"};
const toolLabel=(name:string)=>toolNames[name]??name;
const phaseNames:Record<string,string>={PLANNED:"等待执行",PREPARING:"正在预览",PREVIEW_READY:"预览就绪",COMMITTING:"正在应用",APPLIED:"已应用",COMMITTED:"已应用",DISPLAYED:"已显示",FAILED:"执行失败",CANCELLED:"已取消",SUPERSEDED:"已应用"};
const record=(value:unknown):Record<string,unknown>|undefined=>value!==null&&typeof value==="object"&&!Array.isArray(value)?value as Record<string,unknown>:undefined;

function feedbackSummary(name:string,value:unknown,isError:boolean):string {
 const data=record(value);
 if(isError)return typeof data?.message==="string"?data.message:"工具返回了错误，展开查看原文";
 if(name==="cad_create_task")return `工具当时返回：已创建 ${Array.isArray(data?.steps)?data.steps.length:""} 步计划`;
 if(Array.isArray(data?.checks)){
  const checks=data.checks.map(record),failed=checks.filter(check=>check?.status==="FAILED").length,unknown=checks.filter(check=>check?.status==="NOT_VERIFIED").length;
  return failed||unknown?[failed&&`${failed} 项检查不一致`,unknown&&`${unknown} 项证据待核对`].filter(Boolean).join("，"):"指定检查通过";
 }
 if(Array.isArray(data?.items))return `本页返回 ${data.items.length} 个对象${typeof data.matchedCount==="number"?`，共匹配 ${data.matchedCount} 个`:""}${data.hasMore?"，还有下一页":""}`;
 if(data?.inputSchema)return "已返回工具说明和参数格式";
 if(data?.properties&&data?.type==="object")return "已返回参数类型定义";
 if(typeof data?.message==="string")return data.message;
 if(typeof data?.reply==="string")return data.reply;
 return "已返回结果";
}

function RuntimeInfo({runtime}:{runtime?:RuntimeStamp}){
 if(!runtime)return <p className="assistant-trace-note">这条历史调用未记录执行版本。</p>;
 return <details className="assistant-trace-runtime"><summary>执行版本{runtime.revision&&` · ${runtime.revision.slice(0,8)}${runtime.modified?"（含本地改动）":""}`}</summary><Payload value={runtime}/></details>;
}

export function AssistantTrace({documentId,runId,eventSequence,open,active}:{documentId:string;runId:string;eventSequence:number;open:boolean;active:boolean}){
 const client=useQueryClient();
 const [cursor,setCursor]=useState<string>(),[expandedId,setExpandedId]=useState<string|null>();
 const key=["assistant-trace",documentId,runId,cursor];
 const query=useQuery({queryKey:key,queryFn:()=>assistantApi.trace(documentId,runId,cursor),enabled:open,refetchInterval:open&&active&&!cursor?1500:false});
 useEffect(()=>{setCursor(undefined);setExpandedId(undefined)},[runId]);
 useEffect(()=>{if(open&&!cursor)void client.invalidateQueries({queryKey:["assistant-trace",documentId,runId,undefined]})},[open,runId,eventSequence]);
 const latest=query.data?.items.at(-1)?.id;
 return <div className="assistant-trace" aria-label="模型调用过程">
  <p className="assistant-trace-note">按实际调用顺序查看请求、模型原文、工具参数和反馈。</p>
  {query.isPending&&<p><Spin size="small"/> 正在读取调用记录…</p>}
  {query.error&&<div role="alert"><p>{query.error instanceof ApiError&&query.error.status===404?"当前后端尚未加载调用记录功能，请重新构建并重启后端。":query.error.message}</p><Button size="small" onClick={()=>void query.refetch()}>重试</Button></div>}
  <div className="assistant-trace-navigation">{query.data?.hasEarlier&&<Button size="small" type="link" onClick={()=>{setCursor(query.data!.nextCursor);setExpandedId(null)}}>较早调用</Button>}{cursor&&<Button size="small" type="link" onClick={()=>{setCursor(undefined);setExpandedId(undefined)}}>最近调用</Button>}</div>
  {!query.isPending&&!query.error&&!query.data?.items.length&&<p className="assistant-trace-note">尚无模型调用。</p>}
  {query.data?.items.map(item=><CallEntry key={item.id} documentId={documentId} runId={runId} item={item} open={expandedId===undefined?item.id===latest:expandedId===item.id} enabled={open} active={active} onToggle={next=>setExpandedId(next?item.id:null)}/>)}
 </div>;
}

function CallEntry({documentId,runId,item,open,enabled,active,onToggle}:{documentId:string;runId:string;item:ModelCallSummary;open:boolean;enabled:boolean;active:boolean;onToggle:(open:boolean)=>void}){
 const query=useQuery({queryKey:["assistant-call-trace",documentId,runId,item.id],queryFn:()=>assistantApi.callTrace(documentId,runId,item.id),enabled:enabled&&open,refetchInterval:enabled&&open&&active?1500:false});
 const {refetch}=query;
 useEffect(()=>{if(enabled&&open)void refetch()},[enabled,open,item.status,item.finishedAt,active,refetch]);
 const reply=query.data?.call.response?.content??item.reply;
 return <details className="assistant-call" open={open} onToggle={event=>{if(event.target===event.currentTarget&&event.currentTarget.open!==open)onToggle(event.currentTarget.open)}}>
  <summary><span className="assistant-call-heading">{item.sequence?`第 ${item.sequence} 次调用`:<time>{new Date(item.startedAt).toLocaleTimeString()}</time>}{item.inputId&&<small>提问</small>}</span><span>{item.toolNames.length?Array.from(new Set(item.toolNames.map(toolLabel))).join(" · "):item.status==="SENT"?"正在等待模型回复":item.status==="PREPARED"?"请求已准备，尚未发送":"模型回复"}</span>{!open&&item.reply&&<span className="assistant-call-preview">{item.reply}</span>}</summary>
  {open&&<div className="assistant-call-body">
   {query.isPending&&<p><Spin size="small"/> 正在读取…</p>}{query.error&&<p role="alert">{query.error.message}</p>}
   {query.data&&<>
    <UsageDetails usage={query.data.call.usage?{...query.data.call.usage,reportedCalls:1,unknownAttempts:0}:{prompt_tokens:0,completion_tokens:0,total_tokens:0,reportedCalls:0,unknownAttempts:query.data.call.usageState==="UNKNOWN"?1:0,pendingCalls:query.data.call.usageState==="PENDING"?1:0}} label="本次调用"/>
    <p className="assistant-trace-note">用量状态：{query.data.call.usageState} · 轮次：{query.data.call.turnId??"历史归属缺失"}</p>
    <details className="assistant-trace-request"><summary>① 发送给模型的内容</summary>
     {query.data.request?<>
      <p className="assistant-trace-note">{query.data.call.generation.model} · 思考度 {query.data.call.generation.thinking} · {query.data.request.messages.length} 条公开消息</p>
      {query.data.request.messages.map((message,index)=><details key={index}><summary>{({system:"系统提示 / 应用上下文",user:"用户输入",assistant:"历史模型回复",tool:"历史工具反馈"} as Record<string,string>)[message.role]??message.role}{message.tool_call_id&&<small> · {message.tool_call_id}</small>}</summary><Payload value={message.content}/>{message.tool_calls&&<Payload value={message.tool_calls}/>}</details>)}
      <details><summary>本次可用工具与参数定义</summary><Payload value={query.data.request.tools}/></details>
      <details><summary>本次模型配置</summary><Payload value={query.data.request.generation}/></details>
     </>:<p className="assistant-trace-note">这条历史记录未保存请求原文，无法还原当时的完整提示词。下面的模型回复、调用参数和反馈来自实际账本。</p>}
     <RuntimeInfo runtime={query.data.call.runtime}/>
    </details>
    <div className="assistant-trace-reply"><strong>② 模型回复</strong>{reply?<p>{reply}</p>:<p className="assistant-trace-note">{item.toolNames.length?"模型直接请求调用以下工具，未附文字回复。":"尚未收到公开文字回复。"}</p>}</div>
    {query.data.tools.map((tool,index)=><ToolEntry key={tool.id} tool={tool} index={index}/>)}
    {query.data.call.failure&&<p className="assistant-trace-failure">调用反馈：{query.data.call.failure.message}</p>}
   </>}
  </div>}
 </details>;
}

function ToolEntry({tool,index}:{tool:ModelCallTrace["tools"][number];index:number}){
 const result=record(tool.feedback),checks=Array.isArray(result?.checks)?result.checks.map(record):[];
 return <section className="assistant-trace-tool">
  <strong>③ 工具 {index+1} · {toolLabel(tool.name)}</strong><code className="assistant-trace-tool-name">{tool.name}</code>
  <details><summary>调用参数</summary><Payload value={tool.arguments}/></details>
  <div className={tool.isError?"assistant-trace-failure":"assistant-trace-feedback"}><strong>④ 工具反馈</strong><p>{tool.executed?feedbackSummary(tool.name,tool.feedback,tool.isError):"这条工具调用尚无执行反馈。"}</p>
   {checks.map((check,index)=>check&&<details key={index}><summary>{check.status==="PASSED"?"✓":check.status==="FAILED"?"!":"○"} {cadCheckLabel(String(check.name??check.kind??check.id??"指定检查"))}{check.status==="NOT_VERIFIED"&&" · 证据待核对"}</summary>{typeof check.message==="string"&&<p>{check.message}</p>}{check.expected!==undefined&&<><span>期望</span><Payload value={check.expected}/></>}{check.observed!==undefined&&<><span>实测与对象来源</span><Payload value={check.observed}/></>}</details>)}
   {tool.executed&&<details><summary>工具返回原文</summary><Payload value={tool.feedback}/></details>}
  </div>
  {tool.execution&&<details className="assistant-trace-execution" open={tool.execution.steps.some(step=>step.failure)}><summary>CAD 后续执行结果</summary><p className="assistant-trace-note">这是应用执行计划后的记录，模型在后续调用中通过阶段上下文观察它。</p>{tool.execution.steps.map(step=><div key={step.id}><strong>{step.operationId} · {phaseNames[step.phase]??step.phase}</strong>{step.failure&&<p className="assistant-trace-failure">{step.failure.message}</p>}<details><summary>实际参数与结果版本</summary><Payload value={{arguments:step.arguments,revisionId:step.revisionId,failure:step.failure}}/></details></div>)}</details>}
 </section>;
}

// Mounted only inside the inspector's explicitly opened developer area.
export function AssistantDeveloperTrace({documentId}:{documentId:string}){
 const [id,setId]=useState<string>();
 const runs=useQuery({queryKey:["assistant-runs",documentId,undefined],queryFn:()=>assistantApi.runs(documentId)});
 const selected=runs.data?.items.find(r=>r.id===id)??runs.data?.items[0];
 useEffect(()=>setId(undefined),[documentId]);
 return <div>{runs.isPending?<Spin size="small"/>:runs.error?<><p>无法读取调用记录。</p><Button size="small" onClick={()=>void runs.refetch()}>重试</Button></>:!selected?<p>当前文档尚无助手调用。</p>:<>
  <Select aria-label="开发区助手记录" value={selected.id} options={runs.data?.items.map((r,index)=>({value:r.id,label:r.goal||`任务 ${index+1}`}))} onChange={setId} style={{width:"100%"}}/>
  <a href={assistantApi.exportURL(documentId,selected.id,"json")}>导出完整调用数据</a>
  <AssistantTrace documentId={documentId} runId={selected.id} eventSequence={selected.eventSequence} open active={false}/>
 </>}</div>;
}
