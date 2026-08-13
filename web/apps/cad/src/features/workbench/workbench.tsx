import {
  AimOutlined, ApartmentOutlined, BorderOutlined, BuildOutlined, CheckOutlined, CloudDownloadOutlined, CloudUploadOutlined,
  CompassOutlined, CompressOutlined, DatabaseOutlined, ExportOutlined, GatewayOutlined, HistoryOutlined,
  InsertRowAboveOutlined, MenuFoldOutlined, MenuUnfoldOutlined, NodeIndexOutlined, RedoOutlined, SaveOutlined,
  ScissorOutlined, SelectOutlined, ShareAltOutlined, UndoOutlined,
} from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  App, Button, Descriptions, Empty, Form, Input, InputNumber, List, Modal, Segmented,
  Select, Space, Spin, Tag,
} from "antd";
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { api, isMockMode } from "../../api/client";
import { realtime } from "../../api/realtime-client";
import { queryKeys } from "../../app/query-keys";
import { ShareDialog, type ShareResource } from "../../components/share-dialog";
import { CommandProvider } from "../../cad/command/command-context";
import { CommandRegistry } from "../../cad/command/command-registry";
import { FloatingToolbar, ToolbarGroup, ToolbarSeparator } from "../../cad/overlay/floating-panel";
import { ToolButton } from "../../cad/overlay/tool-button";
import { CAD_WORKBENCHES, resolveCadWorkbench } from "../../cad/workbench/cad-workbench";
import { useWorkbenchStore } from "../../state/workbench-store";
import type { DocumentProperties, DocumentStructureNode, DocumentView, Feature, HistoryEntry, PlaneName, RectangleDraft, Selection, TopologyElementProperties, Vec3 } from "../../types";
import type { CadViewportHandle } from "../../viewport/cad-viewport";
import { SpecificationTree, type SpecificationTreeNode } from "./specification-tree";

const CadViewport = lazy(() => import("../../viewport/cad-viewport").then((module) => ({ default: module.CadViewport })));

function featureIcon(feature: Feature) {
  if (feature.type.toUpperCase().includes("SKETCH")) return <ScissorOutlined />;
  if (feature.type.toUpperCase() === "PAD") return <InsertRowAboveOutlined />;
  return <CloudUploadOutlined />;
}

function featureNode(feature: Feature): SpecificationTreeNode {
  return {
    key: `${feature.type.toUpperCase().includes("SKETCH") ? "sketch" : feature.type.toUpperCase() === "PAD" ? "pad" : "import"}:${feature.id}`,
    title: feature.name ?? feature.type, icon: featureIcon(feature),
  };
}

function structureIcon(kind: DocumentStructureNode["kind"]) {
  if (kind === "PRODUCT") return <ApartmentOutlined />;
  if (kind === "PART" || kind === "INSTANCE") return <BuildOutlined />;
  if (kind === "ORIGIN") return <GatewayOutlined />;
  if (kind === "PLANE") return <NodeIndexOutlined />;
  if (kind === "AXIS_SYSTEM") return <AimOutlined />;
  if (kind === "AXIS") return <NodeIndexOutlined />;
  if (kind === "BODY") return <DatabaseOutlined />;
  if (kind === "SKETCH") return <ScissorOutlined />;
  if (kind === "PAD") return <InsertRowAboveOutlined />;
  return <CloudUploadOutlined />;
}

function structureSelection(node: DocumentStructureNode, view: DocumentView): Selection {
  const resolved = [...(view.resolvedInstances ?? [])].sort((a, b) => b.bodyTreeNodeId.length - a.bodyTreeNodeId.length)
    .find((item) => node.id.startsWith(item.bodyTreeNodeId.replace(/\/body$/, "")));
  const occurrencePath = resolved?.occurrencePath ?? "";
  const geometryKey = resolved?.geometryKey ?? view.artifact?.geometryKey;
  const context = { treeNodeId: node.id, documentId: node.documentId, occurrencePath, geometryKey,
    instanceId: occurrencePath.split("/")[0] || undefined };
  if (node.kind === "INSTANCE" && node.entityId) {
    const path = [...node.id.matchAll(/\/instance:([^/]+)/g)].map((match) => match[1]).join("/") || node.entityId;
    return { kind: "instance", id: path, visualKey: `occurrence:${path}`, ...context,
      occurrencePath: path, instanceId: path.split("/")[0] };
  }
  if (node.kind === "PLANE" && node.entityId && node.plane) return {
    kind: "plane", id: `${occurrencePath || "root"}:${node.entityId}`, plane: node.plane, ...context,
  };
  if (node.kind === "AXIS_SYSTEM" && node.entityId) return {
    kind: "axis-system", id: `${occurrencePath || "root"}:${node.entityId}`, ...context,
  };
  if (node.kind === "AXIS" && node.entityId && node.axis) return {
    kind: "axis", axis: node.axis, id: `${occurrencePath || "root"}:${node.entityId}:${node.axis}`, ...context,
  };
  const bodyID = `${occurrencePath || "root"}:body`;
  if (node.kind === "BODY" || node.kind === "PART") return { kind: "body", id: bodyID, ...context };
  if (["SKETCH", "PAD", "IMPORT"].includes(node.kind) && node.entityId) return {
    kind: node.kind.toLowerCase() as "sketch" | "pad" | "import", id: node.entityId,
    visualKey: node.kind === "SKETCH" ? undefined : `body:${bodyID}`, ...context,
  };
  return null;
}

