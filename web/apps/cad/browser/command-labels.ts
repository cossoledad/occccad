/** Command footers name their action; older generic dialogs still use 确定. */
export const confirmCommandName = /^(?:确\s*定|创\s*建|保\s*存)$/;

/** Both current field layouts expose their physical unit in the label. */
export function quantityLabel(name:string,unit:string){
 const escape=(value:string)=>value.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
 const dimension=unit==='deg'?'(?:deg|°)':escape(unit);
 return new RegExp(`^${escape(name)}(?:（${dimension}）|\\s*${dimension})$`);
}
export async function chooseExtent(dialog:import('@playwright/test').Locator,label:string){
 await dialog.getByRole('combobox',{name:'范围',exact:true}).click();
 await dialog.page().locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText(label,{exact:true}).click();
}
