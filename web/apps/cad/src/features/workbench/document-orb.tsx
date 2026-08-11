import { ApartmentOutlined, BuildOutlined, CloseOutlined, FileAddOutlined, FolderOpenOutlined } from "@ant-design/icons";
import { Popover, Tooltip } from "antd";

export type OpenDocumentItem = { id: string; name: string; type: "PART" | "PRODUCT" };

export function DocumentOrb({ documents, activeID, onCreate, onSwitch, onClose }: {
  documents: OpenDocumentItem[];
  activeID: string;
  onCreate: () => void;
  onSwitch: (id: string) => void;
  onClose: (id: string) => void;
}) {
  const content = <div className="document-orb-menu">
    <button className="document-orb-create" onClick={onCreate}><FileAddOutlined /><span>新建文档</span></button>
    <div className="document-orb-caption">已打开文档</div>
    <div className="document-orb-list">
      {documents.map((document) => <div key={document.id}
        className={`document-orb-item ${document.id === activeID ? "active" : ""}`}>
        <button className="document-orb-switch" onClick={() => onSwitch(document.id)}>
          {document.type === "PART" ? <BuildOutlined /> : <ApartmentOutlined />}
          <span>{document.name}</span><small>{document.type}</small>
        </button>
        <Tooltip title="关闭文档"><button className="document-orb-close" onClick={() => onClose(document.id)}>
          <CloseOutlined />
        </button></Tooltip>
      </div>)}
      {documents.length === 0 && <div className="document-orb-empty">没有打开的文档</div>}
    </div>
  </div>;

  return <Popover trigger="click" placement="topLeft" arrow={false} content={content}>
    <button className="document-orb" aria-label="文档操作" title="新建、切换或关闭文档">
      <FolderOpenOutlined />
    </button>
  </Popover>;
}

