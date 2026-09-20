import type { ParameterDefinition } from "../../types";

const unitScale: Record<string, number> = {
  mm: 1000, cm: 100, m: 1, in: 1 / 0.0254,
  deg: 180 / Math.PI, rad: 1,
};

export function parameterDisplayValue(parameter: ParameterDefinition, preferredLengthUnit?: "mm" | "cm" | "m" | "in"): string {
  if (!parameter.evaluatedValue) return "—";
  const unit = preferredLengthUnit && isLengthParameter(parameter) ? preferredLengthUnit : parameter.displayUnit;
  return `${(parameter.evaluatedValue.siValue * (unitScale[unit] ?? 1)).toPrecision(8)} ${unit}`;
}

export function parameterSourceText(parameter: ParameterDefinition, inputUnit?: string): string {
  if (parameter.source.expression) return parameter.source.expression.sourceText;
  if (parameter.source.external) return `Publication ${parameter.source.external.publicationId} @ ${parameter.source.external.resolvedRevisionId.slice(0, 12)}`;
  if (!parameter.source.literal) return "";
  const unit = inputUnit ?? parameter.displayUnit;
  const value = Number((parameter.source.literal.siValue * (unitScale[unit] ?? 1)).toPrecision(12));
  return inputUnit ? String(value) : `${value} ${unit}`;
}

export function parseParameterSource(source: string, defaultUnit?: string):
  { kind: "LITERAL"; value: number; unit: string } | { kind: "EXPRESSION"; expression: string } {
  const normalized = source.trim();
  const literal = normalized.match(/^([+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?)\s*(mm|cm|m|in|deg|rad)$/i);
  if (defaultUnit && /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?$/i.test(normalized)) {
    const value = Number(normalized);
    if (!Number.isFinite(value)) throw new Error("数值必须有限");
    return { kind: "LITERAL", value, unit: defaultUnit };
  }
  if (literal) return { kind: "LITERAL", value: Number(literal[1]), unit: literal[2].toLowerCase() };
  return { kind: "EXPRESSION", expression: normalized };
}

export function linearExtrudeLengthInput(source: string, defaultUnit = "mm"): { length?: number; lengthExpression?: string } {
  const parsed = parseParameterSource(source, defaultUnit);
  if (parsed.kind === "EXPRESSION") {
    if (!parsed.expression) throw new Error("请输入长度值、参数别名或表达式");
    return { lengthExpression: parsed.expression };
  }
  if (!["mm", "cm", "m", "in"].includes(parsed.unit)) throw new Error("拉伸长度仅支持 mm、cm、m 或 in");
  const millimeters = parsed.value * ({ mm: 1, cm: 10, m: 1000, in: 25.4 }[parsed.unit] ?? 1);
  if (!Number.isFinite(millimeters) || millimeters <= 0) throw new Error("拉伸长度必须大于 0");
  return { length: millimeters };
}

export function isLengthParameter(parameter: ParameterDefinition): boolean {
  const dimension = parameter.dimension;
  return dimension.Length === 1 && dimension.Mass === 0 && dimension.Time === 0 && dimension.Current === 0 &&
    dimension.Temperature === 0 && dimension.Amount === 0 && dimension.Luminous === 0 && !dimension.Semantic;
}
