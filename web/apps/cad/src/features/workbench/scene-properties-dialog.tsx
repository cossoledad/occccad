import {formatDisplayNumber} from "../../utils/display-number";
import { Button, Checkbox, ColorPicker, InputNumber, Select, Slider, Space, Tabs, Typography } from "antd";
import { useEffect, useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import { DEFAULT_SCENE_SETTINGS, lightingPreset, normalizeSceneSettings, type SceneSettings } from "../../cad/rendering/scene-settings";

export function ScenePropertiesDialog({initialSettings,documentName,groundHeight,metrics,onPreview,onApply,onClose}: {
  initialSettings:SceneSettings;documentName:string;groundHeight:()=>number;metrics?:import("../../cad/rendering/scene-settings").SceneMetrics;
  onPreview:(settings:SceneSettings|undefined)=>void;onApply:(settings:SceneSettings)=>void;onClose:()=>void;
}) {
  const [draft,setDraft]=useState(()=>normalizeSceneSettings(initialSettings));
  useEffect(()=>{onPreview(draft)},[draft,onPreview]);
  useEffect(()=>()=>onPreview(undefined),[onPreview]);
  const patch=(v:Partial<SceneSettings>)=>setDraft(s=>normalizeSceneSettings({...s,...v}));
  const number=(key:keyof SceneSettings,label:string,min:number,max:number,step=.1)=>
    <label className="cad-property-row"><span>{label}</span><InputNumber aria-label={label} value={draft[key] as number} min={min} max={max} step={step} onChange={v=>{if(v!==null)patch({[key]:v})}}/></label>;
  const slider=(key:keyof SceneSettings,label:string,min=0,max=3,step=.05)=>
    <div className="cad-property-row"><span>{label}</span><Slider aria-label={label} min={min} max={max} step={step} value={draft[key] as number} onChange={v=>patch({[key]:v})}/></div>;
  const color=(key:"gridColor"|"backgroundTop"|"backgroundBottom",label:string)=>
    <label className="cad-property-row"><span>{label}</span><ColorPicker aria-label={label} value={draft[key]} disabledAlpha showText onChange={v=>patch({[key]:v.toHexString()})}/></label>;
  const ground=<Space orientation="vertical" style={{width:"100%"}}>
    <label className="cad-property-row"><span>地面位置</span><Select aria-label="地面位置" value={draft.groundMode} onChange={groundMode=>patch({groundMode})} options={[{value:"AUTO",label:"自动贴合模型底部"},{value:"WORLD",label:"世界 XY 平面（Z=0）"},{value:"MANUAL",label:"指定高度"},{value:"OFF",label:"关闭地面"}]}/></label>
    {draft.groundMode==="AUTO"&&<>
      <Checkbox checked={draft.groundLocked} onChange={e=>patch({groundLocked:e.target.checked,groundHeight:groundHeight()})}>锁定当前地面高度</Checkbox>
      {!draft.groundLocked&&number("groundOffset","地面偏移（mm）",-1e9,1e9,1)}
      <Button size="small" onClick={()=>patch({groundLocked:false,groundOffset:0})}>重新贴合模型</Button>
    </>}
    {(draft.groundMode==="MANUAL"||draft.groundLocked)&&number("groundHeight","地面高度（mm）",-1e9,1e9,1)}
    {metrics&&<Typography.Text type="secondary" aria-label="当前场景地面">当前高度 {formatDisplayNumber(metrics.groundHeight)} mm · 网格间距 {metrics.gridSpacing.toPrecision(3)} mm</Typography.Text>}
    <Typography.Text type="secondary">地面仅用于观看。拖拽和运动预览期间保持稳定。</Typography.Text>
    <Checkbox checked={draft.grid} onChange={e=>patch({grid:e.target.checked})}>显示地面网格</Checkbox>
    <Checkbox checked={draft.worldGrid} onChange={e=>patch({worldGrid:e.target.checked})}>同时显示世界基准网格（Z=0）</Checkbox>
    <Checkbox checked={draft.gridSpacing===0} onChange={e=>patch({gridSpacing:e.target.checked?0:10})}>随缩放自动调整网格间距</Checkbox>
    {draft.gridSpacing>0&&number("gridSpacing","网格间距（mm）",.001,1e9,1)}
    {color("gridColor","网格颜色")}{slider("gridOpacity","网格强度",0,2)}
    <Checkbox checked={draft.shadows} onChange={e=>patch({shadows:e.target.checked})}>地面柔和阴影</Checkbox>
    {draft.shadows&&slider("shadowOpacity","阴影强度",0,1)}
  </Space>;
  const lighting=<Space orientation="vertical" style={{width:"100%"}}>
    <label className="cad-property-row"><span>光照预设</span><Select aria-label="光照预设" value={draft.lighting} onChange={v=>patch(lightingPreset(v))} options={[{value:"ENGINEERING",label:"工程"},{value:"SOFT",label:"柔和"},{value:"CONTOUR",label:"轮廓强调"}]}/></label>
    {slider("brightness","整体亮度")}{slider("ambient","环境补光")}{slider("fill","侧向补光")}{slider("reflection","环境反射强度")}
    {slider("azimuth","主光方位角",-180,180,1)}{slider("elevation","主光仰角",5,90,1)}
    <Typography.Text type="secondary">角度相对于世界坐标。对象的粗糙度和金属度在对象属性中设置。</Typography.Text>
  </Space>;
  const background=<Space orientation="vertical" style={{width:"100%"}}>
    <label className="cad-property-row"><span>背景</span><Select aria-label="背景" value={draft.background} onChange={background=>patch({background})} options={[{value:"THEME",label:"跟随主题"},{value:"SOLID",label:"纯色"},{value:"GRADIENT",label:"渐变"}]}/></label>
    {draft.background!=="THEME"&&color("backgroundTop",draft.background==="SOLID"?"背景颜色":"顶部颜色")}
    {draft.background==="GRADIENT"&&color("backgroundBottom","底部颜色")}
  </Space>;
  return <CommandDialog id="scene-properties" open title={`场景属性 · ${documentName}`} size="M" onClose={onClose} onConfirm={()=>onApply(draft)}>
    <Typography.Paragraph type="secondary">仅影响我的视图 · 设置按文档保存</Typography.Paragraph>
    <Tabs items={[{key:"ground",label:"地面与网格",children:ground},{key:"lighting",label:"光照",children:lighting},{key:"background",label:"背景",children:background}]}/>
    <Button type="link" onClick={()=>setDraft({...DEFAULT_SCENE_SETTINGS})}>恢复场景默认设置</Button>
  </CommandDialog>;
}
