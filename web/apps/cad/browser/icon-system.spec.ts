import {expect,test} from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
const output=path.resolve('../../../build/agent-work/icon-depth');
test('semantic CAD glyphs remain bounded and readable in compact UI states',async({page})=>{
 fs.mkdirSync(output,{recursive:true});const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 const reference=process.env.OCCCCAD_ICON_REFERENCE;
 if(reference)await page.route('**/icon-reference.json',route=>route.fulfill({body:fs.readFileSync(reference),contentType:'application/json'}));
 await page.goto('/browser/icon-studio.html'+(reference?'?compare':''));
 const names=await page.evaluate(async()=>Object.keys((await import('/src/cad/command/workbench-catalog.ts')).builtinCatalog.icons));
 await expect(page.locator('.all-icons article')).toHaveCount(names.length);
 const bounds=await page.locator('.all-icons svg').evaluateAll(icons=>icons.map(icon=>{const box=(icon as SVGSVGElement).getBBox();return {key:icon.getAttribute('data-icon'),x:box.x,y:box.y,right:box.x+box.width,bottom:box.y+box.height}}));
 fs.writeFileSync(path.join(output,'icon-bounds.json'),JSON.stringify({bounds},null,2));
 for(const box of bounds){expect(box.x,box.key!).toBeGreaterThanOrEqual(0);expect(box.y,box.key!).toBeGreaterThanOrEqual(0);expect(box.right,box.key!).toBeLessThanOrEqual(20);expect(box.bottom,box.key!).toBeLessThanOrEqual(20)}
 const featureGlyphs=await page.locator('.command-comparison article').evaluateAll(rows=>rows.map(row=>({command:row.getAttribute('data-command'),icon:row.querySelector('.icon-sizes>span:last-child svg')?.getAttribute('data-icon')})));
 const solid=featureGlyphs.filter(row=>['part.hole','part.fillet','part.chamfer','part.shell','part.sweep','part.loft'].includes(row.command!));expect(new Set(solid.map(row=>row.icon)).size).toBe(solid.length);
 for(const mode of ['light','dark','mono','disabled']){await page.getByRole('button',{name:mode,exact:true}).click();await page.screenshot({path:path.join(output,`icons-${mode}.png`),fullPage:true});await page.locator('.command-comparison').screenshot({path:path.join(output,`commands-${mode}.png`)})}
 fs.writeFileSync(path.join(output,'icon-bounds.json'),JSON.stringify({bounds,featureGlyphs,limit:'Bounds, semantic bindings and rendered state screenshots; recognition speed needs user observation'},null,2));expect(errors).toEqual([]);
});
