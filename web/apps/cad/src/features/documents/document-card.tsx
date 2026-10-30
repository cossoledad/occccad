import { CopyOutlined, DeleteOutlined, DownloadOutlined, EditOutlined, MoreOutlined, ShareAltOutlined, SwapOutlined, UndoOutlined } from "@ant-design/icons";
import { Button, Card, Checkbox, Dropdown, Tag } from "antd";
import { memo } from "react";
import { DocumentThumbnail } from "../../components/document-thumbnail";
import type { DocumentSummary } from "../../types";
import { relativeDate } from "./document-utils";

/** Modal/search drafts and other cards' selection do not rebuild an unchanged card. */
export const DocumentCard = memo(function DocumentCard({ document, selected, trash, onOpen, onToggle, onAction }: {
  document: DocumentSummary; selected: boolean; trash: boolean;
  onOpen: (document: DocumentSummary) => void; onToggle: (document: DocumentSummary) => void;
  onAction: (action: string, document: DocumentSummary) => void;
}) {
  const editable = document.permission === "OWNER" || document.permission === "EDITOR";
  const items = trash ? [{ key: "restore", icon: <UndoOutlined />, label: "恢复", disabled: !editable }] : [
    { key: "open", label: "打开" },
    ...(editable ? [{ key: "edit", icon: <EditOutlined />, label: "编辑" }, { key: "move", icon: <SwapOutlined />, label: "移动" }] : []),
    { key: "copy", icon: <CopyOutlined />, label: "复制" },
    ...(document.permission === "OWNER" ? [{ key: "share", icon: <ShareAltOutlined />, label: "共享" }] : []),
    { key: "export", icon: <DownloadOutlined />, label: "导出" },
    ...(editable ? [{ key: "delete", danger: true, icon: <DeleteOutlined />, label: "移入回收站" }] : []),
  ];
  return <Card hoverable className={`document-card${selected ? " selected" : ""}`}
    cover={<button className="thumbnail-button" onClick={() => onToggle(document)} onDoubleClick={() => onOpen(document)}
      onKeyDown={event => { if (event.key === "Enter") onOpen(document); }}><DocumentThumbnail document={document} /></button>}>
    <Card.Meta title={<span className="document-title">{!trash && <Checkbox aria-label={`选择 ${document.name}`} checked={selected}
      disabled={!editable} onChange={() => onToggle(document)} />}<span>{document.name}</span><Tag color={document.type === "PART" ? "blue" : "cyan"}>{document.type}</Tag></span>}
      description={<><span className="document-description">{document.description || "暂无说明"}</span><small>{document.workspaceName ?? "Main"} · {document.permission} · {relativeDate(document.lastUpdated)}</small></>} />
    <Dropdown trigger={["click"]} menu={{ items, onClick: ({ key }) => onAction(key, document) }}>
      <Button className="card-menu" type="text" icon={<MoreOutlined />} />
    </Dropdown>
  </Card>;
});