function mapStructureNode(node: DocumentStructureNode, view: DocumentView): SpecificationTreeNode {
  return { key: node.id, title: node.name, icon: structureIcon(node.kind), kind: node.kind,
    entityId: node.entityId, documentId: node.documentId, plane: node.plane,
    selection: structureSelection(node, view), children: node.children?.map((child) => mapStructureNode(child, view)) };
}

function treeData(view: DocumentView): SpecificationTreeNode[] {
  if (view.structureTree) return [mapStructureNode(view.structureTree, view)];
  if (view.document.type === "PART") {
    const features = view.part?.features ?? [];
    const sketches = new Map(features.filter((feature) => feature.type.toUpperCase().includes("SKETCH"))
      .map((feature) => [feature.id, feature]));
    const consumedSketches = new Set(features.filter((feature) => feature.type.toUpperCase() === "PAD" && feature.profile)
      .map((feature) => feature.profile!));
    const bodyFeatures = features.filter((feature) => !consumedSketches.has(feature.id)).map((feature) => {
      const node = featureNode(feature);
      const profile = feature.type.toUpperCase() === "PAD" && feature.profile ? sketches.get(feature.profile) : undefined;
      return profile ? { ...node, children: [featureNode(profile)] } : node;
    });
    return [{ key: "document", title: view.document.name, icon: <BuildOutlined />, children: [
      { key: "origin", title: "Origin", icon: <GatewayOutlined />, children: (view.datumPlanes ?? []).map((plane) => ({
        key: `plane:${plane.id}:${plane.plane}`, title: plane.name, icon: <NodeIndexOutlined />,
      })).concat((view.axisSystems ?? []).map((axis) => ({
        key: `axis:${axis.id}`, title: axis.name, icon: <AimOutlined />,
      }))) },
      { key: "body", title: "PartBody", icon: <DatabaseOutlined />, children: bodyFeatures },
    ] }];
  }
  return [{ key: "document", title: view.document.name, icon: <ApartmentOutlined />, children:
    (view.product?.instances ?? []).map((instance) => ({
      key: `instance:${instance.id}`, title: instance.name, icon: <BuildOutlined />,
    })) }];
}

function selectedFeature(view: DocumentView, selection: Selection): Feature | undefined {
  if (!selection || !["sketch", "pad", "import"].includes(selection.kind)) return undefined;
  return view.part?.features.find((feature) => feature.id === selection.id);
}

function treeKeyForSelection(nodes: SpecificationTreeNode[], selection: Selection): string | undefined {
  if (!selection) return undefined;
  const visit = (items: SpecificationTreeNode[]): string | undefined => {
    for (const node of items) {
      if (selection.treeNodeId === node.key || (selection.documentId && node.documentId === selection.documentId &&
        selection.id === node.entityId)) return node.key;
      const child = node.children ? visit(node.children) : undefined;
      if (child) return child;
    }
    return undefined;
  };
  return visit(nodes);
}

