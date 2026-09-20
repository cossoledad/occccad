/** Select once on focus; subsequent clicks still position the caret normally. */
export function installNumericInputSelection(root: Document): () => void {
  const numericInput = (target: EventTarget | null): target is HTMLInputElement =>
    target instanceof HTMLInputElement && !target.disabled && !target.readOnly &&
    (target.getAttribute("role") === "spinbutton" || target.type === "number" ||
      target.inputMode === "decimal" || target.dataset.quantityInput === "true");
  const focus = (event: FocusEvent) => { if (numericInput(event.target)) event.target.select(); };
  const pointer = (event: PointerEvent) => {
    if (event.button !== 0 || !numericInput(event.target) || root.activeElement === event.target) return;
    // Prevent the first click's native caret placement from collapsing selection.
    event.preventDefault();
    event.target.focus();
    event.target.select();
  };
  root.addEventListener("focusin", focus);
  root.addEventListener("pointerdown", pointer, true);
  return () => {
    root.removeEventListener("focusin", focus);
    root.removeEventListener("pointerdown", pointer, true);
  };
}
