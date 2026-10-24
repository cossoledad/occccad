import {FeaturePickField} from "./feature-pick-field";
import {CommandGroup,CommandField,CommandFormItem,CommandVector} from "../../cad/overlay/command-form";
import {engineeringLabel} from "../../cad/presentation/engineering-text";
import {engineeringProblem} from "../../cad/command/operation-feedback";
import {Alert, Button, Form, Input, Select, Switch, Tag, Typography} from "antd";
import type {AssemblyConstraint, AssemblyGeometryRef, AssemblyGroupMember, CommandPreview, DocumentView} from "../../types";
import {CadNumberInput as InputNumber} from "../../cad/overlay/cad-number-input";
import {AssemblyGroupMemberList} from "./assembly-group-member-list";
import {angleRelationSupportsMeasured} from "../../cad/assembly/assembly-angle";
import {derivedSupportOptions} from "../../cad/assembly/assembly-capability";
import {ASSEMBLY_CONSTRAINT_STATUS, assemblySupportPresentation} from "../../cad/assembly/assembly-constraint-ux";
import {describeAssemblyReference} from "../../cad/assembly/assembly-reference-presentation";

type AssemblyConstraintUIDefinition = { supports: 0 | 1 | 2; direction: boolean; distanceDirection: boolean; value?: "angle" | "distance" };
const assemblyConstraintUI: Record<AssemblyConstraint["kind"], AssemblyConstraintUIDefinition> = {
  FIX: { supports: 1, direction: false, distanceDirection: false },
  RIGID: { supports: 2, direction: false, distanceDirection: false },
  CONTACT: { supports: 2, direction: false, distanceDirection: false },
  FIX_TOGETHER: { supports: 0, direction: false, distanceDirection: false },
  COINCIDENT: { supports: 2, direction: true, distanceDirection: false },
  CONCENTRIC: { supports: 2, direction: false, distanceDirection: false },
  ANGLE: { supports: 2, direction: false, distanceDirection: false, value: "angle" },
  DISTANCE: { supports: 2, direction: true, distanceDirection: true, value: "distance" },
};

