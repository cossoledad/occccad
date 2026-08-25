export type TreeSelectionModifiers = { ctrl?: boolean; meta?: boolean; shift?: boolean };
export type TreeKeyNode = { key: string; children?: TreeKeyNode[] };

export function closestTreeKey(nodes: readonly TreeKeyNode[], treeNodeId: string | undefined): string | undefined {
  if (!treeNodeId) return undefined;
  let closest: string | undefined;
  const visit = (items: readonly TreeKeyNode[]) => {
    for (const node of items) {
      if (treeNodeId === node.key) { closest = node.key; return; }
      if (treeNodeId.startsWith(`${node.key}/`) && node.key.length > (closest?.length ?? -1)) closest = node.key;
      if (node.children) visit(node.children);
      if (closest === treeNodeId) return;
    }
  };
  visit(nodes);
  return closest;
}

export function resolveTreeSelection(visibleKeys: readonly string[], selectedKeys: readonly string[], targetKey: string,
  anchorKey: string | undefined, modifiers: TreeSelectionModifiers): string[] {
  const additive = Boolean(modifiers.ctrl || modifiers.meta);
  if (modifiers.shift && anchorKey) {
    const anchorIndex = visibleKeys.indexOf(anchorKey), targetIndex = visibleKeys.indexOf(targetKey);
    if (anchorIndex >= 0 && targetIndex >= 0) {
      const from = Math.min(anchorIndex, targetIndex), to = Math.max(anchorIndex, targetIndex);
      const range = visibleKeys.slice(from, to + 1);
      const combined = additive ? [...selectedKeys, ...range] : range;
      return [...new Set(combined.filter((key) => key !== targetKey)), targetKey];
    }
  }
  if (additive) {
    return selectedKeys.includes(targetKey)
      ? selectedKeys.filter((key) => key !== targetKey)
      : [...selectedKeys, targetKey];
  }
  return [targetKey];
}
