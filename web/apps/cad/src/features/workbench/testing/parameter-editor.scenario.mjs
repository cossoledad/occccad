import assert from "node:assert/strict";
import { createServer } from "vite";

const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const { isLengthParameter, linearExtrudeLengthInput, parameterDisplayValue, parameterSourceText, parseParameterSource } = await server.ssrLoadModule("/src/features/workbench/parameter-editor.ts");
  const parameter = {
    parameterId: "parameter:sketch-a:constraint:length:value", key: "base_width", label: "LENGTH",
    valueType: "QUANTITY", dimension: { Length: 1, Mass: 0, Time: 0, Current: 0, Temperature: 0, Amount: 0, Luminous: 0, Semantic: "" },
    displayUnit: "mm", role: "INPUT", source: { literal: { siValue: 0.02, dimension: { Length: 1 } } },
    evaluatedValue: { siValue: 0.02, dimension: { Length: 1 } },
  };
  assert.equal(parameterSourceText(parameter), "20 mm");
  assert.equal(parameterDisplayValue(parameter), "20.000000 mm");
  assert.deepEqual(parseParameterSource("40 mm"), { kind: "LITERAL", value: 40, unit: "mm" });
  assert.deepEqual(parseParameterSource("base_width / 2"), { kind: "EXPRESSION", expression: "base_width / 2" });
  assert.deepEqual(linearExtrudeLengthInput("2 cm"), { length: 20 });
  assert.deepEqual(linearExtrudeLengthInput("base_width / 2"), { lengthExpression: "base_width / 2" });
  assert.equal(isLengthParameter(parameter), true);
  assert.throws(() => linearExtrudeLengthInput("90 deg"), /mm、cm、m 或 in/);
  assert.equal(parameterSourceText({ ...parameter, source: { expression: { sourceText: "base_width / 2" } } }), "base_width / 2");
  assert.equal(parameterSourceText({ ...parameter, source: { external: { publicationId: "publication-width",
    resolvedRevisionId: "revision-1234567890" } } }), "Publication publication-width @ revision-123");
} finally {
  await server.close();
}
