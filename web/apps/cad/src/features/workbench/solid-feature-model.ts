import type { Feature } from "../../types";
import { linearExtrudeLengthInput, parseParameterSource } from "./parameter-editor";
const bodyFeatureTypes = new Set(["SOLID_PATTERN", "PAD", "LINEAR_EXTRUDE", "REVOLVE", "IMPORT_BODY", "BOOLEAN", "FILLET", "CHAMFER", "DRAFT", "SHELL", "LOFT"]);
export const solidFeatureNames: Record<string, string> = { FILLET: "圆角", CHAMFER: "倒角", DRAFT: "拔模", SHELL: "抽壳", LOFT: "放样", REVOLVE: "旋转", PAD: "拉伸", LINEAR_EXTRUDE: "拉伸" };
export function booleanInputStages(features: Feature[], editedId?: string): Feature[] {
    const boundary = editedId ? features.findIndex(feature => feature.id === editedId) : features.length;
    return features.slice(0, Math.max(0, boundary)).filter(feature => bodyFeatureTypes.has(feature.type.toUpperCase()) && !feature.suppressed);
}
export function booleanDefinitionReady(target: string | undefined, operation: Feature["operation"], tools: NonNullable<Feature["tools"]>, stages: Feature[]): boolean {
    return !!target && stages.some(feature => feature.bodyId === target) && tools.length > 0 &&
        new Set(tools.map(tool => tool.bodyId)).size === tools.length && (operation !== "INTERSECT" || tools.length === 1) &&
        tools.every(tool => tool.bodyId !== target && stages.some(feature => feature.id === tool.featureId && feature.bodyId === tool.bodyId));
}
// Unchanged text must preserve the original Quantity/expression, including digits
// beyond the display precision. A changed value uses the shared unit parser.
export function solidParameterEdit(slot: "length" | "length2" | "angle", text: string, initialText: string, unit: string): {
    value?: number;
    expression?: string;
} {
    if (text === initialText)
        return {};
    if (slot !== "angle") {
        const result = linearExtrudeLengthInput(text, unit);
        return { value: result.length, expression: result.lengthExpression };
    }
    const result = parseParameterSource(text, "deg");
    if (result.kind === "EXPRESSION") {
        if (!result.expression)
            throw new Error("请输入角度或表达式");
        return { expression: result.expression };
    }
    if (!["deg", "rad"].includes(result.unit))
        throw new Error("角度仅支持 deg 或 rad");
    const value = result.value * (result.unit === "rad" ? 180 / Math.PI : 1);
    if (!Number.isFinite(value) || value <= 0)
        throw new Error("角度必须大于 0");
    return { value };
}

export function solidGeneratorParameters(values: {generator: string; extent?: Feature["extent"]; angle: number | string; length2?: number | string}, unit: string) {
  const result: {angle?: number; length2?: number; parameterExpressions: Record<string,string>} = {parameterExpressions:{}};
  if(values.generator === "REVOLVE") {
    const input=solidParameterEdit("angle",String(values.angle),"",unit);
    result.angle=input.value??360;
    if(input.expression)result.parameterExpressions.angle=input.expression;
  } else if(values.extent === "TWO_SIDED") {
    const input=solidParameterEdit("length2",String(values.length2??10),"",unit);
    result.length2=input.value??10;
    if(input.expression)result.parameterExpressions.length2=input.expression;
  }
  return result;
}

// A spatial pattern is one Feature with explicitly addressed members. Picking
// it again adds its next available slot, never a duplicate implicit profile.
export function pickLoftSection(sections: NonNullable<Feature["sections"]>, source: Feature): NonNullable<Feature["sections"]> {
  if(source.type!=="SKETCH_PATTERN") return sections.some(s=>s.sketchId===source.id)?sections.filter(s=>s.sketchId!==source.id):[...sections,{sketchId:source.id}];
  const pattern=source.pattern;
  if(!pattern)return sections;
  const used=new Set(sections.filter(s=>s.sketchId===source.id).map(s=>s.memberSlot));
  for(let slot=0;slot<pattern.count;slot++)if(!used.has(slot)&&!pattern.skippedSlots?.includes(slot))return [...sections,{sketchId:source.id,memberSlot:slot}];
  return sections;
}
