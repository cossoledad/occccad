import {engineeringProblem,useOperationFeedback} from "../../cad/command/operation-feedback";
import {
  ApartmentOutlined, BuildOutlined, ClockCircleOutlined, DeleteOutlined, EditOutlined,
  FolderAddOutlined, FolderFilled, FolderOutlined, MoreOutlined, PlusOutlined, ReloadOutlined,
  SearchOutlined, ShareAltOutlined, TeamOutlined, UndoOutlined, UploadOutlined, InboxOutlined, LoadingOutlined,
} from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert, App, Breadcrumb, Button, Card, Checkbox, Dropdown, Empty, Form, Input, Layout, Menu, Modal, Pagination,
  Segmented, Select, Space, Spin, Typography, Upload,
} from "antd";
import { useCallback, useEffect,useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, isMockMode } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import { DocumentCard } from "./document-card";
import { ShareDialog, type ShareResource } from "../../components/share-dialog";
import { flattenFolderTree, relativeDate, type LibraryScope } from "./document-utils";
import { DocumentEditorDialog, type DocumentForm } from "./document-editor-dialog";
import type { DocumentSummary, FolderSummary } from "../../types";
import { submitImportBatch, type ImportFile } from "./import-batch";
import "./document-center.css";

type FolderForm = { name: string; description?: string };
type DocumentOperation = { type: "copy" | "move"; document: DocumentSummary };
const canEdit = (permission?: string) => permission === "OWNER" || permission === "EDITOR";

