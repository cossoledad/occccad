import "./styles.css";
import { api } from "./api";
import { CadView } from "./cad-view";
import { documentMetrics, flattenFolderTree, pageLabel, relativeDate, type LibraryScope } from "./document-center";
import type {
  DocumentSummary, DocumentView, Feature, FolderSummary, HistoryEntry, PlaneName, RectangleDraft, Selection, Team, User, Vec3,
} from "./types";

const element = <T extends HTMLElement>(selector: string): T =>
  document.querySelector<T>(selector)!;

function loadTabs(): string[] {
  try {
    const value = JSON.parse(sessionStorage.getItem("occccad.tabs") ?? "[]") as unknown;
    return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
  } catch {
    return [];
  }
}

const state: {
  documents: DocumentSummary[];
  catalog: DocumentSummary[];
  trashCount: number;
  recentCount: number;
  folders: FolderSummary[];
  breadcrumbs: FolderSummary[];
  currentFolderId?: string;
  documentTotal: number;
  documentOffset: number;
  documentLimit: number;
  tabs: string[];
  libraryScope: LibraryScope;
  selectedDocumentId?: string;
  active?: DocumentView;
  selection: Selection;
  sketchPlane?: PlaneName;
  sketchTool: "SELECT" | "RECTANGLE";
  history: HistoryEntry[];
  busy: boolean;
  users: User[];
  teams: Team[];
  currentUser?: User;
} = {
  documents: [], catalog: [], trashCount: 0, recentCount: 0, folders: [], breadcrumbs: [],
  documentTotal: 0, documentOffset: 0, documentLimit: 25,
  tabs: loadTabs(),
  libraryScope: "active", selection: null, sketchTool: "SELECT", history: [], busy: false,
  users: [], teams: [],
};

const canEdit = (permission?: string): boolean => permission === "OWNER" || permission === "EDITOR";

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
  if (error) {
    const toast = element("#toast");
    toast.textContent = message;
    toast.classList.remove("hidden");
    toast.classList.add("error");
    window.setTimeout(() => toast.classList.add("hidden"), 5000);
  }
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

async function initializeIdentity(): Promise<void> {
  const [session, users, teams] = await Promise.all([api.session(), api.listUsers(), api.listTeams()]);
  state.currentUser = session.user;
  state.users = users;
  state.teams = teams;
  element("#avatar").textContent = session.user.displayName.slice(0, 1).toUpperCase();
  element("#current-user-name").textContent = session.user.displayName;
  element("#current-user-role").textContent = session.user.platformRole === "ADMIN" ? "管理员" : "成员";
  element("#admin-button").classList.toggle("hidden", session.user.platformRole !== "ADMIN");
  if (session.user.mustChangePassword) element<HTMLDialogElement>("#password-dialog").showModal();
}

function showAuthentication(): void {
  element("#app").classList.add("hidden");
  element("#auth-screen").classList.remove("hidden");
}

function showApplication(): void {
  element("#auth-screen").classList.add("hidden");
  element("#app").classList.remove("hidden");
}

async function login(event: SubmitEvent): Promise<void> {
  event.preventDefault();
  const error = element("#auth-error"); error.classList.add("hidden");
  try {
    await api.login(element<HTMLInputElement>("#login-email").value, element<HTMLInputElement>("#login-password").value);
    await startApplication();
  } catch (reason) {
    error.textContent = reason instanceof Error ? reason.message : "登录失败"; error.classList.remove("hidden");
  }
}

async function register(event: SubmitEvent): Promise<void> {
  event.preventDefault();
  const message = element("#register-message"); message.classList.add("hidden");
  try {
    await api.register(element<HTMLInputElement>("#register-email").value,
      element<HTMLInputElement>("#register-name").value, element<HTMLInputElement>("#register-password").value);
    message.textContent = "申请已提交。管理员审批后即可登录。";
    message.className = "auth-error success";
  } catch (reason) {
    message.textContent = reason instanceof Error ? reason.message : "注册失败"; message.className = "auth-error";
  }
}

async function logout(): Promise<void> {
  try { await api.logout(); } finally {
    state.active = undefined; state.currentUser = undefined; cad.clear(); showAuthentication();
  }
}

async function changePassword(event: Event): Promise<void> {
  event.preventDefault();
  const changed = await withBusy("修改密码…", () => api.changePassword(
    element<HTMLInputElement>("#current-password").value, element<HTMLInputElement>("#new-password").value));
  if (!changed) return;
  element<HTMLDialogElement>("#password-dialog").close();
  if (state.currentUser) state.currentUser.mustChangePassword = false;
  setStatus("密码已更新");
}

async function showAdmin(): Promise<void> {
  element<HTMLDialogElement>("#admin-dialog").showModal();
  await refreshAdmin();
}

async function refreshAdmin(): Promise<void> {
  const [users, stats] = await Promise.all([api.adminUsers(
    element<HTMLInputElement>("#admin-search").value, element<HTMLSelectElement>("#admin-status").value), api.adminStats()]);
  element("#admin-stats").innerHTML = `<article><b>${stats.users}</b><span>全部账号</span></article><article><b>${stats.pending}</b><span>待审批</span></article><article><b>${stats.activeSessions}</b><span>活跃会话</span></article><article><b>${stats.documents}</b><span>设计文档</span></article>`;
  const host = element("#admin-users"); host.replaceChildren();
  for (const user of users) {
    const row = document.createElement("div"); row.className = "admin-user-row";
    const identity = document.createElement("span"); identity.innerHTML = `<b></b><small></small>`;
    identity.querySelector("b")!.textContent = user.displayName; identity.querySelector("small")!.textContent = user.email;
    const role = document.createElement("span"); role.className = "admin-pill"; role.textContent = user.platformRole === "ADMIN" ? "管理员" : "成员";
    const status = document.createElement("span"); status.className = `admin-pill ${user.status.toLowerCase()}`;
    status.textContent = user.status === "PENDING" ? "待审批" : user.status === "ACTIVE" ? "已启用" : "已禁用";
    const actions = document.createElement("span"); actions.className = "admin-actions";
    const edit = document.createElement("button"); edit.type = "button"; edit.textContent = user.status === "PENDING" ? "审批" : "编辑";
    edit.addEventListener("click", () => showAdminUser(user));
    const reset = document.createElement("button"); reset.type = "button"; reset.textContent = "重置密码";
    reset.addEventListener("click", () => showAdminPasswordReset(user));
    actions.append(edit); if (user.id !== state.currentUser?.id) actions.append(reset);
    row.append(identity, role, status, actions); host.append(row);
  }
}

