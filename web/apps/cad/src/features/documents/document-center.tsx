import {
  ApartmentOutlined, BuildOutlined, ClockCircleOutlined, CopyOutlined, DeleteOutlined, EditOutlined,
  FolderAddOutlined, FolderOpenOutlined, FolderOutlined, MoreOutlined, PlusOutlined, ReloadOutlined,
  SearchOutlined, ShareAltOutlined, SwapOutlined, TeamOutlined, UndoOutlined,
} from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  App, Breadcrumb, Button, Card, Col, Dropdown, Empty, Form, Input, Layout, Menu, Modal, Pagination,
  Row, Segmented, Select, Space, Spin, Statistic, Tag, Typography,
} from "antd";
import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import { DocumentThumbnail } from "../../components/document-thumbnail";
import { ShareDialog, type ShareResource } from "../../components/share-dialog";
import { documentMetrics, flattenFolderTree, relativeDate, type LibraryScope } from "./document-utils";
import { useWorkbenchStore } from "../../state/workbench-store";
import type { DocumentSummary, FolderSummary } from "../../types";

type DocumentForm = { name: string; description?: string; type: "PART" | "PRODUCT" };
type FolderForm = { name: string; description?: string };
type DocumentOperation = { type: "copy" | "move"; document: DocumentSummary };
const canEdit = (permission?: string) => permission === "OWNER" || permission === "EDITOR";

