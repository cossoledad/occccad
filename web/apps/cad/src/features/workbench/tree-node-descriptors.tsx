import {PauseCircleOutlined,InfoCircleOutlined,CheckCircleOutlined,CloseCircleOutlined,ExclamationCircleOutlined,SyncOutlined} from "@ant-design/icons";
import {CadIcon} from "../../cad/overlay/cad-icons";
import {builtinCatalog} from "../../cad/command/workbench-catalog";
import type {ReactNode} from "react";
import {ASSEMBLY_CONSTRAINT_STATUS,assemblyStatusFromDiagnostic} from "../../cad/assembly/assembly-constraint-ux";
import type {DocumentStructureNode,Feature} from "../../types";

type Kind=DocumentStructureNode["kind"];
const objectIcons:Partial<Record<Kind,string>>={
 "MEASUREMENT_GROUP": "group",
 "MEASUREMENT": "measurement",
 "SPATIAL_ANALYSIS_GROUP": "group",
 "SPATIAL_ANALYSIS": "measurement",
 "KINEMATICS_GROUP": "group",
 "MECHANISM_REFERENCE": "link",
 "APPLICATIONS": "group",
 "MECHANISM": "mechanism",
 "MECHANISM_JOINT": "joint-revolute",
 "MOTION_DRIVER": "motion-driver",
 "MOTION_STUDY": "motion-study",
 "INTERFERENCE_ANALYSIS": "interference",
 "JOINT_SUPPORT": "reference-point",
 "JOINT_EXPANSION": "reference",
 "JOINT_SOURCE": "link",
 "MOTION_ASSOCIATION": "link",
 "ASSEMBLY_CONSTRAINT": "coincident",
 "ASSEMBLY_GROUP_MEMBER_REFERENCE": "link",
 "ASSEMBLY_CONSTRAINT_SET": "group",
 "PRODUCT": "product",
 "PART": "part",
 "INSTANCE": "instance",
 "ORIGIN": "axis-system",
 "PLANE": "datum-plane",
 "DATUM_POINT": "reference-point",
 "REFERENCE_POINT": "reference-point",
 "GEOMETRICAL_SET": "geometrical-set",
 "AXIS_SYSTEM": "axis-system",
 "AXIS": "datum-axis",
 "DATUM_AXIS": "datum-axis",
 "BODY": "body",
 "SKETCH": "sketch",
 "DATUM_INPUT_REFERENCE": "link",
 "FEATURE_INPUT_REFERENCE": "link",
 "SKETCH_INPUT_REFERENCE": "link",
 "SKETCH_OFFSET_DEFINITION": "curve-offset",
 "SKETCH_OFFSET_ENTITY": "curve-offset",
 "SKETCH_PATTERN_DEFINITION": "pattern",
 "SKETCH_PATTERN_MEMBER": "pattern",
 "SKETCH_PATTERN_ENTITY": "pattern",
 "SKETCH_ENTITY": "line",
 "SKETCH_CONSTRAINT": "coincident",
 "SKETCH_GEOMETRY_SET": "geometrical-set",
 "SKETCH_EXTERNAL_GEOMETRY_SET": "group",
 "SKETCH_EXTERNAL_GEOMETRY": "link",
 "SKETCH_LOGICAL_CONSTRAINT_SET": "group",
 "SKETCH_DIMENSION_SET": "group",
 "REFERENCE_CYCLE": "link",
 "FEATURE": "body",
 "SKETCH_CONSTRAINT_SET": "group",
 "PARAMETER_GROUP": "parameter-set",
 "PARAMETER_SET": "parameter-set",
 "PARAMETER": "parameter",
 "PAD": "pad",
 "REVOLVE": "revolve",
 "IMPORT": "import",
 "PUBLICATION_SET": "group",
 "PUBLICATION": "publication",
 "PRODUCT_PUBLICATION_SET": "group",
 "PRODUCT_PUBLICATION": "publication",
 "CONTEXT_REFERENCE_SET": "group",
 "CONTEXT_REFERENCE": "link",
 "CONTEXT_INPUT_SET": "group",
 "CONTEXT_INPUT": "link",
 "CONTEXT_BINDING_SET": "group",
 "CONTEXT_BINDING": "link"
};
const visibleKinds=new Set<string>(["MEASUREMENT_GROUP", "MEASUREMENT", "ASSEMBLY_CONSTRAINT", "ASSEMBLY_CONSTRAINT_SET", "INSTANCE", "ORIGIN", "PLANE", "DATUM_POINT", "REFERENCE_POINT", "GEOMETRICAL_SET", "AXIS_SYSTEM", "AXIS", "DATUM_AXIS", "BODY", "SKETCH", "SKETCH_ENTITY", "SKETCH_EXTERNAL_GEOMETRY"]);
const commandIcons=new Map(builtinCatalog.commands.map(command=>[command.id,command.iconKey]));
const featureCommands:Record<string,string>={PAD:'part.pad',LINEAR_EXTRUDE:'part.pad',REVOLVE:'part.revolve',FILLET:'part.fillet',CHAMFER:'part.chamfer',DRAFT:'part.draft',SHELL:'part.shell',SWEEP:'part.sweep',LOFT:'part.loft',HOLE:'part.hole',BOOLEAN:'part.boolean',MOVE_PLANAR_FACE:'geometry.move-planar-face',RESIZE_BORE:'geometry.resize-bore',THICKEN:'geometry.thicken',CLOSE_SURFACE:'geometry.close-surface'};
const geometryKinds=new Set('LINE CIRCLE ARC SPLINE POINT_ON_CURVE EXTRACT BOUNDARY COMPOSITE SURFACE_EXTRUDE SURFACE_REVOLVE TRANSLATE ROTATE MIRROR SPLIT TRIM EXTEND G1_CONNECT OFFSET PROJECT INTERSECTION SURFACE_SWEEP SURFACE_LOFT SURFACE_BOUNDARY SURFACE_BLEND SURFACE_FILL SURFACE_TRIM SURFACE_MUTUAL_TRIM SURFACE_EXTEND SURFACE_JOIN SURFACE_SEW SURFACE_OFFSET'.split(' '));
const geometryKind=(type:string)=>geometryKinds.has(type)?'geometry.'+type.toLowerCase().replaceAll('_','-'):undefined;
const glyphs:Record<string,string>={GROUND:'ground',REVOLUTE:'joint-revolute',PRISMATIC:'joint-prismatic',CYLINDRICAL:'joint-cylindrical',PLANAR:'joint-planar',RIGID:'joint-rigid',SPHERICAL:'joint-spherical',ELLIPSE:'ellipse',ELLIPTICAL_ARC:'elliptical-arc',CONCENTRIC:'concentric',COLLINEAR:'parallel',MIDPOINT:'midpoint',POINT_ON_OBJECT:'point-on-object',MAJOR_RADIUS:'radius',MINOR_RADIUS:'radius',POINT:'point',SKETCH:'sketch',SOLID_PATTERN:'pattern',SKETCH_PATTERN:'pattern',COINCIDENT:'coincident',PARALLEL:'parallel',PERPENDICULAR:'perpendicular',TANGENT:'tangent',FIXED:'fixed',EQUAL:'equal',HORIZONTAL:'horizontal',VERTICAL:'vertical',ANGLE:'angle',RADIUS:'radius',DIAMETER:'diameter',DISTANCE:'distance',LENGTH:'length',SYMMETRY:'symmetry',IMPORT_BODY:'import',IMPORTED:'import'};

