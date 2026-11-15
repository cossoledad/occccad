import type { IconDefinition } from "../command/workbench-catalog";

export const iconTags = new Set(["path", "circle", "rect", "line", "polyline", "polygon", "ellipse"]);
export const iconAttributes = new Set(["d", "cx", "cy", "r", "x", "y", "width", "height", "rx", "ry", "x1", "x2", "y1", "y2", "points", "className"]);
const escapeAttribute = (value: string) => value.replaceAll("&", "&amp;").replaceAll('"', "&quot;").replaceAll("<", "&lt;");

/** Code-native cursor adapter for the same allowlisted 20px glyphs as CadIcon.
 * CSS roles become explicit SVG paint because cursor images have no app CSS. */
export function iconSvgElements(icon: IconDefinition, accent: string): string {
  return (icon.elements ?? []).filter(e => iconTags.has(e.tag)).map(e => {
    const roles = (e.attributes.className ?? "").split(/\s+/);
    const props = Object.entries(e.attributes).filter(([key]) => iconAttributes.has(key) && key !== "className")
      .map(([key,value]) => `${key}="${escapeAttribute(value)}"`).join(" ");
    const paint = roles.includes("icon-operation") ? `stroke="${accent}" stroke-width="1.7"`
      : roles.includes("icon-node") ? `fill="${accent}" stroke="none"`
      : roles.includes("icon-face") ? 'fill="currentColor" fill-opacity=".1" stroke="none"'
      : roles.includes("icon-guide") ? 'opacity=".48" stroke-dasharray="2 2"' : "";
    return `<${e.tag} ${props} ${paint}/>`;
  }).join("");
}
