import { lazy, type ComponentPropsWithRef, type ComponentProps } from "react";
import type { CadViewport } from "../../viewport/cad-viewport";
import { useWorkbenchStore } from "../../state/workbench-store";
import { SpecificationTree } from "./specification-tree";
import { associatedTreeKeyForSelection, treeKeyForSelection, treeKeysForSelections } from "./workbench-tree-model";

const Viewport = lazy(() => import("../../viewport/cad-viewport").then(module => ({default: module.CadViewport})));

// Hover is consumed at the two display boundaries, without reconciling the
// command forms, queries and the rest of the workbench on every pointer move.
export function WorkbenchViewport(props: Omit<ComponentPropsWithRef<typeof CadViewport>, "preselection">) {
  const preselection = useWorkbenchStore(state => state.preselection);
  return <Viewport {...props} preselection={preselection}/>;
}

export function WorkbenchTree(props: Omit<ComponentProps<typeof SpecificationTree>, "highlightedKey" | "highlightedKeys">) {
  const preselection = useWorkbenchStore(state => state.preselection);
  return <SpecificationTree {...props}
    highlightedKey={treeKeyForSelection(props.nodes, preselection) ?? associatedTreeKeyForSelection(props.nodes, preselection)}
    highlightedKeys={treeKeysForSelections(props.nodes, preselection ? [preselection] : [])}/>;
}
