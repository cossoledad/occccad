import "./styles.css";
import { api } from "./api";
import { CadView } from "./cad-view";
import type {
  DocumentSummary, DocumentView, Feature, PlaneName, RectangleDraft, Selection, Vec3,
} from "./types";

const element = <T extends HTMLElement>(selector: string): T =>
  document.querySelector<T>(selector)!;

const state: {
  documents: DocumentSummary[];
  tabs: string[];
  active?: DocumentView;
  selection: Selection;
  sketchPlane?: PlaneName;
  busy: boolean;
} = { documents: [], tabs: [], selection: null, busy: false };

const cad = new CadView(element("#viewport"), {
  selectionChanged: (selection) => {
    state.selection = selection;
    renderTree();
    renderProperties();
    renderToolbar();
  },
  rectangleCreated: (draft) => void createRectangle(draft),
  instanceMoved: (instanceId, translation) => void moveInstance(instanceId, translation),
});

function setStatus(message: string, error = false): void {
  const target = element<HTMLSpanElement>("#status");
  target.textContent = message;
  target.classList.toggle("error", error);
}

async function withBusy<T>(label: string, operation: () => Promise<T>): Promise<T | undefined> {
  if (state.busy) return undefined;
  state.busy = true;
  setStatus(label);
  renderToolbar();
  try {
    return await operation();
  } catch (error) {
    setStatus(error instanceof Error ? error.message : "操作失败", true);
    return undefined;
  } finally {
    state.busy = false;
    renderToolbar();
  }
}

async function refreshDocuments(): Promise<void> {
  const documents = await withBusy("刷新文档库…", () => api.listDocuments());
  if (!documents) return;
  state.documents = documents;
  renderDocuments();
  setStatus("就绪");
}

async function openDocument(documentId: string): Promise<void> {
  const view = await withBusy("打开文档…", () => api.getDocument(documentId));
  if (!view) return;
  if (!state.tabs.includes(documentId)) state.tabs.push(documentId);
  activateView(view);
}

function activateView(view: DocumentView): void {
  state.active = view;
  state.selection = null;
  state.sketchPlane = undefined;
  cad.endSketch();
  cad.render(view);
  const summaryIndex = state.documents.findIndex((item) => item.id === view.document.id);
  if (summaryIndex >= 0) state.documents[summaryIndex] = view.document;
  else state.documents.unshift(view.document);
  renderAll();
  setStatus(`${view.document.name} · ${view.document.type}`);
}

async function reloadActive(selection?: Selection, preserveSketch = false): Promise<void> {
  if (!state.active) return;
  const view = await api.getDocument(state.active.document.id);
  const plane = state.sketchPlane;
  activateView(view);
  if (preserveSketch && plane) {
    state.sketchPlane = plane;
    cad.beginSketch(plane);
  }
  if (selection) cad.select(selection);
  await refreshDocuments();
}

function renderAll(): void {
  renderTabs();
  renderDocuments();
  renderTree();
  renderProperties();
  renderToolbar();
  renderHistory();
}

function renderTabs(): void {
  const host = element("#tabs");
  host.replaceChildren();
  if (state.tabs.length === 0) {
    const empty = document.createElement("div");
    empty.className = "tabs-empty";
    empty.textContent = "打开或新建文档开始建模";
    host.appendChild(empty);
    return;
  }
  for (const id of state.tabs) {
    const summary = state.documents.find((item) => item.id === id);
    if (!summary) continue;
    const tab = document.createElement("button");
    tab.className = `tab${state.active?.document.id === id ? " active" : ""}`;
    const type = document.createElement("span");
    type.className = "type";
    type.textContent = summary.type === "PART" ? "PRT" : "ASM";
    const name = document.createElement("span");
    name.className = "name";
    name.textContent = summary.name;
    const close = document.createElement("span");
    close.className = "close";
    close.textContent = "×";
    close.addEventListener("click", (event) => {
      event.stopPropagation();
      closeTab(id);
    });
    tab.append(type, name, close);
    tab.addEventListener("click", () => void openDocument(id));
    host.appendChild(tab);
  }
}

function closeTab(id: string): void {
  const index = state.tabs.indexOf(id);
  state.tabs = state.tabs.filter((item) => item !== id);
  if (state.active?.document.id === id) {
    state.active = undefined;
    const next = state.tabs[Math.min(index, state.tabs.length - 1)];
    if (next) void openDocument(next);
    else {
      cad.clear();
      renderAll();
    }
  } else renderTabs();
}

