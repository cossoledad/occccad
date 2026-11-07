import {engineeringLabel} from "../../cad/presentation/engineering-text";
import {engineeringProblem} from "../../cad/command/operation-feedback";
import {Alert,Button,Collapse,Input,Select,Space,Table,Tabs,Tag,Typography} from "antd";
import {useEffect,useMemo,useState} from "react";
import type {DocumentView as DocumentDescriptor} from "../../types";
import {assemblyConflictRepairs,type AssemblyConflictReport,type AssemblyConflictMember,type AssemblyConflictRepair} from "../../cad/assembly/assembly-conflict";
import {assemblyEngineeringOverview,engineeringMember,engineeringReason,engineeringConstraintName,engineeringInstanceName,type AssemblyEngineeringEvidence} from "../../cad/assembly/assembly-engineering-state";
import {CommandDialog} from "../../cad/overlay/floating-panel";
import {motionPresentations,motionDegrees,displayDirection,displayLength,type MotionPresentation} from "../../cad/assembly/motion-presentation";

type Props={selectedInstanceId?:string;onSelectInstance?:(id:string)=>void;open:boolean;pending:boolean;report?:AssemblyConflictReport;current:boolean;error?:string;canEdit:boolean;defaultTab?:"issues"|"all"|"motion";view?:DocumentDescriptor;ownerOccurrence?:string;lengthUnit?:"mm"|"cm"|"m"|"in";evidence?:AssemblyEngineeringEvidence;onLocateInstance:(id:string)=>void;onMotion?:(motion:MotionPresentation|undefined)=>void;
 motionStatus?:string;motionError?:string;onRefreshMotion?:()=>void;onCancelMotion?:()=>void;
 onAnalyze:(ids?:string[])=>void;onStop:()=>void;onClose:()=>void;
 onLocate:(member:AssemblyConflictMember,revisionId:string)=>void;onRepair:(member:AssemblyConflictMember,action:AssemblyConflictRepair,revisionId:string)=>void};