export function AssemblyConstraintFields({ kind, references, exactTypes, sourceTypes, contactCapabilities, view, lengthUnit, constraint, previewEvaluation, dirty, replacing, onReplace, onRemove, onDerive, onLocate, onRefresh, onValueCommit, angleRelation,groupPicking,onPickGroup, replacingMember, onReplaceMember, onLocateMember }: {
  kind: keyof typeof assemblyConstraintUI; references: Array<AssemblyGeometryRef | undefined>;
  exactTypes?:string[];sourceTypes?:string[];contactCapabilities?:Array<{subtype:string}>;
  view?: DocumentView;
  lengthUnit: string;
  constraint?: AssemblyConstraint; dirty?: boolean; replacing?: 0 | 1 | 2;
  previewEvaluation?: CommandPreview["constraintEvaluation"];
  onReplace: (index: 0 | 1) => void; onRemove?:(index:0|1)=>void; onLocate: (reference: AssemblyGeometryRef) => void;
  onDerive: (index:0|1,role:string)=>void;
  onRefresh?: () => void;
  onValueCommit: () => void; angleRelation?:AssemblyConstraint["angleRelation"];
  groupPicking?:boolean;onPickGroup?:()=>void;replacingMember?:string; onReplaceMember:(token:string)=>void; onLocateMember:(member:AssemblyGroupMember)=>void;
}) {
  const definition = assemblyConstraintUI[kind];
  const evaluationStatus = previewEvaluation?.status ?? (dirty ? "NOT_UPDATED" : constraint?.evaluationStatus ?? "NOT_UPDATED");
  const status = ASSEMBLY_CONSTRAINT_STATUS[evaluationStatus];
  const planePair = exactTypes?.length===2 && exactTypes.every(kind=>kind==="PLANE");
  const distanceRelation = Form.useWatch("distanceRelation");
  const quantityExpression = Form.useWatch("quantityExpression");
  const expressionDriven = (kind === "DISTANCE" && !!quantityExpression?.trim()) || (kind === "ANGLE" && !!quantityExpression?.trim());
  const minimumValue = kind === "DISTANCE" && distanceRelation && distanceRelation !== "UNSIGNED" ? undefined : 0;
  const relation = angleRelation ?? constraint?.angleRelation;
  const relationOnly = kind === "ANGLE" && !angleRelationSupportsMeasured(relation);
  const directionApplicable = relation === "PARALLEL" || relation === "PERPENDICULAR" || (kind !== "ANGLE" && definition.direction && planePair);
  const distanceDirectionApplicable = definition.distanceDirection && !!exactTypes?.includes("PLANE");
  return <>
    {kind !== "FIX_TOGETHER" && <Typography.Text type="secondary">几何类型：{exactTypes?.map(type=>engineeringLabel(type,"几何元素")).join(" / ") || "请选择几何元素"}</Typography.Text>}
    {kind === "CONTACT" && <>
      <CommandFormItem name="contactKind" label="接触集合" width="enum" rules={[{required:true}]}><Select onChange={onValueCommit} options={[...new Set(contactCapabilities?.map(c=>c.subtype.split("-").at(-1)?.toUpperCase()))].filter(Boolean).map(value=>({value,label:engineeringLabel(value,"接触关系")}))}/></CommandFormItem>
      <CommandFormItem name="contactSide" label="材料侧" width="enum" rules={[{required:true}]}><Select onChange={onValueCommit} options={[{value:"EXTERNAL",label:"外接触"},{value:"INTERNAL",label:"内接触"}]}/></CommandFormItem>
      <CommandFormItem name="contactBranch" label="定向分支" width="enum" rules={[{required:true}]}><Select onChange={onValueCommit} options={[{value:1,label:"正分支"},{value:-1,label:"负分支"}]}/></CommandFormItem>
      {!contactCapabilities?.length && <Alert type="warning" message="所选几何不支持此接触关系"/>}
    </>}
    {kind === "FIX_TOGETHER" && <>
      <CommandFormItem name="groupMembers" noStyle rules={[{required:true,type:"array",min:1}]}><AssemblyGroupMemberList picking={groupPicking} onActivate={onPickGroup} view={view} replacing={replacingMember} onReplace={onReplaceMember} onLocate={onLocateMember}/></CommandFormItem>
    </>}
    <div className={`assembly-constraint-status status-${evaluationStatus.toLowerCase()}`} role="status"
      aria-label={`Constraint status ${status.label}`}>
      <span className="assembly-status-light" style={{ background: status.color }} />
      <span><strong>{status.label}</strong><small>{dirty?"定义已修改":status.description}</small></span>
      <div className="assembly-status-actions">
      {constraint && evaluationStatus !== "VERIFIED" && !dirty && <Button size="small" onClick={onRefresh}>重新计算</Button>}</div>
    </div>
    {definition.supports>0&&<CommandGroup label="支持元素">{Array.from({length:definition.supports},(_,index)=>{
      const reference=references[index],base=assemblySupportPresentation(reference),preview=index===0?previewEvaluation?.first:previewEvaluation?.second;
      const support=preview?{...base,status:preview.status,diagnosticCode:preview.diagnosticCode,diagnostic:preview.diagnostic}:base;
      const presentation=describeAssemblyReference(reference,view);
      return <FeaturePickField key={index} label={`支撑 ${index+1}`} value={presentation.primary} detail={<>{presentation.secondary} · <Tag color={support.status==='CONNECTED'?'success':'error'}>{reference?support.status==='CONNECTED'?'有效':'需要重新选择':'未选择'}</Tag>{presentation.publication&&<Tag>发布引用</Tag>}</>} selected={!!reference} active={replacing===index} onActivate={()=>onReplace(index as 0|1)} onClear={reference&&onRemove?()=>onRemove(index as 0|1):undefined} onLocate={reference?()=>onLocate(reference):undefined} error={support.status!=="CONNECTED"&&support.diagnostic?engineeringProblem({code:support.diagnosticCode,message:support.diagnostic}):undefined}>
       {reference&&derivedSupportOptions(sourceTypes?.[index]).length>0&&<CommandField label="支撑类型" width="enum"><Select aria-label={`支持元素 ${index+1} 的支撑类型`} value={reference.derivedRole??''} options={derivedSupportOptions(sourceTypes?.[index])} onChange={role=>onDerive(index as 0|1,role)}/></CommandField>}
      </FeaturePickField>;
    })}</CommandGroup>}
    {kind === "FIX" && <div className="assembly-fixed-parameters">
      <div className="assembly-fixed-space"><span>在空间中固定</span><CommandFormItem name="fixMode" noStyle getValueProps={value=>({checked:value!=="RELATIVE"})} getValueFromEvent={checked=>checked?"SPACE":"RELATIVE"}><Switch aria-label="在空间中固定" /></CommandFormItem></div>
      {([['fixedTranslation',`位置（${lengthUnit}）`],['fixedAngles','旋转（deg）']] as const).map(([field,label])=><CommandVector key={field} label={label}>
        {["X","Y","Z"].map((axis,index)=><CommandFormItem key={axis} name={[field,index]} width="coordinate" label={axis} rules={[{required:true,type:"number"}]}>
          <InputNumber aria-label={field==='fixedTranslation'?`${axis}（${lengthUnit}）`:`绕 ${axis} 旋转（deg）`}  /></CommandFormItem>)}
      </CommandVector>)}
    </div>}
    {directionApplicable && <CommandFormItem name="directionRelation" label="方向" width="enum"><Select onChange={onValueCommit} options={relation === "PERPENDICULAR" ? [{value:"SAME",label:"正向（90°）"},{value:"OPPOSITE",label:"反向（270°）"}] : [
      {value:"UNORIENTED",label:"未定义"},{value:"SAME",label:"同向"},{value:"OPPOSITE",label:"反向"}]} /></CommandFormItem>}
    {distanceDirectionApplicable && <CommandFormItem name="distanceRelation" label="距离方向"><Select onChange={onValueCommit} options={[
      {value:"UNSIGNED",label:"无符号"},{value:"SELECTED_PLANE_NORMAL_V1",label:"第一元素法向（双平面）/所选平面法向"},
      ...(constraint?.distanceRelation === "ALONG_SECOND_NORMAL" || constraint?.distanceRelation === "OPPOSITE_SECOND_NORMAL" ?
        [{value:constraint.distanceRelation,label:"历史定义：第二法向约定（保留）"}] : [])]} /></CommandFormItem>}
    {kind === "DISTANCE" && <>
      <Typography.Text type="secondary">偏移 = 所选法向 ·（第一位置 − 第二位置）。双平面取第一法向；同向/反向不改变输入正负。无平面仅无符号无限支撑距离。</Typography.Text>
      <CommandFormItem name="quantityKey" label="偏移参数标识"><Input onBlur={onValueCommit} /></CommandFormItem>
      <CommandFormItem name="quantityExpression" label="长度表达式（空白使用数值）"><Input placeholder="例如 offset_1 + 5" onBlur={onValueCommit}  /></CommandFormItem>
      <Typography.Text type="secondary">可引用：{view?.product?.constraints?.flatMap(c=>{const p=c.quantityParameter;return p?[p.key]:[];}).join(", ") || "暂无；无单位常数使用默认长度单位"}</Typography.Text>
      <CommandFormItem name="constraintMode" label="求值模式" width="enum"><Select onChange={onValueCommit} options={[{value:"DRIVING",label:"驱动"},{value:"MEASURED",label:"只测量（保留驱动定义）"}]} /></CommandFormItem>
      {constraint?.mode === "MEASURED" && <Typography.Text>测量：{constraint.measuredValue === undefined ? "不可测" : `${constraint.measuredValue} mm`}</Typography.Text>}
    </>}
    {kind === "ANGLE" && !relationOnly && <>
      <CommandFormItem name="quantityKey" label="角度参数标识"><Input onBlur={onValueCommit}/></CommandFormItem>
      <CommandFormItem name="quantityExpression" label="角度表达式（空白使用数值）"><Input placeholder="例如 Tilt_base + 15 deg" onBlur={onValueCommit} /></CommandFormItem>
      <CommandFormItem name="constraintMode" label="求值模式" width="enum"><Select onChange={onValueCommit} options={[{value:"DRIVING",label:"驱动"},{value:"MEASURED",label:"只测量（保留驱动定义）"}]}/></CommandFormItem>
      {constraint?.mode === "MEASURED" && <Typography.Text>测量：{constraint.measuredValue === undefined ? "不可测" : `${constraint.measuredValue*180/Math.PI} deg`}</Typography.Text>}
    </>}
    {definition.value && !relationOnly && <CommandFormItem name="value" width="number" label={definition.value === "angle" ? "角度（deg）" : `偏移（${lengthUnit}）`}
      rules={expressionDriven ? [] : [{required:true},{type:"number",min:minimumValue,max:definition.value === "angle"?360:undefined}]}>
      <InputNumber disabled={expressionDriven} min={minimumValue} max={definition.value === "angle"?360:undefined} precision={2}
        onBlur={onValueCommit}  /></CommandFormItem>}
  </>;
}
