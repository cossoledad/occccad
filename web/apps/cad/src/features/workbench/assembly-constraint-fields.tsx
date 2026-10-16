import {Alert, Button, Form, Input, Select, Switch, Tag, Typography} from "antd";
import type {AssemblyConstraint, AssemblyGeometryRef, AssemblyGroupMember, CommandPreview, DocumentView} from "../../types";
import {CadNumberInput as InputNumber} from "../../cad/overlay/cad-number-input";
import {DiagnosticCopy, diagnosticReference} from "../../cad/command/diagnostic-copy";
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

export function AssemblyConstraintFields({ kind, references, exactTypes, sourceTypes, contactCapabilities, view, lengthUnit, constraint, previewEvaluation, dirty, replacing, onReplace, onDerive, onLocate, onRefresh, onValueCommit, angleRelation, replacingMember, onReplaceMember, onLocateMember }: {
  kind: keyof typeof assemblyConstraintUI; references: Array<AssemblyGeometryRef | undefined>;
  exactTypes?:string[];sourceTypes?:string[];contactCapabilities?:Array<{subtype:string}>;
  view?: DocumentView;
  lengthUnit: string;
  constraint?: AssemblyConstraint; dirty?: boolean; replacing?: 0 | 1 | 2;
  previewEvaluation?: CommandPreview["constraintEvaluation"];
  onReplace: (index: 0 | 1) => void; onLocate: (reference: AssemblyGeometryRef) => void;
  onDerive: (index:0|1,role:string)=>void;
  onRefresh?: () => void;
  onValueCommit: () => void; angleRelation?:AssemblyConstraint["angleRelation"];
  replacingMember?:string; onReplaceMember:(token:string)=>void; onLocateMember:(member:AssemblyGroupMember)=>void;
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
    {kind !== "FIX_TOGETHER" && <Typography.Text type="secondary">精确支持：{exactTypes?.join(" → ") || "待服务器解析；拾取面/边不代表平面/直线"}</Typography.Text>}
    {kind === "CONTACT" && <>
      <Form.Item name="contactKind" label="接触集合" rules={[{required:true}]}><Select onChange={onValueCommit} options={[...new Set(contactCapabilities?.map(c=>c.subtype.split("-").at(-1)?.toUpperCase()))].filter(Boolean).map(value=>({value,label:value}))}/></Form.Item>
      <Form.Item name="contactSide" label="材料侧" rules={[{required:true}]}><Select onChange={onValueCommit} options={[{value:"EXTERNAL",label:"External（外法向相反）"},{value:"INTERNAL",label:"Internal（外法向相同）"}]}/></Form.Item>
      <Form.Item name="contactBranch" label="定向分支" rules={[{required:true}]}><Select onChange={onValueCommit} options={[{value:1,label:"正分支"},{value:-1,label:"负分支"}]}/></Form.Item>
      {!contactCapabilities?.length && <Alert type="warning" message="此精确支持组合不在 Contact 合同内"/>}
    </>}
    {kind === "FIX_TOGETHER" && <>
      <Form.Item name="groupMembers" label="成员" rules={[{required:true,type:"array",min:1}]}><AssemblyGroupMemberList view={view} replacing={replacingMember} onReplace={onReplaceMember} onLocate={onLocateMember}/></Form.Item>
    </>}
    <div className={`assembly-constraint-status status-${evaluationStatus.toLowerCase()}`} role="status"
      aria-label={`Constraint status ${status.label}`}>
      <span className="assembly-status-light" style={{ background: status.color }} />
      <span><strong>{status.label}</strong><small>{previewEvaluation?.summary ?? (dirty ? "定义已修改，等待权威预览。" : constraint?.evaluationSummary ?? status.description)}</small></span>
      <div className="assembly-status-actions">{(previewEvaluation?.failure??(!dirty?constraint?.evaluationFailure:undefined))&&<DiagnosticCopy text={diagnosticReference(previewEvaluation?.failure??constraint?.evaluationFailure)}/>}
      {constraint && evaluationStatus !== "VERIFIED" && !dirty && <Button size="small" onClick={onRefresh}>重新计算</Button>}</div>
    </div>
    {definition.supports > 0 && <div className="assembly-support-list"><strong>支持元素</strong>{references.slice(0, definition.supports).map((reference,index)=><div className="assembly-support-row" key={index}>
      {(() => { const base = assemblySupportPresentation(reference); const preview = index === 0 ? previewEvaluation?.first : previewEvaluation?.second;
        const support = preview ? { ...base, status: preview.status, label: preview.status === "CONNECTED" ? "Connected" as const : "NotConnected" as const,
          diagnosticCode: preview.diagnosticCode, diagnostic: preview.diagnostic } : base; return <>
        <span className={`assembly-support-index status-${support.status.toLowerCase()}`}>{index+1}</span>
        {(() => { const presentation = describeAssemblyReference(reference, view); return <span className="assembly-support-detail">
          <strong>{presentation.primary}</strong><small>{presentation.secondary}</small>
          <small><Tag color={support.status === "CONNECTED" ? "success" : "error"}>{support.label}</Tag>
            {presentation.publication && <Tag color="blue">Publication</Tag>}
            {support.diagnosticCode && <span>{support.diagnosticCode}</span>}</small>
          {support.diagnostic && <small title={support.evidenceDigest}>{support.diagnostic}</small>}
        </span>; })()}
        <div className="assembly-support-actions">
          <Button size="small" disabled={!reference} onClick={()=>reference&&onLocate(reference)}>定位</Button>
          <Button size="small" type={replacing===index?"primary":"default"} onClick={()=>onReplace(index as 0|1)}>
            {support.status === "NOT_CONNECTED" ? "Reconnect" : "更换"}</Button>
        </div>
        {reference && derivedSupportOptions(sourceTypes?.[index]).length>0 && <div className="assembly-support-options">
          <span>精确子元素</span><Select aria-label={`支持元素 ${index+1} 的精确子元素`}
          value={reference.derivedRole??""} options={derivedSupportOptions(sourceTypes?.[index])} onChange={role=>onDerive(index as 0|1,role)}/>
        </div>}
      </>; })()}</div>)}</div>}
    {kind === "FIX" && <div className="assembly-fixed-parameters">
      <div className="assembly-fixed-space"><span>在空间中固定</span><Form.Item name="fixMode" noStyle getValueProps={value=>({checked:value!=="RELATIVE"})} getValueFromEvent={checked=>checked?"SPACE":"RELATIVE"}><Switch aria-label="在空间中固定" /></Form.Item></div>
      {([['fixedTranslation',`位置（${lengthUnit}）`],['fixedAngles','旋转（deg）']] as const).map(([field,label])=><fieldset key={field} className="assembly-pose-fields"><legend>{label}</legend>
        {["X","Y","Z"].map((axis,index)=><Form.Item key={axis} name={[field,index]} label={axis} rules={[{required:true,type:"number"}]}>
          <InputNumber aria-label={field==='fixedTranslation'?`${axis}（${lengthUnit}）`:`绕 ${axis} 旋转（deg）`} onPressEnter={event=>event.currentTarget.blur()} /></Form.Item>)}
      </fieldset>)}
    </div>}
    {directionApplicable && <Form.Item name="directionRelation" label="方向"><Select onChange={onValueCommit} options={relation === "PERPENDICULAR" ? [{value:"SAME",label:"正向（90°）"},{value:"OPPOSITE",label:"反向（270°）"}] : [
      {value:"UNORIENTED",label:"未定义"},{value:"SAME",label:"同向"},{value:"OPPOSITE",label:"反向"}]} /></Form.Item>}
    {distanceDirectionApplicable && <Form.Item name="distanceRelation" label="距离方向"><Select onChange={onValueCommit} options={[
      {value:"UNSIGNED",label:"无符号"},{value:"SELECTED_PLANE_NORMAL_V1",label:"第一元素法向（双平面）/所选平面法向"},
      ...(constraint?.distanceRelation === "ALONG_SECOND_NORMAL" || constraint?.distanceRelation === "OPPOSITE_SECOND_NORMAL" ?
        [{value:constraint.distanceRelation,label:"历史定义：第二法向约定（保留）"}] : [])]} /></Form.Item>}
    {kind === "DISTANCE" && <>
      <Typography.Text type="secondary">偏移 = 所选法向 ·（第一位置 − 第二位置）。双平面取第一法向；同向/反向不改变输入正负。无平面仅无符号无限支撑距离。</Typography.Text>
      <Form.Item name="quantityKey" label="偏移参数标识"><Input onBlur={onValueCommit} /></Form.Item>
      <Form.Item name="quantityExpression" label="长度表达式（空白使用数值）"><Input placeholder="例如 offset_1 + 5" onBlur={onValueCommit} onPressEnter={event=>event.currentTarget.blur()} /></Form.Item>
      <Typography.Text type="secondary">可引用：{view?.product?.constraints?.flatMap(c=>{const p=c.quantityParameter;return p?[p.key]:[];}).join(", ") || "暂无；无单位常数使用默认长度单位"}</Typography.Text>
      <Form.Item name="constraintMode" label="求值模式"><Select onChange={onValueCommit} options={[{value:"DRIVING",label:"驱动"},{value:"MEASURED",label:"只测量（保留驱动定义）"}]} /></Form.Item>
      {constraint?.mode === "MEASURED" && <Typography.Text>测量：{constraint.measuredValue === undefined ? "不可测" : `${constraint.measuredValue} mm`}</Typography.Text>}
    </>}
    {kind === "ANGLE" && !relationOnly && <>
      <Form.Item name="quantityKey" label="角度参数标识"><Input onBlur={onValueCommit}/></Form.Item>
      <Form.Item name="quantityExpression" label="角度表达式（空白使用数值）"><Input placeholder="例如 Tilt_base + 15 deg" onBlur={onValueCommit} onPressEnter={event=>event.currentTarget.blur()}/></Form.Item>
      <Form.Item name="constraintMode" label="求值模式"><Select onChange={onValueCommit} options={[{value:"DRIVING",label:"驱动"},{value:"MEASURED",label:"只测量（保留驱动定义）"}]}/></Form.Item>
      {constraint?.mode === "MEASURED" && <Typography.Text>测量：{constraint.measuredValue === undefined ? "不可测" : `${constraint.measuredValue*180/Math.PI} deg`}</Typography.Text>}
    </>}
    {definition.value && !relationOnly && <Form.Item name="value" label={definition.value === "angle" ? "角度（deg）" : `偏移（${lengthUnit}）`}
      rules={expressionDriven ? [] : [{required:true},{type:"number",min:minimumValue,max:definition.value === "angle"?360:undefined}]}>
      <InputNumber disabled={expressionDriven} min={minimumValue} max={definition.value === "angle"?360:undefined} precision={2} style={{width:"100%"}}
        onBlur={onValueCommit} onPressEnter={(event)=>event.currentTarget.blur()} /></Form.Item>}
  </>;
}


