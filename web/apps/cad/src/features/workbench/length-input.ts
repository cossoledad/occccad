export type LengthInput = { value: number; unit: "mm" | "cm" | "m" | "in" };

const lengthPattern = /^\s*([+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?)\s*(mm|cm|m|in)?\s*$/i;

export function parseLengthInput(text: string, defaultUnit: LengthInput["unit"] = "mm"): LengthInput {
  const match = lengthPattern.exec(text);
  if (!match) throw new Error("请输入正数长度，例如 40；也可显式输入 4 cm 或 0.04 m");
  const value = Number(match[1]);
  if (!Number.isFinite(value) || value <= 0) throw new Error("拉伸长度必须是有限正数");
  return { value, unit: (match[2]?.toLowerCase() ?? defaultUnit) as LengthInput["unit"] };
}