function showAdminUser(user?: User): void {
  element("#admin-user-title").textContent = user ? (user.status === "PENDING" ? "审批账号" : "编辑账号") : "添加账号";
  element<HTMLInputElement>("#admin-user-id").value = user?.id ?? "";
  element<HTMLInputElement>("#admin-user-name").value = user?.displayName ?? "";
  element<HTMLInputElement>("#admin-user-email").value = user?.email ?? "";
  element<HTMLInputElement>("#admin-user-email").disabled = Boolean(user);
  element<HTMLInputElement>("#admin-user-password").value = "";
  element("#admin-password-label").classList.toggle("hidden", Boolean(user));
  element<HTMLSelectElement>("#admin-user-role").value = user?.platformRole ?? "MEMBER";
  element<HTMLSelectElement>("#admin-user-status").value = user?.status === "PENDING" ? "ACTIVE" : user?.status ?? "ACTIVE";
  element<HTMLDialogElement>("#admin-user-dialog").showModal();
}

async function saveAdminUser(event: Event): Promise<void> {
  event.preventDefault();
  const id = element<HTMLInputElement>("#admin-user-id").value;
  const common = { displayName: element<HTMLInputElement>("#admin-user-name").value,
    platformRole: element<HTMLSelectElement>("#admin-user-role").value,
    status: element<HTMLSelectElement>("#admin-user-status").value };
  const saved = await withBusy("保存账号…", () => id ? api.adminUpdateUser(id, common) : api.adminCreateUser({
    ...common, email: element<HTMLInputElement>("#admin-user-email").value,
    password: element<HTMLInputElement>("#admin-user-password").value,
  }));
  if (!saved) return;
  element<HTMLDialogElement>("#admin-user-dialog").close(); await refreshAdmin(); setStatus("账号已保存");
}

function showAdminPasswordReset(user: User): void {
  element<HTMLInputElement>("#admin-reset-id").value = user.id;
  element("#admin-reset-name").textContent = `账号：${user.displayName} · ${user.email}`;
  element<HTMLInputElement>("#admin-reset-password").value = "";
  element<HTMLDialogElement>("#admin-reset-dialog").showModal();
}

async function resetAdminPassword(event: Event): Promise<void> {
  event.preventDefault();
  const result = await withBusy("重置密码…", () => api.adminResetPassword(
    element<HTMLInputElement>("#admin-reset-id").value, element<HTMLInputElement>("#admin-reset-password").value));
  if (!result) return;
  element<HTMLDialogElement>("#admin-reset-dialog").close();
  setStatus("临时密码已设置，账号下次登录必须修改密码");
}

async function showShareDialog(type: "documents" | "folders", id: string, name: string): Promise<void> {
  const result = await withBusy("读取共享设置…", async () => {
    const [grants, users, teams] = await Promise.all([api.listShares(type, id), api.listUsers(), api.listTeams()]);
    return { grants, users, teams };
  });
  if (!result) return;
  state.users = result.users; state.teams = result.teams;
  element<HTMLInputElement>("#share-resource-type").value = type;
  element<HTMLInputElement>("#share-resource-id").value = id;
  element("#share-resource-name").textContent = `${type === "folders" ? "文件夹" : "文档"}：${name}`;
  renderShareGrants(result.grants);
  renderShareSubjects();
  element<HTMLDialogElement>("#share-dialog").showModal();
}

function renderShareSubjects(): void {
  const select = element<HTMLSelectElement>("#share-subject"); select.replaceChildren();
  for (const user of state.users.filter((item) => item.id !== state.currentUser?.id)) {
    const option = document.createElement("option"); option.value = `USER:${user.id}`;
    option.textContent = `${user.displayName} · 用户`; select.appendChild(option);
  }
  for (const team of state.teams) {
    const option = document.createElement("option"); option.value = `TEAM:${team.id}`;
    option.textContent = `${team.name} · 团队 (${team.memberCount})`; select.appendChild(option);
  }
}

function renderShareGrants(grants: Awaited<ReturnType<typeof api.listShares>>): void {
  const host = element("#share-grants"); host.replaceChildren();
  if (grants.length === 0) {
    const empty = document.createElement("p"); empty.className = "share-empty";
    empty.textContent = "尚未直接共享；所有者仍拥有完整权限。"; host.appendChild(empty); return;
  }
  for (const grant of grants) {
    const row = document.createElement("div"); row.className = "share-grant";
    const identity = document.createElement("span");
    const name = document.createElement("strong"); name.textContent = grant.subjectName;
    const kind = document.createElement("small"); kind.textContent = grant.subjectType === "TEAM" ? "团队" : "用户";
    identity.append(name, kind);
    const role = document.createElement("b"); role.textContent = grant.role === "EDITOR" ? "可编辑" : "可查看";
    const remove = document.createElement("button"); remove.type = "button"; remove.textContent = "移除";
    remove.addEventListener("click", () => void removeShare(grant.id));
    row.append(identity, role, remove); host.appendChild(row);
  }
}

async function confirmShare(): Promise<void> {
  const type = element<HTMLInputElement>("#share-resource-type").value as "documents" | "folders";
  const id = element<HTMLInputElement>("#share-resource-id").value;
  const [subjectType, subjectId] = element<HTMLSelectElement>("#share-subject").value.split(":") as ["USER" | "TEAM", string];
  const role = element<HTMLSelectElement>("#share-role").value as "VIEWER" | "EDITOR";
  if (!id || !subjectId) return;
  const saved = await withBusy("保存共享权限…", () => api.share(type, id, subjectType, subjectId, role));
  if (!saved) return;
  renderShareGrants(await api.listShares(type, id));
  setStatus("共享权限已更新");
}

async function removeShare(grantId: string): Promise<void> {
  const type = element<HTMLInputElement>("#share-resource-type").value as "documents" | "folders";
  const id = element<HTMLInputElement>("#share-resource-id").value;
  const removed = await withBusy("移除共享权限…", async () => { await api.unshare(type, id, grantId); return true; });
  if (!removed) return;
  renderShareGrants(await api.listShares(type, id));
  setStatus("共享权限已移除");
}

