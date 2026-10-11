import {Button,Checkbox,Collapse,Input,InputNumber,Select,Space,Typography} from 'antd';
import type {DocumentView} from '../../types';
import type {MechanismJoint} from '../../cad/assembly/motion-study';
import {jointSupportLabel,type JointSupportRole} from '../../cad/assembly/mechanism-selection';

type Props={view:DocumentView;joint:MechanismJoint;mechanismName?:string;pickRole?:JointSupportRole;onRole:(role:JointSupportRole)=>void;onClear:(role:JointSupportRole)=>void;onChange:(patch:Partial<MechanismJoint>)=>void;angleCommand:boolean;hasCommand:boolean;onAngleCommand:(value:boolean)=>void;pending:boolean;ready:boolean;disabled:boolean};

export function RevoluteJointFields({view,joint,mechanismName,pickRole,onRole,onClear,onChange,angleCommand,hasCommand,onAngleCommand,pending,ready,disabled}:Props){
 const filled=[joint.first.axis,joint.second?.axis,joint.first.plane,joint.second?.plane].filter(Boolean).length;
 const support=(side:'first'|'second',field:'axis'|'plane')=>{
  const role=`${side}.${field}` as JointSupportRole,reference=joint[side]?.[field];
  const label=`${side==='first'?'第一':'第二'}${field==='axis'?'轴线':'定位平面'}`;
  const unavailable=disabled||field==='plane'&&!joint[side]?.axis;
  return <div className="motion-support-row" key={role} data-support-role={role}>
   <span className="motion-support-number">{side==='first'?'1':'2'}</span>
   <Button aria-label={label} type={pickRole===role?'primary':'default'} disabled={unavailable} onClick={()=>onRole(role)}>
    <span className="motion-support-title">{label}</span>
    <span className="motion-support-value">{reference?jointSupportLabel(view,reference):field==='plane'&&!joint[side]?.axis?'先选择该零件的轴线':'点击此处，再从三维或树中选择'}</span>
   </Button>
   {reference&&<Button size="small" aria-label={`清除${label}`} disabled={disabled} onClick={()=>onClear(role)}>清除</Button>}
  </div>;
 };
 return <div className="motion-joint-editor">
  <label className="motion-labeled-field"><span>接合名称</span><Input aria-label="接合名称" value={joint.name} disabled={disabled} onChange={e=>onChange({name:e.target.value})}/></label>
  <Typography.Text type="secondary">所属机械装置：{mechanismName}</Typography.Text>
  <section className="motion-dialog-section" aria-label="轴线重合">
   <h4>轴线重合</h4><p>选择两个不同零件的轴线、直边或圆柱面。圆边、圆锥面也可解析为轴线。</p>
   {support('first','axis')}{support('second','axis')}
  </section>
  <section className="motion-dialog-section" aria-label="轴向定位">
   <h4>轴向定位</h4><p>分别选择所属零件上垂直于轴线的平面面或基准平面。</p>
   {support('first','plane')}{support('second','plane')}
   <div className="motion-labeled-field"><span>轴向偏移 mm</span><InputNumber aria-label="轴向偏移 mm" value={joint.axialOffset?.value??0} disabled={disabled} onChange={v=>onChange({axialOffset:{value:v??0,unit:'mm'}})}/><Button size="small" disabled={disabled} onClick={()=>onChange({axialOffset:{value:0,unit:'mm'}})}>设为零偏移</Button></div>
  </section>
  <Checkbox checked={angleCommand||hasCommand} disabled={disabled||hasCommand} onChange={e=>onAngleCommand(e.target.checked)}>角度命令</Checkbox>
  <Typography.Text type="secondary">角度命令用于驱动旋转，接合本身保留旋转自由度。</Typography.Text>
  <Collapse size="small" items={[{key:'coordinate',label:'角度零位、方向与限位',children:<Space wrap>
   <label>零位 {joint.zero.unit}<InputNumber aria-label="角度零位" value={joint.zero.value} disabled={disabled} onChange={v=>onChange({zero:{...joint.zero,value:v??0}})}/></label>
   <Select aria-label="旋转方向" value={joint.direction} disabled={disabled} options={[{value:1,label:'正向 +Z'},{value:-1,label:'反向 -Z'}]} onChange={direction=>onChange({direction})}/>
   <InputNumber aria-label="角度下限" placeholder="下限（可空）" value={joint.lower?.value} disabled={disabled} onChange={v=>onChange({lower:v===null?undefined:{value:v,unit:joint.zero.unit}})}/>
   <InputNumber aria-label="角度上限" placeholder="上限（可空）" value={joint.upper?.value} disabled={disabled} onChange={v=>onChange({upper:v===null?undefined:{value:v,unit:joint.zero.unit}})}/>
  </Space>}]} />
  <section className="motion-dialog-section motion-generated-relations" aria-label="生成的机构约束">
   <h4>生成的机构约束</h4><div><span>轴线重合</span><span>第一轴线 ↔ 第二轴线</span></div><div><span>平面偏移</span><span>第一定位平面 ↔ 第二定位平面 · {joint.axialOffset?.value??0} mm</span></div>
   <p>保存后形成两个独立的机构约束；产品约束通过“转换为产品约束”创建。</p>
  </section>
  <div role="status" className="motion-dialog-status">已选择 {filled}/4 · {pending?'正在计算预览…':filled<4?'请完成几何选择':ready?'预览通过，可以保存':'等待合格预览'}</div>
 </div>;
}