function renderDocuments(): void {
  const host = element("#documents");
  host.replaceChildren();
  host.className = "documents";
  if (state.documents.length === 0) {
    host.classList.add("empty");
    host.textContent = "还没有文档";
    return;
  }
  for (const documentInfo of state.documents) {
    const row = document.createElement("div");
    row.className = "document-row";
    const icon = document.createElement("span");
    icon.className = "icon";
    icon.textContent = documentInfo.type === "PART" ? "◇" : "▦";
    const name = document.createElement("span");
    name.textContent = documentInfo.name;
    const secondary = document.createElement("span");
    secondary.className = "secondary";
    secondary.textContent = documentInfo.type;
    row.append(icon, name, secondary);
    row.addEventListener("dblclick", () => void openDocument(documentInfo.id));
    row.addEventListener("click", () => void openDocument(documentInfo.id));
    host.appendChild(row);
  }
}

function treeRow(label: string, iconText: string, selection?: Exclude<Selection, null>, detail?: string): HTMLElement {
  const row = document.createElement("div");
  row.className = "tree-row";
  if (selection && state.selection?.kind === selection.kind && state.selection.id === selection.id) {
    row.classList.add("selected");
  }
  const icon = document.createElement("span");
  icon.className = "icon";
  icon.textContent = iconText;
  const text = document.createElement("span");
  text.textContent = label;
  row.append(icon, text);
  if (detail) {
    const secondary = document.createElement("span");
    secondary.className = "secondary";
    secondary.textContent = detail;
    row.appendChild(secondary);
  }
  if (selection) row.addEventListener("click", () => cad.select(selection));
  return row;
}

function renderTree(): void {
  const host = element("#tree");
  host.replaceChildren();
  host.className = "tree";
  const view = state.active;
  if (!view) {
    host.classList.add("empty");
    host.textContent = "没有打开的文档";
    return;
  }
  host.appendChild(treeRow(view.document.name, view.document.type === "PART" ? "◇" : "▦", undefined, view.document.type));
  const children = document.createElement("div");
  children.className = "tree-children";
  if (view.part) {
    for (const datum of view.datumPlanes ?? []) {
      children.appendChild(treeRow(datum.name, "▱", { kind: "plane", id: datum.id, plane: datum.plane }));
    }
    for (const feature of view.part.features) {
      const isSketch = feature.type.toUpperCase().includes("SKETCH");
      children.appendChild(treeRow(feature.name ?? (isSketch ? "Rectangle Sketch" : "Pad"),
        isSketch ? "⌑" : "↥", { kind: isSketch ? "sketch" : "pad", id: feature.id },
        isSketch ? feature.plane ?? "XY" : `${feature.length ?? 0} mm`));
    }
    if (view.artifact) children.appendChild(treeRow("Body", "⬡", { kind: "solid", id: "body-1" }, "SOLID"));
  } else {
    for (const instance of view.product?.instances ?? []) {
      const mode = instance.referenceMode === "PINNED" ? "PINNED" : "LIVE";
      children.appendChild(treeRow(instance.name, "⊹", { kind: "instance", id: instance.id }, mode));
    }
  }
  host.appendChild(children);
}

function selectedFeature(): Feature | undefined {
  if (state.selection?.kind !== "sketch" && state.selection?.kind !== "pad") return undefined;
  return state.active?.part?.features.find((feature) => feature.id === state.selection?.id);
}

function selectedInstance() {
  if (state.selection?.kind !== "instance") return undefined;
  return state.active?.product?.instances.find((item) => item.id === state.selection?.id);
}

