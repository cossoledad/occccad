import type { ParameterDefinition } from "../../types";

const unitScale: Record<string, number> = {
  mm: 1000, cm: 100, m: 1, in: 1 / 0.0254,
  deg: 180 / Math.PI, rad: 1,
};

export function parameterDisplayValue(parameter: ParameterDefinition): string {
  if (!parameter.evaluatedValue) return "—";
  return `${(parameter.evaluatedValue.siValue * (unitScale[parameter.displayUnit] ?? 1)).toPrecision(8)} ${parameter.displayUnit}`;
}

export function parameterSourceText(parameter: ParameterDefinition): string {
  if (parameter.source.expression) return parameter.source.expression.sourceText;
  if (!parameter.source.literal) return "";
  return `${parameter.source.literal.siValue * (unitScale[parameter.displayUnit] ?? 1)} ${parameter.displayUnit}`;
}

export function parseParameterSource(source: string):
  { kind: "LITERAL"; value: number; unit: string } | { kind: "EXPRESSION"; expression: string } {
  const normalized = source.trim();
  const literal = normalized.match(/^([+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?)\s*(mm|cm|m|in|deg|rad)$/i);
  if (literal) return { kind: "LITERAL", value: Number(literal[1]), unit: literal[2].toLowerCase() };
  return { kind: "EXPRESSION", expression: normalized };
}
