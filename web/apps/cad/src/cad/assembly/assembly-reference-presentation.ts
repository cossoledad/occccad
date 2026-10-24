import {engineeringInstanceName} from "./assembly-engineering-state";
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
  BODY: "组件", POINT: "点", AXIS: "轴", PLANE: "平面", CYLINDER: "圆柱面", CIRCLE:"圆支撑", SPHERE:"球面", CONE:"圆锥面", FRAME:"坐标系", FACE: "面（待精确解析）", EDGE: "边（待精确解析）", VERTEX: "顶点",
};

export function describeAssemblyReference(reference: AssemblyGeometryRef | undefined, view?: DocumentView): AssemblyReferencePresentation {
  if (!reference) return { primary: "未选择", secondary: "请选择支持元素" };
  const instanceNode = findNode(view?.structureTree, (node) => node.kind === "INSTANCE" && node.entityId === reference.instanceId);
  const occurrence=reference.instancePath?.display??(view?.product?engineeringInstanceName(view,reference.instanceId):instanceNode?.name??"引用零件已丢失");
  const occurrenceNode=findNode(view?.structureTree,node=>node.instancePath?.segments[0]?.instanceId===reference.instanceId||node.kind==="INSTANCE"&&node.entityId===reference.instanceId);
  const publicationNode = reference.publicationRef&&occurrenceNode&&findNode(occurrenceNode,node=>(node.kind==="PUBLICATION"||node.kind==="PRODUCT_PUBLICATION")&&node.entityId===reference.publicationRef?.publicationId);
  const publicationName = publicationNode?.name;
  const derived:Record<string,string>={"cylinder-axis":"圆柱面轴线","circle-axis":"圆边轴线","cone-axis":"圆锥面轴线","frame-axis-z":"坐标系 Z 轴","frame-origin":"坐标系原点","frame-plane-xy":"坐标系 XY 平面","sphere-center":"球心"};
  const standards:Record<string,string>={"datum-xy":"XY 平面","datum-yz":"YZ 平面","datum-xz":"XZ 平面","datum-zx":"XZ 平面"};
  const geometryNode=reference.geometryId?findNode(view?.structureTree,node=>node.entityId===reference.geometryId&&(node.kind==="PLANE"||node.kind==="DATUM_AXIS"||node.kind==="AXIS_SYSTEM")):undefined;
  const geometry=derived[reference.derivedRole??""]??geometryNode?.name??standards[reference.geometryId??""]??(reference.axis?`${reference.axis} 轴`:kindLabel[reference.kind]);
  return {primary:publicationName?`${occurrence} / ${publicationName}`:occurrence,secondary:geometry,publication:publicationName};
}
