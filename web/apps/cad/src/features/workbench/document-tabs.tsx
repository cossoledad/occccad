import { ApartmentOutlined, BuildOutlined, CloseOutlined, PlusOutlined } from "@ant-design/icons";
import { Reorder, useDragControls, useReducedMotion } from "motion/react";
import { useEffect, useRef, useState } from "react";
import "./document-tabs.css";

export type OpenDocumentItem = { id: string; name: string; type: "PART" | "PRODUCT" };
const orderKey = "occccad.document-tab-order";
function readOrder(): string[] {
  try { const value: unknown = JSON.parse(sessionStorage.getItem(orderKey) ?? "[]");
    return Array.isArray(value) ? value.filter((id): id is string => typeof id === "string") : [];
  } catch { return []; }
}

export function DocumentTabs({ documents, activeID, onCreate, onSwitch, onClose }: {
  documents: OpenDocumentItem[]; activeID: string;
  onCreate: () => void; onSwitch: (id: string) => void; onClose: (id: string) => void;
}) {
  const [order, setOrder] = useState(readOrder);
  const list = useRef<HTMLDivElement>(null);
  const ids = [...new Set([...order.filter(id => documents.some(doc => doc.id === id)), ...documents.map(doc => doc.id)])];
  const reorder = (next: string[]) => {
    setOrder(next);
    try { sessionStorage.setItem(orderKey, JSON.stringify(next)); } catch { /* Storage is optional. */ }
  };
  useEffect(() => {
    list.current?.querySelector('[aria-current="page"]')?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [activeID, documents.length]);
  return <nav className="document-tabs" aria-label="已打开文档">
    <Reorder.Group as="div" axis="x" ref={list} layoutScroll className="document-tabs-list" values={ids} onReorder={reorder}>
      {ids.map(id => <DocumentTab key={id} document={documents.find(doc => doc.id === id)!} active={id === activeID}
        onSwitch={onSwitch} onClose={onClose} onMove={direction => {
          const next = [...ids], index = next.indexOf(id), target = index + direction;
          if (target < 0 || target >= next.length) return;
          [next[index], next[target]] = [next[target], next[index]]; reorder(next);
        }} onDragNearEdge={x => {
          const element = list.current; if (!element) return;
          const bounds = element.getBoundingClientRect();
          if (x < bounds.left + 40) element.scrollLeft -= 18;
          else if (x > bounds.right - 40) element.scrollLeft += 18;
        }} />)}
    </Reorder.Group>
    <button className="document-tab-create" aria-label="新建文档" title="新建文档" onClick={onCreate}><PlusOutlined /></button>
  </nav>;
}

function DocumentTab({ document, active, onSwitch, onClose, onMove, onDragNearEdge }: {
  document: OpenDocumentItem; active: boolean; onSwitch: (id: string) => void; onClose: (id: string) => void;
  onMove: (direction: number) => void; onDragNearEdge: (x: number) => void;
}) {
  const controls = useDragControls();
  const dragged = useRef(false);
  const reducedMotion = useReducedMotion();
  return <Reorder.Item as="div" value={document.id} dragListener={false} dragControls={controls}
    className={`document-tab ${active ? "active" : ""}`} dragMomentum={false}
    transition={reducedMotion ? { duration: 0 } : { type: "spring", stiffness: 500, damping: 38 }}
    whileDrag={{ backgroundColor: "#304761", boxShadow: "0 4px 14px #0005", scale: reducedMotion ? 1 : 1.025 }}
    onDragStart={() => { dragged.current = true; }} onDrag={(_, info) => onDragNearEdge(info.point.x)}>
    <button className="document-tab-switch" aria-current={active ? "page" : undefined}
      title={`${document.name} · ${document.type === "PART" ? "零件" : "装配"} · 拖动排序（Alt+方向键）`}
      onPointerDown={event => { if (event.button === 0) { dragged.current = false; controls.start(event); } }}
      onKeyDown={event => {
        if (event.key === "Enter" || event.key === " ") dragged.current = false;
        if (event.altKey && ["ArrowLeft", "ArrowRight"].includes(event.key)) {
          event.preventDefault(); onMove(event.key === "ArrowLeft" ? -1 : 1);
        }
      }} onClick={() => { if (!dragged.current) onSwitch(document.id); }}>
      {document.type === "PART" ? <BuildOutlined /> : <ApartmentOutlined />}<span>{document.name}</span>
    </button>
    <button className="document-tab-close" aria-label={`关闭 ${document.name}`} onClick={() => onClose(document.id)}><CloseOutlined /></button>
  </Reorder.Item>;
}
