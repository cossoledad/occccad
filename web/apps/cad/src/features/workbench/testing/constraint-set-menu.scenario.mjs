import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFileSync, existsSync, statSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url), ts = require("typescript"), React = require("react");
const modules = new Map();
const hooks = { ...React, useMemo: fn => fn(), useEffect() {}, useRef: value => ({ current: value }),
  useSyncExternalStore: (_subscribe, getSnapshot) => getSnapshot(),
  useState: value => [typeof value === "function" ? value() : value, () => {}] };
function load(file) {
  if (file.endsWith(".json")) return JSON.parse(readFileSync(file, "utf8"));
  if (file.endsWith("/cad/command/command-context.tsx")) return { useOptionalCommandRegistry: () => undefined };
  if (modules.has(file)) return modules.get(file).exports;
  const module = { exports: {} }; modules.set(file, module);
  const code = ts.transpileModule(readFileSync(file, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX, esModuleInterop: true }, fileName: file,
  }).outputText;
  const localRequire = createRequire(file);
  new Function("require", "module", "exports", code)(name => {
    if (name === "react") return hooks;
    if (name === "@tanstack/react-virtual") return { useVirtualizer: () => ({ getTotalSize: () => 27,
      getVirtualItems: () => [{ index: 0, key: "group", start: 0, size: 27 }] }) };
    if (!name.startsWith(".")) return localRequire(name);
    const path = resolve(dirname(file), name);
    const target = [path, `${path}.ts`, `${path}.tsx`, `${path}/index.ts`].find(path => existsSync(path) && statSync(path).isFile());
    assert(target, `cannot resolve ${name}`); return load(target);
  }, module, module.exports);
  return module.exports;
}
const base = dirname(fileURLToPath(import.meta.url));
const { SpecificationTree } = load(resolve(base, "../specification-tree.tsx"));
const { ContextMenuIcon } = load(resolve(base, "../../../components/context-menu-icon.tsx"));
const { Dropdown } = require("antd");
function dropdown(tree) {
  if (!tree) return undefined;
  if (Array.isArray(tree)) return tree.map(dropdown).find(Boolean);
  if (tree.type === Dropdown) return tree;
  return dropdown(tree.props?.children);
}
const node = { key: "product/assembly-constraints", kind: "ASSEMBLY_CONSTRAINT_SET", entityId: "assembly-constraints",
  documentId: "product", title: "约束", localVisible: true, children: [{ key: "constraint", kind: "ASSEMBLY_CONSTRAINT", entityId: "mate" }] };
const calls = [];
function menu(target, activeDocumentId = "product", editSession) {
  return dropdown(SpecificationTree({ nodes: [target], selectedKeys: [], selectionToken: "", activeDocumentId, editSession,
    onSelect() {}, onActivate() {}, onOpenDocumentTab() {}, onViewResult() {}, onEdit() {}, onRename() {}, onCreatePart() {},
    onReferenceMode() {}, onDetach() {}, onReconnect() {}, onRefresh() {}, onDelete() {}, onToggleSuppression() {},
    onToggleVisibility: (...args) => calls.push(args) })).props.menu.items.filter(Boolean);
}
const hide = menu(node).find(item => item.key === "visibility");
assert.equal(hide.label, "隐藏");
assert.equal(menu(node).some(item => item.key === "session-visibility"), false, "bulk display must use the formal definition command");
hide.onClick();
assert.deepEqual(calls.at(-1), [node, "DEFINITION", undefined]);
const hidden = { ...node, localVisible: false };
const show = menu(hidden).find(item => item.key === "visibility");
assert.equal(show.label, "显示"); show.onClick();
assert.deepEqual(calls.at(-1), [hidden, "DEFINITION", undefined]);
assert.equal(menu({ ...node, documentId: "nested-product", instancePath: { canonical: "nested" } }).some(item => item.key === "visibility"), false,
  "a referenced Product does not silently become the active edit target");
// One action chooses ownership from the exact editing occurrence, not just the
// shared Part ID. Instance rows always use the host assembly's overlay.
const partSession = { hostDocumentId: "product", editTarget: { documentId: "part", documentType: "PART",
  instancePath: { canonical: "a" } } };
const partBody = { key: "a/body", kind: "BODY", documentId: "part", entityId: "body",
  instancePath: { canonical: "a" }, localVisible: false, definitionVisible: true, visibilityMode: "HIDE" };
for (const [target, session, scope, label, mode] of [
  [partBody, undefined, "OCCURRENCE", "显示", "SHOW"],
  [partBody, partSession, "DEFINITION", "隐藏", undefined],
  [{ ...partBody, instancePath: { canonical: "b" } }, partSession, "OCCURRENCE", "显示", "SHOW"],
  [{ ...partBody, kind: "INSTANCE" }, partSession, "OCCURRENCE", "显示", "SHOW"],
  [{ ...partBody, kind: "INSTANCE", instancePath: { canonical: "b" }, localVisible: true, visibilityMode: "SHOW" },
    partSession, "OCCURRENCE", "隐藏", "HIDE"],
]) {
  const actions = menu(target, "part", session).filter(item => item.key.includes("visibility"));
  assert.equal(actions.length, 1); assert.equal(actions[0].label, label); actions[0].onClick();
  assert.deepEqual(calls.at(-1), [target, scope, mode]);
}
// Verify real menu elements across domains, including the common icon slot
// supplied through configured command declarations rather than inline JSX.
const keys = new Set();
for (const target of [node,
  { ...node, kind: "INSTANCE", instancePath: { canonical: "instance" }, visibilityMode: "HIDE", capabilities: ["PIN_VERSION", "FOLLOW_HEAD", "DETACH", "DELETE"] },
  { ...node, kind: "BODY", capabilities: ["DELETE"] },
  { ...node, kind: "PAD", capabilities: ["EDIT", "SUPPRESS", "DELETE"] },
  { ...node, kind: "SKETCH_ENTITY", capabilities: ["DELETE"] },
  { ...node, kind: "ASSEMBLY_CONSTRAINT", capabilities: ["EDIT", "RECONNECT", "REFRESH", "SUPPRESS", "DELETE"] },
  { ...node, kind: "PRODUCT", capabilities: ["CREATE_PART"] },
  { ...node, kind: "PUBLICATION", sourceDocumentId: "source" },
  { ...node, kind: "PARAMETER", parameterAlias: "length_1" },
]) {
  for (const item of menu(target)) {
    keys.add(item.key);
    assert(React.isValidElement(item.icon), `menu ${item.key} lost its icon slot`);
    assert.equal(item.icon.type, ContextMenuIcon, `menu ${item.key} bypassed the common icon slot`);
  }
}
for (const key of ["open-document-tab", "open-source", "activate-body", "construction", "view-result", "edit", "rename", "create-part",
  "pin-version", "follow-head", "detach", "reconnect", "refresh", "copy-alias", "suppress", "delete",
  "visibility"]) {
  assert(keys.has(key), `menu coverage omitted ${key}`);
}
console.log("Constraint root menu uses configured labels, stable target identity and formal display scope.");
