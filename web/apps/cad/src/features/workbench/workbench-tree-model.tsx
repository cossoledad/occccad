import {structureSelection} from "./structure-selection";
export {structureSelection} from "./structure-selection";
import {engineeringLabel} from "../../cad/presentation/engineering-text";
import {documentRegistry} from "../../cad/document/document-registry";
import { treeNodeIcon } from "./tree-node-descriptors";
import type { DocumentStructureNode, DocumentView, Feature, Selection, SelectionItem } from "../../types";
import type { SpecificationTreeNode } from "./specification-tree";

export function isSolidFeature(feature: Feature): boolean {
  return ["HOLE", "SOLID_PATTERN", "PAD", "LINEAR_EXTRUDE", "REVOLVE", "IMPORT_BODY", "BOOLEAN", "FILLET", "CHAMFER", "DRAFT", "SHELL", "LOFT"].includes(feature.type.toUpperCase());
}

export function findStructureEntity(node: DocumentStructureNode | undefined, entityID: string): DocumentStructureNode | undefined {
  if (!node) return undefined;
  if (node.entityId === entityID && ["SKETCH", "PAD", "REVOLVE", "IMPORT", "FEATURE"].includes(node.kind)) return node;
  for (const child of node.children ?? []) {
    const found = findStructureEntity(child, entityID);
    if (found) return found;
  }
  return undefined;
}

export function findStructureOccurrenceEntity(root: DocumentStructureNode | undefined, entityID: string,
  occurrencePath: string, bodyID?: string): DocumentStructureNode | undefined {
  if (!root) return undefined;
  if (root.entityId === entityID && root.instancePath?.canonical === occurrencePath &&
      (!bodyID || root.bodyId === bodyID) && ["SKETCH", "PAD", "REVOLVE", "IMPORT", "FEATURE"].includes(root.kind)) return root;
  for (const child of root.children ?? []) {
    const found = findStructureOccurrenceEntity(child, entityID, occurrencePath, bodyID);
    if (found) return found;
  }
  return undefined;
}

