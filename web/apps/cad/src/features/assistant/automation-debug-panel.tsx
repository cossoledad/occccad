import {useRef,useState} from "react";
import {useMutation} from "@tanstack/react-query";
import {Alert,Button,Checkbox,Input,InputNumber,Segmented,Select} from "antd";
import {api} from "../../api/client";
import {ApiError} from "../../api";
import {useWorkbenchStore} from "../../state/workbench-store";
import type {DocumentView} from "../../types";
import type {FrozenScope} from "./assistant-api";
import {automationApi} from "./automation-api";
import {debugEvidenceQuery,debugRecord,debugSelectionSource,debugTopologyArguments,freezeDebugQuery,nextDebugQuery,type DebugQueryInput,type DebugQueryResult,type DebugTool} from "./automation-debug-model";
import {freezeScope} from "./frozen-scope";
import {ToolPayload} from "./tool-payload";
import "./automation-debug.css";

const defaults=(tool:DebugTool)=>tool==="cad_query_topology"?{kind:"EDGE",limit:10}:tool==="cad_query_model"?{fields:["name","type","bodyId","status"],limit:20}:{};
const payload=(value:unknown)=><ToolPayload value={value} className="inspector-object-data"/>;
const geometryLabels:Record<string,string>={centroidMM:"质心 mm",originMM:"原点 / 圆心 mm",direction:"法向 / 轴向",xDirection:"参数 X 方向",radiusMM:"半径 mm",minorRadiusMM:"短半径 mm",lengthMM:"长度 mm",areaMM2:"面积 mm²",halfAngleRadians:"半角 rad",materialSide:"材料侧（圆柱 -1 内壁，+1 外壁）",range:"曲线参数",unavailable:"缺失 / 不支持"};
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
    setTool(next);setArguments(JSON.stringify(topology?{kind:topology,limit:10}:{fields:["name","type","bodyId","definition"],limit:1},null,2));
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
  {tool==="cad_query_topology"&&<TopologyFilters text={argumentsText} disabled={busy} onChange={setArguments}/>}
  <label>工具参数（JSON）<Input.TextArea aria-label="调试查询参数" value={argumentsText} disabled={busy} autoSize={{minRows:4,maxRows:12}} onChange={event=>setArguments(event.target.value)}/></label>
  <Button type="primary" size="small" loading={query.isPending} disabled={capturing} onClick={submit}>执行只读查询</Button>
  {error&&<Alert type="error" title={error}/>}
  {entries.length>0&&<><h4>最近查询（本地保留 10 条）</h4><div className="automation-debug-history">{entries.map(item=><Button key={item.id} size="small" type={item.id===active?"primary":"default"} onClick={()=>setActive(item.id)}>查询 {item.id}{item.failure||item.result?.toolResult.isError?" · 错误":""}</Button>)}</div></>}
  {entry&&<QueryInspection key={entry.id} entry={entry} busy={busy} onNext={cursor=>void execute(nextDebugQuery(entry.request,cursor),entry.human)} onEvidence={id=>void execute(debugEvidenceQuery(entry.request,id),entry.human)}/>}
 </div>;
}

