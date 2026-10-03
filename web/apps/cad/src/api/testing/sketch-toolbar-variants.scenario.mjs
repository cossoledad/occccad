import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../../../package.json',import.meta.url));
const {createServer}=await import(require.resolve('vite'));
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {mockToolbarCatalog}=await server.ssrLoadModule('/src/api/mock-toolbar-catalog.ts');
 const {toolbarVariantGroups,toolbarVariantDefault,rememberToolbarVariant}=await server.ssrLoadModule('/src/features/workbench/toolbar-variants.ts');
 const values=new Map();globalThis.sessionStorage={getItem:k=>values.get(k)??null,setItem:(k,v)=>values.set(k,v)};
 const groups=mockToolbarCatalog.toolbars.flatMap(t=>toolbarVariantGroups(t.items));
 const group=key=>groups.find(g=>g.key===key);
 assert.deepEqual(group('arc').items.map(i=>i.commandId),['sketch.arc','sketch.arc.three_point','sketch.elliptical_arc']);
 assert.deepEqual(group('rectangle').items.map(i=>i.commandId),['sketch.rectangle','sketch.rectangle.center','sketch.rectangle.oriented']);
 for(const key of ['spline','rectangle','circle','arc','polygon','spline_control','linear','axis']){
  const g=group(key);assert.equal(toolbarVariantDefault(g).commandId,g.items[0].commandId);
  rememberToolbarVariant(g,g.items.at(-1).commandId);assert.equal(toolbarVariantDefault(g).commandId,g.items.at(-1).commandId);
  rememberToolbarVariant(g,'illegal-command');assert.equal(toolbarVariantDefault(g).commandId,g.items.at(-1).commandId);
  const hidden={...g,items:g.items.slice(0,-1)};assert.equal(toolbarVariantDefault(hidden).commandId,hidden.items[0].commandId);
 }
 assert.deepEqual(toolbarVariantGroups(mockToolbarCatalog.toolbars.find(t=>t.id==='sketch-dimensional-constraints').items).map(g=>g.key),['linear','axis','angle']);
 assert(!groups.some(g=>g.items.some(i=>i.commandId==='sketch.normal')));
 // A catalog rerender gets the same session choice; a removed ID falls back safely.
 const rect=group('rectangle');values.set('occccad.toolbar.variant.rectangle','removed');assert.equal(toolbarVariantDefault(rect).commandId,'sketch.rectangle');
}finally{await server.close();}
