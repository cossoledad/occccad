import { Button, Tooltip } from "antd";
import type { ReactNode } from "react";
import { useCommandRegistry, useCommandState } from "../command/command-context";

export type ToolButtonProps = {
  command: string;
  icon: ReactNode;
  tooltip: ReactNode;
  label?: ReactNode;
  className?: string;
};

export function ToolButton({ command, icon, tooltip, label, className = "" }: ToolButtonProps) {
  const registry = useCommandRegistry();
  const state = useCommandState(command);
  if (!state.visible) return null;
  const button = <Button className={`cad-tool-button ${state.active ? "active" : ""} ${className}`.trim()}
    type={state.active ? "primary" : "default"} icon={icon} disabled={!state.enabled} aria-pressed={state.active}
    onClick={() => void registry.execute(command)}>{label}</Button>;
  return tooltip ? <Tooltip title={tooltip}>{button}</Tooltip> : button;
}