async function refreshDocuments(): Promise<void> {
  const scope = state.libraryScope === "trash" ? "trash" : "active";
  const type = state.libraryScope === "parts" ? "PART" : state.libraryScope === "products" ? "PRODUCT" : "";
  const recent = state.libraryScope === "recent";
  const shared = state.libraryScope === "shared";
  const query = element<HTMLInputElement>("#document-search").value.trim();
  const selectedType = element<HTMLSelectElement>("#document-type-filter").value || type;
  const sort = recent ? "recent" : element<HTMLSelectElement>("#document-sort").value as "updated" | "name" | "created";
  const allFolders = recent || shared || state.libraryScope === "trash" || query !== "";
  const result = await withBusy("刷新文档库…", async () => {
    const [catalog, active, trash, recentDocuments, folders, breadcrumbs] = await Promise.all([
      api.listDocuments({ scope, query, type: selectedType, folderId: state.currentFolderId,
        recent, shared, allFolders, sort, limit: state.documentLimit, offset: state.documentOffset }),
      api.listDocuments({ scope: "active", allFolders: true, limit: 200 }),
      api.listDocuments({ scope: "trash", allFolders: true, limit: 1 }),
      api.listDocuments({ scope: "active", recent: true, allFolders: true, sort: "recent", limit: 1 }),
      shared ? api.listFolders("", true) : allFolders ? Promise.resolve([]) : api.listFolders(state.currentFolderId ?? ""),
      state.currentFolderId ? api.folderBreadcrumbs(state.currentFolderId) : Promise.resolve([]),
    ]);
    return { catalog, active, trash, recentDocuments, folders, breadcrumbs };
  });
  if (!result) return;
  state.catalog = result.catalog.documents;
  state.documentTotal = result.catalog.total;
  state.documents = result.active.documents;
  state.trashCount = result.trash.total;
  state.recentCount = result.recentDocuments.total;
  state.folders = result.folders;
  state.breadcrumbs = result.breadcrumbs;
  state.tabs = state.tabs.filter((id) => state.documents.some((item) => item.id === id));
  persistTabs();
  renderDocuments();
  renderFolders();
  renderFolderBreadcrumbs();
  renderPagination();
  renderLibrarySummary();
  renderTabs();
  setStatus("就绪");
}

async function openDocument(documentId: string, updateLocation = true): Promise<void> {
  const view = await withBusy("打开文档…", () => api.getDocument(documentId));
  if (!view) return;
  if (!state.tabs.includes(documentId)) state.tabs.push(documentId);
  persistTabs();
  activateView(view);
  showWorkbench();
  if (updateLocation) window.history.pushState({ documentId }, "", `/documents/${documentId}`);
}

function activateView(view: DocumentView): void {
  state.active = view;
  state.selection = null;
  state.sketchPlane = undefined;
  state.sketchTool = "SELECT";
  state.history = [];
  cad.endSketch();
  cad.setSketchTool("SELECT");
  cad.render(view);
  const summaryIndex = state.documents.findIndex((item) => item.id === view.document.id);
  if (summaryIndex >= 0) state.documents[summaryIndex] = view.document;
  else state.documents.unshift(view.document);
  const catalogIndex = state.catalog.findIndex((item) => item.id === view.document.id);
  if (catalogIndex >= 0) state.catalog[catalogIndex] = view.document;
  element("#crumb-name").textContent = view.document.name;
  element("#tree-document-type").textContent = view.document.type === "PART" ? "PART STUDIO" : "PRODUCT";
  renderAll();
  void refreshHistory(view.document.id);
  setStatus(`${view.document.name} · ${view.document.type}`);
}