export function treeNodeIcon(node:Pick<DocumentStructureNode,'kind'|'health'|'diagnostic'|'evaluationStatus'|'operation'|'entityType'|'suppressed'|'connectionStatus'|'currencyStatus'>,hint?:string,feature?:Pick<Feature,'geometry'>):ReactNode {
 const type=feature?.geometry?.kind??node.entityType??node.kind;
 const command=featureCommands[type]??geometryKind(type);
 const glyph=(command?commandIcons.get(command):undefined)??glyphs[type]??objectIcons[node.kind]??'reference';
 const assembly=node.kind==='ASSEMBLY_CONSTRAINT'||node.kind==='JOINT_EXPANSION';
 const assemblyGlyph=({FIX:'fixed',FIX_TOGETHER:'link',DISTANCE:'distance',ANGLE:'angle',COINCIDENT:'coincident',CONCENTRIC:'concentric',PARALLEL:'parallel',PERPENDICULAR:'perpendicular',CONTACT:'tangent'} as Record<string,string>)[type]??glyph;
 const name=node.kind==='SPATIAL_ANALYSIS'?(type==='SECTION'?'section':type==='STATIC_INTERFERENCE'?'interference':'measurement'):assembly?assemblyGlyph:['PAD','LINEAR_EXTRUDE'].includes(type)&&node.operation==='REMOVE'?'pocket':glyph;
 const typeIcon=<CadIcon name={name}/>;
 let label='',color='',badge:ReactNode;
 if(node.health && node.health.severity !== 'NORMAL'){
  const error = node.health.severity === 'ERROR';
  label = [error ? '错误' : '警告', node.health.message].filter(Boolean).join(' · ');
  color = error ? 'var(--color-danger)' : 'var(--color-warning)';
  badge = error ? <CloseCircleOutlined/> : <ExclamationCircleOutlined/>;
 }
 else if(node.kind==='SPATIAL_ANALYSIS'||node.kind==='MEASUREMENT'){const status=node.currencyStatus??node.evaluationStatus;label=status==='CURRENT'?'已更新':status==='BROKEN'?'来源不可用':status==='UNKNOWN'?'状态暂不可用':status==='MISSING'?'尚无结果':'待更新';color=status==='CURRENT'?'var(--color-success)':status==='BROKEN'?'var(--color-danger)':'var(--color-warning)';badge=status==='CURRENT'?<CheckCircleOutlined/>:status==='BROKEN'?<CloseCircleOutlined/>:<SyncOutlined/>}
 else if(node.suppressed){label='已抑制';color='var(--color-text-secondary)';badge=<PauseCircleOutlined/>}
 else if(node.evaluationStatus==='FAILED'||node.evaluationStatus==='BLOCKED'){label=node.evaluationStatus==='FAILED'?'计算失败':'上游失败';color='var(--color-danger)';badge=<ExclamationCircleOutlined/>}
 else if(assembly){const status=assemblyStatusFromDiagnostic(node.evaluationStatus??node.diagnostic),detail=ASSEMBLY_CONSTRAINT_STATUS[status];label=detail.label;color=detail.color;badge=status==='VERIFIED'?<CheckCircleOutlined/>:status==='BROKEN'?<CloseCircleOutlined/>:status==='IMPOSSIBLE'?<ExclamationCircleOutlined/>:<SyncOutlined/>}
 else if(['CONFLICTING','OVER_CONSTRAINED','INCONSISTENT'].includes(node.diagnostic??'')){label='约束冲突';color='var(--color-danger)';badge=<ExclamationCircleOutlined/>}
 else if(node.diagnostic==='REDUNDANT'){label='约束冗余';color='var(--color-warning)';badge=<InfoCircleOutlined/>}
 else if(node.diagnostic==='FULLY_CONSTRAINED'){label='完全约束';color='var(--color-success)';badge=<CheckCircleOutlined/>}
 else if(node.diagnostic==='UNDER_CONSTRAINED'){label='欠约束';color='var(--color-text-secondary)';badge=<InfoCircleOutlined/>}
 else if(node.connectionStatus==='BROKEN'||node.connectionStatus==='BROKEN_PUBLICATION'||node.connectionStatus==='NOT_CONNECTED'){label='引用已失效';color='var(--color-danger)';badge=<CloseCircleOutlined/>}
 else if(node.currencyStatus==='OUTDATED'||hint){label=hint??'需要更新';color='var(--color-text-secondary)';badge=<InfoCircleOutlined/>}
 return <span className="tree-object-icon"><span className="tree-type-icon">{typeIcon}</span>{badge&&<span className="tree-status-badge" aria-label={`状态 ${label}`} title={label} style={{color}}>{badge}</span>}</span>;
}

export function canToggleNodeVisibility(kind: string | undefined): boolean {
  return Boolean(kind && visibleKinds.has(kind));
}
