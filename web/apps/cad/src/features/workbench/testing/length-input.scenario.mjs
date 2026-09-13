import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";

const require = createRequire(new URL("../../../../package.json", import.meta.url));
const ts = require("typescript");
const source = await readFile(new URL("../length-input.ts", import.meta.url), "utf8");
const output = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const { parseLengthInput } = await import(`data:text/javascript;base64,${Buffer.from(output).toString("base64")}`);

assert.deepEqual(parseLengthInput("40 mm"), { value: 40, unit: "mm" });
assert.deepEqual(parseLengthInput("4CM"), { value: 4, unit: "cm" });
assert.deepEqual(parseLengthInput("0.04 m"), { value: 0.04, unit: "m" });
assert.deepEqual(parseLengthInput("1.5 in"), { value: 1.5, unit: "in" });
assert.throws(() => parseLengthInput("40"));
assert.throws(() => parseLengthInput("0 mm"));
assert.throws(() => parseLengthInput("NaN mm"));
console.log("Feature length input scenarios passed.");
