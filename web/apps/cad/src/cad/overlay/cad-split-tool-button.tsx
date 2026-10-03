import { CheckOutlined, DownOutlined } from "@ant-design/icons";
import { Button, Dropdown, Space } from "antd";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import type { ToolbarCatalogItem } from "../../types";
import { useCommandRegistry } from "../command/command-context";
import { commandShortcutLabel } from "../command/command-shortcuts";
import { useOperationFeedback } from "../command/operation-feedback";
import { useUIHelp } from "../help/ui-help-context";
import { CadIcon, type CadIconName } from "./cad-icons";
import { ToolButton, type ToolButtonHandle } from "./tool-button";

export function CadSplitToolButton({items,defaultId,onDefaultChange,familyName,toolbarName}:{
  items:ToolbarCatalogItem[];defaultId:string;onDefaultChange:(id:string)=>void;familyName:string;toolbarName:string;
}) {
  const registry=useCommandRegistry(),help=useUIHelp(),feedback=useOperationFeedback();
  useSyncExternalStore(registry.subscribe,registry.getSnapshot,registry.getSnapshot);
  const [open,setOpen]=useState(false),main=useRef<ToolButtonHandle>(null),arrow=useRef<HTMLButtonElement>(null),popup=useRef<HTMLDivElement>(null);
  const item=items.find(choice=>registry.state(choice.commandId).active)??items.find(choice=>choice.commandId===defaultId)??items[0];
  const active=registry.state(item.commandId).active;
  const available=help.active||items.some(choice=>registry.state(choice.commandId).enabled);
  const changeOpen=(value:boolean)=>{main.current?.cancelPending();setOpen(value);};
  useEffect(()=>{
    if(!open)return;
    // Canvas consumes pointer events before they bubble to Antd's outside-click handler.
    const dismiss=(event:PointerEvent)=>{
      const target=event.target;
      if(!(target instanceof Node)||arrow.current?.closest(".cad-split-tool-button")?.contains(target)||popup.current?.contains(target))return;
      main.current?.cancelPending();setOpen(false);
    };
    document.addEventListener("pointerdown",dismiss,true);
    return ()=>document.removeEventListener("pointerdown",dismiss,true);
  },[open]);
  return <Space.Compact className={`cad-split-tool-button${active?" active":""}${open?" expanded":""}`}>
    <ToolButton ref={main} command={item.commandId} repeatable={item.repeatable} showLabel icon={<CadIcon name={item.iconKey as CadIconName}/>}
      tooltip={item.name} toolbarName={toolbarName} helpText={item.helpText} tooltipDisabled={open}/>
    <Dropdown open={open} onOpenChange={changeOpen} trigger={["click"]} placement="bottomLeft" arrow={false}
      classNames={{root:"cad-command-menu"}} autoAdjustOverflow
      popupRender={menu=><div ref={popup} onPointerDown={e=>e.stopPropagation()} onWheel={e=>e.stopPropagation()}
        onKeyDown={e=>{if(e.key==="Escape"&&!e.nativeEvent.isComposing){e.preventDefault();e.stopPropagation();changeOpen(false);arrow.current?.focus();}}}>{menu}</div>}
      menu={{selectable:true,selectedKeys:[defaultId],items:items.map(choice=>({key:choice.commandId,
        icon:<span className="context-menu-icon-slot"><CadIcon name={choice.iconKey as CadIconName}/></span>,
        label:<span className="cad-menu-label"><span>{choice.name}</span><kbd>{commandShortcutLabel(choice.commandId)}</kbd><span className="cad-menu-check">{choice.commandId===defaultId&&<CheckOutlined aria-label="上次使用"/>}</span></span>,
        disabled:!registry.state(choice.commandId).enabled&&!help.active})),onClick:({key})=>{
        const choice=items.find(value=>value.commandId===key);if(!choice)return;
        changeOpen(false);
        if(help.active){help.explain({toolbarName,commandName:choice.name,helpText:choice.helpText});return;}
        if(!registry.state(key).enabled)return;
        onDefaultChange(key);void registry.execute(key,{continuous:false}).catch(error=>feedback(error,"命令"));
      }}}>
      <Button data-cad-tool-control="true" ref={arrow} type="default" className="cad-split-arrow" disabled={!available} icon={<DownOutlined/>}
        aria-label={`${familyName}方式`} aria-haspopup="menu" aria-expanded={open}
        onPointerDown={()=>main.current?.cancelPending()}
        onKeyDown={e=>{if(e.key==="Enter"||e.key===" "||e.altKey&&e.key==="ArrowDown"){e.preventDefault();e.stopPropagation();changeOpen(true);}else if(e.key==="Escape"&&open){e.preventDefault();e.stopPropagation();changeOpen(false);}}}/>
    </Dropdown>
  </Space.Compact>;
}
