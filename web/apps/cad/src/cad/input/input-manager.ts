import { InputStateTracker, modifiersFromEvent } from "./input-state";
import { InputResult, isHandled, type CadInputSink, type CadKeyboardEvent, type CadPointerEvent, type InputState } from "./input-types";

const POINTER_BUTTONS = [
  { key: "left", button: 0 },
  { key: "middle", button: 1 },
  { key: "right", button: 2 },
] as const;

export function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || Boolean(target.closest("input, textarea, select, [contenteditable='true']"));
}

export class InputManager {
  private readonly state = new InputStateTracker();
  private readonly capturedPointers = new Set<number>();
  private readonly expectedCaptureLoss = new Set<number>();
  private readonly subscribers = new Set<(state: InputState) => void>();
  private lastControlSignature = "";

  constructor(private readonly surface: HTMLElement, private readonly sink: CadInputSink) {
    surface.style.touchAction = "none";
    // Capture phase lets the CAD router claim a Middle-led chord before any
    // other canvas controller sees the same event.
    surface.addEventListener("pointerdown", this.onPointerDown, true);
    surface.addEventListener("pointermove", this.onPointerMove, true);
    surface.addEventListener("pointerup", this.onPointerUp, true);
    surface.addEventListener("pointercancel", this.onPointerCancel, true);
    surface.addEventListener("lostpointercapture", this.onLostPointerCapture);
    surface.addEventListener("wheel", this.onWheel, { passive: false });
    surface.addEventListener("contextmenu", this.preventSurfaceDefault);
    surface.addEventListener("dragstart", this.preventSurfaceDefault);
    surface.addEventListener("selectstart", this.preventSurfaceDefault);
    window.addEventListener("keydown", this.onKeyDown);
    window.addEventListener("keyup", this.onKeyUp);
    window.addEventListener("blur", this.onWindowBlur);
    document.addEventListener("visibilitychange", this.onVisibilityChange);
  }

  getState(): InputState { return this.state.snapshot(); }

  subscribe(listener: (state: InputState) => void): () => void {
    this.subscribers.add(listener);
    listener(this.getState());
    return () => this.subscribers.delete(listener);
  }

  dispose(): void {
    this.resetInput();
    this.surface.removeEventListener("pointerdown", this.onPointerDown, true);
    this.surface.removeEventListener("pointermove", this.onPointerMove, true);
    this.surface.removeEventListener("pointerup", this.onPointerUp, true);
    this.surface.removeEventListener("pointercancel", this.onPointerCancel, true);
    this.surface.removeEventListener("lostpointercapture", this.onLostPointerCapture);
    this.surface.removeEventListener("wheel", this.onWheel);
    this.surface.removeEventListener("contextmenu", this.preventSurfaceDefault);
    this.surface.removeEventListener("dragstart", this.preventSurfaceDefault);
    this.surface.removeEventListener("selectstart", this.preventSurfaceDefault);
    window.removeEventListener("keydown", this.onKeyDown);
    window.removeEventListener("keyup", this.onKeyUp);
    window.removeEventListener("blur", this.onWindowBlur);
    document.removeEventListener("visibilitychange", this.onVisibilityChange);
    this.subscribers.clear();
  }

  private localPoint(event: PointerEvent | WheelEvent): { x: number; y: number } {
    const bounds = this.surface.getBoundingClientRect();
    return { x: event.clientX - bounds.left, y: event.clientY - bounds.top };
  }

  private pointerEvent(event: PointerEvent, phase: CadPointerEvent["phase"]): CadPointerEvent {
    const point = this.localPoint(event);
    const state = this.state.updatePointer(point.x, point.y, event.buttons, modifiersFromEvent(event));
    this.publishControlState(state);
    return { phase, pointerId: event.pointerId, pointerType: event.pointerType, button: event.button,
      x: point.x, y: point.y, deltaX: state.pointer.deltaX, deltaY: state.pointer.deltaY, state, originalEvent: event };
  }

  private onPointerDown = (event: PointerEvent): void => {
    const normalized = this.pointerEvent(event, "down");
    const result = this.sink.pointerDown?.(normalized) ?? InputResult.Ignored;
    this.applyCaptureResult(event.pointerId, result);
    if (isHandled(result)) event.preventDefault();
    if (event.button === 1 || normalized.state.buttons.middle) event.stopImmediatePropagation();
  };

