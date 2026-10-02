import {Alert,Button,Descriptions,List,Space,Typography} from "antd";
import type {DocumentView as DocumentDescriptor} from "../../types";
import {assemblyConflictRepairs,type AssemblyConflictReport,type AssemblyConflictMember,type AssemblyConflictRepair} from "../../cad/assembly/assembly-conflict";
import {assemblyEngineeringOverview,engineeringMember,engineeringReason,engineeringConstraintName,engineeringInstanceName,type AssemblyEngineeringEvidence} from "../../cad/assembly/assembly-engineering-state";
import {CommandDialog} from "../../cad/overlay/floating-panel";

type Props={open:boolean;pending:boolean;report?:AssemblyConflictReport;current:boolean;error?:string;canEdit:boolean;view?:DocumentDescriptor;evidence?:AssemblyEngineeringEvidence;onLocateInstance:(id:string)=>void;
  onAnalyze:(ids?:string[])=>void;onStop:()=>void;onClose:()=>void;
  onLocate:(member:AssemblyConflictMember,revisionId:string)=>void;onRepair:(member:AssemblyConflictMember,action:AssemblyConflictRepair,revisionId:string)=>void};
const actionLabels:Record<AssemblyConflictRepair,string>={EDIT:"编辑",SUPPRESS:"停用",MEASURE:"切换测量",RECONNECT:"重连支持"};
export function AssemblyConflictPanel(p:Props){
  const overview=assemblyEngineeringOverview(p.view),definitions=p.view?.product?.constraints??[];
  const value=p.evidence;
  const evidence=value&&value.documentId===p.view?.document.id&&value.revisionId===p.view?.document.versionId&&value.available?value:undefined;
  const name=(id:string)=>engineeringInstanceName(p.view,id);
  const freedomNames=["固定","转动","滑动","圆柱运动","平面运动","球面运动","自由运动","耦合运动"];
  const member=(value:AssemblyConflictMember,current:boolean,revisionId=p.view?.document.versionId??"")=>{
    const definition=definitions.find(c=>c.id===value.constraintId);
    return <Space wrap><Typography.Text>{definition?engineeringConstraintName(definition,p.view):"原定义已变化"}</Typography.Text>
      <Button size="small" disabled={!current} onClick={()=>p.onLocate(value,revisionId)}>定位</Button>
      {assemblyConflictRepairs(value,definition).map(action=><Button key={action} size="small" disabled={!current||!p.canEdit} onClick={()=>p.onRepair(value,action,revisionId)}>{actionLabels[action]}</Button>)}</Space>;
  };
  return <CommandDialog id="assembly-conflict-analysis" title={`装配状态 · ${overview.name}`} open={p.open} width={680} footer={false} onClose={p.onClose} onConfirm={p.onClose}>
    <Typography.Title level={5}>整体概览</Typography.Title>
    <Descriptions bordered size="small" column={2} items={[
      {key:"status",label:"当前状态",children:overview.status},{key:"units",label:"装配运动组件",children:overview.instances.length},
      {key:"count",label:"用户约束定义",children:overview.total},{key:"active",label:"已满足活动项",children:overview.verified},
      {key:"suppressed",label:"停用",children:overview.suppressed},{key:"measured",label:"测量模式（含停用）",children:overview.measured},
      {key:"broken",label:"支持断开（含非驱动项）",children:overview.broken},{key:"notUpdated",label:"待更新（含非驱动项）",children:overview.notUpdated},
    ]}/>
    <List size="small" dataSource={overview.instances} renderItem={item=><List.Item>{item.name}<Typography.Text type="secondary">{item.state}</Typography.Text></List.Item>}/>
    <Typography.Paragraph type="secondary">停用与测量是独立维度。运动范围不按组件自由度相加；未完全约束可以是正常设计状态。</Typography.Paragraph>
    {evidence?<List size="small" dataSource={evidence.components} renderItem={component=><List.Item><div>
      <Typography.Text strong>{component.bodyIds.map(name).join(" / ")}</Typography.Text>
      <div>{component.solved?`相对可用运动：${component.relativeDof}；整体运动：${component.gaugeDof}`:"该组件网络尚未获得满足证据"}</div>
      {component.solved&&component.freedoms.filter(f=>!f.relativeToBodyId||f.relativeToBodyId!==f.bodyId).map(f=><div key={f.bodyId}>
        {name(f.bodyId)}：{freedomNames[f.kind]??"运动类型未确定"}{f.relativeToBodyId?`，相对 ${name(f.relativeToBodyId)}`:""}
        <Button size="small" onClick={()=>p.onLocateInstance(f.bodyId)}>定位组件</Button>
        {f.kind!==7&&f.translationDirections?.map((v,index)=><div key={`t-${index}`}>沿 ({v.map(n=>n.toFixed(2)).join(", ")}) 平移</div>)}
        {f.kind!==7&&f.rotations?.map((r,index)=><div key={`r-${index}`}>绕 ({r.direction.map(n=>n.toFixed(2)).join(", ")}) 转动</div>)}
      </div>)}
      <Typography.Text type="secondary">方向位于该装配坐标框架；局部瞬时运动，不表示任意有限行程。</Typography.Text>
    </div></List.Item>}/>:<Typography.Paragraph type="secondary">此版本没有匹配的运动范围证据；不沿用旧版本自由度。</Typography.Paragraph>}
    <Typography.Title level={5}>待处理问题</Typography.Title>
    <List dataSource={overview.issues} locale={{emptyText:"当前没有待处理的活动定义"}} renderItem={definition=><List.Item><div>
      {member(engineeringMember(definition),true)}<div>{({BROKEN:"支持已断开，请检查来源。",IMPOSSIBLE:"定义未被接纳，请检查几何要求。",NOT_UPDATED:"尚未获得有效更新。"})[definition.evaluationStatus as "BROKEN"|"IMPOSSIBLE"|"NOT_UPDATED"]}</div>
      <Button size="small" loading={p.pending} onClick={()=>p.onAnalyze([definition.id])}>检查相关关系</Button>
    </div></List.Item>}/>
    <Space><Button loading={p.pending} onClick={()=>p.onAnalyze()}>检查当前约束网络</Button>{p.pending&&<Button onClick={p.onStop}>取消检查</Button>}</Space>
    {p.error&&<Alert type="error" message="检查未完成，请重试或查看技术诊断"/>}
    {p.report&&<>
      <Typography.Title level={5}>本次局部检查</Typography.Title>
      {!p.current&&<Alert type="warning" message="装配已变化，请重新检查；旧诊断不可用于修复。"/>}
      <Typography.Paragraph>{p.report.status==="SAT"?"所检查范围已找到满足位置，不代表全装配完成检查。":p.report.status==="UNSAT"?"所检查范围存在已证明的不兼容要求。":"检查尚无确定结论。"}</Typography.Paragraph>
      <List dataSource={p.report.items} renderItem={item=><List.Item><div>{engineeringReason(item)}<List size="small" dataSource={item.members} renderItem={value=><List.Item>{member(value,p.current,p.report!.baseRevisionId)}</List.Item>}/></div></List.Item>}/>
    </>}
    <details><summary>技术诊断 / 可复制报告</summary><Typography.Paragraph copyable={{text:JSON.stringify({report:p.report,error:p.error,evidence:p.evidence},null,2)}}>复制技术证据</Typography.Paragraph>
      <pre style={{whiteSpace:"pre-wrap",maxHeight:240,overflow:"auto"}}>{JSON.stringify({report:p.report,error:p.error,evidence:p.evidence},null,2)}</pre></details>
  </CommandDialog>;
}
