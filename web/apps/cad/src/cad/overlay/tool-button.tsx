import { commandShortcutLabel, commandShortcutAria } from "../command/command-shortcuts";
import { App, Button } from "antd";
import type { ReactNode } from "react";
import { useEffect, useRef } from "react";
import { useCommandRegistry, useCommandState } from "../command/command-context";
import { useUIHelp } from "../help/ui-help-context";
import { CursorTooltip } from "./cursor-tooltip";

export type ToolButtonProps = {
  command: string;
  icon: ReactNode;
  tooltip: ReactNode;
	toolbarName?: string;
	helpText?: string;
  className?: string;
  repeatable?: boolean;
  showLabel?: boolean;
};

export function ToolButton({ command, icon, tooltip, toolbarName = "", helpText = "", className = "", repeatable = false, showLabel = false }: ToolButtonProps) {
  const registry = useCommandRegistry();
  const { message } = App.useApp();
	const uiHelp = useUIHelp();
  const state = useCommandState(command);
  const singleClick = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(singleClick.current), [command, state.enabled, state.visible, uiHelp.active]);
  const execute = (continuous = false) => {
    void registry.execute(command, { continuous }).catch((error) => message.error(String(error)));
  };
  if (!state.visible) return null;
  const click = () => {
	if (uiHelp.active) { uiHelp.explain({ toolbarName, commandName: String(tooltip), helpText }); return; }
    if (!repeatable) { execute(); return; }
    window.clearTimeout(singleClick.current);
    singleClick.current = window.setTimeout(() => execute(), 220);
  };
  const doubleClick = () => {
    if (uiHelp.active) return;
    if (!repeatable) return;
    window.clearTimeout(singleClick.current);
    execute(true);
  };
  const shortcut = commandShortcutLabel(command);
  const button = <Button aria-keyshortcuts={commandShortcutAria(command)} className={`cad-tool-button ${showLabel ? "with-label" : ""} ${state.active ? "active" : ""} ${className}`.trim()}
	type={state.active ? "primary" : "default"} icon={icon} disabled={!state.enabled && !uiHelp.active} aria-pressed={state.active}
    aria-label={typeof tooltip === "string" ? tooltip.split(" · ")[0] : undefined}
    onClick={click} onDoubleClick={doubleClick}>{showLabel ? tooltip : null}</Button>;
	return tooltip ? <CursorTooltip title={shortcut ? <>{tooltip}<kbd>{shortcut}</kbd></> : tooltip} disabled={uiHelp.active}>{button}</CursorTooltip> : button;
}
