import 'antd/dist/reset.css';
import '../src/styles.css';
import '../src/design/design-system.css';
import '../src/features/workbench/workbench.css';
import {createRoot} from 'react-dom/client';
import {useState} from 'react';
import {builtinCatalog,type WorkbenchCatalog} from '../src/cad/command/workbench-catalog';
import {CadIcon,CadIconContext} from '../src/cad/overlay/cad-icons';
import {treeNodeIcon} from '../src/features/workbench/tree-node-descriptors';
import {installVisualTokens} from '../src/design/visual-tokens';
installVisualTokens(document.documentElement);
const reference:WorkbenchCatalog|undefined=new URLSearchParams(location.search).has('compare')?await(await fetch('/icon-reference.json')).json():undefined;
const examples=['part.pad','part.pocket','part.hole','part.fillet','part.chamfer','part.shell','part.draft','part.sweep','part.loft','part.pattern.linear','part.pattern.circular','geometry.surface-extrude','geometry.surface-revolve','geometry.surface-sweep','geometry.surface-loft','geometry.surface-fill','geometry.surface-trim','geometry.surface-extend','geometry.surface-sew','geometry.surface-offset','geometry.close-surface','sketch.rectangle','sketch.rectangle.center','sketch.rectangle.oriented','dmu.revolute','dmu.prismatic','dmu.cylindrical','dmu.spherical','dmu.rigid','dmu.planar'];
function Studio(){
 const [mode,setMode]=useState('light');
 return <main className="icon-studio" data-mode={mode}>
  <header><h1>CAD 图标语言</h1><span>20 × 20 · 1.5 px · 几何母形 + 动作标记</span><nav>{['light','dark','mono','disabled'].map(value=><button key={value} onClick={()=>setMode(value)} aria-pressed={mode===value}>{value}</button>)}</nav></header>
  <h2>命令辨识 {reference&&'· 原方案 / 当前方案'}</h2>
  <section className="command-comparison">{examples.map(id=>{
   const command=builtinCatalog.commands.find(command=>command.id===id)!;
   const previous=reference?.commands.find(command=>command.id===id);
   return <article key={id} data-command={id}><div className="icon-sizes">{previous&&<CadIconContext.Provider value={reference!.icons}><span className="icon-reference"><CadIcon name={previous.iconKey}/></span></CadIconContext.Provider>}{[16,20,24].map(size=><span key={size} style={{width:size,height:size}}><CadIcon name={command.iconKey}/></span>)}</div><strong>{command.name}</strong><small>{id}</small></article>;
  })}</section>
  <h2>结构树对象</h2><section className="tree-icons">{['PRODUCT','PART','INSTANCE','BODY','GEOMETRICAL_SET','SKETCH','PLANE','REFERENCE_POINT','AXIS_SYSTEM','PARAMETER','PUBLICATION','IMPORT'].map(kind=><article key={kind}>{treeNodeIcon({kind:kind as any})}<span>{kind}</span></article>)}
   <article>{treeNodeIcon({kind:'FEATURE',entityType:'FILLET',evaluationStatus:'FAILED'})}<span>求值失败</span></article>
   <article>{treeNodeIcon({kind:'FEATURE',entityType:'SURFACE_EXTRUDE',suppressed:true})}<span>已抑制</span></article>
   <article>{treeNodeIcon({kind:'FEATURE',entityType:'HOLE'},'需要更新')}<span>需要更新</span></article>
  </section>
  <h2>完整图标目录 · {Object.keys(builtinCatalog.icons).length}</h2><section className="all-icons">{Object.keys(builtinCatalog.icons).map(key=><article key={key}><CadIcon name={key}/><span>{key}</span></article>)}</section>
 </main>;
}
const style=document.createElement('style');style.textContent=`
body{margin:0}.icon-studio{padding:28px 40px;background:#f4f6f8;color:#34485d;font:13px system-ui;min-height:100vh}.icon-studio header{display:flex;align-items:center;gap:24px;margin-bottom:24px}.icon-studio h1{font-size:24px;margin:0}.icon-studio h2{font-size:15px;margin:28px 0 14px}.icon-studio nav{margin-left:auto;display:flex;gap:8px}.icon-studio nav button{border:1px solid #c7d1db;border-radius:6px;padding:6px 12px;background:transparent;color:inherit;cursor:pointer}.icon-studio nav button[aria-pressed=true]{background:#3564d3;color:white}.command-comparison{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:10px}.command-comparison article{display:grid;gap:10px;padding:18px;border-radius:9px;background:#fff}.icon-sizes{display:flex;align-items:center;gap:16px;min-height:28px}.icon-sizes span{display:inline-flex;align-items:center;justify-content:center}.icon-sizes .cad-command-icon{width:100%;height:100%}.icon-sizes .icon-reference{width:24px;height:24px;opacity:1;margin-right:8px;padding-right:8px;box-sizing:content-box;border-right:1px solid #dce3ea}.icon-studio article strong{font-size:13px;font-weight:500}.icon-studio article small{color:#8090a0;font:10px ui-monospace,monospace}.all-icons{display:grid;grid-template-columns:repeat(7,minmax(0,1fr));gap:8px}.all-icons article,.tree-icons article{display:flex;gap:9px;align-items:center;padding:12px;background:#ffffff88;border-radius:6px;font-size:11px}.tree-icons{display:grid;grid-template-columns:repeat(6,minmax(0,1fr));gap:8px}.tree-object-icon{display:inline-flex;position:relative}.icon-studio[data-mode=dark]{background:#1d2938;color:#d1deeb}.icon-studio[data-mode=dark] article{background:#26364a}.icon-studio[data-mode=dark] .cad-command-icon{color:inherit;--cad-icon-accent:currentColor}.icon-studio[data-mode=mono] .cad-command-icon{color:#34485d;--cad-icon-accent:currentColor}.icon-studio[data-mode=disabled] .cad-command-icon{color:#9ba7b4;--cad-icon-accent:currentColor}`;document.head.append(style);
createRoot(document.getElementById('root')!).render(<Studio/>);
