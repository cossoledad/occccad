import { InputResult, type CadPointerEvent } from "../input/input-types";

export class SelectionController {
  private down?: { x: number; y: number };

  constructor(private readonly pick: (x: number, y: number) => void) {}

  pointerDown(event: CadPointerEvent): InputResult {
    if (event.button !== 0 || event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    this.down = { x: event.x, y: event.y };
    return InputResult.Consumed;
  }

  pointerMove(event: CadPointerEvent): InputResult {
    if (event.state.buttons.middle || event.state.buttons.right) { this.down = undefined; return InputResult.Ignored; }
    return this.down && event.state.buttons.left ? InputResult.Consumed : InputResult.Ignored;
  }

  pointerUp(event: CadPointerEvent): InputResult {
    if (event.button !== 0 || !this.down) return InputResult.Ignored;
    if (event.state.buttons.middle || event.state.buttons.right) { this.down = undefined; return InputResult.Ignored; }
    const distance = Math.hypot(event.x - this.down.x, event.y - this.down.y);
    this.down = undefined;
    if (distance <= 4) this.pick(event.x, event.y);
    return InputResult.Consumed;
  }

  cancel(): void { this.down = undefined; }
}
