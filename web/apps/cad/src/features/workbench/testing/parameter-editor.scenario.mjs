import assert from "node:assert/strict";
import { createServer } from "vite";

const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const { insertParameterReference, isLengthParameter, linearExtrudeLengthInput, linearExtrudeLengthEditInput, parameterEditSource, parameterDisplayValue, parameterSourceText, parseParameterSource } = await server.ssrLoadModule("/src/features/workbench/parameter-editor.ts");
  const parameter = {
    parameterId: "parameter:sketch-a:constraint:length:value", key: "base_width", label: "LENGTH",
    valueType: "QUANTITY", dimension: { Length: 1, Mass: 0, Time: 0, Current: 0, Temperature: 0, Amount: 0, Luminous: 0, Semantic: "" },
    displayUnit: "mm", role: "INPUT", source: { literal: { siValue: 0.02, dimension: { Length: 1 } } },
    evaluatedValue: { siValue: 0.02, dimension: { Length: 1 } },
  };
  assert.equal(insertParameterReference("20 mm","r_1"),"r_1");
  assert.equal(insertParameterReference(" + 5 mm","r_1",{start:0,end:0}),"r_1 + 5 mm");
  assert.equal(insertParameterReference("r_2 + 5 mm","r_1",{start:0,end:3}),"r_1 + 5 mm");
  assert.throws(()=>insertParameterReference("","bad key"));
  assert.equal(parameterSourceText(parameter), "20 mm");
  assert.equal(parameterDisplayValue(parameter), "20 mm");
  assert.deepEqual(parseParameterSource("40 mm"), { kind: "LITERAL", value: 40, unit: "mm" });
  assert.deepEqual(parseParameterSource("base_width / 2"), { kind: "EXPRESSION", expression: "base_width / 2" });
  assert.deepEqual(linearExtrudeLengthInput("2 cm"), { length: 20 });
  assert.deepEqual(linearExtrudeLengthInput("base_width / 2"), { lengthExpression: "base_width / 2" });
  for (const [unit, expected] of [["mm",20],["cm",200],["m",20000],["in",508]]) {
    assert.deepEqual(linearExtrudeLengthInput("20", unit), {length:expected});
    assert.deepEqual(linearExtrudeLengthInput("2 cm", unit), {length:20});
  }
  assert.equal(parameterSourceText(parameter,"cm"), "2");
  assert.deepEqual(parseParameterSource("90","deg"), {kind:"LITERAL",value:90,unit:"deg"});
  assert.deepEqual(parseParameterSource("base_width / 2","cm"), {kind:"EXPRESSION",expression:"base_width / 2"});
  assert.throws(()=>linearExtrudeLengthInput("1e999","cm"));
  assert.throws(()=>linearExtrudeLengthInput("-20","cm"));
  assert.equal(isLengthParameter(parameter), true);
  assert.throws(() => linearExtrudeLengthInput("90 deg"), /mm、cm、m 或 in/);
  assert.equal(parameterSourceText({ ...parameter, source: { expression: { sourceText: "base_width / 2" } } }), "base_width / 2");
  assert.equal(parameterSourceText({ ...parameter, source: { external: { publicationId: "publication-width",
    resolvedRevisionId: "revision-1234567890" } } }), "Publication publication-width @ revision-123");
  const precise={...parameter,source:{literal:{siValue:0.0123456789}},evaluatedValue:{siValue:0.0123456789}};
  assert.equal(parameterSourceText(precise,"mm"),"12.35");
  assert.equal(parameterDisplayValue(precise),"12.35 mm");
  assert.equal(parameterEditSource(precise,"12.35","mm"),"12.3456789");
  assert.deepEqual(linearExtrudeLengthEditInput("12.35","mm",12.3456789,precise),{length:12.3456789},"untouched display retains exact feature length");
  assert.deepEqual(linearExtrudeLengthEditInput("15.12","mm",12.3456789,precise),{length:15.12});
  const formula={...precise,source:{expression:{sourceText:"width * 1.23456789"}}};
  assert.equal(parameterSourceText(formula,"mm"),"width * 1.23456789","formula literals are not display numbers");
} finally {
  await server.close();
}
