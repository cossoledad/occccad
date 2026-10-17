import type { SurfaceStyle } from "../../types";
import { palette } from "../../design/visual-tokens";
import type { SceneSettings } from "../../cad/rendering/scene-settings";
import type { SolidDisplaySettings, ReferenceVisibility, GridVisibility } from "../../cad/rendering/display-settings";
import type { CaptureSettings } from "../../cad/interaction/capture-settings";
import type { NavigationProfileID } from "../../cad/navigation/navigation-profile";
import type { DisplayLengthUnit } from "../../state/ui-preferences";

export type CreationDefaults={solid:Required<Pick<SurfaceStyle,"color"|"roughness"|"metalness">>};
export const DEFAULT_CREATION_DEFAULTS:CreationDefaults={solid:{color:palette.solid,roughness:.5,metalness:.04}};
export function normalizeCreationDefaults(value:unknown):CreationDefaults {
 const solid=(value as Partial<CreationDefaults>|undefined)?.solid;
 const coefficient=(v:unknown,fallback:number)=>typeof v==="number"&&Number.isFinite(v)?Math.max(0,Math.min(1,v)):fallback;
 return {solid:{color:typeof solid?.color==="string"&&/^#[0-9a-fA-F]{6}$/.test(solid.color)?solid.color.toLowerCase():DEFAULT_CREATION_DEFAULTS.solid.color,
  roughness:coefficient(solid?.roughness,.5),metalness:coefficient(solid?.metalness,.04)}};
}
export type GlobalOptionsSnapshot={
 sceneSettings:SceneSettings;creationDefaults:CreationDefaults;
 solidDisplay:SolidDisplaySettings;referenceVisibility:ReferenceVisibility;gridVisibility:GridVisibility;
 navigationProfile:NavigationProfileID;catiaRotationSphereVisible:boolean;captureSettings:CaptureSettings;
 displayLengthUnit:DisplayLengthUnit;documentLengthUnits:Record<string,DisplayLengthUnit>;
};
export function globalOptionsSnapshot(value:GlobalOptionsSnapshot):GlobalOptionsSnapshot {
 return structuredClone({sceneSettings:value.sceneSettings,creationDefaults:value.creationDefaults,solidDisplay:value.solidDisplay,
  referenceVisibility:value.referenceVisibility,gridVisibility:value.gridVisibility,navigationProfile:value.navigationProfile,
  catiaRotationSphereVisible:value.catiaRotationSphereVisible,captureSettings:value.captureSettings,
  displayLengthUnit:value.displayLengthUnit,documentLengthUnits:value.documentLengthUnits});
}
