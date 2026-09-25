import type { Artifact, DocumentView, MeshData } from "../../types";
import { decodeMeshGLB, type DecodedVisual } from "./mesh-glb";
export type DisplayArtifact = Artifact & {
    mesh: MeshData;
};
export type DisplayDocumentView = Omit<DocumentView, "artifact" | "artifacts"> & {
    artifact?: DisplayArtifact;
    artifacts?: Record<string, DisplayArtifact>;
};
// One viewport owns the decoded working set. No geometry is put in query/server state.
export class VisualRepository {
    private entries = new Map<string, Promise<DecodedVisual>>();
    private abort = new AbortController();
    async hydrate(view: DocumentView): Promise<DisplayDocumentView> {
        const load = async (a: Artifact): Promise<DisplayArtifact> => {
            const ref = a.representations.VISUAL;
            if (!ref)
                throw new Error("Missing visual artifact");
            if (ref.schemaVersion !== 1 || ref.contentType !== "model/gltf-binary")
                throw new Error("Unsupported visual artifact contract");
            if (!ref.url && !/^[a-f0-9]{64}$/.test(ref.digest))
                throw new Error("Invalid visual artifact digest");
            const key = `${ref.objectId}:${ref.digest}`;
            let pending = this.entries.get(key);
            if (!pending) {
                pending = (async () => {
                    const response = await fetch(ref.url ?? `/api/documents/${encodeURIComponent(view.document.id)}/representations/${encodeURIComponent(ref.objectId)}?versionId=${encodeURIComponent(view.document.versionId)}`, { credentials: "same-origin", signal: this.abort.signal });
                    if (!response.ok)
                        throw new Error(`Display artifact download failed (${response.status})`);
                    const buffer = await response.arrayBuffer();
                    if (buffer.byteLength !== ref.size)
                        throw new Error("Display artifact size mismatch");
                    // Mock URLs are local authored fixtures; persistent references always carry SHA-256.
                    if (/^[a-f0-9]{64}$/.test(ref.digest) && globalThis.crypto?.subtle) {
                        const hash = Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", buffer)), b => b.toString(16).padStart(2, "0")).join("");
                        if (hash !== ref.digest)
                            throw new Error("Display artifact digest mismatch");
                    }
                    return decodeMeshGLB(buffer);
                })();
                this.entries.set(key, pending);
                pending.catch(() => { if (this.entries.get(key) === pending)
                    this.entries.delete(key); });
            }
            const decoded = await pending;
            return { ...a, mesh: decoded.mesh, visualization: decoded.visualization ?? a.visualization };
        };
        const artifact = view.artifact ? await load(view.artifact) : undefined;
        const artifacts: Record<string, DisplayArtifact> = {};
        // Bound simultaneous downloads/decodes independently of occurrence count.
        const entries = Object.entries(view.artifacts ?? {});
        let next = 0;
        await Promise.all(Array.from({ length: Math.min(4, entries.length) }, async () => { while (next < entries.length) {
            const [key, a] = entries[next++];
            artifacts[key] = await load(a);
        } }));
        return { ...view, artifact, artifacts };
    }
    retain(views: DocumentView[]): void {
        const active = new Set(views.flatMap(view => [...(view.artifact ? [view.artifact] : []), ...Object.values(view.artifacts ?? {})]).map(a => `${a.representations.VISUAL?.objectId}:${a.representations.VISUAL?.digest}`));
        for (const key of this.entries.keys())
            if (!active.has(key))
                this.entries.delete(key);
    }
    dispose(): void { this.abort.abort(); this.entries.clear(); }
}
