import {
  ApartmentOutlined, BuildOutlined, ClockCircleOutlined, CopyOutlined, DeleteOutlined, DownloadOutlined, EditOutlined,
  FolderAddOutlined, FolderOpenOutlined, FolderOutlined, MoreOutlined, PlusOutlined, ReloadOutlined,
  SearchOutlined, ShareAltOutlined, SwapOutlined, TeamOutlined, UndoOutlined, UploadOutlined, InboxOutlined,
} from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  App, Breadcrumb, Button, Card, Checkbox, Dropdown, Empty, Form, Input, Layout, Menu, Modal, Pagination,
  Segmented, Select, Space, Spin, Tag, Typography, Upload,
} from "antd";
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, isMockMode } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import { DocumentThumbnail } from "../../components/document-thumbnail";
import { ShareDialog, type ShareResource } from "../../components/share-dialog";
import { defaultDocumentName, flattenFolderTree, relativeDate, type LibraryScope } from "./document-utils";
import type { DocumentSummary, FolderSummary } from "../../types";
import { submitImportBatch, type ImportFile } from "./import-batch";

type DocumentForm = { name: string; description?: string; type: "PART" | "PRODUCT" };
type FolderForm = { name: string; description?: string };
type DocumentOperation = { type: "copy" | "move"; document: DocumentSummary };
const canEdit = (permission?: string) => permission === "OWNER" || permission === "EDITOR";

