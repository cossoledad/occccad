import type { DocumentView } from "../types";

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
  error?: { code: string; message: string; retryable: boolean };
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

const cookie = (name: string): string => decodeURIComponent(document.cookie.split("; ")
  .find((item) => item.startsWith(`${name}=`))?.split("=").slice(1).join("=") ?? "");

function socketURL(): string {
  const target = new URL(`${apiBaseURL}/api/realtime`, window.location.href);
  target.protocol = target.protocol === "https:" ? "wss:" : "ws:";
  return target.toString();
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
  }>();
  private readonly listeners = new Map<string, Set<DocumentListener>>();
  private readonly desiredDocuments = new Set<string>();
  private readonly sequences = new Map<string, number>();

  start(): void {
    this.explicitlyStopped = false;
    void this.ensureConnected().catch(() => undefined);
  }

  stop(): void {
    this.explicitlyStopped = true;
    window.clearTimeout(this.reconnectTimer);
    this.socket?.close(1000, "client stopped");
    this.socket = undefined;
    this.connectPromise = undefined;
    this.connectedOnce = false;
    this.rejectPending(new Error("realtime connection stopped"));
  }

  async subscribe(documentId: string, listener: DocumentListener): Promise<() => void> {
    this.desiredDocuments.add(documentId);
    const listeners = this.listeners.get(documentId) ?? new Set<DocumentListener>();
    listeners.add(listener);
    this.listeners.set(documentId, listeners);
    try {
      const snapshot = await this.request<SubscriptionSnapshot>("document.subscribe.v1", { documentId });
      this.sequences.set(documentId, Math.max(this.sequences.get(documentId) ?? 0, snapshot.sequence));
      listener({ type: "document.snapshot.v1", sequence: snapshot.sequence, payload: snapshot });
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
        void this.request("document.unsubscribe.v1", { documentId }).catch(() => undefined);
      }
    };
  }

  async executeCommand(documentId: string, command: Record<string, unknown>): Promise<DocumentView> {
    const requestId = typeof command.requestId === "string" ? command.requestId : crypto.randomUUID();
    const payload = { documentId, command: { ...command, requestId } };
    for (let attempt = 0; ; attempt++) {
      try {
        const result = await this.request<{ view: DocumentView }>("workspace.command.execute.v1", payload);
        return result.view;
      } catch (error) {
        const message = (error as Error).message;
        if (attempt >= 1 || !(message.includes("realtime connection") || message.includes("timed out"))) throw error;
        // The server may have committed before the connection dropped. Retry
        // once with the same persistent requestId so the command handler
        // returns the stored result instead of applying the intent twice.
      }
    }
  }

  private async ensureConnected(): Promise<void> {
    if (this.socket?.readyState === WebSocket.OPEN && !this.connectPromise) return;
    if (this.connectPromise) return this.connectPromise;
    this.explicitlyStopped = false;
    this.connectPromise = new Promise<void>((resolve, reject) => {
      this.resolveConnect = resolve;
      this.rejectConnect = reject;
    });
    const socket = new WebSocket(socketURL(), protocol);
    this.socket = socket;
    socket.onopen = () => {
      const id = crypto.randomUUID();
      this.addPending(id, () => {
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
        socket.close(1008, "initialization failed");
      });
      this.send({ protocol, id, kind: "request", type: "connection.initialize.v1",
        sentAt: new Date().toISOString(), payload: { csrfToken: cookie("occccad_csrf") } });
    };
    socket.onmessage = (message) => this.receive(message.data);
    socket.onerror = () => undefined;
    socket.onclose = () => this.closed();
    return this.connectPromise;
  }

  private async request<T>(type: string, payload: unknown): Promise<T> {
    await this.ensureConnected();
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) throw new Error("realtime connection unavailable");
    const id = crypto.randomUUID();
    return new Promise<T>((resolve, reject) => {
      this.addPending(id, (value) => resolve(value as T), reject);
      this.send({ protocol, id, kind: "request", type, sentAt: new Date().toISOString(), payload });
    });
  }

  private addPending(id: string, resolve: (payload: unknown) => void, reject: (error: Error) => void): void {
    const timer = window.setTimeout(() => {
      this.pending.delete(id);
      reject(new Error("realtime request timed out"));
    }, 120_000);
    this.pending.set(id, { resolve, reject, timer });
  }

  private send(envelope: Envelope): void {
    this.socket?.send(JSON.stringify(envelope));
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
      this.pending.delete(envelope.correlationId);
      if (envelope.kind === "error") pending.reject(new Error(envelope.error?.message ?? "realtime request failed"));
      else pending.resolve(envelope.payload);
      return;
    }
    if (envelope.kind !== "event") return;
    const payload = envelope.payload as { documentId?: string } | undefined;
    if (!payload?.documentId) return;
    const previous = this.sequences.get(payload.documentId) ?? 0;
    if (envelope.sequence !== undefined) {
      if (envelope.sequence <= previous) return;
      this.sequences.set(payload.documentId, envelope.sequence);
      if (previous > 0 && envelope.sequence > previous + 1) {
        for (const listener of this.listeners.get(payload.documentId) ?? []) {
          listener({ type: "stream.gap.v1", sequence: envelope.sequence, payload: envelope.payload });
        }
        return;
      }
    }
    for (const listener of this.listeners.get(payload.documentId) ?? []) {
      listener({ type: envelope.type, sequence: envelope.sequence, payload: envelope.payload });
    }
    if (envelope.sequence !== undefined && this.socket?.readyState === WebSocket.OPEN) {
      this.send({ protocol, id: crypto.randomUUID(), kind: "ack", type: "stream.ack.v1",
        sequence: envelope.sequence, sentAt: new Date().toISOString(),
        payload: { documentId: payload.documentId } });
    }
  }

  private closed(): void {
    this.socket = undefined;
    const error = new Error("realtime connection closed");
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
      pending.reject(error);
    }
    this.pending.clear();
  }

  private async resubscribe(): Promise<void> {
    for (const documentId of this.desiredDocuments) {
      try {
        const snapshot = await this.request<SubscriptionSnapshot>("document.subscribe.v1", { documentId });
        this.sequences.set(documentId, Math.max(this.sequences.get(documentId) ?? 0, snapshot.sequence));
        for (const listener of this.listeners.get(documentId) ?? []) {
          listener({ type: "document.snapshot.v1", sequence: snapshot.sequence, payload: snapshot });
        }
      } catch {
        // The next reconnect or an explicit route subscription retries. A
        // revoked permission intentionally leaves the document unsubscribed.
      }
    }
  }
}

export const realtime = new RealtimeClient();
