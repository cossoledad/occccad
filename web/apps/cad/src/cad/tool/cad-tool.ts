import type { CadKeyboardEvent, CadPointerEvent } from "../input/input-types";
import { InputResult } from "../input/input-types";
import type { SketchGeometryRef, SketchOperation, Vec2 } from "../../types";

export type ToolViewportPort = {
  sketchPoint(x: number, y: number): Vec2 | null;
  showPolylinePreview(points: Vec2[], closed?: boolean): void;
  showPointPreview(point: Vec2): void;
  clearToolPreview(): void;
  commitSketchOperations(operations: SketchOperation[]): void;
  hasActiveSketch(): boolean;
  sketchReferenceAt(x: number, y: number, kind: "COINCIDENT" | "PARALLEL"): SketchGeometryRef | null;
  showReferencePreview(reference: SketchGeometryRef): void;
  clearReferencePreview(): void;
  setToolPrompt(prompt: string): void;
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
  activate(context: ToolContext): void { context.viewport.setToolPrompt("选择：选择草图元素，或从工具栏启动创建命令"); }
}

abstract class TwoClickSketchTool implements CadTool {
  abstract readonly id: string;
  protected first?: Vec2;
  private capturedPointerID?: number;
  abstract preview(first: Vec2, second: Vec2, context: ToolContext): void;
  abstract commit(first: Vec2, second: Vec2, context: ToolContext): void;
  abstract readonly firstPrompt: string;
  abstract readonly secondPrompt: string;

  activate(context: ToolContext): void { context.viewport.setToolPrompt(this.firstPrompt); }

  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || this.capturedPointerID !== undefined || event.state.buttons.middle || event.state.buttons.right || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y);
    if (!point) return InputResult.Ignored;
    this.capturedPointerID = event.pointerId;
    if (!this.first) {
      this.first = point;
      this.preview(point, point, context);
      context.viewport.setToolPrompt(this.secondPrompt);
      return InputResult.Capture;
    }
    const first = this.first; this.first = undefined; context.viewport.clearToolPreview();
    if (Math.hypot(point[0] - first[0], point[1] - first[1]) >= 0.5) this.commit(first, point, context);
    context.viewport.setToolPrompt(this.firstPrompt);
    return InputResult.Capture;
  }

  pointerUp(event: CadPointerEvent): InputResult {
    if (event.button !== 0 || event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined;
    return InputResult.ReleaseCapture;
  }

  pointerCancel(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined;
    this.cancel(context);
    return InputResult.Consumed;
  }

  pointerMove(event: CadPointerEvent, context: ToolContext): InputResult {
    if (!this.first || event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y);
    if (point) this.preview(this.first, point, context);
    return InputResult.Consumed;
  }

  keyDown(event: CadKeyboardEvent, context: ToolContext): InputResult {
    if (event.key !== "Escape" || !this.first) return InputResult.Ignored;
    this.cancel(context);
    return InputResult.Consumed;
  }
  deactivate(context: ToolContext): void { this.cancel(context); }
  cancel(context: ToolContext): void {
    this.capturedPointerID = undefined;
    this.first = undefined;
    context.viewport.clearToolPreview();
    context.viewport.setToolPrompt(this.firstPrompt);
  }
}

export class LineSketchTool extends TwoClickSketchTool {
  readonly id = "sketch.line";
  readonly firstPrompt = "直线：单击起点";
  readonly secondPrompt = "直线：移动预览，单击终点；Esc 取消当前线";
  preview(first: Vec2, second: Vec2, context: ToolContext): void { context.viewport.showPolylinePreview([first, second]); }
  commit(first: Vec2, second: Vec2, context: ToolContext): void {
    context.viewport.commitSketchOperations([{ type: "ADD_ENTITY", entity: { id: crypto.randomUUID(), kind: "LINE", role: "PROFILE", start: { x: first[0], y: first[1] }, end: { x: second[0], y: second[1] } } }]);
  }
}

export class RectangleSketchTool extends TwoClickSketchTool {
  readonly id = "sketch.rectangle";
  readonly firstPrompt = "矩形：单击第一个角点";
  readonly secondPrompt = "矩形：移动预览，单击对角点；Esc 取消当前矩形";
  preview(first: Vec2, second: Vec2, context: ToolContext): void {
    // The rectangle macro has a deterministic local solution on every pointer
    // move; the authoritative PlaneGCS solve still happens at commit.
    context.viewport.showPolylinePreview([first, [second[0], first[1]], second, [first[0], second[1]]], true);
  }
  commit(first: Vec2, second: Vec2, context: ToolContext): void {
    context.viewport.commitSketchOperations([{ type: "ADD_RECTANGLE", first: { x: first[0], y: first[1] }, second: { x: second[0], y: second[1] } }]);
  }
}

