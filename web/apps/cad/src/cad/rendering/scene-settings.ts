export type SceneMetrics={groundHeight:number;gridSpacing:number};
export type SceneSettings = {
  projection:"PERSPECTIVE"|"ORTHOGRAPHIC";
  groundMode: "AUTO" | "WORLD" | "MANUAL" | "OFF";
  groundHeight: number; groundOffset: number; groundLocked: boolean;
  grid: boolean; worldGrid: boolean; gridSpacing: number; gridOpacity: number; gridColor: string; gridAutoColor:boolean;
  shadows: boolean; shadowOpacity: number;
  lighting: "ENGINEERING" | "SOFT" | "CONTOUR";
  brightness: number; ambient: number; fill: number; reflection: number;
  azimuth: number; elevation: number;
  background: "THEME" | "SOLID" | "GRADIENT"; backgroundTop: string; backgroundBottom: string;
};

export const DEFAULT_SCENE_SETTINGS: Readonly<SceneSettings> = {
  projection:"PERSPECTIVE",groundMode:"AUTO",groundHeight:0,groundOffset:0,groundLocked:false,
  grid:true,worldGrid:false,gridSpacing:10,gridOpacity:1,gridColor:"#cbdced",gridAutoColor:true,
  shadows:false,shadowOpacity:0.18,lighting:"ENGINEERING",brightness:1,ambient:1,fill:1,reflection:1,
  azimuth:-126.87,elevation:54.46,background:"GRADIENT",backgroundTop:"#233346",backgroundBottom:"#70869b",
};

const number=(v:unknown,fallback:number,min:number,max:number)=>typeof v==="number"&&Number.isFinite(v)?Math.max(min,Math.min(max,v)):fallback;
const color=(v:unknown,fallback:string)=>typeof v==="string"&&/^#[0-9a-fA-F]{6}$/.test(v)?v.toLowerCase():fallback;
export function normalizeSceneSettings(value:unknown):SceneSettings {
  const v=value&&typeof value==="object"?value as Partial<SceneSettings>:{};const d=DEFAULT_SCENE_SETTINGS;
  return {
    projection:v.projection==="ORTHOGRAPHIC"?"ORTHOGRAPHIC":"PERSPECTIVE",
    groundMode:["AUTO","WORLD","MANUAL","OFF"].includes(v.groundMode??"")?v.groundMode!:d.groundMode,
    groundHeight:number(v.groundHeight,0,-1e12,1e12),groundOffset:number(v.groundOffset,0,-1e12,1e12),groundLocked:v.groundLocked===true,
    grid:v.grid!==false,worldGrid:v.worldGrid===true,gridSpacing:typeof v.gridSpacing==="number"&&v.gridSpacing>0?number(v.gridSpacing,d.gridSpacing,.001,1e12):d.gridSpacing,gridOpacity:number(v.gridOpacity,1,0,2),gridColor:color(v.gridColor,d.gridColor),gridAutoColor:v.gridAutoColor!==false,
    shadows:v.shadows===true,shadowOpacity:number(v.shadowOpacity,d.shadowOpacity,0,1),
    lighting:["ENGINEERING","SOFT","CONTOUR"].includes(v.lighting??"")?v.lighting!:d.lighting,
    brightness:number(v.brightness,1,0,3),ambient:number(v.ambient,1,0,3),fill:number(v.fill,1,0,3),reflection:number(v.reflection,1,0,3),
    azimuth:number(v.azimuth,d.azimuth,-180,180),elevation:number(v.elevation,d.elevation,5,90),
    background:["THEME","SOLID","GRADIENT"].includes(v.background??"")?v.background!:d.background,
    backgroundTop:color(v.backgroundTop,d.backgroundTop),backgroundBottom:color(v.backgroundBottom,d.backgroundBottom),
  };
}

export type ModelExtent={x:number;y:number;z:number};
/** Similar models get similar clearances; thin/long parts do not inherit the longest edge's gap. */
export function automaticGroundClearance(extent?:ModelExtent):number {
  if(!extent)return 0;
  const dimensions=[extent.x,extent.y,extent.z].map(v=>Number.isFinite(v)?Math.max(0,v):0).sort((a,b)=>a-b);
  const largest=dimensions[2],middle=dimensions[1]||largest;
  if(largest===0)return 0;
  // Normalize before multiplication to avoid overflow for distant/large scenes.
  const mean=largest*Math.cbrt((dimensions[0]/largest)*(dimensions[1]/largest));
  return Math.max(middle*.005,Math.min(middle*.08,mean*.08));
}
export function groundLevel(settings:SceneSettings,minimumZ:number|undefined,extent?:ModelExtent):number {
  if(settings.groundMode==="WORLD")return 0;
  if(settings.groundMode==="MANUAL"||settings.groundLocked)return settings.groundHeight;
  return (minimumZ!==undefined&&Number.isFinite(minimumZ)?minimumZ-automaticGroundClearance(extent):0)+settings.groundOffset;
}

export function lightingPreset(preset:SceneSettings["lighting"]):Partial<SceneSettings>{
  return preset==="SOFT"?{lighting:preset,brightness:.85,ambient:1.35,fill:1.3,reflection:.65,azimuth:-126.87,elevation:65}:
    preset==="CONTOUR"?{lighting:preset,brightness:1,ambient:.65,fill:.55,reflection:.8,azimuth:-140,elevation:35}:
    {lighting:preset,brightness:1,ambient:1,fill:1,reflection:1,azimuth:-126.87,elevation:54.46};
}

export function colorLuminance(hex:string):number {
  const c=[1,3,5].map(i=>parseInt(hex.slice(i,i+2),16)/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4);
  return c[0]*.2126+c[1]*.7152+c[2]*.0722;
}
/** One background contract for grids, sketch feedback and the transparent tree. */
export function sceneBackgroundColors(settings:SceneSettings):[string,string] {
  const top=settings.background==="THEME"?DEFAULT_SCENE_SETTINGS.backgroundTop:settings.backgroundTop;
  const bottom=settings.background==="SOLID"?top:settings.background==="THEME"?DEFAULT_SCENE_SETTINGS.backgroundBottom:settings.backgroundBottom;
  return [top,bottom];
}
export function sceneBackgroundIsLight(settings:SceneSettings):boolean {
  const backgrounds=sceneBackgroundColors(settings).map(colorLuminance);
  const contrast=(foreground:number)=>Math.min(...backgrounds.map(background=>(Math.max(foreground,background)+.05)/(Math.min(foreground,background)+.05)));
  return contrast(colorLuminance("#20384f"))>contrast(colorLuminance("#f0f5fc"));
}
export function sceneGridColor(settings:SceneSettings):string {
  return settings.gridAutoColor?(sceneBackgroundIsLight(settings)?"#354d65":"#cbdced"):settings.gridColor;
}
