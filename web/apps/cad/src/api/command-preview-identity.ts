// Keep generated entity identities stable between one preview and its commit.
// The server still validates Head, payload and expiry before promoting evidence.
export class CommandPreviewIdentities {
  private readonly values = new Map<string, { documentId: string; requestId: string; expires: number }>();

  remember(documentId: string, previewId: string, requestId: string, now = Date.now()): void {
    for (const [key, value] of this.values) if (value.expires <= now) this.values.delete(key);
    this.values.delete(previewId);
    while (this.values.size >= 256) this.values.delete(this.values.keys().next().value!);
    this.values.set(previewId, { documentId, requestId, expires: now + 45_000 });
  }

  requestFor(documentId: string, previewId: unknown, now = Date.now()): string | undefined {
    if (typeof previewId !== "string") return undefined;
    const value = this.values.get(previewId);
    if (!value || value.expires <= now || value.documentId !== documentId) return undefined;
    return value.requestId;
  }
}
