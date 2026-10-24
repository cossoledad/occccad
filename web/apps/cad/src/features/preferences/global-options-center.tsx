import {CommandField} from "../../cad/overlay/command-form";
import {CommandDialog} from "../../cad/overlay/floating-panel";
import {APP_OPTIONS_COMMAND,globalOptionsCommand} from "./global-options-command";
import {SettingOutlined} from "@ant-design/icons";
import {Button,Checkbox,ColorPicker,Input,InputNumber,Menu,Radio,Select,Space,Typography} from "antd";
import {createContext,useCallback,useContext,useMemo,useRef,useState,type PropsWithChildren} from "react";
import {CommandRegistry} from "../../cad/command/command-registry";
import {CaptureSettingsPanel} from "../../cad/overlay/capture-settings-panel";
import {DEFAULT_CAPTURE_SETTINGS} from "../../cad/interaction/capture-settings";
import {DEFAULT_SCENE_SETTINGS,type SceneMetrics} from "../../cad/rendering/scene-settings";
import {DEFAULT_REFERENCE_VISIBILITY,DEFAULT_SOLID_DISPLAY,DEFAULT_GRID_VISIBILITY} from "../../cad/rendering/display-settings";
import {useUIPreferences,type DisplayLengthUnit} from "../../state/ui-preferences";
import {DEFAULT_CREATION_DEFAULTS,globalOptionsSnapshot,type GlobalOptionsSnapshot} from "./global-options-model";
import {SceneOptionsFields} from "./scene-options-fields";


type OptionsContext={draft?:GlobalOptionsSnapshot;open:()=>void;registry:CommandRegistry;setMetrics:(metrics:SceneMetrics|undefined)=>void};
const Context=createContext<OptionsContext|undefined>(undefined);
export const useGlobalOptions=()=>useContext(Context);
const units=[{value:"mm",label:"毫米 (mm)"},{value:"cm",label:"厘米 (cm)"},{value:"m",label:"米 (m)"},{value:"in",label:"英寸 (in)"}];
type Page="scene"|"display"|"creation"|"navigation"|"units";

