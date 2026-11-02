import {expect,type Page} from '@playwright/test';

type TreeNode={id:string;name:string;children?:TreeNode[]};
/** Expand semantic ancestors using the real tree controls. This does not filter
 * rows or install a product-only testing route. */
export function treeNavigator(page:Page){return async(text:string,targetSuffix?:string)=>{
 if(!text.trim())return;
 const tree=await page.evaluate(async()=>{
  // @ts-ignore Vite application module, shared by live and explicit Mock tests.
  const {api}=await import('/src/api/client.ts');
  const id=decodeURIComponent(location.pathname.split('/documents/')[1]?.split('/')[0]??'');
  return (await api.getDocument(id)).structureTree;
 }) as TreeNode|undefined;
 if(!tree)throw new Error('No document structure');
 const expand=new Set<string>();
 let targetKey:string|undefined;
 const subtree=(node:TreeNode)=>{if(node.children?.length){expand.add(node.id);node.children.forEach(subtree)}};
 const visit=(node:TreeNode,parents:TreeNode[])=>{
  if(targetSuffix?node.id.endsWith(targetSuffix):node.name.toLocaleLowerCase().includes(text.trim().toLocaleLowerCase())){parents.forEach(p=>expand.add(p.id));if(!targetSuffix)subtree(node);targetKey??=node.id}
  node.children?.forEach(child=>visit(child,[...parents,node]));
 };visit(tree,[]);
 const scroll=page.locator('.specification-tree');
 for(const key of expand){
  const row=page.locator(`[role=treeitem][data-tree-key=${JSON.stringify(key)}]`);
  let expanded=false;
  for(let attempt=0;attempt<5;attempt++){
   try{
    await scroll.evaluate(el=>{el.scrollTop=0});
    for(let tries=0;await row.count()===0&&tries<100;tries++){
     await scroll.evaluate(el=>{el.scrollTop+=Math.max(80,el.clientHeight*.8)});
     await page.waitForTimeout(20);
    }
    await expect(row).toBeAttached();
    // Overscanned rows can be replaced between protocol lookup and execution.
    // Retry the semantic locator; never keep a detached ElementHandle.
    const attached=await row.evaluate(el=>{const owner=el.closest('.specification-tree');if(!owner)return false;const offset=el.getBoundingClientRect().top-owner.getBoundingClientRect().top;if(offset<0||offset+el.clientHeight>owner.clientHeight)owner.scrollTop+=offset-owner.clientHeight/2;return true});
    if(!attached)continue;
    await expect(row).toBeInViewport({timeout:2000});
    if(await row.getAttribute('aria-expanded')==='false'){
     await row.locator('.specification-tree-junction.branch').click({timeout:3000});
     await expect(row).toHaveAttribute('aria-expanded','true');
    }
    expanded=true;break;
   }catch(error){if(attempt===4)throw error}
  }
  if(!expanded)throw new Error('Tree row stayed detached: '+key);
 }
 await scroll.evaluate(el=>{el.scrollTop=0});
 if(targetSuffix){
  if(!targetKey)throw new Error('No semantic tree target: '+targetSuffix);
  const row=page.locator(`[role=treeitem][data-tree-key=${JSON.stringify(targetKey)}]`);
  for(let tries=0;await row.count()===0&&tries<100;tries++){
   await scroll.evaluate(el=>{el.scrollTop+=Math.max(80,el.clientHeight*.8)});await page.waitForTimeout(20);
  }
  await expect(row).toBeAttached();
  await row.evaluate(el=>{const owner=el.closest('.specification-tree');if(owner)owner.scrollTop+=el.getBoundingClientRect().top-owner.getBoundingClientRect().top-owner.clientHeight/2;});
  await expect(row).toBeInViewport();
 }
};}
