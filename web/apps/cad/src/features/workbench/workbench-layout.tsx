import { MenuFoldOutlined, MenuUnfoldOutlined } from "@ant-design/icons";
import { Button } from "antd";
import { useEffect, useId, useRef, useState, useSyncExternalStore, type CSSProperties, type ReactNode } from "react";
import { MIN_STRUCTURE_TREE_WIDTH, MAX_STRUCTURE_TREE_WIDTH, useUIPreferences } from "../../state/ui-preferences";

import { lastClientPerformanceSample, subscribeClientPerformance } from "../../utils/performance";

function LastRequestTime() {
  const sample = useSyncExternalStore(subscribeClientPerformance, lastClientPerformanceSample, () => undefined);
  return <div className="workbench-request-time" title="最后完成的一次请求，包含网络等待和响应处理" aria-label="最后一次后端请求耗时">
    后端请求：{sample ? `${Math.round(sample.durationMs)} ms` : "—"}
  </div>;
}

type WorkbenchLayoutProps = {
  commands: ReactNode; tree: ReactNode; inspector: ReactNode; children: ReactNode; status: ReactNode;
  inspectorOpen: boolean; onInspectorChange: (open: boolean) => void;lightScene?:boolean;
};

/** Presentation and panel lifecycle only. Model/command ownership stays in Workbench. */
export function WorkbenchLayout({ commands, tree, inspector, children, status,
  inspectorOpen, onInspectorChange,lightScene=false }: WorkbenchLayoutProps) {
  const width = useUIPreferences((state) => state.structureTreeWidth);
  const setWidth = useUIPreferences((state) => state.setStructureTreeWidth);
  const [treeOpen, setTreeOpen] = useState(true);
  const treeId = useId();
  const root = useRef<HTMLDivElement>(null);
  const [availableWidth, setAvailableWidth] = useState(1200);
  const [resizing, setResizing] = useState(false);
  const suppressClick = useRef(false);
  const resize = useRef<{ pointerId: number; x: number; y: number; width: number; savedWidth: number;
    dragged: boolean; target: HTMLElement } | undefined>(undefined);
  const finishResize = (cancel: boolean) => {
    const gesture = resize.current;
    if (!gesture) return;
    resize.current = undefined;
    suppressClick.current = gesture.dragged || cancel;
    setResizing(false);
    if (cancel && gesture.dragged) setWidth(gesture.savedWidth);
    if (gesture.target.hasPointerCapture(gesture.pointerId)) gesture.target.releasePointerCapture(gesture.pointerId);
  };
  useEffect(() => {
    const cancel = () => finishResize(true);
    const key = (event: KeyboardEvent) => { if (event.key === "Escape") cancel(); };
    const visibility = () => { if (document.hidden) cancel(); };
    window.addEventListener("blur", cancel);
    window.addEventListener("keydown", key);
    document.addEventListener("visibilitychange", visibility);
    return () => {
      window.removeEventListener("blur", cancel);
      window.removeEventListener("keydown", key);
      document.removeEventListener("visibilitychange", visibility);
      cancel();
    };
  }, [setWidth]);
  const previousOpen = useRef(treeOpen);
  useEffect(() => {
    if (previousOpen.current === treeOpen) return;
    previousOpen.current = treeOpen;
    // Wait for the formerly hidden panel to participate in layout before focusing it.
    const frame = requestAnimationFrame(() => root.current?.querySelector<HTMLButtonElement>(
      treeOpen ? ".workbench-tree-collapse" : ".workbench-tree-restore")?.focus({preventScroll:true}));
    return () => cancelAnimationFrame(frame);
  }, [treeOpen]);
  useEffect(() => {
    if (!root.current) return;
    const observer = new ResizeObserver(([entry]) => setAvailableWidth(entry.contentRect.width));
    observer.observe(root.current);
    return () => observer.disconnect();
  }, []);
  const responsiveLimit = availableWidth <= 800 ? 220 : availableWidth <= 1100 ? 250 : MAX_STRUCTURE_TREE_WIDTH;
  const limit = Math.max(MIN_STRUCTURE_TREE_WIDTH, Math.min(responsiveLimit, availableWidth * 0.4));
  const panelWidth = Math.min(width, limit);
  return <div className="workbench-layout" ref={root} data-command-overlay-host data-tree-resizing={resizing || undefined} style={{position:"relative"}}>
    {commands}
    <div className={`workbench-content ${treeOpen ? "" : "tree-collapsed"} ${inspectorOpen ? "with-inspector" : ""} ${lightScene?"light-scene":"dark-scene"}`}
      style={{ "--structure-width": `${panelWidth}px` } as CSSProperties}>
      <aside id={treeId} className="workbench-structure" aria-label="模型结构" data-tree-width={Math.round(panelWidth)}>
        <Button className="workbench-tree-collapse" type="text" size="small" icon={<MenuFoldOutlined />}
          aria-label="收起模型结构" aria-expanded={treeOpen} aria-controls={treeId}
          title="点击收起；左右拖动调整宽度，也可用方向键、Home 或 End 调整"
          aria-description={`当前宽度 ${Math.round(panelWidth)} 像素。点击收起；拖动或使用左右方向键调整宽度。`}
          onClick={(event) => {
            if (event.detail !== 0 && suppressClick.current) { suppressClick.current = false; return; }
            setTreeOpen(false);
          }}
          onKeyDown={(event) => {
            const next = event.key === "Home" ? MIN_STRUCTURE_TREE_WIDTH : event.key === "End" ? limit
              : event.key === "ArrowLeft" ? panelWidth - 16 : event.key === "ArrowRight" ? panelWidth + 16 : undefined;
            if (next === undefined || resize.current) return;
            event.preventDefault(); event.stopPropagation(); setWidth(Math.min(limit, next));
          }}
          onPointerDown={(event) => {
            if (event.button !== 0 || !event.isPrimary || resize.current) return;
            suppressClick.current = false;
            resize.current = { pointerId: event.pointerId, x: event.clientX, y: event.clientY,
              width: panelWidth, savedWidth: width, dragged: false, target: event.currentTarget };
            event.currentTarget.setPointerCapture(event.pointerId);
          }}
          onPointerMove={(event) => {
            const gesture = resize.current;
            if (!gesture || gesture.pointerId !== event.pointerId) return;
            if (!gesture.dragged && Math.hypot(event.clientX - gesture.x, event.clientY - gesture.y) < 4) return;
            gesture.dragged = true; setResizing(true);
            setWidth(Math.min(limit, gesture.width + event.clientX - gesture.x));
          }}
          onPointerUp={(event) => { if (resize.current?.pointerId === event.pointerId) finishResize(false); }}
          onPointerCancel={(event) => { if (resize.current?.pointerId === event.pointerId) finishResize(true); }}
          onLostPointerCapture={(event) => { if (resize.current?.pointerId === event.pointerId) finishResize(true); }} />
        <div className="workbench-tree-content">{tree}</div>
        <LastRequestTime />
      </aside>
      <main className="workbench-stage"><section className="viewport-frame" aria-label="三维视口" onContextMenu={(event) => event.preventDefault()}>{children}
        {!treeOpen && <Button className="workbench-tree-restore" icon={<MenuUnfoldOutlined />}
          aria-label="展开模型结构" aria-expanded={false} aria-controls={treeId} onClick={() => setTreeOpen(true)} />}
        <Button className="workbench-inspector-toggle" icon={inspectorOpen ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
          aria-label={inspectorOpen ? "收起属性面板" : "展开属性面板"} aria-expanded={inspectorOpen}
          onClick={() => onInspectorChange(!inspectorOpen)} />
      </section></main>
      {inspectorOpen && <aside className="workbench-inspector" aria-label="属性与历史">{inspector}</aside>}
    </div>
    {status}
  </div>;
}
