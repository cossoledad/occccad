/** UI precision only. Never use the formatted string for geometry or canonical identities. */
export const DISPLAY_DECIMAL_PLACES=2;
export function formatDisplayNumber(value:number,places=DISPLAY_DECIMAL_PLACES):string {
  if(!Number.isFinite(value))return "";
  const text=value.toFixed(Math.max(0,Math.min(DISPLAY_DECIMAL_PLACES,Math.trunc(places))));
  const trimmed=text.replace(/(\.\d*?[1-9])0+$|\.0+$/u,"$1");
  return trimmed==="-0"?"0":trimmed;
}