export function Workbench() {
  const { documentID = "" } = useParams();
  const client = useQueryClient();
  const { message } = App.useApp();
  const commandRegistry = useMemo(() => new CommandRegistry(), []);
  const viewport = useRef<CadViewportHandle>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const [padOpen, setPadOpen] = useState(false);
  const [insertOpen, setInsertOpen] = useState(false);
  const [versionOpen, setVersionOpen] = useState(false);
  const [inspectorOpen, setInspectorOpen] = useState(true);
  const [shareResource, setShareResource] = useState<ShareResource>();
  const store = useWorkbenchStore();
  const document = useQuery({ queryKey: queryKeys.document(documentID), queryFn: () => api.getDocument(documentID), enabled: Boolean(documentID) });
  const properties = useQuery({ queryKey: queryKeys.documentProperties(documentID), queryFn: () => api.getDocumentProperties(documentID), enabled: Boolean(documentID) });
  const history = useQuery({ queryKey: queryKeys.history(documentID), queryFn: () => api.getHistory(documentID), enabled: Boolean(documentID) });
  const catalog = useQuery({ queryKey: queryKeys.documents({ workbench: true }), queryFn: () => api.listDocuments({ limit: 100, allFolders: true }) });
  const topologySelection = store.selection && ["face", "edge", "vertex"].includes(store.selection.kind)
    ? store.selection as Extract<Exclude<Selection, null>, { kind: "face" | "edge" | "vertex" }> : undefined;
  const topology = useQuery({
    queryKey: topologySelection ? queryKeys.topologyProperties(documentID, topologySelection.geometryKey ?? "",
      topologySelection.kind, topologySelection.topologyId) : ["topology-properties", "none"],
    queryFn: () => api.getTopologyProperties(documentID, topologySelection!.geometryKey!,
      topologySelection!.kind.toUpperCase() as "FACE" | "EDGE" | "VERTEX", topologySelection!.topologyId),
    enabled: Boolean(documentID && topologySelection?.geometryKey), staleTime: 5 * 60_000,
  });

  useEffect(() => {
    if (document.data) void client.invalidateQueries({ queryKey: queryKeys.openDocuments });
  }, [client, document.data]);
  const refresh = useCallback(async (view?: DocumentView) => {
    if (view) client.setQueryData(queryKeys.document(view.document.id), view);
    await Promise.all([client.invalidateQueries({ queryKey: queryKeys.document(documentID) }),
    client.invalidateQueries({ queryKey: queryKeys.history(documentID) }), client.invalidateQueries({ queryKey: queryKeys.documentProperties(documentID) }),
    client.invalidateQueries({ queryKey: ["documents"] }),
    client.invalidateQueries({ queryKey: queryKeys.openDocuments })]);
  }, [client, documentID]);
  useEffect(() => {
    if (!documentID || isMockMode) return;
    let disposed = false;
    let unsubscribe: (() => void) | undefined;
    void realtime.subscribe(documentID, (event) => {
      if (event.type === "document.snapshot.v1") {
        const snapshot = event.payload as { view: DocumentView };
        client.setQueryData(queryKeys.document(documentID), snapshot.view);
      } else {
        useWorkbenchStore.getState().setSelection(null);
      }
      void Promise.all([
        client.invalidateQueries({ queryKey: queryKeys.document(documentID) }),
        client.invalidateQueries({ queryKey: queryKeys.history(documentID) }),
        client.invalidateQueries({ queryKey: queryKeys.documentProperties(documentID) }),
        client.invalidateQueries({ queryKey: ["documents"] }),
        client.invalidateQueries({ queryKey: queryKeys.openDocuments }),
      ]);
    }).then((dispose) => {
      if (disposed) dispose(); else unsubscribe = dispose;
    }).catch((error: Error) => {
      if (!disposed) message.error(`实时连接失败：${error.message}`);
    });
    return () => { disposed = true; unsubscribe?.(); };
  }, [client, documentID, message]);
  const command = useMutation({
    mutationFn: (operation: () => Promise<DocumentView>) => operation(),
    onSuccess: async (view) => { store.setSelection(null); await refresh(view); }, onError: (error) => message.error(error.message)
  });
  const view = document.data;
  const treeNodes = useMemo(() => view ? treeData(view) : [], [view]);
  const canEdit = view?.document.permission === "OWNER" || view?.document.permission === "EDITOR";
  const activeWorkbench = resolveCadWorkbench(view?.document.type ?? "PART", Boolean(store.sketchPlane));

  const createRectangle = (draft: RectangleDraft) => {
    if (!view) return;
    command.mutate(() => api.createSketch(view.document.id, draft.plane, draft.origin, draft.width, draft.height));
    store.setActiveTool("select");
  };
  const moveInstance = (instanceID: string, translation: Vec3) => {
    if (view && canEdit) command.mutate(() => api.move(view.document.id, instanceID, translation));
  };
  const executeHistory = (direction: "undo" | "redo") => {
    if (!view) return; command.mutate(() => direction === "undo" ? api.undo(view.document.id) : api.redo(view.document.id));
  };
  const startSketch = () => {
    if (store.selection?.kind !== "plane") return;
    store.beginSketch(store.selection.plane);
  };
  const padSketch = (values: { length: number }) => {
    if (!view || store.selection?.kind !== "sketch") return;
    command.mutate(() => api.pad(view.document.id, store.selection!.id, values.length)); setPadOpen(false);
  };
  const insertDocument = (values: { referencedDocumentID: string; name: string }) => {
    if (!view) return; command.mutate(() => api.insert(view.document.id, values.referencedDocumentID, values.name)); setInsertOpen(false);
  };
  const createVersion = async (values: { name: string; description: string }) => {
    if (!view) return; await api.createVersion(view.document.id, values.name, values.description); setVersionOpen(false);
    await history.refetch(); message.success("版本已创建");
  };
  const importStep = async (file: File) => {
    if (!view) return;
    try { const job = await api.importStep(view.document.id, file); message.success(`STEP 导入任务已创建：${job.id}`); window.setTimeout(() => void refresh(), 1200); }
    catch (error) { message.error((error as Error).message); }
  };
  const exportStep = async () => {
    if (!view) return;
    try {
      let job = await api.startExportStep(view.document.id);
      for (let attempt = 0; attempt < 30 && !["SUCCEEDED", "FAILED"].includes(job.state); attempt++) {
        await new Promise((resolve) => window.setTimeout(resolve, 500)); job = await api.getJob(job.id);
      }
      if (job.state !== "SUCCEEDED") throw new Error(job.errorMessage ?? "STEP 导出失败");
      await api.downloadJob(job.id); message.success("STEP 导出完成");
    } catch (error) { message.error((error as Error).message); }
  };
  useEffect(() => {
    const selectedInstance = () => {
      const selection = useWorkbenchStore.getState().selection;
      return selection?.kind === "instance" ? view?.product?.instances.find((instance) => instance.id === selection.id) : undefined;
    };
    const disposers = [
      commandRegistry.register({ id: "tool.select", execute: () => store.setActiveTool("select"),
        isActive: () => store.activeToolID === "select" }),
      commandRegistry.register({ id: "sketch.start", execute: startSketch,
        isVisible: () => view?.document.type === "PART", isEnabled: () => Boolean(canEdit && store.selection?.kind === "plane") }),
      commandRegistry.register({ id: "sketch.finish", execute: store.endSketch,
        isVisible: () => Boolean(store.sketchPlane), isEnabled: () => Boolean(canEdit) }),
      commandRegistry.register({ id: "sketch.rectangle", execute: () => store.setActiveTool("sketch.rectangle"),
        isVisible: () => Boolean(store.sketchPlane), isEnabled: () => Boolean(canEdit && store.sketchPlane),
        isActive: () => store.activeToolID === "sketch.rectangle" }),
      commandRegistry.register({ id: "part.pad", execute: () => setPadOpen(true), isVisible: () => view?.document.type === "PART",
        isEnabled: () => Boolean(canEdit && store.selection?.kind === "sketch") }),
      commandRegistry.register({ id: "product.insert", execute: () => setInsertOpen(true), isVisible: () => view?.document.type === "PRODUCT",
        isEnabled: () => Boolean(canEdit) }),
      commandRegistry.register({ id: "product.reference.toggle", execute: () => {
        const instance = selectedInstance();
        if (view && instance) command.mutate(() => api.setReferenceMode(view.document.id, instance.id,
          instance.referenceMode === "PINNED" ? "FOLLOW_HEAD" : "PINNED"));
      }, isVisible: () => view?.document.type === "PRODUCT", isEnabled: () => Boolean(canEdit && selectedInstance()) }),
      commandRegistry.register({ id: "history.version", execute: () => setVersionOpen(true), isEnabled: () => Boolean(canEdit) }),
      commandRegistry.register({ id: "document.share", execute: () => view && setShareResource({ type: "documents", id: view.document.id, name: view.document.name }),
        isEnabled: () => view?.document.permission === "OWNER" }),
      commandRegistry.register({ id: "edit.undo", execute: () => executeHistory("undo"),
        isEnabled: () => Boolean(canEdit && view?.document.canUndo && !command.isPending) }),
      commandRegistry.register({ id: "edit.redo", execute: () => executeHistory("redo"),
        isEnabled: () => Boolean(canEdit && view?.document.canRedo && !command.isPending) }),
      commandRegistry.register({ id: "exchange.import", execute: () => fileInput.current?.click(),
        isVisible: () => view?.document.type === "PART", isEnabled: () => Boolean(canEdit) }),
      commandRegistry.register({ id: "exchange.export", execute: exportStep, isVisible: () => view?.document.type === "PART",
        isEnabled: () => Boolean(view?.artifact) }),
      commandRegistry.register({ id: "view.fit", execute: () => viewport.current?.fit() }),
      commandRegistry.register({ id: "view.top", execute: () => viewport.current?.setStandardView("TOP") }),
      commandRegistry.register({ id: "view.front", execute: () => viewport.current?.setStandardView("FRONT") }),
      commandRegistry.register({ id: "view.right", execute: () => viewport.current?.setStandardView("RIGHT") }),
      commandRegistry.register({ id: "view.iso", execute: () => viewport.current?.setStandardView("ISO") }),
      commandRegistry.register({ id: "navigation.profile.toggle", execute: () => store.setNavigationProfile(
        store.navigationProfile === "default" ? "catia" : "default"), isActive: () => store.navigationProfile === "catia" }),
    ];
    return () => { for (const dispose of disposers.reverse()) dispose(); };
  }, [commandRegistry, view, canEdit, store.selection, store.sketchPlane, store.activeToolID, store.navigationProfile, command.isPending]);

  useEffect(() => { commandRegistry.notifyStateChanged(); }, [commandRegistry, view, store.selection, store.sketchPlane,
    store.activeToolID, store.navigationProfile, command.isPending]);

  const selected = selectedFeature(view ?? {} as DocumentView, store.selection);
  const selectedInstance = store.selection?.kind === "instance" ? view?.product?.instances.find((instance) => instance.id === store.selection!.id) : undefined;

  if (document.isLoading) return <div className="workbench-loading"><Spin size="large" /></div>;
  if (!view) return <Empty description="无法打开文档" />;

  return <CommandProvider registry={commandRegistry}><section className="cad-workbench">
    <main className="workbench-stage"><section className={`viewport-frame ${inspectorOpen ? "inspector-open" : ""}`}>
        <Suspense fallback={<div className="viewport-loading"><Spin size="large" /></div>}><CadViewport ref={viewport} view={view} selection={store.selection}
          preselection={store.preselection}
          sketchPlane={store.sketchPlane} activeToolID={store.activeToolID} navigationProfile={store.navigationProfile}
          commandRegistry={commandRegistry} onSelectionChange={store.setSelection} onPreselectionChange={store.setPreselection} onRectangleCreated={createRectangle}
          onInstanceMoved={moveInstance} /></Suspense>
        {activeWorkbench === "PART_DESIGN" && <FloatingToolbar id="part-design" label="Part Design" position="top-left" className="part-design-toolbar">
          <ToolbarGroup><ToolButton command="tool.select" icon={<SelectOutlined />} tooltip="选择 (Esc)" />
            <ToolButton command="sketch.start" icon={<ScissorOutlined />} tooltip="选择基准面后创建草图" />
            <ToolButton command="part.pad" icon={<InsertRowAboveOutlined />} tooltip="拉伸所选草图" /></ToolbarGroup>
          <ToolbarSeparator />
          <ToolbarGroup><ToolButton command="exchange.import" icon={<CloudUploadOutlined />} tooltip="导入 STEP" />
            <ToolButton command="exchange.export" icon={<CloudDownloadOutlined />} tooltip="导出 STEP" /></ToolbarGroup>
        </FloatingToolbar>}
        {activeWorkbench === "SKETCHER" && <FloatingToolbar id="sketcher" label="Sketcher" position="top-left" className="sketcher-toolbar">
          <ToolbarGroup><ToolButton command="tool.select" icon={<SelectOutlined />} tooltip="选择 (Esc)" />
            <ToolButton command="sketch.rectangle" icon={<BorderOutlined />} tooltip="矩形 (R)" />
            <ToolButton command="sketch.finish" icon={<CheckOutlined />} tooltip="退出草图" /></ToolbarGroup>
        </FloatingToolbar>}
        {activeWorkbench === "ASSEMBLY_DESIGN" && <FloatingToolbar id="assembly-design" label="Assembly Design" position="top-left" className="assembly-design-toolbar">
          <ToolbarGroup><ToolButton command="tool.select" icon={<SelectOutlined />} tooltip="选择 (Esc)" />
            <ToolButton command="product.insert" icon={<ApartmentOutlined />} tooltip="插入 Part / Product" />
            <ToolButton command="product.reference.toggle" icon={<GatewayOutlined />}
              tooltip={selectedInstance?.referenceMode === "PINNED" ? "切换为跟随 Head" : "固定当前版本"} /></ToolbarGroup>
        </FloatingToolbar>}
        <FloatingToolbar id="common-edit" label="Common" position="top-center" className="common-toolbar">
          <ToolbarGroup><ToolButton command="edit.undo" icon={<UndoOutlined />} tooltip="Undo (Ctrl+Z)" />
            <ToolButton command="edit.redo" icon={<RedoOutlined />} tooltip="Redo (Ctrl+Y / Ctrl+Shift+Z)" />
            <ToolButton command="history.version" icon={<SaveOutlined />} tooltip="创建命名版本" />
            <ToolButton command="document.share" icon={<ShareAltOutlined />} tooltip="共享文档" /></ToolbarGroup>
        </FloatingToolbar>
        <FloatingToolbar id="view" label="View" position="top-right">
          <ToolbarGroup><ToolButton command="navigation.profile.toggle" icon={<CompassOutlined />}
            tooltip={store.navigationProfile === "default" ? "切换到 CATIA 导航" : "切换到默认导航"} />
            <ToolButton command="view.fit" icon={<CompressOutlined />} tooltip="Fit (F)" />
            <ToolButton command="view.iso" icon={<AimOutlined />} tooltip="Isometric" /></ToolbarGroup>
        </FloatingToolbar>
        <input ref={fileInput} hidden type="file" accept=".step,.stp" onChange={(event) => {
          const file = event.target.files?.[0]; if (file) void importStep(file);
        }} />
        <aside className="floating-structure-tree">
          <SpecificationTree nodes={treeNodes} selectedKey={treeKeyForSelection(treeNodes, store.selection)}
            highlightedKey={treeKeyForSelection(treeNodes, store.preselection)}
            onSelect={(node) => store.setSelection(node.selection ?? null)}
            onHover={(node) => store.setPreselection(node?.selection ?? null)} />
        </aside>
        <button className={`inspector-toggle ${inspectorOpen ? "open" : ""}`} onClick={() => setInspectorOpen((current) => !current)}
          title={inspectorOpen ? "收起属性面板" : "展开属性面板"}>
          {inspectorOpen ? <MenuFoldOutlined /> : <MenuUnfoldOutlined />}
        </button>
        <aside className={`inspector-overlay ${inspectorOpen ? "open" : ""}`}>
          <Segmented block value={store.inspectorTab} onChange={(value) => store.setInspectorTab(value as "properties" | "history")}
            options={[{ label: "属性", value: "properties" }, { label: "历史", value: "history" }]} />
          <div className="inspector-overlay-content">{store.inspectorTab === "properties"
            ? <Properties view={view} selection={store.selection} feature={selected}
              workbench={activeWorkbench} sketchPlane={store.sketchPlane} activeTool={store.activeToolID}
              navigationProfile={store.navigationProfile} diagnostics={properties.data}
              topology={topology.data} topologyLoading={topology.isLoading} />
            : <History entries={history.data ?? []} onRestore={(entry) => command.mutate(() => api.restore(view.document.id, entry.versionId))} />}</div>
        </aside>
        <div className="viewport-status">
          <span>{command.isPending ? "正在更新模型…" : store.selection ? `${store.selection.kind}: ${store.selection.id}` : "就绪"}</span>
          <span>{store.navigationProfile === "catia"
            ? "CATIA：M 平移/定中心 · M+L/R 旋转 · 保持 M 释放侧键后缩放"
            : "默认：右键旋转 · 中键平移"}</span>
          <span>mm</span>
        </div>
      </section></main>
    <Modal title="拉伸草图" open={padOpen} onCancel={() => setPadOpen(false)} footer={null} destroyOnHidden>
      <Form layout="vertical" initialValues={{ length: 40 }} onFinish={padSketch}><Form.Item name="length" label="拉伸长度（mm）" rules={[{ required: true }]}><InputNumber min={0.1} precision={2} style={{ width: "100%" }} /></Form.Item>
        <Button block type="primary" htmlType="submit" loading={command.isPending}>确定拉伸</Button></Form>
    </Modal>
    <Modal title="插入 Part / Product" open={insertOpen} onCancel={() => setInsertOpen(false)} footer={null} destroyOnHidden>
      <Form layout="vertical" onFinish={insertDocument}><Form.Item name="referencedDocumentID" label="引用文档" rules={[{ required: true }]}><Select showSearch optionFilterProp="label" options={(catalog.data?.documents ?? []).filter((item) => item.id !== view.document.id).map((item) => ({ value: item.id, label: `${item.name} (${item.type})` }))} /></Form.Item>
        <Form.Item name="name" label="实例名称" rules={[{ required: true }]}><Input /></Form.Item><Button block type="primary" htmlType="submit">插入实例</Button></Form>
    </Modal>
    <Modal title="创建命名版本" open={versionOpen} onCancel={() => setVersionOpen(false)} footer={null} destroyOnHidden>
      <Form layout="vertical" onFinish={(values) => void createVersion(values)}><Form.Item name="name" label="版本名称" rules={[{ required: true }]}><Input placeholder="V1 - Initial concept" /></Form.Item>
        <Form.Item name="description" label="说明"><Input.TextArea rows={3} /></Form.Item><Button block type="primary" htmlType="submit">创建版本</Button></Form>
    </Modal>
    <ShareDialog resource={shareResource} onClose={() => setShareResource(undefined)} />
  </section></CommandProvider>;
}

