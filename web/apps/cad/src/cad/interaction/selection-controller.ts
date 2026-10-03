import { InputResult, type CadPointerEvent, type CadKeyboardEvent } from "../input/input-types";

export class SelectionController {
  private down?: { x: number; y: number; additive: boolean; box: boolean };

  constructor(private readonly pick: (x: number, y: number, additive: boolean) => void,
    private readonly preselect: (x: number, y: number) => void,
    private readonly clearPreselection: () => void,
    private readonly marquee?: { enabled:()=>boolean; preview:(from:{x:number;y:number},to:{x:number;y:number})=>void; clear:()=>void; select:(from:{x:number;y:number},to:{x:number;y:number},additive:boolean)=>void }) {}

  pointerDown(event: CadPointerEvent): InputResult {
    if (event.button !== 0 || event.state.buttons.middle || event.state.buttons.right) return InputResult.Ignored;
    const box=this.marquee?.enabled()??false;
    this.down = { x: event.x, y: event.y, additive:event.state.modifiers.ctrl||event.state.modifiers.meta, box };
    return box?InputResult.Capture:InputResult.Consumed;
  }

  pointerMove(event: CadPointerEvent): InputResult {
    if (event.state.buttons.middle || event.state.buttons.right) {
      this.down = undefined;
      this.marquee?.clear();
      this.clearPreselection();
      return InputResult.Ignored;
    }
    if (!event.state.buttons.left) this.preselect(event.x, event.y);
    if(this.down?.box&&event.state.buttons.left&&Math.hypot(event.x-this.down.x,event.y-this.down.y)>4){this.clearPreselection();this.marquee?.preview(this.down,{x:event.x,y:event.y});}
    return this.down && event.state.buttons.left ? InputResult.Consumed : InputResult.Ignored;
  }

  pointerUp(event: CadPointerEvent): InputResult {
    if (event.button !== 0 || !this.down) return InputResult.Ignored;
    if (event.state.buttons.middle || event.state.buttons.right) { this.cancel(); return InputResult.Ignored; }
    const distance = Math.hypot(event.x - this.down.x, event.y - this.down.y);
    const down=this.down;this.down = undefined;this.marquee?.clear();
    if (distance <= 4) this.pick(event.x, event.y, event.state.modifiers.ctrl || event.state.modifiers.meta);
    else if(down.box)this.marquee?.select(down,{x:event.x,y:event.y},down.additive);
    return down.box?InputResult.ReleaseCapture:InputResult.Consumed;
  }

  cancel(): void { this.down = undefined; this.marquee?.clear();this.clearPreselection(); }
  keyDown(event:CadKeyboardEvent):InputResult {
    if(event.key!=="Escape"||!this.down)return InputResult.Ignored;
    this.cancel();return InputResult.ReleaseCapture;
  }
}
