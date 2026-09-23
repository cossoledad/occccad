import { ApartmentOutlined, CheckSquareOutlined, ClockCircleOutlined, FolderOutlined, SearchOutlined, TeamOutlined } from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";
import { Alert, Breadcrumb, Button, Empty, Input, Modal, Pagination, Segmented, Select, Spin } from "antd";
import { useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import { DocumentThumbnail } from "../../components/document-thumbnail";
import type { DocumentSummary } from "../../types";
import "./insert-document-dialog.css";

type Scope = "all" | "recent" | "shared" | "folder" | "selected";
const PAGE_SIZE = 12;

/** A paged resource browser. Selection persists across pages and filters. */
export function InsertDocumentDialog({ targetID, rootID, busy, onClose, onInsertDocuments }: {
  targetID: string; rootID: string; busy: boolean;
  onClose: () => void; onInsertDocuments: (ids: string[]) => Promise<unknown>;
}) {
  const [scope, setScope] = useState<Scope>("all");
  const [folderID, setFolderID] = useState("");
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [type, setType] = useState("");
  const [sort, setSort] = useState<"updated" | "name" | "created">("updated");
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<DocumentSummary[]>([]);
  const [selectedPageSnapshot, setSelectedPageSnapshot] = useState<DocumentSummary[]>([]);
  const [error, setError] = useState<string>();
  const [submitting, setSubmitting] = useState(false);
  const submittingRef = useRef(false);
  useEffect(() => {
    const timer = window.setTimeout(() => { setSearch(query.trim()); setPage(1); }, 250);
    return () => window.clearTimeout(timer);
  }, [query]);
  const options = { scope: "active" as const, query: search, type, folderId: scope === "folder" ? folderID : undefined,
    allFolders: scope !== "folder", recent: scope === "recent", shared: scope === "shared",
    sort: scope === "recent" ? "recent" as const : sort, limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE };
  const documents = useQuery({ queryKey: ["documents", "insert", options], queryFn: () => api.listDocuments(options), enabled: scope !== "selected" });
  const folders = useQuery({ queryKey: ["folders", "insert", folderID], queryFn: () => api.listFolders(folderID), enabled: scope === "folder" });
  const breadcrumbs = useQuery({ queryKey: ["folder-breadcrumbs", folderID], queryFn: () => api.folderBreadcrumbs(folderID), enabled: scope === "folder" && Boolean(folderID) });
  const lastSelected = selected.at(-1);
  const selectedFolder = useQuery({ queryKey: ["folder-breadcrumbs", lastSelected?.folderId],
    queryFn: () => api.folderBreadcrumbs(lastSelected!.folderId!), enabled: Boolean(lastSelected?.folderId) });
  const changeLocation = (nextScope: Scope, id = "") => {
    if (scope === "selected" && nextScope === "selected") return;
    if (nextScope === "selected") setSelectedPageSnapshot([...selected]);
    else setSelectedPageSnapshot([]);
    setScope(nextScope); setFolderID(id); setQuery(""); setSearch(""); setPage(1); setError(undefined);
  };
  const visibleDocuments = scope === "selected" ? selectedPageSnapshot.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)
    : documents.data?.documents ?? [];
  const pending = submitting || busy;
  const confirm = async () => {
    if (pending || submittingRef.current || !selected.length) return;
    submittingRef.current = true; setSubmitting(true); setError(undefined);
    try {
      await onInsertDocuments(selected.map((document) => document.id));
      onClose();
    }
    catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)); }
    finally { submittingRef.current = false; setSubmitting(false); }
  };
  return <Modal open title="插入组件" width={1060} className="insert-document-dialog" onCancel={onClose}
    closable={!pending} mask={{ closable: false }} keyboard={!pending} footer={<div className="insert-dialog-footer">
      <span>{selected.length ? `已选择 ${selected.length} 个文档` : "可多选零件或装配并一次插入"}</span>
      <Button onClick={onClose} disabled={pending}>取消</Button>
      <Button type="primary" disabled={!selected.length} loading={pending}
        onClick={() => void confirm()}>插入组件</Button>
    </div>}>
    <div className="insert-browser" aria-busy={pending}>
      <nav className="insert-locations" aria-label="组件来源">
        {([{ id: "all", label: "全部文档", icon: <ApartmentOutlined /> }, { id: "recent", label: "最近打开", icon: <ClockCircleOutlined /> },
          { id: "shared", label: "与我共享", icon: <TeamOutlined /> }, { id: "folder", label: "文件夹", icon: <FolderOutlined /> },
          { id: "selected", label: `当前选择 (${selected.length})`, icon: <CheckSquareOutlined /> }] as const)
          .map((location) => <button key={location.id} aria-label={location.label} disabled={pending} aria-current={scope === location.id ? "page" : undefined}
            onClick={() => changeLocation(location.id)}>{location.icon}{location.label}</button>)}
        <p>{scope === "selected" ? "取消选择后条目暂留本页，可立即重新选择；离开再进入时刷新。" : "通过文件夹逐层浏览，或在全部文档中检索名称与说明。"}</p>
      </nav>
      <section className="insert-results" aria-label="可插入文档">
        {scope !== "selected" && <>
        <Input allowClear prefix={<SearchOutlined />} aria-label="检索插入文档" disabled={pending}
          placeholder={scope === "folder" ? "搜索当前文件夹中的文档…" : "搜索所有可访问文档的名称或说明…"}
          value={query} onChange={(event) => setQuery(event.target.value)} />
        <div className="insert-filters"><Segmented aria-label="组件类型" disabled={pending} value={type}
          options={[{ label: "全部", value: "" }, { label: "零件", value: "PART" }, { label: "装配", value: "PRODUCT" }]}
          onChange={(value) => { setType(value); setPage(1); }} />
          <Select aria-label="组件排序" value={sort} disabled={pending || scope === "recent"} onChange={(value) => { setSort(value); setPage(1); }}
            options={[{ value: "updated", label: "最近修改" }, { value: "name", label: "名称" }, { value: "created", label: "创建时间" }]} /></div>
        </>}
        {scope === "folder" && <Breadcrumb items={[
          { title: <button onClick={() => changeLocation("folder")} disabled={pending}>我的文档</button> },
          ...(breadcrumbs.data ?? []).map((folder) => ({ title: <button disabled={pending} onClick={() => changeLocation("folder", folder.id)}>{folder.name}</button> })),
        ]} />}
        {((scope !== "selected" && documents.error) || (scope === "folder" && (folders.error || breadcrumbs.error))) && <Alert type="error" showIcon title="无法加载文档"
          description={(documents.error ?? folders.error ?? breadcrumbs.error)?.message}
          action={<Button size="small" onClick={() => { void documents.refetch(); if (scope === "folder") { void folders.refetch(); if (folderID) void breadcrumbs.refetch(); } }}>重试</Button>} />}
        {error && <Alert type="error" showIcon title="插入失败" description={error} />}
        <div className="insert-result-scroll">
          {scope === "folder" && <div className="insert-folder-list">{folders.data?.map((folder) => <button key={folder.id} aria-label={folder.name}
            disabled={pending} onClick={() => changeLocation("folder", folder.id)}><FolderOutlined /><span>{folder.name}</span></button>)}</div>}
          {scope !== "selected" && (documents.isPending || query.trim() !== search) ? <div className="insert-loading"><Spin /></div>
            : visibleDocuments.length ? <div className="insert-document-grid">{visibleDocuments.map((document) => {
              const excluded = document.id === targetID || document.id === rootID;
              const isSelected = selected.some((item) => item.id === document.id);
              return <button key={document.id} className={`insert-document-card ${isSelected ? "selected" : ""}`}
                aria-label={`选择 ${document.name}`} aria-pressed={isSelected} disabled={excluded || pending || (!isSelected && selected.length >= 128)}
                title={excluded ? "不能将当前装配或根装配插入自身" : document.name} onClick={() => {
                  setSelected((current) => isSelected ? current.filter((item) => item.id !== document.id) : [...current, document]);
                  setError(undefined);
                }}>
                <DocumentThumbnail document={document} /><strong>{document.name}</strong>
                <small>{excluded ? "当前装配" : document.type === "PART" ? "零件" : "装配"}</small>
              </button>;
            })}</div> : !documents.error && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={scope === "selected" ? "尚未选择文档" : search ? "没有匹配的文档，试试其他关键词" : "此范围没有文档"} />}
        </div>
        <Pagination size="small" current={page} pageSize={PAGE_SIZE} total={scope === "selected" ? selectedPageSnapshot.length : documents.data?.total ?? 0} showSizeChanger={false}
          disabled={pending || (scope !== "selected" && query.trim() !== search)} showTotal={(total) => `${total} 个文档`} onChange={setPage} />
      </section>
      <aside className="insert-preview" aria-label="组件预览">
        {lastSelected ? <><DocumentThumbnail document={lastSelected} /><h3>{lastSelected.name}</h3>
          <span className="insert-type">{lastSelected.type === "PART" ? "零件 · Part" : "装配 · Product"}</span>
          <p>{lastSelected.description || "暂无说明"}</p>
          <dl><dt>所在位置</dt><dd>{lastSelected.folderId ? selectedFolder.isError ? "位置读取失败" : selectedFolder.isPending ? "加载中…" : selectedFolder.data?.map((folder) => folder.name).join(" / ") || "文件夹" : "我的文档"}</dd>
            <dt>最近修改</dt><dd>{new Date(lastSelected.lastUpdated).toLocaleString()}</dd>
            <dt>引用方式</dt><dd>跟随源文档；有更新时需确认接受</dd></dl>
          <p className="insert-preview-note">插入后可在视口中移动和约束组件。模型与缩略图可能存在细节差异。</p>
        </> : <div className="insert-preview-empty"><ApartmentOutlined /><strong>组件预览</strong><p>选择文档以查看缩略图、位置和详细信息</p></div>}
      </aside>
    </div>
  </Modal>;
}
