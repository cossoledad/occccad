import type { ToolButtonProps } from "./tool-button";
import { ToolButton } from "./tool-button";
import { FloatingToolbar, ToolbarGroup } from "./floating-panel";

export function ContextToolbar({ commands }: { commands: ToolButtonProps[] }) {
  if (commands.length === 0) return null;
  return <FloatingToolbar id="context" position="bottom-center" className="cad-context-toolbar">
    <ToolbarGroup>{commands.map((command) => <ToolButton key={command.command} {...command} />)}</ToolbarGroup>
  </FloatingToolbar>;
}
