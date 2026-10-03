import * as THREE from "three";
import { uiTokens } from "../../design/visual-tokens";

type Entry={glyph:THREE.CanvasTexture;width:number;height:number;users:number};
const cache=new Map<string,Entry>(),unusedLimit=64;
function prune(){
  let unused=[...cache.values()].filter(value=>value.users===0).length;
  for(const [key,value] of cache){if(unused<=unusedLimit)break;if(value.users)continue;value.glyph.dispose();cache.delete(key);unused--;}
}
/** Transparent glyphs, shared across labels without a backing or outline. */
export function acquireDimensionLabelTextures(text:string){
  const dpr=Math.max(2,Math.min(4,globalThis.devicePixelRatio||2));
  const key=JSON.stringify([text,dpr,uiTokens.fontFamily,uiTokens.fontDimension]);
  let entry=cache.get(key);
  if(!entry){
    const glyphCanvas=document.createElement("canvas");
    const ctx=glyphCanvas.getContext("2d")!;
    const font=`500 ${uiTokens.fontDimension*dpr}px ${uiTokens.fontFamily}`;
    ctx.font=font;
    const width=Math.ceil(ctx.measureText(text).width/dpr)+8,height=uiTokens.fontDimension+8;
    {
      const canvas=glyphCanvas;
      canvas.width=Math.ceil(width*dpr);canvas.height=Math.ceil(height*dpr);
      const context=canvas.getContext("2d")!;context.font=font;context.textAlign="center";context.textBaseline="middle";
      context.fillStyle="#ffffff";
      context.fillText(text,canvas.width/2,canvas.height/2);
    }
    const glyph=new THREE.CanvasTexture(glyphCanvas);
    glyph.colorSpace=THREE.SRGBColorSpace;
    entry={glyph,width,height,users:0};
  }
  cache.delete(key);cache.set(key,entry);entry.users++;
  prune();let released=false;
  return {...entry,release(){if(released)return;released=true;entry!.users--;prune();}};
}
