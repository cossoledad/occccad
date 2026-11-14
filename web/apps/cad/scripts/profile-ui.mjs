import {createRequire} from 'node:module';import fs from 'node:fs';
import path from 'node:path';import {fileURLToPath} from 'node:url';import {execFileSync} from 'node:child_process';import {gzipSync} from 'node:zlib';
const root=path.resolve(fileURLToPath(new URL('../../../../',import.meta.url))),require=createRequire(root+'/web/apps/cad/package.json'),{chromium}=require('@playwright/test');
const args=process.argv.slice(2);if(args.length!==2||args[0]!=='--base'||!args[1])throw new Error('Usage: node scripts/profile-ui.mjs --base <git-ref>');
const baselineRef=args[1],output=path.join(root,'build/agent-work/ui-optimization');fs.mkdirSync(path.join(output,'reference'),{recursive:true});
const sourcePath='web/apps/cad/src/features/workbench/specification-tree.tsx';
const baselineHash=execFileSync('git',['rev-parse','--verify','--end-of-options',`${baselineRef}^{commit}`],{cwd:root,encoding:'utf8'}).trim();
const resolveReference=specifier=>{const base=path.resolve(root,path.dirname(sourcePath),specifier),target=[base,`${base}.ts`,`${base}.tsx`,`${base}.json`,path.join(base,'index.ts')].find(candidate=>fs.existsSync(candidate)&&fs.statSync(candidate).isFile());if(!target)throw new Error(`Cannot resolve reference import: ${specifier}`);return '/@fs'+target;};
const source=execFileSync('git',['show',`${baselineHash}:${sourcePath}`],{cwd:root,encoding:'utf8'}).replace(/(from\s*['"])(\.[^'"]+)(['"])/g,(_match,start,specifier,end)=>start+resolveReference(specifier)+end);
// The reference has no DocumentSessions; isolate its registration from any current module loaded by shared dependencies.
fs.writeFileSync(path.join(output,'reference/specification-tree.tsx'),source.replace('"workbench.tree"','"profile.reference-tree"'));
const dist=path.join(root,'web/apps/cad/dist'),names=[...new Set([...fs.readFileSync(path.join(dist,'index.html'),'utf8').matchAll(/(?:src|href)="\/assets\/([^"?]+\.js)"/g)].map(match=>match[1]))].sort();
const asset=name=>{const bytes=fs.readFileSync(path.join(dist,'assets',name));return {name,rawBytes:bytes.length,gzipBytes:gzipSync(bytes).length}};
const assets=names.map(asset),chunks=fs.readdirSync(path.join(dist,'assets')).filter(name=>/^(?:workbench|cad-viewport|global-options-dialog|document-center)-.*\.js$/.test(name)).map(asset);
const bundle={method:'production entry and direct HTML modulepreload JS; local gzip level 6; not network timing',assets,total:{files:assets.length,rawBytes:assets.reduce((sum,item)=>sum+item.rawBytes,0),gzipBytes:assets.reduce((sum,item)=>sum+item.gzipBytes,0)},chunks,mockInSourceMaps:fs.readdirSync(path.join(dist,'assets')).filter(name=>name.endsWith('.map')).some(name=>JSON.parse(fs.readFileSync(path.join(dist,'assets',name),'utf8')).sources.some(source=>source.includes('/api/mock-api.ts')))};
fs.writeFileSync(path.join(output,'bundle.json'),JSON.stringify(bundle,null,2));
const {createServer}=await import(require.resolve('vite'));
const server=await createServer({root:root+'/web/apps/cad',configFile:root+'/web/apps/cad/vite.config.ts',mode:'development',server:{hmr:false,host:'127.0.0.1',port:5183,strictPort:true,fs:{allow:[root]}},resolve:{dedupe:['react','react-dom','antd','@ant-design/icons','@tanstack/react-virtual','zustand']}});
let browser;const reports=[];
try{
 await server.listen();browser=await chromium.launch({headless:true});
 for(let round=0;round<3;round++)for(const baseline of [true,false]){
  const page=await browser.newPage({viewport:{width:1280,height:900}});const errors=[];page.on('pageerror',error=>errors.push(error.message));
  const query=baseline?'?reference='+encodeURIComponent('/@fs'+root+'/build/agent-work/ui-optimization/reference/specification-tree.tsx'):'';
  await page.goto('http://127.0.0.1:5183/browser/tree-studio.html'+query);await page.waitForFunction(()=>Boolean(window.treeHarness));
  await page.evaluate(async()=>{await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);window.treeHarness.samples.splice(0)});
  for(let i=0;i<45;i++)await page.evaluate(async i=>{window.treeHarness.highlight(i);await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame)},i);
  const report=await page.evaluate(()=>({nodes:5001,renderedRows:document.querySelectorAll('.specification-tree-virtual-row').length,reactActualDurationMs:window.treeHarness.samples,treeWidth:document.querySelector('.workbench-structure').getBoundingClientRect().width}));
  if(errors.length||report.reactActualDurationMs.length<45)throw new Error('Incomplete profiler run: '+JSON.stringify({errors,samples:report.reactActualDurationMs.length}));
  reports.push({baseline,round,...report,errors});if(round===0)await page.screenshot({path:root+'/build/agent-work/ui-optimization/tree-'+(baseline?'baseline':'current')+'.png'});await page.close();
 }
 fs.writeFileSync(root+'/build/agent-work/ui-optimization/tree-profile.json',JSON.stringify(reports,null,2));
 console.log(JSON.stringify({baselineRef,bundle:bundle.total,tree:reports.map(report=>({...report,reactActualDurationMs:undefined,medianMs:[...report.reactActualDurationMs].sort((a,b)=>a-b)[Math.floor(report.reactActualDurationMs.length/2)]}))},null,2));
}finally{await browser?.close();await server.close()}
