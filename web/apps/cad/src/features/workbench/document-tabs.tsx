import { ApartmentOutlined, BuildOutlined, CloseOutlined, PlusOutlined } from "@ant-design/icons";
import { useEffect, useRef } from "react";
import "./document-tabs.css";

export type OpenDocumentItem = { id: string; name: string; type: "PART" | "PRODUCT" };

export function DocumentTabs({ documents, activeID, onCreate, onSwitch, onClose }: {
  documents: OpenDocumentItem[];
  activeID: string;
  onCreate: () => void;
  onSwitch: (id: string) => void;
  onClose: (id: string) => void;
}) {
  const activeTab = useRef<HTMLButtonElement>(null);
  useEffect(() => { activeTab.current?.scrollIntoView({ block: "nearest", inline: "nearest" }); }, [activeID, documents.length]);
  return <nav className="document-tabs" aria-label="已打开文档">
    <div className="document-tabs-list">
      {documents.map((document) => <div key={document.id} className={`document-tab ${document.id === activeID ? "active" : ""}`}>
        <button ref={document.id === activeID ? activeTab : undefined} className="document-tab-switch"
          aria-current={document.id === activeID ? "page" : undefined} title={`${document.name} · ${document.type === "PART" ? "零件" : "装配"}`}
          onClick={() => onSwitch(document.id)}>
          {document.type === "PART" ? <BuildOutlined /> : <ApartmentOutlined />}<span>{document.name}</span>
        </button>
        <button className="document-tab-close" aria-label={`关闭 ${document.name}`} onClick={() => onClose(document.id)}><CloseOutlined /></button>
      </div>)}
    </div>
    <button className="document-tab-create" aria-label="新建文档" title="新建文档" onClick={onCreate}><PlusOutlined /></button>
  </nav>;
}
