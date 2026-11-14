import { CheckOutlined, FolderOutlined, RightOutlined } from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";
import { Alert, Breadcrumb, Button, Empty, Spin } from "antd";
import { useState } from "react";
import { api } from "../../api/client";
import { engineeringProblem } from "../../cad/command/operation-failure";

/** Browse only the current branch. Folder IDs, never display paths, are values. */
export function FolderPicker({value = "", onChange, disabled = false}: {
  value?: string; onChange?: (id: string) => void; disabled?: boolean;
}) {
  const [folderID, setFolderID] = useState(value);
  const folders = useQuery({queryKey: ["folders", "picker", folderID],
    queryFn: () => api.listFolders(folderID), staleTime: 30_000});
  const path = useQuery({queryKey: ["folder-breadcrumbs", folderID],
    queryFn: () => api.folderBreadcrumbs(folderID), enabled: Boolean(folderID), staleTime: 30_000});
  const selectedPath = useQuery({queryKey: ["folder-breadcrumbs", value],
    queryFn: () => api.folderBreadcrumbs(value), enabled: Boolean(value), staleTime: 30_000});
  const enter = (id: string, permission?: string) => {
    setFolderID(id);
    if (!id || permission === "OWNER" || permission === "EDITOR") onChange?.(id);
  };
  const error = folders.error ?? path.error ?? selectedPath.error;
  return <div className="folder-picker" aria-busy={folders.isFetching}>
    <Breadcrumb items={[
      {title: <Button type="link" disabled={disabled} onClick={() => enter("")}>我的文档</Button>},
      ...(path.data ?? []).map(folder => ({key: folder.id, title: <Button type="link" disabled={disabled}
        onClick={() => enter(folder.id, folder.permission)}>{folder.name}</Button>})),
    ]}/>
    {error && <Alert type="error" title="无法读取文件夹" description={engineeringProblem(error)}
      action={<Button size="small" onClick={() => {void folders.refetch(); if(folderID)void path.refetch();if(value)void selectedPath.refetch();}}>重试</Button>}/>}
    <div className="folder-picker-list">
      {folders.isPending ? <Spin/> : folders.data?.length ? folders.data.map(folder => <button type="button" key={folder.id}
        disabled={disabled} className={value === folder.id ? "selected" : ""} onClick={() => enter(folder.id, folder.permission)}>
        <FolderOutlined/><span>{folder.name}</span>{value === folder.id && <CheckOutlined/>}<RightOutlined/>
      </button>) : !folders.error && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有子文件夹"/>}
    </div>
    <div className="folder-picker-target"><CheckOutlined/><span>目标位置：{!value ? "我的文档" : selectedPath.data?.map(folder => folder.name).join(" / ") || "…"}</span></div>
  </div>;
}
