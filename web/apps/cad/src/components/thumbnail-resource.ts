export type ThumbnailImage = { blob: Blob; pending: boolean };
type ThumbnailRequest = { source: string; signal: AbortSignal; resolve: (image: ThumbnailImage) => void; reject: (error: unknown) => void; cancel: () => void };

/** Low-priority image work must leave connections available for navigation. */
export class ThumbnailRequests {
  private active = 0;
  private waiting: ThumbnailRequest[] = [];
  constructor(private readonly fetchImage: typeof fetch = (input, init) => fetch(input, init), private readonly limit = 4) {}

  load(source: string, signal: AbortSignal): Promise<ThumbnailImage> {
    return new Promise((resolve, reject) => {
      if (signal.aborted) { reject(signal.reason); return; }
      const request: ThumbnailRequest = { source, signal, resolve, reject, cancel: () => {
        this.waiting = this.waiting.filter(item => item !== request);
        reject(signal.reason);
      } };
      signal.addEventListener("abort", request.cancel, { once: true });
      this.waiting.push(request);
      this.drain();
    });
  }

  private drain() {
    while (this.active < this.limit && this.waiting.length) {
      const request = this.waiting.shift()!;
      request.signal.removeEventListener("abort", request.cancel);
      if (request.signal.aborted) { request.reject(request.signal.reason); continue; }
      this.active++;
      // Read the body before releasing the slot; fetch resolves at headers.
      void this.fetchImage(request.source, { credentials: "include", cache: "no-cache", signal: request.signal, priority: "low" })
        .then(async response => {
          if (!response.ok) throw new Error(`Thumbnail HTTP ${response.status}`);
          const blob = await response.blob();
          return { blob, pending: response.headers.get("X-OCCCCAD-Thumbnail-State") === "pending" };
        })
        .then(request.resolve, request.reject)
        .finally(() => { this.active--; this.drain(); });
    }
  }
}

export const thumbnailRequests = new ThumbnailRequests();