function QueryInspection({entry,busy,onNext,onEvidence}:{entry:QueryEntry;busy:boolean;onNext:(cursor:string)=>void;onEvidence:(id:string)=>void}){
 const [source,setSource]=useState("tool"),[inspected,setInspected]=useState<unknown>();
 const result=entry.result,page=result?.supplement.page;
 const value=source==="tool"?result?.toolResult.content:result?.serviceResult?.content;
 const data=debugRecord(value),items=Array.isArray(data?.items)?data.items.map(debugRecord).filter((item):item is Record<string,unknown>=>Boolean(item)):[];
 const cursor=typeof page?.nextCursor==="string"?page.nextCursor:"";
 const coversAll=page?.pageComplete??page?.complete;
 return <section className="automation-debug-inspection">
  <h4>查询 {entry.id}</h4>
  <details><summary>固定输入与实际工具参数</summary>{payload(entry.request)}{result&&payload(result.toolCall)}</details>
  {entry.failure!==undefined&&<div role="alert">{payload(entry.failure)}</div>}
  {result&&<>
   <p>服务内容 {result.serviceBytes.toLocaleString()} B · 模型可见内容 {result.toolBytes.toLocaleString()} B / {result.toolLimitBytes.toLocaleString()} B</p>
   <p className="automation-debug-note">字节数为 UTF-8 JSON 内容，不是 Token 或 HTTP 总大小。服务原页仍受现有字段和分页边界约束。</p>
   {result.outputReplaced&&<Alert type="warning" title="工具输出已替换为错误反馈；原服务页不会进入模型消息"/>}
   {page&&Object.keys(page).length>0&&<div className="automation-debug-page">
    <strong>服务分页信息（人工对照）</strong>
    {page?.returnedCount!==undefined&&<span>本页 {String(page.returnedCount)} · 匹配 {String(page.matchedCount??"未知")} · 集合 {String(page.selectionSetCount??"无")}</span>}
    {page?.scopeCount!==undefined&&<span>范围 {String(page.scopeCount)} · 候选 {String(page.candidateCount)} · 无法判断 {String(page.unknownCount)}</span>}
    {page?.selectionSetComplete!==undefined&&<span>集合引用覆盖全部匹配：{page.selectionSetComplete?"是":"否"}</span>}
    {coversAll!==undefined&&<span>本页包含匹配全集：{coversAll?"是":"否"}</span>}
    {page?.hasMore!==undefined&&<span>还有后续页：{page.hasMore?"是":"否"}</span>}
   </div>}
   {page?.matchComplete===false&&<Alert type="warning" title="部分候选缺少筛选所需属性或关系；已知匹配不能作为完整集合执行。"/>}
   <Segmented block value={source} onChange={value=>{setSource(String(value));setInspected(undefined)}} options={[{value:"tool",label:"模型可见输出"},{value:"service",label:"服务原页（人工对照）"}]}/>
   {source==="service"&&result.serviceOmitted?<Alert type="warning" title={`原页超过人工调试响应边界 ${result.serviceLimitBytes.toLocaleString()} B；请减小页或字段。未返回任何部分 JSON。`}/>:<>
    {entry.request.tool==="cad_query_topology"&&items.length>0&&<div className="automation-debug-observations">
     <p className="automation-debug-note">{String(debugRecord(data?.scope)?.coordinateSpace??"坐标未知")} · 长度 mm · 角度 rad。平面为法向，圆/圆柱为轴向；轴向正负不表示孔入口/出口。省略值无可用测量，不按零处理。</p>
     {items.map(item=>{const geometry=debugRecord(item.geometry),faces=debugRecord(item.adjacentFaces);return <article key={String(item.id)}>
      <strong>{String(item.geometryType)}</strong><code>{String(item.id)}</code>
      <dl>{Object.entries(geometry??{}).map(([key,value])=><div key={key}><dt title={key}>{geometryLabels[key]??key}</dt><dd>{JSON.stringify(value)}</dd></div>)}</dl>
      <details><summary>关联面 {String(faces?.knownCount??"未知")} · {faces?.complete?"完整":"部分 / 未知"}</summary>{payload(faces)}</details>
      <Button size="small" disabled={busy} onClick={()=>onEvidence(String(item.id))}>读取此快照的完整证据</Button>
      {item.selection!==undefined&&<details open><summary>显式读取的正式引用</summary>{payload(item.selection)}</details>}
     </article>})}
    </div>}
    {data?.items!==undefined?<details><summary>完整响应 JSON（含集合与游标）</summary>{payload(value)}</details>:payload(value)}
   </>}
   {entry.request.tool!=="cad_query_topology"&&items.length>0&&<details><summary>查看本页对象</summary><ul>{items.map((item,index)=>{const ref=debugRecord(item.ref);return <li key={String(item.id??ref?.id??index)}><Button type="link" size="small" onClick={()=>setInspected(item)}>{String(ref?.kind??"对象")} · {String(item.id??ref?.id??index)}</Button></li>})}</ul>{inspected!==undefined&&payload(inspected)}</details>}
   {cursor&&<Button size="small" disabled={busy} onClick={()=>onNext(cursor)}>下一服务页（沿用此固定输入）</Button>}
   <details><summary>人工补充信息（不进入模型输出）</summary>{payload({queryEvidence:result.supplement,capturedProperties:entry.human})}</details>
  </>}
 </section>;
}

