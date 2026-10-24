import {
  PauseCircleOutlined,InfoCircleOutlined,AimOutlined, ApartmentOutlined, BuildOutlined, CheckCircleOutlined, CloseCircleOutlined,
  CloudUploadOutlined, DatabaseOutlined, ExclamationCircleOutlined, GatewayOutlined,
  InsertRowAboveOutlined, NodeIndexOutlined, SyncOutlined,
} from "@ant-design/icons";
import {CadIcon} from "../../cad/overlay/cad-icons";
import type { ReactNode } from "react";
import { ASSEMBLY_CONSTRAINT_STATUS, assemblyStatusFromDiagnostic } from "../../cad/assembly/assembly-constraint-ux";
import type { DocumentStructureNode } from "../../types";

type Kind = DocumentStructureNode["kind"];
type Descriptor = { icon: ReactNode; visibility?: true };

const descriptors: Partial<Record<Kind, Descriptor>> = {
 KINEMATICS_GROUP:{icon:<ApartmentOutlined/>},MECHANISM_REFERENCE:{icon:<GatewayOutlined/>},APPLICATIONS:{icon:<ApartmentOutlined/>},MECHANISM:{icon:<GatewayOutlined/>},MECHANISM_JOINT:{icon:<NodeIndexOutlined/>},MOTION_DRIVER:{icon:<AimOutlined/>},MOTION_STUDY:{icon:<SyncOutlined/>},INTERFERENCE_ANALYSIS:{icon:<ExclamationCircleOutlined/>},JOINT_SUPPORT:{icon:<AimOutlined/>},JOINT_EXPANSION:{icon:<NodeIndexOutlined/>},JOINT_SOURCE:{icon:<GatewayOutlined/>},MOTION_ASSOCIATION:{icon:<GatewayOutlined/>},
  ASSEMBLY_CONSTRAINT:{icon:<GatewayOutlined />,visibility:true},
  ASSEMBLY_GROUP_MEMBER_REFERENCE:{icon:<GatewayOutlined />},
  ASSEMBLY_CONSTRAINT_SET:{icon:<GatewayOutlined />,visibility:true},
  PRODUCT: { icon: <ApartmentOutlined /> }, PART: { icon: <BuildOutlined /> },
  INSTANCE: { icon: <BuildOutlined />, visibility: true },
  ORIGIN: { icon: <GatewayOutlined />, visibility:true },
  PLANE: { icon: <CadIcon name="datum-plane"/>, visibility: true },
  DATUM_POINT: {icon:<AimOutlined />,visibility:true},
  AXIS_SYSTEM: { icon: <AimOutlined />, visibility: true },
  AXIS: { icon: <CadIcon name="datum-axis"/>, visibility: true },
  DATUM_AXIS: { icon: <CadIcon name="datum-axis"/>, visibility: true },
  BODY: { icon: <DatabaseOutlined />, visibility: true },
  SKETCH: { icon: <CadIcon name="sketch"/>, visibility: true },
  DATUM_INPUT_REFERENCE: { icon: <GatewayOutlined /> },
  FEATURE_INPUT_REFERENCE: { icon: <GatewayOutlined /> },
  SKETCH_INPUT_REFERENCE: { icon: <GatewayOutlined /> },
  SKETCH_PATTERN_DEFINITION: {icon:<NodeIndexOutlined />},
  SKETCH_PATTERN_MEMBER: {icon:<NodeIndexOutlined />},
  SKETCH_PATTERN_ENTITY: {icon:<NodeIndexOutlined />},
  SKETCH_ENTITY: { icon: <NodeIndexOutlined />, visibility: true },
  SKETCH_CONSTRAINT: { icon: <GatewayOutlined /> },
  SKETCH_GEOMETRY_SET: { icon: <DatabaseOutlined /> },
  SKETCH_EXTERNAL_GEOMETRY_SET:{icon:<DatabaseOutlined/>},SKETCH_EXTERNAL_GEOMETRY:{icon:<GatewayOutlined/>,visibility:true},SKETCH_LOGICAL_CONSTRAINT_SET:{icon:<DatabaseOutlined/>},SKETCH_DIMENSION_SET:{icon:<DatabaseOutlined/>},REFERENCE_CYCLE:{icon:<GatewayOutlined/>},FEATURE:{icon:<BuildOutlined/>},
  SKETCH_CONSTRAINT_SET: { icon: <DatabaseOutlined /> },
  PARAMETER_GROUP: {icon:<DatabaseOutlined />},
  PARAMETER_SET: { icon: <DatabaseOutlined /> },
  PARAMETER: { icon: <NodeIndexOutlined /> },
  PAD: { icon: <InsertRowAboveOutlined /> },
  REVOLVE: { icon: <InsertRowAboveOutlined /> },
  IMPORT: { icon: <CloudUploadOutlined /> },
  PUBLICATION_SET: { icon: <GatewayOutlined /> }, PUBLICATION: { icon: <GatewayOutlined /> },
  PRODUCT_PUBLICATION_SET: { icon: <GatewayOutlined /> }, PRODUCT_PUBLICATION: { icon: <GatewayOutlined /> },
  CONTEXT_REFERENCE_SET: { icon: <GatewayOutlined /> }, CONTEXT_REFERENCE: { icon: <GatewayOutlined /> },
  CONTEXT_INPUT_SET: { icon: <GatewayOutlined /> }, CONTEXT_INPUT: { icon: <GatewayOutlined /> },
  CONTEXT_BINDING_SET: { icon: <GatewayOutlined /> }, CONTEXT_BINDING: { icon: <GatewayOutlined /> },
};

