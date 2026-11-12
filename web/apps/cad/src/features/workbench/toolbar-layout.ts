import type { ToolbarCatalogItem } from "../../types";
import type { ToolbarVariantGroup } from "./toolbar-variants";

export const toolbarMetrics = { rows: 2, rowHeight: 32, rowGap: 6, columnGap: 6, groupGap: 10, groupPadding: 8, arrowWidth: 22, iconWidth: 40, moreWidth: 64 } as const;
export type ToolbarLayoutInput = { id: string; name: string; variants: (ToolbarVariantGroup & { displayCommandId?: string })[] };
export type ToolbarControl = { key: string; toolbarName: string; family: ToolbarVariantGroup; items: ToolbarCatalogItem[]; width: number; iconOnly: boolean };
export type ToolbarLayoutGroup = { id: string; name: string; label: string; sourceIds: string[]; controls: ToolbarControl[]; columns: number[]; width: number };
export type ToolbarLayout = { rows: 2; groups: ToolbarLayoutGroup[]; overflowGroups: ToolbarLayoutInput[]; width: number };
export const approximateToolbarTextWidth = (text: string): number => [...text].reduce((width, char) => width + (char.codePointAt(0)! > 255 ? 12 : 6.5), 0);

/** Presentation only. Keep two comfortable rows and declared variant families.
 * Dense groups may use icon buttons without shrinking their hit targets. Only
 * when those cannot fit do trailing groups move to the overflow menu. */
export function layoutToolbar(input: ToolbarLayoutInput[], availableWidth: number,
  measure: (text: string) => number = approximateToolbarTextWidth): ToolbarLayout {
  const groups = input.filter(group => group.variants.length);
  const measured = new Map<string, number>();
  const textWidth = (text: string) => { if (!measured.has(text)) measured.set(text, measure(text)); return measured.get(text)!; };
  const compact = new Set<string>();
  const build = (visible: ToolbarLayoutInput[], overflowGroups: ToolbarLayoutInput[] = []): ToolbarLayout => {
    const source = visible.map(group => {
      const controls = group.variants.map(family => {
        const iconOnly = compact.has(group.id);
        const item = family.items.find(item => item.commandId === family.displayCommandId) ?? family.items[0];
        const width = (iconOnly ? toolbarMetrics.iconWidth : Math.max(72, Math.ceil(textWidth(item.name)) + 44))
          + (family.items.length > 1 ? toolbarMetrics.arrowWidth : 0);
        return { key: family.key, toolbarName: group.name, family, items: family.items, width, iconOnly };
      });
      return { id: group.id, name: group.name, label: group.name, sourceIds: [group.id], controls, small: controls.length <= 2 };
    });
    const packed: typeof source = [];
    for (const group of source) {
      const previous = packed.at(-1);
      if (group.small && previous?.small) {
        previous.id += `+${group.id}`; previous.name = ""; previous.label += ` · ${group.name}`;
        previous.sourceIds.push(...group.sourceIds); previous.controls.push(...group.controls);
      } else packed.push(group);
    }
    const result = packed.map(group => {
      const columns: number[] = [];
      for (let offset = 0; offset < group.controls.length; offset += 2)
        columns.push(Math.max(...group.controls.slice(offset, offset + 2).map(control => control.width)));
      const width = Math.max(Math.ceil(textWidth(group.name) * 11 / 12), columns.reduce((sum, value) => sum + value, 0) + Math.max(0, columns.length - 1) * toolbarMetrics.columnGap)
        + toolbarMetrics.groupPadding * 2 + 1;
      return { id: group.id, name: group.name, label: group.label, sourceIds: group.sourceIds, controls: group.controls, columns, width };
    });
    const width = result.reduce((sum, group) => sum + group.width, 0) + Math.max(0, result.length - 1) * toolbarMetrics.groupGap
      + (overflowGroups.length ? toolbarMetrics.moreWidth + (result.length ? toolbarMetrics.groupGap : 0) : 0);
    return { rows: 2, groups: result, overflowGroups, width };
  };
  let plan = build(groups);
  if (availableWidth <= 0 || plan.width <= availableWidth) return plan;
  // Keep the rectangle family label visible so its temporary default is explicit.
  const candidates = groups.filter(group => !group.variants.some(family => family.key === "rectangle"));
  while (plan.width > availableWidth) {
    let best: string | undefined, saving = 0;
    for (const group of candidates) {
      if (compact.has(group.id)) continue;
      compact.add(group.id); const candidate = build(groups); compact.delete(group.id);
      if (plan.width - candidate.width > saving) { best = group.id; saving = plan.width - candidate.width; }
    }
    if (!best) break;
    compact.add(best); plan = build(groups);
  }
  let visible = groups;
  while (plan.width > availableWidth && visible.length) {
    visible = visible.slice(0, -1);
    plan = build(visible, groups.slice(visible.length));
  }
  // Reclaim labels after moving a group, instead of leaving unnecessary density.
  for (const id of [...compact].reverse()) {
    compact.delete(id);
    const candidate = build(visible, groups.slice(visible.length));
    if (candidate.width <= availableWidth) plan = candidate;
    else compact.add(id);
  }
  return plan;
}
