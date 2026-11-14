import { queryOptions } from "@tanstack/react-query";
import { api } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import type { Job } from "../../types";

export const jobIsActive = (job: Pick<Job, "state">) => ["QUEUED", "RETRY_WAIT", "RUNNING"].includes(job.state);

/** Status events do not own the full result payload. Keep it until a resource
 * reconciliation and reject older attempts/completions. Events within an
 * attempt follow WebSocket order; reconnect reconciles the resource list. */
export function mergeJobEvent(jobs: Job[] | undefined, event: Omit<Job, "payload">): Job[] | undefined {
  if (!jobs) return jobs;
  const index = jobs.findIndex(job => job.id === event.id);
  if (index < 0) return jobs;
  const previous = jobs[index];
  if (event.attemptCount < previous.attemptCount || event.completedAt && previous.completedAt &&
    Date.parse(event.completedAt) < Date.parse(previous.completedAt)) return jobs;
  return jobs.map((job, i) => i === index ? {...job, ...event, payload: job.payload} : job);
}

// Realtime drives updates. An active task keeps a modest fallback for a lost
// event/connection; reconnect and returning to the page also reconcile the list.
export const jobsQueryOptions = () => queryOptions({
  queryKey: queryKeys.jobs, queryFn: api.listJobs,
  refetchInterval: query => query.state.data?.some(jobIsActive) ? 10_000 : false,
  refetchOnWindowFocus: true, refetchOnReconnect: "always",
});
