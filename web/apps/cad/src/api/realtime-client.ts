import type { CommandPreview, DocumentView, Job } from "../types";
import { closeAfterInitializationFailure } from "./websocket-lifecycle";
import { randomUUID } from "../utils/random-uuid";

const protocol = "occccad.realtime.v1";
const apiBaseURL = (import.meta.env.VITE_API_BASE_URL ?? "").replace(/\/$/, "");

type EnvelopeKind = "request" | "response" | "event" | "ack" | "error";

interface Envelope<T = unknown> {
  protocol: typeof protocol;
  id: string;
  kind: EnvelopeKind;
  type: string;
  correlationId?: string;
  sequence?: number;
  sentAt: string;
  payload?: T;
  error?: { code: string; message: string; retryable: boolean; phase?: string };
}

interface SubscriptionSnapshot {
  documentId: string;
  workspaceId: string;
  sequence: number;
  view: DocumentView;
}

interface DocumentEvent {
  type: string;
  sequence?: number;
  payload: unknown;
}

type DocumentListener = (event: DocumentEvent) => void;
// Realtime carries status only; full Job payload belongs to the resource query.
type JobEvent = Omit<Job, "payload">;
type JobListener = (job: JobEvent) => void;

const cookie = (name: string): string => decodeURIComponent(document.cookie.split("; ")
  .find((item) => item.startsWith(`${name}=`))?.split("=").slice(1).join("=") ?? "");

function socketURL(): string {
  const target = new URL(`${apiBaseURL}/api/realtime`, window.location.href);
  target.protocol = target.protocol === "https:" ? "wss:" : "ws:";
  return target.toString();
}

export class RealtimeError extends Error {
 constructor(public readonly code: string, message: string, public readonly retryable = false, public readonly phase?: string) { super(message); }
}

export class RealtimeClient {
  private socket?: WebSocket;
  private connectPromise?: Promise<void>;
  private resolveConnect?: () => void;
  private rejectConnect?: (error: Error) => void;
  private explicitlyStopped = true;
  private connectedOnce = false;
  private reconnectAttempt = 0;
  private reconnectTimer?: number;
  private readonly pending = new Map<string, {
    resolve: (payload: unknown) => void;
    reject: (error: Error) => void;
    timer: number;
    cleanup?: () => void;
  }>();
  private readonly listeners = new Map<string, Set<DocumentListener>>();
  private readonly desiredDocuments = new Set<string>();
  private readonly inlineSnapshots = new Map<string, boolean>();
  private readonly sequences = new Map<string, number>();
  private connectionGeneration = 0;
  private connectTimer?: number;
  private readonly refreshing = new Map<string, Promise<SubscriptionSnapshot>>();
  private readonly previews = new Map<string, { sequence: number; abort: AbortController; cleanup: () => void }>();
  private readonly recoveryListeners = new Set<() => void>();
  private readonly jobListeners = new Set<JobListener>();

  start(): void {
    this.explicitlyStopped = false;
    void this.ensureConnected().catch(() => undefined);
  }

  stop(): void {
    this.explicitlyStopped = true;
    this.connectionGeneration++;
    this.refreshing.clear();
    window.clearTimeout(this.connectTimer);
    for (const entry of this.previews.values()) { entry.abort.abort(); entry.cleanup(); }
    this.previews.clear();
    window.clearTimeout(this.reconnectTimer);
    this.socket?.close(1000, "client stopped");
    this.socket = undefined;
    const error = new RealtimeError("STOPPED", "realtime connection stopped");
    this.rejectConnect?.(error);
    this.connectPromise = undefined;
    this.resolveConnect = undefined;
    this.rejectConnect = undefined;
    this.connectedOnce = false;
    this.rejectPending(error);
  }

  async subscribe(documentId: string, listener: DocumentListener): Promise<() => void> {
    this.desiredDocuments.add(documentId);
    const listeners = this.listeners.get(documentId) ?? new Set<DocumentListener>();
    listeners.add(listener);
    this.listeners.set(documentId, listeners);
    try {
      await this.request("document.subscribe.v1", { documentId });
      await this.refreshSnapshot(documentId);
    } catch (error) {
      listeners.delete(listener);
      if (listeners.size === 0) {
        this.listeners.delete(documentId);
        this.desiredDocuments.delete(documentId);
      }
      throw error;
    }
    return () => {
      const current = this.listeners.get(documentId);
      current?.delete(listener);
      if (current && current.size === 0) {
        this.listeners.delete(documentId);
        this.desiredDocuments.delete(documentId);
        this.sequences.delete(documentId);
        this.inlineSnapshots.delete(documentId);
        if (!this.explicitlyStopped && this.socket?.readyState === WebSocket.OPEN) void this.request("document.unsubscribe.v1", { documentId }).catch(() => undefined);
      }
    };
  }

