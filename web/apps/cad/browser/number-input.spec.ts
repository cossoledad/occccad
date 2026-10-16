import {expect,test,type Page} from '@playwright/test';

async function openInput(page:Page,maximum?:number){
 await page.goto('/');
 await page.evaluate(async maximum=>{
  // Vite resolves this fixture's ordinary React imports through the same graph
  // as the production control, including dependency version queries.
  // @ts-ignore browser-only Vite fixture.
  const {mountNumberFixture}=await import('/browser/fixtures/number-input.tsx');
  mountNumberFixture(maximum);
 },maximum);
 await expect(page.getByRole('spinbutton',{name:'精确值'})).toHaveValue('12.35');
}

test('rounded numeric display survives blur and Enter while edits keep full precision',async({page})=>{
 await openInput(page);const input=page.getByRole('spinbutton',{name:'精确值'}),model=page.getByTestId('canonical-value');
 await input.focus();await page.getByRole('button',{name:'移出焦点'}).click();await expect(model).toHaveText('12.3456789');
 await input.press('Enter');await expect(model).toHaveText('12.3456789');
 await input.fill('7.654321');await expect(input).toHaveValue('7.654321');
 await page.getByRole('button',{name:'移出焦点'}).click();await expect(input).toHaveValue('7.65');await expect(model).toHaveText('7.654321');
 await input.fill('');await input.pressSequentially('7.65');await input.press('Enter');await expect(model).toHaveText('7.65');
});

test('display preservation retains native range validation',async({page})=>{
 await openInput(page,10);const input=page.getByRole('spinbutton',{name:'精确值'});
 await input.focus();await page.getByRole('button',{name:'移出焦点'}).click();
 await expect(page.getByTestId('canonical-value')).toHaveText('10');await expect(input).toHaveValue('10');
});
