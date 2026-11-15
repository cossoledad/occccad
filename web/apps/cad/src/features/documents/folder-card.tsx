import { DeleteOutlined, EditOutlined, FolderFilled, MoreOutlined, ShareAltOutlined } from "@ant-design/icons";
import { Button, Dropdown } from "antd";
import type { FolderSummary } from "../../types";

export function FolderCard({ folder, onOpen, onAction }: {
  folder: FolderSummary; onOpen: (id: string) => void; onAction: (action: string, folder: FolderSummary) => void;
}) {
  const editable = folder.permission === "OWNER" || folder.permission === "EDITOR";
  const actions = { items: [
    { key: "open", label: "打开" },
    ...(folder.permission === "OWNER" ? [{ key: "share", icon: <ShareAltOutlined aria-hidden />, label: "共享" }] : []),
    ...(editable ? [{ key: "edit", icon: <EditOutlined aria-hidden />, label: "编辑" },
      { key: "delete", icon: <DeleteOutlined aria-hidden />, label: "移入回收站", danger: true }] : []),
  ], onClick: ({ key }: { key: string }) => key === "open" ? onOpen(folder.id) : onAction(key, folder) };
  return <Dropdown trigger={["contextMenu"]} menu={actions}><div className="folder-card">
    <button type="button" className="folder-open" aria-label={`打开文件夹 ${folder.name}`} onClick={() => onOpen(folder.id)}>
      <FolderFilled /><span><strong>{folder.name}</strong><small>{folder.documentCount} 文档{folder.childCount > 0 ? ` · ${folder.childCount} 文件夹` : ""}</small></span>
    </button>
    <Dropdown trigger={["click"]} menu={actions}>
      <Button type="text" aria-label={`文件夹操作 ${folder.name}`} icon={<MoreOutlined />} />
    </Dropdown>
  </div></Dropdown>;
}