function mapStructureNode(node: DocumentStructureNode, view: DocumentView, editingView?: DocumentView): SpecificationTreeNode {
  const ownerDocumentID = node.kind === "INSTANCE" ? node.instancePath?.segments.at(-1)?.ownerDocumentId : node.documentId;
  const nodeView = node.documentId === editingView?.document.id ? editingView : node.documentId === view.document.id ? view : undefined;
  const capabilityView = ownerDocumentID === editingView?.document.id ? editingView : ownerDocumentID === view.document.id ? view : nodeView;
  const canEdit = capabilityView?.document.permission === "OWNER" || capabilityView?.document.permission === "EDITOR";
  const sketch=nodeView?.part?.features.find((feature)=>feature.id===node.ownerEntityId)?.sketch;
  const conflictConstraints=new Set(sketch?.solve.conflictingConstraintIds??[]), conflictEntities=new Set<string>();
  let changed=true;while(changed){changed=false;for(const constraint of sketch?.constraints??[]){if(constraint.suppressed)continue;
    const ids=constraint.references.flatMap((reference)=>reference.entityId?[reference.entityId]:[]);
    if(conflictConstraints.has(constraint.id)||ids.some((id)=>conflictEntities.has(id))){if(!conflictConstraints.has(constraint.id)){conflictConstraints.add(constraint.id);changed=true;}
      for(const id of ids)if(!conflictEntities.has(id)){conflictEntities.add(id);changed=true;}}
  }}
	const component=node.entityId?sketch?.solve.components?.find((candidate)=>candidate.entityIds.includes(node.entityId!)):undefined;
  const featureStatus = node.evaluationStatus === "FAILED" ? "求值失败" : node.evaluationStatus === "BLOCKED" ? "上游失败" : node.evaluationStatus === "SUPPRESSED" ? "已抑制" : undefined;
  const inputLabel:Record<string,string>={POSITION:"定位点",PROFILE:"轮廓",SECTION:"截面",SECTION_POINT:"截面点",AXIS:"轴",SUPPORT:"支撑",TOOL:"工具",RANGE_START:"范围起始",SEED:"种子",TOPOLOGY:"拓扑引用",STAGE:"输入阶段",NEUTRAL_PLANE:"中性面",TERMINATION_PLANE:"终止平面",PATTERN_REFERENCE:"阵列参考"};
  const displayName=node.kind==="APPLICATIONS"?"机构与模拟":node.inputRole&&node.kind!=="KINEMATICS_GROUP"?`${inputLabel[node.inputRole]??"输入"}${["SECTION","SECTION_POINT"].includes(node.inputRole)?` ${1+(node.inputOrder??0)}`:""} · ${node.name}`:node.name===node.entityId?engineeringLabel(node.entityType??node.kind,"引用对象已丢失"):node.name;
  const currentSelection=structureSelection(node,view);
  const contribution=currentSelection?.geometryKey?view.artifacts?.[currentSelection.geometryKey]?.visualization?.featureContributions?.[node.entityId??""]:undefined;
  const contributionStatus=contribution==="NO_CURRENT_CONTRIBUTION"?"无当前几何贡献":contribution==="MAPPING_MISSING"?"拓扑关联缺失":undefined;
  const outputStatus = node.consumed ? "已用于布尔" : featureStatus??contributionStatus;
  return { key: node.id, title: displayName,
    icon: treeNodeIcon(node,outputStatus), kind: node.kind,
    axis:node.axis, parameterAlias:node.parameterAlias, entityId: node.entityId, documentId: node.documentId, documentType: node.documentType, instancePath: node.instancePath,
    ownerDocumentId: node.ownerDocumentId, bodyId: node.bodyId, presentationRole: node.presentationRole,
    definitionVisible: node.localVisible, localVisible: node.localVisible, visibilityMode: node.visibilityMode,
    childrenState: node.childrenState, resolutionStatus: node.resolutionStatus,
    connectionStatus: node.connectionStatus, currencyStatus: node.currencyStatus,
    evaluationStatus: node.evaluationStatus,
    sourceDocumentId: node.sourceDocumentId, sourceRevisionId: node.sourceRevisionId,
    sourceDisplayPath: node.sourceDisplayPath,
    plane: node.plane, ownerEntityId: node.ownerEntityId,
	role: node.role, definitionDigest: node.definitionDigest, suppressed: node.suppressed, diagnostic: node.diagnostic??(node.kind==="SKETCH_ENTITY"&&node.entityId&&conflictEntities.has(node.entityId)?"CONFLICTING":component?.definitionStatus==="FULLY_CONSTRAINED"||component?.status==="SOLVED"?"FULLY_CONSTRAINED":undefined),
    capabilities: canEdit ? [...new Set([...(node.capabilities??[]), ...(node.kind==="SKETCH"&&node.presentationRole!=="INPUT_REFERENCE"?["EDIT" as const,"RECONNECT" as const]:[]), ...(["SKETCH","SKETCH_GEOMETRY_SET","SKETCH_CONSTRAINT_SET","SKETCH_LOGICAL_CONSTRAINT_SET","SKETCH_DIMENSION_SET"].includes(node.kind)?["SUPPRESS" as const]:[])])] : undefined,
    selection: structureSelection(node, view), children: node.children?.map((child) => mapStructureNode(child, view, editingView)) };
}

export function treeData(view: DocumentView, editingView?: DocumentView): SpecificationTreeNode[] {
  const structure=documentRegistry.get(view.document.type).structure(view);
  if(!structure)return [];
 return [mapStructureNode(structure,view,editingView)];
}

export function selectedFeature(view: DocumentView, selection: Selection): Feature | undefined {
  if (!selection || !["sketch", "pad", "import"].includes(selection.kind)) return undefined;
  if (selection.documentId !== view.document?.id || selection.versionId !== view.document.versionId) return undefined;
  return view.part?.features.find((feature) => feature.id === selection.id);
}

export { associatedTreeKeyForSelection, deletableTreeNodesForSelections, ancestorHintKeysForSelections, treeKeyForSelection, treeKeysForSelections } from "./tree-projection-index";
