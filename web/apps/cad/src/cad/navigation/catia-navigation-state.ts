export enum CatiaNavigationState {
  Idle = "idle",
  MiddlePending = "middle-pending",
  Pan = "pan",
  Rotate = "rotate",
  ZoomArmed = "zoom-armed",
  Zoom = "zoom",
}

export type CatiaAuxiliaryButton = "left" | "right";
export type NavigationPivotSource = "existing" | "raycast" | "view-plane";
