import assert from "node:assert/strict";
import { createServer } from "vite";

globalThis.window = { setTimeout };
const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const { mockApi } = await server.ssrLoadModule("/src/api/mock-api.ts");
  const root = await mockApi.createFolder("Trash check root", "");
  const child = await mockApi.createFolder("Trash check child", "", root.id);
  const active = await mockApi.createDocument("PART", "Active in folder", "", child.id);
  const alreadyTrashed = await mockApi.createDocument("PART", "Already trashed", "", child.id);
  await mockApi.deleteDocument(alreadyTrashed.document.id);
  await mockApi.deleteFolder(root.id);
  assert.equal((await mockApi.listFolders()).some((folder) => folder.id === root.id), false);
  assert.equal((await mockApi.listFolders(root.id)).some((folder) => folder.id === child.id), false);
  assert.equal((await mockApi.listTrashedFolders()).some((folder) => folder.id === root.id), true);
  assert.equal((await mockApi.listDocuments({ allFolders: true })).documents.some((item) => item.id === active.document.id), false);
  assert.equal((await mockApi.listDocuments({ scope: "trash", allFolders: true })).documents.some((item) => item.id === alreadyTrashed.document.id), false);
  await mockApi.restoreFolder(root.id);
  assert.equal((await mockApi.listFolders()).some((folder) => folder.id === root.id), true);
  assert.equal((await mockApi.listFolders(root.id)).some((folder) => folder.id === child.id), true);
  assert.equal((await mockApi.listDocuments({ allFolders: true })).documents.some((item) => item.id === active.document.id), true);
  assert.equal((await mockApi.listDocuments({ scope: "trash", allFolders: true })).documents.some((item) => item.id === alreadyTrashed.document.id), true);
  console.log("Folder trash and restore preserve independent document trash state.");
} finally { await server.close(); }
