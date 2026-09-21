import { Alert, Button, Select, Switch, Typography } from "antd";
import type { AssemblyConstraint, AssemblyGeometryRef, DocumentView } from "../../types";
import { describeAssemblyReference } from "../../cad/assembly/assembly-reference-presentation";

type Parameters = Pick<AssemblyConstraint,"angleRelation"|"angleAxis"|"reverseAngleAxis">;
export function AssemblyAngleParameters({value,view,onChange,onPick}:{
  value:Parameters; view?:DocumentView; onChange:(value:Parameters)=>void; onPick:()=>void;
}) {
  const relation = value.angleRelation ?? "DIRECTED";
  const reference = value.angleAxis && describeAssemblyReference(value.angleAxis,view);
  return <div style={{display:"grid",gap:8,marginBottom:12}}>
    <Select aria-label="角度关系" value={relation} style={{width:"100%"}}
      options={[{value:"DIRECTED",label:"绕所选轴的角"},{value:"PARALLEL",label:"平行"},{value:"PERPENDICULAR",label:"垂直"}]}
      onChange={angleRelation=>onChange({...value,angleRelation})} />
    {relation === "DIRECTED" && <>
      <Typography.Paragraph>参考轴随第二支持元素的组件运动；平面使用法向，直线使用其方向。</Typography.Paragraph>
      <Typography.Text>{reference ? `${reference.primary} · ${reference.secondary}` : "尚未选择参考轴"}</Typography.Text>
      <Button onClick={onPick}>选择参考轴</Button>
      <Switch checkedChildren="参考轴反向" unCheckedChildren="参考轴正向" aria-label="反转参考轴" checked={Boolean(value.reverseAngleAxis)}
        onChange={reverseAngleAxis=>onChange({...value,reverseAngleAxis})} />
      {!value.angleAxis && <Alert type="info" message="在第二组件上选择稳定的轴、直线或平面作为参考轴。" />}
    </>}
  </div>;
}

export function angleAxisCandidateError(reference:AssemblyGeometryRef, second?:AssemblyGeometryRef):string|undefined {
  if (reference.instanceId !== second?.instanceId) return "参考轴必须属于第二支持元素的组件。";
  if (!["AXIS","PLANE","FACE","EDGE"].includes(reference.kind)) return "请选择轴、直线或平面；曲面/曲线的方向由精确几何验证。";
}
