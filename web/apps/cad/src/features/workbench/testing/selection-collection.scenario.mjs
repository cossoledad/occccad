import assert from 'node:assert/strict';
import {createServer} from 'vite';
import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {FeaturePickField}=await server.ssrLoadModule('/src/features/workbench/feature-pick-field.tsx');
 const items=Array.from({length:12},(_,i)=>({key:`private-stable-id-${i}`,value:`轴承 ${i+1}`,detail:i===7?'支撑已失效':'机架 / 轴承',error:i===7,state:i===7?'MISSING':'RESOLVED',onActivate(){},onClear(){},onLocate(){}}));
 const html=renderToStaticMarkup(React.createElement(FeaturePickField,{label:'成员',mode:'multiple',active:false,items,onActivate(){},onClear(){}}));
 assert.equal((html.match(/role="listitem"/g)??[]).length,12);
 assert.equal((html.match(/class="feature-pick-field /g)??[]).length,1,'collection has one field instead of per-item cards');
 assert.match(html,/筛选成员/);assert.match(html,/12 项/);assert.match(html,/1 项待修复/);assert.match(html,/data-state="MISSING"/);assert.match(html,/定位成员 8/);assert.doesNotMatch(html,/private-stable-id/,'UI keys are not display labels');
 const ordered=renderToStaticMarkup(React.createElement(FeaturePickField,{label:'截面',mode:'ordered',active:false,items:items.slice(0,2)}));assert.equal((ordered.match(/feature-pick-ordinal/g)??[]).length,2,'ordered selection retains explicit order');
 console.log('Compact collections retain item actions, missing inputs, private identities and order');
}finally{await server.close()}
