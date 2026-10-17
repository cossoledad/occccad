import type { NavigationSnapshot } from "./navigation-controller";
const paths = {
  orbit: '<ellipse cx="16" cy="16" rx="12" ry="6"/><ellipse cx="16" cy="16" rx="6" ry="12"/><path d="M5 9V4h5M5 4a15 15 0 0 1 24 12"/>',
  pan: '<path d="M14 3v22M3 14h22M10 7l4-4 4 4M10 21l4 4 4-4M7 10l-4 4 4 4M21 10l4 4-4 4"/>',
  zoom: '<circle cx="11" cy="11" r="7"/><path d="m16 16 9 9M8 11h6M11 8v6"/>',
  roll: '<path d="M6 10a9 9 0 1 1 0 9M6 3v7h7"/>',
};
const cursors = new Map<string, string>();
export function navigationCursor(snapshot: NavigationSnapshot): string {
  const action = snapshot.action;
  const reference = snapshot.solidworks?.reference;
  if (action === "none" && !reference) return "";
  const mode = action === "none" ? "orbit" : action;
  const key = mode + Boolean(reference);
  if (!cursors.has(key)) {
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32"><g fill="none" stroke="white" stroke-width="4">${paths[mode]}</g><g fill="none" stroke="#253b50" stroke-width="1.5">${paths[mode]}</g>${reference ? '<path d="M3 29 29 3" stroke="#54b66c" stroke-width="2"/>' : ''}</svg>`;
    cursors.set(key, `url("data:image/svg+xml,${encodeURIComponent(svg)}") 16 16, move`);
  }
  return cursors.get(key)!;
}