export function DocumentCenter() {
  const navigate = useNavigate();
  const client = useQueryClient();
  const { message, modal } = App.useApp();
  const [scope, setScope] = useState<LibraryScope>("active");
  const [query, setQuery] = useState("");
  const [type, setType] = useState("");
  const [sort, setSort] = useState<"updated" | "name" | "created">("updated");
  const [offset, setOffset] = useState(0);
  const [currentFolderID, setCurrentFolderID] = useState("");
  const [selectedDocuments, setSelectedDocuments] = useState<DocumentSummary[]>([]);
  const [selectionBusy, setSelectionBusy] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<DocumentSummary>();
  const [folderEditor, setFolderEditor] = useState<FolderSummary | "new">();
  const [operation, setOperation] = useState<DocumentOperation>();
  const [shareResource, setShareResource] = useState<ShareResource>();
  const [importOpen, setImportOpen] = useState(false);
  const [importFiles, setImportFiles] = useState<ImportFile[]>([]);
  const [importProgress, setImportProgress] = useState(0);
  const [exportDocument, setExportDocument] = useState<DocumentSummary>();
  const [exportFormat, setExportFormat] = useState<"STEP" | "BREP">("STEP");
  const [documentForm] = Form.useForm<DocumentForm>();
  const [folderForm] = Form.useForm<FolderForm>();
  const [operationForm] = Form.useForm<{ name?: string; folderID?: string }>();

  const specialScope = scope === "recent" || scope === "shared" || scope === "trash";
  const filters = { scope, query, type, sort, offset, currentFolderID };
  const documents = useQuery({
    queryKey: queryKeys.documents(filters), queryFn: () => api.listDocuments({
      scope: scope === "trash" ? "trash" : "active", query,
      type: type || (scope === "parts" ? "PART" : scope === "products" ? "PRODUCT" : ""),
      folderId: specialScope || query ? undefined : currentFolderID, recent: scope === "recent", shared: scope === "shared",
      allFolders: specialScope || Boolean(query), sort: scope === "recent" ? "recent" : sort, limit: 24, offset,
    })
  });
  const catalog = useQuery({ queryKey: queryKeys.documents({ defaultNames: true }), queryFn: () => api.listDocuments({ limit: 200, allFolders: true }) });
  const folders = useQuery({
    queryKey: queryKeys.folders(`${scope}:${currentFolderID}`),
    queryFn: () => api.listFolders(currentFolderID, scope === "shared"), enabled: scope === "active" || scope === "parts" || scope === "products" || scope === "shared"
  });
  const trashedFolders = useQuery({ queryKey: ["folders", "trash"], queryFn: () => api.listTrashedFolders(), enabled: scope === "trash" });
  const breadcrumbs = useQuery({ queryKey: ["folder-breadcrumbs", currentFolderID], queryFn: () => api.folderBreadcrumbs(currentFolderID), enabled: Boolean(currentFolderID) });
  const folderTree = useQuery({ queryKey: ["folder-options"], queryFn: () => flattenFolderTree((parentID) => api.listFolders(parentID)), staleTime: 10_000 });
  const currentPermission = breadcrumbs.data?.at(-1)?.permission;
  const writableLocation = !currentFolderID || canEdit(currentPermission);

  const invalidateDocuments = async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ["documents"] }),
      client.invalidateQueries({ queryKey: queryKeys.openDocuments }),
    ]);
  };
  const invalidateFolders = async () => {
    await Promise.all([client.invalidateQueries({ queryKey: ["folders"] }), client.invalidateQueries({ queryKey: ["folder-options"] })]);
  };
  const saveDocument = useMutation({
    mutationFn: async (values: DocumentForm) => editing
      ? api.updateDocument(editing.id, values.name, values.description ?? "")
      : api.createDocument(values.type, values.name, values.description ?? "", currentFolderID || undefined),
    onSuccess: async (view) => {
      const created = !editing; setCreateOpen(false); setEditing(undefined); documentForm.resetFields(); await invalidateDocuments();
      message.success("文档已保存"); if (created) navigate(`/documents/${view.document.id}`);
    }, onError: (error) => message.error(error.message)
  });
  const saveFolder = useMutation({
    mutationFn: (values: FolderForm) => folderEditor !== "new" && folderEditor
      ? api.updateFolder(folderEditor.id, values.name, values.description ?? "")
      : api.createFolder(values.name, values.description ?? "", currentFolderID || undefined),
    onSuccess: async () => { setFolderEditor(undefined); folderForm.resetFields(); await invalidateFolders(); message.success("文件夹已保存"); },
    onError: (error) => message.error(error.message)
  });
  const runOperation = useMutation({
    mutationFn: (values: { name?: string; folderID?: string }) => {
      if (!operation) throw new Error("没有待执行操作");
      return operation.type === "copy"
        ? api.copyDocument(operation.document.id, values.name || `${operation.document.name} Copy`, values.folderID || undefined)
        : api.moveDocument(operation.document.id, values.folderID || undefined);
    }, onSuccess: async () => { setOperation(undefined); operationForm.resetFields(); await invalidateDocuments(); message.success("操作已完成"); },
    onError: (error) => message.error(error.message)
  });
  const importDocument = useMutation({
    mutationFn: async () => {
      if (importFiles.length === 0) throw new Error("请选择需要导入的 STEP 或 BREP 文件");
      setImportProgress(0);
      return submitImportBatch(importFiles, (file) => api.importDocument(file, currentFolderID), 3,
        (finished) => setImportProgress(finished));
    },
    onSuccess: async ({ submitted, failures }) => {
      await client.invalidateQueries({ queryKey: queryKeys.jobs });
      if (isMockMode && submitted.length > 0) await invalidateDocuments();
      if (submitted.length > 0) message.info(isMockMode ? `已导入 ${submitted.length} 个文件` : `已提交 ${submitted.length} 个导入任务，可在任务中心查看进度`);
      if (failures.length > 0) {
        setImportFiles((previous) => previous.filter(({ uid }) => failures.some((item) => item.uid === uid)));
        message.error(`${failures.length} 个文件提交失败：${failures.map(({ name, reason }) => `${name} (${reason})`).join("；")}`);
      } else { setImportOpen(false); setImportFiles([]); }
    },
    onError: (error) => message.error(error.message),
  });
  const exportExchange = useMutation({
    mutationFn: async () => {
      if (!exportDocument) throw new Error("没有待导出的文档");
      return api.startExport(exportDocument.id, exportFormat);
    },
    onSuccess: async (job) => {
      setExportDocument(undefined);
      await client.invalidateQueries({ queryKey: queryKeys.jobs });
      if (isMockMode && job.state === "SUCCEEDED") void api.downloadJob(job.id);
      message.info(isMockMode ? `${exportFormat} 导出完成` : "导出任务已提交，完成后会通知你");
    },
    onError: (error) => message.error(error.message),
  });

  const openDocument = (document: DocumentSummary) => { if (!document.deletedAt) navigate(`/documents/${document.id}`); };
  const enterFolder = (folderID: string) => { if (scope === "shared") setScope("active"); setCurrentFolderID(folderID); setOffset(0); };
  const openDocumentEditor = (document?: DocumentSummary, documentType?: "PART" | "PRODUCT") => {
    setEditing(document); documentForm.resetFields(); documentForm.setFieldsValue(document
      ? { name: document.name, description: document.description, type: document.type }
      : { type: documentType ?? "PART", name: defaultDocumentName(documentType ?? "PART", catalog.data?.documents ?? []), description: "" });
    setCreateOpen(true);
  };
  const openFolderEditor = (folder?: FolderSummary) => { setFolderEditor(folder ?? "new"); folderForm.setFieldsValue(folder ?? { name: "", description: "" }); };
  const removeDocument = (document: DocumentSummary) => modal.confirm({
    title: `将“${document.name}”移入回收站？`,
    content: "历史、几何制品和 Product 引用将继续保留。", okText: "移入回收站", okButtonProps: { danger: true },
    onOk: async () => { await api.deleteDocument(document.id); setSelectedDocuments((current) => current.filter((item) => item.id !== document.id));
      await invalidateDocuments(); message.success("文档已移入回收站"); }
  });
  const restoreDocument = async (document: DocumentSummary) => { await api.restoreDocument(document.id); await invalidateDocuments(); message.success("文档已恢复"); };
  const removeFolder = (folder: FolderSummary) => modal.confirm({
    title: `将文件夹“${folder.name}”移入回收站？`, content: "文件夹及其中仍在使用的子文件夹、文档会一同移入回收站，可整体恢复。", okButtonProps: { danger: true },
    onOk: async () => {
      try { await api.deleteFolder(folder.id); setSelectedDocuments([]); await Promise.all([invalidateFolders(), invalidateDocuments()]); message.success("文件夹已移入回收站"); }
      catch (error) { message.error(error instanceof Error ? error.message : "文件夹删除失败"); }
    }
  });
  const restoreFolder = async (folder: FolderSummary) => {
    try { await api.restoreFolder(folder.id); await Promise.all([invalidateFolders(), invalidateDocuments()]); message.success("文件夹及其内容已恢复"); }
    catch (error) { message.error(error instanceof Error ? error.message : "文件夹恢复失败"); }
  };
  const addSelected = (items: DocumentSummary[]) => setSelectedDocuments((current) => {
    const byID = new Map(current.map((item) => [item.id, item]));
    for (const item of items) if (canEdit(item.permission) && !item.deletedAt) byID.set(item.id, item);
    return [...byID.values()];
  });
  const toggleSelected = (document: DocumentSummary) => setSelectedDocuments((current) => current.some((item) => item.id === document.id)
    ? current.filter((item) => item.id !== document.id) : canEdit(document.permission) && !document.deletedAt ? [...current, document] : current);
  const selectAllResults = async () => {
    setSelectionBusy(true);
    try {
      const all: DocumentSummary[] = [];
      for (let next = 0; ; next += 200) {
        const page = await api.listDocuments({ scope: "active", query, type: type || (scope === "parts" ? "PART" : scope === "products" ? "PRODUCT" : ""),
          folderId: specialScope || query ? undefined : currentFolderID, recent: scope === "recent", shared: scope === "shared",
          allFolders: specialScope || Boolean(query), sort: scope === "recent" ? "recent" : sort, limit: 200, offset: next });
        all.push(...page.documents);
        if (all.length >= page.total || page.documents.length === 0) break;
      }
      addSelected(all);
    } catch (error) { message.error(error instanceof Error ? error.message : "全选失败"); }
    finally { setSelectionBusy(false); }
  };
  const removeSelected = () => modal.confirm({
    title: `将选中的 ${selectedDocuments.length} 个文档移入回收站？`,
    content: "已成功移入回收站的文档会从选择中移除；失败项保留以便重试。", okText: "批量移入回收站", okButtonProps: { danger: true },
    onOk: async () => {
      const items = [...selectedDocuments]; let cursor = 0;
      const succeeded = new Set<string>(), failed: string[] = [];
      await Promise.all(Array.from({ length: Math.min(4, items.length) }, async () => {
        for (;;) {
          const index = cursor++;
          if (index >= items.length) return;
          try { await api.deleteDocument(items[index].id); succeeded.add(items[index].id); }
          catch (error) { failed.push(`${items[index].name}（${error instanceof Error ? error.message : String(error)}）`); }
        }
      }));
      setSelectedDocuments((current) => current.filter((item) => !succeeded.has(item.id)));
      await invalidateDocuments();
      failed.length ? message.warning(`已删除 ${succeeded.size} 个，失败 ${failed.length} 个：${failed.join("、")}`)
        : message.success(`已将 ${succeeded.size} 个文档移入回收站`);
    },
  });
  const listAllTrash = async () => {
    const result: DocumentSummary[] = [];
    for (let offset = 0; ; offset += 200) {
      const page = await api.listDocuments({ scope: "trash", limit: 200, offset, allFolders: true });
      result.push(...page.documents);
      if (result.length >= page.total || page.documents.length === 0) return result;
    }
  };
  const restoreAllTrash = () => modal.confirm({ title: "还原回收站中的全部文档？", okText: "全部还原",
    onOk: async () => {
      const items = (await listAllTrash()).filter((item) => canEdit(item.permission));
      const results = await Promise.allSettled(items.map((item) => api.restoreDocument(item.id)));
      await invalidateDocuments();
      const failed = results.filter((result) => result.status === "rejected").length;
      failed ? message.warning(`已还原 ${items.length - failed} 个，${failed} 个失败`) : message.success(`已还原 ${items.length} 个文档`);
    } });
  const emptyTrash = () => modal.confirm({ title: "永久删除回收站中的全部文档？",
    content: "此操作不可撤销；仍被 Product 引用的文档将保留并报告失败。", okText: "清空回收站", okButtonProps: { danger: true },
    onOk: async () => {
      const items = (await listAllTrash()).filter((item) => item.permission === "OWNER");
      const results = await Promise.allSettled(items.map((item) => api.purgeDocument(item.id)));
      await invalidateDocuments();
      const failed = results.filter((result) => result.status === "rejected").length;
      failed ? message.warning(`已永久删除 ${items.length - failed} 个，${failed} 个因权限或引用关系保留`)
        : message.success(`已永久删除 ${items.length} 个文档`);
    } });
  const chooseOperation = (kind: "copy" | "move", document: DocumentSummary) => {
    setOperation({ type: kind, document }); operationForm.setFieldsValue({ name: kind === "copy" ? `${document.name} Copy` : undefined, folderID: document.folderId });
  };

  const navItems = [
    { key: "active", icon: <FolderOutlined />, label: "全部文档" }, { key: "recent", icon: <ClockCircleOutlined />, label: "最近打开" },
    { key: "shared", icon: <TeamOutlined />, label: "与我共享" }, { key: "parts", icon: <BuildOutlined />, label: "零件" },
    { key: "products", icon: <ApartmentOutlined />, label: "产品" }, { key: "trash", icon: <DeleteOutlined />, label: "回收站" },
  ];
  const folderOptions = [{ value: "", label: "我的文档（根目录）" }, ...(folderTree.data ?? []).map((item) => ({ value: item.id, label: item.label }))];
  const selectablePage = (documents.data?.documents ?? []).filter((item) => canEdit(item.permission));

  return <Layout className="document-center-layout">
    <Layout.Sider width={224} className="library-sider">
      <Menu mode="inline" theme="dark" selectedKeys={[scope]} items={navItems} onSelect={({ key }) => {
        setScope(key as LibraryScope); setCurrentFolderID(""); if (key === "trash") setSelectedDocuments([]); setOffset(0);
      }} />
    </Layout.Sider>
    <Layout.Content className="library-main">
      <header className="page-heading"><div><Typography.Title level={2}>文档中心</Typography.Title>
        <Typography.Text type="secondary">管理零件、装配与共享设计</Typography.Text></div>
        {!specialScope && <Space.Compact><Button title="新建文件夹" aria-label="新建文件夹" icon={<FolderAddOutlined />} disabled={!writableLocation} onClick={() => openFolderEditor()}>新建文件夹</Button>
          <Button title="导入" aria-label="导入" icon={<UploadOutlined />} disabled={!writableLocation} onClick={() => setImportOpen(true)}>导入</Button>
          <Button type="primary" title="创建文档" aria-label="创建文档" icon={<PlusOutlined />} disabled={!writableLocation}
            onClick={() => openDocumentEditor()}>新建文档</Button></Space.Compact>}
        {scope === "trash" && <Space.Compact><Button icon={<UndoOutlined />} onClick={restoreAllTrash}>全部还原</Button>
          <Button danger icon={<DeleteOutlined />} onClick={emptyTrash}>清空文档</Button></Space.Compact>}</header>
      {!specialScope && <Breadcrumb className="folder-breadcrumb" items={[
        { title: <Button type="link" onClick={() => enterFolder("")}>我的文档</Button> },
        ...(breadcrumbs.data ?? []).map((folder) => ({ title: <Button type="link" onClick={() => enterFolder(folder.id)}>{folder.name}</Button> })),
      ]} />}
      <Card className="document-browser" bordered={false}>
        <div className="browser-controls">
          <Input allowClear prefix={<SearchOutlined />} aria-label="搜索文档" placeholder="搜索文档名称或说明" value={query} onChange={(event) => { setQuery(event.target.value); setOffset(0); }} />
          <Segmented value={type} onChange={(value) => { setType(String(value)); setOffset(0); }} options={[{ label: "全部", value: "" }, { label: "Part", value: "PART" }, { label: "Product", value: "PRODUCT" }]} />
          <Select value={sort} onChange={setSort} options={[{ value: "updated", label: "最近修改" }, { value: "name", label: "名称" }, { value: "created", label: "创建时间" }]} />
          <Button icon={<ReloadOutlined />} onClick={() => void documents.refetch()} />
        </div>
        {scope !== "trash" && <div className="browser-selection-controls">
          <Checkbox disabled={!selectablePage.length} checked={selectablePage.length > 0 && selectablePage.every((item) => selectedDocuments.some((selected) => selected.id === item.id))}
            onChange={(event) => event.target.checked ? addSelected(documents.data?.documents ?? [])
              : setSelectedDocuments((current) => current.filter((item) => !documents.data?.documents.some((visible) => visible.id === item.id)))}>
            全选当前页</Checkbox>
          <Button size="small" loading={selectionBusy} onClick={() => void selectAllResults()}>全选筛选结果</Button>
          <span>已选择 {selectedDocuments.length} 个</span>
          <Button size="small" disabled={!selectedDocuments.length} onClick={() => setSelectedDocuments([])}>取消选择</Button>
          <Button size="small" danger icon={<DeleteOutlined />} disabled={!selectedDocuments.length} onClick={removeSelected}>批量移入回收站</Button>
        </div>}
        {scope === "trash" && trashedFolders.data?.length ? <div className="folder-grid">{trashedFolders.data.map((folder) =>
          <Card key={folder.id} size="small" className="folder-card"><div className="folder-card-content">
            <FolderOpenOutlined /><span><strong>{folder.name}</strong><small>文件夹及其内容 · {relativeDate(folder.deletedAt ?? folder.updatedAt)}</small></span>
            <Button size="small" disabled={!canEdit(folder.permission)} onClick={() => void restoreFolder(folder)}>恢复</Button>
          </div></Card>)}</div> : null}
        {folders.data?.length ? <div className="folder-grid">{folders.data.map((folder) => <Card key={folder.id} size="small" hoverable
          className="folder-card" onDoubleClick={() => enterFolder(folder.id)}>
          <div className="folder-card-content"><FolderOpenOutlined /><span><strong>{folder.name}</strong><small>{folder.documentCount} 文档 · {folder.childCount} 子文件夹</small></span>
            <Dropdown trigger={["click"]} menu={{
              items: [
                ...(folder.permission === "OWNER" ? [{ key: "share", icon: <ShareAltOutlined />, label: "共享" }] : []),
                ...(canEdit(folder.permission) ? [{ key: "edit", icon: <EditOutlined />, label: "编辑" },
                { key: "delete", icon: <DeleteOutlined />, label: "移入回收站", danger: true }] : []),
              ], onClick: ({ key, domEvent }) => {
                domEvent.stopPropagation(); if (key === "share") setShareResource({ type: "folders", id: folder.id, name: folder.name });
                else if (key === "edit") openFolderEditor(folder); else removeFolder(folder);
              }
            }}><Button type="text" icon={<MoreOutlined />} /></Dropdown>
          </div></Card>)}</div> : null}
        {documents.isLoading ? <div className="center-spinner"><Spin /></div> : documents.data?.documents.length
          ? <div className="document-grid">{documents.data.documents.map((document) => {
            const menuItems = scope === "trash" ? [{ key: "restore", icon: <UndoOutlined />, label: "恢复", disabled: !canEdit(document.permission) }] : [
              { key: "open", label: "打开" },
              ...(canEdit(document.permission) ? [{ key: "edit", icon: <EditOutlined />, label: "编辑" }, { key: "move", icon: <SwapOutlined />, label: "移动" }] : []),
              { key: "copy", icon: <CopyOutlined />, label: "复制" },
              ...(document.permission === "OWNER" ? [{ key: "share", icon: <ShareAltOutlined />, label: "共享" }] : []),
              { key: "export", icon: <DownloadOutlined />, label: "导出" },
              ...(canEdit(document.permission) ? [{ key: "delete", danger: true, icon: <DeleteOutlined />, label: "移入回收站" }] : []),
            ];
            const selected = selectedDocuments.some((item) => item.id === document.id);
            return <Card key={document.id} hoverable className={`document-card${selected ? " selected" : ""}`}
              cover={<button className="thumbnail-button" onClick={() => toggleSelected(document)} onDoubleClick={() => openDocument(document)}
                onKeyDown={(event) => { if (event.key === "Enter") openDocument(document); }}><DocumentThumbnail document={document} /></button>}>
              <Card.Meta title={<span className="document-title">{scope !== "trash" && <Checkbox aria-label={`选择 ${document.name}`} checked={selected}
                disabled={!canEdit(document.permission)} onChange={() => toggleSelected(document)} />}<span>{document.name}</span><Tag color={document.type === "PART" ? "blue" : "cyan"}>{document.type}</Tag></span>}
                description={<><span className="document-description">{document.description || "暂无说明"}</span><small>{document.workspaceName ?? "Main"} · {document.permission} · {relativeDate(document.lastUpdated)}</small></>} />
              <Dropdown trigger={["click"]} menu={{
                items: menuItems, onClick: ({ key }) => {
                  if (key === "open") openDocument(document); else if (key === "edit") openDocumentEditor(document); else if (key === "delete") removeDocument(document);
                  else if (key === "restore") void restoreDocument(document); else if (key === "copy" || key === "move") chooseOperation(key, document);
                  else if (key === "export") { setExportFormat("STEP"); setExportDocument(document); }
                  else if (key === "share") setShareResource({ type: "documents", id: document.id, name: document.name });
                }
              }}><Button className="card-menu" type="text" icon={<MoreOutlined />} /></Dropdown>
            </Card>;
          })}</div> : scope === "trash" && trashedFolders.data?.length ? null : <Empty description="没有找到文档" />}
        <Pagination current={Math.floor(offset / 24) + 1} pageSize={24} total={documents.data?.total ?? 0} hideOnSinglePage onChange={(page) => setOffset((page - 1) * 24)} />
      </Card>
    </Layout.Content>

    <Modal title={editing ? "编辑文档" : "创建设计文档"} open={createOpen} onCancel={() => { setCreateOpen(false); setEditing(undefined); documentForm.resetFields(); }}
      onOk={() => documentForm.submit()} confirmLoading={saveDocument.isPending} destroyOnHidden>
      <Form form={documentForm} layout="vertical" onFinish={(values) => saveDocument.mutate(values)}>
        <Form.Item name="type" label="文档类型" rules={[{ required: true }]}><Segmented block disabled={Boolean(editing)}
          onChange={(value) => { if (!editing) documentForm.setFieldValue("name", defaultDocumentName(value as "PART" | "PRODUCT", catalog.data?.documents ?? [])); }}
          options={[{ label: "Part 零件", value: "PART", icon: <BuildOutlined /> }, { label: "Product 产品", value: "PRODUCT", icon: <ApartmentOutlined /> }]} /></Form.Item>
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
    <Modal title="导入文档" open={importOpen} okText={importDocument.isPending ? `已提交 ${importProgress}/${importFiles.length}` : `开始导入（${importFiles.length}）`} confirmLoading={importDocument.isPending}
      okButtonProps={{ disabled: importFiles.length === 0 }} onOk={() => importDocument.mutate()}
      onCancel={() => { if (!importDocument.isPending) { setImportOpen(false); setImportFiles([]); } }} destroyOnHidden>
      <Typography.Paragraph type="secondary">支持一次选择或拖入多个 STEP/STP、BREP/BRP 文件，单个文件最大 128 MiB。每个文件独立创建导入任务；装配文件可生成 Product 和多个 Part。</Typography.Paragraph>
      <Upload.Dragger accept=".step,.stp,.brep,.brp" multiple disabled={importDocument.isPending}
        fileList={importFiles.map(({ uid, file }) => ({ uid, name: file.name, status: "done" as const, size: file.size }))}
        beforeUpload={(file) => {
          if (!/\.(step|stp|brep|brp)$/i.test(file.name)) { message.error(`${file.name}：仅支持 STEP/STP、BREP/BRP`); return false; }
          if (file.size === 0 || file.size > 128 * 1024 * 1024) { message.error(`${file.name}：文件需大于 0 且不超过 128 MiB`); return false; }
          setImportFiles((previous) => previous.some((entry) => entry.uid === file.uid) ? previous
            : previous.length >= 32 ? (message.warning("一次最多选择 32 个文件"), previous) : [...previous, { uid: file.uid, file }]);
          return false;
        }} onRemove={(file) => { setImportFiles((previous) => previous.filter((entry) => entry.uid !== file.uid)); return true; }}>
        <p className="ant-upload-drag-icon"><InboxOutlined /></p><p className="ant-upload-text">拖入多个文件，或点击一次选择多个文件</p>
      </Upload.Dragger>
    </Modal>
    <Modal title={`导出${exportDocument ? `“${exportDocument.name}”` : "文档"}`} open={Boolean(exportDocument)} okText="导出并下载"
      confirmLoading={exportExchange.isPending} onOk={() => exportExchange.mutate()}
      onCancel={() => { if (!exportExchange.isPending) setExportDocument(undefined); }} destroyOnHidden>
      <Typography.Paragraph type="secondary">选择用于交换或归档的输出格式。Part 与 Product 均可导出。</Typography.Paragraph>
      <Segmented block value={exportFormat} onChange={(value) => setExportFormat(value as "STEP" | "BREP")}
        options={[{ label: "STEP (.step)", value: "STEP" }, { label: "OpenCascade BREP (.brep)", value: "BREP" }]} />
    </Modal>
    <ShareDialog resource={shareResource} onClose={() => setShareResource(undefined)} />
  </Layout>;
}
