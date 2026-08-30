import { createPortal } from "react-dom";
import { useEffect, useRef, useState, type PropsWithChildren, type ReactNode } from "react";

export function CursorTooltip({ title, disabled = false, children }: PropsWithChildren<{ title: ReactNode; disabled?: boolean }>) {
  const timer = useRef<number | undefined>(undefined);
  const [position, setPosition] = useState<{ x: number; y: number }>();
  const pointer = useRef({ x: 0, y: 0 });
  const clear = () => { window.clearTimeout(timer.current); setPosition(undefined); };
  useEffect(() => () => window.clearTimeout(timer.current), []);

  return <span className="cad-cursor-tooltip-target"
    onPointerEnter={(event) => {
      if (disabled || !title) return;
      pointer.current = { x: event.clientX, y: event.clientY };
      window.clearTimeout(timer.current);
      timer.current = window.setTimeout(() => setPosition(pointer.current), 420);
    }}
    onPointerMove={(event) => {
      pointer.current = { x: event.clientX, y: event.clientY };
      if (position) setPosition(pointer.current);
    }}
    onPointerLeave={clear} onPointerDown={clear}>
    {children}
    {position && !disabled && createPortal(<span className="cad-cursor-tooltip"
      style={{ left: position.x + 12, top: position.y + 16 }}>{title}</span>, document.body)}
  </span>;
}
