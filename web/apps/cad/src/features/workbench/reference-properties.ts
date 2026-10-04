import type { DocumentProperties, DocumentView, SelectionItem } from "../../types";
import { formatDisplayNumber } from "../../utils/display-number";

// Values are owning-document definitions; occurrence transforms stay separate.
export function referencePropertyRows(view:DocumentView,selection:SelectionItem,diagnostics?:DocumentProperties) {
 const format=(value?:number[])=>value?.map(v=>formatDisplayNumber(v)).join(", ")??"—";
 const row=(key:string,label:string,children:string)=>({key,label,children});
 const sources=diagnostics?.artifacts.map(a=>a.visualization.referenceGeometry)??[];
 if(selection.kind==="plane") {
  const plane=(view.part?.datumPlanes??view.datumPlanes??sources.flatMap(s=>s.datumPlanes)).find(p=>p.id===selection.entityId)??selection.datumPlane;
  return [row("type","类型","基准面"),row("name","名称",plane?.name??selection.id),row("id","标识",plane?.id??selection.entityId??"—"),row("plane","平面",plane?.plane??selection.plane),row("origin","原点 (mm)",format(plane?.origin)),row("normal","法向",format(plane?.normal)),row("u","U 方向",format(plane?.uDirection)),row("size","显示尺寸 (mm)",plane?formatDisplayNumber(plane.size):"—"),row("frame","坐标系","所有者文档局部坐标")];
 }
 if(selection.kind!=="axis"&&selection.kind!=="axis-system")return undefined;
 const system=(view.part?.axisSystems??view.axisSystems??sources.flatMap(s=>s.axisSystems)).find(a=>a.id===selection.entityId);
 const datum=selection.kind==="axis"&&selection.axis==="DATUM"?(view.part?.datumAxes??view.datumAxes??sources.flatMap(s=>s.datumAxes??[])).find(a=>a.id===selection.entityId):undefined;
 const direction=selection.kind==="axis"?(datum?.direction??(selection.axis==="X"?system?.xDirection:selection.axis==="Y"?system?.yDirection:system?.zDirection)):undefined;
 return [row("type","类型",selection.kind==="axis-system"?"坐标系":selection.axis==="DATUM"?"基准轴":`${selection.axis} 轴`),row("name","名称",datum?.name??system?.name??selection.id),row("id","标识",datum?.id??system?.id??selection.entityId??"—"),row("origin","原点 (mm)",format(datum?.origin??system?.origin)),...(direction?[row("direction","方向",format(direction))]:[]),...(system?[row("x","X 方向",format(system.xDirection)),row("y","Y 方向",format(system.yDirection)),row("z","Z 方向",format(system.zDirection))]:[]),row("frame","坐标系","所有者文档局部坐标"),row("reference","引用路径",selection.occurrencePath||"Part root")];
}
