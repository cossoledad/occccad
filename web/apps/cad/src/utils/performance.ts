export type ClientPerformanceSample = {
  name: string; durationMs: number; status: number; serverTiming: string; at: string;
};

const samples: ClientPerformanceSample[] = [];
const capacity = 200;
const listeners = new Set<() => void>();

export function subscribeClientPerformance(listener: () => void): () => void {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}

export function lastClientPerformanceSample(): ClientPerformanceSample | undefined {
  return samples.at(-1);
}

// Wall time for one transport request, including response body processing,
// connection wait, failures and cancellation. No server CPU claim.
export async function measureClientRequest<T>(name: string, action: () => Promise<T>): Promise<T> {
  const started = performance.now();
  let status = 0;
  try {
    const result = await action();
    status = 200;
    return result;
  } catch (error) {
    if (error && typeof error === "object" && "status" in error && typeof error.status === "number") status = error.status;
    throw error;
  } finally {
    recordClientPerformance({ name, durationMs: performance.now() - started, status, serverTiming: "", at: new Date().toISOString() });
  }
}

export function recordClientPerformance(sample: ClientPerformanceSample): void {
  samples.push(sample);
  if (samples.length > capacity) samples.splice(0, samples.length - capacity);
  for (const listener of listeners) listener();
  if (typeof window !== "undefined" && window.dispatchEvent && typeof CustomEvent !== "undefined")
    window.dispatchEvent(new CustomEvent("occccad:performance", { detail: sample }));
}

export function clientPerformanceSnapshot(): ClientPerformanceSample[] {
  return samples.map((sample) => ({ ...sample }));
}