function Properties({ view, selection, feature, workbench, sketchPlane, activeTool, navigationProfile, diagnostics, topology, topologyLoading }: {
  view: DocumentView;
  selection: Selection;
  feature?: Feature;
  workbench: keyof typeof CAD_WORKBENCHES;
  sketchPlane?: PlaneName;
  activeTool: string;
  navigationProfile: string;
  diagnostics?: DocumentProperties;
  topology?: TopologyElementProperties;
  topologyLoading?: boolean;
}) {
  if (!selection) {
    const triangleCount = view.artifact?.mesh.triangles.length
      ?? (view.resolvedInstances ?? []).reduce((total, instance) =>
        total + (view.artifacts?.[instance.geometryKey]?.mesh.triangles.length ?? 0), 0);
    const geometryCount = diagnostics?.aggregate.artifactCount
      ?? (view.document.type === "PRODUCT" ? view.resolvedInstances?.length ?? 0 : view.artifact ? 1 : 0);
    const detail = diagnostics?.artifacts[0];
    const bytes = (value = 0) => value < 1024 ? `${value} B` : `${(value / 1024).toFixed(1)} KiB`;
    return <><div className="property-context-hint">未选择对象 · 当前工作环境</div><Descriptions column={1} size="small"
      bordered className="property-list" items={[
        { key: "workbench", label: "Workbench", children: `${CAD_WORKBENCHES[workbench].label} · ${CAD_WORKBENCHES[workbench].domain}` },
        { key: "document", label: "文档", children: `${view.document.name} (${view.document.type})` },
        { key: "workspace", label: "工作区", children: view.document.workspaceName ?? "Main" },
        { key: "permission", label: "权限", children: view.document.permission },
        { key: "version", label: "Head Version", children: view.document.versionId.slice(0, 16) },
        { key: "tool", label: "Active Tool", children: activeTool },
        { key: "navigation", label: "Navigation", children: navigationProfile.toUpperCase() },
        ...(sketchPlane ? [{ key: "plane", label: "Sketch Plane", children: sketchPlane }] : []),
        { key: "history", label: "History", children: `Undo ${view.document.canUndo ? "Yes" : "No"} · Redo ${view.document.canRedo ? "Yes" : "No"}` },
        { key: "geometry", label: "Display Geometry", children: `${geometryCount} object(s) · ${triangleCount} triangles` },
        { key: "topology", label: "Topology", children: diagnostics
          ? `${diagnostics.aggregate.solidCount} solid(s) · ${diagnostics.aggregate.vertexCount} vertices` : "Loading…" },
        { key: "artifact", label: "Geometry Key", children: detail?.geometryKey ?? "—" },
        { key: "geometry-id", label: "Geometry ID", children: detail?.geometryId ?? "—" },
        { key: "evaluator", label: "Evaluator", children: detail?.evaluatorVersion ?? "—" },
        { key: "artifacts", label: "Artifacts", children: diagnostics
          ? `GLB ${bytes(diagnostics.aggregate.glbBytes)} · B-Rep ${bytes(diagnostics.aggregate.brepBytes)}` : "Loading…" },
        { key: "storage", label: "Storage", children: detail?.storageState ?? "—" },
        { key: "artifact-worker", label: "Artifact Worker", children: detail?.workerId ?? "—" },
        { key: "worker", label: "Worker Route", children: diagnostics?.worker.available
          ? `${diagnostics.worker.workerId} · ${diagnostics.worker.residentGeometryCount} resident` : diagnostics?.worker.error ?? "Loading…" },
        { key: "occt", label: "OCCT", children: diagnostics?.worker.occtVersion ?? detail?.occtVersion ?? "—" },
        { key: "reference", label: "Reference Geometry", children: detail
          ? `${detail.referenceGeometry.datumPlanes.length} planes · ${detail.referenceGeometry.axisSystems.length} axis system(s)` : "—" },
        { key: "rendering", label: "Rendering", children: "Phong Solid + welded feature edges" },
        { key: "features", label: view.document.type === "PART" ? "Features" : "Instances",
          children: view.document.type === "PART" ? view.part?.features.length ?? 0 : view.product?.instances.length ?? 0 },
      ]} /></>;
  }
  if (["face", "edge", "vertex"].includes(selection.kind)) {
    const format = (value: unknown): string => Array.isArray(value)
      ? value.map((entry) => typeof entry === "number" ? Number(entry).toPrecision(7) : String(entry)).join(", ")
      : typeof value === "number" ? Number(value).toPrecision(9) : String(value);
    if (topologyLoading || !topology) return <Spin size="small" tip="从 Geometry Worker 读取 B-Rep…" />;
    return <><div className="property-context-hint">OCCT B-Rep 拓扑属性</div><Descriptions column={1} size="small"
      bordered className="property-list" items={[
        { key: "kind", label: "Topology", children: `${topology.kind} #${topology.localId}` },
        { key: "geometry-type", label: "Geometry", children: topology.geometryType },
        { key: "geometry-id", label: "Geometry ID", children: topology.geometryId },
        { key: "worker", label: "Worker", children: topology.workerId },
        { key: "occt", label: "OCCT", children: topology.occtVersion },
        ...(topology.point ? [{ key: "point", label: "Point", children: format(topology.point) }] : []),
        ...Object.entries(topology.properties).map(([key, value]) => ({ key: `brep-${key}`, label: key, children: format(value) })),
      ]} /></>;
  }
  if (selection.kind === "axis" || selection.kind === "axis-system") {
    const systems = view.axisSystems ?? diagnostics?.artifacts.flatMap((item) => item.referenceGeometry.axisSystems) ?? [];
    const axisSystem = systems.find((item) => selection.id.includes(item.id));
    const direction = selection.kind === "axis" && axisSystem
      ? selection.axis === "X" ? axisSystem.xDirection : selection.axis === "Y" ? axisSystem.yDirection : axisSystem.zDirection
      : undefined;
    return <Descriptions column={1} size="small" bordered className="property-list" items={[
      { key: "type", label: "类型", children: selection.kind === "axis" ? `${selection.axis} Axis` : "Axis System" },
      { key: "name", label: "名称", children: axisSystem?.name ?? selection.id },
      { key: "origin", label: "原点", children: axisSystem?.origin.join(", ") ?? "—" },
      ...(direction ? [{ key: "direction", label: "方向", children: direction.join(", ") }] : []),
      { key: "reference", label: "引用路径", children: selection.occurrencePath || "Part root" },
    ]} />;
  }
  const instance = selection.kind === "instance" ? view.product?.instances.find((item) => item.id === selection.id) : undefined;
  return <Descriptions column={1} size="small" bordered className="property-list" items={[
    { key: "type", label: "类型", children: selection.kind.toUpperCase() },
    { key: "name", label: "名称", children: feature?.name ?? instance?.name ?? selection.id },
    ...(selection.kind === "plane" ? [{ key: "plane", label: "基准面", children: selection.plane }] : []),
    ...(feature?.rectangle ? [{ key: "size", label: "尺寸", children: `${feature.rectangle.width} × ${feature.rectangle.height} mm` }] : []),
    ...(feature?.length ? [{ key: "length", label: "长度", children: `${feature.length} mm` }] : []),
    ...(instance ? [{ key: "transform", label: "位移", children: instance.translation.map((value) => value.toFixed(2)).join(", ") },
    { key: "reference", label: "引用", children: instance.referenceMode ?? "FOLLOW_HEAD" }] : []),
  ]} />;
}

function History({ entries, onRestore }: { entries: HistoryEntry[]; onRestore: (entry: HistoryEntry) => void }) {
  return <List className="history-list" dataSource={[...entries].reverse()} renderItem={(entry) => <List.Item actions={!entry.isHead ? [<Button key="restore" type="link" icon={<ExportOutlined />} onClick={() => onRestore(entry)}>恢复</Button>] : []}>
    <List.Item.Meta title={<Space><span>#{entry.sequence} {entry.commandType}</span>{entry.isHead && <Tag color="blue">HEAD</Tag>}</Space>}
      description={<span>{entry.versionName ?? entry.versionId.slice(0, 12)}<br />{new Date(entry.createdAt).toLocaleString()}</span>} />
  </List.Item>} />;
}