export function DocumentCenter() {
  const navigate = useNavigate();
  const client = useQueryClient();
  const { message, modal } = App.useApp();
  const feedback=useOperationFeedback();
  const [scope, setScope] = useState<LibraryScope>("active");
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [type, setType] = useState("");
  const [sort, setSort] = useState<"updated" | "name" | "created">("updated");
  const [offset, setOffset] = useState(0);
  const [pageSize,setPageSize]=useState(24);
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
  const [excludeHidden, setExcludeHidden] = useState(true);
  const [stepSchema, setStepSchema] = useState<"AP242" | "AP214IS">("AP242");
  const [folderForm] = Form.useForm<FolderForm>();
  const [operationForm] = Form.useForm<{ name?: string; folderID?: string }>();

  const presentation=useQuery({queryKey:["ui","toolbars"],queryFn:api.toolbarCatalog,staleTime:5*60_000});
  const documentTypes=presentation.data?.documents??[];
  const exchangeCapabilities = useQuery({ queryKey: ["exchange-capabilities"], queryFn: () => api.exchangeCapabilities(), enabled: importOpen });
  const uploadLimit = exchangeCapabilities.data?.maxUploadBytes;
  const specialScope = scope === "recent" || scope === "shared" || scope === "trash";
  useEffect(() => {
    if (query.trim() === search) return;
    const timer = window.setTimeout(() => { setSearch(query.trim()); setOffset(0); }, 250);
    return () => window.clearTimeout(timer);
  }, [query, search]);
  const filters = { scope, search, type, sort, offset, pageSize,currentFolderID };
  const documents = useQuery({
    queryKey: queryKeys.documents(filters), queryFn: ({ signal }) => api.listDocuments({
      scope: scope === "trash" ? "trash" : "active", query: search,
      type: type || (scope === "parts" ? "PART" : scope === "products" ? "PRODUCT" : ""),
      folderId: specialScope || search ? undefined : currentFolderID, recent: scope === "recent", shared: scope === "shared",
      allFolders: specialScope || Boolean(search), sort: scope === "recent" ? "recent" : sort, limit: pageSize, offset,
    }, signal)
  });
  useEffect(()=>{const total=documents.data?.total;if(total!==undefined&&offset>=Math.max(1,total))setOffset(Math.max(0,Math.ceil(total/pageSize)-1)*pageSize);},[documents.data?.total,offset,pageSize]);
  const folders = useQuery({
    queryKey: queryKeys.folders(`${scope}:${currentFolderID}`),
    queryFn: () => api.listFolders(currentFolderID, scope === "shared"), enabled: scope === "active" || scope === "parts" || scope === "products" || scope === "shared"
  });
  const trashedFolders = useQuery({ queryKey: ["folders", "trash"], queryFn: () => api.listTrashedFolders(), enabled: scope === "trash" });
  const breadcrumbs = useQuery({ queryKey: ["folder-breadcrumbs", currentFolderID], queryFn: () => api.folderBreadcrumbs(currentFolderID), enabled: Boolean(currentFolderID) });
  const folderTree = useQuery({ queryKey: ["folder-options"], queryFn: () => flattenFolderTree((parentID) => api.listFolders(parentID)), staleTime: 10_000, enabled: Boolean(operation) });
  const currentPermission = breadcrumbs.data?.at(-1)?.permission;
  const writableLocation = !currentFolderID || canEdit(currentPermission);

  const invalidateDocuments = useCallback(async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ["documents"] }),
      client.invalidateQueries({ queryKey: queryKeys.openDocuments }),
    ]);
  }, [client]);
  const invalidateFolders = async () => {
    await Promise.all([client.invalidateQueries({ queryKey: ["folders"] }), client.invalidateQueries({ queryKey: ["folder-options"] })]);
  };
  const saveDocument = useMutation({
    mutationFn: async (values: DocumentForm) => editing
      ? api.updateDocument(editing.id, values.name, values.description ?? "", editing.versionId)
      : api.createDocument(values.type, values.name, values.description ?? "", currentFolderID || undefined),
    onSuccess: async (view) => {
      const created = !editing; setCreateOpen(false); setEditing(undefined);
      client.setQueryData(queryKeys.document(view.document.id), view);
      void invalidateDocuments();
      if (created) navigate(`/documents/${view.document.id}`);
    }, onError: (error) => feedback(error,"文件操作")
  });
  const saveFolder = useMutation({
    mutationFn: (values: FolderForm) => folderEditor !== "new" && folderEditor
      ? api.updateFolder(folderEditor.id, values.name, values.description ?? "")
      : api.createFolder(values.name, values.description ?? "", currentFolderID || undefined),
    onSuccess: async () => { setFolderEditor(undefined); folderForm.resetFields(); await invalidateFolders(); },
    onError: (error) => feedback(error,"文件操作")
  });
  const runOperation = useMutation({
    mutationFn: (values: { name?: string; folderID?: string }) => {
      if (!operation) throw new Error("没有待执行操作");
      return operation.type === "copy"
        ? api.copyDocument(operation.document.id, values.name || `${operation.document.name} Copy`, values.folderID || undefined)
        : api.moveDocument(operation.document.id, values.folderID || undefined);
    }, onSuccess: async () => { setOperation(undefined); operationForm.resetFields(); await invalidateDocuments(); },
    onError: (error) => feedback(error,"文件操作")
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
      if (submitted.length > 0) message.info(isMockMode ? `已导入 ${submitted.length} 个文件` : `已提交 ${submitted.length} 个导入任务`);
      if (failures.length > 0) {
        setImportFiles((previous) => previous.filter(({ uid }) => failures.some((item) => item.uid === uid)));
        message.error(`${failures.length} 个文件提交失败：${failures.map(({ name, reason }) => `${name}（${engineeringProblem(reason)}）`).join("；")}`);
      } else { setImportOpen(false); setImportFiles([]); }
    },
    onError: (error) => feedback(error,"文件操作"),
  });
  const exportExchange = useMutation({
    mutationFn: async () => {
      if (!exportDocument) throw new Error("没有待导出的文档");
      return api.startExport(exportDocument.id, exportFormat, { excludeHidden, stepSchema });
    },
    onSuccess: async (job) => {
      setExportDocument(undefined);
      await client.invalidateQueries({ queryKey: queryKeys.jobs });
      if (isMockMode && job.state === "SUCCEEDED") void api.downloadJob(job.id);
      message.info(isMockMode ? `${exportFormat} 导出完成` : "已提交导出任务");
    },
    onError: (error) => feedback(error,"文件操作"),
  });

  const openDocument = useCallback((document: DocumentSummary) => { if (!document.deletedAt) navigate(`/documents/${document.id}`); }, [navigate]);
  const enterFolder = (folderID: string) => { if (scope === "shared") setScope("active"); setCurrentFolderID(folderID); setOffset(0); };
  const openDocumentEditor = useCallback((document?: DocumentSummary) => { setEditing(document); setCreateOpen(true); }, []);
  const openFolderEditor = (folder?: FolderSummary) => { setFolderEditor(folder ?? "new"); folderForm.setFieldsValue(folder ?? { name: "", description: "" }); };
  const removeDocument = useCallback((document: DocumentSummary) => modal.confirm({
    title: `将“${document.name}”移入回收站？`,
    okText: "移入回收站", okButtonProps: { danger: true },
    onOk: async () => { await api.deleteDocument(document.id); setSelectedDocuments((current) => current.filter((item) => item.id !== document.id));
      await invalidateDocuments(); message.success("文档已移入回收站"); }
  }), [modal, invalidateDocuments, message]);
  const restoreDocument = useCallback(async (document: DocumentSummary) => { await api.restoreDocument(document.id); await invalidateDocuments(); message.success("文档已恢复"); }, [invalidateDocuments, message]);
  const removeFolder = (folder: FolderSummary) => modal.confirm({
    title: `将文件夹“${folder.name}”移入回收站？`, content: "包含其中的文件夹与文档。", okText: "移入回收站", okButtonProps: { danger: true },
    onOk: async () => {
      try { await api.deleteFolder(folder.id); setSelectedDocuments([]); await Promise.all([invalidateFolders(), invalidateDocuments()]); message.success("文件夹已移入回收站"); }
      catch (error) { feedback(error,"文件操作"); }
    }
  });
  const restoreFolder = async (folder: FolderSummary) => {
    try { await api.restoreFolder(folder.id); await Promise.all([invalidateFolders(), invalidateDocuments()]); message.success("文件夹及其内容已恢复"); }
    catch (error) { feedback(error,"文件操作"); }
  };
  const addSelected = (items: DocumentSummary[]) => setSelectedDocuments((current) => {
    const byID = new Map(current.map((item) => [item.id, item]));
    for (const item of items) if (canEdit(item.permission) && !item.deletedAt) byID.set(item.id, item);
    return [...byID.values()];
  });
  const toggleSelected = useCallback((document: DocumentSummary) => setSelectedDocuments((current) => current.some((item) => item.id === document.id)
    ? current.filter((item) => item.id !== document.id) : canEdit(document.permission) && !document.deletedAt ? [...current, document] : current), []);
  const selectAllResults = async () => {
    setSelectionBusy(true);
    try {
      const all: DocumentSummary[] = [];
      for (let next = 0; ; next += 200) {
        const page = await api.listDocuments({ scope: "active", query: search, type: type || (scope === "parts" ? "PART" : scope === "products" ? "PRODUCT" : ""),
          folderId: specialScope || search ? undefined : currentFolderID, recent: scope === "recent", shared: scope === "shared",
          allFolders: specialScope || Boolean(search), sort: scope === "recent" ? "recent" : sort, limit: 200, offset: next });
        all.push(...page.documents);
        if (all.length >= page.total || page.documents.length === 0) break;
      }
      addSelected(all);
    } catch (error) { feedback(error,"文件操作"); }
    finally { setSelectionBusy(false); }
  };
  const removeSelected = () => modal.confirm({
    title: `将选中的 ${selectedDocuments.length} 个文档移入回收站？`,
    okText: "移入回收站", okButtonProps: { danger: true },
    onOk: async () => {
      const items = [...selectedDocuments]; let cursor = 0;
      const succeeded = new Set<string>(), failed: string[] = [];
      await Promise.all(Array.from({ length: Math.min(4, items.length) }, async () => {
        for (;;) {
          const index = cursor++;
          if (index >= items.length) return;
          try { await api.deleteDocument(items[index].id); succeeded.add(items[index].id); }
          catch (error) { failed.push(`${items[index].name}（${engineeringProblem(error)}）`); }
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
  const restoreAllTrash = () => modal.confirm({ title: "还原回收站中的全部文档？", okText: "确定",
    onOk: async () => {
      const items = (await listAllTrash()).filter((item) => canEdit(item.permission));
      const results = await Promise.allSettled(items.map((item) => api.restoreDocument(item.id)));
      await invalidateDocuments();
      const failed = results.filter((result) => result.status === "rejected").length;
      failed ? message.warning(`已还原 ${items.length - failed} 个，${failed} 个失败`) : message.success(`已还原 ${items.length} 个文档`);
    } });
  const emptyTrash = () => modal.confirm({ title: "永久删除回收站中的全部文档？",
    content: "此操作不可撤销。", okText: "永久删除", okButtonProps: { danger: true },
    onOk: async () => {
      const items = (await listAllTrash()).filter((item) => item.permission === "OWNER");
      const results = await Promise.allSettled(items.map((item) => api.purgeDocument(item.id)));
      await invalidateDocuments();
      const failed = results.filter((result) => result.status === "rejected").length;
      failed ? message.warning(`已永久删除 ${items.length - failed} 个，${failed} 个因权限或引用关系保留`)
        : message.success(`已永久删除 ${items.length} 个文档`);
    } });
  const chooseOperation = useCallback((kind: "copy" | "move", document: DocumentSummary) => {
    setOperation({ type: kind, document }); operationForm.setFieldsValue({ name: kind === "copy" ? `${document.name} Copy` : undefined, folderID: document.folderId });
  }, [operationForm]);
  const documentAction = useCallback((key: string, document: DocumentSummary) => {
    if (key === "open") openDocument(document);
    else if (key === "edit") openDocumentEditor(document);
    else if (key === "delete") removeDocument(document);
    else if (key === "restore") void restoreDocument(document);
    else if (key === "copy" || key === "move") chooseOperation(key, document);
    else if (key === "export") { setExportFormat("STEP"); setExportDocument(document); }
    else if (key === "share") setShareResource({ type: "documents", id: document.id, name: document.name });
  }, [openDocument, openDocumentEditor, removeDocument, restoreDocument, chooseOperation]);

  const navItems = [
    { key: "active", icon: <FolderOutlined aria-hidden />, label: "全部文档" }, { key: "recent", icon: <ClockCircleOutlined aria-hidden />, label: "最近打开" },
    { key: "shared", icon: <TeamOutlined aria-hidden />, label: "与我共享" }, { key: "parts", icon: <BuildOutlined aria-hidden />, label: "零件" },
    { key: "products", icon: <ApartmentOutlined aria-hidden />, label: "产品" }, { key: "trash", icon: <DeleteOutlined aria-hidden />, label: "回收站" },
  ];
  const folderOptions = [{ value: "", label: "我的文档（根目录）" }, ...(folderTree.data ?? []).map((item) => ({ value: item.id, label: item.label }))];
  const selectablePage = (documents.data?.documents ?? []).filter((item) => canEdit(item.permission));
  const visibleFolders = !specialScope && !search ? folders.data ?? [] : [];

  return <Layout className="document-center-layout">
    <Layout.Sider width={224} className="library-sider">
      <Menu mode="inline" selectedKeys={[scope]} items={navItems} onSelect={({ key }) => {
        setScope(key as LibraryScope); setCurrentFolderID(""); setType(""); if (key === "trash") setSelectedDocuments([]); setOffset(0);
      }} />
    </Layout.Sider>
    <Layout.Content className="library-main">
      <header className="page-heading"><Typography.Title level={2}>{navItems.find(item => item.key === scope)?.label}</Typography.Title>
        {!specialScope && <Space wrap><Button title="新建文件夹" aria-label="新建文件夹" icon={<FolderAddOutlined />} disabled={!writableLocation} onClick={() => openFolderEditor()}>新建文件夹</Button>
          <Button title="导入" aria-label="导入" icon={<UploadOutlined />} disabled={!writableLocation} onClick={() => setImportOpen(true)}>导入</Button>
          <Button type="primary" title="创建文档" aria-label="创建文档" icon={<PlusOutlined />} disabled={!writableLocation}
            onClick={() => openDocumentEditor()}>新建文档</Button></Space>}
        {scope === "trash" && <Space wrap><Button icon={<UndoOutlined />} onClick={restoreAllTrash}>全部还原</Button>
          <Button danger icon={<DeleteOutlined />} onClick={emptyTrash}>清空文档</Button></Space>}</header>
      {!specialScope && currentFolderID && <Breadcrumb className="folder-breadcrumb" items={[
        { title: <Button type="link" onClick={() => enterFolder("")}>我的文档</Button> },
        ...(breadcrumbs.data ?? []).map((folder) => ({ title: <Button type="link" onClick={() => enterFolder(folder.id)}>{folder.name}</Button> })),
      ]} />}
      <Card className="document-browser" variant="borderless">
        <div className="browser-controls">
          <Input allowClear prefix={<SearchOutlined />} aria-label="搜索文档" placeholder="搜索文档" value={query} onChange={(event) => setQuery(event.target.value)} />
          <div className="browser-filters">{scope !== "parts" && scope !== "products" && <Segmented aria-label="文档类型" value={type} onChange={(value) => { setType(String(value)); setOffset(0); }} options={[{label:"全部",value:""},...documentTypes.map(type=>({label:type.name,value:type.id}))]} />}
          {scope !== "recent" && <Select aria-label="排序" value={sort} onChange={value => { setSort(value); setOffset(0); }} options={[{ value: "updated", label: "最近修改" }, { value: "name", label: "名称" }, { value: "created", label: "创建时间" }]} />}
          <Button type="text" aria-label="刷新文档" icon={<ReloadOutlined spin={documents.isFetching} />} onClick={() => void documents.refetch()} /></div>
        </div>
        {scope !== "trash" && <div className="browser-selection-controls">
          <Checkbox disabled={!selectablePage.length} checked={selectablePage.length > 0 && selectablePage.every((item) => selectedDocuments.some((selected) => selected.id === item.id))}
            indeterminate={selectablePage.some(item => selectedDocuments.some(selected => selected.id === item.id)) && !selectablePage.every(item => selectedDocuments.some(selected => selected.id === item.id))}
            onChange={(event) => event.target.checked ? addSelected(documents.data?.documents ?? [])
              : setSelectedDocuments((current) => current.filter((item) => !documents.data?.documents.some((visible) => visible.id === item.id)))}>
            全选</Checkbox>
          <span className="selection-count">{selectedDocuments.length ? `已选择 ${selectedDocuments.length} 个` : `${documents.data?.total ?? 0} 个文档`}</span>
          {selectedDocuments.length > 0 && <><Button size="small" type="text" disabled={query.trim() !== search || documents.isPending} loading={selectionBusy} onClick={() => void selectAllResults()}>全选筛选结果</Button>
          <Button size="small" type="text" onClick={() => setSelectedDocuments([])}>取消选择</Button>
          <Button size="small" danger type="text" aria-label="移入回收站" icon={<DeleteOutlined aria-hidden />} onClick={removeSelected}>移入回收站</Button></>}
        </div>}
        {scope === "trash" && trashedFolders.data?.length ? <div className="folder-grid">{trashedFolders.data.map((folder) =>
          <div key={folder.id} className="folder-card"><div className="folder-card-content">
            <FolderFilled /><span><strong>{folder.name}</strong><small>{relativeDate(folder.deletedAt ?? folder.updatedAt)}</small></span>
            <Button size="small" disabled={!canEdit(folder.permission)} onClick={() => void restoreFolder(folder)}>恢复</Button>
          </div></div>)}</div> : null}
        {visibleFolders.length ? <section className="folder-section" aria-label="文件夹"><h3>文件夹</h3><div className="folder-grid">{visibleFolders.map((folder) => <div key={folder.id} className="folder-card">
          <button type="button" className="folder-open" aria-label={`打开文件夹 ${folder.name}`} onClick={() => enterFolder(folder.id)}><FolderFilled /><span><strong>{folder.name}</strong><small>{folder.documentCount} 文档{folder.childCount > 0 ? ` · ${folder.childCount} 文件夹` : ""}</small></span></button>
            {canEdit(folder.permission) && <Dropdown trigger={["click"]} menu={{
              items: [
                ...(folder.permission === "OWNER" ? [{ key: "share", icon: <ShareAltOutlined />, label: "共享" }] : []),
                ...(canEdit(folder.permission) ? [{ key: "edit", icon: <EditOutlined />, label: "编辑" },
                { key: "delete", icon: <DeleteOutlined />, label: "移入回收站", danger: true }] : []),
              ], onClick: ({ key, domEvent }) => {
                domEvent.stopPropagation(); if (key === "share") setShareResource({ type: "folders", id: folder.id, name: folder.name });
                else if (key === "edit") openFolderEditor(folder); else removeFolder(folder);
              }
            }}><Button type="text" aria-label={`文件夹操作 ${folder.name}`} icon={<MoreOutlined />} /></Dropdown>}
          </div>)}</div></section> : null}
        {documents.error && <Alert type="error" title="无法加载文档" description={engineeringProblem(documents.error)}
          action={<Button size="small" onClick={() => void documents.refetch()}>重试</Button>} />}
        {documents.isLoading ? <div className="center-spinner"><Spin /></div> : documents.data?.documents.length
          ? <div className="document-grid">{documents.data.documents.map(document => <DocumentCard key={document.id} document={document}
            selected={selectedDocuments.some(item => item.id === document.id)} trash={scope === "trash"}
            onOpen={openDocument} onToggle={toggleSelected} onAction={documentAction} />)}</div> : documents.error || visibleFolders.length || (scope === "trash" && trashedFolders.data?.length) ? null : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={search ? "没有找到文档" : "暂无文档"} />}
        <Pagination current={Math.floor(offset / pageSize) + 1} pageSize={pageSize} showSizeChanger pageSizeOptions={[24,50,100]} total={documents.data?.total ?? 0} onChange={(page,size) => {setPageSize(size);setOffset(size!==pageSize?0:(page-1)*size)}} />
      </Card>
    </Layout.Content>

    {createOpen && <DocumentEditorDialog title={editing ? "编辑文档" : "创建设计文档"} editing={editing}
      documents={documents.data?.documents} documentTypes={documentTypes} busy={saveDocument.isPending}
      onClose={() => { setCreateOpen(false); setEditing(undefined); }} onSave={values => saveDocument.mutate(values)} />}
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
    <Modal className="exchange-dialog" title="导入文档" open={importOpen} okText="导入" confirmLoading={importDocument.isPending}
      okButtonProps={{ disabled: importFiles.length === 0 }} onOk={() => importDocument.mutate()}
      onCancel={() => { if (!importDocument.isPending) { setImportOpen(false); setImportFiles([]); } }} destroyOnHidden>
      {importDocument.isPending&&<Typography.Paragraph>已提交 {importProgress}/{importFiles.length}</Typography.Paragraph>}
      {exchangeCapabilities.error && <Alert type="error" title="无法读取导入设置" description={engineeringProblem(exchangeCapabilities.error)} action={<Button onClick={() => void exchangeCapabilities.refetch()}>重试</Button>} />}
      <Upload.Dragger accept=".step,.stp,.brep,.brp" multiple disabled={importDocument.isPending || !uploadLimit}
        fileList={importFiles.map(({ uid, file }) => ({ uid, name: file.name, status: "done" as const, size: file.size }))}
        beforeUpload={(file) => {
          if (!/\.(step|stp|brep|brp)$/i.test(file.name)) { message.error(`${file.name}：仅支持 STEP/STP、BREP/BRP`); return false; }
          if (!file.size) { message.error(`${file.name}：文件为空`); return false; }
          if (!uploadLimit || file.size > uploadLimit) { message.error(`${file.name}：超过单文件大小上限${uploadLimit ? ` ${(uploadLimit / 1024 ** 3).toFixed(1)} GiB` : ""}`); return false; }
          setImportFiles((previous) => previous.some((entry) => entry.uid === file.uid) ? previous
            : previous.length >= 32 ? (message.warning("一次最多选择 32 个文件"), previous) : [...previous, { uid: file.uid, file }]);
          return false;
        }} onRemove={(file) => { setImportFiles((previous) => previous.filter((entry) => entry.uid !== file.uid)); return true; }}>
        <p className="ant-upload-drag-icon">{exchangeCapabilities.isPending ? <LoadingOutlined spin /> : <InboxOutlined />}</p><p className="ant-upload-text">拖入文件或点击选择</p><p className="ant-upload-hint">STEP · BREP</p>
      </Upload.Dragger>
    </Modal>
    <Modal className="exchange-dialog" title={`导出${exportDocument ? `“${exportDocument.name}”` : "文档"}`} open={Boolean(exportDocument)} okText="导出"
      confirmLoading={exportExchange.isPending} onOk={() => exportExchange.mutate()}
      onCancel={() => { if (!exportExchange.isPending) setExportDocument(undefined); }} destroyOnHidden>
      <Form layout="vertical" requiredMark={false}><Form.Item label="格式">
      <Segmented block value={exportFormat} onChange={(value) => setExportFormat(value as "STEP" | "BREP")}
        options={[{ label: "STEP", value: "STEP" }, { label: "BREP", value: "BREP" }]} /></Form.Item>
      {exportFormat === "STEP" && <Form.Item label="STEP 协议"><Select aria-label="STEP 协议" value={stepSchema} onChange={setStepSchema} getPopupContainer={trigger=>trigger.parentElement!}
        options={[{value:"AP242",label:"AP242"},{value:"AP214IS",label:"AP214（兼容）"}]} /></Form.Item>}
      <Checkbox checked={excludeHidden} onChange={(event) => setExcludeHidden(event.target.checked)}>不导出隐藏对象</Checkbox>
      </Form>
    </Modal>
    <ShareDialog resource={shareResource} onClose={() => setShareResource(undefined)} />
  </Layout>;
}