async function refreshHistory(documentId = state.active?.document.id): Promise<void> {
  if (!documentId) return;
  try {
    const history = await api.getHistory(documentId);
    if (state.active?.document.id !== documentId) return;
    state.history = history;
    renderHistory();
  } catch (error) {
    setStatus(error instanceof Error ? error.message : "读取历史失败", true);
  }
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

function persistTabs(): void {
  sessionStorage.setItem("occccad.tabs", JSON.stringify(state.tabs));
}

function showWorkbench(): void {
  element("#document-center").classList.add("hidden");
  element("#workbench").classList.remove("hidden");
  element("#workspace-crumb").classList.remove("hidden");
  requestAnimationFrame(() => window.dispatchEvent(new Event("resize")));
}

function showDocumentCenter(updateLocation = true): void {
  element("#workbench").classList.add("hidden");
  element("#document-center").classList.remove("hidden");
  element("#workspace-crumb").classList.add("hidden");
  if (updateLocation) window.history.pushState({}, "", "/");
  void refreshDocuments();
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
  persistTabs();
  if (state.active?.document.id === id) {
    state.active = undefined;
    const next = state.tabs[Math.min(index, state.tabs.length - 1)];
    if (next) void openDocument(next);
    else {
      cad.clear();
      renderAll();
      showDocumentCenter();
    }
  } else renderTabs();
}

function renderDocuments(): void {
  const host = element("#documents");
  host.replaceChildren();
  host.className = "document-list";
  if (state.catalog.length === 0) {
    host.classList.add("empty");
    const title = document.createElement("strong");
    title.textContent = state.libraryScope === "trash" ? "回收站为空" : "没有找到文档";
    const detail = document.createElement("span");
    detail.textContent = state.libraryScope === "trash" ? "移入回收站的文档会显示在这里。" : "新建 Part 或 Product 开始设计。";
    host.append(title, detail);
    return;
  }
  for (const documentInfo of state.catalog) {
    const row = document.createElement("div");
    row.className = "document-row";
    row.tabIndex = 0;
    if (state.selectedDocumentId === documentInfo.id) row.classList.add("selected");
    const identity = document.createElement("div");
    identity.className = "document-identity";
    const icon = document.createElement("span");
    icon.className = "icon";
    icon.textContent = documentInfo.type === "PART" ? "◇" : "▦";
    if (documentInfo.type === "PART") {
      const preview = document.createElement("img"); preview.alt = "";
      preview.src = `/api/documents/${documentInfo.id}/preview?v=${encodeURIComponent(documentInfo.versionId)}`;
      preview.addEventListener("load", () => { icon.textContent = ""; icon.append(preview); }, { once: true });
    }
    const names = document.createElement("span");
    const name = document.createElement("strong"); name.textContent = documentInfo.name;
    const workspace = document.createElement("small"); workspace.textContent = `${documentInfo.workspaceName ?? "Main"} · ${documentInfo.permission}`;
    names.append(name, workspace); identity.append(icon, names);
    const type = document.createElement("span"); type.className = `document-type ${documentInfo.type.toLowerCase()}`; type.textContent = documentInfo.type === "PART" ? "Part" : "Product";
    const description = document.createElement("span"); description.className = "document-description"; description.textContent = documentInfo.description || "—";
    const updated = document.createElement("time"); updated.dateTime = documentInfo.lastUpdated; updated.textContent = relativeDate(documentInfo.lastUpdated);
    const actions = document.createElement("div"); actions.className = "row-actions";
    if (state.libraryScope === "trash") {
      const restore = document.createElement("button"); restore.textContent = "恢复"; restore.disabled = !canEdit(documentInfo.permission);
      restore.addEventListener("click", (event) => { event.stopPropagation(); void restoreDocument(documentInfo.id); });
      actions.appendChild(restore);
    } else {
      const copy = document.createElement("button"); copy.textContent = "复制";
      copy.addEventListener("click", (event) => { event.stopPropagation(); showCopyDialog(documentInfo); });
      actions.append(copy);
      if (canEdit(documentInfo.permission)) {
        const edit = document.createElement("button"); edit.textContent = "编辑";
        edit.addEventListener("click", (event) => { event.stopPropagation(); showEditDocumentDialog(documentInfo); });
        const move = document.createElement("button"); move.textContent = "移动";
        move.addEventListener("click", (event) => { event.stopPropagation(); void showMoveDialog(documentInfo.id); });
        const remove = document.createElement("button"); remove.className = "remove"; remove.textContent = "删除";
        remove.addEventListener("click", (event) => { event.stopPropagation(); showDeleteDocumentDialog(documentInfo.id); });
        actions.prepend(edit); actions.append(move, remove);
      }
      if (documentInfo.permission === "OWNER") {
        const share = document.createElement("button"); share.textContent = "共享";
        share.addEventListener("click", (event) => { event.stopPropagation(); void showShareDialog("documents", documentInfo.id, documentInfo.name); });
        actions.prepend(share);
      }
    }
    row.append(identity, type, description, updated, actions);
    row.addEventListener("click", () => { state.selectedDocumentId = documentInfo.id; renderDocuments(); });
    if (!documentInfo.deletedAt) {
      row.addEventListener("dblclick", () => void openDocument(documentInfo.id));
      row.addEventListener("keydown", (event) => { if (event.key === "Enter") void openDocument(documentInfo.id); });
    }
    host.appendChild(row);
  }
}

function renderLibrarySummary(): void {
  const metrics = documentMetrics(state.documents);
  element("#active-count").textContent = String(state.documents.length);
  element("#part-count").textContent = String(metrics.parts);
  element("#product-count").textContent = String(metrics.products);
  element("#trash-count").textContent = String(state.trashCount);
  element("#recent-count").textContent = String(state.recentCount);
  element("#part-stat").textContent = String(metrics.parts);
  element("#product-stat").textContent = String(metrics.products);
  element("#recent-stat").textContent = String(metrics.recentlyUpdated);
}

function renderFolders(): void {
  const host = element("#folders");
  host.replaceChildren();
  host.classList.toggle("hidden", state.folders.length === 0 || state.libraryScope === "trash" || state.libraryScope === "recent" || state.libraryScope === "shared");
  for (const folder of state.folders) {
    const card = document.createElement("article");
    card.className = "folder-card";
    const icon = document.createElement("span"); icon.className = "folder-icon"; icon.textContent = "▰";
    const details = document.createElement("span");
    const name = document.createElement("strong"); name.textContent = folder.name;
    const counts = document.createElement("small");
    counts.textContent = `${folder.documentCount} 文档${folder.trashCount ? ` · ${folder.trashCount} Trash` : ""} · ${folder.childCount} 子文件夹`;
    details.append(name, counts);
    const actions = document.createElement("span"); actions.className = "folder-actions";
    if (folder.permission === "OWNER") {
      const share = document.createElement("button"); share.textContent = "共享";
      share.addEventListener("click", (event) => { event.stopPropagation(); void showShareDialog("folders", folder.id, folder.name); });
      actions.append(share);
    }
    if (canEdit(folder.permission)) {
      const edit = document.createElement("button"); edit.textContent = "编辑";
      edit.addEventListener("click", (event) => { event.stopPropagation(); showFolderDialog(folder); });
      const remove = document.createElement("button"); remove.textContent = "删除";
      remove.disabled = folder.documentCount > 0 || folder.trashCount > 0 || folder.childCount > 0;
      remove.title = remove.disabled ? "请先移出文件夹中的内容" : "删除空文件夹";
      remove.addEventListener("click", (event) => { event.stopPropagation(); void deleteFolder(folder); });
      actions.append(edit, remove);
    }
    card.append(icon, details, actions);
    card.addEventListener("dblclick", () => openFolder(folder.id));
    card.addEventListener("click", () => card.classList.toggle("selected"));
    host.appendChild(card);
  }
}

function renderFolderBreadcrumbs(): void {
  const host = element("#folder-breadcrumbs");
  host.replaceChildren();
  const root = document.createElement("button"); root.textContent = "我的文档";
  root.addEventListener("click", () => openFolder(undefined)); host.appendChild(root);
  for (const folder of state.breadcrumbs) {
    const separator = document.createElement("span"); separator.textContent = "›";
    const button = document.createElement("button"); button.textContent = folder.name;
    button.addEventListener("click", () => openFolder(folder.id)); host.append(separator, button);
  }
  host.classList.toggle("hidden", state.libraryScope === "trash" || state.libraryScope === "recent" || state.libraryScope === "shared");
  const currentPermission = state.breadcrumbs.at(-1)?.permission;
  const writable = !state.currentFolderId || canEdit(currentPermission);
  element<HTMLButtonElement>("#new-folder").disabled = !writable;
  element<HTMLButtonElement>("#new-part").disabled = !writable;
  element<HTMLButtonElement>("#new-product").disabled = !writable;
}

function openFolder(folderId?: string): void {
  if (state.libraryScope === "shared") {
    state.libraryScope = "active";
    for (const item of document.querySelectorAll<HTMLElement>("[data-scope]")) {
      item.classList.toggle("active", item.dataset.scope === "active");
    }
    element("#library-heading").textContent = "全部文档";
    element("#library-subtitle").textContent = "管理 Part 与 Product，双击文档进入 CAD 工作台。";
    element("#new-part").classList.remove("hidden"); element("#new-product").classList.remove("hidden");
    element("#new-folder").classList.remove("hidden");
  }
  state.currentFolderId = folderId;
  state.documentOffset = 0;
  void refreshDocuments();
}

function renderPagination(): void {
  element("#pagination-info").textContent = pageLabel(
    state.documentOffset, state.documentLimit, state.documentTotal);
  element<HTMLButtonElement>("#previous-page").disabled = state.documentOffset === 0;
  element<HTMLButtonElement>("#next-page").disabled = state.documentOffset + state.documentLimit >= state.documentTotal;
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
    children.appendChild(treeRow("Origin", "⌄", undefined, "基准几何"));
    const originChildren = document.createElement("div");
    originChildren.className = "tree-children nested";
    for (const datum of view.datumPlanes ?? []) {
      originChildren.appendChild(treeRow(datum.name, "▱", { kind: "plane", id: datum.id, plane: datum.plane }));
    }
    children.appendChild(originChildren);
    for (const feature of view.part.features) {
      const type = feature.type.toUpperCase();
      const isSketch = type.includes("SKETCH");
      const isImport = type === "IMPORT_STEP";
      const kind = isSketch ? "sketch" : isImport ? "import" : "pad";
      const detail = isSketch ? feature.plane ?? "XY" : isImport ? "STEP" : `${feature.length ?? 0} mm`;
      children.appendChild(treeRow(feature.name ?? (isSketch ? "Sketch" : "Extrude"),
        isSketch ? "⌑" : isImport ? "⇥" : "↥", { kind, id: feature.id }, detail));
    }
    if (view.artifact) {
      children.appendChild(treeRow("Parts", "⌄", undefined, `${view.artifact.topology.solids}`));
      const parts = document.createElement("div");
      parts.className = "tree-children nested";
      parts.appendChild(treeRow("Part 1", "⬡", { kind: "solid", id: "body-1" }, "SOLID"));
      children.appendChild(parts);
    }
  } else {
    for (const instance of view.product?.instances ?? []) {
      const mode = instance.referenceMode === "PINNED" ? "PINNED" : "LIVE";
      children.appendChild(treeRow(instance.name, "⊹", { kind: "instance", id: instance.id }, mode));
    }
  }
  host.appendChild(children);
}

