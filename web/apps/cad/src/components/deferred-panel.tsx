import { createElement, lazy, Suspense, type ComponentProps, type ComponentType } from "react";

/** Keep code loading local to a conditional panel; the viewport stays mounted. */
export function deferredPanel<PanelType extends ComponentType<any>>(load: () => Promise<{default: PanelType}>) {
  const Panel = lazy(load);
  return function DeferredPanel(props: ComponentProps<PanelType>) {
    return <Suspense fallback={null}>{createElement(Panel, props)}</Suspense>;
  };
}
