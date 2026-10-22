import {useRef,useState} from "react";
import {useMutation} from "@tanstack/react-query";
import {Alert,Button,Checkbox,Input,Segmented,Select} from "antd";
import {api} from "../../api/client";
import {ApiError} from "../../api";
import {useWorkbenchStore} from "../../state/workbench-store";
import type {DocumentView} from "../../types";
import type {FrozenScope} from "./assistant-api";
import {automationApi} from "./automation-api";
import {debugRecord,debugSelectionSource,freezeDebugQuery,nextDebugQuery,type DebugQueryInput,type DebugQueryResult,type DebugTool} from "./automation-debug-model";
import {freezeScope} from "./frozen-scope";
import {ToolPayload} from "./tool-payload";
import "./automation-debug.css";

const defaults=(tool:DebugTool)=>tool==="cad_query_topology"?{kind:"EDGE",limit:10,includeSelections:true}:tool==="cad_query_model"?{fields:["name","type","bodyId","status"],limit:20}:{};
const payload=(value:unknown)=><ToolPayload value={value} className="inspector-object-data"/>;
type QueryEntry={id:number;request:DebugQueryInput;result?:DebugQueryResult;failure?:unknown;human?:unknown};

/** A read-only local trace. No model settings, Run, provider or CAD executor. */
export function AutomationDebugPanel({view}:{view:DocumentView}){
 const [scope,setScope]=useState<FrozenScope>(()=>({rootDocumentId:view.document.id,documentId:view.document.id,revisionId:view.document.versionId,workingBodyId:view.part?.activeBodyId,selections:[],selectionCount:0,complete:true}));
 const [tool,setTool]=useState<DebugTool>(view.part?"cad_query_topology":"cad_query_model");
 const [argumentsText,setArguments]=useState(()=>JSON.stringify(defaults(view.part?"cad_query_topology":"cad_query_model"),null,2));
 const [selectedOnly,setSelectedOnly]=useState(false),[capturing,setCapturing]=useState(false),[error,setError]=useState<string>();
 const [human,setHuman]=useState<unknown>(),[entries,setEntries]=useState<QueryEntry[]>([]),[active,setActive]=useState<number>();
 const serial=useRef(0);
 const query=useMutation({mutationFn:automationApi.debugQuery});
 const busy=query.isPending||capturing;
 const entry=entries.find(item=>item.id===active);
 const changeScope=(patch:Partial<FrozenScope>)=>setScope(current=>({...current,...patch}));

 const capture=async()=>{
  setCapturing(true);setError(undefined);
  try{
   const selections=structuredClone(useWorkbenchStore.getState().selections);
   if(selections.length>20)throw new Error("此调试入口一次最多固定 20 个选择；可改查整个 Body 并分页。");
   const source=debugSelectionSource(view,selections),properties:unknown[]=[];
   const frozen=await freezeScope(source,async(documentId,key,kind,localId,revisionId)=>{
    const value=await api.getTopologyProperties(documentId,key,kind,localId,revisionId);
    properties.push({documentId,revisionId,geometryKey:key,kind,localId,properties:value});
    return value;
   });
   const bodies=new Set(frozen.selections.map(item=>item.bodyId).filter((id):id is string=>Boolean(id)));
   if(bodies.size>1)throw new Error("正式引用来自多个 Body，请分别固定查询范围。");
   if(bodies.size===1)frozen.workingBodyId=bodies.values().next().value;
   setScope(frozen);setSelectedOnly(selections.length>0);
   setHuman({source:{documentId:source.documentId,revisionId:source.revisionId,bodyId:source.workingBodyId},selections,topologyProperties:properties});
   if(selections.length){
    const first=frozen.selections[0],topology=first?.selection?.expectedType;
    const next:DebugTool=topology?"cad_query_topology":"cad_query_model";
    setTool(next);setArguments(JSON.stringify(topology?{kind:topology,limit:10,includeSelections:true}:{fields:["name","type","bodyId","definition"],limit:1},null,2));
   }
  }catch(reason){setError(reason instanceof Error?reason.message:String(reason))}finally{setCapturing(false)}
 };
 const execute=async(input:DebugQueryInput,supplement:unknown)=>{
  setError(undefined);
  let request:DebugQueryInput;
  try{request=freezeDebugQuery(input)}catch(reason){setError(reason instanceof Error?reason.message:String(reason));return}
  const record:QueryEntry={id:++serial.current,request,human:structuredClone(supplement)};
  try{record.result=await query.mutateAsync(request)}catch(reason){record.failure=reason instanceof ApiError?{status:reason.status,message:reason.message}: {message:reason instanceof Error?reason.message:String(reason)}}
  setEntries(current=>[...current,record].slice(-10));setActive(record.id);
 };
 const submit=()=>{
  try{void execute({scope,tool,arguments:JSON.parse(argumentsText),selectedOnly},human)}catch(reason){setError(reason instanceof Error?reason.message:String(reason))}
 };
 return <div className="automation-debug">
  <p>固定快照后检查已有查询。无需 API Key 或模型调用，结果使用定义坐标。</p>
  <label>文档 ID<Input value={scope.documentId} disabled={busy} onChange={event=>changeScope({documentId:event.target.value,rootDocumentId:event.target.value})}/></label>
  <label>Revision ID<Input value={scope.revisionId} disabled={busy} onChange={event=>changeScope({revisionId:event.target.value})}/></label>
  <label>Body ID（留空为文档范围）<Input value={scope.workingBodyId??""} disabled={busy} onChange={event=>changeScope({workingBodyId:event.target.value||undefined})}/></label>
  <Button size="small" loading={capturing} disabled={query.isPending} onClick={()=>void capture()}>采用当前文档与选区</Button>
  <p className="automation-debug-note">已固定 {scope.selectionCount} 个选择。界面选择和模型 Head 的后续变化不会更新这些输入。</p>
  <Select aria-label="只读调试工具" value={tool} disabled={busy} options={[{value:"cad_query_model",label:"模型对象 · cad_query_model"},{value:"cad_query_topology",label:"拓扑 · cad_query_topology"},{value:"cad_context",label:"上下文 · cad_context"}]} onChange={value=>{setTool(value);setArguments(JSON.stringify(defaults(value),null,2));if(value==="cad_context")setSelectedOnly(false)}}/>
  <Checkbox checked={selectedOnly} disabled={busy||tool==="cad_context"} onChange={event=>setSelectedOnly(event.target.checked)}>仅查询已固定的选区</Checkbox>
  <label>工具参数（JSON）<Input.TextArea aria-label="调试查询参数" value={argumentsText} disabled={busy} autoSize={{minRows:4,maxRows:12}} onChange={event=>setArguments(event.target.value)}/></label>
  <Button type="primary" size="small" loading={query.isPending} disabled={capturing} onClick={submit}>执行只读查询</Button>
  {error&&<Alert type="error" title={error}/>}
  {entries.length>0&&<><h4>最近查询（本地保留 10 条）</h4><div className="automation-debug-history">{entries.map(item=><Button key={item.id} size="small" type={item.id===active?"primary":"default"} onClick={()=>setActive(item.id)}>查询 {item.id}{item.failure||item.result?.toolResult.isError?" · 错误":""}</Button>)}</div></>}
  {entry&&<QueryInspection key={entry.id} entry={entry} busy={busy} onNext={cursor=>void execute(nextDebugQuery(entry.request,cursor),entry.human)}/>}
 </div>;
}