function renderProperties(): void {
  const host = element("#properties");
  host.replaceChildren();
  host.className = "properties";
  if (!state.active) {
    host.classList.add("empty");
    host.textContent = "选择对象查看属性";
    return;
  }
  const title = document.createElement("h3");
  const rows: [string, string][] = [];
  const feature = selectedFeature();
  if (state.selection?.kind === "plane") {
    title.textContent = `${state.selection.plane} Plane`;
    rows.push(["类型", "基准面"], ["标识", state.selection.id], ["状态", "可用于草图"]);
  } else if (feature) {
    const isSketch = feature.type.toUpperCase().includes("SKETCH");
    title.textContent = feature.name ?? (isSketch ? "Rectangle Sketch" : "Pad");
    if (isSketch) {
      const rectangle = feature.rectangle ?? { origin: feature.origin ?? [0, 0], width: feature.width ?? 0, height: feature.height ?? 0 };
      rows.push(["类型", "矩形草图"], ["基准面", feature.plane ?? "XY"],
        ["原点", `${rectangle.origin[0].toFixed(2)}, ${rectangle.origin[1].toFixed(2)}`],
        ["宽度", `${rectangle.width.toFixed(2)} mm`], ["高度", `${rectangle.height.toFixed(2)} mm`]);
    } else rows.push(["类型", "拉伸"], ["草图", feature.profile ?? "—"], ["长度", `${feature.length ?? 0} mm`]);
  } else if (state.selection?.kind === "instance") {
    const instance = selectedInstance();
    title.textContent = instance?.name ?? "Instance";
    rows.push(["类型", "文档实例"], ["引用", instance?.documentId ?? "—"],
      ["引用策略", instance?.referenceMode === "PINNED" ? "固定 Version" : "跟随 Head"],
      ["解析版本", instance?.resolvedVersionId?.slice(0, 13) ?? instance?.versionId.slice(0, 13) ?? "—"],
      ["Head 变化", instance?.headChanged ? "已自动使用最新版本" : "无"],
      ["位置", instance?.translation.map((value) => value.toFixed(2)).join(", ") ?? "—"]);
  } else if (state.active.artifact) {
    title.textContent = "Body";
    rows.push(["GeometryId", state.active.artifact.geometryId],
      ["体积", `${state.active.artifact.volume.toFixed(2)} mm³`],
      ["拓扑", `${state.active.artifact.topology.faces} F / ${state.active.artifact.topology.edges} E / ${state.active.artifact.topology.vertices} V`]);
  } else {
    title.textContent = state.active.document.name;
    rows.push(["类型", state.active.document.type], ["单位", "mm"]);
  }
  host.appendChild(title);
  const list = document.createElement("dl");
  for (const [key, value] of rows) {
    const row = document.createElement("div");
    const term = document.createElement("dt"); term.textContent = key;
    const description = document.createElement("dd"); description.textContent = value;
    row.append(term, description); list.appendChild(row);
  }
  host.appendChild(list);
}

function renderHistory(): void {
  const host = element("#history-info");
  host.replaceChildren();
  const view = state.active;
  const rows: [string, string][] = view ? [
    ["Version", view.document.versionId.slice(0, 13)],
    ["Undo", view.document.canUndo ? "可用" : "不可用"],
    ["Redo", view.document.canRedo ? "可用" : "不可用"],
  ] : [["Version", "—"]];
  for (const [key, value] of rows) {
    const row = document.createElement("div");
    const term = document.createElement("dt"); term.textContent = key;
    const description = document.createElement("dd"); description.textContent = value;
    row.append(term, description); host.appendChild(row);
  }
}

function renderToolbar(): void {
  const view = state.active;
  const isPart = view?.document.type === "PART";
  const isProduct = view?.document.type === "PRODUCT";
  const inSketch = Boolean(state.sketchPlane);
  element<HTMLButtonElement>("#start-sketch").disabled = state.busy || !isPart || state.selection?.kind !== "plane" || inSketch;
  element("#exit-sketch").classList.toggle("hidden", !inSketch);
  element("#start-sketch").classList.toggle("hidden", inSketch);
  const hasPad = Boolean(view?.part?.features.some((feature) => feature.type.toUpperCase() === "PAD"));
  element<HTMLButtonElement>("#pad-sketch").disabled = state.busy || !isPart || state.selection?.kind !== "sketch" || hasPad || inSketch;
  element<HTMLButtonElement>("#insert-document").disabled = state.busy || !isProduct;
  const referenceButton = element<HTMLButtonElement>("#reference-mode");
  const instance = selectedInstance();
  referenceButton.disabled = state.busy || !isProduct || !instance;
  referenceButton.querySelector("span")!.textContent = instance?.referenceMode === "PINNED"
    ? "跟随最新" : "固定版本";
  element<HTMLButtonElement>("#undo").disabled = state.busy || !view?.document.canUndo;
  element<HTMLButtonElement>("#redo").disabled = state.busy || !view?.document.canRedo;
  const mode = element("#mode-label");
  mode.textContent = inSketch ? `${state.sketchPlane} 草图模式 · 按住拖动绘制矩形` : "选择模式";
  mode.classList.toggle("sketch", inSketch);
  element("#hint").textContent = inSketch
    ? "在视图区按住鼠标左键拖动绘制矩形，完成后点击“退出草图”"
    : isProduct ? "选择实例后使用三轴手柄拖动" : "选择一个基准面，然后点击“新建草图”";
}

