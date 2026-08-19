import { useVirtualizer } from "@tanstack/react-virtual";
import { DeleteOutlined } from "@ant-design/icons";
import { useEffect, useMemo, useRef, useState, type CSSProperties, type KeyboardEvent, type ReactNode } from "react";
import type { Selection } from "../../types";

export type SpecificationTreeNode = {
  key: string;
  title: ReactNode;
  icon?: ReactNode;
  children?: SpecificationTreeNode[];
  kind?: string;
  entityId?: string;
  documentId?: string;
  plane?: string;
  selection?: Selection;
  capabilities?: Array<"DELETE">;
  ownerEntityId?: string;
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

export function SpecificationTree({ nodes, selectedKey, highlightedKey, onSelect, onHover, onDelete }: {
  nodes: SpecificationTreeNode[];
  selectedKey?: string;
  highlightedKey?: string;
  onSelect: (node: SpecificationTreeNode) => void;
  onHover?: (node?: SpecificationTreeNode) => void;
  onDelete?: (node: SpecificationTreeNode) => void;
}) {
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const scrollElement = useRef<HTMLElement>(null);
  const visible = useMemo(() => flatten(nodes, expanded), [nodes, expanded]);
  useEffect(() => {
    const available = new Set(visible.filter((entry) => entry.hasChildren).map((entry) => entry.node.key));
    setExpanded((current) => new Set([...current].filter((key) => available.has(key))));
  // Node identity changes only when a new document view arrives.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nodes]);
  useEffect(() => {
    if (!selectedKey) return;
    const ancestors = ancestorsOf(nodes, selectedKey);
    if (ancestors?.length) setExpanded((current) => new Set([...current, ...ancestors]));
  }, [nodes, selectedKey]);
  const virtualizer = useVirtualizer({ count: visible.length, getScrollElement: () => scrollElement.current,
    estimateSize: () => 27, overscan: 14, getItemKey: (index) => visible[index]?.node.key ?? index });

  const toggle = (key: string) => setExpanded((current) => {
    const next = new Set(current); if (next.has(key)) next.delete(key); else next.add(key); return next;
  });
  const keyboardSelect = (event: KeyboardEvent, entry: VisibleNode) => {
    if (event.key === "Enter" || event.key === " ") { event.preventDefault(); onSelect(entry.node); }
    if (event.key === "ArrowRight" && entry.hasChildren) setExpanded((current) => new Set(current).add(entry.node.key));
    if (event.key === "ArrowLeft" && entry.hasChildren) setExpanded((current) => {
      const next = new Set(current); next.delete(entry.node.key); return next;
    });
    if (event.key === "Delete" && entry.node.capabilities?.includes("DELETE")) {
      event.preventDefault(); onDelete?.(entry.node);
    }
  };

  return <nav ref={scrollElement} className="specification-tree specification-tree-virtual" aria-label="Specification tree"
    role="tree" onMouseLeave={() => onHover?.()}>
    <div className="specification-tree-virtual-space" style={{ height: virtualizer.getTotalSize() }}>
      {virtualizer.getVirtualItems().map((item) => {
        const entry = visible[item.index];
        const { node, depth, hasChildren } = entry;
        const isExpanded = hasChildren && (depth === 0 || expanded.has(node.key));
        const rowStyle = { transform: `translateY(${item.start}px)`, paddingLeft: depth * 31,
          "--tree-depth": depth } as CSSProperties;
        return <div key={node.key} className={`specification-tree-virtual-row ${depth > 0 ? "nested" : "root"}`} style={rowStyle}>
          {Array.from({ length: depth }, (_, guide) => <i key={guide} className="specification-tree-depth-guide"
            style={{ left: guide * 31 + 13 }} />)}
          <div className={`specification-tree-row ${selectedKey === node.key ? "selected" : ""} ${highlightedKey === node.key ? "highlighted" : ""}`}
            role="treeitem" aria-level={depth + 1} aria-expanded={hasChildren ? isExpanded : undefined}
            aria-selected={selectedKey === node.key} tabIndex={0} onClick={() => onSelect(node)}
            onMouseEnter={() => onHover?.(node)} onKeyDown={(event) => keyboardSelect(event, entry)}>
            {depth === 0 ? <span className="specification-tree-root-anchor" /> : hasChildren
              ? <button className={`specification-tree-junction branch ${isExpanded ? "expanded" : "collapsed"}`}
                tabIndex={-1} aria-label={isExpanded ? "折叠" : "展开"}
                onClick={(event) => { event.stopPropagation(); toggle(node.key); }}>
                {isExpanded && <><i /><i /><i /><i /></>}
              </button> : <span className="specification-tree-junction leaf" />}
            <span className="specification-tree-icon">{node.icon}</span>
            <span className="specification-tree-label">{node.title}</span>
            {node.capabilities?.includes("DELETE") && <button className="specification-tree-delete" title="删除" aria-label={`删除 ${String(node.title)}`}
              onClick={(event) => { event.stopPropagation(); onDelete?.(node); }}><DeleteOutlined /></button>}
          </div>
        </div>;
      })}
    </div>
  </nav>;
}
