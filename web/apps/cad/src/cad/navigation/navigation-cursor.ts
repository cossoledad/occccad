import type { NavigationSnapshot } from "./navigation-controller";
import { builtinIcons } from "../overlay/builtin-icons";
import { iconSvgElements } from "../overlay/icon-elements";
import { palette } from "../../design/visual-tokens";
const cursors = new Map<string, string>();
export function navigationCursor(snapshot: NavigationSnapshot, lightBackground = false): string {
  const action = snapshot.action;
  const reference = snapshot.solidworks?.reference;
  if (action === "none" && !reference) return "";
  const mode = action === "none" ? "orbit" : action;
  const key = action + Boolean(reference) + lightBackground;
  if (!cursors.has(key)) {
    const ink = lightBackground ? palette.text : palette.chromeText;
    const accent = lightBackground ? palette.primary : palette.axisZ;
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 20 20" color="${ink}"><g fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">${iconSvgElements(builtinIcons[`navigation-${mode}`],accent)}</g>${reference ? `<path d="M14 16h4m-2-2v4" fill="none" stroke="${lightBackground?palette.success:palette.axisY}" stroke-width="1.25" stroke-linecap="round"/>` : ''}</svg>`;
    const fallback=mode==='pan'?'move':mode==='zoom'?'ns-resize':action==='none'?'grab':'grabbing';
    cursors.set(key, `url("data:image/svg+xml,${encodeURIComponent(svg)}") 16 16, ${fallback}`);
  }
  return cursors.get(key)!;
}
