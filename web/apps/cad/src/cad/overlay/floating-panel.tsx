import {CommandFormContext,useCommandForm} from "./command-form";
import { clampPanelPosition, normalizePanelPosition, initialCommandPanelPosition } from "../../utils/panel-position";
import {useOperationFeedback} from "../command/operation-feedback";
import {commandPanelWidths,type CommandPanelSize} from "../../design/visual-tokens";
import { CloseOutlined } from "@ant-design/icons";
import {createPortal} from "react-dom";
import { App, Button } from "antd";
import { useEffect, useRef, useState, type CSSProperties, type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent, type PropsWithChildren, type ReactNode } from "react";
import { normalizeToolbarLayout, useUIPreferences, type ToolbarLayout, type ToolbarOrientation } from "../../state/ui-preferences";

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

export function FloatingToolbar({ children, id, label, orientation = "horizontal", position = "top-center", className = "", stackIndex = 0 }:
  FloatingPanelProps & { id: string; label?: string; orientation?: ToolbarOrientation; stackIndex?: number }) {
  const savedLayout = useUIPreferences((state) => state.toolbarLayouts[id]);
  const saveLayout = useUIPreferences((state) => state.setToolbarLayout);
  const [layout, setLayout] = useState<ToolbarLayout>(() => normalizeToolbarLayout(savedLayout, orientation));
  const toolbar = useRef<HTMLElement>(null);
  const layoutRef = useRef(layout);
  layoutRef.current = layout;
  const drag = useRef<{ pointerId: number; offsetX: number; offsetY: number } | undefined>(undefined);
  const placed = layout.x !== undefined && layout.y !== undefined;
  const style = {
    "--toolbar-direction": layout.orientation === "horizontal" ? "row" : "column",
    "--toolbar-stack-index": stackIndex,
    ...(placed ? { left: layout.x, top: layout.y, right: "auto", bottom: "auto", transform: "none" } : {}),
  } as CSSProperties;

  const persist = (next: ToolbarLayout) => {
    layoutRef.current = next;
    setLayout(next);
    saveLayout(id, next);
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
      title={label}
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

export function CommandDialog({ id, open, title, children, onClose, onConfirm, confirmText="确定",cancelText="取消", confirmLoading = false, confirmDisabled = false, size = "M",footer=true,placement,onCancelPick,secondaryActions,commandStatus,bodyClassName="",scrollBody=true }: PropsWithChildren<{
  bodyClassName?:string;scrollBody?:boolean;id: string; open: boolean; title: ReactNode; onClose: () => void; onConfirm: () => void | Promise<void>;
  confirmText?:string;cancelText?:string;confirmLoading?: boolean; confirmDisabled?: boolean; size?:CommandPanelSize;footer?:boolean;placement?:'left'|'right';onCancelPick?:()=>boolean;secondaryActions?:ReactNode;commandStatus?:ReactNode;
}>) {
  const feedback=useOperationFeedback(),parentForm=useCommandForm();
  const openRef=useRef(open);openRef.current=open;
  const cancelPickRef=useRef(onCancelPick);cancelPickRef.current=onCancelPick;
  const savedPosition = useUIPreferences((state) => state.commandDialogPositions[id]);
  const savePosition = useUIPreferences((state) => state.setCommandDialogPosition);
  const width=commandPanelWidths[size];
  const placed=useRef(Boolean(normalizePanelPosition(savedPosition)));
  const [position, setPosition] = useState(() => normalizePanelPosition(savedPosition) ?? { x: 16, y: 16 });
  const [submitting, setSubmitting] = useState(false);
  const submittingRef = useRef(false);
  const closeRef = useRef(onClose); closeRef.current = onClose;
  const dialog = useRef<HTMLElement>(null);
  const confirmRef=useRef<()=>Promise<void>>(async()=>{}),enterFrame=useRef<number|undefined>(undefined),composing=useRef(false),compositionFrame=useRef<number|undefined>(undefined);
  useEffect(()=>()=>{if(enterFrame.current!==undefined)cancelAnimationFrame(enterFrame.current);if(compositionFrame.current!==undefined)cancelAnimationFrame(compositionFrame.current);},[]);
  const [overlayHost,setOverlayHost]=useState<HTMLElement|null>(()=>typeof document!=="undefined"?document.querySelector?.<HTMLElement>('[data-command-overlay-host]')??document.body:null);
  useEffect(()=>{if(open)setOverlayHost(document.querySelector<HTMLElement>('[data-command-overlay-host]')??document.body);},[open]);
  const positionRef = useRef(position); positionRef.current = position;
  const drag = useRef<{ pointerId: number; offsetX: number; offsetY: number } | undefined>(undefined);
  useEffect(() => {
    if (!open || !overlayHost || dialog.current?.parentElement !== overlayHost) return;
    const element = dialog.current;
    const parent = element.parentElement!;
    const keepInBounds = () => {
      const host=parent.getBoundingClientRect(),panel=element.getBoundingClientRect();
      // DOM rectangles include CSS/browser zoom; saved positions are host-local CSS pixels.
      const scale=host.width/parent.clientWidth||1;
      const hostSize={width:host.width/scale,height:host.height/scale},panelSize={width:panel.width/scale,height:panel.height/scale};
      const viewport=parent.querySelector<HTMLElement>(".viewport-frame")?.getBoundingClientRect()??host;
      const next=placed.current?clampPanelPosition(positionRef.current,panelSize,hostSize):placement?clampPanelPosition({x:placement==='left'?16:hostSize.width-panelSize.width-16,y:(viewport.top-host.top)/scale+16},panelSize,hostSize):initialCommandPanelPosition(panelSize,hostSize,{left:(viewport.left-host.left)/scale,top:(viewport.top-host.top)/scale,width:viewport.width/scale,height:viewport.height/scale},size==="L");
      placed.current=true;
      if (next.x !== positionRef.current.x || next.y !== positionRef.current.y) {
        positionRef.current = next; setPosition(next);
      }
    };
    keepInBounds();
    const observer = new ResizeObserver(keepInBounds);
    observer.observe(parent); observer.observe(element);
    const previousFocus = document.activeElement;
    element.querySelector<HTMLElement>(".cad-command-dialog-body input:not(:disabled):not([readonly]):not([type=hidden]), .cad-command-dialog-body textarea:not(:disabled):not([readonly]), .cad-command-dialog-body select:not(:disabled), .cad-command-dialog-body button:not(:disabled)")?.focus({preventScroll:true});
    const keyDown = (event: globalThis.KeyboardEvent) => {
      const topmost = [...document.querySelectorAll(".cad-command-dialog")].at(-1);
      if (event.key === "Escape" && !event.isComposing && event.keyCode !== 229 && !event.defaultPrevented && topmost === element
        && !document.querySelector('[role="dialog"][aria-modal="true"], .ant-dropdown:not(.ant-dropdown-hidden), .ant-select-dropdown:not(.ant-select-dropdown-hidden)')) {
        event.preventDefault(); if(!cancelPickRef.current?.())closeRef.current();
      }
    };
    window.addEventListener("keydown", keyDown);
    return () => {
      observer.disconnect(); window.removeEventListener("keydown", keyDown);
      if (previousFocus instanceof HTMLElement && previousFocus.isConnected) previousFocus.focus();
    };
  }, [open,overlayHost,size]);
  if (!open) return null;
  const pointerDown = (event: ReactPointerEvent<HTMLElement>) => {
    if (event.button !== 0) return;
    const rect = dialog.current?.getBoundingClientRect(); if (!rect) return;
    drag.current = { pointerId: event.pointerId, offsetX: event.clientX - rect.left, offsetY: event.clientY - rect.top };
    event.currentTarget.setPointerCapture(event.pointerId); event.preventDefault();
  };
  const pointerMove = (event: ReactPointerEvent<HTMLElement>) => {
    if (drag.current?.pointerId !== event.pointerId || !dialog.current?.parentElement) return;
    const parent = dialog.current.parentElement.getBoundingClientRect(),scale=parent.width/dialog.current.parentElement.clientWidth||1;
    const panel=dialog.current.getBoundingClientRect();
    const next = clampPanelPosition({ x: (event.clientX - parent.left - drag.current.offsetX)/scale,
      y: (event.clientY - parent.top - drag.current.offsetY)/scale }, {width:panel.width/scale,height:panel.height/scale}, {width:parent.width/scale,height:parent.height/scale});
    positionRef.current = next; setPosition(next);
  };
  const pointerUp = (event: ReactPointerEvent<HTMLElement>) => {
    if (drag.current?.pointerId !== event.pointerId) return;
    drag.current = undefined;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    savePosition(id, positionRef.current);
  };
  const confirm = async () => {
    if (submittingRef.current || confirmLoading || confirmDisabled) return;
    submittingRef.current = true; setSubmitting(true);
    try { await onConfirm(); }
    catch (error) {
      // Ant Form owns field validation feedback; unexpected command failures remain observable.
      if (!(error && typeof error === "object" && "errorFields" in error)) feedback(error,"命令");
    } finally { submittingRef.current = false; setSubmitting(false); }
  };
  confirmRef.current=confirm;
  const content=<section ref={dialog} className="cad-command-dialog" role="dialog" aria-modal="false" aria-label={String(title)} data-panel-size={size}
    onCompositionStart={()=>{composing.current=true;if(compositionFrame.current!==undefined)cancelAnimationFrame(compositionFrame.current);}} onCompositionEnd={()=>{compositionFrame.current=requestAnimationFrame(()=>{composing.current=false;});}}
    onPointerDown={event=>event.stopPropagation()} onClick={event=>event.stopPropagation()} onWheel={event=>event.stopPropagation()} onContextMenu={event=>event.stopPropagation()}
    onPointerMove={event=>event.stopPropagation()} onPointerUp={event=>event.stopPropagation()} onKeyDown={event=>{
      if(event.key!=="Escape")event.stopPropagation();
      const target=event.target instanceof HTMLElement?event.target:undefined;
      if(footer&&event.key==="Enter"&&!composing.current&&!event.repeat&&!event.defaultPrevented&&!event.nativeEvent.isComposing&&event.nativeEvent.keyCode!==229
        &&target?.tagName==="INPUT"&&!target.isContentEditable&&!target.closest("[role='combobox'], textarea, [contenteditable='true']")){
        event.preventDefault();
        if(enterFrame.current===undefined)enterFrame.current=requestAnimationFrame(()=>{enterFrame.current=undefined;if(openRef.current)void confirmRef.current();});
      }
    }}
    style={{ left: position.x, top: position.y, width }}>
    <header className="cad-command-dialog-header" style={{flexShrink:0}} onPointerDown={pointerDown} onPointerMove={pointerMove}
      onPointerUp={pointerUp} onPointerCancel={pointerUp} onLostPointerCapture={pointerUp}>
      <strong>{title}</strong><button aria-label="关闭" onPointerDown={(event) => {event.preventDefault();event.stopPropagation();}}
        onClick={onClose}><CloseOutlined /></button>
    </header>
    <div className={`cad-command-dialog-body command-form ${bodyClassName}`} style={{overflowY:scrollBody?"auto":"hidden",minHeight:0}}>{children}</div>
    {/* Cancel must not blur/commit the focused draft input before its click. */}
    {footer&&<footer className="cad-command-dialog-footer" style={{flexShrink:0}}><div className="command-secondary-actions">{commandStatus}{secondaryActions}</div><Button onPointerDown={event=>event.preventDefault()} onClick={onClose}>{cancelText}</Button>
      <Button type="primary" loading={confirmLoading || submitting} disabled={confirmDisabled}
        onClick={() => void confirm()}>{confirmText}</Button></footer>}
  </section>;
  const framed=<CommandFormContext.Provider value={{...parentForm,cancelPick:onCancelPick}}>{content}</CommandFormContext.Provider>;
  return overlayHost?createPortal(framed,overlayHost):framed;
}
