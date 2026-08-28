import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";

const require = createRequire(new URL("../../../package.json", import.meta.url));
const ts = require("typescript");

const sourceURL = new URL("../websocket-lifecycle.ts", import.meta.url);
const source = await readFile(sourceURL, "utf8");
const output = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const moduleURL = `data:text/javascript;base64,${Buffer.from(`${output}\n//# sourceURL=${pathToFileURL(sourceURL.pathname)}`).toString("base64")}`;
const lifecycle = await import(moduleURL);

const uuidSourceURL = new URL("../../utils/random-uuid.ts", import.meta.url);
const uuidSource = await readFile(uuidSourceURL, "utf8");
const uuidOutput = ts.transpileModule(uuidSource, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const uuidModuleURL = `data:text/javascript;base64,${Buffer.from(`${uuidOutput}\n//# sourceURL=${pathToFileURL(uuidSourceURL.pathname)}`).toString("base64")}`;
const uuid = await import(uuidModuleURL);

assert.ok(lifecycle.initializationFailureCloseCode >= 3000 && lifecycle.initializationFailureCloseCode <= 4999);

const calls = [];
lifecycle.closeAfterInitializationFailure({ readyState: 1, close: (...args) => calls.push(args) });
assert.deepEqual(calls, [[4001, "initialization failed"]]);

lifecycle.closeAfterInitializationFailure({ readyState: 3, close: () => assert.fail("closed socket must not be closed again") });

const fallbackUUID = uuid.randomUUID({ getRandomValues: (bytes) => { bytes.fill(0); return bytes; } });
assert.match(fallbackUUID, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
assert.equal(uuid.randomUUID({ randomUUID: () => "native-uuid" }), "native-uuid");

console.log("Realtime WebSocket lifecycle tests passed.");
