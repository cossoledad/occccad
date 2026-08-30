export type ClientPerformanceSample = {
  name: string; durationMs: number; status: number; serverTiming: string; at: string;
};

const samples: ClientPerformanceSample[] = [];
const capacity = 200;

export function recordClientPerformance(sample: ClientPerformanceSample): void {
  samples.push(sample);
  if (samples.length > capacity) samples.splice(0, samples.length - capacity);
  window.dispatchEvent(new CustomEvent("occccad:performance", { detail: sample }));
}

export function clientPerformanceSnapshot(): ClientPerformanceSample[] {
  return samples.map((sample) => ({ ...sample }));
}
