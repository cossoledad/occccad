import type { ReactNode } from "react";

/** Keeps every actionable context-menu row on the same text grid. */
export function ContextMenuIcon({ children,className="" }: { children?: ReactNode;className?:string }) {
  return <span className={`context-menu-icon-slot ${className}`.trim()} aria-hidden="true">{children}</span>;
}
