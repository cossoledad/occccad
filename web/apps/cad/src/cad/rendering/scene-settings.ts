export type SceneMetrics={groundHeight:number;gridSpacing:number};
export type SceneSettings = {
  groundMode: "AUTO" | "WORLD" | "MANUAL" | "OFF";
  groundHeight: number; groundOffset: number; groundLocked: boolean;
  grid: boolean; worldGrid: boolean; gridSpacing: number; gridOpacity: number; gridColor: string;
  shadows: boolean; shadowOpacity: number;
  lighting: "ENGINEERING" | "SOFT" | "CONTOUR";
  brightness: number; ambient: number; fill: number; reflection: number;
  azimuth: number; elevation: number;
  background: "THEME" | "SOLID" | "GRADIENT"; backgroundTop: string; backgroundBottom: string;
};

export const DEFAULT_SCENE_SETTINGS: Readonly<SceneSettings> = {
  groundMode:"AUTO",groundHeight:0,groundOffset:0,groundLocked:false,
  grid:true,worldGrid:false,gridSpacing:0,gridOpacity:1,gridColor:"#617483",
  shadows:false,shadowOpacity:0.18,lighting:"ENGINEERING",brightness:1,ambient:1,fill:1,reflection:1,
  azimuth:-126.87,elevation:54.46,background:"THEME",backgroundTop:"#e7eef3",backgroundBottom:"#b9cbd8",
};

const number=(v:unknown,fallback:number,min:number,max:number)=>typeof v==="number"&&Number.isFinite(v)?Math.max(min,Math.min(max,v)):fallback;
const color=(v:unknown,fallback:string)=>typeof v==="string"&&/^#[0-9a-fA-F]{6}$/.test(v)?v.toLowerCase():fallback;
export function normalizeSceneSettings(value:unknown):SceneSettings {
  const v=value&&typeof value==="object"?value as Partial<SceneSettings>:{};const d=DEFAULT_SCENE_SETTINGS;
  return {
    groundMode:["AUTO","WORLD","MANUAL","OFF"].includes(v.groundMode??"")?v.groundMode!:d.groundMode,
    groundHeight:number(v.groundHeight,0,-1e12,1e12),groundOffset:number(v.groundOffset,0,-1e12,1e12),groundLocked:v.groundLocked===true,
    grid:v.grid!==false,worldGrid:v.worldGrid===true,gridSpacing:number(v.gridSpacing,0,0,1e12),gridOpacity:number(v.gridOpacity,1,0,2),gridColor:color(v.gridColor,d.gridColor),
    shadows:v.shadows===true,shadowOpacity:number(v.shadowOpacity,d.shadowOpacity,0,1),
    lighting:["ENGINEERING","SOFT","CONTOUR"].includes(v.lighting??"")?v.lighting!:d.lighting,
    brightness:number(v.brightness,1,0,3),ambient:number(v.ambient,1,0,3),fill:number(v.fill,1,0,3),reflection:number(v.reflection,1,0,3),
    azimuth:number(v.azimuth,d.azimuth,-180,180),elevation:number(v.elevation,d.elevation,5,90),
    background:["THEME","SOLID","GRADIENT"].includes(v.background??"")?v.background!:d.background,
    backgroundTop:color(v.backgroundTop,d.backgroundTop),backgroundBottom:color(v.backgroundBottom,d.backgroundBottom),
  };
}

export function groundLevel(settings:SceneSettings,minimumZ:number|undefined):number {
  if(settings.groundMode==="WORLD")return 0;
  if(settings.groundMode==="MANUAL"||settings.groundLocked)return settings.groundHeight;
  return (minimumZ!==undefined&&Number.isFinite(minimumZ)?minimumZ:0)+settings.groundOffset;
}

export function lightingPreset(preset:SceneSettings["lighting"]):Partial<SceneSettings>{
  return preset==="SOFT"?{lighting:preset,brightness:.85,ambient:1.35,fill:1.3,reflection:.65,azimuth:-126.87,elevation:65}:
    preset==="CONTOUR"?{lighting:preset,brightness:1,ambient:.65,fill:.55,reflection:.8,azimuth:-140,elevation:35}:
    {lighting:preset,brightness:1,ambient:1,fill:1,reflection:1,azimuth:-126.87,elevation:54.46};
}
