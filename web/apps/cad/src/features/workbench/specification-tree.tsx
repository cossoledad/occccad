import { SearchOutlined, DeleteOutlined, EditOutlined, EyeInvisibleOutlined, LinkOutlined, LockOutlined, PauseCircleOutlined, PlusOutlined, ReloadOutlined, SwapOutlined, UnlockOutlined } from "@ant-design/icons";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Dropdown, Input } from "antd";
import { isValidElement, useEffect, useMemo, useRef, useState, type CSSProperties, type KeyboardEvent, type MouseEvent, type ReactNode } from "react";
import { selectionKey, selectionSetToken } from "../../cad/interaction/selection-identity";
import type { InstancePath, Selection } from "../../types";
import { filterTree } from "./tree-filter";
import { resolveTreeSelection, type TreeSelectionModifiers } from "./tree-selection";

export type SpecificationTreeNode = {
  key: string; title: ReactNode; icon?: ReactNode; children?: SpecificationTreeNode[];
  kind?: string; entityId?: string; documentId?: string; documentType?: string; plane?: string; selection?: Selection;
  instancePath?: InstancePath;
  capabilities?: Array<"ACTIVATE" | "DEACTIVATE" | "DELETE" | "SUPPRESS" | "EDIT" | "DETACH" | "RECONNECT" | "REFRESH" | "CREATE_PART" | "UPDATE_REFERENCES" | "PIN_VERSION" | "FOLLOW_HEAD">; ownerEntityId?: string; role?: "PROFILE" | "CONSTRUCTION";
  definitionDigest?: string;
  suppressed?: boolean; diagnostic?: string; hidden?: boolean;
};

function titleText(title: ReactNode): string {
  if (typeof title === "string" || typeof title === "number") return String(title);
  if (Array.isArray(title)) return title.map(titleText).join(" ");
  if (isValidElement<{ children?: ReactNode }>(title)) return titleText(title.props.children);
  return "";
}

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

function branchKeys(nodes: SpecificationTreeNode[], output = new Set<string>()): Set<string> {
  for (const node of nodes) {
    if (node.children?.length) { output.add(node.key); branchKeys(node.children, output); }
  }
  return output;
}

function initiallyExpandedKeys(nodes: SpecificationTreeNode[], output = new Set<string>()): Set<string> {
  for (const node of nodes) if (node.children?.length && ["PRODUCT", "INSTANCE", "PART"].includes(node.kind ?? "")) {
    output.add(node.key); initiallyExpandedKeys(node.children, output);
  }
  return output;
}

