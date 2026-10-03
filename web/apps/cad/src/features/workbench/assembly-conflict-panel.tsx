import {Alert,Button,Collapse,Dropdown,Input,Select,Space,Table,Tabs,Tag,Typography} from "antd";
import {useEffect,useMemo,useState} from "react";
import type {DocumentView as DocumentDescriptor} from "../../types";
import {assemblyConflictRepairs,type AssemblyConflictReport,type AssemblyConflictMember,type AssemblyConflictRepair} from "../../cad/assembly/assembly-conflict";
import {assemblyEngineeringOverview,engineeringMember,engineeringReason,engineeringConstraintName,engineeringInstanceName,type AssemblyEngineeringEvidence} from "../../cad/assembly/assembly-engineering-state";
import {CommandDialog} from "../../cad/overlay/floating-panel";
import {ContextMenuIcon} from "../../components/context-menu-icon";
import {motionPresentations,displayDirection,displayLength,type MotionPresentation} from "../../cad/assembly/motion-presentation";

type Props={open:boolean;pending:boolean;report?:AssemblyConflictReport;current:boolean;error?:string;canEdit:boolean;defaultTab?:"issues"|"all"|"motion";view?:DocumentDescriptor;ownerOccurrence?:string;lengthUnit?:"mm"|"cm"|"m"|"in";evidence?:AssemblyEngineeringEvidence;onLocateInstance:(id:string)=>void;onMotion?:(motion:MotionPresentation|undefined)=>void;
 onAnalyze:(ids?:string[])=>void;onStop:()=>void;onClose:()=>void;
 onLocate:(member:AssemblyConflictMember,revisionId:string)=>void;onRepair:(member:AssemblyConflictMember,action:AssemblyConflictRepair,revisionId:string)=>void};
