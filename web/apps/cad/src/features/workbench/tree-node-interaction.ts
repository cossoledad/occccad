import type { SpecificationTreeNode } from "./specification-tree";

/** Editable nodes always use the same definition command, including future kinds. */
export function defaultTreeCommand(node:SpecificationTreeNode):"tree.edit"|"tree.activate"|undefined {
  if(node.presentationRole==="INPUT_REFERENCE")return node.capabilities?.includes("ACTIVATE")?"tree.activate":undefined;
  if(node.capabilities?.includes("EDIT"))return "tree.edit";
  if(node.capabilities?.includes("ACTIVATE")||["BODY","PART","PRODUCT","INSTANCE","MECHANISM","MECHANISM_REFERENCE","SKETCH"].includes(node.kind??""))return "tree.activate";
}