function showDocumentDialog(type: "PART" | "PRODUCT"): void {
  element<HTMLInputElement>("#document-type").value = type;
  const base = type === "PART" ? "Part" : "Product";
  let index = state.documents.filter((item) => item.type === type).length + 1;
  while (state.documents.some((item) => item.name === `${base} ${index}`)) index++;
  element<HTMLInputElement>("#document-name").value = `${base} ${index}`;
  element<HTMLDialogElement>("#document-dialog").showModal();
  element<HTMLInputElement>("#document-name").select();
}

async function createDocument(): Promise<void> {
  const type = element<HTMLInputElement>("#document-type").value as "PART" | "PRODUCT";
  const name = element<HTMLInputElement>("#document-name").value.trim();
  if (!name) return;
  const view = await withBusy(`创建 ${type}…`, () => api.createDocument(type, name));
  if (!view) return;
  element<HTMLDialogElement>("#document-dialog").close();
  state.documents.unshift(view.document);
  state.tabs.push(view.document.id);
  activateView(view);
}

function startSketch(): void {
  if (state.selection?.kind !== "plane") return;
  state.sketchPlane = state.selection.plane;
  cad.beginSketch(state.sketchPlane);
  renderToolbar();
}

function exitSketch(): void {
  state.sketchPlane = undefined;
  cad.endSketch();
  state.selection = null;
  renderAll();
}

async function createRectangle(draft: RectangleDraft): Promise<void> {
  if (!state.active) return;
  const previousIDs = new Set(state.active.part?.features.map((feature) => feature.id));
  const result = await withBusy("保存矩形草图…", () => api.createSketch(
    state.active!.document.id, draft.plane, draft.origin, draft.width, draft.height));
  if (!result) return;
  state.active = result;
  const feature = result.part?.features.find((item) => !previousIDs.has(item.id));
  const plane = state.sketchPlane;
  cad.render(result);
  if (plane) cad.beginSketch(plane);
  if (feature) cad.select({ kind: "sketch", id: feature.id });
  renderAll();
  await refreshDocuments();
  setStatus(`已创建 ${draft.width.toFixed(2)} × ${draft.height.toFixed(2)} mm 矩形草图`);
}

async function padSketch(): Promise<void> {
  if (state.selection?.kind !== "sketch") return;
  element<HTMLDialogElement>("#pad-dialog").showModal();
  element<HTMLInputElement>("#pad-length").select();
}

async function confirmPad(): Promise<void> {
  if (!state.active || state.selection?.kind !== "sketch") return;
  const sketchID = state.selection.id;
  const length = Number(element<HTMLInputElement>("#pad-length").value);
  if (!(length > 0)) return;
  const result = await withBusy("OCCT 正在拉伸草图…", () => api.pad(state.active!.document.id, sketchID, length));
  if (!result) return;
  element<HTMLDialogElement>("#pad-dialog").close();
  activateView(result);
  const pad = [...(result.part?.features ?? [])].reverse()
    .find((feature) => feature.type.toUpperCase() === "PAD");
  if (pad) cad.select({ kind: "pad", id: pad.id });
  await refreshDocuments();
  setStatus(`拉伸完成 · ${length} mm`);
}

function showInsertDialog(): void {
  if (!state.active || state.active.document.type !== "PRODUCT") return;
  const select = element<HTMLSelectElement>("#insert-document-select");
  select.replaceChildren();
  for (const documentInfo of state.documents.filter((item) => item.id !== state.active?.document.id)) {
    const option = document.createElement("option");
    option.value = documentInfo.id;
    option.textContent = `${documentInfo.name} (${documentInfo.type})`;
    select.appendChild(option);
  }
  element<HTMLInputElement>("#instance-name").value = "";
  element<HTMLDialogElement>("#insert-dialog").showModal();
}

async function confirmInsert(): Promise<void> {
  if (!state.active) return;
  const referencedID = element<HTMLSelectElement>("#insert-document-select").value;
  if (!referencedID) return;
  const reference = state.documents.find((item) => item.id === referencedID);
  const name = element<HTMLInputElement>("#instance-name").value.trim() || `${reference?.name ?? "Instance"}-${(state.active.product?.instances.length ?? 0) + 1}`;
  const result = await withBusy("插入文档实例…", () => api.insert(state.active!.document.id, referencedID, name));
  if (!result) return;
  element<HTMLDialogElement>("#insert-dialog").close();
  activateView(result);
  const instances = result.product?.instances ?? [];
  const instance = instances[instances.length - 1];
  if (instance) cad.select({ kind: "instance", id: instance.id });
  await refreshDocuments();
  setStatus(`已插入 ${name}`);
}