function QueryInspection({entry,busy,onNext}:{entry:QueryEntry;busy:boolean;onNext:(cursor:string)=>void}){
 const [source,setSource]=useState("tool"),[inspected,setInspected]=useState<unknown>();
 const result=entry.result,page=result?.supplement.page;
 const value=source==="tool"?result?.toolResult.content:result?.serviceResult?.content;
 const data=debugRecord(value),items=Array.isArray(data?.items)?data.items.map(debugRecord).filter((item):item is Record<string,unknown>=>Boolean(item)):[];
 const cursor=typeof page?.nextCursor==="string"?page.nextCursor:"";
 const coversAll=page?.pageComplete??page?.complete;
 return <section className="automation-debug-inspection">
  <h4>查询 {entry.id}</h4>
  <details open><summary>固定输入</summary>{payload(entry.request)}{result&&<details><summary>实际工具参数（已绑定范围与稳定选择 ID）</summary>{payload(result.toolCall)}</details>}</details>
  {entry.failure!==undefined&&<div role="alert">{payload(entry.failure)}</div>}
  {result&&<>
   <p>服务内容 {result.serviceBytes.toLocaleString()} B · 模型可见内容 {result.toolBytes.toLocaleString()} B / {result.toolLimitBytes.toLocaleString()} B</p>
   <p className="automation-debug-note">字节数为 UTF-8 JSON 内容，不是 Token 或 HTTP 总大小。服务原页仍受现有字段和分页边界约束。</p>
   {result.outputReplaced&&<Alert type="warning" title="工具输出已替换为错误反馈；原服务页不会进入模型消息"/>}
   {page&&Object.keys(page).length>0&&<div className="automation-debug-page">
    <strong>服务分页信息（人工对照）</strong>
    {page?.returnedCount!==undefined&&<span>本页 {String(page.returnedCount)} / 匹配 {String(page.matchedCount??"未知")}</span>}
    {page?.selectionSetComplete!==undefined&&<span>集合引用覆盖全部匹配：{page.selectionSetComplete?"是":"否"}</span>}
    {coversAll!==undefined&&<span>本页包含匹配全集：{coversAll?"是":"否"}</span>}
    {page?.hasMore!==undefined&&<span>还有后续页：{page.hasMore?"是":"否"}</span>}
   </div>}
   <Segmented block value={source} onChange={value=>{setSource(String(value));setInspected(undefined)}} options={[{value:"tool",label:"模型可见输出"},{value:"service",label:"服务原页（人工对照）"}]}/>
   {source==="service"&&result.serviceOmitted?<Alert type="warning" title={`原页超过人工调试响应边界 ${result.serviceLimitBytes.toLocaleString()} B；请减小页或字段。未返回任何部分 JSON。`}/>:payload(value)}
   {items.length>0&&<details><summary>查看本页对象</summary><ul>{items.map((item,index)=>{const ref=debugRecord(item.ref);return <li key={String(item.id??ref?.id??index)}><Button type="link" size="small" onClick={()=>setInspected(item)}>{String(item.geometryType??debugRecord(item.properties)?.type??ref?.kind??"对象")} · {String(item.id??ref?.id??index)}</Button></li>})}</ul>{inspected!==undefined&&payload(inspected)}</details>}
   {cursor&&<Button size="small" disabled={busy} onClick={()=>onNext(cursor)}>下一服务页（沿用此固定输入）</Button>}
   <details><summary>人工补充信息（不进入模型输出）</summary>{payload({queryEvidence:result.supplement,capturedProperties:entry.human})}</details>
  </>}
 </section>;
}
