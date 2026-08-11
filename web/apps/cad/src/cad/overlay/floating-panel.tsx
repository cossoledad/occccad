import type { CSSProperties, PropsWithChildren, ReactNode } from "react";

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

export function FloatingToolbar({ children, orientation = "horizontal", position = "top-center", className = "" }:
  FloatingPanelProps & { orientation?: "horizontal" | "vertical" }) {
  const style = { "--toolbar-direction": orientation === "horizontal" ? "row" : "column" } as CSSProperties;
  return <FloatingPanel position={position} className={`cad-floating-toolbar ${className}`}>
    <div className="cad-floating-toolbar-content" style={style}>{children}</div>
  </FloatingPanel>;
}

export function ToolbarGroup({ children }: PropsWithChildren) {
  return <div className="cad-toolbar-group">{children}</div>;
}

export function ToolbarSeparator() { return <span className="cad-toolbar-separator" aria-hidden />; }

