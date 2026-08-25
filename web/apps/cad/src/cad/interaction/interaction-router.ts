import { InputResult, isHandled, type CadInputSink, type CadKeyboardEvent, type CadPointerEvent, type CadWheelEvent } from "../input/input-types";
import type { NavigationController } from "../navigation/navigation-controller";
import type { ToolManager } from "../tool/tool-manager";
import type { SelectionController } from "./selection-controller";

export class InteractionRouter implements CadInputSink {
  constructor(
    private readonly tools: ToolManager,
    private readonly selection: SelectionController,
    private readonly navigation: NavigationController,
  ) {}

  // Middle-led gestures are routed to navigation first. This guarantees that an
  // auxiliary Left/Right edge in a CATIA chord cannot also draw/select/open UI.
  // Without a navigation chord the active tool and click selection keep priority.
  pointerDown(event: CadPointerEvent): InputResult {
    if (this.navigation.wantsPointerPriority(event)) {
      return this.first(() => this.navigation.pointerDown(event), () => this.tools.pointerDown(event), () => this.selection.pointerDown(event));
    }
    return this.first(() => this.tools.pointerDown(event), () => this.selection.pointerDown(event), () => this.navigation.pointerDown(event));
  }
  pointerMove(event: CadPointerEvent): InputResult {
    if (this.navigation.wantsPointerPriority(event)) {
      return this.first(() => this.navigation.pointerMove(event), () => this.tools.pointerMove(event), () => this.selection.pointerMove(event));
    }
    return this.first(() => this.tools.pointerMove(event), () => this.selection.pointerMove(event), () => this.navigation.pointerMove(event));
  }
  pointerUp(event: CadPointerEvent): InputResult {
    if (this.navigation.wantsPointerPriority(event)) {
      return this.first(() => this.navigation.pointerUp(event), () => this.tools.pointerUp(event), () => this.selection.pointerUp(event));
    }
    return this.first(() => this.tools.pointerUp(event), () => this.selection.pointerUp(event), () => this.navigation.pointerUp(event));
  }
  pointerCancel(event: CadPointerEvent): InputResult {
    this.tools.pointerCancel(event); this.selection.cancel(); this.navigation.cancel();
    return InputResult.Consumed;
  }
  wheel(event: CadWheelEvent): InputResult { return this.navigation.wheel(event); }
  keyDown(event: CadKeyboardEvent): InputResult { return this.tools.keyDown(event); }
  keyUp(event: CadKeyboardEvent): InputResult { return this.tools.keyUp(event); }
  cancel(): void { this.tools.cancel(); this.selection.cancel(); this.navigation.cancel(); }

  private first(...handlers: Array<() => InputResult>): InputResult {
    for (const handler of handlers) {
      const result = handler();
      if (isHandled(result)) return result;
    }
    return InputResult.Ignored;
  }
}
