import { DeleteOutlined } from "@ant-design/icons";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Dropdown } from "antd";
import { useEffect, useMemo, useRef, useState, type CSSProperties, type KeyboardEvent, type MouseEvent, type ReactNode } from "react";
import { selectionKey, selectionSetToken } from "../../cad/interaction/selection-identity";
import type { Selection } from "../../types";
import { resolveTreeSelection, type TreeSelectionModifiers } from "./tree-selection";

export type SpecificationTreeNode = {
  key: string; title: ReactNode; icon?: ReactNode; children?: SpecificationTreeNode[];
  kind?: string; entityId?: string; documentId?: string; plane?: string; selection?: Selection;
  capabilities?: Array<"DELETE">; ownerEntityId?: string;
};

type VisibleNode = { node: SpecificationTreeNode; depth: number; hasChildren: boolean };

function flatten(nodes: SpecificationTreeNode[], expanded: Set<string>, depth = 0, output: VisibleNode[] = []): VisibleNode[] {
  for (const node of nodes) {
    const hasChildren = Boolean(node.children?.length);
    output.push({ node, depth, hasChildren });
    if (hasChildren && (depth === 0 || expanded.has(node.key))) flatten(node.children!, expanded, depth + 1, output);
  }
  return output;
}

function ancestorsOf(nodes: SpecificationTreeNode[], target: string, parents: string[] = []): string[] | undefined {
  for (const node of nodes) {
    if (node.key === target) return parents;
    const found = node.children ? ancestorsOf(node.children, target, [...parents, node.key]) : undefined;
    if (found) return found;
  }
  return undefined;
}

function indexNodes(nodes: SpecificationTreeNode[], output = new Map<string, SpecificationTreeNode>()): Map<string, SpecificationTreeNode> {
  for (const node of nodes) { output.set(node.key, node); if (node.children) indexNodes(node.children, output); }
  return output;
}

