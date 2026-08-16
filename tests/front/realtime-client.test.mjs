import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";

const require = createRequire(new URL("../../web/apps/cad/package.json", import.meta.url));
const ts = require("typescript");

const sourceURL = new URL("../../web/apps/cad/src/api/websocket-lifecycle.ts", import.meta.url);
const source = await readFile(sourceURL, "utf8");
const output = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const moduleURL = `data:text/javascript;base64,${Buffer.from(`${output}\n//# sourceURL=${pathToFileURL(sourceURL.pathname)}`).toString("base64")}`;
const lifecycle = await import(moduleURL);

assert.ok(lifecycle.initializationFailureCloseCode >= 3000 && lifecycle.initializationFailureCloseCode <= 4999);

const calls = [];
lifecycle.closeAfterInitializationFailure({ readyState: 1, close: (...args) => calls.push(args) });
assert.deepEqual(calls, [[4001, "initialization failed"]]);

lifecycle.closeAfterInitializationFailure({ readyState: 3, close: () => assert.fail("closed socket must not be closed again") });

console.log("Realtime WebSocket lifecycle tests passed.");