export function GlobalOptionsProvider({children,activeDocumentID}:PropsWithChildren<{activeDocumentID?:string}>) {
 const [draft,setDraft]=useState<GlobalOptionsSnapshot>();const [page,setPage]=useState<Page>("scene");
 const [metrics,setMetrics]=useState<SceneMetrics>();
 const draftRef=useRef<GlobalOptionsSnapshot|undefined>(undefined);
 const open=useCallback(()=>{const value=globalOptionsSnapshot(useUIPreferences.getState());draftRef.current=value;setDraft(value)},[]);
 const close=()=>{draftRef.current=undefined;setDraft(undefined)};
 const registry=useMemo(()=>{const r=new CommandRegistry();r.register(globalOptionsCommand(open));return r},[open]);
 const patch=(change:Partial<GlobalOptionsSnapshot>)=>{
  const current=draftRef.current;if(!current)return;const next={...current,...change};draftRef.current=next;setDraft(next);
  const preferences=useUIPreferences.getState();
  // An incomplete HEX draft remains local; every other valid setting applies now.
  preferences.applyGlobalOptions(/^#[0-9a-fA-F]{6}$/.test(next.creationDefaults.solid.color)?next:{...next,creationDefaults:{solid:{...next.creationDefaults.solid,color:preferences.creationDefaults.solid.color}}});
 };
 const reset=()=>{if(!draft)return;switch(page){
  case "scene":patch({sceneSettings:{...DEFAULT_SCENE_SETTINGS}});break;
  case "display":patch({solidDisplay:{...DEFAULT_SOLID_DISPLAY},referenceVisibility:{...DEFAULT_REFERENCE_VISIBILITY},gridVisibility:{...DEFAULT_GRID_VISIBILITY}});break;
  case "creation":patch({creationDefaults:structuredClone(DEFAULT_CREATION_DEFAULTS)});break;
  case "navigation":patch({navigationProfile:"default",catiaRotationSphereVisible:false,captureSettings:structuredClone(DEFAULT_CAPTURE_SETTINGS)});break;
  case "units":{const documentLengthUnits={...draft.documentLengthUnits};if(activeDocumentID)delete documentLengthUnits[activeDocumentID];patch({displayLengthUnit:"mm",documentLengthUnits});break}
 }};
 const contents=draft&&<>
  {page==="scene"&&<SceneOptionsFields value={draft.sceneSettings} onChange={sceneSettings=>patch({sceneSettings})} metrics={metrics}/>}
  {page==="display"&&<Space orientation="vertical" className="global-options-fields">
   <Typography.Title level={5}>实体显示</Typography.Title><Radio.Group aria-label="实体显示" value={draft.solidDisplay.mode} onChange={e=>patch({solidDisplay:{...draft.solidDisplay,mode:e.target.value}})} options={[{value:"shaded",label:"正常显示"},{value:"wireframe",label:"线框显示"}]}/>
   <Checkbox disabled={draft.solidDisplay.mode==="wireframe"} checked={draft.solidDisplay.mode==="wireframe"||draft.solidDisplay.edges} onChange={e=>patch({solidDisplay:{...draft.solidDisplay,edges:e.target.checked}})}>显示边线</Checkbox>
   <Checkbox checked={draft.solidDisplay.vertices} onChange={e=>patch({solidDisplay:{...draft.solidDisplay,vertices:e.target.checked}})}>显示交点</Checkbox>
   <Typography.Title level={5}>基准与草图</Typography.Title>
   {([["planes","基准面"],["axes","基准轴"],["coordinateSystems","坐标系"]] as const).map(([key,label])=><Checkbox key={key} checked={draft.referenceVisibility[key]} onChange={e=>patch({referenceVisibility:{...draft.referenceVisibility,[key]:e.target.checked}})}>{label}</Checkbox>)}
   <Checkbox checked={draft.gridVisibility.sketch} onChange={e=>patch({gridVisibility:{...draft.gridVisibility,sketch:e.target.checked}})}>显示草图网格</Checkbox>
  </Space>}
  {page==="creation"&&<Space orientation="vertical" className="global-options-fields">
   <Typography.Title level={5}>新建零件默认外观</Typography.Title>
   <CommandField label="实体表面颜色" width="content"><Space><ColorPicker aria-label="新建实体表面颜色" disabledAlpha showText value={draft.creationDefaults.solid.color} onChange={c=>patch({creationDefaults:{solid:{...draft.creationDefaults.solid,color:c.toHexString()}}})}/><Input aria-label="默认实体颜色 HEX" style={{width:105}} maxLength={7} value={draft.creationDefaults.solid.color} status={/^#[0-9a-fA-F]{6}$/.test(draft.creationDefaults.solid.color)?undefined:"error"} onChange={e=>patch({creationDefaults:{solid:{...draft.creationDefaults.solid,color:e.target.value}}})}/></Space></CommandField>
   {([["roughness","粗糙度"],["metalness","金属度"]] as const).map(([key,label])=><CommandField key={key} label={label} width="number"><InputNumber aria-label={`默认${label}`} min={0} max={1} step={.05} value={draft.creationDefaults.solid[key]} onChange={v=>{if(v!==null)patch({creationDefaults:{solid:{...draft.creationDefaults.solid,[key]:v}}})}}/></CommandField>)}

  </Space>}
  {page==="navigation"&&<Space orientation="vertical" className="global-options-fields">
   <Typography.Title level={5}>鼠标操作</Typography.Title>
   <Select aria-label="鼠标操作模式" value={draft.navigationProfile} onChange={navigationProfile=>patch({navigationProfile})} options={[{value:"default",label:"occccad"},{value:"catia",label:"3DEXPERIENCE CATIA"},{value:"solidworks",label:"SOLIDWORKS"}]}/>
   <Typography.Paragraph type="secondary">{draft.navigationProfile==="catia"?"中键拖动平移；按住中键再按左键或右键旋转；释放侧键并保持中键缩放。":draft.navigationProfile==="solidworks"?"中键旋转；Ctrl + 中键平移；Shift + 中键缩放。":"右键拖动旋转；中键拖动平移；滚轮缩放。"}</Typography.Paragraph>
   {draft.navigationProfile==="catia"&&<Checkbox checked={draft.catiaRotationSphereVisible} onChange={e=>patch({catiaRotationSphereVisible:e.target.checked})}>显示旋转球</Checkbox>}
   <CaptureSettingsPanel settings={draft.captureSettings} onEnabledChange={enabled=>patch({captureSettings:{...draft.captureSettings,enabled}})} onSelectionToggle={kind=>patch({captureSettings:{...draft.captureSettings,selection:draft.captureSettings.selection.includes(kind)?draft.captureSettings.selection.filter(k=>k!==kind):[...draft.captureSettings.selection,kind]}})} onSketchToggle={kind=>patch({captureSettings:{...draft.captureSettings,sketch:draft.captureSettings.sketch.includes(kind)?draft.captureSettings.sketch.filter(k=>k!==kind):[...draft.captureSettings.sketch,kind]}})} onAll={()=>patch({captureSettings:structuredClone(DEFAULT_CAPTURE_SETTINGS)})} onPointsOnly={()=>patch({captureSettings:{enabled:true,selection:["POINT"],sketch:["GRID","ORIGIN","POINT","ENDPOINT","CENTER","MIDPOINT"]}})}/>
  </Space>}
  {page==="units"&&<Space orientation="vertical" className="global-options-fields">
   <CommandField label="默认长度单位" width="enum"><Select aria-label="默认长度单位" value={draft.displayLengthUnit} options={units} onChange={displayLengthUnit=>patch({displayLengthUnit:displayLengthUnit as DisplayLengthUnit})}/></CommandField>
   {activeDocumentID&&<CommandField label="当前文档显示单位" width="enum"><Select aria-label="当前文档显示单位" value={draft.documentLengthUnits[activeDocumentID]??"INHERIT"} options={[{value:"INHERIT",label:"继承全局设置"},...units]} onChange={unit=>{const documentLengthUnits={...draft.documentLengthUnits};if(String(unit)==="INHERIT")delete documentLengthUnits[activeDocumentID];else documentLengthUnits[activeDocumentID]=unit as DisplayLengthUnit;patch({documentLengthUnits})}}/></CommandField>}
  </Space>}
 </>;
 const context=useMemo(()=>({draft,open,registry,setMetrics}),[draft,open,registry]);
 return <Context.Provider value={context}>{children}<CommandDialog id="global-options" open={Boolean(draft)} title="选项" size="L" onClose={close} onConfirm={close} footer={false}>
  <div className="global-options-layout"><Menu selectedKeys={[page]} onClick={({key})=>setPage(key as Page)} items={[{key:"scene",label:"场景"},{key:"display",label:"显示"},{key:"creation",label:"新建对象"},{key:"navigation",label:"导航与捕获"},{key:"units",label:"单位"}]}/><div className="global-options-content">{contents}<Button type="link" onClick={reset}>恢复当前页默认设置</Button></div></div>
 </CommandDialog></Context.Provider>;
}
export function GlobalOptionsButton(){const options=useGlobalOptions();return <Button ghost aria-label="选项" title="全局选项" icon={<SettingOutlined/>} onClick={()=>{void options?.registry.execute(APP_OPTIONS_COMMAND)}}/>}