const statuses:Record<string,string>={VERIFIED:"已满足",BROKEN:"支持断开",NOT_UPDATED:"待更新",IMPOSSIBLE:"未接纳"};
export function AssemblyConflictPanel(p:Props){
 const [tab,setTab]=useState<string>(p.defaultTab??"issues"),[search,setSearch]=useState(""),[filter,setFilter]=useState("all"),[selected,setSelected]=useState<string>();
 const [directions,setDirections]=useState<{key:string;ids:string[]}>();
 const overview=useMemo(()=>assemblyEngineeringOverview(p.view),[p.view]);
 const revision=p.view?.document.versionId??"",context=`${p.view?.document.id}/${p.ownerOccurrence??""}/${revision}`;
 const motions=useMemo(()=>motionPresentations(p.view,p.evidence,p.ownerOccurrence),[p.view,p.evidence,p.ownerOccurrence]);
 const definitions=p.view?.product?.constraints??[];
 const index=useMemo(()=>new Map(definitions.map(c=>[c.id,c])),[definitions]);
 const problems=useMemo(()=>new Map(p.current?(p.report?.items??[]).flatMap(item=>item.members.map(member=>[member.constraintId,item] as const)):[]),[p.report,p.current]);
 const rows=useMemo(()=>{
   if(tab==="motion")return motions.map(i=>({id:i.id,name:i.name,status:i.status,components:motionDegrees(i)?.overall?"连通组件整体":i.reference,description:i.description}));
   return (tab==="issues"?overview.issues:definitions).map(c=>({id:c.id,name:engineeringConstraintName(c,p.view),status:[c.suppressed?"停用":"",c.mode==="MEASURED"?"测量":"",statuses[c.evaluationStatus??""]??"待检查"].filter(Boolean).join(" / "),components:[c.first,c.second].filter(Boolean).map(r=>engineeringInstanceName(p.view,r!.instancePath?.segments[0]?.instanceId??r!.instanceId)).join(" / ")}));
 },[tab,overview,definitions,p.view,motions]);
 const visible=rows.filter(r=>(filter==="all"||r.status.includes(filter))&&`${r.name} ${r.components}`.toLocaleLowerCase().includes(search.toLocaleLowerCase()));
 const id=selected?.startsWith(`${context}:`)?selected.slice(context.length+1):undefined;
 const motionIndex=useMemo(()=>new Map(motions.map(m=>[m.id,m])),[motions]);
 const baseMotion=tab==="motion"&&id?motionIndex.get(id):undefined;
 const selectedMotion=useMemo(()=>baseMotion?{...baseMotion,activeDirections:directions?.key===`${context}:${baseMotion.id}`?directions.ids:undefined}:undefined,[baseMotion,directions,context]);
 const degrees=selectedMotion?motionDegrees(selectedMotion):undefined;
 const choose=(key:string)=>{setSelected(`${context}:${key}`);setDirections(undefined);if(tab==="motion"){const motion=motionIndex.get(key);if(motion)p.onSelectInstance?.(motion.bodyId);}};
 useEffect(()=>{if(p.open&&tab==="motion"&&p.selectedInstanceId){const motion=motions.find(m=>m.bodyId===p.selectedInstanceId);if(motion)setSelected(`${context}:${motion.id}`);}},[p.open,tab,p.selectedInstanceId,context]);
 useEffect(()=>{p.onMotion?.(p.open&&degrees?.directions.length?selectedMotion:undefined);},[selectedMotion,p.open,p.onMotion]);
 useEffect(()=>()=>p.onMotion?.(undefined),[]);
 const definition=id?index.get(id):undefined;

 return <CommandDialog id="assembly-conflict-analysis" title={`约束分析 · ${overview.name}`} open={p.open} size="L" footer={false} onClose={p.onClose} onConfirm={p.onClose}>
  <Space wrap style={{marginBottom:8}}><Tag>{overview.status}</Tag><span>定义 {overview.total}</span><span>停用 {overview.suppressed}</span><span>测量 {overview.measured}</span><span>断链 {overview.broken}</span><span>待更新 {overview.notUpdated}</span></Space>
  <Tabs size="small" activeKey={tab} onChange={value=>{setTab(value);setFilter("all");setSelected(undefined);p.onMotion?.(undefined);}} items={[{key:"issues",label:`待处理问题 (${overview.issues.length})`},{key:"all",label:"全部约束"},{key:"motion",label:"自由度"}]}/>
  <Space wrap style={{marginBottom:8}}><Input.Search placeholder="搜索名称或组件" allowClear value={search} onChange={e=>setSearch(e.target.value)} style={{width:220}}/>
   <Select value={filter} onChange={setFilter} style={{width:120}} options={(tab==="motion"?["all","可运动","已定位","待计算","结果已失效"]:["all","停用","测量","已满足","支持断开","待更新","未接纳"]).map(value=>({value,label:value==="all"?"全部状态":value}))}/>
   {tab!=="motion"?<><Button size="small" loading={p.pending} onClick={()=>p.onAnalyze(id&&definition?[id]:undefined)}>检查{definition?"所选关系":"当前网络"}</Button>{p.pending&&<Button size="small" onClick={p.onStop}>取消检查</Button>}</>:<>
    <Button size="small" disabled={!p.onRefreshMotion||p.motionStatus==="COMPUTING"} onClick={p.onRefreshMotion}>{(["FAILED","CANCELLED"].includes(p.motionStatus??"")||p.evidence?.status==="UNKNOWN")?"重试":"刷新剩余运动"}</Button>
    {p.motionStatus==="COMPUTING"&&<Button size="small" onClick={p.onCancelMotion}>取消计算</Button>}
   </>}
  </Space>
  {tab==="motion"&&<div role="status" aria-label="剩余运动计算状态" style={{marginBottom:8}}>
   {['COMPUTING','READING','CANCELLED'].includes(p.motionStatus??'')&&<span>{p.motionStatus==='COMPUTING'?'正在计算…':p.motionStatus==='READING'?'正在读取…':'已取消'}</span>}
   {!!p.evidence?.excluded?.length&&<Collapse size="small" items={[{key:"excluded",label:`未纳入硬约束集合 (${p.evidence.excluded.length})`,children:p.evidence.excluded.map(e=><p key={e.constraintId}>{index.has(e.constraintId)?engineeringConstraintName(index.get(e.constraintId)!,p.view):"引用约束已丢失"} · {engineeringLabel(e.reason,"未纳入当前分析")}</p>)}]}/>}
   {(p.motionError||p.evidence?.diagnosticCode)&&<Alert type="warning" title={p.motionStatus==='FAILED'?'计算失败':'当前运动尚无法确定'} description={engineeringProblem(p.motionError??{code:p.evidence?.diagnosticCode,message:p.evidence?.diagnostic})}/>}
  </div>}
  <div className={tab==="motion"&&selectedMotion?"assembly-motion-workarea":""}>
  <Table size="small" rowKey="id" dataSource={visible} scroll={{y:230,x:tab==="motion"&&selectedMotion?580:720}} pagination={{defaultPageSize:10,showSizeChanger:true,pageSizeOptions:[10,20,50],showTotal:n=>`${n} 项`}}
   rowSelection={{type:"radio",selectedRowKeys:id?[id]:[],onChange:keys=>keys[0]?choose(String(keys[0])):setSelected(undefined)}} onRow={row=>({tabIndex:0,onClick:()=>choose(row.id),onKeyDown:e=>{if(e.key==="Enter"||e.key===" "){e.preventDefault();choose(row.id);}}})}
   columns={[{title:"名称",dataIndex:"name",ellipsis:true},{title:"状态",dataIndex:"status",width:95},{title:tab==="motion"?"参考对象":"相关组件",dataIndex:"components",ellipsis:true,width:130},...(tab==="motion"?[
     {title:"平移",width:48,render:(_:unknown,row:{id:string})=>{const m=motionIndex.get(row.id);return m?motionDegrees(m)?.translationDof??"—":"—";}},
     {title:"转动",width:48,render:(_:unknown,row:{id:string})=>{const m=motionIndex.get(row.id);return m?motionDegrees(m)?.rotationDof??"—":"—";}},
     ]:[]),
    ...(tab==='issues'?[{title:'原因',render:(_:unknown,row:{id:string})=>{const issue=problems.get(row.id);return issue?engineeringReason(issue):'—';}}]:[]),
    {title:"操作",width:tab==='motion'?70:170,render:(_,row)=><Space size={4}>
     <Button size="small" type="link" onClick={e=>{e.stopPropagation();if(tab==="motion"){const m=motionIndex.get(row.id);if(m){setSelected(`${context}:${row.id}`);p.onLocateInstance(m.bodyId);}}else {const c=index.get(row.id);if(c)p.onLocate(engineeringMember(c),revision);}}}>定位</Button>
     {tab!=='motion'&&index.has(row.id)&&<>
      <Button size="small" type="link" disabled={!p.canEdit} onClick={e=>{e.stopPropagation();p.onRepair(engineeringMember(index.get(row.id)!),"EDIT",revision)}}>编辑</Button>
      {!index.get(row.id)!.suppressed&&assemblyConflictRepairs(engineeringMember(index.get(row.id)!),index.get(row.id)).includes('SUPPRESS')&&<Button size="small" type="link" disabled={!p.canEdit} onClick={e=>{e.stopPropagation();p.onRepair(engineeringMember(index.get(row.id)!),"SUPPRESS",revision)}}>停用</Button>}
     </>}
    </Space>}]}/>
  {selectedMotion&&<div className="assembly-freedom-detail" aria-label="自由度详情">
   <div><Typography.Text strong>{selectedMotion.name}</Typography.Text><span className="assembly-freedom-reference">{degrees?.overall?"连通组件整体运动":`相对 ${selectedMotion.reference}`}</span>
    {!!selectedMotion.gaugeDof&&!degrees?.overall&&<Tag>整体自由度 {selectedMotion.gaugeDof}</Tag>}</div>
   <div className="assembly-freedom-description">{selectedMotion.description}</div>
   {degrees?<>
    <div className="assembly-freedom-directions" role="group" aria-label="运动方向">
     {degrees.directions.length?degrees.directions.map(item=><Button key={item.id} size="small" aria-pressed={selectedMotion.activeDirections?.includes(item.id)??false}
      type={selectedMotion.activeDirections?.includes(item.id)?"primary":"default"} onClick={()=>setDirections(previous=>{const key=`${context}:${selectedMotion.id}`,ids=previous?.key===key?previous.ids:[];return {key,ids:ids.includes(item.id)?ids.filter(id=>id!==item.id):[...ids,item.id]};})}>{item.label}</Button>):<span>无剩余自由度</span>}
    </div>
    {degrees.directions.filter(item=>selectedMotion.activeDirections?.includes(item.id)).map(item=><div key={item.id} className="assembly-freedom-vector" aria-label={`${item.label}详情`}>
     <span>{item.label}</span><span>方向 ({displayDirection(item.direction)})</span>
     {item.axisPoint&&<><span>轴上点</span><span>({item.axisPoint.map(v=>displayLength(v,p.lengthUnit)).join(", ")})</span></>}
     {!!item.pitch&&Math.abs(item.pitch)>1e-12&&<><span>螺距／弧度</span><span>{displayLength(item.pitch,p.lengthUnit)}</span></>}
    </div>)}
   </>:<Typography.Text type="secondary">{selectedMotion.description}</Typography.Text>}
  </div>}
  </div>
  {p.report&&<Typography.Paragraph style={{margin:"8px 0"}}>{!p.current?"结果已失效":p.report.status==="SAT"?"所选范围：约束可满足":p.report.status==="UNSAT"?"所选范围：存在不兼容要求":"检查未完成"}</Typography.Paragraph>}
  {p.error&&<Alert type="error" message="检查未完成，请重试"/>}

 </CommandDialog>;
}
