import type { Artifact, DocumentView, MeshData } from "../../types";
import type { DecodedVisual } from "./mesh-glb";
import { VisualDecoder } from "./visual-decoder";
import { applyVisualDelta, VISUAL_DELTA_TYPE } from "./visual-delta";
type LoadedVisual = {decoded: DecodedVisual; buffer?: ArrayBuffer};
type RawBase = {buffer: ArrayBuffer; path: string; digest: string};
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
    private entries = new Map<string, Promise<LoadedVisual>>();
    private decoder = new VisualDecoder();
    private bases = new Map<string, RawBase>();
    private generation = 0;
    private latest = new Map<string, number>();
    private abort = new AbortController();
    // Only a completed, verified preview crosses the cancellation boundary.
    // Keep at most one; pending downloads remain owned by their preview.
    private reusablePreview?: { key: string; loaded: LoadedVisual };
    constructor(private readonly owner?: VisualRepository) {}
    previewRepository(): VisualRepository { return new VisualRepository(this); }
    clearReusablePreview(): void { this.reusablePreview = undefined; }
    async hydrate(view: DocumentView): Promise<DisplayDocumentView> {
        const load = async (a: Artifact): Promise<DisplayArtifact> => {
            const ref = a.representations.VISUAL;
            if (!ref)
                throw new Error("Missing visual artifact");
            if (ref.schemaVersion !== 2 || ref.contentType !== "model/gltf-binary")
                throw new Error("Unsupported visual artifact contract");
            if (!ref.url && !/^[a-f0-9]{64}$/.test(ref.digest))
                throw new Error("Invalid visual artifact digest");
            const key = `${ref.objectId}:${ref.digest}`;
            const path = ref.url ?? `/api/documents/${encodeURIComponent(view.document.id)}/representations/${encodeURIComponent(ref.objectId)}?versionId=${encodeURIComponent(view.document.versionId)}`;
            const scope = `${view.document.id}:${a.bodyId ?? a.result?.resultId ?? "part"}`;
            const generation = ++this.generation;
            if (!this.owner) this.latest.set(scope, generation);
            let pending = this.entries.get(key);
            const reuse = (this.owner ?? this).reusablePreview;
            if (!pending && reuse?.key === key) {
                pending = Promise.resolve(reuse.loaded);
                this.entries.set(key, pending);
            }
            if (!pending) {
                pending = (async () => {
                    const base = (this.owner ?? this).bases.get(scope);
                    const delta = base && base.digest !== ref.digest && ref.size >= 4096 && ref.size <= 64*1024*1024 && globalThis.crypto?.subtle && path.startsWith(`/api/documents/${encodeURIComponent(view.document.id)}/representations/`);
                    const requestPath = delta ? `${path}${path.includes('?') ? '&' : '?'}base=${encodeURIComponent(base.path)}` : path;
                    let response = await fetch(requestPath.startsWith("/") ? `${apiBaseURL}${requestPath}` : requestPath, { credentials: "include", signal: this.abort.signal, headers: delta ? {Accept: `${VISUAL_DELTA_TYPE}, model/gltf-binary`} : undefined });
                    // The optional base may have expired or lost authorization
                    // while the target remains valid. Retry only the full target.
                    if (delta && [400,403,404,409,410].includes(response.status)) response = await fetch(path.startsWith("/") ? `${apiBaseURL}${path}` : path, {credentials: "include", signal: this.abort.signal});
                    if (!response.ok)
                        throw new Error(`Display artifact download failed (${response.status})`);
                    let buffer = await response.arrayBuffer();
                    if (response.headers.get('Content-Type')?.split(';')[0] === VISUAL_DELTA_TYPE) {
                        if (!delta || !base) throw new Error('Unrequested visual delta');
                        buffer = applyVisualDelta(base.buffer, buffer, base.digest, ref.digest, ref.size);
                    }
                    if (buffer.byteLength !== ref.size)
                        throw new Error("Display artifact size mismatch");
                    // Mock URLs are local authored fixtures; persistent references always carry SHA-256.
                    if (/^[a-f0-9]{64}$/.test(ref.digest) && globalThis.crypto?.subtle) {
                        const hash = Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", buffer)), b => b.toString(16).padStart(2, "0")).join("");
                        if (hash !== ref.digest)
                            throw new Error("Display artifact digest mismatch");
                    }
                    const result = await (this.owner ?? this).decoder.decode(buffer, true, this.abort.signal);
                    if (this.abort.signal.aborted) throw new DOMException("Preview canceled", "AbortError");
                    return result;
                })();
                this.entries.set(key, pending);
                pending.catch(() => { if (this.entries.get(key) === pending)
                    this.entries.delete(key); });
            }
            const loaded = await pending, decoded = loaded.decoded;
            if (this.abort.signal.aborted) throw new DOMException("Display canceled", "AbortError");
            if ((a.representations.NAMING || decoded.association?.geometryId) && decoded.association?.geometryId !== a.geometryId) throw new Error("Visual geometry identity mismatch");
            if ((a.visualNamingDigest || a.representations.NAMING?.digest || decoded.association?.namingDigest) && decoded.association?.namingDigest !== (a.visualNamingDigest ?? a.representations.NAMING?.digest)) throw new Error("Visual Naming digest mismatch");
            if (this.owner) this.owner.reusablePreview = {key, loaded};
            else if (this.latest.get(scope) === generation && a.representationKind === 'PERSISTENT' && loaded.buffer && loaded.buffer.byteLength <= 64*1024*1024 && /^[a-f0-9]{64}$/.test(ref.digest)) {
                const basePath = new URL(path, 'http://display.local');
                if (basePath.origin === 'http://display.local' && basePath.searchParams.has('versionId') && !basePath.searchParams.has('previewId')) {
                    this.bases.delete(scope);
                    this.bases.set(scope, {buffer: loaded.buffer, path, digest: ref.digest});
                    let bytes = 0; for (const base of this.bases.values()) bytes += base.buffer.byteLength;
                    while (bytes > 64*1024*1024) {const oldest = this.bases.keys().next().value!; bytes -= this.bases.get(oldest)!.buffer.byteLength; this.bases.delete(oldest);}
                }
            }
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
    dispose(): void { this.abort.abort(); this.decoder.dispose(); this.entries.clear(); this.bases.clear(); this.latest.clear(); this.clearReusablePreview(); }
}
