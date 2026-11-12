import { CopyOutlined, DeleteOutlined, DownloadOutlined, EditOutlined, MoreOutlined, ShareAltOutlined, SwapOutlined, UndoOutlined } from "@ant-design/icons";
import { Button, Card, Checkbox, Dropdown } from "antd";
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
  const menu = <Dropdown trigger={["click"]} menu={{ items, onClick: ({ key }) => onAction(key, document) }}>
    <Button className="card-menu" type="text" aria-label={`文档操作 ${document.name}`} icon={<MoreOutlined />} />
  </Dropdown>;
  return <Card hoverable className={`document-card${selected ? " selected" : ""}`}
    cover={<button className="thumbnail-button" aria-label={`预览 ${document.name}`} onClick={() => onToggle(document)} onDoubleClick={() => onOpen(document)}
      onKeyDown={event => { if (event.key === "Enter") { event.preventDefault(); onOpen(document); } }}><DocumentThumbnail document={document} /></button>}>
    <Card.Meta title={<span className="document-title">{!trash && <Checkbox aria-label={`选择 ${document.name}`} checked={selected}
      disabled={!editable} onChange={() => onToggle(document)} />}<button className="document-name" disabled={trash} title={document.name} onClick={() => onOpen(document)}>{document.name}</button>{menu}</span>}
      description={<>{document.description && <span className="document-description">{document.description}</span>}<small><span className={`document-kind ${document.type.toLowerCase()}`}>{document.type === "PART" ? "零件" : "装配"}</span><span>{relativeDate(document.lastUpdated)}</span></small></>} />
  </Card>;
});
