import { DISPLAY_DECIMAL_PLACES, formatDisplayNumber } from "../../utils/display-number";
/** Presentation precision in the chosen display unit; never round stored Quantity. */
export const DIMENSION_DECIMAL_PLACES = DISPLAY_DECIMAL_PLACES;
export const formatDimensionValue = formatDisplayNumber;

/** Format numeric annotation text from display artifacts without touching names or expressions. */
export function formatDimensionLabelText(text:string):string {
  const match=text.match(/^((?:Δ[XY]|R|Ø|[ab])?\s*\(?)([+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?)(\)?(?:°|\s*(?:mm|cm|m|in|deg|rad))?)$/iu);
  return match?`${match[1]}${formatDisplayNumber(Number(match[2]))}${match[3]}`:text;
}

/** Only initial focus selects all: returning from the viewport preserves the caret. */
export function selectInitialDimensionValue<T extends { select(): void }>(input: T, session: { current: T | null }): void {
  if (session.current === input) return;
  input.select();
  session.current = input;
}