export function treeNodeIcon(node:Pick<DocumentStructureNode,'kind'|'diagnostic'|'evaluationStatus'|'operation'|'entityType'|'suppressed'|'connectionStatus'|'currencyStatus'>,hint?:string):ReactNode {
 const assembly=node.kind==='ASSEMBLY_CONSTRAINT'||node.kind==='JOINT_EXPANSION';
 const glyph=({GROUND:'fixed',REVOLUTE:'rotate',PRISMATIC:'move',CYLINDRICAL:'rotate',PLANAR:'datum-plane',RIGID:'link',SPHERICAL:'rotate',ELLIPSE:'circle',ELLIPTICAL_ARC:'arc',CONCENTRIC:'concentric',COLLINEAR:'parallel',MIDPOINT:'midpoint',POINT_ON_OBJECT:'point-on-object',MAJOR_RADIUS:'radius',MINOR_RADIUS:'radius',POINT:'point',LINE:'line',CIRCLE:'circle',ARC:'arc',SPLINE:'spline',SKETCH:'sketch',PAD:'pad',LINEAR_EXTRUDE:'pad',REVOLVE:'revolve',FILLET:'radius',CHAMFER:'angle',DRAFT:'angle',SHELL:'pocket',LOFT:'pad',BOOLEAN:'link',SOLID_PATTERN:'pattern',SKETCH_PATTERN:'pattern',COINCIDENT:'coincident',PARALLEL:'parallel',PERPENDICULAR:'perpendicular',TANGENT:'tangent',FIXED:'fixed',EQUAL:'equal',HORIZONTAL:'horizontal',VERTICAL:'vertical',ANGLE:'angle',RADIUS:'radius',DIAMETER:'diameter',DISTANCE:'distance',LENGTH:'length',SYMMETRY:'symmetry'} as Record<string,string>)[node.entityType??node.kind];
 const assemblyGlyph=({FIX:'fixed',FIX_TOGETHER:'link',DISTANCE:'distance',ANGLE:'angle',COINCIDENT:'coincident',CONCENTRIC:'concentric',PARALLEL:'parallel',PERPENDICULAR:'perpendicular',CONTACT:'coincident'} as Record<string,string>)[node.entityType??'']??'coincident';
 const typeIcon=assembly?<CadIcon name={assemblyGlyph}/>:['PAD','LINEAR_EXTRUDE'].includes(node.entityType??node.kind)&&node.operation==='REMOVE'?<CadIcon name="pocket"/>:glyph?<CadIcon name={glyph}/>:node.entityType==='HOLE'?<AimOutlined/>:descriptors[node.kind]?.icon??<NodeIndexOutlined/>;
 let label='',color='',badge:ReactNode;
 if(node.suppressed){label='已抑制';color='var(--color-text-secondary)';badge=<PauseCircleOutlined/>}
 else if(node.evaluationStatus==='FAILED'||node.evaluationStatus==='BLOCKED'){label=node.evaluationStatus==='FAILED'?'计算失败':'上游失败';color='var(--color-danger)';badge=<ExclamationCircleOutlined/>}
 else if(assembly){const status=assemblyStatusFromDiagnostic(node.evaluationStatus??node.diagnostic),detail=ASSEMBLY_CONSTRAINT_STATUS[status];label=detail.label;color=detail.color;badge=status==='VERIFIED'?<CheckCircleOutlined/>:status==='BROKEN'?<CloseCircleOutlined/>:status==='IMPOSSIBLE'?<ExclamationCircleOutlined/>:<SyncOutlined/>}
 else if(['CONFLICTING','OVER_CONSTRAINED','INCONSISTENT'].includes(node.diagnostic??'')){label='约束冲突';color='var(--color-danger)';badge=<ExclamationCircleOutlined/>}
 else if(node.diagnostic==='REDUNDANT'){label='约束冗余';color='var(--color-warning)';badge=<InfoCircleOutlined/>}
 else if(node.diagnostic==='FULLY_CONSTRAINED'){label='完全约束';color='var(--color-success)';badge=<CheckCircleOutlined/>}
 else if(node.diagnostic==='UNDER_CONSTRAINED'){label='欠约束';color='var(--color-text-secondary)';badge=<InfoCircleOutlined/>}
 else if(node.connectionStatus==='BROKEN'||node.connectionStatus==='NOT_CONNECTED'){label='引用已失效';color='var(--color-danger)';badge=<CloseCircleOutlined/>}
 else if(node.currencyStatus==='OUTDATED'||hint){label=hint??'需要更新';color='var(--color-text-secondary)';badge=<InfoCircleOutlined/>}
 return <span className="tree-object-icon"><span className="tree-type-icon">{typeIcon}</span>{badge&&<span className="tree-status-badge" aria-label={`状态 ${label}`} title={label} style={{color}}>{badge}</span>}</span>;
}

export function canToggleNodeVisibility(kind: string | undefined): boolean {
  return Boolean(kind && descriptors[kind as Kind]?.visibility);
}
