import type { Job } from "../../types";

export type ExchangeNotice = Omit<Job, "payload"> & {payload?: Job["payload"]};
const terminal = (job: ExchangeNotice) => ["SUCCEEDED", "FAILED", "CANCELED"].includes(job.state);
type Batch = {jobs: Map<string, ExchangeNotice>; sealed: boolean};

/** One completion per submitted batch, with durable per-job details in Activity.
 * Registration starts before uploading, so fast completions cannot escape as
 * individual notices while other files are still being submitted. */
export class ExchangeNotices {
  private batches = new Set<Batch>();
  private owners = new Map<string, Batch>();
  private pending = new Map<string, ExchangeNotice>();
  private timers = new Map<string, ReturnType<typeof setTimeout>>();
  private delivered = new Set<string>();
  private listener?: (jobs: ExchangeNotice[]) => void;
  constructor(private readonly delay = 500) {}
  listen(listener: (jobs: ExchangeNotice[]) => void) {this.listener = listener; return () => this.reset();}
  begin() {
    const batch: Batch = {jobs: new Map(), sealed: false}; this.batches.add(batch);
    return {
      track: (job: ExchangeNotice) => {
        this.owners.set(job.id, batch);
        batch.jobs.set(job.id, this.pending.get(job.id) ?? job);
        this.pending.delete(job.id); this.clearTimer(job.id);
      },
      seal: () => {
        batch.sealed = true; this.finish(batch);
        for (const job of this.pending.values()) this.schedule(job);
      },
    };
  }
  receive(job: ExchangeNotice) {
    if (!["EXCHANGE_IMPORT", "EXCHANGE_EXPORT"].includes(job.type) || !terminal(job)) return;
    if (this.delivered.has(this.key(job))) return;
    const batch = this.owners.get(job.id);
    if (batch) {batch.jobs.set(job.id, job); this.finish(batch);}
    else {this.pending.set(job.id, job); this.schedule(job);}
  }
  reconcile(jobs: Job[]) {for (const job of jobs) if (this.owners.has(job.id)) this.receive(job);}
  private finish(batch: Batch) {
    if (!batch.sealed || [...batch.jobs.values()].some(job => !terminal(job))) return;
    this.batches.delete(batch);
    const jobs = [...batch.jobs.values()];
    for (const job of jobs) this.owners.delete(job.id);
    if (jobs.length) this.deliver(jobs);
  }
  private schedule(job: ExchangeNotice) {
    if ([...this.batches].some(batch => !batch.sealed) || this.timers.has(job.id)) return;
    this.timers.set(job.id, setTimeout(() => {
      this.timers.delete(job.id);
      const latest = this.pending.get(job.id); this.pending.delete(job.id);
      if (latest) this.deliver([latest]);
    }, this.delay));
  }
  private key(job: ExchangeNotice) {return `${job.id}/${job.attemptCount}/${job.state}`;}
  private deliver(jobs: ExchangeNotice[]) {
    const fresh = jobs.filter(job => !this.delivered.has(this.key(job)));
    for (const job of fresh) this.delivered.add(this.key(job));
    // Session-local deduplication is bounded; the server remains the history.
    while (this.delivered.size > 512) this.delivered.delete(this.delivered.values().next().value!);
    if (fresh.length) this.listener?.(fresh);
  }
  private clearTimer(id: string) {const timer = this.timers.get(id); if (timer !== undefined) clearTimeout(timer); this.timers.delete(id);}
  reset() {
    for (const id of this.timers.keys()) this.clearTimer(id);
    this.batches.clear(); this.owners.clear(); this.pending.clear(); this.delivered.clear(); this.listener = undefined;
  }
}
export const exchangeNotices = new ExchangeNotices();
