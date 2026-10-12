import {Button,Checkbox,Collapse,Input,InputNumber,Select,Typography} from 'antd';
import type {DocumentView} from '../../types';
import type {MechanismJoint} from '../../cad/assembly/motion-study';
import {jointSupportLabel,type JointSupportRole} from '../../cad/assembly/mechanism-selection';
import {jointLimitReading,jointLimitValue} from '../../cad/assembly/motion-coordinates';

type Props={view:DocumentView;joint:MechanismJoint;mechanismName?:string;pickRole?:JointSupportRole;onRole:(role:JointSupportRole)=>void;onClear:(role:JointSupportRole)=>void;onChange:(patch:Partial<MechanismJoint>)=>void;angleCommand:boolean;commandReferenced:boolean;reference:number;limitsIssue?:string;onAngleCommand:(value:boolean)=>void;pending:boolean;disabled:boolean};

export function RevoluteJointFields({view,joint,mechanismName,pickRole,onRole,onClear,onChange,angleCommand,commandReferenced,reference,limitsIssue,onAngleCommand,pending,disabled}:Props){
 const support=(side:'first'|'second',field:'axis'|'plane')=>{
  const role=`${side}.${field}` as JointSupportRole,reference=joint[side]?.[field];
  const label=`${side==='first'?'第一':'第二'}${field==='axis'?'轴线':'定位平面'}`;
  const unavailable=disabled||field==='plane'&&!joint[side]?.axis;
  return <div className="motion-support-row" key={role} data-support-role={role}>
   <span className="motion-support-number">{side==='first'?'1':'2'}</span>
   <Button aria-label={label} type={pickRole===role?'primary':'default'} disabled={unavailable} onClick={()=>onRole(role)}>
    <span className="motion-support-title">{label}</span>
    <span className="motion-support-value">{reference?jointSupportLabel(view,reference):field==='plane'&&!joint[side]?.axis?'先选轴线':'选择几何'}</span>
   </Button>
   {reference&&<Button size="small" aria-label={`清除${label}`} disabled={disabled} onClick={()=>onClear(role)}>清除</Button>}
  </div>;
 };
 return <div className="motion-joint-editor">
  <label className="motion-labeled-field"><span>接合名称</span><Input aria-label="接合名称" value={joint.name} disabled={disabled} onChange={e=>onChange({name:e.target.value})}/></label>
  <Typography.Text type="secondary">所属机械装置：{mechanismName}</Typography.Text>
  <section className="motion-dialog-section" aria-label="轴线重合">
   <h4>轴线重合</h4>
   {support('first','axis')}{support('second','axis')}
  </section>
  <section className="motion-dialog-section" aria-label="轴向定位">
   <h4>轴向定位</h4>
   {support('first','plane')}{support('second','plane')}
   <div className="motion-labeled-field"><span>轴向偏移 mm</span><InputNumber aria-label="轴向偏移 mm" value={joint.axialOffset?.value??0} disabled={disabled} onChange={v=>onChange({axialOffset:{value:v??0,unit:'mm'}})}/></div>
  <label className="motion-labeled-field"><span>平面方向</span><Select aria-label="平面方向" value={joint.axialDirection??'SAME'} disabled={disabled} options={[{value:'SAME',label:'同向'},{value:'OPPOSITE',label:'相对'}]} onChange={axialDirection=>onChange({axialDirection})}/></label>
  </section>
  <Checkbox checked={angleCommand} disabled={disabled||commandReferenced} onChange={e=>onAngleCommand(e.target.checked)}>角度驱动</Checkbox>
  {commandReferenced&&<Typography.Text type="secondary">命令被模拟引用</Typography.Text>}
  <Collapse size="small" defaultActiveKey={['limits']} items={[{key:'limits',label:'接合限制',children:<div className="motion-study-fields">
   <Select aria-label="旋转方向" value={joint.direction} disabled={disabled} options={[{value:1,label:'正向 +Z'},{value:-1,label:'反向 -Z'}]} onChange={direction=>onChange({direction})}/>
   <label className="motion-labeled-field"><Checkbox aria-label="启用角度下限" checked={angleCommand||Boolean(joint.lower)} disabled={disabled||angleCommand} onChange={e=>onChange({lower:e.target.checked?jointLimitValue(-360,reference):undefined})}>下限 °</Checkbox><InputNumber aria-label="角度下限 °" value={jointLimitReading(joint.lower,reference)} precision={2} changeOnBlur={false} disabled={disabled||!angleCommand&&!joint.lower} onChange={v=>onChange({lower:v===null?undefined:jointLimitValue(v,reference)})}/></label>
   <label className="motion-labeled-field"><Checkbox aria-label="启用角度上限" checked={angleCommand||Boolean(joint.upper)} disabled={disabled||angleCommand} onChange={e=>onChange({upper:e.target.checked?jointLimitValue(360,reference):undefined})}>上限 °</Checkbox><InputNumber aria-label="角度上限 °" value={jointLimitReading(joint.upper,reference)} precision={2} changeOnBlur={false} disabled={disabled||!angleCommand&&!joint.upper} onChange={v=>onChange({upper:v===null?undefined:jointLimitValue(v,reference)})}/></label>
   {limitsIssue&&<Typography.Text type="warning">{limitsIssue}</Typography.Text>}
  </div>}]} />
  {pending&&<div role="status" className="motion-dialog-status">计算中…</div>}

 </div>;
}
