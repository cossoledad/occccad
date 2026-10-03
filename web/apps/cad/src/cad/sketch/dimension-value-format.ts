/** Presentation precision in the chosen display unit; never round stored Quantity. */
export const DIMENSION_DECIMAL_PLACES = 6;

export function formatDimensionValue(value: number): string {
  if (!Number.isFinite(value)) return "";
  const rounded = Number(value.toFixed(DIMENSION_DECIMAL_PLACES));
  // Preserve meaningful tiny values instead of displaying a false zero.
  return rounded === 0 && value !== 0
    ? Number(value.toPrecision(DIMENSION_DECIMAL_PLACES)).toString()
    : rounded.toString();
}

/** Only initial focus selects all: returning from the viewport preserves the caret. */
export function selectInitialDimensionValue<T extends { select(): void }>(input: T, session: { current: T | null }): void {
  if (session.current === input) return;
  input.select();
  session.current = input;
}