const actions:Record<AssemblyConflictRepair,string>={EDIT:"编辑",SUPPRESS:"停用",MEASURE:"切换测量",RECONNECT:"重连支持"};
const statuses:Record<string,string>={VERIFIED:"已满足",BROKEN:"支持断开",NOT_UPDATED:"待更新",IMPOSSIBLE:"未接纳"};
export function AssemblyConflictPanel(p:Props){
 const [tab,setTab]=useState<string>(p.defaultTab??"issues"),[search,setSearch]=useState(""),[filter,setFilter]=useState("all"),[selected,setSelected]=useState<string>(),[technical,setTechnical]=useState(false);
 const overview=useMemo(()=>assemblyEngineeringOverview(p.view),[p.view]);
 const revision=p.view?.document.versionId??"",context=`${p.view?.document.id}/${p.ownerOccurrence??""}/${revision}`;
 const motions=useMemo(()=>motionPresentations(p.view,p.evidence,p.ownerOccurrence),[p.view,p.evidence,p.ownerOccurrence]);
 const definitions=p.view?.product?.constraints??[];
 const index=useMemo(()=>new Map(definitions.map(c=>[c.id,c])),[definitions]);
 const problems=useMemo(()=>new Map(p.current?(p.report?.items??[]).flatMap(item=>item.members.map(member=>[member.constraintId,item] as const)):[]),[p.report,p.current]);
 const rows=useMemo(()=>{
   if(tab==="motion")return motions.map(i=>({id:i.id,name:i.name,status:i.status,components:i.reference,description:i.description}));
   return (tab==="issues"?overview.issues:definitions).map(c=>({id:c.id,name:engineeringConstraintName(c,p.view),status:[c.suppressed?"停用":"",c.mode==="MEASURED"?"测量":"",statuses[c.evaluationStatus??""]??"待检查"].filter(Boolean).join(" / "),components:[c.first,c.second].filter(Boolean).map(r=>engineeringInstanceName(p.view,r!.instancePath?.segments[0]?.instanceId??r!.instanceId)).join(" / ")}));
 },[tab,overview,definitions,p.view,motions]);
 const visible=rows.filter(r=>(filter==="all"||r.status.includes(filter))&&`${r.name} ${r.components}`.toLocaleLowerCase().includes(search.toLocaleLowerCase()));
 const id=selected?.startsWith(`${context}:`)?selected.slice(context.length+1):undefined;
 const motionIndex=useMemo(()=>new Map(motions.map(m=>[m.id,m])),[motions]);
 const selectedMotion=tab==="motion"&&id?motionIndex.get(id):undefined;
 useEffect(()=>{p.onMotion?.(p.open&&selectedMotion?.freedom?selectedMotion:undefined);},[selectedMotion,p.open,p.onMotion]);
 useEffect(()=>()=>p.onMotion?.(undefined),[]);
 const definition=id?index.get(id):undefined,issue=id?problems.get(id):undefined;
 const act=(action:AssemblyConflictRepair)=>{if(definition)p.onRepair(engineeringMember(definition),action,revision);};
 const technicalText=technical?JSON.stringify({report:p.report,error:p.error,evidence:p.evidence},null,2):"";
 return <CommandDialog id="assembly-conflict-analysis" title={`装配状态 · ${overview.name}`} open={p.open} size="L" footer={false} onClose={p.onClose} onConfirm={p.onClose}>
  <Space wrap style={{marginBottom:8}}><Tag>{overview.status}</Tag><span>定义 {overview.total}</span><span>停用 {overview.suppressed}</span><span>测量 {overview.measured}</span><span>断链 {overview.broken}</span><span>待更新 {overview.notUpdated}</span></Space>
  <Tabs size="small" activeKey={tab} onChange={value=>{setTab(value);setFilter("all");setSelected(undefined);p.onMotion?.(undefined);}} items={[{key:"issues",label:`待处理问题 (${overview.issues.length})`},{key:"all",label:"全部约束"},{key:"motion",label:"剩余运动"}]}/>
  <Space wrap style={{marginBottom:8}}><Input.Search placeholder="搜索名称或组件" allowClear value={search} onChange={e=>setSearch(e.target.value)} style={{width:220}}/>
   <Select value={filter} onChange={setFilter} style={{width:120}} options={(tab==="motion"?["all","可运动","已定位","待计算","结果已失效"]:["all","停用","测量","已满足","支持断开","待更新","未接纳"]).map(value=>({value,label:value==="all"?"全部状态":value}))}/>
   <Button size="small" loading={p.pending} onClick={()=>p.onAnalyze(id&&definition?[id]:undefined)}>检查{definition?"所选关系":"当前网络"}</Button>{p.pending&&<Button size="small" onClick={p.onStop}>取消检查</Button>}
  </Space>
  <Table size="small" rowKey="id" dataSource={visible} scroll={{y:230,x:720}} pagination={{defaultPageSize:10,showSizeChanger:true,pageSizeOptions:[10,20,50],showTotal:n=>`${n} 项`}}
   rowSelection={{type:"radio",selectedRowKeys:id?[id]:[],onChange:keys=>setSelected(keys[0]?`${context}:${keys[0]}`:undefined)}} onRow={row=>({tabIndex:0,onClick:()=>setSelected(`${context}:${row.id}`),onKeyDown:e=>{if(e.key==="Enter"||e.key===" "){e.preventDefault();setSelected(`${context}:${row.id}`);}}})}
   columns={[{title:"名称",dataIndex:"name",ellipsis:true},{title:"状态",dataIndex:"status",width:95},{title:tab==="motion"?"参考对象":"相关组件",dataIndex:"components",ellipsis:true,width:130},...(tab==="motion"?[{title:"剩余运动",dataIndex:"description",ellipsis:true}]:[]),
    {title:"操作",width:90,render:(_,row)=><Button size="small" onClick={e=>{e.stopPropagation();if(tab==="motion"){const m=motionIndex.get(row.id);if(m){setSelected(`${context}:${row.id}`);p.onLocateInstance(m.bodyId);}}else {const c=index.get(row.id);if(c)p.onLocate(engineeringMember(c),revision);}}}>定位</Button>}]}/>
  {definition&&tab!=="motion"&&<div style={{borderTop:"1px solid #53606b",paddingTop:8}}>
   <Typography.Text strong>{engineeringConstraintName(definition,p.view)}</Typography.Text><div>{issue?engineeringReason(issue):statuses[definition.evaluationStatus??""]??"尚无评价证据"}</div>
   <Space wrap><Button size="small" disabled={!p.canEdit} onClick={()=>act("EDIT")}>编辑</Button>
    <Dropdown trigger={["click"]} placement="bottomLeft" classNames={{root:"cad-command-menu"}} menu={{items:assemblyConflictRepairs(engineeringMember(definition),definition).filter(a=>a!=="EDIT").map(action=>({key:action,disabled:!p.canEdit,icon:<ContextMenuIcon/>,label:actions[action]})),onClick:info=>act(info.key as AssemblyConflictRepair)}}><Button size="small">更多操作</Button></Dropdown>
    {issue?.members.map(member=>{const c=index.get(member.constraintId);return c&&member.constraintId!==definition.id?<Button key={member.constraintId} size="small" onClick={()=>p.onLocate(member,revision)}>{engineeringConstraintName(c,p.view)}</Button>:null;})}
   </Space>
  </div>}
  {selectedMotion&&<div style={{borderTop:"1px solid var(--color-border)",paddingTop:8,maxHeight:170,overflow:"auto"}}>
   <Typography.Text strong>{selectedMotion.description}</Typography.Text>
   <div style={{display:"grid",gridTemplateColumns:"130px minmax(0,1fr)",gap:"4px 12px",marginTop:8}}>
    <span>参考对象</span><span>{selectedMotion.reference}</span>
    {selectedMotion.freedom?.translationDirections.map((d,i)=><div key={`t${i}`} style={{display:"contents"}}><span>可平移方向 {i+1}</span><span>{displayDirection(d)}</span></div>)}
    {selectedMotion.freedom?.rotations.map((r,i)=><div key={`r${i}`} style={{display:"contents"}}><span>旋转轴 {i+1}</span><span>{displayDirection(r.direction)}</span><span>轴上参考位置</span><span>{r.axisPoint.map(v=>displayLength(v,p.lengthUnit)).join(" · ")}</span>{Math.abs(r.pitch)>1e-12&&<><span>螺距（每弧度）</span><span>{displayLength(r.pitch,p.lengthUnit)}</span></>}</div>)}
   </div>
  </div>}
  {p.report&&<Typography.Paragraph style={{margin:"8px 0"}}>{!p.current?"结果已失效":p.report.status==="SAT"?"所选范围：约束可满足":p.report.status==="UNSAT"?"所选范围：存在不兼容要求":"检查未完成"}</Typography.Paragraph>}
  {p.error&&<Alert type="error" message="检查未完成，可重试或查看技术详情"/>}
  <Collapse size="small" activeKey={technical?["technical"]:[]} onChange={keys=>setTechnical(keys.includes("technical"))} items={[{key:"technical",label:"技术诊断 / 可复制报告",children:technical?<Typography.Paragraph copyable={{text:technicalText}}><pre style={{whiteSpace:"pre-wrap",maxHeight:140,overflow:"auto"}}>{technicalText}</pre></Typography.Paragraph>:null}]}/>
 </CommandDialog>;
}
