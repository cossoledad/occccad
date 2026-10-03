import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../../../../package.json',import.meta.url));
const {createServer}=await import(require.resolve('vite'));
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const {uiTokens,commandPanelWidths,palette}=await server.ssrLoadModule('/src/design/visual-tokens.ts');
 const {ToolClickSequence}=await server.ssrLoadModule('/src/cad/overlay/tool-button.tsx');
 const {inlineEditorPosition}=await server.ssrLoadModule('/src/utils/inline-editor-position.ts');
 const {initialCommandPanelPosition,clampPanelPosition}=await server.ssrLoadModule('/src/utils/panel-position.ts');
 assert.deepEqual(commandPanelWidths,{S:380,M:520,L:960});assert.equal(uiTokens.splitWidth,uiTokens.toolWidth+uiTokens.arrowWidth);
 const calls=[],callbacks=new Map();let timer=0;const originalSet=globalThis.setTimeout,originalClear=globalThis.clearTimeout;
 globalThis.setTimeout=(fn,ms)=>{assert.equal(ms,220);callbacks.set(++timer,fn);return timer;};globalThis.clearTimeout=id=>callbacks.delete(id);
 try {
  const sequence=new ToolClickSequence(mode=>calls.push(mode));
  const flush=()=>{const pending=[...callbacks.values()];callbacks.clear();pending.forEach(fn=>fn());};
  sequence.click(true);sequence.cancel();flush();assert.deepEqual(calls,[],'arrow or unmount cancels a delayed main action');
  sequence.click(true);sequence.click(true);sequence.doubleClick(true);flush();assert.deepEqual(calls,[true],'double click executes exactly once in continuous mode');
  sequence.click(true);flush();assert.deepEqual(calls,[true,false],'single click preserves one-shot semantics');
 }finally{globalThis.setTimeout=originalSet;globalThis.clearTimeout=originalClear;}
 for(const width of [1920,1366,1024])for(const dpr of [1,1.5]){
  const viewport={width,height:768},size={width:160,height:32};
  for(const anchor of [[2,2],[width-2,766],[width/2,384]]){
   const position=inlineEditorPosition(anchor,size,viewport);assert(position.x>=12&&position.y>=12);assert(position.x+size.width<=width-12);assert(position.y+size.height<=756);
   assert.deepEqual(position,inlineEditorPosition(anchor,size,viewport),'DPR never changes logical CSS placement');
  }
  const host={width,height:720},view={left:250,top:124,width:width-510,height:596};
  const panel={width:520,height:320},position=initialCommandPanelPosition(panel,host,view);
  assert.equal(position.y,140);assert(position.x>=16&&position.x+520<=width-16);
  const saved=clampPanelPosition({x:width,y:900},panel,host);assert.equal(saved.y,384);
 }
 // Actual render textures share immutable masks and dispose evicted entries.
 const canvases=[];globalThis.document={createElement:()=>{const ops=[];const canvas={width:0,height:0,ops,getContext:()=>({measureText:t=>({width:t.length*24}),fillText:()=>ops.push('fill'),strokeText:()=>ops.push('stroke')})};canvases.push(canvas);return canvas;}};
 globalThis.devicePixelRatio=1.5;
 const {acquireDimensionLabelTextures}=await server.ssrLoadModule('/src/cad/rendering/dimension-label-textures.ts');
 const first=acquireDimensionLabelTextures('R5'),second=acquireDimensionLabelTextures('R5');
 assert.equal(first.glyph,second.glyph);assert.equal(canvases.length,1,'same style and text reuse transparent glyphs');assert.deepEqual(canvases[0].ops,['fill'],'no backing or outline is painted');
 let disposed=0;first.glyph.addEventListener('dispose',()=>disposed++);
 first.release();first.release();assert.equal(disposed,0,'shared live textures remain available');second.release();
 for(let n=0;n<70;n++)acquireDimensionLabelTextures(`R${n+10}`).release();assert.equal(disposed,1,'bounded unused cache releases the texture');
 const luminance=color=>{const channels=color.slice(1).match(/../g).map(c=>parseInt(c,16)/255).map(c=>c<=.04045?c/12.92:((c+.055)/1.055)**2.4);return channels[0]*.2126+channels[1]*.7152+channels[2]*.0722;};
 for(const foreground of [palette.text,palette.textSecondary,palette.primary])for(const background of [palette.surface,palette.subtle,palette.primarySoft])assert((luminance(background)+.05)/(luminance(foreground)+.05)>=4.5,`${foreground} on ${background} remains readable`);
 console.log('Shared UI timing, logical placement, panel sizes, text contrast and texture lifecycle passed');
}finally{await server.close();}
