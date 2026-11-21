import { ExclamationCircleOutlined, CloseCircleOutlined, CopyOutlined, DeleteOutlined, DownloadOutlined, EditOutlined, MoreOutlined, ShareAltOutlined, SwapOutlined, UndoOutlined } from "@ant-design/icons";
import { Button, Card, Checkbox, Dropdown } from "antd";
import { memo } from "react";
import { DocumentThumbnail } from "../../components/document-thumbnail";
import type { DocumentSummary } from "../../types";
import { relativeDate } from "./document-utils";

/** Modal/search drafts and other cards' selection do not rebuild an unchanged card. */
export const DocumentCard = memo(function DocumentCard({ document, selected, trash, compact, onOpen, onToggle, onAction }: {
  document: DocumentSummary; selected: boolean; trash: boolean;
  compact?: boolean;
  onOpen: (document: DocumentSummary) => void; onToggle: (document: DocumentSummary) => void;
  onAction: (action: string, document: DocumentSummary) => void;
}) {
  const editable = document.permission === "OWNER" || document.permission === "EDITOR";
  const health = document.health;
  const items = trash ? [{ key: "restore", icon: <UndoOutlined aria-hidden />, label: "恢复", disabled: !editable }] : [
    { key: "open", label: "打开" },
    ...(editable ? [{ key: "edit", icon: <EditOutlined aria-hidden />, label: "编辑" }, { key: "move", icon: <SwapOutlined aria-hidden />, label: "移动" }] : []),
    { key: "copy", icon: <CopyOutlined aria-hidden />, label: "复制" },
    ...(document.permission === "OWNER" ? [{ key: "share", icon: <ShareAltOutlined aria-hidden />, label: "共享" }] : []),
    { key: "export", icon: <DownloadOutlined aria-hidden />, label: "导出" },
    ...(editable ? [{ key: "delete", danger: true, icon: <DeleteOutlined aria-hidden />, label: "移入回收站" }] : []),
  ];
  const actions = { items, onClick: ({ key }: { key: string }) => onAction(key, document) };
  const menu = <Dropdown trigger={["click"]} menu={actions}>
    <Button className="card-menu" type="text" aria-label={`文档操作 ${document.name}`} icon={<MoreOutlined />} />
  </Dropdown>;
  return <Dropdown trigger={["contextMenu"]} menu={actions}><Card hoverable className={`document-card${selected ? " selected" : ""}${compact ? " compact" : ""}`}
    cover={<button className="thumbnail-button" disabled={trash} aria-label={`打开 ${document.name}`} onClick={() => onOpen(document)}><DocumentThumbnail document={document} /></button>}>
    <Card.Meta title={<span className="document-title">{!trash && <Checkbox aria-label={`选择 ${document.name}`} checked={selected}
      disabled={!editable} onChange={() => onToggle(document)} />}<button className="document-name" disabled={trash} title={document.name} onClick={() => onOpen(document)}>{document.name}</button>{menu}</span>}
      description={<>{document.description && <span className="document-description">{document.description}</span>}<small><span className={`document-kind ${document.type.toLowerCase()}`}>{document.type === "PART" ? "零件" : "装配"}</span>{health && health.severity !== "NORMAL" && <span className={health.severity === "ERROR" ? "document-error" : "document-warning"} title={health.message}>
        {health.severity === "ERROR" ? <><CloseCircleOutlined/> 错误</> : <><ExclamationCircleOutlined/> 警告</>}</span>}<span>{relativeDate(document.lastUpdated)}</span></small></>} />
  </Card></Dropdown>;
});
