import { commandShortcutLabel, commandShortcutAria } from "../command/command-shortcuts";
import { Button, Tooltip } from "antd";
import { useEffect, useImperativeHandle, useRef, type ReactNode, type Ref } from "react";
import { useCommandRegistry, useCommandState } from "../command/command-context";
import { useUIHelp } from "../help/ui-help-context";
import {useOperationFeedback} from "../command/operation-feedback";

/** Preserves the existing 220 ms single/double-click contract. */
export class ToolClickSequence {
  private timer?: ReturnType<typeof setTimeout>;
  constructor(private execute:(continuous:boolean)=>void) {}
  cancel = () => { clearTimeout(this.timer); this.timer=undefined; };
  click(repeatable:boolean) { this.cancel(); if(!repeatable)this.execute(false);else this.timer=setTimeout(()=>{this.timer=undefined;this.execute(false);},220); }
  doubleClick(repeatable:boolean) { if(!repeatable)return;this.cancel();this.execute(true); }
}
export type ToolButtonHandle = { cancelPending():void };
export type ToolButtonProps = {
  command:string;icon:ReactNode;tooltip:ReactNode;toolbarName?:string;helpText?:string;
  className?:string;repeatable?:boolean;showLabel?:boolean;label?:ReactNode;
  tooltipDisabled?:boolean;ref?:Ref<ToolButtonHandle>;
};
export function ToolButton({command,icon,tooltip,toolbarName="",helpText="",className="",repeatable=false,showLabel=false,label,tooltipDisabled=false,ref}:ToolButtonProps) {
  const registry=useCommandRegistry(),feedback=useOperationFeedback(),uiHelp=useUIHelp(),state=useCommandState(command);
  const execute=useRef((continuous:boolean)=>{void registry.execute(command,{continuous}).catch(error=>feedback(error,"命令"));});
  execute.current=continuous=>{void registry.execute(command,{continuous}).catch(error=>feedback(error,"命令"));};
  const sequence=useRef<ToolClickSequence>(null);
  if(!sequence.current)sequence.current=new ToolClickSequence(continuous=>execute.current(continuous));
  useImperativeHandle(ref,()=>({cancelPending:()=>sequence.current!.cancel()}),[]);
  useEffect(()=>()=>sequence.current!.cancel(),[command,state.enabled,state.visible,uiHelp.active]);
  if(!state.visible)return null;
  const shortcut=commandShortcutLabel(command);
  return <Tooltip title={<>{tooltip}{shortcut&&<kbd>{shortcut}</kbd>}</>} placement="bottom" mouseEnterDelay={.45} mouseLeaveDelay={.1}
    open={tooltipDisabled||uiHelp.active?false:undefined} classNames={{root:"cad-tool-tooltip"}} arrow={false}>
    <Button data-cad-tool-control="true" aria-keyshortcuts={commandShortcutAria(command)} className={`cad-tool-button ${showLabel?"with-label":""} ${state.active?"active":""} ${className}`.trim()}
      type="default" icon={icon} disabled={!state.enabled&&!uiHelp.active} aria-pressed={state.active}
      aria-label={typeof tooltip==="string"?tooltip.split(" · ")[0]:undefined}
      onClick={()=>{if(uiHelp.active){uiHelp.explain({toolbarName,commandName:String(tooltip),helpText});return;}sequence.current!.click(repeatable);}}
      onDoubleClick={()=>{if(!uiHelp.active)sequence.current!.doubleClick(repeatable);}}>
      {showLabel&&<span className="cad-tool-label">{label??tooltip}</span>}
    </Button>
  </Tooltip>;
}
