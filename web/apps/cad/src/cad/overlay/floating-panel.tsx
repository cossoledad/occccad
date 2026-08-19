import { CloseOutlined } from "@ant-design/icons";
import { Button } from "antd";
import { useRef, useState, type CSSProperties, type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent, type PropsWithChildren, type ReactNode } from "react";

export type FloatingPanelProps = PropsWithChildren<{
  className?: string;
  title?: ReactNode;
  position?: "top-left" | "top-center" | "top-right" | "bottom-left" | "bottom-center" | "bottom-right";
}>;

export function FloatingPanel({ className = "", title, position = "top-left", children }: FloatingPanelProps) {
  return <section className={`cad-floating-panel ${position} ${className}`.trim()}>
    {title && <header className="cad-floating-panel-title">{title}</header>}
    {children}
  </section>;
}

type ToolbarOrientation = "horizontal" | "vertical";
type ToolbarLayout = { x?: number; y?: number; orientation: ToolbarOrientation };

export function FloatingToolbar({ children, id, label, orientation = "horizontal", position = "top-center", className = "" }:
  FloatingPanelProps & { id: string; label?: string; orientation?: ToolbarOrientation }) {
  const storageKey = `occccad.toolbar.${id}`;
  const [layout, setLayout] = useState<ToolbarLayout>(() => {
    try {
      const saved = window.localStorage.getItem(storageKey);
      if (!saved) return { orientation };
      const parsed = JSON.parse(saved) as Partial<ToolbarLayout>;
      return { ...parsed, orientation: parsed.orientation ?? orientation };
    } catch { return { orientation }; }
  });
  const toolbar = useRef<HTMLElement>(null);
  const layoutRef = useRef(layout);
  layoutRef.current = layout;
  const drag = useRef<{ pointerId: number; offsetX: number; offsetY: number } | undefined>(undefined);
  const placed = layout.x !== undefined && layout.y !== undefined;
  const style = {
    "--toolbar-direction": layout.orientation === "horizontal" ? "row" : "column",
    ...(placed ? { left: layout.x, top: layout.y, right: "auto", bottom: "auto", transform: "none" } : {}),
  } as CSSProperties;

  const persist = (next: ToolbarLayout) => {
    layoutRef.current = next;
    setLayout(next);
    window.localStorage.setItem(storageKey, JSON.stringify(next));
  };
  const pointerDown = (event: ReactPointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0 || !toolbar.current?.parentElement) return;
    const rect = toolbar.current.getBoundingClientRect();
    const parent = toolbar.current.parentElement.getBoundingClientRect();
    const x = rect.left - parent.left;
    const y = rect.top - parent.top;
    drag.current = { pointerId: event.pointerId, offsetX: event.clientX - rect.left, offsetY: event.clientY - rect.top };
    event.currentTarget.setPointerCapture(event.pointerId);
    const next = { ...layoutRef.current, x, y };
    layoutRef.current = next;
    setLayout(next);
    event.preventDefault();
  };
  const pointerMove = (event: ReactPointerEvent<HTMLButtonElement>) => {
    const active = drag.current;
    const element = toolbar.current;
    const parent = element?.parentElement;
    if (!active || active.pointerId !== event.pointerId || !element || !parent) return;
    const parentRect = parent.getBoundingClientRect();
    const x = Math.max(4, Math.min(event.clientX - parentRect.left - active.offsetX, parentRect.width - element.offsetWidth - 4));
    const y = Math.max(4, Math.min(event.clientY - parentRect.top - active.offsetY, parentRect.height - element.offsetHeight - 29));
    const next = { ...layoutRef.current, x, y };
    layoutRef.current = next;
    setLayout(next);
  };
  const pointerUp = (event: ReactPointerEvent<HTMLButtonElement>) => {
    if (drag.current?.pointerId !== event.pointerId) return;
    drag.current = undefined;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    persist(layoutRef.current);
  };
  const toggleOrientation = (event: ReactMouseEvent) => {
    event.preventDefault();
    persist({ ...layout, orientation: layout.orientation === "horizontal" ? "vertical" : "horizontal" });
  };

  return <section ref={toolbar} style={style} aria-label={label}
    className={`cad-floating-panel cad-floating-toolbar ${placed ? "custom-position" : position} ${layout.orientation} ${className}`.trim()}>
    <button className="cad-toolbar-handle" aria-label={`拖动${label ?? "工具栏"}；右键切换方向`}
      title={`${label ? `${label} · ` : ""}拖动工具栏 · 右键切换横竖方向`}
      onPointerDown={pointerDown} onPointerMove={pointerMove} onPointerUp={pointerUp} onPointerCancel={pointerUp}
      onLostPointerCapture={pointerUp}
      onContextMenu={toggleOrientation}><span /><span /><span /></button>
    <div className="cad-floating-toolbar-content">{children}</div>
  </section>;
}

