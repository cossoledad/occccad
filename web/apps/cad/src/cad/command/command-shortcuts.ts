export type CommandShortcut = { command: string; key: string; modifier?: boolean; shift?: boolean; display: string; aria: string };
export const COMMAND_SHORTCUTS: readonly CommandShortcut[] = [
  { command: "edit.delete", key: "delete", display: "Delete", aria: "Delete" },
  { command: "edit.undo", key: "z", modifier: true, display: "Ctrl / ⌘ + Z", aria: "Control+z Meta+z" },
  { command: "edit.redo", key: "y", modifier: true, display: "Ctrl / ⌘ + Y", aria: "Control+y Meta+y" },
  { command: "edit.redo", key: "z", modifier: true, shift: true, display: "Ctrl / ⌘ + Shift + Z", aria: "Control+Shift+z Meta+Shift+z" },
  { command: "ui.command-search", key: "k", modifier: true, display: "Ctrl / ⌘ + K", aria: "Control+k Meta+k" },
  { command: "product.pattern", key: "e", modifier: true, display: "Ctrl / ⌘ + E", aria: "Control+e Meta+e" },
  { command: "tool.select", key: "v", display: "V", aria: "v" },
  { command: "view.fit", key: "f", display: "F", aria: "f" },
  { command: "view.iso", key: "1", display: "1", aria: "1" },
  { command: "view.front", key: "2", display: "2", aria: "2" },
  { command: "view.top", key: "3", display: "3", aria: "3" },
  { command: "view.right", key: "4", display: "4", aria: "4" },
  { command: "sketch.line", key: "l", display: "L", aria: "l" },
  { command: "sketch.circle", key: "c", display: "C", aria: "c" },
  { command: "sketch.rectangle", key: "r", display: "R", aria: "r" },
  { command: "ui.shortcut-help", key: "?", shift: true, display: "Shift + /", aria: "Shift+/" },
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
