import assert from "node:assert/strict";
import { createServer } from "vite";
const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const { openDocumentTab } = await server.ssrLoadModule("/src/features/workbench/open-document-tab.ts");
  const { QueryClient } = await import("@tanstack/react-query");
  const client = new QueryClient();
  const root = { id: "assembly", name: "Assembly", type: "PRODUCT" };
  const part = { document: { id: "part", name: "Part", type: "PART" } };
  client.setQueryData(["open-documents"], [root]);
  client.setQueryData(["document", "part"], part);
  let registrations = 0;
  const load = async (id) => { assert.equal(id, "part"); registrations++; return part; };
  const paths = [];
  const navigate = (path) => {
    assert.deepEqual(client.getQueryData(["open-documents"]).map(doc => doc.id), ["assembly", "part"], "Tab must exist before navigation");
    paths.push(path);
  };
  await openDocumentTab("part", client, load, navigate);
  await openDocumentTab("part", client, load, navigate);
  assert.equal(registrations, 2, "cached document must still register as open");
  assert.deepEqual(paths, ["/documents/part", "/documents/part"]);
  assert.equal(client.getQueryData(["open-documents"]).length, 2, "reuse existing Tab");
  await assert.rejects(openDocumentTab("denied", client, async () => { throw new Error("denied"); }, navigate));
  assert.equal(paths.length, 2, "failed open must not navigate");
  client.clear();
  console.log("Document Tab registration passed.");
} finally { await server.close(); }
