import { useQuery } from "@tanstack/react-query";
import { Alert, Segmented, Spin } from "antd";
import type { ComponentProps } from "react";
import { api } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import { useWorkbenchStore } from "../../state/workbench-store";
import type { HistoryEntry, Selection } from "../../types";
import { inspectorOwnerDocumentId } from "./inspector-object-data";
import { topologyPropertyContext } from "./topology-property-context";
import { History, Properties } from "./workbench-inspector";

type InspectorPanelProps = Omit<ComponentProps<typeof Properties>, "diagnostics" | "topology" | "topologyLoading"> & {
  documentID: string;
  canRestore: boolean;
  onRestore: (entry: HistoryEntry) => void;
};

/** Mounted only while the inspector is open; each tab owns its query lifetime. */
export function WorkbenchInspectorPanel({ documentID, canRestore, onRestore, ...props }: InspectorPanelProps) {
  const tab = useWorkbenchStore((state) => state.inspectorTab);
  const setTab = useWorkbenchStore((state) => state.setInspectorTab);
  const ownerID=inspectorOwnerDocumentId(props.view,props.selection);
  const owner=useQuery({queryKey:queryKeys.document(ownerID),queryFn:()=>api.getDocument(ownerID),
    enabled:tab==="properties"&&Boolean(props.selection)&&ownerID!==props.view.document.id,staleTime:30_000});
  const properties = useQuery({ queryKey: queryKeys.documentProperties(documentID),
    queryFn: () => api.getDocumentProperties(documentID), enabled: tab === "properties", staleTime: 30_000 });
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
  const error = tab === "history" ? history.error : topology.error ?? owner.error ?? properties.error;
  return <>
    <Segmented block aria-label="检查器内容" value={tab} onChange={(value) => setTab(value as "properties" | "history")}
      options={[{ label: "属性", value: "properties" }, { label: "历史", value: "history" }]} />
    <div className="inspector-overlay-content">
      {error && <Alert type="error" showIcon title="无法读取信息" description={error.message} />}
      {tab === "history" ? history.isPending ? <Spin size="small" />
        : <History entries={history.data ?? []} onRestore={onRestore} canRestore={canRestore} />
        : <Properties {...props} ownerView={owner.data} diagnostics={properties.data} topology={topology.data} topologyLoading={topology.isFetching}
          readErrors={{owner:owner.error?.message,diagnostics:properties.error?.message,
            topology:selection&&!selection.geometryKey ? "当前对象没有可查询的几何制品" : topology.error?.message}} />}
    </div>
  </>;
}