export function SpecificationTree({ nodes, selectedKeys, selectedIdentityKeys, selectionToken, highlightedKey, activeDocumentId, activeInstancePath, onSelect, onActivate, onEdit, onCreatePart, onReferenceMode, onDetach, onReconnect, onRefresh, onHover, onDelete, onToggleConstruction, onToggleVisibility, onToggleSuppression }: {
  nodes: SpecificationTreeNode[]; selectedKeys: readonly string[]; selectedIdentityKeys: readonly string[];
  selectionToken: string; highlightedKey?: string; activeDocumentId?: string; activeInstancePath?: string;
  onSelect: (nodes: SpecificationTreeNode[]) => void; onHover?: (node?: SpecificationTreeNode) => void;
  onActivate?: (node: SpecificationTreeNode) => void;
  onEdit?: (node: SpecificationTreeNode) => void;
  onCreatePart?: (node: SpecificationTreeNode) => void;
  onReferenceMode?: (node: SpecificationTreeNode, mode: "PINNED" | "FOLLOW_HEAD") => void;
  onDetach?: (node: SpecificationTreeNode) => void;
  onReconnect?: (node: SpecificationTreeNode) => void;
  onRefresh?: (node: SpecificationTreeNode) => void;
  onDelete?: (nodes: SpecificationTreeNode[]) => void;
  onToggleConstruction?: (node: SpecificationTreeNode) => void;
  onToggleVisibility?: (node: SpecificationTreeNode) => void;
  onToggleSuppression?: (node: SpecificationTreeNode) => void;
}) {
  const [query, setQuery] = useState("");
  const [focusedKey, setFocusedKey] = useState<string>();
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const knownBranches = useRef(new Set<string>());
  const [contextMenu, setContextMenu] = useState<{ nodeKey: string; selectionSignature: string }>();
  const anchorKey = useRef<string | undefined>(undefined);
  const scrollElement = useRef<HTMLElement>(null);
  const filteredNodes = useMemo(() => filterTree(nodes, query, (node) => titleText(node.title)), [nodes, query]);
  const visible = useMemo(() => flatten(filteredNodes, query.trim() ? branchKeys(filteredNodes) : expanded), [filteredNodes, query, expanded]);
  const nodeIndex = useMemo(() => indexNodes(nodes), [nodes]);
  const visibleKeys = useMemo(() => visible.map((entry) => entry.node.key), [visible]);
  const selected = useMemo(() => new Set(selectedKeys.map((key) => {
    if (visibleKeys.includes(key)) return key;
    return [...(ancestorsOf(nodes, key) ?? [])].reverse().find((candidate) => visibleKeys.includes(candidate)) ?? key;
  })), [nodes, selectedKeys, visibleKeys]);
  useEffect(() => {
    const available = branchKeys(nodes);
    setExpanded((current) => new Set([
      ...[...current].filter((key) => available.has(key)),
      ...[...initiallyExpandedKeys(nodes)].filter((key) => !knownBranches.current.has(key)),
    ]));
    knownBranches.current = available;
  // Node identity changes only when a new document view arrives.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nodes]);
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
  const focusNode = (index: number) => {
    const entry = visible[index];
    if (!entry) return;
    setFocusedKey(entry.node.key);
    virtualizer.scrollToIndex(index);
    requestAnimationFrame(() => {
      const rows = scrollElement.current?.querySelectorAll<HTMLElement>("[data-tree-key]");
      for (const row of rows ?? []) if (row.dataset.treeKey === entry.node.key) row.focus();
    });
  };
  const keyboardSelect = (event: KeyboardEvent, entry: VisibleNode) => {
    if (["Enter", " ", "ArrowDown", "ArrowUp", "ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) event.stopPropagation();
    const index = visible.findIndex((item) => item.node.key === entry.node.key);
    if (event.key === "Enter" || event.key === " ") { event.preventDefault(); selectNode(entry.node, eventModifiers(event)); }
    if (event.key === "ArrowDown" || event.key === "ArrowUp" || event.key === "Home" || event.key === "End") {
      event.preventDefault();
      focusNode(event.key === "Home" ? 0 : event.key === "End" ? visible.length - 1
        : Math.max(0, Math.min(visible.length - 1, index + (event.key === "ArrowDown" ? 1 : -1))));
    }
    if (event.key === "ArrowRight" && entry.hasChildren) {
      event.preventDefault();
      if (entry.depth === 0 || expanded.has(entry.node.key) || query.trim()) focusNode(index + 1);
      else setExpanded((current) => new Set(current).add(entry.node.key));
    }
    if (event.key === "ArrowLeft") {
      event.preventDefault();
      if (entry.depth > 0 && entry.hasChildren && expanded.has(entry.node.key) && !query.trim()) {
        setExpanded((current) => { const next = new Set(current); next.delete(entry.node.key); return next; });
      } else {
        const parent = [...visible.slice(0, index)].reverse().find((item) => item.depth < entry.depth);
        if (parent) focusNode(visible.indexOf(parent));
      }
    }
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
  return <div className="specification-tree-browser">
    <div className="workbench-tree-filter"><Input size="small" allowClear prefix={<SearchOutlined />}
      aria-label="筛选模型结构" placeholder="筛选模型结构…" value={query}
      onChange={(event) => { setQuery(event.target.value); setContextMenu(undefined); onHover?.(); }} /></div>
    {!visible.length && <div className="workbench-tree-empty">没有匹配的对象</div>}
    <nav ref={scrollElement} className="specification-tree specification-tree-virtual" aria-label="Specification tree"
    role="tree" aria-multiselectable="true" onMouseLeave={() => onHover?.()}>
    <div className="specification-tree-virtual-space" style={{ height: virtualizer.getTotalSize() }}>
      {virtualizer.getVirtualItems().map((item) => {
        const entry = visible[item.index];
        const { node, depth, hasChildren } = entry;
        const isExpanded = hasChildren && (depth === 0 || expanded.has(node.key) || Boolean(query.trim()));
        const isSelected = selected.has(node.key);
        const selectedNodes = isSelected
          ? selectedKeys.map((key) => nodeIndex.get(key)).filter((candidate): candidate is SpecificationTreeNode => Boolean(candidate)) : [node];
        const deletable = selectedNodes.filter((candidate) => candidate.capabilities?.includes("DELETE"));
        const rowStyle = { transform: `translateY(${item.start}px)`, paddingLeft: depth * 22,
          "--tree-depth": depth } as CSSProperties;
        const isActiveDocument = Boolean((node.kind === "PART" || node.kind === "PRODUCT" || node.kind === "INSTANCE") &&
          (activeInstancePath ? node.instancePath?.canonical === activeInstancePath
            : activeDocumentId && node.documentId === activeDocumentId && !node.instancePath));
        const row = <div className={`specification-tree-row ${isSelected ? "selected" : ""} ${isActiveDocument ? "active-document" : ""} ${highlightedKey === node.key ? "highlighted" : ""} ${node.suppressed ? "suppressed" : ""} ${node.diagnostic ? `diagnostic-${node.diagnostic.toLowerCase()}` : ""}`}
          role="treeitem" aria-level={depth + 1} aria-expanded={hasChildren ? isExpanded : undefined}
          aria-selected={isSelected} data-tree-key={node.key}
          tabIndex={node.key === (visibleKeys.includes(focusedKey ?? "") ? focusedKey : visibleKeys[0]) ? 0 : -1}
          onFocus={() => setFocusedKey(node.key)} onClick={(event) => { event.stopPropagation(); selectNode(node, eventModifiers(event)); }}
          onDoubleClick={(event) => { event.stopPropagation(); onActivate?.(node); }}
          onContextMenu={(event) => contextSelection(event, node)}
          onMouseEnter={() => onHover?.(node)} onMouseLeave={() => onHover?.()}
          onKeyDown={(event) => keyboardSelect(event, entry)}>
          {depth === 0 ? <span className="specification-tree-root-anchor" /> : hasChildren
            ? <button className={`specification-tree-junction branch ${isExpanded ? "expanded" : "collapsed"}`}
              tabIndex={-1} aria-label={isExpanded ? "折叠" : "展开"}
              onClick={(event) => { event.stopPropagation(); toggle(node.key); }}>
              <svg className="specification-tree-chevron" viewBox="0 0 16 16" aria-hidden="true">
                <path d="m6 3 5 5-5 5" />
              </svg>
            </button> : <span className="specification-tree-junction leaf" />}
          <span className="specification-tree-icon">{node.icon}</span>
          <span className="specification-tree-label">{node.title}</span>
        </div>;
        return <div key={node.key} className={`specification-tree-virtual-row ${depth > 0 ? "nested" : "root"}`} style={rowStyle}>
          {Array.from({ length: depth }, (_, guide) => <i key={guide} className="specification-tree-depth-guide"
            style={{ left: guide * 22 + 13 }} />)}
          <Dropdown trigger={[]} placement="bottomLeft" overlayClassName="specification-tree-context-menu"
            open={contextMenu?.nodeKey === node.key}
            onOpenChange={(open) => { if (!open) setContextMenu(undefined); }}
            menu={{ items: [node.kind === "SKETCH_ENTITY" ? { key: "construction", icon: <SwapOutlined />,
              label: node.role === "CONSTRUCTION" ? "设为轮廓元素" : "设为构造元素",
              disabled: !node.capabilities?.includes("DELETE"),
              onClick: () => { setContextMenu(undefined); onToggleConstruction?.(node); } } : null,
            node.capabilities?.includes("EDIT") ? { key: "edit", icon: <EditOutlined />, label: "编辑",
              onClick: () => { setContextMenu(undefined); onEdit?.(node); } } : null,
            node.capabilities?.includes("CREATE_PART") ? { key: "create-part", icon: <PlusOutlined />, label: "新建零件",
              onClick: () => { setContextMenu(undefined); onCreatePart?.(node); } } : null,
            node.capabilities?.includes("PIN_VERSION") ? { key: "pin-version", icon: <LockOutlined />, label: "固定当前版本",
              onClick: () => { setContextMenu(undefined); onReferenceMode?.(node, "PINNED"); } } : null,
            node.capabilities?.includes("FOLLOW_HEAD") ? { key: "follow-head", icon: <UnlockOutlined />, label: "恢复跟随最新版本",
              onClick: () => { setContextMenu(undefined); onReferenceMode?.(node, "FOLLOW_HEAD"); } } : null,
            node.capabilities?.includes("DETACH") ? { key: "detach", icon: <SwapOutlined />, label: "断开外部关联并冻结",
              onClick: () => { setContextMenu(undefined); onDetach?.(node); } } : null,
            node.capabilities?.includes("RECONNECT") ? { key: "reconnect", icon: <LinkOutlined />, label: "Reconnect 支持元素",
              onClick: () => { setContextMenu(undefined); onReconnect?.(node); } } : null,
            node.capabilities?.includes("REFRESH") ? { key: "refresh", icon: <ReloadOutlined />, label: "重新解析并求解",
              onClick: () => { setContextMenu(undefined); onRefresh?.(node); } } : null,
            { key: "visibility", icon: <EyeInvisibleOutlined />, label: node.hidden ? "显示" : "隐藏",
              onClick: () => { setContextMenu(undefined); onToggleVisibility?.(node); } },
            node.capabilities?.includes("SUPPRESS") ? { key: "suppress", icon: <PauseCircleOutlined />,
              label: node.suppressed ? "解除抑制" : "抑制",
              onClick: () => { setContextMenu(undefined); onToggleSuppression?.(node); } } : null,
            { key: "delete", icon: <DeleteOutlined />, danger: true,
            label: deletable.length > 1 ? `删除 ${deletable.length} 项` : "删除", disabled: deletable.length === 0,
            onClick: () => { setContextMenu(undefined); onDelete?.(deletable); } }] }}>{row}</Dropdown>
        </div>;
      })}
    </div>
  </nav></div>;
}