  private onPointerMove = (event: PointerEvent): void => {
    const previous = this.state.snapshot();
    const normalized = this.pointerEvent(event, "move");
    let handled = false;

    // Pointer Events only emit pointerdown for the first mouse button and
    // pointerup for the last one. Chorded Left/Middle/Right transitions arrive
    // as pointermove with a changed buttons mask, so synthesize semantic edges
    // for the CAD router while preserving the original PointerEvent.
    for (const descriptor of POINTER_BUTTONS) {
      if (!previous.buttons[descriptor.key] && normalized.state.buttons[descriptor.key]) {
        const result = this.sink.pointerDown?.({ ...normalized, phase: "down", button: descriptor.button })
          ?? InputResult.Ignored;
        this.applyCaptureResult(event.pointerId, result);
        handled ||= isHandled(result);
      }
    }
    for (const descriptor of POINTER_BUTTONS) {
      if (previous.buttons[descriptor.key] && !normalized.state.buttons[descriptor.key]) {
        const result = this.sink.pointerUp?.({ ...normalized, phase: "up", button: descriptor.button })
          ?? InputResult.Ignored;
        this.applyCaptureResult(event.pointerId, result);
        handled ||= isHandled(result);
      }
    }

    const moveResult = this.sink.pointerMove?.(normalized) ?? InputResult.Ignored;
    handled ||= isHandled(moveResult);
    if (handled) event.preventDefault();
    if (previous.buttons.middle || normalized.state.buttons.middle) event.stopImmediatePropagation();
  };

  private onPointerUp = (event: PointerEvent): void => {
    const result = this.sink.pointerUp?.(this.pointerEvent(event, "up")) ?? InputResult.Ignored;
    if (result === InputResult.ReleaseCapture || event.buttons === 0) this.releasePointer(event.pointerId);
    if (isHandled(result)) event.preventDefault();
    if (event.button === 1) event.stopImmediatePropagation();
  };

  private onPointerCancel = (event: PointerEvent): void => {
    this.sink.pointerCancel?.(this.pointerEvent(event, "cancel"));
    this.releasePointer(event.pointerId);
    this.resetInput();
    event.stopImmediatePropagation();
  };

  private onLostPointerCapture = (event: PointerEvent): void => {
    this.capturedPointers.delete(event.pointerId);
    if (this.expectedCaptureLoss.delete(event.pointerId)) return;
    this.sink.pointerCancel?.(this.pointerEvent(event, "cancel"));
    this.resetInput();
  };

  private onWheel = (event: WheelEvent): void => {
    const point = this.localPoint(event);
    const state = this.state.updatePointer(point.x, point.y, event.buttons, modifiersFromEvent(event));
    const result = this.sink.wheel?.({ x: point.x, y: point.y, deltaX: event.deltaX,
      deltaY: event.deltaY, state, originalEvent: event }) ?? InputResult.Ignored;
    if (isHandled(result)) event.preventDefault();
  };

  private onKeyDown = (event: KeyboardEvent): void => {
    const state = this.state.updateKey(event.code, true, modifiersFromEvent(event));
    this.publishControlState(state);
    const normalized: CadKeyboardEvent = { phase: "down", key: event.key, code: event.code, repeat: event.repeat,
      editableTarget: isEditableTarget(event.target), state, originalEvent: event };
    if (isHandled(this.sink.keyDown?.(normalized) ?? InputResult.Ignored)) event.preventDefault();
  };

  private onKeyUp = (event: KeyboardEvent): void => {
    const state = this.state.updateKey(event.code, false, modifiersFromEvent(event));
    this.publishControlState(state);
    const normalized: CadKeyboardEvent = { phase: "up", key: event.key, code: event.code, repeat: event.repeat,
      editableTarget: isEditableTarget(event.target), state, originalEvent: event };
    if (isHandled(this.sink.keyUp?.(normalized) ?? InputResult.Ignored)) event.preventDefault();
  };

  private onWindowBlur = (): void => this.resetInput();
  private onVisibilityChange = (): void => { if (document.visibilityState === "hidden") this.resetInput(); };
  private preventSurfaceDefault = (event: Event): void => event.preventDefault();

  private releasePointer(pointerId: number): void {
    if (!this.capturedPointers.delete(pointerId)) return;
    if (this.surface.hasPointerCapture(pointerId)) {
      this.expectedCaptureLoss.add(pointerId);
      this.surface.releasePointerCapture(pointerId);
    }
  }

  private applyCaptureResult(pointerId: number, result: InputResult): void {
    if (result === InputResult.ReleaseCapture) {
      this.releasePointer(pointerId);
      return;
    }
    if (result === InputResult.Capture && !this.capturedPointers.has(pointerId)) {
      this.surface.setPointerCapture(pointerId);
      this.capturedPointers.add(pointerId);
    }
  }

  private resetInput(): void {
    for (const pointerId of [...this.capturedPointers]) this.releasePointer(pointerId);
    this.sink.cancel?.();
    this.publishControlState(this.state.reset());
  }

  private publishControlState(state: InputState): void {
    const signature = `${Number(state.buttons.left)}${Number(state.buttons.middle)}${Number(state.buttons.right)}:` +
      `${Number(state.modifiers.ctrl)}${Number(state.modifiers.shift)}${Number(state.modifiers.alt)}${Number(state.modifiers.meta)}`;
    if (signature === this.lastControlSignature) return;
    this.lastControlSignature = signature;
    for (const subscriber of this.subscribers) subscriber(state);
  }
}
