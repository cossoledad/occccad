import type { Vec2 } from "../types";
/** CSS-pixel placement only; never writes model labelPosition. */
export function inlineEditorPosition(anchor:Vec2,size:{width:number;height:number},viewport:{width:number;height:number},editing=true){
 const margin=12;
 let x=editing?anchor[0]+margin:anchor[0]-size.width/2;
 let y=editing?anchor[1]-size.height-margin:anchor[1]-size.height/2;
 if(editing&&x+size.width>viewport.width-margin)x=anchor[0]-size.width-margin;
 if(editing&&y<margin)y=anchor[1]+margin;
 return {x:Math.max(margin,Math.min(x,viewport.width-size.width-margin)),y:Math.max(margin,Math.min(y,viewport.height-size.height-margin))};
}
export function inlineParameterPrefix(label:string){
 if(/半径|^R\b/.test(label))return "R";
 if(/直径/.test(label))return "⌀";
 if(/角度/.test(label))return "角度";
 if(/长度|距离|间距/.test(label))return "长度";
 if(/边数/.test(label))return "边数";
 return label;
}
