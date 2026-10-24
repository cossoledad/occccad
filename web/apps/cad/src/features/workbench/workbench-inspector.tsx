import {sketchProfileFeedback,profileIssueText} from "../../cad/sketch/sketch-profile-analysis";
import {inspectorOwnerDocumentId,inspectorTopologyData} from './inspector-object-data';
import {referencePropertyRows} from './reference-properties';
import {formatDisplayNumber as number} from '../../utils/display-number';
import {Button,Descriptions,List,Space,Spin,Tag,Typography} from 'antd';
import type {DocumentProperties,DocumentView,Feature,HistoryEntry,Selection,SketchPlane,TopologyElementProperties} from '../../types';
import {parameterDisplayValue,parameterSourceText} from './parameter-editor';
import {engineeringLabel,featureName,historyAction,objectName,selectionName} from '../../cad/presentation/engineering-text';
import {engineeringInstanceName,engineeringConstraintName} from '../../cad/assembly/assembly-engineering-state';
import {describeAssemblyReference} from '../../cad/assembly/assembly-reference-presentation';
import {engineeringProblem} from '../../cad/command/operation-feedback';

type PropertiesProps={view:DocumentView;ownerView?:DocumentView;selection:Selection;feature?:Feature;workbench:string;sketchPlane?:SketchPlane;activeTool:string;navigationProfile:string;diagnostics?:DocumentProperties;topology?:TopologyElementProperties;topologyLoading?:boolean;readErrors?:{owner?:string;diagnostics?:string;topology?:string};onEditParameter?:(id:string)=>void;onEditPublication?:(id:string,kind:'PART'|'PRODUCT')=>void};
export function Properties(p:PropertiesProps){
 const target=inspectorOwnerDocumentId(p.view,p.selection),candidate=target===p.view.document.id?p.view:p.ownerView;
 const definition=candidate?.document.id===target&&(!p.selection?.versionId||candidate.document.versionId===p.selection.versionId)?candidate:undefined;
 const view=definition??p.view,s=p.selection;
 const rows:Array<{key:string;label:string;children:React.ReactNode}>=[];
 const add=(key:string,label:string,value:React.ReactNode)=>rows.push({key,label,children:value??'—'});
 if(!s){add('name','文档',view.document.name);add('type','类型',engineeringLabel(view.document.type));add('count',view.part?'特征':'零件实例',view.part?.features.length??view.product?.instances.length??0);return <Descriptions column={1} size="small" bordered items={rows}/>}
 if(!definition&&target!==p.view.document.id&&!['body','instance','face','edge','vertex'].includes(s.kind))return <Typography.Text type="secondary">正在读取所选对象的属性…</Typography.Text>;
 const feature=definition?.part?.features.find(f=>f.id===(s.entityId??s.id))??p.feature;
 const parameter=view.part?.parameters?.find(v=>v.parameterId===(s.entityId??s.id));
 if(s.kind==='parameter'){
  if(!parameter)return <Typography.Text type="secondary">该参数已不可用，请重新选择。</Typography.Text>;
  add('name','参数',parameter.qualifiedDisplayPath??parameter.displayName??parameter.label??'参数');add('source','表达式 / 输入',parameterSourceText(parameter));add('value','当前值',parameterDisplayValue(parameter));add('owner','所属对象',parameter.ownerFeatureId?objectName(view,parameter.ownerFeatureId):'文档参数');add('state','状态','可用');
  if(p.onEditParameter)add('edit','操作',<Button size="small" onClick={()=>p.onEditParameter?.(parameter.parameterId)}>编辑参数</Button>);
 }else if(s.publication){add('type','类型','发布引用');add('name','名称',s.publication.name);add('geometry','几何类型',engineeringLabel(s.publication.type,'几何对象'));add('state','关联状态',engineeringLabel(s.publication.resolution?.status,'待检查'));if(p.onEditPublication)add('edit','操作',<Button size="small" onClick={()=>p.onEditPublication?.(s.publication!.id,view.document.type==='PRODUCT'?'PRODUCT':'PART')}>编辑发布引用</Button>);
 }else if(['face','edge','vertex'].includes(s.kind)){
  const topology=inspectorTopologyData(s,p.topology);
  if(p.readErrors?.topology)return <Typography.Text type="secondary">无法读取该几何的属性，请重新选择或重试。</Typography.Text>;
  if(p.topologyLoading||!topology)return <Spin size="small" tip="正在读取几何属性…"/>;
  add('type','类型',engineeringLabel(topology.kind,'几何元素'));add('geometry','几何形状',engineeringLabel(topology.geometryType,'曲面或曲线'));add('owner','所属对象',objectName(view,s.bodyId,'实体'));if(s.instancePath?.display)add('path','所属位置',s.instancePath.display);if(topology.persistentSelection)add('feature','生成步骤',objectName(view,topology.persistentSelection.anchor.featureId,'生成步骤暂不可用'));
  if(topology.point)add('point','位置 (mm)',topology.point.map(v=>number(v)).join(', '));
  const props:Record<string,string>={area:'面积 (mm²)',length:'长度 (mm)',radius:'半径 (mm)',diameter:'直径 (mm)',volume:'体积 (mm³)',center:'中心 (mm)',centroid:'形心 (mm)',normal:'法向',direction:'方向',axis:'轴线方向',axisOrigin:'轴线上一点 (mm)',axisPoint:'轴线上一点 (mm)',origin:'原点 (mm)',closed:'闭合',periodic:'周期曲线',majorRadius:'主半径 (mm)',minorRadius:'次半径 (mm)',referenceRadius:'参考半径 (mm)',semiAngle:'半锥角 (°)',apex:'顶点 (mm)',axisDirection:'轴线方向',xDirection:'X 方向',yDirection:'Y 方向'};
  for(const [key,value] of Object.entries(topology.properties??{}))if(props[key])add(key,props[key],Array.isArray(value)?value.map(v=>typeof v==='number'?number(v):'—').join(', '):typeof value==='number'?number(key==='semiAngle'?value*180/Math.PI:value):typeof value==='boolean'?value?'是':'否':'—');
 }else if(s.kind==='body'){
  const body=view.part?.bodies.find(b=>b.id===s.bodyId),resolved=p.view.resolvedInstances?.find(i=>i.bodyId===s.bodyId&&i.documentId===s.documentId&&i.occurrencePath===(s.occurrencePath??''));
  add('name','实体',body?.name??resolved?.name??'引用实体暂不可用');add('path','所属位置',s.instancePath?.display??view.document.name);add('state','状态',body?.displayFallback?'计算失败，显示上次成功结果':body?.consumed?'已用于布尔运算':'参与输出');add('visible','显示',body?.visible===false?'隐藏':'可见');
 }else if(s.entityKind==='ASSEMBLY_GROUP_MEMBER_REFERENCE'){
  add('type','类型','固联组成员');add('owner','所属固联组',objectName(p.view,s.ownerEntityId,'固联组已丢失'));add('member','成员',objectName(p.view,s.entityId,'引用成员已丢失'));
 }else if(s.kind==='kinematic-object'){
  const k=p.view.product?.kinematics,m=k?.mechanisms.find(m=>m.id===s.mechanismId||m.id===s.id),joint=m?.joints.find(j=>j.id===s.id||j.constraints?.some(c=>c.id===s.id)),relation=joint?.constraints?.find(c=>c.id===s.id),driver=k?.drivers?.find(d=>d.id===s.id),study=k?.studies.find(v=>v.id===s.id);
  add('type','类型',relation?'机构约束':joint?'接合':driver?'驱动命令':study?'模拟':'机构');add('name','名称',(relation??joint??driver??study??m)?.name??'引用定义已丢失');if(m&&m.id!==s.id)add('owner','所属机构',m.name);if(joint)add('kind','接合类型',engineeringLabel(joint.kind,'接合'));if(relation)add('state','当前状态',engineeringLabel(relation.evaluationStatus));
 }else if(s.kind==='assembly-constraint'){
  const c=view.product?.constraints?.find(c=>c.id===s.constraintId);
  if(!c)return <Typography.Text type="secondary">该约束已不可用，请重新选择。</Typography.Text>;
  add('name','名称',engineeringConstraintName(c,view));add('kind','关系',engineeringLabel(c.kind,'约束'));add('state','状态',c.suppressed?'已停用':engineeringLabel(c.evaluationStatus));add('mode','模式',engineeringLabel(c.mode??'DRIVING'));
  if(c.kind==='ANGLE'||c.kind==='DISTANCE')add('value',c.kind==='ANGLE'?'角度':'偏移',c.kind==='ANGLE'?`${number((c.value??0)*180/Math.PI)}°`:`${number(c.value??0)} mm`);
  for(const [index,ref] of [c.first,c.second].entries())if(ref){const label=describeAssemblyReference(ref,view);add(`support${index}`,`支撑 ${index+1}`,`${label.primary} / ${label.secondary}`)}
  if(c.evaluationFailure)add('issue','问题',engineeringProblem(c.evaluationFailure));
 }else if(s.kind==='instance'){
  const instance=p.view.product?.instances.find(i=>i.id===(s.instanceId??s.id));add('name','零件实例',engineeringInstanceName(p.view,s.instanceId??s.id));if(instance){add('version','关联方式',engineeringLabel(instance.referenceMode??'FOLLOW_HEAD'));add('translation','位置 (mm)',instance.translation.map(v=>number(v)).join(', '));add('update','更新状态',instance.headChanged?'有可用更新':'使用已接受版本')}
 }else if(s.kind==='sketch-constraint'){
  const c=view.part?.features.find(f=>f.id===s.featureId)?.sketch?.constraints.find(c=>c.id===s.constraintId);add('type','约束',engineeringLabel(c?.kind??s.constraintType,'草图约束'));add('sketch','所属草图',featureName(view,s.featureId,'草图'));if(c&&'value'in c&&typeof c.value==='number')add('value','数值',`${number(c.value)} ${c.unit==='deg'?'°':c.unit??'mm'}`);
 }else{
  const refs=referencePropertyRows(view,s,p.diagnostics);if(refs)return <Descriptions column={1} size="small" bordered items={refs}/>;
  add('name','名称',feature?.name??selectionName(view,s));add('type','类型',engineeringLabel(feature?.type??s.kind.toUpperCase(),'几何对象'));
  if(feature?.evaluationStatus)add('state','状态',engineeringLabel(feature.evaluationStatus));
  if(feature?.sketch){const sketch=feature.sketch;const profile=sketchProfileFeedback(view.sketchAnalyses?.[feature.id]);add('profile','轮廓',<Space orientation="vertical" size={2}><span>{profile.summary}</span>{profile.issues.map((issue,i)=><Typography.Text key={i} type="warning">{profileIssueText(issue.code)}{issue.position?`（${number(issue.position.x)}, ${number(issue.position.y)} mm）`:''}</Typography.Text>)}</Space>);add('entities','草图元素',sketch.entities.length);add('constraints','约束',sketch.constraints.length);add('solve','求解',`${engineeringLabel(sketch.solve.status,'待求解')} · ${sketch.solve.degreesOfFreedom} 个自由度`);add('support','草图支撑',engineeringLabel(sketch.support.status,'可用'));}
  if(feature&&!feature.sketch){
   if(feature.bodyId)add('body','所属实体',objectName(view,feature.bodyId,'引用实体已丢失'));
   if(feature.operation)add('operation','材料处理',engineeringLabel(feature.operation));
   if(feature.extent)add('extent','终止方式',engineeringLabel(feature.extent));
   if(feature.profile)add('profile','输入草图',featureName(view,feature.profile,'引用草图已丢失'));
   if(feature.profileSelection)add('regions','轮廓输入',feature.profileSelection.mode==='REGIONS'?`明确选择的 ${feature.profileSelection.regionIds?.length??0} 个区域`:'整张草图');
   const lengthLabel=feature.type==='FILLET'?'圆角半径':feature.type==='CHAMFER'?'倒角距离':feature.type==='SHELL'?'壁厚':'长度';
   const owned=view.part?.parameters?.filter(v=>v.ownerFeatureId===feature.id)??[];
   if(owned.length)for(const parameter of owned)add(parameter.parameterId,parameter.propertySlot==='length'?lengthLabel:parameter.displayName??parameter.label??'参数',<><span>{parameterDisplayValue(parameter)}</span>{parameter.source.expression&&<small> · {parameter.source.expression.sourceText}</small>}</>);
   else for(const [slot,label,unit] of [['length',lengthLabel,'mm'],['length2','第二方向长度','mm'],['diameter','直径','mm'],['depth','深度','mm'],['angle','角度','°']] as const){const value=feature[slot];if(typeof value==='number')add(slot,label,`${number(value)} ${unit}`)}
   if(feature.diagnostic)add('issue','问题',engineeringProblem(feature.diagnostic));
  }
 }
 return <Descriptions column={1} size="small" bordered className="property-list" items={rows}/>;
}
export function History({entries,onRestore,canRestore=true}:{entries:HistoryEntry[];onRestore:(entry:HistoryEntry)=>void;canRestore?:boolean}){
 return <List className="history-list" dataSource={[...entries].reverse()} locale={{emptyText:'尚无编辑历史'}} renderItem={entry=><List.Item actions={!entry.isHead&&canRestore?[<Button key="restore" type="link" onClick={()=>onRestore(entry)}>恢复</Button>]:[]}><List.Item.Meta title={<Space><span>{entry.versionName||historyAction(entry.commandType)}</span>{entry.isHead&&<Tag color="blue">当前版本</Tag>}</Space>} description={new Date(entry.createdAt).toLocaleString()}/></List.Item>}/>;
}
export function PartBodies({view}:{view:DocumentView}){return view.part?<section aria-label="实体"><strong>实体</strong>{view.part.bodies.map(body=><div key={body.id}><Space wrap><strong>{body.name||'实体'}</strong>{body.id===view.part!.activeBodyId&&<Tag>活动实体</Tag>}<Tag>{body.visible?'可见':'隐藏'}</Tag>{body.consumed&&<Tag>已用于布尔</Tag>}{body.displayFallback&&<Tag color="error">计算失败，显示上次成功结果</Tag>}</Space>{body.displayFallback&&<p>修复失败特征后可更新实体。</p>}</div>)}</section>:null;}
