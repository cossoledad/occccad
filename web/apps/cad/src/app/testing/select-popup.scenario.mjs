import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import React from 'react';
const require=createRequire(import.meta.url),{createServer}=await import(require.resolve('vite'));
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {selectOptionLabels,measureSelectOptions,selectPopupStyles,clearSelectMeasurements}=await server.ssrLoadModule('/src/app/select-popup.ts');
 const options=Array.from({length:10_000},(_,value)=>({value,label:value===9999?'最后一项仍必须完整显示的最长选项':'短'}));
 assert.equal(measureSelectOptions({options},text=>text.length*13),'最后一项仍必须完整显示的最长选项'.length*13+76);
 assert.deepEqual(selectOptionLabels({options:[{title:'分类',items:[{id:'one',title:React.createElement('span',null,'保留','第一侧')},{id:'two'}]}],fieldNames:{options:'items',label:'title',value:'id'}}),['分类','保留第一侧','two']);
 assert.deepEqual(selectOptionLabels({children:[React.createElement('option',{key:'one',value:'one'},'第一项'),React.createElement('optgroup',{key:'group',label:'组'},React.createElement('option',{value:'two'},'第二项'))]}),['第一项','组','第二项']);
 assert.equal(measureSelectOptions({options:[{value:1,label:'短\n较长的第二行'}]},text=>text.length),'较长的第二行'.length+76);
 const previous=globalThis.document;let measures=0;
 globalThis.document={createElement:()=>({getContext:()=>({font:'',measureText:text=>{measures++;return {width:text.length*10}}})})};
 try{
  clearSelectMeasurements();const props={options};const style=selectPopupStyles(props);
  assert.equal(style.popup.root.minWidth,`min(${'最后一项仍必须完整显示的最长选项'.length*10+76}px, calc(100vw - 24px))`);
  for(let i=0;i<200;i++)assert.deepEqual(selectPopupStyles(props),style);
  assert.equal(measures,10_000,'unchanged options are measured once, not on every hover/render');
  selectPopupStyles({...props,style:{fontSize:16}});assert.equal(measures,20_000,'font changes invalidate the measured width');
  clearSelectMeasurements();selectPopupStyles(props);assert.equal(measures,30_000,'loaded fonts can invalidate measurements');
 }finally{globalThis.document=previous}
}finally{await server.close()}
