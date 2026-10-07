import {documentRegistry} from "../../cad/document/document-registry";
import type { DocumentStructureNode, DocumentView, InstancePath, ProductDesignSession } from "../../types";

export type EditTarget = {
  documentId: string;
  documentType: "PART" | "PRODUCT";
  instancePath?: InstancePath;
};

export type EditSession = {
  hostDocumentId: string;
  editTarget: EditTarget;
  snapshot: {
    hostRevisionId: string;
    targetRevisionId: string;
    rootSnapshotDigest?: string;
  };
  workingBodyId?: string;
  activationGeneration: number;
};

export type ActivatableTreeNode = Pick<DocumentStructureNode,
  "kind" | "documentId" | "documentType" | "instancePath">;

export class EditActivationGate {
  private generation = 0;

  begin(): number { this.generation += 1; return this.generation; }
  invalidate(): void { this.generation += 1; }
  isCurrent(generation: number): boolean { return generation === this.generation; }
  commit(generation: number, session: EditSession, apply: (session: EditSession) => void): boolean {
    if (!this.isCurrent(generation)) return false;
    apply(session);
    return true;
  }
}

export function rootEditSession(host: DocumentView, activationGeneration: number): EditSession {
  return {
    hostDocumentId: host.document.id,
    editTarget: documentRegistry.target(host),
    snapshot: { hostRevisionId: host.document.versionId, targetRevisionId: host.document.versionId },
    workingBodyId: documentRegistry.get(host.document.type).workingBody(host),
    activationGeneration,
  };
}

function isPathPrefix(prefix: string, path: string): boolean {
  return path === prefix || path.startsWith(`${prefix}/`);
}

export function pinnedReferenceInPath(root: DocumentStructureNode | undefined, path: string): DocumentStructureNode | undefined {
  if (!root) return undefined;
  if (root.kind === "INSTANCE" && root.referenceMode === "PINNED" && root.instancePath?.canonical &&
      isPathPrefix(root.instancePath.canonical, path)) return root;
  for (const child of root.children ?? []) {
    const pinned = pinnedReferenceInPath(child, path);
    if (pinned) return pinned;
  }
  return undefined;
}

export async function prepareOccurrenceEditSession(input: {
  host: DocumentView;
  node: ActivatableTreeNode;
  activationGeneration: number;
  getDesignSession: (hostDocumentId: string, activePath: string) => Promise<ProductDesignSession>;
  getDocument: (documentId: string) => Promise<DocumentView>;
}): Promise<{ session: EditSession; targetView: DocumentView }> {
  const { host, node, activationGeneration } = input;
  const path = node.instancePath?.canonical;
  if (host.document.type !== "PRODUCT" || !path || !node.documentId || !node.documentType) {
    throw new Error("该节点不属于可编辑的 Product occurrence");
  }
  const pinned = pinnedReferenceInPath(host.structureTree, path);
  if (pinned) {
    throw new Error(`路径 ${pinned.instancePath?.display ?? pinned.instancePath?.canonical ?? path} 已固定版本；请先显式恢复跟随最新版本再进入编辑`);
  }
  const resolved = await input.getDesignSession(host.document.id, path);
  if (resolved.rootProductDocumentId !== host.document.id || resolved.rootProductRevisionId !== host.document.versionId ||
      resolved.activeInstancePath?.canonical !== path || resolved.activeDocumentId !== node.documentId) {
    throw new Error("Product 上下文已变化，请等待更新完成后重试");
  }
  const targetView = await input.getDocument(resolved.activeDocumentId);
  if (targetView.document.id !== resolved.activeDocumentId || targetView.document.type !== node.documentType) {
    throw new Error("解析出的编辑目标与结构树身份不一致");
  }
  if (targetView.document.versionId !== resolved.activeRevisionId) {
    throw new Error("Product occurrence 尚未解析到目标文档当前版本，请先完成引用更新");
  }
  return {
    session: {
      hostDocumentId: host.document.id,
      editTarget: { documentId: resolved.activeDocumentId, documentType: targetView.document.type,
        instancePath: resolved.activeInstancePath },
      snapshot: { hostRevisionId: resolved.rootProductRevisionId, targetRevisionId: resolved.activeRevisionId,
        rootSnapshotDigest: resolved.rootSnapshotDigest },
      workingBodyId: documentRegistry.get(targetView.document.type).workingBody(targetView),
      activationGeneration,
    },
    targetView,
  };
}

export function isEditTargetNode(session: EditSession | undefined, node: ActivatableTreeNode): boolean {
  if (!session || (node.kind !== "PART" && node.kind !== "PRODUCT")) return false;
  return node.documentId === session.editTarget.documentId && node.kind === session.editTarget.documentType &&
    (node.instancePath?.canonical ?? "") === (session.editTarget.instancePath?.canonical ?? "");
}

export function withWorkingBody(session: EditSession, workingBodyId: string): EditSession {
  return { ...session, workingBodyId };
}