  async executeCommand(documentId: string, command: Record<string, unknown>): Promise<DocumentView> {
    const requestId = typeof command.requestId === "string" ? command.requestId : randomUUID();
    const payload = { documentId, inlineSnapshot: this.inlineSnapshots.get(documentId) ?? true, command: { ...command, requestId } };
    for (let attempt = 0; ; attempt++) {
      try {
        const receipt = await this.request<{ sequence: number; versionId?: string; view?: DocumentView }>("workspace.command.execute.v1", payload);
        this.sequences.set(documentId, Math.max(this.sequences.get(documentId) ?? 0, receipt.sequence));
        if (receipt.view && receipt.view.document.id === documentId && receipt.view.document.versionId === receipt.versionId
          && receipt.sequence >= (this.sequences.get(documentId) ?? 0)) return receipt.view;
        return (await this.refreshSnapshot(documentId)).view;
      } catch (error) {
        if (attempt >= 1 || this.explicitlyStopped || !(error instanceof RealtimeError) || !["CONNECTION_CLOSED", "TIMEOUT", "DATABASE_BUSY", "REALTIME_BUSY"].includes(error.code)) throw error;
        // The server may have committed before the connection dropped. Retry
        // once with the same persistent requestId so the command handler
        // returns the stored result instead of applying the intent twice.
      }
    }
  }

  onJobEvent(listener: JobListener): () => void {
    this.jobListeners.add(listener);
    return () => this.jobListeners.delete(listener);
  }

  private async ensureConnected(): Promise<void> {
    if (this.socket?.readyState === WebSocket.OPEN && !this.connectPromise) return;
    if (this.connectPromise) return this.connectPromise;
    this.explicitlyStopped = false;
    this.connectPromise = new Promise<void>((resolve, reject) => {
      this.resolveConnect = resolve;
      this.rejectConnect = reject;
    });
    this.connectionGeneration++;
    const socket = new WebSocket(socketURL(), protocol);
    this.socket = socket;
    this.connectTimer = window.setTimeout(() => { if (this.socket === socket && this.connectPromise) socket.close(4000, "connection timeout"); }, 10_000);
    socket.onopen = () => {
      if (this.socket !== socket) return;
      const id = randomUUID();
      this.addPending(id, () => {
        window.clearTimeout(this.connectTimer);
        const reconnecting = this.connectedOnce;
        this.connectedOnce = true;
        this.reconnectAttempt = 0;
        this.resolveConnect?.();
        this.connectPromise = undefined;
        this.resolveConnect = undefined;
        this.rejectConnect = undefined;
        if (reconnecting) void this.resubscribe();
      }, (error) => {
        this.rejectConnect?.(error);
        closeAfterInitializationFailure(socket);
      });
      this.send({ protocol, id, kind: "request", type: "connection.initialize.v1",
        sentAt: new Date().toISOString(), payload: { csrfToken: cookie("occccad_csrf") } });
    };
    socket.onmessage = (message) => { if (this.socket === socket) this.receive(message.data); };
    socket.onerror = () => undefined;
    socket.onclose = () => this.closed(socket);
    return this.connectPromise;
  }

  async previewCommand(documentId: string, command: Record<string, unknown>, signal?: AbortSignal): Promise<CommandPreview> {
    const interactionId = typeof command.interactionId === "string" ? command.interactionId : `feature/${documentId}`;
    const key = `${documentId}/${interactionId}`;
    const previous = this.previews.get(key);
    const sequence = typeof command.previewSequence === "number" ? command.previewSequence : (previous?.sequence ?? 0) + 1;
    if (!Number.isSafeInteger(sequence) || sequence <= (previous?.sequence ?? 0)) throw new RealtimeError("STALE_PREVIEW", "preview sequence is not newer");
    previous?.abort.abort(); previous?.cleanup();
    const abort = new AbortController();
    const onAbort = () => abort.abort();
    const cancel = () => {
      // Do not reconnect just to cancel an already disconnected session.
      if (this.socket?.readyState === WebSocket.OPEN) this.send({ protocol, id: randomUUID(), kind: "request", type: "workspace.preview.cancel.v1", sentAt: new Date().toISOString(), payload: { documentId, interactionId, previewSequence: sequence } });
    };
    signal?.addEventListener("abort", onAbort, { once: true });
    abort.signal.addEventListener("abort", cancel, { once: true });
    const cleanup = () => { signal?.removeEventListener("abort", onAbort); abort.signal.removeEventListener("abort", cancel); };
    const entry = { sequence, abort, cleanup };
    this.previews.set(key, entry);
    if (this.previews.size > 64) { const oldest = this.previews.keys().next().value!; if (oldest !== key) { const old = this.previews.get(oldest)!; old.abort.abort(); old.cleanup(); this.previews.delete(oldest); } }
    if (signal?.aborted) abort.abort();
    try {
      const result = await this.request<{ interactionId: string; previewSequence: number; preview?: CommandPreview }>("workspace.preview.request.v1", {
        documentId, interactionId, previewSequence: sequence,
        command: { ...command, requestId: command.requestId ?? randomUUID(), interactionId, previewSequence: sequence },
      }, abort.signal, 20_000);
      if (abort.signal.aborted || this.previews.get(key) !== entry || !result.preview || result.interactionId !== interactionId || result.previewSequence !== sequence) throw new DOMException("Preview superseded or canceled", "AbortError");
      if (result.preview.baseSequence < (this.sequences.get(documentId) ?? 0)) throw new DOMException("Preview base expired", "AbortError");
      return result.preview;
    } catch (error) {
      abort.abort(); cleanup(); throw error;
    }
  }

