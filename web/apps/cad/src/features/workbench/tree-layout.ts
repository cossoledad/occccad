import type {SpecificationTreeNode} from './specification-tree';

export type VisibleTreeNode = {
  node: SpecificationTreeNode; depth: number; hasChildren: boolean;
  lastSibling: boolean; continuationLevels: number[];
};

/** Branch geometry follows actual siblings, independent of the virtual window. */
export function visibleTreeLayout(nodes: SpecificationTreeNode[], expanded: ReadonlySet<string>): VisibleTreeNode[] {
  const output: VisibleTreeNode[] = [];
  const visit = (siblings: SpecificationTreeNode[], depth: number, continuations: number[]) => {
    siblings.forEach((node, index) => {
      const hasChildren = Boolean(node.children?.length), lastSibling = index === siblings.length - 1;
      output.push({node, depth, hasChildren, lastSibling, continuationLevels: continuations});
      if (hasChildren && (depth === 0 || expanded.has(node.key))) {
        visit(node.children!, depth + 1, depth > 0 && !lastSibling ? [...continuations, depth - 1] : continuations);
      }
    });
  };
  visit(nodes, 0, []);
  return output;
}
