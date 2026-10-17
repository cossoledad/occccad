import { ColorPicker, Input, Space, Typography } from "antd";
import { useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";

export function BodyColorDialog({ initialColor, hasFaceColors, pending, onClose, onApply }: {
  initialColor?: string; hasFaceColors: boolean; pending: boolean;
  onClose: () => void; onApply: (color: string) => void;
}) {
  const [color, setColor] = useState(initialColor ?? "#98a8b8");
  const valid = /^#[0-9a-fA-F]{6}$/.test(color);
  return <CommandDialog id="body-color" open title="颜色材质" size="S" onClose={onClose}
    confirmLoading={pending} confirmDisabled={!valid} onConfirm={() => onApply(color.toLowerCase())}>
    <Space orientation="vertical">
      <ColorPicker value={valid?color:"#98a8b8"} onChange={value => setColor(value.toHexString())} showText disabledAlpha
        presets={[{label:"基础颜色",colors:["#98a8b8","#d64a3a","#e8b541","#3b9863","#3978bd","#8b62b2","#e6e6e6","#3b4048"]}]} />
      <Input aria-label="颜色 HEX" value={color} onChange={event=>setColor(event.target.value)} status={valid?undefined:"error"} maxLength={7} />
      <Typography.Text type="secondary">为整个 Body 设置不透明纯色。</Typography.Text>
      {hasFaceColors && <Typography.Text type="secondary">应用后将替换导入的分面颜色，可通过撤销恢复。</Typography.Text>}
      <Typography.Link disabled={pending} onClick={() => onApply("")}>使用默认颜色</Typography.Link>
    </Space>
  </CommandDialog>;
}
