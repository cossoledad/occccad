import { InputResult, type CadKeyboardEvent, type CadPointerEvent } from "../input/input-types";
import type { CadTool, ToolContext } from "./cad-tool";

export class ToolManager {
  private readonly tools = new Map<string, CadTool>();
  private active?: CadTool;
  private readonly listeners = new Set<(toolID: string | undefined) => void>();

  constructor(private readonly context: ToolContext) {}

  register(tool: CadTool): () => void {
    this.tools.set(tool.id, tool);
    return () => {
      if (this.active === tool) this.activate(undefined);
      if (this.tools.get(tool.id) === tool) this.tools.delete(tool.id);
    };
  }

  activate(toolID?: string): void {
    const next = toolID ? this.tools.get(toolID) : undefined;
    if (toolID && !next) throw new Error(`CAD tool is not registered: ${toolID}`);
    if (next === this.active) return;
    this.active?.cancel?.(this.context);
    this.active?.deactivate?.(this.context);
    this.active = next;
    this.active?.activate?.(this.context);
    for (const listener of this.listeners) listener(this.active?.id);
  }

  get activeToolID(): string | undefined { return this.active?.id; }
  subscribe(listener: (toolID: string | undefined) => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  pointerDown(event: CadPointerEvent): InputResult { return this.active?.pointerDown?.(event, this.context) ?? InputResult.Ignored; }
  pointerMove(event: CadPointerEvent): InputResult { return this.active?.pointerMove?.(event, this.context) ?? InputResult.Ignored; }
  pointerUp(event: CadPointerEvent): InputResult { return this.active?.pointerUp?.(event, this.context) ?? InputResult.Ignored; }
  pointerCancel(event: CadPointerEvent): InputResult { return this.active?.pointerCancel?.(event, this.context) ?? InputResult.Ignored; }
  keyDown(event: CadKeyboardEvent): InputResult {
    const result = this.active?.keyDown?.(event, this.context) ?? InputResult.Ignored;
    if (result !== InputResult.Ignored || event.key !== "Escape" || this.active?.id === "select") return result;
    this.activate("select");
    return InputResult.Consumed;
  }
  keyUp(event: CadKeyboardEvent): InputResult { return this.active?.keyUp?.(event, this.context) ?? InputResult.Ignored; }
  cancel(): void { this.active?.cancel?.(this.context); }
}
