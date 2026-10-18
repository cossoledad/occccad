import {Button,Checkbox,Collapse,ColorPicker,InputNumber,Select,Slider,Space,Tabs,Typography} from "antd";
import {formatDisplayNumber} from "../../utils/display-number";
import {DEFAULT_SCENE_SETTINGS,lightingPreset,sceneGridColor,type SceneSettings,type SceneMetrics} from "../../cad/rendering/scene-settings";

export function SceneOptionsFields({value,onChange,metrics}:{value:SceneSettings;onChange:(value:SceneSettings)=>void;metrics?:SceneMetrics}) {
 const patch=(change:Partial<SceneSettings>)=>onChange({...value,...change});
 const number=(key:keyof SceneSettings,label:string,min:number,max:number,step=1)=><label className="cad-property-row"><span>{label}</span><InputNumber aria-label={label} value={value[key] as number} min={min} max={max} step={step} onChange={v=>{if(v!==null)patch({[key]:v})}}/></label>;
 const slider=(key:keyof SceneSettings,label:string,min=0,max=3,step=.05)=><div className="cad-property-row"><span>{label}</span><Slider aria-label={label} min={min} max={max} step={step} value={value[key] as number} onChange={v=>patch({[key]:v})}/></div>;
 const color=(key:"gridColor"|"backgroundTop"|"backgroundBottom",label:string)=><label className="cad-property-row"><span>{label}</span><ColorPicker aria-label={label} value={value[key]} disabledAlpha showText onChange={v=>patch({[key]:v.toHexString()})}/></label>;
 const background=<Space orientation="vertical" className="global-options-fields">
  <label className="cad-property-row"><span>投影方式</span><Select aria-label="投影方式" value={value.projection} onChange={projection=>patch({projection})} options={[{value:"PERSPECTIVE",label:"透视（近大远小）"},{value:"ORTHOGRAPHIC",label:"平行（无透视）"}]}/></label>
  <div className="scene-color-preview" aria-label="背景预览" style={{color:sceneGridColor(value),background:`linear-gradient(${value.background==="SOLID"?value.backgroundTop:value.background==="THEME"?DEFAULT_SCENE_SETTINGS.backgroundTop:value.backgroundTop},${value.background==="SOLID"?value.backgroundTop:value.background==="THEME"?DEFAULT_SCENE_SETTINGS.backgroundBottom:value.backgroundBottom})`}}><span style={{background:sceneGridColor(value)}}/>工作区</div>
  <Space><Button onClick={()=>patch({background:"GRADIENT",backgroundTop:"#233346",backgroundBottom:"#70869b",gridAutoColor:true})}>深蓝</Button><Button onClick={()=>patch({background:"GRADIENT",backgroundTop:"#b7cbdc",backgroundBottom:"#eef3f8",gridAutoColor:true})}>明亮</Button><Button onClick={()=>patch({background:"GRADIENT",backgroundTop:"#202833",backgroundBottom:"#56616e",gridAutoColor:true})}>石墨</Button></Space>
  <label className="cad-property-row"><span>背景方式</span><Select aria-label="背景方式" value={value.background} onChange={background=>patch({background})} options={[{value:"THEME",label:"默认工作室"},{value:"GRADIENT",label:"上下渐变"},{value:"SOLID",label:"纯色"}]}/></label>
  {value.background!=="THEME"&&color("backgroundTop",value.background==="SOLID"?"背景颜色":"顶部颜色")}
  {value.background==="GRADIENT"&&color("backgroundBottom","底部颜色")}
  <Typography.Text type="secondary">渐变固定在屏幕上下方向，旋转模型时保持稳定。</Typography.Text>
 </Space>;
 const ground=<Space orientation="vertical" className="global-options-fields">
  <label className="cad-property-row"><span>地面位置</span><Select aria-label="地面位置" value={value.groundMode} onChange={groundMode=>patch({groundMode})} options={[{value:"AUTO",label:"自动贴合模型底部"},{value:"WORLD",label:"世界 XY 平面（Z=0）"},{value:"MANUAL",label:"指定高度"},{value:"OFF",label:"关闭地面"}]}/></label>
  {value.groundMode==="AUTO"&&<><Checkbox checked={value.groundLocked} disabled={!metrics&&!value.groundLocked} onChange={e=>patch({groundLocked:e.target.checked,groundHeight:metrics?.groundHeight??value.groundHeight})}>锁定当前地面高度</Checkbox>{!value.groundLocked&&number("groundOffset","地面偏移（mm）",-1e9,1e9)}<Button size="small" onClick={()=>patch({groundLocked:false,groundOffset:0})}>重新贴合模型</Button></>}
  {(value.groundMode==="MANUAL"||value.groundLocked)&&number("groundHeight","地面高度（mm）",-1e9,1e9)}
  {metrics&&<Typography.Text type="secondary" aria-label="当前场景地面">当前高度 {formatDisplayNumber(metrics.groundHeight)} mm · 网格间距 {formatDisplayNumber(metrics.gridSpacing)} mm</Typography.Text>}
  <Checkbox checked={value.grid} onChange={e=>patch({grid:e.target.checked})}>显示地面网格</Checkbox>
  {number("gridSpacing","网格间距（mm）",.001,1e9)}
  <Typography.Text type="secondary">地面为随可见实体和草图范围确定的正方形，大小与网格间距不随相机缩放改变。默认位于模型下方，间隙随模型尺寸调整；编辑草图时自动隐藏地面。</Typography.Text>
  <Checkbox checked={value.gridAutoColor} onChange={e=>patch({gridAutoColor:e.target.checked})}>网格颜色自动适应背景</Checkbox>
  {!value.gridAutoColor&&color("gridColor","网格颜色")}{slider("gridOpacity","网格强度",0,2)}
  <Collapse size="small" items={[{key:"advanced",label:"更多地面设置",children:<Space orientation="vertical"><Checkbox checked={value.worldGrid} onChange={e=>patch({worldGrid:e.target.checked})}>同时显示世界基准网格（Z=0）</Checkbox><Checkbox checked={value.shadows} onChange={e=>patch({shadows:e.target.checked})}>地面柔和阴影</Checkbox>{value.shadows&&slider("shadowOpacity","阴影强度",0,1)}</Space>}]}/>
  <Typography.Text type="secondary">所有文档共用此观看规则；自动模式按各自模型计算高度。</Typography.Text>
 </Space>;
 const lighting=<Space orientation="vertical" className="global-options-fields">
  <label className="cad-property-row"><span>光照预设</span><Select aria-label="光照预设" value={value.lighting} onChange={v=>patch(lightingPreset(v))} options={[{value:"ENGINEERING",label:"工程"},{value:"SOFT",label:"柔和"},{value:"CONTOUR",label:"轮廓强调"}]}/></label>
  {slider("brightness","整体亮度")}{slider("ambient","环境补光")}{slider("fill","侧向补光")}{slider("reflection","环境反射强度")}
  <Collapse size="small" items={[{key:"direction",label:"主光方向",children:<>{slider("azimuth","主光方位角",-180,180,1)}{slider("elevation","主光仰角",5,90,1)}</>}]}/>
 </Space>;
 return <Tabs items={[{key:"background",label:"背景",children:background},{key:"ground",label:"地面与网格",children:ground},{key:"lighting",label:"光照",children:lighting}]}/>;
}
