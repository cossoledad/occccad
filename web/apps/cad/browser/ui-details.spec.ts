import {expect,test} from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

const output=path.resolve('../../../build/agent-work/ui-details');
test.beforeEach(()=>fs.mkdirSync(output,{recursive:true}));

test('login rotates three real images, handles wheel bursts and keeps the form independent',async({page})=>{
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 await page.addInitScript(()=>localStorage.setItem('occccad.mock.auth','false'));await page.clock.install();await page.goto('/');
 const hero=page.locator('.auth-hero'),image=hero.locator('img.active'),caption=hero.locator('h2');
 await expect(caption).toHaveText('径流叶轮');
 await expect.poll(()=>hero.locator('img').evaluateAll(images=>images.every(image=>(image as HTMLImageElement).complete&&(image as HTMLImageElement).naturalWidth>0))).toBe(true);
 await page.clock.fastForward(6100);await expect(caption).toHaveText('过渡壳体');
 await page.clock.fastForward(6100);await expect(caption).toHaveText('法兰联轴器');
 await page.clock.fastForward(6100);await expect(caption).toHaveText('径流叶轮');
 await page.mouse.move(300,400);await page.mouse.wheel(0,100);await expect(caption).toHaveText('过渡壳体');
 for(let i=0;i<8;i++)await hero.dispatchEvent('wheel',{deltaY:30,deltaMode:0});await expect(caption).toHaveText('过渡壳体');
 await page.clock.fastForward(800);await hero.dispatchEvent('wheel',{deltaY:-3,deltaMode:1});await expect(caption).toHaveText('径流叶轮');
 await page.locator('.auth-panel').dispatchEvent('wheel',{deltaY:120,deltaMode:0});await expect(caption).toHaveText('径流叶轮');
 await page.clock.fastForward(5100);await expect(caption).toHaveText('径流叶轮');
 await page.clock.fastForward(1000);await expect(caption).toHaveText('过渡壳体');
 await hero.getByRole('button',{name:'暂停轮播',exact:true}).click();await page.clock.fastForward(20_000);await expect(caption).toHaveText('过渡壳体');
 await hero.getByRole('button',{name:'法兰联轴器',exact:true}).click();await expect(caption).toHaveText('法兰联轴器');
 await hero.getByRole('button',{name:'播放轮播',exact:true}).click();await page.clock.fastForward(6100);await expect(caption).toHaveText('径流叶轮');
 await expect(image).toHaveAttribute('aria-hidden','false');
 await page.getByRole('textbox',{name:'电子邮箱',exact:true}).fill('designer@example.com');await page.clock.fastForward(6100);await expect(page.getByRole('textbox',{name:'电子邮箱',exact:true})).toHaveValue('designer@example.com');
 await page.clock.runFor(600);await page.screenshot({path:path.join(output,'login-carousel.png')});expect(errors).toEqual([]);
});

test('reduced motion starts paused and supports manual selection',async({page})=>{
 await page.addInitScript(()=>localStorage.setItem('occccad.mock.auth','false'));await page.emulateMedia({reducedMotion:'reduce'});await page.clock.install();await page.goto('/');
 const hero=page.locator('.auth-hero');await expect(hero.getByRole('button',{name:'播放轮播',exact:true})).toBeVisible();
 await page.clock.fastForward(20_000);await expect(hero.locator('h2')).toHaveText('径流叶轮');
 await hero.getByRole('button',{name:'过渡壳体',exact:true}).click();await expect(hero.locator('h2')).toHaveText('过渡壳体');
 await expect(hero.locator('img.active')).toHaveCSS('transition-duration','0s');
});

test('dropdown fits offscreen options, wraps within narrow screens and retains virtual scrolling',async({page})=>{
 const samples:unknown[]=[];
 for(const width of [1440,360]){
  await page.setViewportSize({width,height:900});await page.goto('/browser/ui-details-studio.html');
  const select=page.getByRole('combobox',{name:'长列表',exact:true});const start=await page.evaluate(()=>performance.now());await select.click();
  const popup=page.locator('.ant-select-dropdown:visible');await expect(popup).toBeVisible();
  const initial=await popup.locator('.ant-select-item-option').count();expect(initial).toBeLessThan(40);
  const metrics=await popup.evaluate(element=>{const r=element.getBoundingClientRect();return {x:r.x,right:r.right,width:r.width}});
  expect(metrics.width).toBeGreaterThan(96);expect(metrics.x).toBeGreaterThanOrEqual(0);expect(metrics.right).toBeLessThanOrEqual(width);
  await select.press('ArrowUp');const last=popup.locator('.ant-select-item-option-content').filter({hasText:'末项：'});await expect(last).toBeVisible();
  const text=await last.evaluate(element=>({scroll:element.scrollWidth,client:element.clientWidth,height:element.getBoundingClientRect().height,lineHeight:parseFloat(getComputedStyle(element).lineHeight)}));expect(text.scroll).toBeLessThanOrEqual(text.client+1);
  if(width===360)expect(text.height).toBeGreaterThan(text.lineHeight);
  samples.push({viewport:width,options:1000,renderedOptions:initial,popup:metrics,text,openAndKeyboardMs:await page.evaluate(()=>performance.now())-start});
  await page.screenshot({path:path.join(output,`dropdown-${width}.png`)});await select.press('Enter');
  await expect(page.locator('.ant-select').first()).toContainText('末项：');await expect(popup).toHaveCount(0);
  await page.getByRole('combobox',{name:'分组选项',exact:true}).click();const grouped=page.locator('.ant-select-dropdown:visible');await expect(grouped.getByText('保留第二侧及全部不相连的区域',{exact:true})).toBeVisible();
  const groupText=await grouped.locator('.ant-select-item-option-content').last().evaluate(element=>({scroll:element.scrollWidth,client:element.clientWidth}));expect(groupText.scroll).toBeLessThanOrEqual(groupText.client+1);
 }
 fs.writeFileSync(path.join(output,'dropdown-measurement.json'),JSON.stringify({renderer:'Chromium',samples,limit:'UI automation wall time includes protocol and scheduling; not a steady-state latency benchmark'},null,2));
});
