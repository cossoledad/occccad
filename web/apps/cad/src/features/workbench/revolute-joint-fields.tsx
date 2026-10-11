import {Button,Checkbox,Collapse,Input,InputNumber,Select,Space,Typography} from 'antd';
import type {DocumentView} from '../../types';
import type {MechanismJoint} from '../../cad/assembly/motion-study';
import {jointSupportLabel,type JointSupportRole} from '../../cad/assembly/mechanism-selection';

type Props={view:DocumentView;joint:MechanismJoint;mechanismName?:string;pickRole?:JointSupportRole;onRole:(role:JointSupportRole)=>void;onClear:(role:JointSupportRole)=>void;onChange:(patch:Partial<MechanismJoint>)=>void;angleCommand:boolean;hasCommand:boolean;onAngleCommand:(value:boolean)=>void;pending:boolean;disabled:boolean};

export function RevoluteJointFields({view,joint,mechanismName,pickRole,onRole,onClear,onChange,angleCommand,hasCommand,onAngleCommand,pending,disabled}:Props){
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
  <Checkbox checked={angleCommand||hasCommand} disabled={disabled||hasCommand} onChange={e=>onAngleCommand(e.target.checked)}>角度命令</Checkbox>
  <Collapse size="small" items={[{key:'coordinate',label:'角度零位、方向与限位',children:<Space wrap>
   <label>零位 {joint.zero.unit}<InputNumber aria-label="角度零位" value={joint.zero.value} disabled={disabled} onChange={v=>onChange({zero:{...joint.zero,value:v??0}})}/></label>
   <Select aria-label="旋转方向" value={joint.direction} disabled={disabled} options={[{value:1,label:'正向 +Z'},{value:-1,label:'反向 -Z'}]} onChange={direction=>onChange({direction})}/>
   <InputNumber aria-label="角度下限" placeholder="下限（可空）" value={joint.lower?.value} disabled={disabled} onChange={v=>onChange({lower:v===null?undefined:{value:v,unit:joint.zero.unit}})}/>
   <InputNumber aria-label="角度上限" placeholder="上限（可空）" value={joint.upper?.value} disabled={disabled} onChange={v=>onChange({upper:v===null?undefined:{value:v,unit:joint.zero.unit}})}/>
  </Space>}]} />
  {pending&&<div role="status" className="motion-dialog-status">计算中…</div>}

 </div>;
}
