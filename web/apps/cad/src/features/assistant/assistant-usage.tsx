import {Dropdown,Button} from "antd";
import {DownloadOutlined} from "@ant-design/icons";
import {assistantApi,type RunUsage,type TurnUsage} from "./assistant-api";

export function UsageDetails({usage,label}:{usage?:RunUsage|TurnUsage;label:string}){
 if(!usage)return <small className="assistant-usage-missing">{label}用量记录缺失</small>;
 const unknown=usage.unknownAttempts??0,pending=usage.pendingCalls??0;
 return <details className="assistant-usage"><summary>{label}{unknown||pending?"已报告 ":" "}{usage.total_tokens.toLocaleString("zh-CN")} Token{unknown>0&&` · ${unknown} 次未知`}{pending>0&&` · ${pending} 次待返回`}</summary><div>
  <p>输入 {usage.prompt_tokens.toLocaleString("zh-CN")} · 输出 {usage.completion_tokens.toLocaleString("zh-CN")}</p>
  <p>{"calls" in usage?`实际调用 ${usage.calls} 次 · `:""}已报告 {usage.reportedCalls} 次 · 未知 {unknown} 次 · 待返回 {pending} 次</p>
  {unknown>0&&<p>未知用量未计入 Token 数；失败及重试中已报告的消耗均包含在内。</p>}
 </div></details>;
}

export function ExportMenu({documentId,runId,turnId}:{documentId:string;runId:string;turnId?:string}){
 const label=turnId?"导出本轮":"导出会话";
 return <Dropdown trigger={["click"]} menu={{items:(["markdown"] as const).map(format=>({key:format,label:<a href={assistantApi.exportURL(documentId,runId,format,turnId)}>会话记录</a>}))}}><Button type="text" size="small" icon={<DownloadOutlined/>} aria-label={label}>{label}</Button></Dropdown>;
}
