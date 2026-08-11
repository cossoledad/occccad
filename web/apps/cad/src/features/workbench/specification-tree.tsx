import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";

export type SpecificationTreeNode = {
  key: string;
  title: ReactNode;
  icon?: ReactNode;
  children?: SpecificationTreeNode[];
  kind?: string;
  entityId?: string;
  documentId?: string;
  plane?: string;
};

function branchKeys(nodes: SpecificationTreeNode[], result = new Set<string>()): Set<string> {
  for (const node of nodes) {
    if (node.children?.length) {
      result.add(node.key);
      branchKeys(node.children, result);
    }
  }
  return result;
}

export function SpecificationTree({ nodes, selectedKey, onSelect }: {
  nodes: SpecificationTreeNode[];
  selectedKey?: string;
  onSelect: (node: SpecificationTreeNode) => void;
}) {
  const initialBranches = branchKeys(nodes);
  const knownBranches = useRef(initialBranches);
  const [expanded, setExpanded] = useState<Set<string>>(() => initialBranches);
  useEffect(() => {
    const currentBranches = branchKeys(nodes);
    setExpanded((current) => new Set([
      ...[...current].filter((key) => currentBranches.has(key)),
      ...[...currentBranches].filter((key) => !knownBranches.current.has(key)),
    ]));
    knownBranches.current = currentBranches;
  }, [nodes]);

  const toggle = (key: string) => setExpanded((current) => {
    const next = new Set(current);
    if (next.has(key)) next.delete(key); else next.add(key);
    return next;
  });
  const keyboardSelect = (event: KeyboardEvent, node: SpecificationTreeNode) => {
    if (event.key === "Enter" || event.key === " ") { event.preventDefault(); onSelect(node); }
    if (event.key === "ArrowRight" && node.children?.length) setExpanded((current) => new Set(current).add(node.key));
    if (event.key === "ArrowLeft" && node.children?.length) setExpanded((current) => {
      const next = new Set(current); next.delete(node.key); return next;
    });
  };

  const renderNodes = (items: SpecificationTreeNode[], nested = false): ReactNode => <ul
    className={`specification-tree-level ${nested ? "nested" : "root"}`} role={nested ? "group" : "tree"}>
    {items.map((node) => {
      const hasChildren = Boolean(node.children?.length);
      const isExpanded = hasChildren && (!nested || expanded.has(node.key));
      return <li className="specification-tree-node" key={node.key}>
        <div className={`specification-tree-row ${selectedKey === node.key ? "selected" : ""}`}
          role="treeitem" aria-expanded={hasChildren ? isExpanded : undefined} aria-selected={selectedKey === node.key}
          tabIndex={0} onClick={() => onSelect(node)} onKeyDown={(event) => keyboardSelect(event, node)}>
          {!nested ? <span className="specification-tree-root-anchor" /> : hasChildren
            ? <button className={`specification-tree-junction branch ${isExpanded ? "expanded" : "collapsed"}`}
              tabIndex={-1} aria-label={isExpanded ? "折叠" : "展开"}
            onClick={(event) => { event.stopPropagation(); toggle(node.key); }}>
              {isExpanded && <><i /><i /><i /><i /></>}
            </button> : <span className="specification-tree-junction leaf" />}
          <span className="specification-tree-icon">{node.icon}</span>
          <span className="specification-tree-label">{node.title}</span>
        </div>
        {isExpanded && node.children ? renderNodes(node.children, true) : null}
      </li>;
    })}
  </ul>;

  return <nav className="specification-tree" aria-label="Specification tree">{renderNodes(nodes)}</nav>;
}
