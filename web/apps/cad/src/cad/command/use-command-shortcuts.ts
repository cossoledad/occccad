import { App } from "antd";
import { useEffect } from "react";
import { useUIHelp } from "../help/ui-help-context";
import type { CommandRegistry } from "./command-registry";
import { resolveCommandShortcut } from "./command-shortcuts";

export function useCommandShortcuts(registry: CommandRegistry): void {
  const { message } = App.useApp();
  const help = useUIHelp();
  useEffect(() => {
    const keyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || help.active) return;
      const target = event.target instanceof HTMLElement ? event.target : undefined;
      if (target?.isContentEditable || target?.closest("input, textarea, select, [contenteditable='true'], [role='textbox'], [role='combobox']")) return;
      // Nonmodal command panels also own their input session; do not change history underneath them.
      if ([...document.querySelectorAll<HTMLElement>('[role="dialog"]')].some((dialog) => dialog.getClientRects().length > 0)) return;
      const shortcut = resolveCommandShortcut(event);
      if (!shortcut || !registry.state(shortcut.command).visible) return;
      event.preventDefault(); event.stopPropagation();
      void registry.execute(shortcut.command, { continuous: false }).catch((error) => message.error(String(error)));
    };
    window.addEventListener("keydown", keyDown, true);
    return () => window.removeEventListener("keydown", keyDown, true);
  }, [registry, message, help.active]);
}
