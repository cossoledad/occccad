/** Preserve ancestor paths and original stable identities. A matching branch keeps its subtree. */
export function filterTree<T extends { children?: T[] }>(nodes: readonly T[], query: string,
  label: (node: T) => string): T[] {
  const term = query.trim().toLocaleLowerCase();
  if (!term) return [...nodes];
  const visit = (items: readonly T[]): T[] => items.flatMap((node) => {
    if (label(node).toLocaleLowerCase().includes(term)) return [node];
    const children = node.children ? visit(node.children) : [];
    return children.length ? [{ ...node, children }] : [];
  });
  return visit(nodes);
}
