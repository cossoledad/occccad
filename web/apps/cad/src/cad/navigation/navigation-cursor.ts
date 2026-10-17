import type { NavigationSnapshot } from "./navigation-controller";
const paths = {
  orbit: '<ellipse cx="16" cy="16" rx="10.5" ry="5.5"/><ellipse cx="16" cy="16" rx="5.5" ry="10.5"/><path d="M3 16A13 13 0 1 1 25.2 25.2M29.2 23.5 25.2 25.2 26.8 21"/>',
  pan: '<path d="M16 5v22M5 16h22M12 9l4-4 4 4M12 23l4 4 4-4M9 12l-4 4 4 4M23 12l4 4-4 4"/>',
  zoom: '<circle cx="16" cy="16" r="8"/><path d="m21.7 21.7 7.3 7.3M13 16h6M16 13v6"/>',
  roll: '<path d="M6 10a9 9 0 1 1 0 9M6 3v7h7"/>',
};
const cursors = new Map<string, string>();
export function navigationCursor(snapshot: NavigationSnapshot, lightBackground = false): string {
  const action = snapshot.action;
  const reference = snapshot.solidworks?.reference;
  if (action === "none" && !reference) return "";
  const mode = action === "none" ? "orbit" : action;
  const key = mode + Boolean(reference) + lightBackground;
  if (!cursors.has(key)) {
    const ink = lightBackground ? "#253b50" : "#f3bd71";
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32"><g fill="none" stroke="${ink}" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">${paths[mode]}</g>${reference ? '<path d="M3 29 29 3" stroke="#54b66c" stroke-width="2"/>' : ''}</svg>`;
    cursors.set(key, `url("data:image/svg+xml,${encodeURIComponent(svg)}") 16 16, move`);
  }
  return cursors.get(key)!;
}
