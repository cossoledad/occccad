import type {ToolbarCatalogItem} from "../../types";

export type ToolbarVariantGroup={key:string;label:string;items:ToolbarCatalogItem[]};
const labels:Record<string,string>={spline:"样条线",rectangle:"矩形",circle:"圆",arc:"圆弧",ellipse:"椭圆",polygon:"多边形",spline_control:"样条控制",linear:"线性尺寸",axis:"轴尺寸",angle:"角度"};
const remembered=new Map<string,string>();
export function toolbarVariantGroups(items:ToolbarCatalogItem[]):ToolbarVariantGroup[]{
 const groups:ToolbarVariantGroup[]=[];
 for(const item of items){const family=item.groupKey.startsWith("variants:")?item.groupKey.slice(9):undefined,key=family??item.commandId;
  const group=family?groups.find(g=>g.key===key):undefined;
  if(group)group.items.push(item);else groups.push({key,label:family?labels[family]??item.name:item.name,items:[item]});
 }
 return groups;
}
export function toolbarVariantDefault(group:ToolbarVariantGroup):ToolbarCatalogItem{
 let id=remembered.get(group.key);try{id=sessionStorage.getItem(`occccad.toolbar.variant.${group.key}`)??id;}catch{/* Storage may be disabled. */}
 return group.items.find(item=>item.commandId===id)??group.items[0];
}
export function rememberToolbarVariant(group:ToolbarVariantGroup,commandId:string):void{
 if(!group.items.some(item=>item.commandId===commandId))return;
 remembered.set(group.key,commandId);try{sessionStorage.setItem(`occccad.toolbar.variant.${group.key}`,commandId);}catch{/* Session memory remains available. */}
}
