import { MenuFoldOutlined, MenuUnfoldOutlined } from "@ant-design/icons";
import { Button } from "antd";
import { useEffect, useRef, useState, type CSSProperties, type PointerEvent, type ReactNode } from "react";
import { MIN_STRUCTURE_TREE_WIDTH, MAX_STRUCTURE_TREE_WIDTH, useUIPreferences } from "../../state/ui-preferences";

type WorkbenchLayoutProps = {
  commands: ReactNode; tree: ReactNode; inspector: ReactNode; children: ReactNode; status: ReactNode;
  documentName: string; inspectorOpen: boolean; onInspectorChange: (open: boolean) => void;
};

/** Presentation and panel lifecycle only. Model/command ownership stays in Workbench. */
export function WorkbenchLayout({ commands, tree, inspector, children, status, documentName,
  inspectorOpen, onInspectorChange }: WorkbenchLayoutProps) {
  const width = useUIPreferences((state) => state.structureTreeWidth);
  const setWidth = useUIPreferences((state) => state.setStructureTreeWidth);
  const [treeOpen, setTreeOpen] = useState(true);
  const root = useRef<HTMLDivElement>(null);
  const [availableWidth, setAvailableWidth] = useState(1200);
  const resize = useRef<{ pointerId: number; x: number; width: number } | undefined>(undefined);
  useEffect(() => {
    if (!root.current) return;
    const observer = new ResizeObserver(([entry]) => setAvailableWidth(entry.contentRect.width));
    observer.observe(root.current);
    return () => observer.disconnect();
  }, []);
  const responsiveLimit = availableWidth <= 800 ? 220 : availableWidth <= 1100 ? 250 : MAX_STRUCTURE_TREE_WIDTH;
  const limit = Math.max(MIN_STRUCTURE_TREE_WIDTH, Math.min(responsiveLimit, availableWidth * 0.4));
  const panelWidth = Math.min(width, limit);
  const finishResize = (event: PointerEvent<HTMLDivElement>) => {
    if (resize.current?.pointerId !== event.pointerId) return;
    resize.current = undefined;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
  };
  return <div className="workbench-layout" ref={root}>
    {commands}
    <div className={`workbench-content ${treeOpen ? "" : "tree-collapsed"} ${inspectorOpen ? "with-inspector" : ""}`}
      style={{ "--structure-width": `${panelWidth}px` } as CSSProperties}>
      <aside className="workbench-structure" aria-label="模型结构">
        <div className="workbench-panel-heading"><strong>模型结构</strong><Button type="text" size="small"
          icon={<MenuFoldOutlined />} aria-label="收起模型结构" onClick={() => setTreeOpen(false)} /></div>
        <div className="workbench-document-name" title={documentName}>{documentName}</div>
        <div className="workbench-tree-content">{tree}</div>
        <div className="workbench-panel-resizer" role="separator" aria-label="调整结构树宽度" aria-orientation="vertical"
          aria-valuemin={MIN_STRUCTURE_TREE_WIDTH} aria-valuemax={Math.floor(limit)} aria-valuenow={Math.round(panelWidth)} tabIndex={0}
          onKeyDown={(event) => {
            const next = event.key === "Home" ? MIN_STRUCTURE_TREE_WIDTH : event.key === "End" ? limit
              : event.key === "ArrowLeft" ? panelWidth - 16 : event.key === "ArrowRight" ? panelWidth + 16 : undefined;
            if (next === undefined) return;
            event.preventDefault(); setWidth(Math.min(limit, next));
          }}
          onPointerDown={(event) => {
            if (event.button !== 0) return;
            resize.current = { pointerId: event.pointerId, x: event.clientX, width: panelWidth };
            event.currentTarget.setPointerCapture(event.pointerId); event.preventDefault();
          }} onPointerMove={(event) => {
            if (resize.current?.pointerId !== event.pointerId) return;
            setWidth(Math.min(limit, resize.current.width + event.clientX - resize.current.x));
          }} onPointerUp={finishResize} onPointerCancel={finishResize} onLostPointerCapture={finishResize} />
      </aside>
      <main className="workbench-stage"><section className="viewport-frame" aria-label="三维视口" onContextMenu={(event) => event.preventDefault()}>{children}
        {!treeOpen && <Button className="workbench-tree-restore" icon={<MenuUnfoldOutlined />}
          aria-label="展开模型结构" onClick={() => setTreeOpen(true)} />}
        <Button className="workbench-inspector-toggle" icon={inspectorOpen ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
          aria-label={inspectorOpen ? "收起属性面板" : "展开属性面板"} aria-expanded={inspectorOpen}
          onClick={() => onInspectorChange(!inspectorOpen)} />
      </section></main>
      {inspectorOpen && <aside className="workbench-inspector" aria-label="属性与历史">{inspector}</aside>}
    </div>
    {status}
  </div>;
}
