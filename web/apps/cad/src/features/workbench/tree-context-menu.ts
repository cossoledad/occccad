import type {MenuProps} from "antd";
type Item=NonNullable<MenuProps["items"]>[number];

// CATIA-style separation: common actions, an object-specific submenu, Properties last.
// Only project operations that exist; no decorative Cut/Copy/Paste placeholders.
export function structureMenu(items:Item[],name:string,multiple=false):Item[] {
 const live=items.filter((item):item is Exclude<Item,null>=>Boolean(item)&&item?.type!=="divider");
 const take=(keys:string[])=>live.filter(item=>"key" in item&&keys.includes(String(item.key)));
 const object=multiple?[]:take(["activate-body","construction","view-result","move","edit","create-part","pin-version","follow-head","detach","reconnect","refresh","suppress"]);
 const groups=[multiple?[]:take(["activate","open-document-tab","open-source"]),multiple?[]:take(["visibility"]),
  take(["copy-alias","delete"]).filter(item=>item?.key!=="delete"||!("disabled" in item&&item.disabled)),
  object.length?[{key:"object-actions",label:`${name} 对象`,children:object}]:[],take(["properties"])];
 return groups.filter(group=>group.length).flatMap((group,index)=>index?[{type:"divider" as const},...group]:group);
}
