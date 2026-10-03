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
  private readonly owners=new Map<number,"tool"|"selection"|"navigation">();
  pointerDown(event:CadPointerEvent):InputResult {
    const navigationFirst=this.navigation.wantsPointerPriority(event);
    const previous=this.owners.get(event.pointerId);
    if(navigationFirst&&previous&&previous!=="navigation"){
      if(previous==="tool")this.tools.pointerCancel({...event,phase:"cancel"});this.selection.cancel();this.owners.delete(event.pointerId);
    }
    const candidates:Array<"tool"|"selection"|"navigation">=navigationFirst?["navigation","tool","selection"]:["tool","selection","navigation"];
    for(const owner of candidates){const result=owner==="tool"?this.tools.pointerDown(event):owner==="selection"?this.selection.pointerDown(event):this.navigation.pointerDown(event);
      if(isHandled(result)){this.owners.set(event.pointerId,owner);return result;}}
    return InputResult.Ignored;
  }
  pointerMove(event:CadPointerEvent):InputResult {
    const owner=this.owners.get(event.pointerId);
    const result=owner==="tool"?this.tools.pointerMove(event):owner==="selection"?this.selection.pointerMove(event):owner==="navigation"?this.navigation.pointerMove(event):
      this.navigation.wantsPointerPriority(event)?this.first(()=>this.navigation.pointerMove(event),()=>this.tools.pointerMove(event),()=>this.selection.pointerMove(event)):
      this.first(()=>this.tools.pointerMove(event),()=>this.selection.pointerMove(event),()=>this.navigation.pointerMove(event));
    // A preview may consume hover; pointer ownership can only begin on down.
    return result===InputResult.Capture?InputResult.Consumed:result;
  }
  pointerUp(event:CadPointerEvent):InputResult {
    const owner=this.owners.get(event.pointerId);if(!owner)return InputResult.Ignored;
    const result=owner==="tool"?this.tools.pointerUp(event):owner==="selection"?this.selection.pointerUp(event):this.navigation.pointerUp(event);
    if(!event.state.buttons.left&&!event.state.buttons.middle&&!event.state.buttons.right||result===InputResult.ReleaseCapture)this.owners.delete(event.pointerId);
    return result===InputResult.Ignored?InputResult.Consumed:result;
  }
  pointerCancel(event:CadPointerEvent):InputResult {
    const owner=this.owners.get(event.pointerId);this.owners.delete(event.pointerId);
    if(owner==="tool")this.tools.pointerCancel(event);if(owner==="selection")this.selection.cancel();if(owner==="navigation")this.navigation.cancel();return InputResult.Consumed;
  }
  auxiliaryClick(event: MouseEvent): InputResult { return this.navigation.auxiliaryClick(event); }
  wheel(event: CadWheelEvent): InputResult { return this.navigation.wheel(event); }
  keyDown(event: CadKeyboardEvent): InputResult { if(event.editableTarget||event.isComposing)return InputResult.Ignored;return this.first(() => this.navigation.keyChanged(event), () => this.selection.keyDown(event), () => this.tools.keyDown(event)); }
  keyUp(event: CadKeyboardEvent): InputResult { if(event.editableTarget||event.isComposing)return InputResult.Ignored;return this.first(() => this.navigation.keyChanged(event), () => this.tools.keyUp(event)); }
  cancel(): void { this.owners.clear();this.tools.cancel(); this.selection.cancel(); this.navigation.cancel(); }

  private first(...handlers: Array<() => InputResult>): InputResult {
    for (const handler of handlers) {
      const result = handler();
      if (isHandled(result)) return result;
    }
    return InputResult.Ignored;
  }
}
