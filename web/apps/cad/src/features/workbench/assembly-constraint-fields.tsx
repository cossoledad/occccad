import {formatDisplayNumber} from "../../utils/display-number";
import {assemblyQuantityInputFields} from "../../cad/assembly/assembly-quantity-input";
import {FeaturePickField} from "./feature-pick-field";
import {CommandGroup,CommandField,CommandFormItem,CommandVector} from "../../cad/overlay/command-form";
import {engineeringLabel} from "../../cad/presentation/engineering-text";
import {engineeringProblem} from "../../cad/command/operation-feedback";
import {Form, Input, Select, Switch, Typography} from "antd";
import type {AssemblyConstraint, AssemblyGeometryRef, AssemblyGroupMember, CommandPreview, DocumentView} from "../../types";
import {CadNumberInput as InputNumber} from "../../cad/overlay/cad-number-input";
import {AssemblyGroupMemberList} from "./assembly-group-member-list";
import {angleRelationSupportsMeasured} from "../../cad/assembly/assembly-angle";
import {derivedSupportOptions} from "../../cad/assembly/assembly-capability";
import {assemblySupportPresentation} from "../../cad/assembly/assembly-constraint-ux";
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

export function AssemblyConstraintFields({ kind, references, supportIssues,exactTypes, sourceTypes, contactCapabilities, view, lengthUnit, constraint, previewEvaluation, replacing, onReplace, onRemove, onDerive, onLocate, onSelectSupport, onHoverSupport, onValueCommit, angleRelation,groupPicking,onPickGroup, replacingMember, onReplaceMember, onLocateMember }: {
  kind: keyof typeof assemblyConstraintUI; references: Array<AssemblyGeometryRef | undefined>;
  supportIssues?:Array<string|undefined>;exactTypes?:string[];sourceTypes?:string[];contactCapabilities?:Array<{subtype:string}>;
  view?: DocumentView;
  lengthUnit: string;
  constraint?: AssemblyConstraint; replacing?: 0 | 1 | 2;
  previewEvaluation?: CommandPreview["constraintEvaluation"];
  onSelectSupport?:(reference:AssemblyGeometryRef)=>void;onHoverSupport?:(reference:AssemblyGeometryRef|undefined)=>void;
  onReplace: (index: 0 | 1) => void; onRemove?:(index:0|1)=>void; onLocate: (reference: AssemblyGeometryRef) => void;
  onDerive: (index:0|1,role:string)=>void;
  onValueCommit: () => void; angleRelation?:AssemblyConstraint["angleRelation"];
  groupPicking?:boolean;onPickGroup?:()=>void;replacingMember?:string; onReplaceMember:(token:string)=>void; onLocateMember:(member:AssemblyGroupMember)=>void;
}) {
  const definition = assemblyConstraintUI[kind];
  const planePair = exactTypes?.length===2 && exactTypes.every(kind=>kind==="PLANE");
  const distanceRelation = Form.useWatch("distanceRelation");
  const form=Form.useFormInstance();
  const sourceExpression=Form.useWatch("quantityExpression",{form,preserve:true});
  const sourceValue=Form.useWatch("value",{form,preserve:true});
  const relation = angleRelation ?? constraint?.angleRelation;
  const relationOnly = kind === "ANGLE" && !angleRelationSupportsMeasured(relation);
  const directionApplicable = relation === "PARALLEL" || relation === "PERPENDICULAR" || (kind !== "ANGLE" && definition.direction && planePair);
  const distanceDirectionApplicable = definition.distanceDirection && !!exactTypes?.includes("PLANE");
  return <>
    {kind === "CONTACT" && <>
      <CommandFormItem name="contactKind" label="接触集合" width="enum" rules={[{required:true}]}><Select onChange={onValueCommit} options={[...new Set(contactCapabilities?.map(c=>c.subtype.split("-").at(-1)?.toUpperCase()))].filter(Boolean).map(value=>({value,label:engineeringLabel(value,"接触关系")}))}/></CommandFormItem>
      <CommandFormItem name="contactSide" label="材料侧" width="enum" rules={[{required:true}]}><Select onChange={onValueCommit} options={[{value:"EXTERNAL",label:"外接触"},{value:"INTERNAL",label:"内接触"}]}/></CommandFormItem>
      <CommandFormItem name="contactBranch" label="定向分支" width="enum" rules={[{required:true}]}><Select onChange={onValueCommit} options={[{value:1,label:"正分支"},{value:-1,label:"负分支"}]}/></CommandFormItem>
    </>}
    {kind === "FIX_TOGETHER" && <>
      <CommandFormItem name="groupMembers" noStyle rules={[{required:true,type:"array",min:1}]}><AssemblyGroupMemberList picking={groupPicking} onActivate={onPickGroup} view={view} replacing={replacingMember} onReplace={onReplaceMember} onLocate={onLocateMember}/></CommandFormItem>
    </>}
    {definition.supports>0&&<CommandGroup label="支持元素">{Array.from({length:definition.supports},(_,index)=>{
      const reference=references[index],base=assemblySupportPresentation(reference),preview=index===0?previewEvaluation?.first:previewEvaluation?.second;
      const support=preview?{...base,status:preview.status,diagnosticCode:preview.diagnosticCode,diagnostic:preview.diagnostic}:base;
      const presentation=describeAssemblyReference(reference,view);
      return <FeaturePickField compact onSelect={reference&&onSelectSupport?()=>onSelectSupport(reference):undefined} onHover={onHoverSupport?active=>onHoverSupport(active?reference:undefined):undefined} key={index} label={`支撑 ${index+1}`} value={presentation.publication?<span className="assembly-publication-label" title={presentation.primary}><span className="assembly-publication-owner">{presentation.owner}</span><span className="assembly-publication-separator">{' : '}</span><strong>{presentation.publication}</strong></span>:presentation.primary} detail={reference?(reference.derivedRole?presentation.secondary:exactTypes?.[index]?engineeringLabel(exactTypes[index],presentation.secondary):presentation.secondary):undefined} selected={!!reference} active={replacing===index} onActivate={()=>onReplace(index as 0|1)} onClear={reference&&onRemove?()=>onRemove(index as 0|1):undefined} onLocate={reference?()=>onLocate(reference):undefined} error={reference?(supportIssues?.[index]??(support.status!=="CONNECTED"&&support.diagnostic?engineeringProblem({code:support.diagnosticCode,message:support.diagnostic}):undefined)):undefined}>
       {reference&&derivedSupportOptions(sourceTypes?.[index]).length>0&&<CommandField label="支撑类型" width="enum"><Select aria-label={`支持元素 ${index+1} 的支撑类型`} value={reference.derivedRole??''} options={derivedSupportOptions(sourceTypes?.[index])} onChange={role=>onDerive(index as 0|1,role)}/></CommandField>}
      </FeaturePickField>;
    })}</CommandGroup>}
    {kind === "FIX" && <div className="assembly-fixed-parameters">
      <div className="assembly-fixed-space"><span>在空间中固定</span><CommandFormItem name="fixMode" noStyle getValueProps={value=>({checked:value!=="RELATIVE"})} getValueFromEvent={checked=>checked?"SPACE":"RELATIVE"}><Switch aria-label="在空间中固定" /></CommandFormItem></div>
      {([['fixedTranslation',`位置（${lengthUnit}）`],['fixedAngles','旋转（deg）']] as const).map(([field,label])=><CommandVector key={field} label={label}>
        {["X","Y","Z"].map((axis,index)=><CommandFormItem key={axis} name={[field,index]} width="coordinate" label={axis} rules={[{required:true,type:"number"}]}>
          <InputNumber aria-label={field==='fixedTranslation'?`${axis}（${lengthUnit}）`:`绕 ${axis} 旋转（deg）`} onBlur={onValueCommit}/></CommandFormItem>)}
      </CommandVector>)}
    </div>}
    {directionApplicable && <CommandFormItem name="directionRelation" label="方向" width="enum"><Select onChange={onValueCommit} options={relation === "PERPENDICULAR" ? [{value:"SAME",label:"正向（90°）"},{value:"OPPOSITE",label:"反向（270°）"}] : [
      {value:"UNORIENTED",label:"未定义"},{value:"SAME",label:"同向"},{value:"OPPOSITE",label:"反向"}]} /></CommandFormItem>}
    {distanceDirectionApplicable && <CommandFormItem name="distanceRelation" label="距离方向"><Select onChange={onValueCommit} options={[
      {value:"UNSIGNED",label:"无符号"},{value:"SELECTED_PLANE_NORMAL_V1",label:"第一元素法向 / 平面法向"},
      ...(constraint?.distanceRelation === "ALONG_SECOND_NORMAL" || constraint?.distanceRelation === "OPPOSITE_SECOND_NORMAL" ?
        [{value:constraint.distanceRelation,label:"沿第二法向"}] : [])]} /></CommandFormItem>}
    {definition.value && !relationOnly && <>
      <CommandFormItem name="quantityInput" label={kind==='ANGLE'?"角度":"偏移"} width="expression" rules={[{validator:async(_,source)=>{
        if(source===undefined)return;
        assemblyQuantityInputFields(kind,{quantityInput:source,distanceRelation},lengthUnit);
      }}]}>
        <QuantityInput fallback={sourceExpression?.trim()||formatDisplayNumber(sourceValue??0)} unit={kind==='ANGLE'?'deg':lengthUnit} onBlur={onValueCommit}/>
      </CommandFormItem>
      {constraint?.mode==='MEASURED'&&<Typography.Text type="secondary">当前测量：{constraint.measuredValue===undefined?'不可测':kind==='ANGLE'?`${constraint.measuredValue*180/Math.PI}°`:`${constraint.measuredValue} mm`}</Typography.Text>}
      <CommandFormItem name="constraintMode" label="模式" width="enum"><Select options={[{value:'DRIVING',label:'驱动'},{value:'MEASURED',label:'参考测量'}]}/></CommandFormItem>
    </>}
  </>;
}

function QuantityInput({value,onChange,fallback,unit,onBlur,...props}:{value?:string;onChange?:(value:string)=>void;fallback:string;unit:string;onBlur:()=>void}){
 return <Input {...props} aria-label="数值或公式" value={value??fallback} onChange={event=>onChange?.(event.target.value)} onBlur={onBlur} onPressEnter={event=>{if(!event.nativeEvent.isComposing){event.preventDefault();event.stopPropagation();event.currentTarget.blur();}}} placeholder="数值或公式" suffix={unit}/>;
}
