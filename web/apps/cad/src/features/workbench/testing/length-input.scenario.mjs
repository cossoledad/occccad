import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({server:{middlewareMode:true},appType:"custom",logLevel:"silent"});
let parseLengthInput;
try{({parseLengthInput}=await server.ssrLoadModule("/src/features/workbench/length-input.ts"));}finally{await server.close();}

assert.deepEqual(parseLengthInput("40 mm"), { value: 40, unit: "mm" });
assert.deepEqual(parseLengthInput("4CM"), { value: 4, unit: "cm" });
assert.deepEqual(parseLengthInput("0.04 m"), { value: 0.04, unit: "m" });
assert.deepEqual(parseLengthInput("1.5 in"), { value: 1.5, unit: "in" });
assert.deepEqual(parseLengthInput("40"), { value: 40, unit: "mm" });
assert.deepEqual(parseLengthInput("40", "cm"), { value: 40, unit: "cm" });
assert.throws(() => parseLengthInput("0 mm"));
assert.throws(() => parseLengthInput("NaN mm"));
console.log("Feature length input scenarios passed.");