function selectedFeature(): Feature | undefined {
  if (state.selection?.kind !== "sketch" && state.selection?.kind !== "pad" && state.selection?.kind !== "import") return undefined;
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
    const type = feature.type.toUpperCase();
    const isSketch = type.includes("SKETCH");
    title.textContent = feature.name ?? (isSketch ? "Rectangle Sketch" : "Pad");
    if (type === "IMPORT_STEP") {
      rows.push(["类型", "导入 STEP"], ["源文件", feature.fileName ?? "—"],
        ["GeometryKey", feature.geometryKey ?? "—"]);
    } else if (isSketch) {
      const rectangle = feature.rectangle ?? { origin: feature.origin ?? [0, 0], width: feature.width ?? 0, height: feature.height ?? 0 };
      rows.push(["类型", "矩形草图"], ["基准面", feature.plane ?? "XY"],
        ["原点", `${rectangle.origin[0].toFixed(2)}, ${rectangle.origin[1].toFixed(2)}`],
        ["宽度", `${rectangle.width.toFixed(2)} mm`], ["高度", `${rectangle.height.toFixed(2)} mm`]);
    } else rows.push(["类型", "拉伸"], ["操作", feature.operation ?? "ADD"],
      ["草图", feature.profile ?? "—"], ["长度", `${feature.length ?? 0} mm`]);
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
  const list = element("#history-list");
  list.replaceChildren();
  for (const entry of state.history) {
    const row = document.createElement("div");
    row.className = `history-entry${entry.isHead ? " current" : ""}`;
    const marker = document.createElement("i");
    const content = document.createElement("span");
    const label = document.createElement("b");
    label.textContent = entry.versionName
      ? `${entry.versionName} · ${entry.commandType.replaceAll("_", " ")}`
      : entry.commandType.replaceAll("_", " ");
    const meta = document.createElement("small");
    meta.textContent = `#${entry.sequence} · ${new Date(entry.createdAt).toLocaleString()}`;
    content.append(label, meta); row.append(marker, content);
    if (!entry.isHead && canEdit(view?.document.permission)) {
      const restore = document.createElement("button");
      restore.textContent = "恢复";
      restore.title = "将此状态作为 Main 工作区的新变更恢复";
      restore.addEventListener("click", () => void restoreVersion(entry));
      row.appendChild(restore);
    }
    list.appendChild(row);
  }
}

async function restoreVersion(entry: HistoryEntry): Promise<void> {
  if (!state.active || !window.confirm(`将 #${entry.sequence} 恢复为 Main 的最新状态？\n当前历史不会被覆盖。`)) return;
  const result = await withBusy("恢复历史状态…", () => api.restore(state.active!.document.id, entry.versionId));
  if (!result) return;
  activateView(result);
  await refreshDocuments();
  setStatus(`已将 #${entry.sequence} 作为新的 RESTORE 变更恢复`);
}

function renderToolbar(): void {
  const view = state.active;
  const isPart = view?.document.type === "PART";
  const isProduct = view?.document.type === "PRODUCT";
  const editable = canEdit(view?.document.permission);
  const inSketch = Boolean(state.sketchPlane);
  element<HTMLButtonElement>("#start-sketch").disabled = state.busy || !editable || !isPart || state.selection?.kind !== "plane" || inSketch;
  element("#exit-sketch").classList.toggle("hidden", !inSketch);
  element("#start-sketch").classList.toggle("hidden", inSketch);
  const rectangle = element<HTMLButtonElement>("#rectangle-tool");
  rectangle.classList.toggle("hidden", !inSketch);
  rectangle.classList.toggle("active", state.sketchTool === "RECTANGLE");
  rectangle.disabled = state.busy || !editable || !inSketch;
  element<HTMLButtonElement>("#pad-sketch").disabled = state.busy || !editable || !isPart || state.selection?.kind !== "sketch" || inSketch;
  element<HTMLButtonElement>("#import-step").disabled = state.busy || !editable || !isPart || (view?.part?.features.length ?? 0) > 0 || inSketch;
  element<HTMLButtonElement>("#export-step").disabled = state.busy || !isPart || !view?.artifact || inSketch;
  element<HTMLButtonElement>("#insert-document").disabled = state.busy || !editable || !isProduct;
  const referenceButton = element<HTMLButtonElement>("#reference-mode");
  const instance = selectedInstance();
  referenceButton.disabled = state.busy || !editable || !isProduct || !instance;
  referenceButton.querySelector("span")!.textContent = instance?.referenceMode === "PINNED"
    ? "跟随最新" : "固定版本";
  element<HTMLButtonElement>("#undo").disabled = state.busy || !editable || !view?.document.canUndo;
  element<HTMLButtonElement>("#redo").disabled = state.busy || !editable || !view?.document.canRedo;
  element<HTMLButtonElement>("#create-version").disabled = state.busy || !editable || !view;
  element<HTMLButtonElement>("#share-document").disabled = state.busy || view?.document.permission !== "OWNER";
  const mode = element("#mode-label");
  mode.textContent = inSketch
    ? `${state.sketchPlane} 草图 · ${state.sketchTool === "RECTANGLE" ? "矩形命令" : "选择工具"}`
    : "选择模式";
  mode.classList.toggle("sketch", inSketch);
  element("#hint").textContent = inSketch
    ? state.sketchTool === "RECTANGLE"
      ? "矩形：单击并拖动两个对角点；按 Esc 返回选择工具"
      : "选择“矩形”工具开始绘制，或按 R"
    : isProduct ? "选择实例后使用三轴手柄拖动" : "选择一个基准面，然后点击“新建草图”";
}

