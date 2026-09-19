import type { AssemblyGeometryRef, DocumentStructureNode, DocumentView } from "../../types";

export type AssemblyReferencePresentation = { primary: string; secondary: string; publication?: string };

function findNode(root: DocumentStructureNode | undefined, predicate: (node: DocumentStructureNode) => boolean): DocumentStructureNode | undefined {
  if (!root) return undefined;
  if (predicate(root)) return root;
  for (const child of root.children ?? []) {
    const found = findNode(child, predicate);
    if (found) return found;
  }
  return undefined;
}

const kindLabel: Record<AssemblyGeometryRef["kind"], string> = {
  BODY: "实体", POINT: "点", AXIS: "轴", PLANE: "平面", CYLINDER: "圆柱面", FACE: "面", EDGE: "边", VERTEX: "顶点",
};

export function describeAssemblyReference(reference: AssemblyGeometryRef | undefined, view?: DocumentView): AssemblyReferencePresentation {
  if (!reference) return { primary: "未选择", secondary: "请选择支持元素" };
  const instanceNode = findNode(view?.structureTree, (node) => node.kind === "INSTANCE" && node.entityId === reference.instanceId);
  const occurrence = instanceNode?.referenceName && instanceNode.instanceName
    ? `${instanceNode.referenceName}(${instanceNode.instanceName})`
    : instanceNode?.name ?? view?.product?.instances.find((instance) => instance.id === reference.instanceId)?.name ?? "未命名实例";
  const publicationNode = reference.publicationRef && findNode(view?.structureTree,
    (node) => (node.kind === "PUBLICATION" || node.kind === "PRODUCT_PUBLICATION") && node.entityId === reference.publicationRef?.publicationId);
  const publicationName = publicationNode?.name;
  const semantic = reference.persistentSelection?.anchor.outputSlot;
  const geometry = semantic ? `${kindLabel[reference.kind]} · ${semantic}`
    : reference.geometryId ? `${kindLabel[reference.kind]} · ${reference.geometryId}`
    : reference.axis ? `${kindLabel[reference.kind]} · ${reference.axis}` : kindLabel[reference.kind];
  return publicationName
    ? { primary: `${occurrence} / ${publicationName}`, secondary: `${geometry} · 稳定发布接口`, publication: publicationName }
    : { primary: occurrence, secondary: geometry };
}
