import type { InputState } from "../input/input-types";
import type { NavigationAction, NavigationProfileID } from "../navigation/navigation-profile";
import type { NavigationSnapshot } from "../navigation/navigation-controller";
import { FloatingPanel } from "./floating-panel";

export type InputDebugSnapshot = {
  input: InputState;
  activeTool: string;
  navigationProfile: NavigationProfileID;
  navigationAction: NavigationAction;
  navigation?: NavigationSnapshot;
  hudScreen?: { x: number; y: number };
};

export function InputDebugOverlay({ snapshot }: { snapshot: InputDebugSnapshot }) {
  const { buttons, modifiers } = snapshot.input;
  const catia = snapshot.navigation?.catia;
  return <FloatingPanel position="bottom-left" title="INPUT DEBUG" className="cad-input-debug">
    <code>L: {buttons.left ? "down" : "up"} · M: {buttons.middle ? "down" : "up"} · R: {buttons.right ? "down" : "up"}</code>
    <code>Ctrl: {String(modifiers.ctrl)} · Shift: {String(modifiers.shift)} · Alt: {String(modifiers.alt)}</code>
    <code>Tool: {snapshot.activeTool} · Nav: {snapshot.navigationProfile}/{snapshot.navigationAction}</code>
    {catia && <>
      <code>CATIA: {catia.state} · Pivot: {catia.pivotSource}</code>
      <code>P: {catia.pivot.x.toFixed(3)}, {catia.pivot.y.toFixed(3)}, {catia.pivot.z.toFixed(3)}</code>
      <code>Hit: {catia.hitObject ?? "none"} · Distance: {catia.cameraDistance.toFixed(3)}</code>
      <code>HUD: {snapshot.hudScreen ? `${snapshot.hudScreen.x.toFixed(1)}, ${snapshot.hudScreen.y.toFixed(1)}` : "hidden"}</code>
    </>}
  </FloatingPanel>;
}
