import type { CadKeyboardEvent, CadPointerEvent } from "../input/input-types";
import { InputResult } from "../input/input-types";
import type { RectangleDraft, Vec2 } from "../../types";

export type ToolViewportPort = {
  sketchPoint(x: number, y: number): Vec2 | null;
  showRectanglePreview(start: Vec2, end: Vec2): void;
  clearToolPreview(): void;
  commitRectangle(draft: RectangleDraft): void;
  currentSketchPlane(): RectangleDraft["plane"] | undefined;
};

export type ToolContext = { viewport: ToolViewportPort };

export interface CadTool {
  readonly id: string;
  activate?(context: ToolContext): void;
  deactivate?(context: ToolContext): void;
  pointerDown?(event: CadPointerEvent, context: ToolContext): InputResult;
  pointerMove?(event: CadPointerEvent, context: ToolContext): InputResult;
  pointerUp?(event: CadPointerEvent, context: ToolContext): InputResult;
  pointerCancel?(event: CadPointerEvent, context: ToolContext): InputResult;
  keyDown?(event: CadKeyboardEvent, context: ToolContext): InputResult;
  keyUp?(event: CadKeyboardEvent, context: ToolContext): InputResult;
  cancel?(context: ToolContext): void;
}

export class SelectTool implements CadTool {
  readonly id = "select";
}

export class RectangleSketchTool implements CadTool {
  readonly id = "sketch.rectangle";
  private start?: Vec2;

  deactivate(context: ToolContext): void { this.cancel(context); }

  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || event.state.buttons.middle || event.state.buttons.right || !context.viewport.currentSketchPlane()) {
      return InputResult.Ignored;
    }
    const point = context.viewport.sketchPoint(event.x, event.y);
    if (!point) return InputResult.Ignored;
    this.start = point;
    context.viewport.showRectanglePreview(point, point);
    return InputResult.Capture;
  }

  pointerMove(event: CadPointerEvent, context: ToolContext): InputResult {
    if (!this.start || !event.state.buttons.left) return InputResult.Ignored;
    if (event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y);
    if (point) context.viewport.showRectanglePreview(this.start, point);
    return InputResult.Consumed;
  }

  pointerUp(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || !this.start) return InputResult.Ignored;
    if (event.state.buttons.middle || event.state.buttons.right) {
      this.cancel(context);
      return InputResult.Ignored;
    }
    const start = this.start;
    this.start = undefined;
    const end = context.viewport.sketchPoint(event.x, event.y);
    context.viewport.clearToolPreview();
    const plane = context.viewport.currentSketchPlane();
    if (!end || !plane) return InputResult.Consumed;
    const origin: Vec2 = [Math.min(start[0], end[0]), Math.min(start[1], end[1])];
    const width = Math.abs(end[0] - start[0]);
    const height = Math.abs(end[1] - start[1]);
    if (width >= 0.5 && height >= 0.5) context.viewport.commitRectangle({ plane, origin, width, height });
    return InputResult.Consumed;
  }

  pointerCancel(_event: CadPointerEvent, context: ToolContext): InputResult {
    if (!this.start) return InputResult.Ignored;
    this.cancel(context);
    return InputResult.Consumed;
  }

  cancel(context: ToolContext): void {
    this.start = undefined;
    context.viewport.clearToolPreview();
  }
}
