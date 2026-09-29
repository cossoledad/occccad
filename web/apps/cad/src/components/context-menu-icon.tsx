import type { ReactNode } from "react";

/** Keeps every actionable context-menu row on the same text grid. */
export function ContextMenuIcon({ children }: { children?: ReactNode }) {
  return <span className="context-menu-icon-slot" aria-hidden="true">{children}</span>;
}
