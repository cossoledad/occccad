import {createElement} from "react";
import {ContextMenuIcon} from "../../components/context-menu-icon";
import type {MenuProps} from "antd";
type Item=NonNullable<MenuProps["items"]>[number];

// CATIA-style separation: common actions, an object-specific submenu, Properties last.
// Only project operations that exist; no decorative Cut/Copy/Paste placeholders.
export function structureMenu(items:Item[],name:string,multiple=false):Item[] {
 const live=items.filter((item):item is Exclude<Item,null>=>Boolean(item)&&item?.type!=="divider");
 const take=(keys:string[])=>live.filter(item=>"key" in item&&keys.includes(String(item.key)));
 const object=multiple?[]:take(["activate-body","construction","view-result","replace","insert-after","move","edit","create-part","pin-version","follow-head","detach","reconnect","refresh","suppress"]);
 const groups=[multiple?[]:take(["activate","open-document-tab","open-source"]),multiple?[]:take(["visibility"]),
  take(["copy-alias","delete"]).filter(item=>item?.key!=="delete"||!("disabled" in item&&item.disabled)),
  object.length?[{key:"object-actions",label:"对象",children:object}]:[],take(["properties"])];
 const decorate=(list:Item[]):Item[]=>list.map(item=>{
  if(!item||item.type==='divider')return item;
  const child='children' in item?{children:decorate(item.children as Item[]),popupClassName:'cad-command-submenu'}:{};
  const shortcut=({delete:'Del',properties:'Alt+Enter',edit:'Enter'} as Record<string,string>)[String(item.key)]??'';
  return {...item,...child,icon:'icon' in item&&item.icon?item.icon:createElement(ContextMenuIcon),label:createElement('span',{className:'cad-menu-label'},createElement('span',{className:'cad-menu-text'},'label' in item?item.label:null),createElement('kbd',{'aria-hidden':true},shortcut))};
 });
 return decorate(groups.filter(group=>group.length).flatMap((group,index)=>index?[{type:"divider" as const},...group]:group));
}
