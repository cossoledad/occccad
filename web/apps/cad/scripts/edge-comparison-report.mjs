import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

const directory=path.resolve(process.argv[2]??fileURLToPath(new URL('../../../../build/agent-work/natural-edges/comparison',import.meta.url)));
const report=JSON.parse(fs.readFileSync(path.join(directory,'measurements.json'),'utf8'));
const scenes=[...new Set(report.reports.map(r=>r.key))];
const escape=s=>s.replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const html=`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>自然边线 · 实际视口对比</title>
<style>*{box-sizing:border-box}body{margin:0;background:#edf0f3;color:#22323e;font:15px/1.6 system-ui,sans-serif}main{max-width:1800px;margin:auto;padding:28px}h1{font-size:24px;margin:0 0 12px}p{margin:8px 0 18px;color:#53636e}select,button{font:inherit;padding:7px 14px;border:1px solid #bdc8d0;border-radius:6px;background:white;color:inherit}nav{display:flex;gap:12px;align-items:center;margin:16px 0}.grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px}figure{margin:0;background:white;border-radius:10px;overflow:hidden;box-shadow:0 2px 12px #23344112}img{width:100%;display:block}figcaption{padding:12px 16px}strong{display:block}.stats{font-size:13px;color:#53636e}details{margin-top:24px}pre{white-space:pre-wrap;font-size:12px}a{color:#216a94}button{cursor:pointer}@media(max-width:1000px){.grid{grid-template-columns:1fr}}</style>
<main><h1>自然边线 · 实际视口对比</h1><p>同一 OCCT GLB、相机、材质与光照。基础版和优化版已逐像素核对一致；图片来自生产视口引擎。场景数据由验收夹具提供，真实文档链路截图另列于下方。</p>
<nav><label>场景 <select id="scene">${scenes.map(s=>`<option>${escape(s)}</option>`).join('')}</select></label><button id="previous">上一个</button><button id="next">下一个</button></nav>
<div class="grid">${[['uniform','统一边线'],['basic','基础方案'],['natural','优化后的默认方案']].map(([key,title])=>`<figure><a id="link-${key}"><img id="image-${key}"></a><figcaption><strong>${title}</strong><span class="stats" id="stats-${key}"></span></figcaption></figure>`).join('')}</div>
<p>Chromium SwiftShader；1280 × 900；DPR 1；关闭地面、阴影与 AO。统计包含背景和既有深度通道。每阶段预热后记录 4 帧；submitMs 是 renderer.render 的 CPU 提交耗时，包含可能的驱动阻塞，不是 GPU 时间或稳定帧率。attributeBytes 仅统计新建的边线派生属性数组，不能当作总显存。</p>
<details><summary>真实 Jobs / 参数化建模 → Router → Worker → Artifact → 文档视图</summary><div class="grid">${['cube','fillet','cylinder','sphere','coplanar','varying','open','parametric-cube','assembly'].map(s=>`<figure><a href="live-${s}.png"><img loading="lazy" src="live-${s}.png"></a><figcaption>${s}</figcaption></figure>`).join('')}</div></details>
<details><summary>原始测量与采样条件</summary><pre id="raw"></pre><p><a href="measurements.json">测量 JSON</a> · <a href="stability.json">稳定性检查</a> · <a href="live-chain.json">真实链路</a> · <a href="../fixtures/geometry.json">原生几何统计</a></p></details>
</main><script>const data=${JSON.stringify(report).replaceAll('<','\\u003c')};const select=document.getElementById('scene');function update(){for(const mode of ['uniform','basic','natural']){const r=data.reports.find(r=>r.key===select.value&&r.mode===mode);const url=select.value+'-'+mode+'.png';document.getElementById('image-'+mode).src=url;document.getElementById('link-'+mode).href=url;document.getElementById('stats-'+mode).textContent='绘制 '+r.frames[0].calls+' 次 · 边线几何 '+r.frames[0].edgeGeometries+' 份 · 派生属性 '+r.edgePreparation.attributeBytes+' B';}document.getElementById('raw').textContent=JSON.stringify(data.reports.filter(r=>r.key===select.value),null,2);}select.onchange=update;for(const [id,step] of [['previous',-1],['next',1]])document.getElementById(id).onclick=()=>{select.selectedIndex=(select.selectedIndex+step+select.length)%select.length;update()};update();</script></html>`;
fs.writeFileSync(path.join(directory,'index.html'),html);
console.log(path.join(directory,'index.html'));
