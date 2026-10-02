import {Alert,Button,Descriptions,InputNumber,List,Space,Tag,Typography} from "antd";
import type {AssemblyConstraint} from "../../types";
import {assemblyConflictEvidenceLabel,assemblyConflictRepairs,type AssemblyConflictReport,type AssemblyConflictMember,type AssemblyConflictRepair} from "../../cad/assembly/assembly-conflict";
import {CommandDialog} from "../../cad/overlay/floating-panel";

type Props={open:boolean;pending:boolean;report?:AssemblyConflictReport;current:boolean;error?:string;canEdit:boolean;definitions:AssemblyConstraint[];
  maxProbes:number;timeBudgetMs:number;onBudget:(probes:number,timeMs:number)=>void;onAnalyze:()=>void;onStop:()=>void;onClose:()=>void;
  onLocate:(member:AssemblyConflictMember)=>void;onRepair:(member:AssemblyConflictMember,action:AssemblyConflictRepair)=>void};
const actionLabels:Record<AssemblyConflictRepair,string>={EDIT:"编辑定义",SUPPRESS:"停用定义",MEASURE:"切换测量",RECONNECT:"重连支持"};

/** Diagnostic evidence never mutates activation/mode implicitly. Every repair
 * below is a separately clicked existing formal Domain Command or editor. */
export function AssemblyConflictPanel(p:Props){
  return <CommandDialog id="assembly-conflict-analysis" title="装配约束分析" open={p.open} width={760} onClose={p.onClose} onConfirm={p.onClose} confirmText="关闭" cancelText="关闭">
    <Space wrap style={{marginBottom:12}}>
      <span>探测上限</span><InputNumber aria-label="约束分析探测上限" min={1} max={256} value={p.maxProbes} disabled={p.pending} onChange={v=>p.onBudget(v??32,p.timeBudgetMs)}/>
      <span>时间预算（ms）</span><InputNumber aria-label="约束分析时间预算" min={100} max={30000} step={100} value={p.timeBudgetMs} disabled={p.pending} onChange={v=>p.onBudget(p.maxProbes,v??5000)}/>
      <Button type="primary" loading={p.pending} onClick={p.onAnalyze}>分析当前快照</Button>
      {p.pending&&<Button onClick={p.onStop}>取消分析</Button>}
    </Space>
    <Alert type="info" showIcon message="分析与操纵受限不是同一件事" description="鼠标目标不可达不证明约束冲突。数值未收敛/预算耗尽标为未知；局部冗余不要求删除。不可约集不承诺最小基数。分析不会推进 Head 或自动停用定义。"/>
    {p.error&&<Alert style={{marginTop:12}} type="error" showIcon message="分析未完成" description={p.error}/>}
    {p.report&&<>
      {!p.current&&<Alert style={{marginTop:12}} type="warning" showIcon message="诊断已失效" description="装配版本或上下文已变化。旧证据仅供查看，请重新分析；所有定位/修复操作已禁用。"/>}
      <Descriptions bordered size="small" column={2} style={{marginTop:12}} items={[
        {key:"status",label:"Oracle",children:<Tag color={p.report.status==="UNSAT"?"red":p.report.status==="SAT"?"green":"gold"}>{p.report.status}</Tag>},
        {key:"complete",label:"探测完成",children:p.report.complete?"是":"否（不补造结论）"},
        {key:"head",label:"冻结 Revision",children:p.report.baseRevisionId},
        {key:"policy",label:"Solver policy/build",children:p.report.solverBuildPolicy},
        {key:"input",label:"输入 digest",children:p.report.inputDigest},
        {key:"budget",label:"实际探测 / 用时",children:`${p.report.probeCount} / ${p.report.elapsedMs.toFixed(1)} ms`},
        {key:"scope",label:"范围",children:`${p.report.scopeConstraintIds.length} 定义 / ${p.report.scopeBodyIds.length} 运动单元`},
        {key:"background",label:"固定背景",children:p.report.backgroundConstraintIds.length?p.report.backgroundConstraintIds.join(", "):"无额外隐藏背景"},
      ]}/>
      {p.report.budgetReason&&<Alert type="warning" message={p.report.budgetReason}/>}
      <List dataSource={p.report.items} locale={{emptyText:"该范围没有诊断条目"}} renderItem={item=><List.Item>
        <div style={{width:"100%"}}><Typography.Text strong>{assemblyConflictEvidenceLabel(item)}</Typography.Text> <Tag>{item.oracle}</Tag>
          <div>{item.reason}</div>
          <List size="small" dataSource={item.members} renderItem={member=>{
            const definition=p.definitions.find(c=>c.id===member.constraintId);
            return <List.Item><Space wrap>
              <Typography.Text>{definition?.name??definition?.family??definition?.kind??member.constraintId}</Typography.Text>
              {member.groupId&&<Tag>Group {member.groupId}</Tag>}
              {member.equationIds?.length? <Typography.Text type="secondary">方程来源：{member.equationIds.join(", ")}</Typography.Text>:null}
              <Button size="small" disabled={!p.current} onClick={()=>p.onLocate(member)}>定位及高亮支持</Button>
              {assemblyConflictRepairs(member,definition).map(action=><Button key={action} size="small" disabled={!p.current||!p.canEdit} onClick={()=>p.onRepair(member,action)}>{actionLabels[action]}</Button>)}
              <Typography.Text type="secondary">{member.constraintId}</Typography.Text>
            </Space></List.Item>;
          }}/>
        </div>
      </List.Item>}/>
      <details><summary>冻结分支及实际 probe 证据</summary><pre style={{whiteSpace:"pre-wrap",maxHeight:240,overflow:"auto"}}>{JSON.stringify({branches:p.report.branches,probes:p.report.probes},null,2)}</pre></details>
    </>}
  </CommandDialog>;
}
