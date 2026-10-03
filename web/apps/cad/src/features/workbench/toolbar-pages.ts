import { uiTokens } from "../../design/visual-tokens";
import type { ToolbarVariantGroup } from "./toolbar-variants";

export type ToolbarPageGroup = { id:string; name:string; variants:ToolbarVariantGroup[]; start:number };
const columnGap=4, groupChrome=13; // 6px padding on each side + separator.
function width(variants:ToolbarVariantGroup[]):number {
  let result=groupChrome;
  for(let i=0;i<variants.length;i+=2){
    if(i)result+=columnGap;
    result+=Math.max(...variants.slice(i,i+2).map(group=>group.items.length>1?uiTokens.splitWidth:uiTokens.toolWidth));
  }
  return result;
}
/** Ordered catalog groups are kept whole; an oversized group splits only between columns. */
export function toolbarPages(groups:Omit<ToolbarPageGroup,"start">[],availableWidth:number):ToolbarPageGroup[][] {
  if(availableWidth<=0)return [groups.map(group=>({...group,start:0}))];
  const pages:ToolbarPageGroup[][]=[[]];let used=0;
  for(const group of groups){
    let start=0;
    while(start<group.variants.length){
      const rest=group.variants.slice(start),remainingWidth=width(rest);
      if(used&&remainingWidth>availableWidth-used){pages.push([]);used=0;}
      let end=group.variants.length;
      if(remainingWidth>availableWidth){
        end=start;
        do{end=Math.min(end+2,group.variants.length);}while(end<group.variants.length&&width(group.variants.slice(start,end+2))<=availableWidth);
      }
      const segment={...group,start,variants:group.variants.slice(start,end)};
      pages.at(-1)!.push(segment);used+=width(segment.variants);start=end;
      if(start<group.variants.length){pages.push([]);used=0;}
    }
  }
  return pages;
}
