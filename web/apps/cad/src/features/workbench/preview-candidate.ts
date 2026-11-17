/** One immutable input owns one computation. Confirmation starts a debounced
 * candidate immediately or joins its running/completed preview; it never sends
 * a second evaluation. The editor still checks its current key and Revision. */
export class PreviewCandidate<T> {
  readonly controller = new AbortController();
  private result?: Promise<T>;
  private readonly run: (signal: AbortSignal) => Promise<T>;
  constructor(run: (signal: AbortSignal) => Promise<T>) {this.run=run;}
  start(): Promise<T> {
    return this.result ??= this.controller.signal.aborted
      ? Promise.reject(new DOMException('Preview canceled', 'AbortError'))
      : this.run(this.controller.signal);
  }
  cancel(): void { this.controller.abort(); }
}