function TopologyFilters({text,disabled,onChange}:{text:string;disabled:boolean;onChange:(value:string)=>void}){
 let args:Record<string,unknown>|undefined;
 try{args=debugRecord(JSON.parse(text))}catch{/* The JSON editor retains invalid drafts. */}
 if(!args)return <p className="automation-debug-note">修正参数 JSON 后可使用筛选控件。</p>;
 const filter=debugRecord(args.filter),radius=debugRecord(filter?.radius);
 const write=(next:Record<string,unknown>)=>onChange(JSON.stringify(next,null,2));
 const patch=(value:Record<string,unknown>)=>write(debugTopologyArguments(args,value));
 const numberValue=(value:unknown)=>typeof value==="number"?value:undefined;
 const radiusPatch=(key:string,value:number|null)=>{
  const next={...radius,[key]:value??undefined};
  for(const name of Object.keys(next))if(next[name]===undefined)delete next[name];
  if(key==="equalMM"&&value!==null){delete next.minMM;delete next.maxMM}
  if((key==="minMM"||key==="maxMM")&&value!==null)delete next.equalMM;
  patch({radius:Object.keys(next).length?next:undefined});
 };
 return <fieldset className="automation-debug-filters" disabled={disabled}>
  <legend>完整固定范围上的筛选</legend>
  <label>拓扑类型<Select value={String(args.kind??"EDGE")} disabled={disabled} options={["EDGE","FACE","VERTEX"].map(value=>({value,label:value}))} onChange={kind=>{const next:Record<string,unknown>={...args,kind};delete next.cursor;write(next)}}/></label>
  <label>几何类型<Select allowClear value={typeof filter?.geometryType==="string"?filter.geometryType:undefined} placeholder="全部类型" disabled={disabled} options={["LINE","CIRCLE","ELLIPSE","PLANE","CYLINDER","SPHERE","CONE","BSPLINE","BEZIER","POINT","TORUS","HYPERBOLA","PARABOLA","OFFSET","OTHER_CURVE"].map(value=>({value,label:value}))} onChange={value=>patch({geometryType:value})}/></label>
  <label>半径等于（mm）<InputNumber value={numberValue(radius?.equalMM)} min={0} disabled={disabled} onChange={value=>radiusPatch("equalMM",value)}/></label>
  <div className="automation-debug-radius-range"><label>最小半径（mm）<InputNumber value={numberValue(radius?.minMM)} min={0} disabled={disabled} onChange={value=>radiusPatch("minMM",value)}/></label><label>最大半径（mm）<InputNumber value={numberValue(radius?.maxMM)} min={0} disabled={disabled} onChange={value=>radiusPatch("maxMM",value)}/></label></div>
  <label>半径容差（mm，筛选半径时必填）<InputNumber value={numberValue(radius?.toleranceMM)} placeholder="例如 0.000001" min={0} disabled={disabled} onChange={value=>radiusPatch("toleranceMM",value)}/></label>
  <label>关联面稳定 ID<Input value={typeof filter?.adjacentFaceId==="string"?filter.adjacentFaceId:""} disabled={disabled} onChange={event=>patch({adjacentFaceId:event.target.value})}/></label>
  <p className="automation-debug-note">等值与范围条件互斥。筛选先生成集合，再分页；查看摘要或展开证据不会改变以上输入。</p>
 </fieldset>;
}
