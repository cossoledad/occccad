import { colorNumber, palette,studioMaterial } from "../../design/visual-tokens";

// Kept as the established theme entrypoint for all renderer primitives.
export const CATIA_VISUAL_THEME = {
  backgroundTop: colorNumber(palette.viewportTop), backgroundBottom: colorNumber(palette.viewportBottom), backgroundVignette: 0.12,
  surface: colorNumber(palette.solid), productSurface: colorNumber(palette.productSolid), placeholder: 0x8493a3,
  engineeringCurve: 0xa99bff, engineeringCurveLight: 0x6745a3, engineeringSurface: 0x819db4, engineeringPoint: 0xffffff,
  edge: colorNumber(palette.edge), vertex: colorNumber(palette.vertex),
  sketchProfile: colorNumber(palette.sketch), sketchConstruction: colorNumber(palette.construction), sketchExternal: colorNumber(palette.external),
  sketchSolved: colorNumber(palette.solved), sketchInvalid: colorNumber(palette.invalid), sketchRedundant: colorNumber(palette.redundant),
  sketchGrid: colorNumber(palette.sketchGrid),
  sketchPointHalo:colorNumber(palette.sketchPointHalo),
  constraint: colorNumber(palette.solved), gridMinor: colorNumber(palette.gridMinor), gridMajor: colorNumber(palette.gridMajor),
  reference:colorNumber(palette.reference),preview: colorNumber(palette.preview), commandPreview: colorNumber(palette.preview), snap: colorNumber(palette.snap),
  analysisClearance:colorNumber(palette.solved),analysisClash:colorNumber(palette.danger),analysisContact:colorNumber(palette.warning),
  hover: colorNumber(palette.hover), selected: colorNumber(palette.selected), selectedEmissive: 0x49331c,
  axisX: colorNumber(palette.axisX), axisY: colorNumber(palette.axisY), axisZ: colorNumber(palette.axisZ),
  navigation: colorNumber(palette.selected), navigationHighlight: colorNumber(palette.snap),
  // Satin dielectric: broad highlights must not exaggerate tessellation normals.
  surfaceRoughness:studioMaterial.roughness, surfaceMetalness:studioMaterial.metalness, surfaceEnvironmentIntensity:studioMaterial.environmentIntensity, datumOpacity: 0.07,
  lightSky: 0xf7f8fa, lightGround: 0x747b82, lightKey: 0xffffff, lightFill: 0xf0f3f6, lightRim: 0xffffff,
  hemisphereIntensity: 0.28, keyIntensity: 1.35, fillIntensity: 0.18, rimIntensity: 0.14,
} as const;
export type CadVisualTheme = {[K in keyof typeof CATIA_VISUAL_THEME]:number};
