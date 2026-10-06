type Task<T> = {
  run: (signal: AbortSignal) => Promise<T>;
  controller: AbortController;
  resolve: (value: T) => void;
  reject: (error: unknown) => void;
  cleanup: () => void;
};

/** One running computation and one replaceable pending input per editor. */
export class LatestPreviewQueue<T> {
  private active?: Task<T>;
  private pending?: Task<T>;

  submit(run: (signal: AbortSignal) => Promise<T>, signal: AbortSignal): Promise<T> {
    this.active?.controller.abort();
    if (this.pending) {
      this.pending.controller.abort();
      this.pending.cleanup();
      this.pending.reject(new DOMException('Preview superseded', 'AbortError'));
    }
    return new Promise((resolve, reject) => {
      const controller = new AbortController();
      const cancel = () => controller.abort();
      signal.addEventListener('abort', cancel, { once: true });
      if (signal.aborted) cancel();
      this.pending = {
        run,
        controller,
        resolve,
        reject,
        cleanup: () => signal.removeEventListener('abort', cancel),
      };
      this.pump();
    });
  }

  private pump(): void {
    if (this.active || !this.pending) return;
    const task = this.pending;
    this.pending = undefined;
    this.active = task;
    void (async () => {
      try {
        if (task.controller.signal.aborted) {
          throw new DOMException('Preview canceled', 'AbortError');
        }
        const value = await task.run(task.controller.signal);
        if (task.controller.signal.aborted) {
          throw new DOMException('Preview superseded', 'AbortError');
        }
        task.resolve(value);
      } catch (error) {
        task.reject(error);
      } finally {
        task.cleanup();
        this.active = undefined;
        this.pump();
      }
    })();
  }
}
