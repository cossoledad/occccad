import type { InputState } from "../input/input-types";
import type { NavigationAction, NavigationProfileID } from "../navigation/navigation-profile";
import type { NavigationSnapshot } from "../navigation/navigation-controller";
import { FloatingPanel } from "./floating-panel";
import type { SceneGridDisplay } from "../rendering/scene-ground";

export type InputDebugSnapshot = {
  sceneGrid?:SceneGridDisplay;
  input: InputState;
  activeTool: string;
  selectionKeys?: string[];
  highlightedVisible?: number;
  featureSelection?:{role:string;count:number;overlays:number};
  datumPreview?:{kind:string;origin:[number,number,number];direction:[number,number,number]};
  navigationProfile: NavigationProfileID;
  navigationAction: NavigationAction;
  navigation?: NavigationSnapshot;
  hudScreen?: { x: number; y: number };
  viewTransitionActive?: boolean;
};

export function InputDebugOverlay({ snapshot }: { snapshot: InputDebugSnapshot }) {
  const { buttons, modifiers } = snapshot.input;
  const catia = snapshot.navigation?.catia;
  return <FloatingPanel position="bottom-left" title="INPUT DEBUG" className="cad-input-debug">
    <code>L: {buttons.left ? "down" : "up"} · M: {buttons.middle ? "down" : "up"} · R: {buttons.right ? "down" : "up"}</code>
    <code>Ctrl: {String(modifiers.ctrl)} · Shift: {String(modifiers.shift)} · Alt: {String(modifiers.alt)}</code>
    <code>Tool: {snapshot.activeTool} · Nav: {snapshot.navigationProfile}/{snapshot.navigationAction}</code>
    <code data-testid="viewport-selection" data-count={snapshot.selectionKeys?.length ?? 0} data-highlighted={snapshot.highlightedVisible ?? 0}>Selection: {snapshot.selectionKeys?.join(", ") || "none"}</code>
    {snapshot.sceneGrid&&<code data-testid="scene-grid" data-state={JSON.stringify(snapshot.sceneGrid)}>Grid: {snapshot.sceneGrid.sketch?"sketch":snapshot.sceneGrid.ground?"ground":"hidden"}</code>}
    {snapshot.featureSelection&&<code data-testid="viewport-feature-selection" data-role={snapshot.featureSelection.role} data-count={snapshot.featureSelection.count} data-overlays={snapshot.featureSelection.overlays}>Feature selection: {snapshot.featureSelection.role} / {snapshot.featureSelection.count}</code>}
    {snapshot.datumPreview && <code data-testid="viewport-datum-preview" data-kind={snapshot.datumPreview.kind}>Datum: {JSON.stringify(snapshot.datumPreview)}</code>}
    <code data-testid="navigation-camera" data-transition-active={String(Boolean(snapshot.viewTransitionActive))} data-camera-state={snapshot.navigation?JSON.stringify({position:snapshot.navigation.cameraPosition,quaternion:snapshot.navigation.cameraQuaternion,zoom:snapshot.navigation.cameraZoom,projection:snapshot.navigation.projection,fov:snapshot.navigation.cameraFov,aspect:snapshot.navigation.cameraAspect,span:snapshot.navigation.cameraSpan}):undefined}>Camera: {snapshot.navigation?.cameraPosition.map((v) => v.toFixed(4)).join(",")} / {snapshot.navigation?.cameraQuaternion.map((v) => v.toFixed(4)).join(",")} / zoom {snapshot.navigation?.cameraZoom.toPrecision(10)}</code>
      <code data-testid="navigation-projection">{snapshot.navigation?.projection} zoom: {snapshot.navigation?.cameraZoom.toPrecision(10)}</code>
    {snapshot.navigation?.solidworks && <code>Reference: {snapshot.navigation.solidworks.reference?.highlight?.kind ?? "none"}</code>}
    {catia && <>
      <code>CATIA: {catia.state} · Pivot: {catia.pivotSource}</code>
      <code>P: {catia.pivot.x.toFixed(3)}, {catia.pivot.y.toFixed(3)}, {catia.pivot.z.toFixed(3)}</code>
      <code>Hit: {catia.hitObject ?? "none"} · Distance: {catia.cameraDistance.toFixed(3)}</code>
      <code>HUD: {catia.hudVisible&&snapshot.hudScreen ? `${snapshot.hudScreen.x.toFixed(1)}, ${snapshot.hudScreen.y.toFixed(1)}` : "hidden"}</code>
      <code data-testid="navigation-pivot" data-visible={String(catia.showPivot)}>Centre: {catia.pivot.toArray().map(v=>v.toFixed(6)).join(",")} · {catia.hitObject??"existing"}</code>
    </>}
  </FloatingPanel>;
}
