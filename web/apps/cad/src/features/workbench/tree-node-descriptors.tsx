import {
  AimOutlined, ApartmentOutlined, BuildOutlined, CheckCircleOutlined, CloseCircleOutlined,
  CloudUploadOutlined, DatabaseOutlined, ExclamationCircleOutlined, GatewayOutlined,
  InsertRowAboveOutlined, NodeIndexOutlined, ScissorOutlined, SyncOutlined,
} from "@ant-design/icons";
import type { ReactNode } from "react";
import { ASSEMBLY_CONSTRAINT_STATUS, assemblyStatusFromDiagnostic } from "../../cad/assembly/assembly-constraint-ux";
import type { DocumentStructureNode } from "../../types";

type Kind = DocumentStructureNode["kind"];
type Descriptor = { icon: ReactNode; visibility?: true };

const descriptors: Partial<Record<Kind, Descriptor>> = {
 MOTION_RUN:{icon:<SyncOutlined/>},APPLICATIONS:{icon:<ApartmentOutlined/>},MECHANISM:{icon:<GatewayOutlined/>},MECHANISM_JOINT:{icon:<NodeIndexOutlined/>},MOTION_DRIVER:{icon:<AimOutlined/>},MOTION_STUDY:{icon:<SyncOutlined/>},INTERFERENCE_ANALYSIS:{icon:<ExclamationCircleOutlined/>},JOINT_SUPPORT:{icon:<AimOutlined/>},JOINT_EXPANSION:{icon:<NodeIndexOutlined/>},JOINT_SOURCE:{icon:<GatewayOutlined/>},MOTION_ASSOCIATION:{icon:<GatewayOutlined/>},
  ASSEMBLY_CONSTRAINT:{icon:<GatewayOutlined />,visibility:true},
  ASSEMBLY_CONSTRAINT_SET:{icon:<GatewayOutlined />,visibility:true},
  PRODUCT: { icon: <ApartmentOutlined /> }, PART: { icon: <BuildOutlined /> },
  INSTANCE: { icon: <BuildOutlined />, visibility: true },
  ORIGIN: { icon: <GatewayOutlined />, visibility:true },
  PLANE: { icon: <NodeIndexOutlined />, visibility: true },
  DATUM_POINT: {icon:<AimOutlined />,visibility:true},
  AXIS_SYSTEM: { icon: <AimOutlined />, visibility: true },
  AXIS: { icon: <NodeIndexOutlined />, visibility: true },
  DATUM_AXIS: { icon: <NodeIndexOutlined />, visibility: true },
  BODY: { icon: <DatabaseOutlined />, visibility: true },
  SKETCH: { icon: <ScissorOutlined />, visibility: true },
  DATUM_INPUT_REFERENCE: { icon: <GatewayOutlined /> },
  FEATURE_INPUT_REFERENCE: { icon: <GatewayOutlined /> },
  SKETCH_INPUT_REFERENCE: { icon: <GatewayOutlined /> },
  SKETCH_PATTERN_DEFINITION: {icon:<NodeIndexOutlined />},
  SKETCH_PATTERN_MEMBER: {icon:<NodeIndexOutlined />},
  SKETCH_PATTERN_ENTITY: {icon:<NodeIndexOutlined />},
  SKETCH_ENTITY: { icon: <NodeIndexOutlined />, visibility: true },
  SKETCH_CONSTRAINT: { icon: <GatewayOutlined /> },
  SKETCH_GEOMETRY_SET: { icon: <DatabaseOutlined /> },
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

export function treeNodeIcon(node: Pick<DocumentStructureNode, "kind" | "diagnostic" | "evaluationStatus" | "operation">): ReactNode {
  if (node.kind === "ASSEMBLY_CONSTRAINT") {
    const status = assemblyStatusFromDiagnostic(node.evaluationStatus ?? node.diagnostic);
    const detail = ASSEMBLY_CONSTRAINT_STATUS[status];
    const icon = status === "VERIFIED" ? <CheckCircleOutlined /> : status === "BROKEN" ? <CloseCircleOutlined />
      : status === "IMPOSSIBLE" ? <ExclamationCircleOutlined /> : <SyncOutlined />;
    return <span aria-label={`Constraint status ${detail.label}`} title={`${detail.label}: ${detail.description}`}
      style={{ color: detail.color }}>{icon}</span>;
  }
  if (node.evaluationStatus === "FAILED" || node.evaluationStatus === "BLOCKED") return <span aria-label={node.evaluationStatus === "FAILED" ? "计算失败" : "上游失败"} style={{color: node.evaluationStatus === "FAILED" ? "#b86151" : "#b38a48"}}><ExclamationCircleOutlined /></span>;
  if ((node.kind === "PAD" || node.kind === "REVOLVE") && node.operation === "REMOVE") return <ScissorOutlined />;
  return descriptors[node.kind]?.icon ?? <CloudUploadOutlined />;
}

export function canToggleNodeVisibility(kind: string | undefined): boolean {
  return Boolean(kind && descriptors[kind as Kind]?.visibility);
}
