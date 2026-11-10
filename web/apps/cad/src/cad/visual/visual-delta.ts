export const VISUAL_DELTA_TYPE='application/vnd.occccad.visual-delta';
const MAX_BYTES=64*1024*1024;

/** Reconstruct exact GLB bytes. Geometry interpretation remains in the GLB
 * decoder; the caller verifies the resulting target SHA before publishing it. */
export function applyVisualDelta(base:ArrayBuffer,patch:ArrayBuffer,baseDigest:string,targetDigest:string,expectedSize:number):ArrayBuffer {
  if(patch.byteLength<80||patch.byteLength>MAX_BYTES*2||base.byteLength>MAX_BYTES||!Number.isSafeInteger(expectedSize)||expectedSize<0||expectedSize>MAX_BYTES)throw new Error('Invalid visual delta size');
  const bytes=new Uint8Array(patch),view=new DataView(patch);
  if(new TextDecoder().decode(bytes.subarray(0,8))!=='OCCDLT01')throw new Error('Unsupported visual delta');
  const hex=(begin:number,end:number)=>Array.from(bytes.subarray(begin,end),b=>b.toString(16).padStart(2,'0')).join('');
  if(hex(8,40)!==baseDigest||hex(40,72)!==targetDigest||view.getUint32(72,true)!==expectedSize)throw new Error('Visual delta identity mismatch');
  const result=new ArrayBuffer(expectedSize),out=new Uint8Array(result),source=new Uint8Array(base),sourceView=new DataView(base),outView=new DataView(result);
  const count=view.getUint32(76,true);let cursor=80,written=0;
  const take=(length:number)=>{if(length>patch.byteLength-cursor)throw new Error('Truncated visual delta');const start=cursor;cursor+=length;return start;};
  for(let i=0;i<count;i++){
    const op=bytes[take(1)],length=view.getUint32(take(4),true);
    if(length>expectedSize-written)throw new Error('Visual delta output overflow');
    if(op===1)out.set(bytes.subarray(take(length),cursor),written);
    else if(op===0||op===2){
      const offset=view.getUint32(take(4),true);if(length>base.byteLength-offset)throw new Error('Visual delta source overflow');
      if(op===0)out.set(source.subarray(offset,offset+length),written);
      else {if(offset%4||length%4||written%4)throw new Error('Unaligned visual index delta');const delta=view.getUint32(take(4),true);for(let j=0;j<length;j+=4)outView.setUint32(written+j,(sourceView.getUint32(offset+j,true)+delta)>>>0,true);}
    }else throw new Error('Unknown visual delta operation');
    written+=length;
  }
  if(cursor!==patch.byteLength||written!==expectedSize)throw new Error('Incomplete visual delta');return result;
}
