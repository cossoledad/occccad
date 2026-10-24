import {CommandDialog} from "../../cad/overlay/floating-panel";
import {Alert,Button,Collapse,Empty,Space,Tag,Typography} from "antd";
import type {DocumentView,ProductUpdatePlan} from "../../types";
import {engineeringConstraintName,engineeringInstanceName} from "../../cad/assembly/assembly-engineering-state";
import {acceptedSupportState,associationRepairGroups,associationPlanCurrent,constraintSupports} from "./association-repair-model";

type Props={open:boolean;view:DocumentView;plan?:ProductUpdatePlan;loading:boolean;error?:string;failure?:string;busy:boolean;canEdit:boolean;
 onClose:()=>void;onRetry:()=>void;onAccept:(revision:string,digest:string)=>void;onRefresh:(revision:string)=>void;
 onLocateInstance:(id:string,revision:string)=>void;onLocateConstraint:(id:string,revision:string,support?:number)=>void;onEdit:(id:string,revision:string,reconnect:boolean)=>void};
const states:Record<string,string>={VERIFIED:"已满足",BROKEN:"支持断开",NOT_UPDATED:"待更新",IMPOSSIBLE:"未接纳"};
const revision=(id?:string)=><Typography.Text code copyable={id?{text:id}:false} style={{fontSize:11,overflowWrap:"anywhere"}}>{id??"待检查"}</Typography.Text>;
export function AssociationRepairPanel(p:Props){
 const version=p.view.document.versionId,current=associationPlanCurrent(p.view,p.plan)&&!p.loading&&!p.error;
 const groups=associationRepairGroups(p.view,current?p.plan:undefined),definitions=p.view.product?.constraints??[];
 const enabled=current&&p.canEdit&&!p.busy;
 return <CommandDialog id="association-repair" open={p.open} title="关联更新与修复" size="M" footer={false} onConfirm={()=>{}} onClose={p.onClose}>
  <p>当前 owning Product：{p.view.document.name}<br/>已接受 Revision：{revision(version)}</p>
  <Space wrap style={{marginBottom:12}}>
   <Button loading={p.loading} disabled={p.busy} onClick={p.onRetry}>重试检查</Button>
   <Button disabled={!enabled||!p.plan?.canAccept||!p.plan.hasUpdates} onClick={()=>p.onAccept(version,p.plan!.digest)}>更新关联零件</Button>
   <Button disabled={!enabled||!p.plan?.canAccept||p.plan.hasUpdates} onClick={()=>p.onRefresh(version)}>重新计算</Button>
  </Space>
  {(!current||p.failure)&&<Alert type={p.error||p.failure?"error":"info"} title={!current?"更新检查未就绪":"上次更新未完成"} description={p.error??p.failure??"正在刷新当前版本；旧计划的操作已失效。"} style={{marginBottom:12}}/>}
  <p>普通 FOLLOW_HEAD 自动跟随；共享参数待更新时需明确接受。PINNED 固定版本。重试检查不改写约束定义。</p>
  {!groups.length&&<Empty description="当前 Product 没有直接引用的 Part"/>}
  {groups.map(group=><section key={group.documentId} aria-label={`引用零件 ${group.name}`} style={{borderTop:"1px solid var(--color-border)",paddingTop:10,marginTop:10}}>
   <Typography.Title level={5}>{group.name} · {group.instances.length} 个实例</Typography.Title>
   {group.parameters.some(n=>n.needsUpdate)&&<Alert type="info" title="共享参数有待接受的更新" description="更新关联零件会沿既有参数计划重新计算，再接受确切结果版本。"/>}
   {group.instances.map(row=><div key={row.instance.id} data-reference-instance={row.instance.id} style={{padding:"10px 0",borderBottom:"1px solid var(--color-border)"}}>
    <Space wrap><strong>{row.name}</strong><Tag>{row.instance.referenceMode??"FOLLOW_HEAD"}</Tag><Button size="small" onClick={()=>p.onLocateInstance(row.instance.id,version)}>定位实例</Button></Space>
    <div>路径：{row.path.display} <Typography.Text type="secondary" style={{fontSize:11}}>({row.path.canonical})</Typography.Text></div>
    <div>已接受：{revision(row.instance.versionId)}</div>
    {row.instance.referenceMode==="PINNED"?<div>固定版本，不参与跟随更新</div>:<div>{!current?"可用更新：待刷新":!row.update?"更新状态待检查":row.update.currency==="UPDATE_BLOCKED"?"更新被阻塞":row.update?.candidateRevisionId!==row.instance.versionId?"可用更新：":"当前已是已检查版本"} {current&&row.update?.candidateRevisionId&&revision(row.update.candidateRevisionId)}</div>}
    {[...(row.update?[row.update]:[]),...row.diagnostics].filter(e=>e.diagnostic||e.currency==="UPDATE_BLOCKED").map(e=><Alert key={e.bindingId} type="warning" title={`${e.name} · ${e.diagnosticCode??e.evaluation}`} description={`${e.sourceDisplayPath} → ${e.owningDisplayPath}：${e.diagnostic??"来源被阻塞"}`} style={{marginTop:6}}/>)}
   </div>)}
  </section>)}
  <Typography.Title level={5}>当前已接受约束与支持</Typography.Title>
  <p>以下状态来自上述 Revision。计划的 ASSEMBLY_SOLVE 描述已接受状态，affectedConstraintIds 是保守集合；均不表示候选版本的逐约束求解结论。</p>
  {definitions.map(c=><section key={`${version}/${c.id}`} data-repair-constraint={c.id} style={{borderTop:"1px solid var(--color-border)",padding:"12px 0"}}>
   <strong>{engineeringConstraintName(c,p.view)}</strong> <Tag>{states[c.evaluationStatus]??"待检查"}</Tag>{c.suppressed&&<Tag>停用</Tag>}{c.mode==="MEASURED"&&<Tag>测量</Tag>}
   <p>{c.evaluationSummary??"该修订没有附加诊断。"}</p>
   {c.evaluationFailure&&<p>{c.evaluationFailure.phase} · {c.evaluationFailure.code} {c.evaluationFailure.retryable?"可重试":""}</p>}
   <Space wrap><Button size="small" onClick={()=>p.onLocateConstraint(c.id,version)}>定位约束</Button>
    <Button size="small" disabled={!enabled} onClick={()=>p.onEdit(c.id,version,false)}>编辑</Button>
    <Button size="small" disabled={!enabled} onClick={()=>p.onEdit(c.id,version,true)}>Reconnect</Button></Space>
   {c.kind==="FIX_TOGETHER"&&(c.groupMembers??[]).map((member,index)=>{
    const instanceId=member.instancePath?.segments[0]?.instanceId??member.instanceId;
    const group=member.groupId?definitions.find(d=>d.id===member.groupId&&d.kind==="FIX_TOGETHER"):undefined;
    const instance=instanceId?p.view.product?.instances.find(i=>i.id===instanceId):undefined;
    const connected=!!(group||instance);
    return <div key={member.groupId??member.instancePath?.canonical??instanceId??index} style={{paddingTop:8}}>成员 · {group?.name??(instance?engineeringInstanceName(p.view,instance.id):member.groupId??instanceId??"缺失成员")}
     <Tag>{connected?"Connected":"NotConnected"}</Tag><Button size="small" disabled={!connected} onClick={()=>group?p.onLocateConstraint(group.id,version):instance&&p.onLocateInstance(instance.id,version)}>定位成员</Button>
    </div>;
   })}
   {constraintSupports(c).map(({label,reference},index)=>{const support=acceptedSupportState(p.view,reference),instance=reference.instancePath?.segments[0]?.instanceId??reference.instanceId;return <div key={label} style={{paddingTop:8}}>
    {label} · {engineeringInstanceName(p.view,instance)} · {reference.kind} <Tag>{support.label}</Tag><Button size="small" onClick={()=>p.onLocateConstraint(c.id,version,index)}>定位支持</Button>
    {(support.diagnosticCode||support.diagnostic)&&<div>{support.diagnosticCode} {support.diagnostic}</div>}
    {reference.resolution&&<div>支持解析版本：{revision(reference.resolution.targetVersionId)}</div>}
   </div>})}
  </section>)}
  {!definitions.length&&<Empty description="当前已接受 Revision 没有约束"/>}
  <Collapse size="small" items={[{key:"source",label:"计划与诊断来源（只读）",children:<Typography.Paragraph copyable={{text:JSON.stringify({revision:version,plan:current?p.plan:undefined},null,2)}}><pre style={{maxHeight:200,overflow:"auto",whiteSpace:"pre-wrap"}}>{JSON.stringify({revision:version,plan:current?p.plan:undefined},null,2)}</pre></Typography.Paragraph>}]}/>
 </CommandDialog>;
}
