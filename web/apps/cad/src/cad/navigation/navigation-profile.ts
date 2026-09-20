import type { InputState, PointerButtons } from "../input/input-types";

export type NavigationAction = "none" | "orbit" | "pan" | "zoom" | "roll";
export type NavigationProfileID = "default" | "catia" | "solidworks";

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
  readonly label = "3DEXPERIENCE CATIA";
  // CATIA depends on edge ordering and movement thresholds, so the profile is a
  // selectable identity only; CatiaNavigationController owns its state machine.
  pointerAction(_buttons: PointerButtons, _state: InputState): NavigationAction { return "none"; }
  wheelAction(_state: InputState): NavigationAction { return "none"; }
}

export class SolidWorksNavigationProfile implements NavigationProfile {
  readonly id = "solidworks" as const;
  readonly label = "SOLIDWORKS";
  pointerAction(buttons: PointerButtons, state: InputState): NavigationAction {
    if (!buttons.middle) return "none";
    if (state.modifiers.ctrl && state.modifiers.alt) return "none";
    if (state.modifiers.ctrl) return "pan";
    if (state.modifiers.shift) return "zoom";
    if (state.modifiers.alt) return "roll";
    return "orbit";
  }
  wheelAction(_state: InputState): NavigationAction { return "zoom"; }
}

export function createNavigationProfile(id: NavigationProfileID): NavigationProfile {
  return id === "solidworks" ? new SolidWorksNavigationProfile() : id === "catia" ? new CatiaNavigationProfile() : new DefaultNavigationProfile();
}