export class PointSketchTool implements CadTool {
  readonly id = "sketch.point";
  private capturedPointerID?: number;
  activate(context: ToolContext): void { context.viewport.setToolPrompt("点：单击放置；Esc 返回选择"); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || this.capturedPointerID !== undefined || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y); if (!point) return InputResult.Ignored;
    this.capturedPointerID = event.pointerId;
    context.viewport.commitSketchOperations([{ type: "ADD_ENTITY", entity: { id: crypto.randomUUID(), kind: "POINT", role: "PROFILE", point: { x: point[0], y: point[1] } } }]);
    return InputResult.Capture;
  }
  pointerUp(event: CadPointerEvent): InputResult {
    if (event.button !== 0 || event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined;
    return InputResult.ReleaseCapture;
  }
  pointerCancel(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined;
    this.cancel(context);
    return InputResult.Consumed;
  }
  pointerMove(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.state.buttons.middle || event.state.buttons.right || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    const point = context.viewport.sketchPoint(event.x, event.y); if (!point) return InputResult.Ignored;
    context.viewport.showPointPreview(point);
    return InputResult.Consumed;
  }
  deactivate(context: ToolContext): void { this.cancel(context); }
  cancel(context: ToolContext): void { this.capturedPointerID = undefined; context.viewport.clearToolPreview(); }
}

// Constraint tools intentionally share the same tool lifecycle now. Entity
// reference picking is the next extension point; commands stay typed and no
// topology or array index is persisted.
export class ConstraintSketchTool implements CadTool {
  private first?: SketchGeometryRef;
  private capturedPointerID?: number;
  readonly id: "sketch.constraint.coincident" | "sketch.constraint.parallel";
  constructor(id: "sketch.constraint.coincident" | "sketch.constraint.parallel") { this.id = id; }
  private get kind(): "COINCIDENT" | "PARALLEL" { return this.id.endsWith("parallel") ? "PARALLEL" : "COINCIDENT"; }
  private firstPrompt(): string { return this.kind === "PARALLEL" ? "平行约束：选择第一条线" : "重合约束：选择第一个端点"; }
  private secondPrompt(): string { return this.kind === "PARALLEL" ? "平行约束：选择第二条线；Esc 取消选择" : "重合约束：选择第二个端点；Esc 取消选择"; }
  activate(context: ToolContext): void { context.viewport.setToolPrompt(this.firstPrompt()); }
  pointerDown(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.button !== 0 || this.capturedPointerID !== undefined || !context.viewport.hasActiveSketch()) return InputResult.Ignored;
    const kind = this.kind;
    const reference = context.viewport.sketchReferenceAt(event.x, event.y, kind); if (!reference) return InputResult.Ignored;
    this.capturedPointerID = event.pointerId;
    if (!this.first) {
      this.first = reference;
      context.viewport.showReferencePreview(reference);
      context.viewport.setToolPrompt(this.secondPrompt());
      return InputResult.Capture;
    }
    const first = this.first; this.first = undefined;
    context.viewport.clearReferencePreview();
    context.viewport.commitSketchOperations([{ type: "ADD_CONSTRAINT", constraint: { id: crypto.randomUUID(), kind, references: [first, reference] } }]);
    context.viewport.setToolPrompt(this.firstPrompt());
    return InputResult.Capture;
  }
  pointerUp(event: CadPointerEvent): InputResult {
    if (event.button !== 0 || event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined;
    return InputResult.ReleaseCapture;
  }
  pointerCancel(event: CadPointerEvent, context: ToolContext): InputResult {
    if (event.pointerId !== this.capturedPointerID) return InputResult.Ignored;
    this.capturedPointerID = undefined;
    this.cancel(context);
    return InputResult.Consumed;
  }
  pointerMove(event: CadPointerEvent, context: ToolContext): InputResult {
    if (this.first || event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    const reference = context.viewport.sketchReferenceAt(event.x, event.y, this.kind);
    if (!reference) { context.viewport.clearReferencePreview(); return InputResult.Ignored; }
    context.viewport.showReferencePreview(reference);
    return InputResult.Consumed;
  }
  keyDown(event: CadKeyboardEvent, context: ToolContext): InputResult {
    if (event.key !== "Escape" || !this.first) return InputResult.Ignored;
    this.cancel(context);
    return InputResult.Consumed;
  }
  deactivate(context: ToolContext): void { this.cancel(context); }
  cancel(context: ToolContext): void {
    this.capturedPointerID = undefined;
    this.first = undefined;
    context.viewport.clearReferencePreview();
    context.viewport.setToolPrompt(this.firstPrompt());
  }
}
