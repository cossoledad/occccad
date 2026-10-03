import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {toolbarPages}=await server.ssrLoadModule('/src/features/workbench/toolbar-pages.ts');
 const {uiTokens}=await server.ssrLoadModule('/src/design/visual-tokens.ts');
 const variants=(prefix,count)=>Array.from({length:count},(_,i)=>({key:`${prefix}-${i}`,label:`${prefix}-${i}`,items:[{commandId:`${prefix}-${i}`}]}));
 const groups=[{id:'basic',name:'基本元素',variants:variants('basic',4)},{id:'profile',name:'轮廓',variants:variants('profile',6)},{id:'edit',name:'编辑',variants:variants('edit',8)},{id:'constraints',name:'约束',variants:variants('constraints',8)}];
 const expected=groups.flatMap(g=>g.variants.map(v=>v.key));
 for(const available of [1920,1366,1024,600,240,112]){
  const pages=toolbarPages(groups,available);
  assert.deepEqual(pages.flatMap(page=>page.flatMap(g=>g.variants.map(v=>v.key))),expected,'paging preserves every catalog item in priority order');
  assert.equal(pages[0][0].id,'basic');
  for(const page of pages){const width=page.reduce((sum,g)=>sum+13+Math.ceil(g.variants.length/2)*uiTokens.toolWidth+(Math.ceil(g.variants.length/2)-1)*4,0);assert(width<=available,'no page overlaps navigation or clips a column');}
 }
 const pages=toolbarPages(groups,600);
 assert.deepEqual(pages[0].map(g=>g.id),['basic','profile']);assert.deepEqual(pages[1].map(g=>g.id),['edit'],'next page replaces earlier groups');
 assert.deepEqual(pages[2].map(g=>g.id),['constraints']);
 const split=variants('split',3);split[1].items.push({commandId:'alternative'});
 const splitPages=toolbarPages([{id:'split',name:'分体',variants:split}],125);
 assert.deepEqual(splitPages.map(p=>p[0].variants.map(v=>v.key)),[['split-0','split-1'],['split-2']],'column width accounts for the wider split button');
 assert.deepEqual(toolbarPages([],400),[[]]);
 console.log('Toolbar priority, whole-group pages, oversized columns and split widths passed');
}finally{await server.close();}
