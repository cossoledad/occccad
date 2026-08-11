export enum InputResult {
  Ignored = "ignored",
  Consumed = "consumed",
  Capture = "capture",
  ReleaseCapture = "release-capture",
}

export type PointerButtons = { left: boolean; middle: boolean; right: boolean };
export type InputModifiers = { ctrl: boolean; shift: boolean; alt: boolean; meta: boolean };

export type InputState = {
  pointer: { x: number; y: number; deltaX: number; deltaY: number };
  buttons: PointerButtons;
  modifiers: InputModifiers;
  keys: ReadonlySet<string>;
};

export type CadPointerEvent = {
  phase: "down" | "move" | "up" | "cancel";
  pointerId: number;
  pointerType: string;
  button: number;
  x: number;
  y: number;
  deltaX: number;
  deltaY: number;
  state: InputState;
  originalEvent: PointerEvent;
};

export type CadWheelEvent = {
  x: number;
  y: number;
  deltaX: number;
  deltaY: number;
  state: InputState;
  originalEvent: WheelEvent;
};

export type CadKeyboardEvent = {
  phase: "down" | "up";
  key: string;
  code: string;
  repeat: boolean;
  editableTarget: boolean;
  state: InputState;
  originalEvent: KeyboardEvent;
};

export interface CadInputSink {
  pointerDown?(event: CadPointerEvent): InputResult;
  pointerMove?(event: CadPointerEvent): InputResult;
  pointerUp?(event: CadPointerEvent): InputResult;
  pointerCancel?(event: CadPointerEvent): InputResult;
  wheel?(event: CadWheelEvent): InputResult;
  keyDown?(event: CadKeyboardEvent): InputResult;
  keyUp?(event: CadKeyboardEvent): InputResult;
  cancel?(): void;
}

export const isHandled = (result: InputResult): boolean => result !== InputResult.Ignored;
