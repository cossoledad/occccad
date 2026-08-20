import type { Job } from "../../types";

export type ActivityStatus = "PENDING" | "RUNNING" | "SUCCEEDED" | "FAILED" | "CANCELED";
export type ActivityAction = "CANCEL" | "RETRY" | "DOWNLOAD" | "OPEN_DOCUMENT";

export type ActivityItem = {
  id: string;
  source: string;
  sourceType: string;
  resourceId: string;
  documentId?: string;
  title: string;
  description: string;
  status: ActivityStatus;
  progress?: number;
  createdAt: string;
  completedAt?: string;
  canceling?: boolean;
  actions: ActivityAction[];
};

type JobPresentation = {
  title: string;
  running: string;
  succeeded: string;
  failed: string;
};

const jobPresentations: Record<string, JobPresentation> = {
  EXCHANGE_IMPORT: { title: "文档导入", running: "正在解析并创建文档", succeeded: "文档已导入", failed: "文档导入失败" },
  EXCHANGE_EXPORT: { title: "文档导出", running: "正在生成交换文件", succeeded: "文件已可下载", failed: "文档导出失败" },
};

const fallbackPresentation: JobPresentation = {
  title: "后台任务", running: "任务正在处理", succeeded: "任务已完成", failed: "任务执行失败",
};

export function activityStatus(job: Job): ActivityStatus {
  if (job.state === "QUEUED" || job.state === "RETRY_WAIT") return "PENDING";
  return job.state;
}

export function projectJobActivity(job: Job): ActivityItem {
  const presentation = jobPresentations[job.type] ?? fallbackPresentation;
  const fileName = typeof job.payload.fileName === "string" ? job.payload.fileName : "";
  const status = activityStatus(job);
  let description = status === "SUCCEEDED" ? presentation.succeeded
    : status === "FAILED" ? (job.errorMessage || presentation.failed)
      : status === "CANCELED" ? "任务已取消"
        : job.cancelRequestedAt ? "正在取消任务" : presentation.running;
  if (fileName) description = `${fileName} · ${description}`;
  const actions: ActivityAction[] = [];
  if (job.canCancel) actions.push("CANCEL");
  if (job.canRetry) actions.push("RETRY");
  if (job.state === "SUCCEEDED" && job.type === "EXCHANGE_EXPORT" && job.resultObjectId) actions.push("DOWNLOAD");
  if (job.state === "SUCCEEDED" && job.type === "EXCHANGE_IMPORT" && job.documentId) actions.push("OPEN_DOCUMENT");
  return { id: `job:${job.id}`, source: "JOB", sourceType: job.type, resourceId: job.id, documentId: job.documentId,
    title: presentation.title,
    description, status, progress: status === "PENDING" || status === "RUNNING" ? job.progress : undefined,
    createdAt: job.createdAt, completedAt: job.completedAt, canceling: Boolean(job.cancelRequestedAt), actions };
}

// New durable message sources add their own projector and can be merged here;
// the drawer does not need to know their storage or transport representation.
export function buildActivityFeed(jobs: Job[]): ActivityItem[] {
  return jobs.map(projectJobActivity).sort((left, right) => right.createdAt.localeCompare(left.createdAt));
}
