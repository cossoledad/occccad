export type CommandShortcut = { command: string; label: string; key: string; modifier?: boolean; shift?: boolean; display: string; aria: string };
export const COMMAND_SHORTCUTS: readonly CommandShortcut[] = [
  { command: "edit.undo", label: "撤销", key: "z", modifier: true, display: "Ctrl / ⌘ + Z", aria: "Control+z Meta+z" },
  { command: "edit.redo", label: "重做", key: "y", modifier: true, display: "Ctrl / ⌘ + Y", aria: "Control+y Meta+y" },
  { command: "edit.redo", label: "重做（另一组合）", key: "z", modifier: true, shift: true, display: "Ctrl / ⌘ + Shift + Z", aria: "Control+Shift+z Meta+Shift+z" },
  { command: "ui.command-search", label: "搜索工具", key: "k", modifier: true, display: "Ctrl / ⌘ + K", aria: "Control+k Meta+k" },
  { command: "tool.select", label: "选择工具", key: "v", display: "V", aria: "v" },
  { command: "view.fit", label: "适合窗口", key: "f", display: "F", aria: "f" },
  { command: "view.iso", label: "等轴测", key: "1", display: "1", aria: "1" },
  { command: "view.front", label: "前视图", key: "2", display: "2", aria: "2" },
  { command: "view.top", label: "顶视图", key: "3", display: "3", aria: "3" },
  { command: "view.right", label: "右视图", key: "4", display: "4", aria: "4" },
  { command: "sketch.line", label: "草图 · 直线", key: "l", display: "L", aria: "l" },
  { command: "sketch.circle", label: "草图 · 圆", key: "c", display: "C", aria: "c" },
  { command: "sketch.rectangle", label: "草图 · 矩形", key: "r", display: "R", aria: "r" },
  { command: "ui.shortcut-help", label: "快捷键帮助", key: "?", shift: true, display: "Shift + /", aria: "Shift+/" },
];
export type ShortcutKeyEvent = Pick<KeyboardEvent, "key" | "ctrlKey" | "metaKey" | "shiftKey" | "altKey" | "repeat" | "isComposing">;
export function resolveCommandShortcut(event: ShortcutKeyEvent): CommandShortcut | undefined {
  if (event.isComposing || event.repeat || event.altKey || (event.ctrlKey && event.metaKey)) return undefined;
  return COMMAND_SHORTCUTS.find((shortcut) => shortcut.key === event.key.toLowerCase()
    && Boolean(shortcut.modifier) === (event.ctrlKey || event.metaKey) && Boolean(shortcut.shift) === event.shiftKey);
}
export function commandShortcutLabel(command: string): string | undefined {
  return COMMAND_SHORTCUTS.find((shortcut) => shortcut.command === command)?.display;
}
export function commandShortcutAria(command: string): string | undefined {
  return COMMAND_SHORTCUTS.filter((shortcut) => shortcut.command === command).map((shortcut) => shortcut.aria).join(" ") || undefined;
}
