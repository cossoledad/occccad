import { Button, Checkbox, Popover, Radio } from "antd";
import { EyeOutlined } from "@ant-design/icons";
import type { ReferenceVisibility, RenderMode } from "../rendering/display-settings";

export function DisplaySettingsButton({ references, mode, onReferences, onMode }: {
  references: ReferenceVisibility; mode: RenderMode;
  onReferences: (value: ReferenceVisibility) => void; onMode: (value: RenderMode) => void;
}) {
  return <Popover trigger="click" placement="topRight" content={<div className="cad-capture-panel">
    <strong>显示设置</strong>
    <section><strong>基准元素</strong><div className="cad-capture-presets">
      <Button size="small" onClick={() => onReferences({ planes: true, axes: true, coordinateSystems: true })}>全部显示</Button>
      <Button size="small" onClick={() => onReferences({ planes: false, axes: false, coordinateSystems: false })}>全部隐藏</Button>
    </div><div className="cad-capture-grid">
      {([['planes','基准面'],['axes','基准轴'],['coordinateSystems','坐标系']] as const).map(([key,label]) =>
        <Checkbox key={key} checked={references[key]} onChange={event => onReferences({ ...references, [key]: event.target.checked })}>{label}</Checkbox>)}
    </div></section>
    <section><strong>实体渲染</strong><Radio.Group value={mode} onChange={event => onMode(event.target.value)}
      options={[{value:'default',label:'默认'},{value:'smooth',label:'平滑'},{value:'wireframe',label:'线框'}]} /></section>
    <small>统一控制视区显示；结构树仍可访问隐藏元素。</small>
  </div>}><Button className="cad-tool-button" aria-label="显示设置" title="显示设置" icon={<EyeOutlined />} /></Popover>;
}
