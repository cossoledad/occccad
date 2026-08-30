import { Button, Tooltip } from "antd";
import type { ReactNode } from "react";
import { useEffect, useRef } from "react";
import { useCommandRegistry, useCommandState } from "../command/command-context";
import { useUIHelp } from "../help/ui-help-context";

export type ToolButtonProps = {
  command: string;
  icon: ReactNode;
  tooltip: ReactNode;
	toolbarName?: string;
	helpText?: string;
  className?: string;
  repeatable?: boolean;
};

export function ToolButton({ command, icon, tooltip, toolbarName = "", helpText = "", className = "", repeatable = false }: ToolButtonProps) {
  const registry = useCommandRegistry();
	const uiHelp = useUIHelp();
  const state = useCommandState(command);
  const singleClick = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(singleClick.current), []);
  if (!state.visible) return null;
  const click = () => {
	if (uiHelp.active) { uiHelp.explain({ toolbarName, commandName: String(tooltip), helpText }); return; }
    if (!repeatable) { void registry.execute(command); return; }
    window.clearTimeout(singleClick.current);
    singleClick.current = window.setTimeout(() => void registry.execute(command, { continuous: false }), 220);
  };
  const doubleClick = () => {
    if (uiHelp.active) return;
    if (!repeatable) return;
    window.clearTimeout(singleClick.current);
    void registry.execute(command, { continuous: true });
  };
  const button = <Button className={`cad-tool-button ${state.active ? "active" : ""} ${className}`.trim()}
	type={state.active ? "primary" : "default"} icon={icon} disabled={!state.enabled && !uiHelp.active} aria-pressed={state.active}
    aria-label={typeof tooltip === "string" ? tooltip.split(" · ")[0] : undefined}
    onClick={click} onDoubleClick={doubleClick} />;
	return tooltip ? <Tooltip title={tooltip} mouseEnterDelay={0.45} overlayClassName="cad-short-tooltip">{button}</Tooltip> : button;
}