async function moveInstance(instanceId: string, translation: Vec3): Promise<void> {
  if (!state.active || state.active.document.type !== "PRODUCT") return;
  const result = await withBusy("保存实例位置…", () => api.move(state.active!.document.id, instanceId, translation));
  if (!result) {
    await reloadActive();
    return;
  }
  activateView(result);
  cad.select({ kind: "instance", id: instanceId });
  await refreshDocuments();
  setStatus(`实例位置：${translation.map((value) => value.toFixed(1)).join(", ")}`);
}

async function toggleReferenceMode(): Promise<void> {
  if (!state.active) return;
  const instance = selectedInstance();
  if (!instance) return;
  const mode = instance.referenceMode === "PINNED" ? "FOLLOW_HEAD" : "PINNED";
  const result = await withBusy(mode === "PINNED" ? "固定引用版本…" : "切换为跟随最新…", () =>
    api.setReferenceMode(state.active!.document.id, instance.id, mode));
  if (!result) return;
  activateView(result);
  cad.select({ kind: "instance", id: instance.id });
  await refreshDocuments();
  setStatus(mode === "PINNED" ? "实例已固定到当前 Version" : "实例将跟随引用文档 Head");
}

async function history(direction: "undo" | "redo"): Promise<void> {
  if (!state.active) return;
  const result = await withBusy(direction === "undo" ? "Undo…" : "Redo…", () =>
    direction === "undo" ? api.undo(state.active!.document.id) : api.redo(state.active!.document.id));
  if (!result) return;
  activateView(result);
  await refreshDocuments();
  setStatus(direction === "undo" ? "已撤销上一条命令" : "已重做下一条命令");
}

async function checkHealth(): Promise<void> {
  const health = element("#health");
  try {
    const result = await api.health();
    health.className = "health online";
    health.innerHTML = `<i></i>服务在线 · OCCT ${result.occtVersion}`;
  } catch {
    health.className = "health offline";
    health.innerHTML = "<i></i>服务未连接";
  }
}

element("#new-part").addEventListener("click", () => showDocumentDialog("PART"));
element("#new-product").addEventListener("click", () => showDocumentDialog("PRODUCT"));
element("#confirm-document").addEventListener("click", (event) => { event.preventDefault(); void createDocument(); });
element("#start-sketch").addEventListener("click", startSketch);
element("#exit-sketch").addEventListener("click", exitSketch);
element("#pad-sketch").addEventListener("click", () => void padSketch());
element("#confirm-pad").addEventListener("click", (event) => { event.preventDefault(); void confirmPad(); });
element("#insert-document").addEventListener("click", showInsertDialog);
element("#reference-mode").addEventListener("click", () => void toggleReferenceMode());
element("#confirm-insert").addEventListener("click", (event) => { event.preventDefault(); void confirmInsert(); });
element("#undo").addEventListener("click", () => void history("undo"));
element("#redo").addEventListener("click", () => void history("redo"));
element("#refresh-documents").addEventListener("click", () => {
  if (state.active) void reloadActive(state.selection, Boolean(state.sketchPlane));
  else void refreshDocuments();
});
element("#fit-view").addEventListener("click", () => cad.fit());
for (const button of document.querySelectorAll<HTMLButtonElement>("[data-view]")) {
  button.addEventListener("click", () =>
    cad.setStandardView(button.dataset.view as "TOP" | "FRONT" | "RIGHT" | "ISO"));
}
window.addEventListener("keydown", (event) => {
  const target = event.target;
  const editing = target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement || (target instanceof HTMLElement && target.isContentEditable);
  if (editing) return;
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "z") {
    event.preventDefault();
    void history(event.shiftKey ? "redo" : "undo");
  } else if (!event.ctrlKey && !event.metaKey && !event.altKey && event.key.toLowerCase() === "f") {
    event.preventDefault();
    cad.fit();
  }
});

void (async () => {
  await checkHealth();
  await refreshDocuments();
  const firstPart = state.documents.find((documentInfo) => documentInfo.type === "PART");
  if (firstPart) await openDocument(firstPart.id);
  else renderAll();
})();
