import { colorNumber, palette } from "../../design/visual-tokens";

// Kept as the established theme entrypoint for all renderer primitives.
export const CATIA_VISUAL_THEME = {
  backgroundTop: colorNumber(palette.viewportTop), backgroundBottom: colorNumber(palette.viewportBottom), backgroundVignette: 0.12,
  surface: colorNumber(palette.solid), productSurface: colorNumber(palette.productSolid), placeholder: 0x8493a3,
  edge: colorNumber(palette.edge), vertex: colorNumber(palette.vertex),
  sketchProfile: colorNumber(palette.sketch), sketchConstruction: colorNumber(palette.construction), sketchExternal: colorNumber(palette.external),
  sketchSolved: colorNumber(palette.solved), sketchInvalid: colorNumber(palette.invalid), sketchRedundant: colorNumber(palette.redundant),
  sketchGrid: colorNumber(palette.sketchGrid),
  constraint: colorNumber(palette.solved), gridMinor: colorNumber(palette.gridMinor), gridMajor: colorNumber(palette.gridMajor),
  preview: colorNumber(palette.preview), commandPreview: colorNumber(palette.preview), snap: colorNumber(palette.snap),
  hover: colorNumber(palette.hover), selected: colorNumber(palette.selected), selectedEmissive: 0x49331c,
  axisX: colorNumber(palette.axisX), axisY: colorNumber(palette.axisY), axisZ: colorNumber(palette.axisZ),
  navigation: colorNumber(palette.selected), navigationHighlight: colorNumber(palette.snap),
  // Satin dielectric: broad highlights must not exaggerate tessellation normals.
  surfaceRoughness: 0.5, surfaceMetalness: 0.04, surfaceEnvironmentIntensity: 0.36, datumOpacity: 0.09,
  lightSky: 0xf1f5fc, lightGround: 0x667381, lightKey: 0xffffff, lightFill: 0xdae5f2, lightRim: 0xe4edff,
  hemisphereIntensity: 0.5, keyIntensity: 1.0, fillIntensity: 0.25, rimIntensity: 0.26,
} as const;
