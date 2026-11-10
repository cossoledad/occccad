import assert from 'node:assert/strict';import{createServer}from'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const{applyVisualDelta}=await server.ssrLoadModule('/src/cad/visual/visual-delta.ts');
 const base=new Uint32Array([0xffffffff,5,7,9]).buffer,a='12'.repeat(32),b='34'.repeat(32);
 const make=(size,ops)=>{const parts=[];for(const[op,len,offset,delta,data]of ops){parts.push(op);const word=n=>{for(let i=0;i<4;i++)parts.push((n>>>(i*8))&255)};word(len);if(op!==1)word(offset);if(op===2)word(delta);if(op===1)parts.push(...data)}const out=new ArrayBuffer(80+parts.length),bytes=new Uint8Array(out),v=new DataView(out);bytes.set(new TextEncoder().encode('OCCDLT01'));bytes.fill(0x12,8,40);bytes.fill(0x34,40,72);v.setUint32(72,size,true);v.setUint32(76,ops.length,true);bytes.set(parts,80);return out};
 const patch=make(23,[[0,8,0],[2,12,4,0xffffffff],[1,3,0,0,[5,6,7]]]);
 assert.deepEqual([...new Uint8Array(applyVisualDelta(base,patch,a,b,23))],[...new Uint8Array(base,0,8),4,0,0,0,6,0,0,0,8,0,0,0,5,6,7]);
 assert.throws(()=>applyVisualDelta(base,patch,'ff'.repeat(32),b,23),/identity/);
 assert.throws(()=>applyVisualDelta(base,patch.slice(0,-1),a,b,23),/Truncated/);
 assert.throws(()=>applyVisualDelta(base,make(8,[[0,8,15]]),a,b,8),/source/);
 assert.throws(()=>applyVisualDelta(base,make(8,[[2,8,1,7]]),a,b,8),/Unaligned/);
 assert.throws(()=>applyVisualDelta(base,patch,a,b,2**31),/size/);
 console.log('Exact binary copies, wrapped index translations and malformed-delta rejection passed.');
}finally{await server.close()}
