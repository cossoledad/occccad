import type { InputModifiers, InputState, PointerButtons } from "./input-types";

const buttonsFromMask = (mask: number): PointerButtons => ({
  left: Boolean(mask & 1),
  right: Boolean(mask & 2),
  middle: Boolean(mask & 4),
});

export class InputStateTracker {
  private x = 0;
  private y = 0;
  private deltaX = 0;
  private deltaY = 0;
  private buttons: PointerButtons = { left: false, middle: false, right: false };
  private modifiers: InputModifiers = { ctrl: false, shift: false, alt: false, meta: false };
  private readonly keys = new Set<string>();

  updatePointer(x: number, y: number, buttonsMask: number, modifiers: InputModifiers): InputState {
    this.deltaX = x - this.x;
    this.deltaY = y - this.y;
    this.x = x;
    this.y = y;
    this.buttons = buttonsFromMask(buttonsMask);
    this.modifiers = { ...modifiers };
    return this.snapshot();
  }

  updateKey(code: string, pressed: boolean, modifiers: InputModifiers): InputState {
    if (pressed) this.keys.add(code);
    else this.keys.delete(code);
    this.modifiers = { ...modifiers };
    this.deltaX = 0;
    this.deltaY = 0;
    return this.snapshot();
  }

  reset(): InputState {
    this.buttons = { left: false, middle: false, right: false };
    this.modifiers = { ctrl: false, shift: false, alt: false, meta: false };
    this.keys.clear();
    this.deltaX = 0;
    this.deltaY = 0;
    return this.snapshot();
  }

  snapshot(): InputState {
    return {
      pointer: { x: this.x, y: this.y, deltaX: this.deltaX, deltaY: this.deltaY },
      buttons: { ...this.buttons }, modifiers: { ...this.modifiers }, keys: new Set(this.keys),
    };
  }
}

export function modifiersFromEvent(event: Pick<MouseEvent, "ctrlKey" | "shiftKey" | "altKey" | "metaKey">): InputModifiers {
  return { ctrl: event.ctrlKey, shift: event.shiftKey, alt: event.altKey, meta: event.metaKey };
}

