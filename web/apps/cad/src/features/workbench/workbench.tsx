import {
  AimOutlined, ApartmentOutlined, BorderOutlined, BuildOutlined, CheckOutlined, CloudDownloadOutlined, CloudUploadOutlined,
  CompassOutlined, CompressOutlined, DatabaseOutlined, ExportOutlined, GatewayOutlined, HistoryOutlined,
  InsertRowAboveOutlined, NodeIndexOutlined, PlusSquareOutlined, RedoOutlined, SaveOutlined,
  ScissorOutlined, SelectOutlined, ShareAltOutlined, UndoOutlined,
} from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  App, Button, Descriptions, Empty, Form, Input, InputNumber, List, Modal, Segmented,
  Select, Space, Spin, Tabs, Tag, Tree, Typography,
} from "antd";
import type { DataNode } from "antd/es/tree";
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import { ShareDialog, type ShareResource } from "../../components/share-dialog";
import { CommandProvider } from "../../cad/command/command-context";
import { CommandRegistry } from "../../cad/command/command-registry";
import { FloatingToolbar, ToolbarGroup, ToolbarSeparator } from "../../cad/overlay/floating-panel";
import { ToolButton } from "../../cad/overlay/tool-button";
import { useWorkbenchStore } from "../../state/workbench-store";
import type { DocumentView, Feature, HistoryEntry, PlaneName, RectangleDraft, Selection, Vec3 } from "../../types";
import type { CadViewportHandle } from "../../viewport/cad-viewport";

const CadViewport = lazy(() => import("../../viewport/cad-viewport").then((module) => ({ default: module.CadViewport })));

function featureIcon(feature: Feature) {
  if (feature.type.toUpperCase().includes("SKETCH")) return <ScissorOutlined />;
  if (feature.type.toUpperCase() === "PAD") return <InsertRowAboveOutlined />;
  return <CloudUploadOutlined />;
}

function treeData(view: DocumentView): DataNode[] {
  if (view.document.type === "PART") return [
    {
      key: "origin", title: "Origin", icon: <GatewayOutlined />, children: (view.datumPlanes ?? []).map((plane) => ({
        key: `plane:${plane.id}:${plane.plane}`, title: plane.name, icon: <NodeIndexOutlined />,
      }))
    },
    {
      key: "features", title: "Feature list", icon: <HistoryOutlined />, children: (view.part?.features ?? []).map((feature) => ({
        key: `${feature.type.toUpperCase().includes("SKETCH") ? "sketch" : feature.type.toUpperCase() === "PAD" ? "pad" : "import"}:${feature.id}`,
        title: feature.name ?? feature.type, icon: featureIcon(feature),
      }))
    },
    ...(view.artifact ? [{ key: "solid:body-1", title: "Part body", icon: <DatabaseOutlined /> }] : []),
  ];
  return [{
    key: "assembly", title: view.document.name, icon: <ApartmentOutlined />, children: (view.product?.instances ?? []).map((instance) => ({
      key: `instance:${instance.id}`, title: instance.name, icon: <BuildOutlined />,
    }))
  }];
}

function selectionFromKey(value: React.Key): Selection {
  const [kind, id, plane] = String(value).split(":");
  if (kind === "plane") return { kind, id, plane: plane as PlaneName };
  if (["sketch", "pad", "import", "instance", "solid"].includes(kind)) return { kind: kind as Exclude<Selection, null>["kind"], id } as Selection;
  return null;
}

function selectedFeature(view: DocumentView, selection: Selection): Feature | undefined {
  if (!selection || !["sketch", "pad", "import"].includes(selection.kind)) return undefined;
  return view.part?.features.find((feature) => feature.id === selection.id);
}