  onRecovery(listener: () => void): () => void {
    this.recoveryListeners.add(listener);
    return () => this.recoveryListeners.delete(listener);
  }

  private async request<T>(type: string, payload: unknown, signal?: AbortSignal, timeout = 120_000): Promise<T> {
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    await this.ensureConnected();
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) throw new RealtimeError("CONNECTION_CLOSED", "realtime connection unavailable", true);
    const id = randomUUID();
    return new Promise<T>((resolve, reject) => {
      this.addPending(id, (value) => resolve(value as T), reject, timeout);
      const onAbort = () => {
        const pending = this.pending.get(id);
        if (!pending) return;
        window.clearTimeout(pending.timer); this.pending.delete(id); pending.cleanup?.();
        reject(new DOMException("Aborted", "AbortError"));
      };
      signal?.addEventListener("abort", onAbort, { once: true });
      this.pending.get(id)!.cleanup = () => signal?.removeEventListener("abort", onAbort);
      try { this.send({ protocol, id, kind: "request", type, sentAt: new Date().toISOString(), payload }); }
      catch (error) { const pending = this.pending.get(id)!; window.clearTimeout(pending.timer); pending.cleanup?.(); this.pending.delete(id); reject(error); }
    });
  }

  private addPending(id: string, resolve: (payload: unknown) => void, reject: (error: Error) => void, timeout = 120_000): void {
    const timer = window.setTimeout(() => {
      this.pending.get(id)?.cleanup?.(); this.pending.delete(id);
      reject(new RealtimeError("TIMEOUT", "realtime request timed out", true));
    }, timeout);
    this.pending.set(id, { resolve, reject, timer });
  }

  private send(envelope: Envelope): void {
    const raw = JSON.stringify(envelope);
    if (new TextEncoder().encode(raw).byteLength > 1 << 20) throw new RealtimeError("MESSAGE_TOO_LARGE", "realtime request exceeds control-plane limit");
    this.socket?.send(raw);
  }

  private receive(raw: unknown): void {
    if (typeof raw !== "string") return;
    let envelope: Envelope;
    try { envelope = JSON.parse(raw) as Envelope; } catch { return; }
    if (envelope.protocol !== protocol) return;
    if ((envelope.kind === "response" || envelope.kind === "error") && envelope.correlationId) {
      const pending = this.pending.get(envelope.correlationId);
      if (!pending) return;
      window.clearTimeout(pending.timer);
      pending.cleanup?.();
      this.pending.delete(envelope.correlationId);
      if (envelope.kind === "error") pending.reject(new RealtimeError(envelope.error?.code ?? "INTERNAL", envelope.error?.message ?? "realtime request failed", envelope.error?.retryable, envelope.error?.phase));
      else pending.resolve(envelope.payload);
      return;
    }
    if (envelope.kind !== "event") return;
    if (envelope.type === "job.state.changed.v1") {
      const job = (envelope.payload as { job?: JobEvent } | undefined)?.job;
      if (job) for (const listener of this.jobListeners) listener(job);
      return;
    }
    const payload = envelope.payload as { documentId?: string } | undefined;
    if (!payload?.documentId) return;
    const previous = this.sequences.get(payload.documentId) ?? 0;
    if (envelope.sequence !== undefined) {
      if (envelope.sequence <= previous) return;
      this.sequences.set(payload.documentId, envelope.sequence);
      for (const [key, entry] of this.previews) if (key.startsWith(`${payload.documentId}/`)) { entry.abort.abort(); entry.cleanup(); }
      if (envelope.sequence > previous + 1) {
        for (const listener of this.listeners.get(payload.documentId) ?? []) {
          listener({ type: "stream.gap.v1", sequence: envelope.sequence, payload: envelope.payload });
        }
        void this.refreshSnapshot(payload.documentId).catch(() => this.socket?.close(4002, "snapshot recovery failed"));
        return;
      }
    }
    for (const listener of this.listeners.get(payload.documentId) ?? []) {
      listener({ type: envelope.type, sequence: envelope.sequence, payload: envelope.payload });
    }
    if (envelope.sequence !== undefined && this.socket?.readyState === WebSocket.OPEN) {
      this.send({ protocol, id: randomUUID(), kind: "ack", type: "stream.ack.v1",
        sequence: envelope.sequence, sentAt: new Date().toISOString(),
        payload: { documentId: payload.documentId } });
    }
  }

  private closed(socket: WebSocket): void {
    // A late close event from a deliberately stopped or superseded socket
    // must not tear down the current connection or its pending requests.
    if (this.socket !== socket) return;
    this.socket = undefined;
    this.connectionGeneration++;
    window.clearTimeout(this.connectTimer);
    this.refreshing.clear();
    for (const entry of this.previews.values()) { entry.abort.abort(); entry.cleanup(); }
    this.previews.clear();
    const error = new RealtimeError("CONNECTION_CLOSED", "realtime connection closed", true);
    this.rejectConnect?.(error);
    this.connectPromise = undefined;
    this.resolveConnect = undefined;
    this.rejectConnect = undefined;
    this.rejectPending(error);
    if (this.explicitlyStopped) return;
    const delay = Math.min(10_000, 250 * 2 ** Math.min(this.reconnectAttempt++, 6));
    this.reconnectTimer = window.setTimeout(() => {
      void this.ensureConnected().catch(() => undefined);
    }, delay);
  }

  private rejectPending(error: Error): void {
    for (const pending of this.pending.values()) {
      window.clearTimeout(pending.timer);
      pending.cleanup?.();
      pending.reject(error);
    }
    this.pending.clear();
  }

  private async fetchSnapshot(documentId: string): Promise<SubscriptionSnapshot> {
    for (let attempt = 0; attempt < 3; attempt++) {
      const response = await fetch(`${apiBaseURL}/api/documents/${encodeURIComponent(documentId)}/realtime-snapshot`, { credentials: "include", cache: "no-store", signal: AbortSignal.timeout(30_000) });
      if ([409, 503].includes(response.status)) continue;
      if (!response.ok) throw new RealtimeError(response.status === 403 ? "FORBIDDEN" : response.status === 404 ? "NOT_FOUND" : "SNAPSHOT_FAILED", `snapshot failed (${response.status})`);
      const snapshot = await response.json() as SubscriptionSnapshot;
      this.inlineSnapshots.set(documentId, new TextEncoder().encode(JSON.stringify(snapshot.view)).byteLength <= 64 * 1024);
      return snapshot;
    }
    throw new RealtimeError("SNAPSHOT_BUSY", "authoritative snapshot is changing; retry", true);
  }

  private refreshSnapshot(documentId: string): Promise<SubscriptionSnapshot> {
    const current = this.refreshing.get(documentId);
    if (current) return current;
    const generation = this.connectionGeneration;
    const pending = (async () => {
      for (let attempt = 0; attempt < 3; attempt++) {
        const snapshot = await this.fetchSnapshot(documentId);
        if (generation !== this.connectionGeneration) throw new RealtimeError("CONNECTION_CLOSED", "realtime connection changed", true);
        if (snapshot.sequence < (this.sequences.get(documentId) ?? 0)) continue;
        this.sequences.set(documentId, snapshot.sequence);
        for (const listener of this.listeners.get(documentId) ?? []) listener({ type: "document.snapshot.v1", sequence: snapshot.sequence, payload: snapshot });
        return snapshot;
      }
      throw new RealtimeError("SNAPSHOT_BUSY", "snapshot is behind the event stream", true);
    })();
    this.refreshing.set(documentId, pending);
    void pending.finally(() => { if (this.refreshing.get(documentId) === pending) this.refreshing.delete(documentId); }).catch(() => undefined);
    return pending;
  }

  private async resubscribe(): Promise<void> {
    for (const documentId of this.desiredDocuments) {
      try {
        await this.request("document.subscribe.v1", { documentId });
        await this.refreshSnapshot(documentId);
      } catch (error) {
        if (error instanceof RealtimeError && ["FORBIDDEN", "NOT_FOUND"].includes(error.code)) {
          for (const listener of this.listeners.get(documentId) ?? []) listener({ type: "document.unavailable.v1", payload: { documentId } });
          this.desiredDocuments.delete(documentId);
        } else { this.socket?.close(4002, "subscription recovery failed"); return; }
      }
    }
    for (const listener of this.recoveryListeners) listener();
  }
}

export const realtime = new RealtimeClient();