function showVersionDialog(): void {
  if (!state.active) return;
  const count = state.history.filter((entry) => entry.versionName).length + 1;
  element<HTMLInputElement>("#version-name").value = `V${count}`;
  element<HTMLInputElement>("#version-description").value = "";
  element<HTMLDialogElement>("#version-dialog").showModal();
  element<HTMLInputElement>("#version-name").select();
}

async function createVersion(): Promise<void> {
  if (!state.active) return;
  const name = element<HTMLInputElement>("#version-name").value.trim();
  const description = element<HTMLInputElement>("#version-description").value.trim();
  if (!name) return;
  const history = await withBusy(`创建版本 ${name}…`, () =>
    api.createVersion(state.active!.document.id, name, description));
  if (!history) return;
  state.history = history;
  element<HTMLDialogElement>("#version-dialog").close();
  renderHistory();
  setStatus(`已创建不可变版本 ${name}`);
}

function showDocumentDialog(type: "PART" | "PRODUCT"): void {
  element<HTMLInputElement>("#document-type").value = type;
  const base = type === "PART" ? "Part" : "Product";
  let index = state.documents.filter((item) => item.type === type).length + 1;
  while (state.documents.some((item) => item.name === `${base} ${index}`)) index++;
  element<HTMLInputElement>("#document-name").value = `${base} ${index}`;
  element<HTMLTextAreaElement>("#document-description").value = "";
  element<HTMLDialogElement>("#document-dialog").showModal();
  element<HTMLInputElement>("#document-name").select();
}

async function createDocument(): Promise<void> {
  const type = element<HTMLInputElement>("#document-type").value as "PART" | "PRODUCT";
  const name = element<HTMLInputElement>("#document-name").value.trim();
  const description = element<HTMLTextAreaElement>("#document-description").value.trim();
  if (!name) return;
  const view = await withBusy(`创建 ${type}…`, () =>
    api.createDocument(type, name, description, state.currentFolderId));
  if (!view) return;
  element<HTMLDialogElement>("#document-dialog").close();
  state.documents.unshift(view.document);
  state.tabs.push(view.document.id);
  activateView(view);
  showWorkbench();
  persistTabs();
  window.history.pushState({ documentId: view.document.id }, "", `/documents/${view.document.id}`);
}

function showEditDocumentDialog(documentInfo: DocumentSummary): void {
  element<HTMLInputElement>("#edit-document-id").value = documentInfo.id;
  element<HTMLInputElement>("#edit-document-name").value = documentInfo.name;
  element<HTMLTextAreaElement>("#edit-document-description").value = documentInfo.description;
  element<HTMLDialogElement>("#edit-document-dialog").showModal();
  element<HTMLInputElement>("#edit-document-name").select();
}

async function confirmEditDocument(): Promise<void> {
  const id = element<HTMLInputElement>("#edit-document-id").value;
  const name = element<HTMLInputElement>("#edit-document-name").value.trim();
  const description = element<HTMLTextAreaElement>("#edit-document-description").value.trim();
  if (!id || !name) return;
  const view = await withBusy("保存文档信息…", () => api.updateDocument(id, name, description));
  if (!view) return;
  element<HTMLDialogElement>("#edit-document-dialog").close();
  if (state.active?.document.id === id) activateView(view);
  await refreshDocuments();
  setStatus(`已更新 ${name}`);
}

function showDeleteDocumentDialog(documentId: string): void {
  element<HTMLInputElement>("#delete-document-id").value = documentId;
  element<HTMLDialogElement>("#delete-document-dialog").showModal();
}

async function confirmDeleteDocument(): Promise<void> {
  const id = element<HTMLInputElement>("#delete-document-id").value;
  if (!id) return;
  const removed = await withBusy("移入回收站…", async () => { await api.deleteDocument(id); return true; });
  if (!removed) return;
  element<HTMLDialogElement>("#delete-document-dialog").close();
  state.tabs = state.tabs.filter((tabId) => tabId !== id);
  persistTabs();
  if (state.active?.document.id === id) {
    state.active = undefined;
    cad.clear();
  }
  await refreshDocuments();
  showDocumentCenter();
  setStatus("文档已移入回收站");
}

async function restoreDocument(id: string): Promise<void> {
  const view = await withBusy("恢复文档…", () => api.restoreDocument(id));
  if (!view) return;
  await refreshDocuments();
  setStatus(`已恢复 ${view.document.name}`);
}

function showFolderDialog(folder?: FolderSummary): void {
  element<HTMLInputElement>("#folder-id").value = folder?.id ?? "";
  element("#folder-dialog-title").textContent = folder ? "编辑文件夹" : "新建文件夹";
  element<HTMLInputElement>("#folder-name").value = folder?.name ?? "";
  element<HTMLTextAreaElement>("#folder-description").value = folder?.description ?? "";
  element<HTMLDialogElement>("#folder-dialog").showModal();
  element<HTMLInputElement>("#folder-name").focus();
}

async function confirmFolder(): Promise<void> {
  const id = element<HTMLInputElement>("#folder-id").value;
  const name = element<HTMLInputElement>("#folder-name").value.trim();
  const description = element<HTMLTextAreaElement>("#folder-description").value.trim();
  if (!name) return;
  const result = await withBusy(id ? "保存文件夹…" : "创建文件夹…", () => id
    ? api.updateFolder(id, name, description)
    : api.createFolder(name, description, state.currentFolderId));
  if (!result) return;
  element<HTMLDialogElement>("#folder-dialog").close();
  await refreshDocuments();
  setStatus(id ? `已更新 ${name}` : `已创建 ${name}`);
}

async function deleteFolder(folder: FolderSummary): Promise<void> {
  if (!window.confirm(`删除空文件夹“${folder.name}”？`)) return;
  const deleted = await withBusy("删除文件夹…", async () => { await api.deleteFolder(folder.id); return true; });
  if (!deleted) return;
  await refreshDocuments();
  setStatus(`已删除空文件夹 ${folder.name}`);
}

