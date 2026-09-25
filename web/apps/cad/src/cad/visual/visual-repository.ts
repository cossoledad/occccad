import type { Artifact, DocumentView, MeshData } from "../../types";
import { decodeMeshGLB, type DecodedVisual } from "./mesh-glb";
const apiBaseURL = (import.meta.env?.VITE_API_BASE_URL ?? "").replace(/\/$/, "");

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
    // Only a completed, verified preview crosses the cancellation boundary.
    // Keep at most one; pending downloads remain owned by their preview.
    private reusablePreview?: { key: string; decoded: DecodedVisual };
    constructor(private readonly owner?: VisualRepository) {}
    previewRepository(): VisualRepository { return new VisualRepository(this); }
    clearReusablePreview(): void { this.reusablePreview = undefined; }
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
            const reuse = (this.owner ?? this).reusablePreview;
            if (!pending && reuse?.key === key) {
                pending = Promise.resolve(reuse.decoded);
                this.entries.set(key, pending);
            }
            if (!pending) {
                pending = (async () => {
                    const path = ref.url ?? `/api/documents/${encodeURIComponent(view.document.id)}/representations/${encodeURIComponent(ref.objectId)}?versionId=${encodeURIComponent(view.document.versionId)}`;
                    const response = await fetch(path.startsWith("/") ? `${apiBaseURL}${path}` : path, { credentials: "include", signal: this.abort.signal });
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
                    const decoded = decodeMeshGLB(buffer);
                    if (this.abort.signal.aborted) throw new DOMException("Preview canceled", "AbortError");
                    if (this.owner) this.owner.reusablePreview = { key, decoded };
                    return decoded;
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
    dispose(): void { this.abort.abort(); this.entries.clear(); this.clearReusablePreview(); }
}
