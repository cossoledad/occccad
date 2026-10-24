import { Select, Switch, Typography } from "antd";
import {FeaturePickField} from "./feature-pick-field";
import {CommandField} from "../../cad/overlay/command-form";
import type { AssemblyGeometryRef, DocumentView } from "../../types";
import { describeAssemblyReference } from "../../cad/assembly/assembly-reference-presentation";
import { derivedSupportOptions } from "../../cad/assembly/assembly-capability";

import { changeAngleRelation, type AngleParameters as Parameters } from "../../cad/assembly/assembly-angle";
export function AssemblyAngleParameters({value,view,onChange,onPick,axisSourceType,axisExactType,onDerive,picking,onLocate}:{
  value:Parameters; view?:DocumentView; onChange:(value:Parameters)=>void; onPick:()=>void;
  picking?:boolean;onLocate?:()=>void;axisSourceType?:string;axisExactType?:string;onDerive?:(role:string)=>void;
}) {
  const relation = value.angleRelation ?? "FREE";
  const reference = value.angleAxis && describeAssemblyReference(value.angleAxis,view);
  return <div style={{display:"grid",gap:8,marginBottom:12}}>
    <CommandField label="角度关系" width="expression"><Select aria-label="角度关系" value={relation}
      options={[{value:"FREE",label:"空间夹角（无指定轴）"},{value:"DIRECTED",label:"绕所选轴的角"},{value:"PARALLEL",label:"平行"},{value:"PERPENDICULAR",label:"垂直"}]}
      onChange={angleRelation=>onChange(changeAngleRelation(value,angleRelation))} /></CommandField>
    {relation === "FREE" && <Typography.Text type="secondary">只限制两支持方向的夹角，保留旋转轴自由度。大于 180° 选择反向解；方向分支随已接受姿态更新，不固定旋转轴。</Typography.Text>}
    {relation === "PERPENDICULAR" && <Typography.Text type="secondary">正向90°与反向270°选择相反的垂直姿态，均不指定旋转轴。</Typography.Text>}
    {relation === "DIRECTED" && <>
      <Typography.Paragraph>参考轴随所属组件运动；平面使用法向，直线使用其方向。</Typography.Paragraph>
      <FeaturePickField label="参考轴" value={reference?.primary??"未选择"} detail={reference?.secondary} selected={!!value.angleAxis} active={!!picking} onActivate={onPick} onLocate={value.angleAxis?onLocate:undefined} onClear={value.angleAxis?()=>onChange({...value,angleAxis:undefined}):undefined}>
       {value.angleAxis&&onDerive&&derivedSupportOptions(axisSourceType).length>0&&<CommandField label="方向类型" width="enum"><Select aria-label="参考轴的工程方向" value={value.angleAxis.derivedRole??""} options={derivedSupportOptions(axisSourceType)} onChange={onDerive}/></CommandField>}
      </FeaturePickField>
      <Switch checkedChildren="参考轴反向" unCheckedChildren="参考轴正向" aria-label="反转参考轴" checked={Boolean(value.reverseAngleAxis)}
        onChange={reverseAngleAxis=>onChange({...value,reverseAngleAxis})} />
    </>}
  </div>;
}

export function angleAxisCandidateError(reference:AssemblyGeometryRef, _second?:AssemblyGeometryRef):string|undefined {
  if (!reference.instanceId) return "参考轴必须带有完整的组件身份。";
  if (!["AXIS","PLANE","FACE","EDGE","CYLINDER"].includes(reference.kind)) return "请选择轴、直线或平面；曲面/曲线的方向由精确几何验证。";
}
