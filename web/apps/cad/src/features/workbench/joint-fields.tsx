import {FeaturePickField} from "./feature-pick-field";
import {owningInstanceSelection} from "../../cad/assembly/instance-selection";
import {useCommandForm} from "../../cad/overlay/command-form";
import {CommandField} from "../../cad/overlay/command-form";
import {Button,Checkbox,Collapse,Input,Select,Typography} from 'antd';
import {CadNumberInput as InputNumber} from '../../cad/overlay/cad-number-input';
import type {DocumentView} from '../../types';
import type {MechanismJoint} from '../../cad/assembly/motion-study';
import {jointDefinitions} from '../../cad/assembly/joint-definitions';
import {jointSupportLabel,type JointSupportRole} from '../../cad/assembly/mechanism-selection';
import {jointLimitReading,jointLimitValue} from '../../cad/assembly/motion-coordinates';

type Props={view:DocumentView;joint:MechanismJoint;pickRole?:JointSupportRole;onRole:(role:JointSupportRole)=>void;onClear:(role:JointSupportRole)=>void;onChange:(patch:Partial<MechanismJoint>)=>void;driving:Record<string,boolean>;referenced:ReadonlySet<string>;references:Record<string,number>;limitsIssue?:string;onDrive:(id:string,value:boolean)=>void;pending:boolean;disabled:boolean};
export function JointFields({view,joint,pickRole,onRole,onClear,onChange,driving,referenced,references,limitsIssue,onDrive,pending,disabled}:Props){
 const form=useCommandForm();
 const support=(role:JointSupportRole)=>{
  const [side,field]=role.split('.') as ['first'|'second','axis'|'plane'|'point'|'instance'];
  const endpoint=joint[side],reference=field==='instance'?endpoint?.instanceId?{instanceId:endpoint.instanceId,kind:'BODY' as const}:undefined:endpoint?.[field];
  const noun=field==='axis'?'轴线':field==='plane'?(joint.kind==='PRISMATIC'?'定向平面':joint.kind==='REVOLUTE'?'定位平面':'平面'):field==='point'?'点':'零件';
  const label=`${side==='first'?'第一':'第二'}${noun}`;
  const unavailable=disabled||field==='plane'&&['REVOLUTE','PRISMATIC'].includes(joint.kind)&&!endpoint?.axis;
  return <FeaturePickField key={role} label={label} value={reference?jointSupportLabel(view,reference):unavailable?'先选轴线':'未选择'} selected={!!reference} active={pickRole===role} disabled={unavailable} onActivate={()=>onRole(role)} onClear={reference?()=>onClear(role):undefined} onLocate={reference&&form.locate?()=>{form.cancelPick?.();const item=owningInstanceSelection(view,{kind:'instance',id:reference.instanceId,instanceId:reference.instanceId});if(item)form.locate?.([item])}:undefined}/>;
 };
 const coordinatePatch=(id:string,patch:Partial<NonNullable<MechanismJoint['coordinates']>[number]>)=>onChange({coordinates:joint.coordinates?.map(c=>c.id===id?{...c,...patch}:c)});
 return <div className="motion-joint-editor">
  <CommandField  label={<><span>接合名称</span></>} width="expression"><Input aria-label="接合名称" value={joint.name} disabled={disabled} onChange={e=>onChange({name:e.target.value})}/></CommandField>
  {jointDefinitions[joint.kind].roles.map(support)}
  {['PRISMATIC','CYLINDRICAL'].includes(joint.kind)&&<CommandField  label={<><span>轴线方向</span></>} width="enum"><Select aria-label="轴线方向" value={joint.axisDirection??'SAME'} disabled={disabled} options={[{value:'SAME',label:'同向'},{value:'OPPOSITE',label:'相对'}]} onChange={axisDirection=>onChange({axisDirection})}/></CommandField>}
  {joint.kind==='PRISMATIC'&&<CommandField  label={<><span>定向角度 °</span></>} width="number"><InputNumber aria-label="定向角度 °" value={joint.orientationAngle?.value??0} disabled={disabled} onChange={value=>onChange({orientationAngle:{value:value??0,unit:'deg'}})}/></CommandField>}
  {joint.kind==='PRISMATIC'&&<CommandField  label={<><span>平面方向</span></>} width="enum"><Select aria-label="平面方向" value={joint.axialDirection??'SAME'} disabled={disabled} options={[{value:'SAME',label:'同向'},{value:'OPPOSITE',label:'相对'}]} onChange={axialDirection=>onChange({axialDirection})}/></CommandField>}
  {['REVOLUTE','PLANAR'].includes(joint.kind)&&<>
   <CommandField  label={<><span>{joint.kind==='REVOLUTE'?'轴向偏移':'平面偏移'} mm</span></>} width="number"><InputNumber aria-label={joint.kind==='REVOLUTE'?'轴向偏移 mm':'平面偏移 mm'} value={joint.axialOffset?.value??0} disabled={disabled} onChange={value=>onChange({axialOffset:{value:value??0,unit:'mm'}})}/></CommandField>
   <CommandField  label={<><span>平面方向</span></>} width="enum"><Select aria-label="平面方向" value={joint.axialDirection??'SAME'} disabled={disabled} options={[{value:'SAME',label:'同向'},{value:'OPPOSITE',label:'相对'}]} onChange={axialDirection=>onChange({axialDirection})}/></CommandField>
  </>}
  {(joint.coordinates??[]).map(coordinate=>{
   const angular=coordinate.dimension==='ANGLE',name=angular?'角度':'长度',unit=angular?'°':'mm',drive=Boolean(driving[coordinate.id]),reference=drive?references[coordinate.id]??0:0,span=angular?360:100;
   return <section key={coordinate.id} className="motion-dialog-section">
    <Checkbox checked={drive} disabled={disabled||referenced.has(coordinate.id)} onChange={event=>onDrive(coordinate.id,event.target.checked)}>{name}驱动</Checkbox>
    {referenced.has(coordinate.id)&&<Typography.Text type="secondary">命令被模拟引用</Typography.Text>}
    <Collapse size="small" defaultActiveKey={['limits']} items={[{key:'limits',label:joint.kind==='CYLINDRICAL'?`${name}限制`:'接合限制',children:<div className="motion-study-fields">
     <Select aria-label={angular?'旋转方向':'移动方向'} value={coordinate.direction} disabled={disabled} options={[{value:1,label:'正向 +Z'},{value:-1,label:'反向 -Z'}]} onChange={direction=>coordinatePatch(coordinate.id,{direction})}/>
     {(['lower','upper'] as const).map(bound=><CommandField key={bound} label={`${bound==='lower'?'下限':'上限'} ${unit}`} width="expression"><div className="command-inline-option"><Checkbox aria-label={`启用${name}${bound==='lower'?'下限':'上限'}`} checked={drive||Boolean(coordinate[bound])} disabled={disabled||drive} onChange={event=>coordinatePatch(coordinate.id,{[bound]:event.target.checked?jointLimitValue(bound==='lower'?-span:span,reference,coordinate):undefined})}/><InputNumber aria-label={`${name}${bound==='lower'?'下限':'上限'} ${unit}`} value={jointLimitReading(coordinate[bound],reference,coordinate)} precision={2} changeOnBlur={false} disabled={disabled||!drive&&!coordinate[bound]} onChange={value=>coordinatePatch(coordinate.id,{[bound]:value===null?undefined:jointLimitValue(value,reference,coordinate)})}/></div></CommandField>)}
    </div>}]} />
   </section>;
  })}
  {limitsIssue&&<Typography.Text type="warning">{limitsIssue}</Typography.Text>}
 </div>;
}