export function Workbench() {
  const { documentID = "" } = useParams();
  const navigate = useNavigate();
  const client = useQueryClient();
  const { message } = App.useApp();
  const commandRegistry = useMemo(() => new CommandRegistry(), []);
  const viewport = useRef<CadViewportHandle>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const [padOpen, setPadOpen] = useState(false);
  const [insertOpen, setInsertOpen] = useState(false);
  const [versionOpen, setVersionOpen] = useState(false);
  const [shareResource, setShareResource] = useState<ShareResource>();
  const store = useWorkbenchStore();
  const document = useQuery({ queryKey: queryKeys.document(documentID), queryFn: () => api.getDocument(documentID), enabled: Boolean(documentID) });
  const history = useQuery({ queryKey: queryKeys.history(documentID), queryFn: () => api.getHistory(documentID), enabled: Boolean(documentID) });
  const catalog = useQuery({ queryKey: queryKeys.documents({ workbench: true }), queryFn: () => api.listDocuments({ limit: 100, allFolders: true }) });
  const tabDocuments = useQuery({ queryKey: ["workbench-tabs", store.tabs], queryFn: async () => Promise.all(store.tabs.map((id) => api.getDocument(id))), enabled: store.tabs.length > 0 });

  useEffect(() => { if (documentID) store.openTab(documentID); }, [documentID]);
  const refresh = useCallback(async (view?: DocumentView) => {
    if (view) client.setQueryData(queryKeys.document(view.document.id), view);
    await Promise.all([client.invalidateQueries({ queryKey: queryKeys.document(documentID) }),
    client.invalidateQueries({ queryKey: queryKeys.history(documentID) }), client.invalidateQueries({ queryKey: ["documents"] })]);
  }, [client, documentID]);
  const command = useMutation({
    mutationFn: (operation: () => Promise<DocumentView>) => operation(),
    onSuccess: async (view) => { store.setSelection(null); await refresh(view); }, onError: (error) => message.error(error.message)
  });
  const view = document.data;
  const canEdit = view?.document.permission === "OWNER" || view?.document.permission === "EDITOR";

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

  const tabs = (tabDocuments.data ?? []).map((item) => ({ key: item.document.id, label: <span>{item.document.type === "PART" ? <BuildOutlined /> : <ApartmentOutlined />} {item.document.name}</span> }));
  const selected = selectedFeature(view ?? {} as DocumentView, store.selection);
  const selectedInstance = store.selection?.kind === "instance" ? view?.product?.instances.find((instance) => instance.id === store.selection!.id) : undefined;

  if (document.isLoading) return <div className="workbench-loading"><Spin size="large" /></div>;
  if (!view) return <Empty description="无法打开文档" />;

  return <CommandProvider registry={commandRegistry}><section className="cad-workbench">
    <div className="command-ribbon">
      <div className="ribbon-group"><span>CREATE</span><Space.Compact>
        <ToolButton command="sketch.start" icon={<ScissorOutlined />} tooltip="选择基准面后创建草图" label="草图" />
        <ToolButton command="sketch.finish" icon={<CheckOutlined />} tooltip="退出草图" label="完成" />
        <ToolButton command="sketch.rectangle" icon={<PlusSquareOutlined />} tooltip="矩形 (R)" label="矩形" />
        <ToolButton command="part.pad" icon={<InsertRowAboveOutlined />} tooltip="拉伸所选草图" label="拉伸" />
      </Space.Compact></div>
      <div className="ribbon-group"><span>ASSEMBLY</span><Space.Compact>
        <ToolButton command="product.insert" icon={<ApartmentOutlined />} tooltip="插入 Part / Product" label="插入" />
        <ToolButton command="product.reference.toggle" icon={<GatewayOutlined />} tooltip="切换引用策略"
          label={selectedInstance?.referenceMode === "PINNED" ? "跟随 Head" : "固定版本"} />
      </Space.Compact></div>
      <div className="ribbon-group"><span>HISTORY</span><Space.Compact>
        <ToolButton command="history.version" icon={<SaveOutlined />} tooltip="创建命名版本" label="版本" />
        <ToolButton command="document.share" icon={<ShareAltOutlined />} tooltip="共享文档" label="共享" />
        <ToolButton command="edit.undo" icon={<UndoOutlined />} tooltip="Undo (Ctrl+Z)" />
        <ToolButton command="edit.redo" icon={<RedoOutlined />} tooltip="Redo (Ctrl+Y / Ctrl+Shift+Z)" />
      </Space.Compact></div>
      <div className="ribbon-group"><span>EXCHANGE</span><Space.Compact>
        <ToolButton command="exchange.import" icon={<CloudUploadOutlined />} tooltip="导入 STEP" label="导入" />
        <ToolButton command="exchange.export" icon={<CloudDownloadOutlined />} tooltip="导出 STEP" label="导出" />
        <input ref={fileInput} hidden type="file" accept=".step,.stp" onChange={(event) => { const file = event.target.files?.[0]; if (file) void importStep(file); }} />
      </Space.Compact></div>
      <div className="ribbon-spacer" /><Tag color={store.sketchPlane ? "blue" : "default"}>{store.sketchPlane ? `${store.sketchPlane} SKETCH · ${store.activeToolID}` : store.activeToolID}</Tag>
    </div>
    <Tabs className="document-tabs" type="editable-card" hideAdd activeKey={documentID} items={tabs}
      onChange={(key) => { store.setActiveDocument(key); navigate(`/documents/${key}`); }}
      onEdit={(key) => { store.closeTab(String(key)); const next = useWorkbenchStore.getState().activeDocumentID; navigate(next ? `/documents/${next}` : "/"); }} />
    <main className="workbench-grid">
      <aside className="feature-panel">
        <header><span><strong>{view.document.name}</strong><small>{view.document.type} · Main</small></span><Tag>{view.document.permission}</Tag></header>
        <Tree showIcon defaultExpandAll blockNode treeData={treeData(view)} selectedKeys={store.selection ? [`${store.selection.kind}:${store.selection.id}${store.selection.kind === "plane" ? `:${store.selection.plane}` : ""}`] : []}
          onSelect={(keys) => store.setSelection(keys[0] ? selectionFromKey(keys[0]) : null)} />
      </aside>
      <section className="viewport-frame">
        <Suspense fallback={<div className="viewport-loading"><Spin size="large" /></div>}><CadViewport ref={viewport} view={view} selection={store.selection}
          sketchPlane={store.sketchPlane} activeToolID={store.activeToolID} navigationProfile={store.navigationProfile}
          commandRegistry={commandRegistry} onSelectionChange={store.setSelection} onRectangleCreated={createRectangle}
          onInstanceMoved={moveInstance} /></Suspense>
        <FloatingToolbar position="top-center">
          <ToolbarGroup><ToolButton command="tool.select" icon={<SelectOutlined />} tooltip="Select (Esc)" />
            <ToolButton command="sketch.rectangle" icon={<BorderOutlined />} tooltip="Rectangle (R)" /></ToolbarGroup>
          <ToolbarSeparator />
          <ToolbarGroup><ToolButton command="navigation.profile.toggle" icon={<CompassOutlined />}
            tooltip={store.navigationProfile === "default" ? "切换到 CATIA 导航" : "切换到默认导航"} />
            <ToolButton command="view.fit" icon={<CompressOutlined />} tooltip="Fit (F)" />
            <ToolButton command="view.iso" icon={<AimOutlined />} tooltip="Isometric" /></ToolbarGroup>
        </FloatingToolbar>
        <div className="viewport-status">
          <span>{command.isPending ? "正在更新模型…" : store.selection ? `${store.selection.kind}: ${store.selection.id}` : "就绪"}</span>
          <span>{store.navigationProfile === "catia"
            ? "CATIA：M 平移/定中心 · M+L/R 旋转 · 保持 M 释放侧键后缩放"
            : "默认：右键旋转 · 中键平移"}</span>
          <span>mm</span>
        </div>
      </section>
      <aside className="inspector-panel">
        <Segmented block value={store.inspectorTab} onChange={(value) => store.setInspectorTab(value as "properties" | "history")}
          options={[{ label: "属性", value: "properties" }, { label: "历史", value: "history" }]} />
        {store.inspectorTab === "properties" ? <Properties view={view} selection={store.selection} feature={selected} />
          : <History entries={history.data ?? []} onRestore={(entry) => command.mutate(() => api.restore(view.document.id, entry.versionId))} />}
      </aside>
    </main>
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

function Properties({ view, selection, feature }: { view: DocumentView; selection: Selection; feature?: Feature }) {
  if (!selection) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="选择对象查看属性" />;
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
