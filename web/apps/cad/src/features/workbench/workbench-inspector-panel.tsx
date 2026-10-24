import { useQuery } from "@tanstack/react-query";
import { Alert, Button, Segmented, Spin } from "antd";
import {useState,useEffect,useSyncExternalStore,type ComponentProps} from "react";
import {inspectorObjectData} from "./inspector-object-data";
import {useDeveloperEvidence} from "../../cad/command/developer-evidence";
import {engineeringProblem,useOperationFeedback} from "../../cad/command/operation-feedback";
import { api } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import { useWorkbenchStore } from "../../state/workbench-store";
import type { HistoryEntry, Selection } from "../../types";
import { inspectorOwnerDocumentId } from "./inspector-object-data";
import { topologyPropertyContext } from "./topology-property-context";
import { History, Properties } from "./workbench-inspector";
import {AssistantDeveloperTrace} from "../assistant/assistant-trace";
import {AutomationDebugPanel} from "../assistant/automation-debug-panel";

import {lastClientPerformanceSample,subscribeClientPerformance} from "../../utils/performance";
function LastRequestTime(){
 const sample=useSyncExternalStore(subscribeClientPerformance,lastClientPerformanceSample,()=>undefined);
 return <p title="最后完成的一次请求，包含网络等待和响应处理" aria-label="最后一次后端请求耗时">后端请求：{sample?`${Math.round(sample.durationMs)} ms`:"—"}</p>;
}

type InspectorPanelProps = Omit<ComponentProps<typeof Properties>, "diagnostics" | "topology" | "topologyLoading"> & {
  documentID: string;
  canRestore: boolean;
  onRestore: (entry: HistoryEntry) => void;
};

/** Mounted only while the inspector is open; each tab owns its query lifetime. */
export function WorkbenchInspectorPanel({ documentID, canRestore, onRestore, ...props }: InspectorPanelProps) {
  const [developer,setDeveloper]=useState(false),[developerTab,setDeveloperTab]=useState("object");
  const evidence=useDeveloperEvidence(),feedback=useOperationFeedback();
  const tab = useWorkbenchStore((state) => state.inspectorTab);
  const setTab = useWorkbenchStore((state) => state.setInspectorTab);
  useEffect(()=>{if(tab==="query")setTab("properties")},[tab]);
  const ownerID=inspectorOwnerDocumentId(props.view,props.selection);
  const owner=useQuery({queryKey:queryKeys.document(ownerID),queryFn:()=>api.getDocument(ownerID),
    enabled:tab==="properties"&&Boolean(props.selection)&&ownerID!==props.view.document.id,staleTime:30_000});
  const properties = useQuery({ queryKey: queryKeys.documentProperties(documentID),
    queryFn: () => api.getDocumentProperties(documentID), enabled: developer, staleTime: 30_000 });
  const history = useQuery({ queryKey: queryKeys.history(documentID), queryFn: () => api.getHistory(documentID),
    enabled: tab === "history", staleTime: 10_000 });
  const selection = props.selection && ["face", "edge", "vertex"].includes(props.selection.kind)
    ? props.selection as Extract<Exclude<Selection, null>, { kind: "face" | "edge" | "vertex" }> : undefined;
  const context = topologyPropertyContext(selection ?? null, documentID);
  const topology = useQuery({
    queryKey: selection ? queryKeys.topologyProperties(context.documentId, selection.geometryKey ?? "",
      selection.kind, selection.topologyId, selection.versionId) : ["topology-properties", "none"],
    queryFn: () => api.getTopologyProperties(context.documentId, selection!.geometryKey!,
      selection!.kind.toUpperCase() as "FACE" | "EDGE" | "VERTEX", selection!.topologyId, context.versionId),
    enabled: tab === "properties" && Boolean(selection?.geometryKey), staleTime: 5 * 60_000,
  });
  const error = tab === "query" ? undefined : tab === "history" ? history.error : topology.error ?? owner.error ?? properties.error;
  return <>
    <Segmented block aria-label="检查器内容" value={tab==="query"?"properties":tab} onChange={(value) => setTab(value as "properties" | "history")}
      options={[{ label: "属性", value: "properties" }, { label: "历史", value: "history" }]} />
    <div className="inspector-overlay-content">
      {error && <Alert type="error" showIcon title="无法读取信息" description={engineeringProblem(error)} action={<Button size="small" onClick={()=>void (tab==="history"?history.refetch():topology.error?topology.refetch():owner.error?owner.refetch():properties.refetch())}>重试</Button>}/>}
      {tab === "history" ? history.isPending ? <Spin size="small" />
        : <History entries={history.data ?? []} onRestore={onRestore} canRestore={canRestore} />
        : <Properties {...props} ownerView={owner.data} diagnostics={properties.data} topology={topology.data} topologyLoading={topology.isFetching}
          readErrors={{owner:owner.error?.message,diagnostics:properties.error?.message,
            topology:selection&&!selection.geometryKey ? "当前对象没有可查询的几何制品" : topology.error?.message}} />}
      <details className="inspector-diagnostics" open={developer} onToggle={event=>setDeveloper(event.currentTarget.open)}>
        <summary>开发信息</summary>
        {developer&&<><LastRequestTime/><Button size="small" onClick={()=>void (props.view.product?api.downloadAssemblyDiagnostic(props.view.document.id):api.downloadDiagnosticBundle(props.view.document.id)).catch(error=>feedback(error,"开发诊断"))}>下载诊断数据</Button><Segmented size="small" aria-label="开发信息内容" value={developerTab} onChange={value=>setDeveloperTab(String(value))} options={[{value:"object",label:"当前对象"},{value:"failure",label:"最近结果与失败"},{value:"query",label:"查询调试"},{value:"assistant",label:"助手调用"}]}/>
          {developerTab==="assistant"?<AssistantDeveloperTrace documentId={documentID}/>:developerTab==="query"?<AutomationDebugPanel view={props.view}/>:<pre className="inspector-object-data" aria-label="开发数据">{JSON.stringify(developerTab==="failure"?evidence??{message:"暂无记录"}:inspectorObjectData(props.view,props.selection,props.feature,topology.data,properties.data,owner.data),(key,value)=>value instanceof Error?{...value,message:value.message,stack:value.stack}:value,2)}</pre>}
        </>}
      </details>
    </div>
  </>;
}
