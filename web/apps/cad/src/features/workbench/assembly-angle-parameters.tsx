import { Select, Checkbox } from "antd";
import {FeaturePickField} from "./feature-pick-field";
import {CommandField} from "../../cad/overlay/command-form";
import type { AssemblyGeometryRef, DocumentView } from "../../types";
import { describeAssemblyReference } from "../../cad/assembly/assembly-reference-presentation";
import { derivedSupportOptions } from "../../cad/assembly/assembly-capability";

import { changeAngleRelation, type AngleParameters as Parameters } from "../../cad/assembly/assembly-angle";
export function AssemblyAngleParameters({value,view,error,onChange,onPick,axisSourceType,axisExactType,onDerive,picking,onLocate}:{
  value:Parameters; view?:DocumentView;error?:string; onChange:(value:Parameters)=>void; onPick:()=>void;
  picking?:boolean;onLocate?:()=>void;axisSourceType?:string;axisExactType?:string;onDerive?:(role:string)=>void;
}) {
  const relation = value.angleRelation ?? "FREE";
  const reference = value.angleAxis && describeAssemblyReference(value.angleAxis,view);
  return <div style={{display:"grid",gap:8,marginBottom:12}}>
    <CommandField label="角度关系" width="enum"><Select aria-label="角度关系" value={relation}
      options={[{value:"FREE",label:"空间角"},{value:"DIRECTED",label:"平面角"},{value:"PARALLEL",label:"平行"},{value:"PERPENDICULAR",label:"垂直"}]}
      onChange={angleRelation=>onChange(changeAngleRelation(value,angleRelation))} /></CommandField>
    {relation === "DIRECTED" && <>
      <FeaturePickField compact error={error} label="参考轴" value={reference?.primary??"未选择"} detail={reference?.secondary} selected={!!value.angleAxis} active={!!picking} onActivate={onPick} onLocate={value.angleAxis?onLocate:undefined} onClear={value.angleAxis?()=>onChange({...value,angleAxis:undefined}):undefined}>
       {value.angleAxis&&onDerive&&derivedSupportOptions(axisSourceType).length>0&&<CommandField label="方向类型" width="enum"><Select aria-label="参考轴的工程方向" value={value.angleAxis.derivedRole??""} options={derivedSupportOptions(axisSourceType)} onChange={onDerive}/></CommandField>}
      </FeaturePickField>
      <Checkbox aria-label="反转参考轴" checked={Boolean(value.reverseAngleAxis)}
        onChange={event=>onChange({...value,reverseAngleAxis:event.target.checked})}>反转参考轴</Checkbox>
    </>}
  </div>;
}

export function angleAxisCandidateError(reference:AssemblyGeometryRef, _second?:AssemblyGeometryRef):string|undefined {
  if (!reference.instanceId) return "参考轴必须带有完整的组件身份。";
  if (!["AXIS","PLANE","FACE","EDGE","CYLINDER"].includes(reference.kind)) return "请选择轴、直线或平面；曲面/曲线的方向由精确几何验证。";
}