export function SpecificationTree({ nodes, selectedKeys, selectedIdentityKeys, selectionToken, highlightedKey, onSelect, onActivate, onHover, onDelete }: {
  nodes: SpecificationTreeNode[]; selectedKeys: readonly string[]; selectedIdentityKeys: readonly string[];
  selectionToken: string; highlightedKey?: string;
  onSelect: (nodes: SpecificationTreeNode[]) => void; onHover?: (node?: SpecificationTreeNode) => void;
  onActivate?: (node: SpecificationTreeNode) => void;
  onDelete?: (nodes: SpecificationTreeNode[]) => void;
}) {
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const [contextMenu, setContextMenu] = useState<{ nodeKey: string; selectionSignature: string }>();
  const anchorKey = useRef<string | undefined>(undefined);
  const scrollElement = useRef<HTMLElement>(null);
  const visible = useMemo(() => flatten(nodes, expanded), [nodes, expanded]);
  const nodeIndex = useMemo(() => indexNodes(nodes), [nodes]);
  const visibleKeys = useMemo(() => visible.map((entry) => entry.node.key), [visible]);
  const selected = useMemo(() => new Set(selectedKeys), [selectedKeys]);
  useEffect(() => {
    const available = new Set(visible.filter((entry) => entry.hasChildren).map((entry) => entry.node.key));
    setExpanded((current) => new Set([...current].filter((key) => available.has(key))));
  // Node identity changes only when a new document view arrives.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nodes]);
  useEffect(() => {
    const ancestors = selectedKeys.flatMap((selectedKey) => ancestorsOf(nodes, selectedKey) ?? []);
    if (ancestors.length) setExpanded((current) => new Set([...current, ...ancestors]));
  }, [nodes, selectedKeys]);
  useEffect(() => {
    setContextMenu((current) => current && current.selectionSignature !== selectionToken ? undefined : current);
  }, [selectionToken]);
  const virtualizer = useVirtualizer({ count: visible.length, getScrollElement: () => scrollElement.current,
    estimateSize: () => 27, overscan: 14, getItemKey: (index) => visible[index]?.node.key ?? index });

  const toggle = (key: string) => setExpanded((current) => {
    const next = new Set(current); if (next.has(key)) next.delete(key); else next.add(key); return next;
  });
  const selectNode = (node: SpecificationTreeNode, modifiers: TreeSelectionModifiers) => {
    setContextMenu(undefined);
    const keys = resolveTreeSelection(visibleKeys, selectedKeys, node.key, anchorKey.current, modifiers);
    if (!modifiers.shift) anchorKey.current = node.key;
    onSelect(keys.map((key) => nodeIndex.get(key)).filter((item): item is SpecificationTreeNode => Boolean(item)));
  };
  const eventModifiers = (event: { ctrlKey: boolean; metaKey: boolean; shiftKey: boolean }): TreeSelectionModifiers =>
    ({ ctrl: event.ctrlKey, meta: event.metaKey, shift: event.shiftKey });
  const keyboardSelect = (event: KeyboardEvent, entry: VisibleNode) => {
    if (event.key === "Enter" || event.key === " ") { event.preventDefault(); selectNode(entry.node, eventModifiers(event)); }
    if (event.key === "ArrowRight" && entry.hasChildren) setExpanded((current) => new Set(current).add(entry.node.key));
    if (event.key === "ArrowLeft" && entry.hasChildren) setExpanded((current) => {
      const next = new Set(current); next.delete(entry.node.key); return next;
    });
  };
  const contextSelection = (event: MouseEvent<HTMLElement>, node: SpecificationTreeNode) => {
    event.preventDefault(); event.stopPropagation();
    const exactNodeSelected = Boolean(node.selection && selectedIdentityKeys.includes(selectionKey(node.selection)));
    const menuSelectionSignature = exactNodeSelected ? selectionToken : node.selection ? selectionSetToken([node.selection]) : "";
    if (!exactNodeSelected) {
      anchorKey.current = node.key;
      onSelect([node]);
    }
    setContextMenu({ nodeKey: node.key, selectionSignature: menuSelectionSignature });
    event.currentTarget.focus();
  };
  const clearFromBlank = (event: MouseEvent) => {
    if (event.target !== event.currentTarget) return;
    setContextMenu(undefined); anchorKey.current = undefined; onSelect([]);
  };

  return <nav ref={scrollElement} className="specification-tree specification-tree-virtual" aria-label="Specification tree"
    role="tree" onMouseLeave={() => onHover?.()} onClick={clearFromBlank}>
    <div className="specification-tree-virtual-space" style={{ height: virtualizer.getTotalSize() }} onClick={clearFromBlank}>
      {virtualizer.getVirtualItems().map((item) => {
        const entry = visible[item.index];
        const { node, depth, hasChildren } = entry;
        const isExpanded = hasChildren && (depth === 0 || expanded.has(node.key));
        const isSelected = selected.has(node.key);
        const selectedNodes = isSelected
          ? selectedKeys.map((key) => nodeIndex.get(key)).filter((candidate): candidate is SpecificationTreeNode => Boolean(candidate)) : [node];
        const deletable = selectedNodes.filter((candidate) => candidate.capabilities?.includes("DELETE"));
        const rowStyle = { transform: `translateY(${item.start}px)`, paddingLeft: depth * 31,
          "--tree-depth": depth } as CSSProperties;
        const row = <div className={`specification-tree-row ${isSelected ? "selected" : ""} ${highlightedKey === node.key ? "highlighted" : ""}`}
          role="treeitem" aria-level={depth + 1} aria-expanded={hasChildren ? isExpanded : undefined}
          aria-selected={isSelected} tabIndex={0} onClick={(event) => { event.stopPropagation(); selectNode(node, eventModifiers(event)); }}
          onDoubleClick={(event) => { event.stopPropagation(); onActivate?.(node); }}
          onContextMenu={(event) => contextSelection(event, node)}
          onMouseEnter={() => onHover?.(node)} onKeyDown={(event) => keyboardSelect(event, entry)}>
          {depth === 0 ? <span className="specification-tree-root-anchor" /> : hasChildren
            ? <button className={`specification-tree-junction branch ${isExpanded ? "expanded" : "collapsed"}`}
              tabIndex={-1} aria-label={isExpanded ? "折叠" : "展开"}
              onClick={(event) => { event.stopPropagation(); toggle(node.key); }}>
              {isExpanded && <><i /><i /><i /><i /></>}
            </button> : <span className="specification-tree-junction leaf" />}
          <span className="specification-tree-icon">{node.icon}</span>
          <span className="specification-tree-label">{node.title}</span>
        </div>;
        return <div key={node.key} className={`specification-tree-virtual-row ${depth > 0 ? "nested" : "root"}`} style={rowStyle}>
          {Array.from({ length: depth }, (_, guide) => <i key={guide} className="specification-tree-depth-guide"
            style={{ left: guide * 31 + 13 }} />)}
          <Dropdown trigger={[]} placement="bottomLeft" overlayClassName="specification-tree-context-menu"
            open={contextMenu?.nodeKey === node.key}
            onOpenChange={(open) => { if (!open) setContextMenu(undefined); }}
            menu={{ items: [{ key: "delete", icon: <DeleteOutlined />, danger: true,
            label: deletable.length > 1 ? `删除 ${deletable.length} 项` : "删除", disabled: deletable.length === 0,
            onClick: () => { setContextMenu(undefined); onDelete?.(deletable); } }] }}>{row}</Dropdown>
        </div>;
      })}
    </div>
  </nav>;
}
