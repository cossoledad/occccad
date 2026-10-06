import { useEffect, useState } from "react";
import { ViewportSettingsButton } from "../../cad/overlay/viewport-settings-button";
import { Button, Popover, Select, Space, Tooltip, Typography } from "antd";
import type { SketchCommandState, SketchCommandAction, SketchCommitReceipt } from "../../cad/tool/sketch-command-session";
import { LoadingOutlined } from "@ant-design/icons";
import { useUIPreferences } from "../../state/ui-preferences";

export function WorkbenchStatus({ busy, canEdit, selectionCount, toolName, lengthUnit, continuous, sketchCommand, onSketchAction, sketchReceipt, onSketchReceiptCheck }: {
  busy: boolean; canEdit: boolean; selectionCount: number; toolName: string; lengthUnit: string; continuous: boolean;sketchCommand?:SketchCommandState;onSketchAction?:(action:SketchCommandAction)=>void;
  sketchReceipt?:SketchCommitReceipt;onSketchReceiptCheck?:()=>void;
}) {
  const preferences = useUIPreferences();
  const [optionsOpen,setOptionsOpen]=useState(false);
  useEffect(()=>setOptionsOpen(false),[sketchCommand?.toolId]);
 const pending=sketchCommand?.phase==="committing"||sketchCommand?.phase==="unknown";
  return <footer className="workbench-status" aria-label="工作台状态">
    <span className="workbench-operation" role="status">{busy ? <><LoadingOutlined /> 正在提交…</> : <><i />{canEdit ? "可编辑" : "只读"}</>}</span>
    <span className="workbench-current-tool">{toolName}{continuous ? " · 连续执行" : ""}</span>
    {sketchCommand&&<div className="workbench-sketch-action" role="status" aria-label="当前草图动作">
      <Tooltip title={sketchCommand.role} mouseEnterDelay={.45}><span className="workbench-sketch-role" tabIndex={0}>{sketchCommand.role}</span></Tooltip>
      {sketchCommand.count&&<span>{sketchCommand.count.accepted}{sketchCommand.count.required!==undefined?` / ${sketchCommand.count.required}`:""}</span>}
      {sketchCommand.presentation==="inline"&&sketchCommand.fields.length>1&&sketchCommand.fields.map((field,index)=><Button size="small" type="text" key={index} disabled={pending} title={field.value} onClick={()=>onSketchAction?.({type:"input",index})}>{field.label}</Button>)}
      {sketchCommand.presentation==="inline"&&sketchCommand.options.length>0&&<Popover trigger="click" placement="top" title="工具选项" open={optionsOpen} onOpenChange={setOptionsOpen} content={
        <Space orientation="vertical" role="dialog" aria-label="工具选项" style={{minWidth:200}} onPointerDown={event=>event.stopPropagation()} onKeyDown={event=>{if(event.key==="Escape"){event.stopPropagation();if(!event.nativeEvent.isComposing&&event.nativeEvent.keyCode!==229)setOptionsOpen(false);}}}>
          {sketchCommand.options.map(option=><label key={option.name} style={{display:"block",width:"100%"}}>
            <span>{option.label}</span>
            <Select aria-label={option.label} style={{width:"100%"}} value={option.value} options={option.choices} disabled={pending}
              onChange={value=>onSketchAction?.({type:"option",name:option.name,value})}/>
          </label>)}
        </Space>
      }><Button size="small" disabled={pending}>选项</Button></Popover>}
      {sketchCommand.completion&&<Button size="small" type="default" className="cad-status-complete" disabled={pending||!sketchCommand.canConfirm} onClick={()=>onSketchAction?.({type:"confirm"})}>{sketchCommand.completion.label}</Button>}
      {sketchCommand.error&&!sketchCommand.input&&<Typography.Text role="alert" type="danger" ellipsis={{tooltip:sketchCommand.error}} style={{maxWidth:280}}>{sketchCommand.error}</Typography.Text>}
      {sketchCommand.recovery&&<Button size="small" disabled={pending} onClick={()=>onSketchAction?.({type:sketchCommand.recovery!.action})}>{sketchCommand.recovery.label}</Button>}
      {sketchCommand.phase==="unknown"&&!sketchReceipt&&<Button size="small" onClick={()=>onSketchAction?.({type:"retry"})}>确认原请求结果</Button>}
    </div>}
    {sketchReceipt?.status==="unknown"&&<Button size="small" onClick={onSketchReceiptCheck}>确认草图结果</Button>}
    {!sketchCommand&&<span className="workbench-selection-count">{selectionCount ? `已选择 ${selectionCount} 项` : "未选择对象"}</span>}
    <span className="workbench-status-spacer" />
    <ViewportSettingsButton grids={preferences.gridVisibility} onGrids={preferences.setGridVisibility} references={preferences.referenceVisibility} display={preferences.solidDisplay}
      onReferences={preferences.setReferenceVisibility} onDisplay={preferences.setSolidDisplay}
      capture={{ settings: preferences.captureSettings, onEnabledChange: preferences.setCaptureEnabled,
        onSelectionToggle: preferences.toggleSelectionCapture, onSketchToggle: preferences.toggleSketchSnap,
        onAll: preferences.captureAll, onPointsOnly: preferences.capturePointsOnly }} />
    <span>长度 · {lengthUnit}</span>
    <span className="workbench-navigation-label">{preferences.navigationProfile === "catia" ? "3DEXPERIENCE CATIA" : preferences.navigationProfile === "solidworks" ? "SOLIDWORKS" : "默认"} 导航</span>
  </footer>;
}
