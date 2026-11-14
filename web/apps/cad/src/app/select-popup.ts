import { Children, isValidElement, type CSSProperties, type ReactNode } from "react";
import type { SelectProps } from "antd";
import { uiTokens } from "../design/visual-tokens";

// Measure all options, including those outside the virtual list's rendered window.
// Keeping popupMatchSelectWidth enabled preserves Select's virtual scrolling.
function textOf(node: ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(textOf).join("");
  return isValidElement<{ children?: ReactNode }>(node) ? textOf(node.props.children) : "";
}

export function selectOptionLabels(props: Pick<SelectProps, "options" | "children" | "fieldNames" | "optionLabelProp">): string[] {
  const labels: string[] = [];
  const fields = props.fieldNames;
  if (props.options) {
    const visit = (options: NonNullable<SelectProps["options"]>) => {
      for (const option of options) {
        const children = option[fields?.options ?? "options"];
        const key = children ? fields?.groupLabel ?? fields?.label ?? "label" : fields?.label ?? "label";
        labels.push(textOf(option[key]) || String(option[fields?.value ?? "value"] ?? ""));
        if (Array.isArray(children)) visit(children);
      }
    };
    visit(props.options);
  } else {
    const visit = (children: ReactNode) => Children.forEach(children, child => {
      if (!isValidElement<{ label?: ReactNode; value?: ReactNode; children?: ReactNode; [key: string]: ReactNode }>(child)) return;
      const { props: option } = child;
      labels.push(textOf(option.label ?? option.children) || textOf(option.value));
      if (option.label !== undefined) visit(option.children);
    });
    visit(props.children);
  }
  return labels;
}

export function measureSelectOptions(props: Pick<SelectProps, "options" | "children" | "fieldNames" | "optionLabelProp">, measure: (text: string) => number): number {
  let width = 0;
  for (const label of selectOptionLabels(props)) for (const line of label.split(/\r?\n/)) width = Math.max(width, measure(line));
  return Math.ceil(width) + 76;
}

let context: CanvasRenderingContext2D | null | undefined;
let widths = new WeakMap<object, Map<string, number>>();
export function clearSelectMeasurements() { widths = new WeakMap(); }

export function selectPopupStyles(props: SelectProps): { popup: { root: CSSProperties } } {
  const font = `${props.style?.fontWeight ?? 400} ${props.style?.fontSize ?? uiTokens.fontSize}${typeof props.style?.fontSize === "string" ? "" : "px"} ${props.style?.fontFamily ?? uiTokens.fontFamily}`;
  const source = props.options ?? props.children;
  const key = `${font}|${JSON.stringify(props.fieldNames)}|${props.optionLabelProp ?? ""}`;
  const cacheable = source !== null && typeof source === "object";
  let width = cacheable ? widths.get(source)?.get(key) : undefined;
  if (width === undefined) {
    if (context === undefined && typeof document !== "undefined") context = document.createElement("canvas").getContext("2d");
    if (context) context.font = font;
    width = measureSelectOptions(props, text => context?.measureText(text).width ?? [...text].length * uiTokens.fontSize);
    if (cacheable) {
      if (!widths.has(source)) widths.set(source, new Map());
      widths.get(source)!.set(key, width);
    }
  }
  return { popup: { root: { minWidth: `min(${width}px, calc(100vw - 24px))`, maxWidth: "calc(100vw - 24px)" } } };
}
