import { expect, test } from "@playwright/test";

// UI acceptance uses the isolated adapter; authoritative solving is tested separately.
test("assembly suppression is visible and reversible from the tree", async ({ page }) => {
  const errors:string[]=[];
  page.on("pageerror",error=>errors.push(error.message));
  await page.goto("/documents/mock-product-frame");
  await expect(page.locator("canvas")).toBeVisible();
  await page.getByRole("textbox",{name:"筛选模型结构"}).fill("COINCIDENT");
  const constraint=page.getByRole("treeitem").filter({hasText:"#COINCIDENT.1"});
  await expect(constraint).toBeVisible();
  await constraint.click({button:"right"});
  await page.getByRole("menuitem",{name:/ 抑制$/}).click();
  await expect(constraint.getByLabel("状态 停用")).toBeVisible();
  await constraint.click({button:"right"});
  await page.getByRole("menuitem",{name:/解除抑制$/}).click();
  await expect(constraint.getByLabel("状态 停用")).toHaveCount(0);
  await constraint.dblclick();
  const dialog=page.getByRole("dialog",{name:"约束定义",exact:true});
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button",{name:"停用约束",exact:true}).click();
  await expect(dialog).toHaveCount(0);
  await expect(constraint.getByLabel("状态 停用")).toBeVisible();
  expect(errors).toEqual([]);
});

test("fixed coordinates and stable angle parameters survive editing", async ({page})=>{
  const errors:string[]=[];
  page.on("pageerror",error=>errors.push(error.message));
  await page.goto("/documents/mock-product-frame");
  await expect(page.locator("canvas")).toBeVisible();
  // Seed definitions through the same mock domain adapter, then exercise real editors.
  await page.evaluate(async()=>{
    const { api } = await import("/src/api/client.ts");
    const { queryClient } = await import("/src/app/providers.tsx");
    await api.command("mock-product-frame",{type:"ADD_ASSEMBLY_CONSTRAINT",constraintKind:"FIX",firstAssemblyRef:{instanceId:"mock-instance-a",kind:"BODY"},fixedPose:{translation:[1,2,3],rotation:[0,0,0,1]}});
    await api.command("mock-product-frame",{type:"ADD_ASSEMBLY_CONSTRAINT",constraintKind:"ANGLE",angleRelation:"DIRECTED",value:1,
      firstAssemblyRef:{instanceId:"mock-instance-a",kind:"PLANE",geometryId:"datum-yz"},secondAssemblyRef:{instanceId:"mock-instance-b",kind:"PLANE",geometryId:"datum-yz"},
      angleAxis:{instanceId:"mock-instance-b",kind:"PLANE",geometryId:"datum-xy"}});
    await queryClient.invalidateQueries();
  });
  const filter=page.getByRole("textbox",{name:"筛选模型结构"});
  await filter.fill("FIX");
  const fixed=page.getByRole("treeitem").filter({hasText:/#FIX\./});
  await fixed.dblclick();
  const dialog=page.getByRole("dialog",{name:"约束定义",exact:true});
  await expect(dialog.getByRole("spinbutton",{name:"* X（mm）",exact:true})).toHaveValue("1");
  await dialog.getByRole("spinbutton",{name:"* X（mm）",exact:true}).fill("12");
  await dialog.getByRole("spinbutton",{name:"* 绕 Z 旋转（deg）",exact:true}).fill("90");
  await dialog.getByRole("spinbutton",{name:"* 绕 Z 旋转（deg）",exact:true}).press("Tab");
  await dialog.getByRole("button",{name:/^确\s*定$/}).click();
  await expect(dialog).toHaveCount(0);
  await fixed.dblclick();
  await expect(dialog.getByRole("spinbutton",{name:"* X（mm）",exact:true})).toHaveValue("12");
  await expect(dialog.getByRole("spinbutton",{name:"* 绕 Z 旋转（deg）",exact:true})).toHaveValue("90");
  await dialog.getByRole("button",{name:/^取\s*消$/}).click();
  await filter.fill("ANGLE");
  const angle=page.getByRole("treeitem").filter({hasText:/#ANGLE\./});
  await angle.dblclick();
  await expect(dialog.getByText("尚未选择参考轴")).toHaveCount(0);
  await dialog.getByRole("switch",{name:"反转参考轴"}).click();
  await dialog.getByRole("spinbutton",{name:"* 角度（deg）",exact:true}).fill("270");
  await dialog.getByRole("spinbutton",{name:"* 角度（deg）",exact:true}).press("Tab");
  await dialog.getByRole("button",{name:/^确\s*定$/}).click();
  await expect(dialog).toHaveCount(0);
  await angle.dblclick();
  await expect(dialog.getByRole("switch",{name:"反转参考轴"})).toBeChecked();
  await expect(dialog.getByRole("spinbutton",{name:"* 角度（deg）",exact:true})).toHaveValue("270.000");
  expect(errors).toEqual([]);
});
