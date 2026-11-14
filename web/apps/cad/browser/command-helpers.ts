import {expect, type Page} from '@playwright/test';

/** Visible names may repeat across CAD domains; prefer the active category or an explicit stable command ID. */
export async function searchTool(page:Page,name:string,commandId?:string){
 await page.getByRole('button',{name:'搜索工具',exact:true}).click();
 await page.getByRole('textbox',{name:'搜索工具名称或用途'}).fill(name);
 const results=page.locator('.workbench-command-result');
 const exact=results.filter({has:page.locator('strong',{hasText:new RegExp(`^${name.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')}$`)})});
 const contextual=exact.and(page.locator('button[data-current-context="true"]'));
 const result=commandId?results.and(page.locator(`button[data-command-id=${JSON.stringify(commandId)}]`)):await exact.count()>1&&await contextual.count()===1?contextual:exact;
 await expect(result,'Choose a unique command or supply its stable ID').toHaveCount(1);
 await result.click();
}
