import { Dropdown } from "antd";
import { useEffect } from "react";
import type { SelectionItem } from "../../types";
import { ContextMenuIcon } from "../../components/context-menu-icon";

export type ViewportContext={x:number;y:number;candidates:SelectionItem[]};

export function ViewportContextMenu({context,onClose,onProperties,onScene,onSelect,label}: {
  context:ViewportContext|undefined;onClose:()=>void;onProperties:()=>void;onScene:()=>void;
  onSelect:(selection:SelectionItem)=>void;label:(selection:SelectionItem)=>string;
}) {
  useEffect(()=>{if(!context)return;const key=(e:KeyboardEvent)=>{if(e.key==="Escape")onClose()};window.addEventListener("keydown",key,true);return ()=>window.removeEventListener("keydown",key,true)},[context,onClose]);
  if(!context)return null;
  const items=[{key:"properties",icon:<ContextMenuIcon/>,label:"属性…",onClick:()=>{onClose();onProperties()}},
    ...(context.candidates.length>1?[{key:"behind",icon:<ContextMenuIcon/>,label:"选择后方对象",children:context.candidates.slice(1,33).map((selection,index)=>({key:`behind-${index}`,label:label(selection),onClick:()=>{onSelect(selection);onClose()}}))}]:[]),
    {type:"divider" as const},{key:"scene",icon:<ContextMenuIcon/>,label:"场景属性…",onClick:()=>{onClose();onScene()}}];
  return <Dropdown open trigger={[]} placement="bottomLeft" onOpenChange={open=>{if(!open)onClose()}} menu={{items}} classNames={{root:"cad-command-menu"}}>
    <span style={{position:"fixed",left:context.x,top:context.y,width:1,height:1,pointerEvents:"none"}}/>
  </Dropdown>;
}
