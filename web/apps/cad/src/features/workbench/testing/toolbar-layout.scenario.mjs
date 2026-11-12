import assert from "node:assert/strict";
import { createServer } from "vite";
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const { layoutToolbar, toolbarMetrics } = await server.ssrLoadModule("/src/features/workbench/toolbar-layout.ts");
  const { toolbarVariantGroups } = await server.ssrLoadModule("/src/features/workbench/toolbar-variants.ts");
  const { builtinCatalog, projectToolbars } = await server.ssrLoadModule("/src/cad/command/workbench-catalog.ts");
  const toolbars = projectToolbars(builtinCatalog);
  const flatten = plan => [...plan.groups.flatMap(group => group.controls.flatMap(control => control.items.map(item => item.commandId))),
    ...plan.overflowGroups.flatMap(group => group.variants.flatMap(family => family.items.map(item => item.commandId)))];
  for (const tab of builtinCatalog.tabs) {
    const input = toolbars.filter(bar => bar.tabIds?.includes(tab.id)).map(bar => ({ id: bar.id, name: bar.name, variants: toolbarVariantGroups(bar.items) }));
    const expected = input.flatMap(group => group.variants.flatMap(family => family.items.map(item => item.commandId)));
    for (const width of [1920, 1440, 1280, 1024, 800, 600, 320]) {
      const plan = layoutToolbar(input, width);
      assert.deepEqual(flatten(plan), expected, `${tab.id}: resizing never drops, duplicates or reorders commands`);
      assert.deepEqual([...plan.groups.flatMap(group => group.sourceIds), ...plan.overflowGroups.map(group => group.id)], input.map(group => group.id), "column sharing preserves source groups and help context");
      assert.equal(plan.rows, 2, "every category and width keeps exactly two rows");
      assert(plan.width <= width, `${tab.id}: all controls and the overflow entry fit without a scrollbar at ${width}`);
      for (const group of plan.groups) for (const control of group.controls) assert(control.width >= toolbarMetrics.iconWidth, "density never shrinks hit targets");
      if (["PART_DESIGN.model", "ASSEMBLY_DESIGN.model"].includes(tab.id) && width >= 1280) assert.equal(plan.overflowGroups.length, 0, `${tab.id} has direct access to every command`);
    }
    const wide = layoutToolbar(input, 10000);
    assert.equal(wide.groups.flatMap(group => group.controls).length, input.reduce((sum, group) => sum + group.variants.length, 0), "wide screens keep related variants together");
  }
  const input = [{ id: "profile", name: "轮廓", variants: toolbarVariantGroups([
    { commandId: "circle", name: "圆", groupKey: "variants:circle", variantLabel: "圆" },
    { commandId: "three-point", name: "三点圆", groupKey: "variants:circle", variantLabel: "圆" },
    { commandId: "spline", name: "控制点样条", groupKey: "primary" },
  ]) }];
  const calls = new Map();
  const dense = layoutToolbar(input, 160, name => { calls.set(name, (calls.get(name) ?? 0) + 1); return name.length * 12; });
  assert([...calls.values()].every(count => count === 1), "adaptive attempts measure each label once");
  assert.equal(dense.groups[0].controls.at(-1).items[0].commandId, "spline");
  assert(!layoutToolbar(input, 1000).overflow);
  assert.deepEqual(layoutToolbar([], 500).groups, []);
  assert.deepEqual(flatten(layoutToolbar(input, 0)), ["circle", "three-point", "spline"], "initial unmeasured rendering preserves accessibility");
  const rectangle = [{ id: "shapes", name: "轮廓", variants: [{ key: "rectangle", label: "矩形", items: [
    { commandId: "rectangle", name: "矩形" }, { commandId: "oriented", name: "定向矩形" }, { commandId: "center", name: "中心矩形" },
  ], displayCommandId: "center" }] }];
  const rectanglePlan = layoutToolbar(rectangle, 1920);
  assert.equal(rectanglePlan.groups[0].controls.length, 1, "rectangle variants always use one split button");
  assert.equal(rectanglePlan.groups[0].controls[0].items.length, 3);
  assert(rectanglePlan.groups[0].controls[0].width > layoutToolbar([{...rectangle[0],variants:[{...rectangle[0].variants[0],displayCommandId:"rectangle"}]}],1920).groups[0].controls[0].width, "session default is measured without truncating longer labels");
  assert.equal(rectanglePlan.groups[0].controls[0].iconOnly, false);
  assert(toolbarMetrics.rowHeight >= 32, "comfortable button height is preserved");
  console.log("Two-row toolbar: complete catalog, comfortable targets, grouped variants and no strip overflow passed");
} finally { await server.close(); }