async function showMoveDialog(documentId: string): Promise<void> {
  const options = await withBusy("读取文件夹…", () => flattenFolderTree(api.listFolders));
  if (!options) return;
  const select = element<HTMLSelectElement>("#move-folder-select"); select.replaceChildren();
  const root = document.createElement("option"); root.value = ""; root.textContent = "我的文档（根目录）";
  select.appendChild(root);
  for (const folder of options) {
    const option = document.createElement("option"); option.value = folder.id; option.textContent = folder.label;
    select.appendChild(option);
  }
  const current = state.documents.find((item) => item.id === documentId)?.folderId ?? "";
  select.value = current;
  element<HTMLInputElement>("#move-document-id").value = documentId;
  element<HTMLDialogElement>("#move-dialog").showModal();
}

async function confirmMove(): Promise<void> {
  const documentId = element<HTMLInputElement>("#move-document-id").value;
  const folderId = element<HTMLSelectElement>("#move-folder-select").value || undefined;
  const view = await withBusy("移动文档…", () => api.moveDocument(documentId, folderId));
  if (!view) return;
  element<HTMLDialogElement>("#move-dialog").close();
  if (state.active?.document.id === documentId) state.active = view;
  await refreshDocuments();
  setStatus(`已移动 ${view.document.name}`);
}

function showCopyDialog(documentInfo: DocumentSummary): void {
  element<HTMLInputElement>("#copy-document-id").value = documentInfo.id;
  element<HTMLInputElement>("#copy-document-name").value = `${documentInfo.name} Copy`;
  element<HTMLDialogElement>("#copy-dialog").showModal();
  element<HTMLInputElement>("#copy-document-name").select();
}

async function confirmCopy(): Promise<void> {
  const id = element<HTMLInputElement>("#copy-document-id").value;
  const name = element<HTMLInputElement>("#copy-document-name").value.trim();
  if (!id || !name) return;
  const source = state.documents.find((item) => item.id === id);
  const targetFolder = canEdit(source?.permission) ? source?.folderId : "";
  const view = await withBusy("复制 Main Workspace…", () => api.copyDocument(id, name, targetFolder));
  if (!view) return;
  element<HTMLDialogElement>("#copy-dialog").close();
  state.documents.unshift(view.document);
  if (!state.tabs.includes(view.document.id)) state.tabs.push(view.document.id);
  persistTabs(); activateView(view); showWorkbench();
  window.history.pushState({ documentId: view.document.id }, "", `/documents/${view.document.id}`);
  setStatus(`已创建副本 ${name}`);
}

function startSketch(): void {
  if (state.selection?.kind !== "plane") return;
  state.sketchPlane = state.selection.plane;
  state.sketchTool = "SELECT";
  cad.beginSketch(state.sketchPlane);
  cad.setSketchTool("SELECT");
  renderToolbar();
}

function exitSketch(): void {
  state.sketchPlane = undefined;
  state.sketchTool = "SELECT";
  cad.setSketchTool("SELECT");
  cad.endSketch();
  state.selection = null;
  renderAll();
}

function toggleRectangleTool(force?: boolean): void {
  if (!state.sketchPlane) return;
  const enabled = force ?? state.sketchTool !== "RECTANGLE";
  state.sketchTool = enabled ? "RECTANGLE" : "SELECT";
  cad.setSketchTool(state.sketchTool);
  renderToolbar();
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
  if (plane) {
    cad.beginSketch(plane);
    cad.setSketchTool(state.sketchTool);
  }
  if (feature) cad.select({ kind: "sketch", id: feature.id });
  renderAll();
  void refreshHistory(result.document.id);
  await refreshDocuments();
  setStatus(`已创建 ${draft.width.toFixed(2)} × ${draft.height.toFixed(2)} mm 矩形草图`);
}

async function importStep(file: File): Promise<void> {
  if (!state.active || state.active.document.type !== "PART") return;
  const result = await withBusy(`上传 ${file.name}…`, async () => {
    const job = await api.importStep(state.active!.document.id, file);
    await waitForJob(job.id, `正在导入 ${file.name}`);
    return api.getDocument(state.active!.document.id);
  });
  element<HTMLInputElement>("#step-file").value = "";
  if (!result) return;
  activateView(result);
  const imported = result.part?.features.find((feature) => feature.type.toUpperCase() === "IMPORT_STEP");
  if (imported) cad.select({ kind: "import", id: imported.id });
  await refreshDocuments();
  setStatus(`STEP 导入完成 · ${file.name}`);
}

async function exportStep(): Promise<void> {
  if (!state.active || state.active.document.type !== "PART") return;
  const completed = await withBusy("正在生成 STEP…", async () => {
    const job = await api.startExportStep(state.active!.document.id);
    await waitForJob(job.id, "正在生成 STEP");
    await api.downloadJob(job.id);
    return true;
  });
  if (completed) setStatus("STEP 导出完成");
}

