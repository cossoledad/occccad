import { Button, Tooltip } from "antd";
import type { ReactNode } from "react";
import { useEffect, useRef } from "react";
import { useCommandRegistry, useCommandState } from "../command/command-context";

export type ToolButtonProps = {
  command: string;
  icon: ReactNode;
  tooltip: ReactNode;
  className?: string;
  repeatable?: boolean;
};

export function ToolButton({ command, icon, tooltip, className = "", repeatable = false }: ToolButtonProps) {
  const registry = useCommandRegistry();
  const state = useCommandState(command);
  const singleClick = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(singleClick.current), []);
  if (!state.visible) return null;
  const click = () => {
    if (!repeatable) { void registry.execute(command); return; }
    window.clearTimeout(singleClick.current);
    singleClick.current = window.setTimeout(() => void registry.execute(command, { continuous: false }), 220);
  };
  const doubleClick = () => {
    if (!repeatable) return;
    window.clearTimeout(singleClick.current);
    void registry.execute(command, { continuous: true });
  };
  const button = <Button className={`cad-tool-button ${state.active ? "active" : ""} ${className}`.trim()}
    type={state.active ? "primary" : "default"} icon={icon} disabled={!state.enabled} aria-pressed={state.active}
    aria-label={typeof tooltip === "string" ? tooltip.split(" · ")[0] : undefined}
    onClick={click} onDoubleClick={doubleClick} />;
  return tooltip ? <Tooltip title={tooltip}>{button}</Tooltip> : button;
}
