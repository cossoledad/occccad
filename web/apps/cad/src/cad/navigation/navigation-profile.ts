import type { InputState, PointerButtons } from "../input/input-types";

export type NavigationAction = "none" | "orbit" | "pan" | "zoom";
export type NavigationProfileID = "default" | "catia";

export interface NavigationProfile {
  readonly id: NavigationProfileID;
  readonly label: string;
  pointerAction(buttons: PointerButtons, state: InputState): NavigationAction;
  wheelAction(state: InputState): NavigationAction;
}

export class DefaultNavigationProfile implements NavigationProfile {
  readonly id = "default" as const;
  readonly label = "occccad Default";
  pointerAction(buttons: PointerButtons, _state: InputState): NavigationAction {
    if (buttons.right) return "orbit";
    if (buttons.middle) return "pan";
    return "none";
  }
  wheelAction(_state: InputState): NavigationAction { return "zoom"; }
}

export class CatiaNavigationProfile implements NavigationProfile {
  readonly id = "catia" as const;
  readonly label = "CATIA V5";
  // CATIA depends on edge ordering and movement thresholds, so the profile is a
  // selectable identity only; CatiaNavigationController owns its state machine.
  pointerAction(_buttons: PointerButtons, _state: InputState): NavigationAction { return "none"; }
  wheelAction(_state: InputState): NavigationAction { return "zoom"; }
}

export function createNavigationProfile(id: NavigationProfileID): NavigationProfile {
  return id === "catia" ? new CatiaNavigationProfile() : new DefaultNavigationProfile();
}
