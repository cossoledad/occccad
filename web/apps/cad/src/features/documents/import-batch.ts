import type { Job } from "../../types";

export type ImportFile = { uid: string; file: File };
export type ImportFailure = { uid: string; name: string; reason: string };

/** Each file creates its own recoverable Job; failures do not cancel siblings. */
export async function submitImportBatch(
  files: readonly ImportFile[], submit: (file: File) => Promise<Job>, concurrency = 3,
  onProgress?: (finished: number, total: number) => void,
): Promise<{ submitted: Job[]; failures: ImportFailure[] }> {
  if (!Number.isInteger(concurrency) || concurrency < 1) throw new Error("无效的导入并发数");
  const results: Array<{ job?: Job; failure?: ImportFailure }> = new Array(files.length);
  let cursor = 0, finished = 0;
  const run = async () => {
    for (;;) {
      const index = cursor++;
      if (index >= files.length) return;
      const entry = files[index];
      try { results[index] = { job: await submit(entry.file) }; }
      catch (error) { results[index] = { failure: { uid: entry.uid, name: entry.file.name,
        reason: error instanceof Error ? error.message : String(error) } }; }
      onProgress?.(++finished, files.length);
    }
  };
  await Promise.all(Array.from({ length: Math.min(files.length, concurrency) }, run));
  return { submitted: results.flatMap((result) => result.job ? [result.job] : []),
    failures: results.flatMap((result) => result.failure ? [result.failure] : []) };
}