export function DocumentCenter() {
  const navigate = useNavigate();
  const client = useQueryClient();
  const { message, modal } = App.useApp();
  const openTab = useWorkbenchStore((state) => state.openTab);
  const [scope, setScope] = useState<LibraryScope>("active");
  const [query, setQuery] = useState("");
  const [type, setType] = useState("");
  const [sort, setSort] = useState<"updated" | "name" | "created">("updated");
  const [offset, setOffset] = useState(0);
  const [currentFolderID, setCurrentFolderID] = useState("");
  const [selectedID, setSelectedID] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<DocumentSummary>();
  const [folderEditor, setFolderEditor] = useState<FolderSummary | "new">();
  const [operation, setOperation] = useState<DocumentOperation>();
  const [shareResource, setShareResource] = useState<ShareResource>();
  const [documentForm] = Form.useForm<DocumentForm>();
  const [folderForm] = Form.useForm<FolderForm>();
  const [operationForm] = Form.useForm<{ name?: string; folderID?: string }>();

  const specialScope = scope === "recent" || scope === "shared" || scope === "trash";
  const filters = { scope, query, type, sort, offset, currentFolderID };
  const documents = useQuery({ queryKey: queryKeys.documents(filters), queryFn: () => api.listDocuments({
    scope: scope === "trash" ? "trash" : "active", query,
    type: type || (scope === "parts" ? "PART" : scope === "products" ? "PRODUCT" : ""),
    folderId: specialScope || query ? undefined : currentFolderID, recent: scope === "recent", shared: scope === "shared",
    allFolders: specialScope || Boolean(query), sort: scope === "recent" ? "recent" : sort, limit: 24, offset,
  }) });
  const overview = useQuery({ queryKey: queryKeys.documents({ overview: true }), queryFn: () => api.listDocuments({ limit: 200, allFolders: true }) });
  const folders = useQuery({ queryKey: queryKeys.folders(`${scope}:${currentFolderID}`),
    queryFn: () => api.listFolders(currentFolderID, scope === "shared"), enabled: scope === "active" || scope === "parts" || scope === "products" || scope === "shared" });
  const breadcrumbs = useQuery({ queryKey: ["folder-breadcrumbs", currentFolderID], queryFn: () => api.folderBreadcrumbs(currentFolderID), enabled: Boolean(currentFolderID) });
  const folderTree = useQuery({ queryKey: ["folder-options"], queryFn: () => flattenFolderTree((parentID) => api.listFolders(parentID)), staleTime: 10_000 });
  const metrics = useMemo(() => documentMetrics(overview.data?.documents ?? []), [overview.data]);
  const currentPermission = breadcrumbs.data?.at(-1)?.permission;
  const writableLocation = !currentFolderID || canEdit(currentPermission);

  const invalidateDocuments = async () => { await client.invalidateQueries({ queryKey: ["documents"] }); };
  const invalidateFolders = async () => {
    await Promise.all([client.invalidateQueries({ queryKey: ["folders"] }), client.invalidateQueries({ queryKey: ["folder-options"] })]);
  };
  const saveDocument = useMutation({ mutationFn: async (values: DocumentForm) => editing
    ? api.updateDocument(editing.id, values.name, values.description ?? "")
    : api.createDocument(values.type, values.name, values.description ?? "", currentFolderID || undefined),
  onSuccess: async (view) => {
    const created = !editing; setCreateOpen(false); setEditing(undefined); documentForm.resetFields(); await invalidateDocuments();
    message.success("文档已保存"); if (created) { openTab(view.document.id); navigate(`/documents/${view.document.id}`); }
  }, onError: (error) => message.error(error.message) });
  const saveFolder = useMutation({ mutationFn: (values: FolderForm) => folderEditor !== "new" && folderEditor
    ? api.updateFolder(folderEditor.id, values.name, values.description ?? "")
    : api.createFolder(values.name, values.description ?? "", currentFolderID || undefined),
  onSuccess: async () => { setFolderEditor(undefined); folderForm.resetFields(); await invalidateFolders(); message.success("文件夹已保存"); },
  onError: (error) => message.error(error.message) });
  const runOperation = useMutation({ mutationFn: (values: { name?: string; folderID?: string }) => {
    if (!operation) throw new Error("没有待执行操作");
    return operation.type === "copy"
      ? api.copyDocument(operation.document.id, values.name || `${operation.document.name} Copy`, values.folderID || undefined)
      : api.moveDocument(operation.document.id, values.folderID || undefined);
  }, onSuccess: async () => { setOperation(undefined); operationForm.resetFields(); await invalidateDocuments(); message.success("操作已完成"); },
  onError: (error) => message.error(error.message) });

  const openDocument = (document: DocumentSummary) => { if (document.deletedAt) return; openTab(document.id); navigate(`/documents/${document.id}`); };
  const enterFolder = (folderID: string) => { if (scope === "shared") setScope("active"); setCurrentFolderID(folderID); setOffset(0); };
  const openDocumentEditor = (document?: DocumentSummary, documentType?: "PART" | "PRODUCT") => {
    setEditing(document); documentForm.setFieldsValue(document
      ? { name: document.name, description: document.description, type: document.type }
      : { type: documentType }); setCreateOpen(true);
  };
  const openFolderEditor = (folder?: FolderSummary) => { setFolderEditor(folder ?? "new"); folderForm.setFieldsValue(folder ?? { name: "", description: "" }); };
  const removeDocument = (document: DocumentSummary) => modal.confirm({ title: `将“${document.name}”移入回收站？`,
    content: "历史、几何制品和 Product 引用将继续保留。", okText: "移入回收站", okButtonProps: { danger: true },
    onOk: async () => { await api.deleteDocument(document.id); await invalidateDocuments(); message.success("文档已移入回收站"); } });
  const restoreDocument = async (document: DocumentSummary) => { await api.restoreDocument(document.id); await invalidateDocuments(); message.success("文档已恢复"); };
  const removeFolder = (folder: FolderSummary) => modal.confirm({ title: `删除空文件夹“${folder.name}”？`, okButtonProps: { danger: true },
    onOk: async () => { await api.deleteFolder(folder.id); await invalidateFolders(); message.success("文件夹已删除"); } });
  const chooseOperation = (kind: "copy" | "move", document: DocumentSummary) => {
    setOperation({ type: kind, document }); operationForm.setFieldsValue({ name: kind === "copy" ? `${document.name} Copy` : undefined, folderID: document.folderId });
  };

  const navItems = [
    { key: "active", icon: <FolderOutlined />, label: "全部文档" }, { key: "recent", icon: <ClockCircleOutlined />, label: "最近打开" },
    { key: "shared", icon: <TeamOutlined />, label: "与我共享" }, { key: "parts", icon: <BuildOutlined />, label: "零件" },
    { key: "products", icon: <ApartmentOutlined />, label: "产品" }, { key: "trash", icon: <DeleteOutlined />, label: "回收站" },
  ];
  const folderOptions = [{ value: "", label: "我的文档（根目录）" }, ...(folderTree.data ?? []).map((item) => ({ value: item.id, label: item.label }))];

  return <Layout className="document-center-layout">
    <Layout.Sider width={224} className="library-sider">
      <Typography.Text className="sider-caption">WORKSPACE</Typography.Text>
      <Menu mode="inline" theme="dark" selectedKeys={[scope]} items={navItems} onSelect={({ key }) => {
        setScope(key as LibraryScope); setCurrentFolderID(""); setSelectedID(""); setOffset(0);
      }} />
      <div className="workspace-note"><strong>Main Workspace</strong><span>单一连续变更线</span></div>
    </Layout.Sider>
    <Layout.Content className="library-main">
      <header className="page-heading"><div><Typography.Text className="eyebrow">DOCUMENT CENTER</Typography.Text>
        <Typography.Title level={2}>设计文档</Typography.Title><Typography.Paragraph type="secondary">管理 Part、Product、版本与工作区；双击打开文档或文件夹。</Typography.Paragraph></div>
        {!specialScope && <Space><Button icon={<FolderAddOutlined />} disabled={!writableLocation} onClick={() => openFolderEditor()}>新建文件夹</Button>
          <Button icon={<BuildOutlined />} disabled={!writableLocation} onClick={() => openDocumentEditor(undefined, "PART")}>新建 Part</Button>
          <Button type="primary" icon={<ApartmentOutlined />} disabled={!writableLocation} onClick={() => openDocumentEditor(undefined, "PRODUCT")}>新建 Product</Button></Space>}</header>
      {!specialScope && <Breadcrumb className="folder-breadcrumb" items={[
        { title: <Button type="link" onClick={() => enterFolder("")}>我的文档</Button> },
        ...(breadcrumbs.data ?? []).map((folder) => ({ title: <Button type="link" onClick={() => enterFolder(folder.id)}>{folder.name}</Button> })),
      ]} />}
      <Row gutter={14} className="metric-row">
        <Col span={8}><Card><Statistic title="Part 文档" value={metrics.parts} prefix={<BuildOutlined />} /></Card></Col>
        <Col span={8}><Card><Statistic title="Product 文档" value={metrics.products} prefix={<ApartmentOutlined />} /></Card></Col>
        <Col span={8}><Card><Statistic title="最近 7 天更新" value={metrics.recentlyUpdated} prefix={<ClockCircleOutlined />} /></Card></Col>
      </Row>
      <Card className="document-browser" bordered={false}>
        <div className="browser-controls">
          <Input allowClear prefix={<SearchOutlined />} placeholder="搜索文档名称或说明" value={query} onChange={(event) => { setQuery(event.target.value); setOffset(0); }} />
          <Segmented value={type} onChange={(value) => { setType(String(value)); setOffset(0); }} options={[{ label: "全部", value: "" }, { label: "Part", value: "PART" }, { label: "Product", value: "PRODUCT" }]} />
          <Select value={sort} onChange={setSort} options={[{ value: "updated", label: "最近修改" }, { value: "name", label: "名称" }, { value: "created", label: "创建时间" }]} />
          <Button icon={<ReloadOutlined />} onClick={() => void documents.refetch()} />
        </div>
        {folders.data?.length ? <div className="folder-grid">{folders.data.map((folder) => <Card key={folder.id} size="small" hoverable
          className="folder-card" onDoubleClick={() => enterFolder(folder.id)}>
          <div className="folder-card-content"><FolderOpenOutlined /><span><strong>{folder.name}</strong><small>{folder.documentCount} 文档 · {folder.childCount} 子文件夹</small></span>
            <Dropdown trigger={["click"]} menu={{ items: [
              ...(folder.permission === "OWNER" ? [{ key: "share", icon: <ShareAltOutlined />, label: "共享" }] : []),
              ...(canEdit(folder.permission) ? [{ key: "edit", icon: <EditOutlined />, label: "编辑" },
                { key: "delete", icon: <DeleteOutlined />, label: "删除", danger: true, disabled: folder.documentCount + folder.trashCount + folder.childCount > 0 }] : []),
            ], onClick: ({ key, domEvent }) => { domEvent.stopPropagation(); if (key === "share") setShareResource({ type: "folders", id: folder.id, name: folder.name });
              else if (key === "edit") openFolderEditor(folder); else removeFolder(folder); } }}><Button type="text" icon={<MoreOutlined />} /></Dropdown>
          </div></Card>)}</div> : null}
        {documents.isLoading ? <div className="center-spinner"><Spin /></div> : documents.data?.documents.length
          ? <div className="document-grid">{documents.data.documents.map((document) => {
            const menuItems = scope === "trash" ? [{ key: "restore", icon: <UndoOutlined />, label: "恢复", disabled: !canEdit(document.permission) }] : [
              { key: "open", label: "打开" },
              ...(canEdit(document.permission) ? [{ key: "edit", icon: <EditOutlined />, label: "编辑" }, { key: "move", icon: <SwapOutlined />, label: "移动" }] : []),
              { key: "copy", icon: <CopyOutlined />, label: "复制" },
              ...(document.permission === "OWNER" ? [{ key: "share", icon: <ShareAltOutlined />, label: "共享" }] : []),
              ...(canEdit(document.permission) ? [{ key: "delete", danger: true, icon: <DeleteOutlined />, label: "移入回收站" }] : []),
            ];
            return <Card key={document.id} hoverable className={`document-card${selectedID === document.id ? " selected" : ""}`}
              cover={<button className="thumbnail-button" onClick={() => setSelectedID(document.id)} onDoubleClick={() => openDocument(document)}
                onKeyDown={(event) => { if (event.key === "Enter") openDocument(document); }}><DocumentThumbnail document={document} /></button>}>
              <Card.Meta title={<span className="document-title"><span>{document.name}</span><Tag color={document.type === "PART" ? "blue" : "cyan"}>{document.type}</Tag></span>}
                description={<><span className="document-description">{document.description || "暂无说明"}</span><small>{document.workspaceName ?? "Main"} · {document.permission} · {relativeDate(document.lastUpdated)}</small></>} />
              <Dropdown trigger={["click"]} menu={{ items: menuItems, onClick: ({ key }) => {
                if (key === "open") openDocument(document); else if (key === "edit") openDocumentEditor(document); else if (key === "delete") removeDocument(document);
                else if (key === "restore") void restoreDocument(document); else if (key === "copy" || key === "move") chooseOperation(key, document);
                else if (key === "share") setShareResource({ type: "documents", id: document.id, name: document.name });
              } }}><Button className="card-menu" type="text" icon={<MoreOutlined />} /></Dropdown>
            </Card>;
          })}</div> : <Empty description="没有找到文档" />}
        <Pagination current={Math.floor(offset / 24) + 1} pageSize={24} total={documents.data?.total ?? 0} hideOnSinglePage onChange={(page) => setOffset((page - 1) * 24)} />
      </Card>
    </Layout.Content>

    <Modal title={editing ? "编辑文档" : "创建设计文档"} open={createOpen} onCancel={() => { setCreateOpen(false); setEditing(undefined); documentForm.resetFields(); }}
      onOk={() => documentForm.submit()} confirmLoading={saveDocument.isPending} destroyOnHidden>
      <Form form={documentForm} layout="vertical" onFinish={(values) => saveDocument.mutate(values)}>
        <Form.Item name="type" label="文档类型" rules={[{ required: true }]}><Segmented block disabled={Boolean(editing)} options={[{ label: "Part 零件", value: "PART", icon: <BuildOutlined /> }, { label: "Product 产品", value: "PRODUCT", icon: <ApartmentOutlined /> }]} /></Form.Item>
        <Form.Item name="name" label="文档名称" rules={[{ required: true, max: 120 }]}><Input autoFocus /></Form.Item>
        <Form.Item name="description" label="说明"><Input.TextArea rows={3} maxLength={500} showCount /></Form.Item>
      </Form>
    </Modal>
    <Modal title={folderEditor !== "new" && folderEditor?.id ? "编辑文件夹" : "新建文件夹"} open={Boolean(folderEditor)} onCancel={() => { setFolderEditor(undefined); folderForm.resetFields(); }}
      onOk={() => folderForm.submit()} confirmLoading={saveFolder.isPending} destroyOnHidden>
      <Form form={folderForm} layout="vertical" onFinish={(values) => saveFolder.mutate(values)}><Form.Item name="name" label="文件夹名称" rules={[{ required: true }]}><Input autoFocus /></Form.Item>
        <Form.Item name="description" label="说明"><Input.TextArea rows={3} /></Form.Item></Form>
    </Modal>
    <Modal title={operation?.type === "copy" ? "复制文档" : "移动文档"} open={Boolean(operation)} onCancel={() => setOperation(undefined)}
      onOk={() => operationForm.submit()} confirmLoading={runOperation.isPending} destroyOnHidden>
      <Form form={operationForm} layout="vertical" onFinish={(values) => runOperation.mutate(values)}>
        {operation?.type === "copy" && <Form.Item name="name" label="副本名称" rules={[{ required: true }]}><Input autoFocus /></Form.Item>}
        <Form.Item name="folderID" label="目标文件夹"><Select showSearch optionFilterProp="label" options={folderOptions} /></Form.Item>
      </Form>
    </Modal>
    <ShareDialog resource={shareResource} onClose={() => setShareResource(undefined)} />
  </Layout>;
}