export function ToolbarGroup({ children }: PropsWithChildren) {
  return <div className="cad-toolbar-group">{children}</div>;
}

export function ToolbarSeparator() { return <span className="cad-toolbar-separator" aria-hidden />; }

export function CommandDialog({ id, open, title, children, onClose, onConfirm, confirmText = "确定",
  cancelText = "取消", confirmLoading = false, width = 320 }: PropsWithChildren<{
  id: string; open: boolean; title: ReactNode; onClose: () => void; onConfirm: () => void | Promise<void>;
  confirmText?: string; cancelText?: string; confirmLoading?: boolean; width?: number;
}>) {
  const storageKey = `occccad.command-dialog.${id}`;
  const [position, setPosition] = useState<{ x: number; y: number }>(() => {
    try { return JSON.parse(window.localStorage.getItem(storageKey) ?? "null") ?? { x: 360, y: 84 }; }
    catch { return { x: 360, y: 84 }; }
  });
  const dialog = useRef<HTMLElement>(null);
  const positionRef = useRef(position); positionRef.current = position;
  const drag = useRef<{ pointerId: number; offsetX: number; offsetY: number } | undefined>(undefined);
  if (!open) return null;
  const pointerDown = (event: ReactPointerEvent<HTMLElement>) => {
    if (event.button !== 0) return;
    const rect = dialog.current?.getBoundingClientRect(); if (!rect) return;
    drag.current = { pointerId: event.pointerId, offsetX: event.clientX - rect.left, offsetY: event.clientY - rect.top };
    event.currentTarget.setPointerCapture(event.pointerId); event.preventDefault();
  };
  const pointerMove = (event: ReactPointerEvent<HTMLElement>) => {
    if (drag.current?.pointerId !== event.pointerId || !dialog.current?.parentElement) return;
    const parent = dialog.current.parentElement.getBoundingClientRect();
    const next = { x: Math.max(8, Math.min(event.clientX - parent.left - drag.current.offsetX, parent.width - width - 8)),
      y: Math.max(8, Math.min(event.clientY - parent.top - drag.current.offsetY, parent.height - dialog.current.offsetHeight - 32)) };
    positionRef.current = next; setPosition(next);
  };
  const pointerUp = (event: ReactPointerEvent<HTMLElement>) => {
    if (drag.current?.pointerId !== event.pointerId) return;
    drag.current = undefined;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    window.localStorage.setItem(storageKey, JSON.stringify(positionRef.current));
  };
  const confirm = async () => {
    try { await onConfirm(); }
    catch (error) {
      // Ant Form owns field validation feedback; unexpected command failures remain observable.
      if (!(error && typeof error === "object" && "errorFields" in error)) throw error;
    }
  };
  return <section ref={dialog} className="cad-command-dialog" role="dialog" aria-modal="false" aria-label={String(title)}
    style={{ left: position.x, top: position.y, width }}>
    <header className="cad-command-dialog-header" onPointerDown={pointerDown} onPointerMove={pointerMove}
      onPointerUp={pointerUp} onPointerCancel={pointerUp} onLostPointerCapture={pointerUp}>
      <strong>{title}</strong><button aria-label="关闭" title="关闭" onPointerDown={(event) => event.stopPropagation()}
        onClick={onClose}><CloseOutlined /></button>
    </header>
    <div className="cad-command-dialog-body">{children}</div>
    <footer className="cad-command-dialog-footer"><Button onClick={onClose}>{cancelText}</Button>
      <Button type="primary" loading={confirmLoading} onClick={() => void confirm()}>{confirmText}</Button></footer>
  </section>;
}
