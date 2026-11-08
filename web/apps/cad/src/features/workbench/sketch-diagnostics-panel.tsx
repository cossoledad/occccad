import {Button,Input,Select,Space,Table,Tabs,Tag} from 'antd';
import {useState} from 'react';
import type {Feature} from '../../types';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {engineeringLabel} from '../../cad/presentation/engineering-text';
import {useOperationFeedback} from '../../cad/command/operation-feedback';
import {sketchConstraintIssues,sketchDiagnosticComponents} from '../../cad/sketch/sketch-diagnostics';

type DiagnosticRow={key:string;name:string;status:string;kind:string;entities:string[];constraints:string[];constraintId?:string;internal?:boolean;suppressed?:boolean;dof?:number};
const statusLabel=(status:string)=>({NONE:'有效',SUPPRESSED:'停用',CONFLICTING:'冲突',REDUNDANT:'冗余'}[status]??engineeringLabel(status,'待确定'));
export function SketchDiagnosticsPanel({feature,revision,canEdit,onClose,onSelect,onLocate,onEdit,onSuppress}:{
 feature:Feature;revision:string;canEdit:boolean;onClose:()=>void;
 onSelect?:(entityIds:string[],constraintIds:string[],revision:string)=>void;
 onLocate:(entityIds:string[],constraintIds:string[],revision:string)=>void;
 onEdit:(constraintId:string,revision:string)=>void;
 onSuppress:(constraintId:string,suppressed:boolean,revision:string)=>Promise<unknown>;
}){
 const sketch=feature.sketch!,components=sketchDiagnosticComponents(sketch),issues=sketchConstraintIssues(sketch);
 const [tab,setTab]=useState('issues'),[search,setSearch]=useState(''),[filter,setFilter]=useState('all'),[busy,setBusy]=useState(false);
 const [selection,setSelection]=useState<{scope:string;key:string}>(),feedback=useOperationFeedback();
 const scope=JSON.stringify([feature.id,revision]);
 const unknown=sketch.solve.definitionStatus==='UNRESOLVED'||['INVALID','CONFLICTING','FAILED','EXECUTION_FAILURE','UNRESOLVED_EXTERNAL'].includes(sketch.solve.status);
 const rows:DiagnosticRow[]=tab==='freedom'?components.map((component,index)=>({key:component.key,name:`分量 ${index+1}`,status:component.known?component.definitionStatus??component.status:'UNRESOLVED',kind:`${component.entityIds.length} 个元素`,entities:component.entityIds,constraints:component.constraintIds,dof:component.known?component.degreesOfFreedom:undefined})):issues.filter(item=>tab==='all'||item.issue!=='NONE').map(({constraint,issue})=>({key:constraint.id,name:constraint.name??engineeringLabel(constraint.kind),status:issue,kind:engineeringLabel(constraint.kind),entities:[],constraints:[constraint.id],constraintId:constraint.id,internal:constraint.internal||['MIRROR','SAME_SUPPORT'].includes(constraint.kind),suppressed:constraint.suppressed}));
 const visible=rows.filter(row=>(filter==='all'||row.status===filter)&&`${row.name} ${row.kind}`.toLocaleLowerCase().includes(search.toLocaleLowerCase()));
 const selected=selection?.scope===scope?selection.key:undefined;
 const choose=(row:DiagnosticRow)=>{setSelection({scope,key:row.key});onSelect?.(row.entities,row.constraints,revision)};
 const suppress=async(row:DiagnosticRow)=>{if(busy||!row.constraintId)return;setBusy(true);try{await onSuppress(row.constraintId,!row.suppressed,revision)}catch(cause){feedback(cause,'草图诊断')}finally{setBusy(false)}};
 return <CommandDialog id="sketch-diagnostics" open title={`草图诊断 · ${feature.name}`} size="L" onClose={onClose} onConfirm={onClose} footer={false}>
  <Space wrap style={{marginBottom:8}}><Tag>{engineeringLabel(sketch.solve.definitionStatus??sketch.solve.status,'待求解')}</Tag><span>自由度 {unknown?'—':sketch.solve.degreesOfFreedom}</span><span>约束 {issues.length}</span><span>冲突 {issues.filter(i=>i.issue==='CONFLICTING').length}</span><span>冗余 {issues.filter(i=>i.issue==='REDUNDANT').length}</span><span>停用 {issues.filter(i=>i.constraint.suppressed).length}</span></Space>
  <Tabs size="small" activeKey={tab} onChange={value=>{setTab(value);setFilter('all');setSelection(undefined)}} items={[{key:'issues',label:`待处理问题 (${issues.filter(i=>i.issue!=='NONE').length})`},{key:'all',label:'全部约束'},{key:'freedom',label:'自由度'}]}/>
  <Space style={{marginBottom:8}}><Input.Search aria-label="搜索草图诊断" placeholder={tab==='freedom'?'搜索分量':'搜索名称或类型'} allowClear value={search} onChange={e=>setSearch(e.target.value)} style={{width:220}}/><Select aria-label="诊断状态" value={filter} onChange={setFilter} style={{width:130}} options={(tab==='freedom'?['all',...new Set(rows.map(r=>r.status))]:['all','CONFLICTING','REDUNDANT','SUPPRESSED','NONE']).map(value=>({value,label:value==='all'?'全部状态':statusLabel(value)}))}/></Space>
  <Table<DiagnosticRow> size="small" rowKey="key" dataSource={visible} locale={{emptyText:tab==='issues'?'无待处理约束':'无记录'}} scroll={{y:250}} pagination={{defaultPageSize:10,showSizeChanger:true,pageSizeOptions:[10,20,50],showTotal:n=>`${n} 项`}}
   rowSelection={{type:'radio',selectedRowKeys:selected?[selected]:[],onChange:keys=>{const row=visible.find(r=>r.key===keys[0]);if(row)choose(row)}}}
   onRow={row=>({tabIndex:0,'aria-selected':row.key===selected,onClick:()=>choose(row),onKeyDown:e=>{if(e.target===e.currentTarget&&(e.key==='Enter'||e.key===' ')){e.preventDefault();choose(row)}}})}
   columns={[{title:'名称',dataIndex:'name',ellipsis:true},{title:'状态',width:110,render:(_,row)=>statusLabel(row.status)},...(tab==='freedom'?[{title:'元素',dataIndex:'kind',width:110},{title:'自由度',width:70,render:(_:unknown,row:DiagnosticRow)=>row.dof??'—'}]:[{title:'类型',dataIndex:'kind',width:130}]),{title:'操作',width:tab==='freedom'?70:180,render:(_,row)=><Space size={4}>
    <Button size="small" type="link" disabled={!row.entities.length&&!row.constraints.length} onClick={e=>{e.stopPropagation();choose(row);onLocate(row.entities,row.constraints,revision)}}>定位</Button>
    {row.constraintId&&<><Button size="small" type="link" disabled={!canEdit||busy||row.internal} onClick={e=>{e.stopPropagation();onEdit(row.constraintId!,revision)}}>编辑</Button><Button size="small" type="link" disabled={!canEdit||busy||row.internal} onClick={e=>{e.stopPropagation();void suppress(row)}}>{row.suppressed?'启用':'停用'}</Button></>}
   </Space>}]}/>
 </CommandDialog>;
}