async function waitForJob(id: string, label: string): Promise<void> {
  for (;;) {
    const job = await api.getJob(id);
    if (job.state === "SUCCEEDED") return;
    if (job.state === "FAILED" || job.state === "CANCELED") throw new Error(job.errorMessage ?? `${label}失败`);
    setStatus(`${label} · ${job.state === "RUNNING" ? "处理中" : "排队中"}`);
    await new Promise((resolve) => window.setTimeout(resolve, 700));
  }
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
  if (!canEdit(state.active.document.permission)) {
    cad.render(state.active); setStatus("当前文档为只读", true); return;
  }
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
  if (!state.active || !canEdit(state.active.document.permission)) return;
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
element("#new-folder").addEventListener("click", () => showFolderDialog());
element("#create-version").addEventListener("click", showVersionDialog);
element("#share-document").addEventListener("click", () => {
  if (state.active) void showShareDialog("documents", state.active.document.id, state.active.document.name);
});
element("#confirm-share").addEventListener("click", (event) => { event.preventDefault(); void confirmShare(); });
element<HTMLFormElement>("#login-form").addEventListener("submit", (event) => void login(event));
element<HTMLFormElement>("#register-form").addEventListener("submit", (event) => void register(event));
element("#show-register").addEventListener("click", () => {
  element("#login-form").classList.add("hidden"); element("#register-form").classList.remove("hidden");
});
element("#show-login").addEventListener("click", () => {
  element("#register-form").classList.add("hidden"); element("#login-form").classList.remove("hidden");
});
element("#logout-button").addEventListener("click", () => void logout());
element("#admin-button").addEventListener("click", () => void showAdmin());
element("#admin-new-user").addEventListener("click", () => showAdminUser());
element("#admin-save-user").addEventListener("click", (event) => void saveAdminUser(event));
element("#admin-confirm-reset").addEventListener("click", (event) => void resetAdminPassword(event));
element("#confirm-password").addEventListener("click", (event) => void changePassword(event));
element("#admin-status").addEventListener("change", () => void refreshAdmin());
let adminSearchTimer = 0;
element<HTMLInputElement>("#admin-search").addEventListener("input", () => {
  window.clearTimeout(adminSearchTimer); adminSearchTimer = window.setTimeout(() => void refreshAdmin(), 200);
});
element("#confirm-version").addEventListener("click", (event) => { event.preventDefault(); void createVersion(); });
element("#confirm-document").addEventListener("click", (event) => { event.preventDefault(); void createDocument(); });
element("#confirm-edit-document").addEventListener("click", (event) => { event.preventDefault(); void confirmEditDocument(); });
element("#confirm-delete-document").addEventListener("click", (event) => { event.preventDefault(); void confirmDeleteDocument(); });
element("#confirm-folder").addEventListener("click", (event) => { event.preventDefault(); void confirmFolder(); });
element("#confirm-move").addEventListener("click", (event) => { event.preventDefault(); void confirmMove(); });
element("#confirm-copy").addEventListener("click", (event) => { event.preventDefault(); void confirmCopy(); });
element("#home-button").addEventListener("click", () => showDocumentCenter());
element("#start-sketch").addEventListener("click", startSketch);
element("#exit-sketch").addEventListener("click", exitSketch);
element("#rectangle-tool").addEventListener("click", () => toggleRectangleTool());
element("#pad-sketch").addEventListener("click", () => void padSketch());
element("#import-step").addEventListener("click", () => element<HTMLInputElement>("#step-file").click());
element<HTMLInputElement>("#step-file").addEventListener("change", (event) => {
  const file = (event.currentTarget as HTMLInputElement).files?.[0];
  if (file) void importStep(file);
});
element("#export-step").addEventListener("click", () => void exportStep());
element("#confirm-pad").addEventListener("click", (event) => { event.preventDefault(); void confirmPad(); });
element("#insert-document").addEventListener("click", showInsertDialog);
element("#reference-mode").addEventListener("click", () => void toggleReferenceMode());
element("#confirm-insert").addEventListener("click", (event) => { event.preventDefault(); void confirmInsert(); });
element("#undo").addEventListener("click", () => void history("undo"));
element("#redo").addEventListener("click", () => void history("redo"));
element("#refresh-documents").addEventListener("click", () => void refreshDocuments());
element("#fit-view").addEventListener("click", () => cad.fit());
for (const button of document.querySelectorAll<HTMLButtonElement>("[data-scope]")) {
  button.addEventListener("click", () => {
    const scope = button.dataset.scope as typeof state.libraryScope;
    state.libraryScope = scope;
    state.documentOffset = 0;
    if (scope === "trash" || scope === "recent" || scope === "shared") state.currentFolderId = undefined;
    state.selectedDocumentId = undefined;
    for (const item of document.querySelectorAll("[data-scope]")) item.classList.toggle("active", item === button);
    const headings = {
      active: ["全部文档", "管理 Part 与 Product，双击文档进入 CAD 工作台。"],
      recent: ["最近打开", "快速返回最近进入过的设计文档。"],
      shared: ["与我共享", "通过用户、团队或文件夹继承获得访问权限的设计。"],
      parts: ["零件文档", "参数化 Part Studio 与导入的 STEP 零件。"],
      products: ["产品文档", "由 Part 或子 Product 实例组成的装配文档。"],
      trash: ["回收站", "文档仍保留历史和引用关系，恢复后可继续编辑。"],
    } as const;
    element("#library-heading").textContent = headings[scope][0];
    element("#library-subtitle").textContent = headings[scope][1];
    element("#document-type-filter").classList.toggle("hidden", scope === "parts" || scope === "products");
    element("#new-part").classList.toggle("hidden", scope === "trash" || scope === "shared");
    element("#new-product").classList.toggle("hidden", scope === "trash" || scope === "shared");
    element("#new-folder").classList.toggle("hidden", scope === "trash" || scope === "recent" || scope === "shared");
    element("#document-sort").classList.toggle("hidden", scope === "recent");
    void refreshDocuments();
  });
}
let searchTimer = 0;
element<HTMLInputElement>("#document-search").addEventListener("input", () => {
  window.clearTimeout(searchTimer);
  state.documentOffset = 0;
  searchTimer = window.setTimeout(() => void refreshDocuments(), 220);
});
element<HTMLSelectElement>("#document-type-filter").addEventListener("change", () => void refreshDocuments());
element<HTMLSelectElement>("#document-sort").addEventListener("change", () => {
  state.documentOffset = 0; void refreshDocuments();
});
element("#previous-page").addEventListener("click", () => {
  state.documentOffset = Math.max(0, state.documentOffset - state.documentLimit); void refreshDocuments();
});
element("#next-page").addEventListener("click", () => {
  if (state.documentOffset + state.documentLimit < state.documentTotal) {
    state.documentOffset += state.documentLimit; void refreshDocuments();
  }
});
for (const button of document.querySelectorAll<HTMLButtonElement>("[data-inspector]")) {
  button.addEventListener("click", () => {
    const page = button.dataset.inspector;
    for (const item of document.querySelectorAll("[data-inspector]")) item.classList.toggle("active", item === button);
    element("#properties-panel").classList.toggle("active", page === "properties");
    element("#history-panel").classList.toggle("active", page === "history");
  });
}
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
  } else if (!event.ctrlKey && !event.metaKey && !event.altKey && event.key.toLowerCase() === "r" && state.sketchPlane) {
    event.preventDefault();
    toggleRectangleTool(true);
  } else if (event.key === "Escape" && state.sketchTool !== "SELECT") {
    event.preventDefault();
    toggleRectangleTool(false);
  }
});

window.addEventListener("popstate", () => {
  const match = window.location.pathname.match(/^\/documents\/([^/]+)$/);
  if (match) void openDocument(match[1], false);
  else showDocumentCenter(false);
});

async function startApplication(): Promise<void> {
  await initializeIdentity();
  showApplication();
  await checkHealth();
  await refreshDocuments();
  const match = window.location.pathname.match(/^\/documents\/([^/]+)$/);
  if (match) await openDocument(match[1], false);
  else {
    showDocumentCenter(false);
    renderAll();
  }
}

void startApplication().catch(() => showAuthentication());
