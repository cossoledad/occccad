import { BellOutlined, DownloadOutlined, FolderOpenOutlined, ReloadOutlined, StopOutlined } from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { App, Badge, Button, Drawer, Empty, Progress, Space, Spin, Tag, Typography } from "antd";
import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import { buildActivityFeed, type ActivityStatus } from "./activity-model";

const activeStates = new Set(["QUEUED", "RETRY_WAIT", "RUNNING"]);
const statusPresentation: Record<ActivityStatus, { color: string; label: string }> = {
  PENDING: { color: "default", label: "等待中" }, RUNNING: { color: "processing", label: "进行中" },
  SUCCEEDED: { color: "success", label: "已完成" }, FAILED: { color: "error", label: "失败" },
  CANCELED: { color: "default", label: "已取消" },
};

export function ActivityCenter() {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const client = useQueryClient();
  const { message } = App.useApp();
  const jobs = useQuery({ queryKey: queryKeys.jobs, queryFn: api.listJobs,
    refetchInterval: (query) => query.state.data?.some((job) => activeStates.has(job.state)) ? 2_500 : false });
  const items = useMemo(() => buildActivityFeed(jobs.data ?? []), [jobs.data]);
  const activeCount = jobs.data?.filter((job) => activeStates.has(job.state)).length ?? 0;
  const refresh = async () => { await client.invalidateQueries({ queryKey: queryKeys.jobs }); };
  const cancel = useMutation({ mutationFn: api.cancelJob, onSuccess: refresh,
    onError: (error) => message.error(error.message) });
  const retry = useMutation({ mutationFn: api.retryJob, onSuccess: refresh,
    onError: (error) => message.error(error.message) });

  return <>
    <Badge count={activeCount} size="small" offset={[-2, 3]}>
      <Button ghost aria-label="打开消息中心" icon={<BellOutlined />} onClick={() => setOpen(true)} />
    </Badge>
    <Drawer title={<Space><BellOutlined /><span>消息中心</span></Space>} placement="right" width={420} open={open}
      onClose={() => setOpen(false)} extra={<Button type="text" icon={<ReloadOutlined />} onClick={() => void jobs.refetch()} />}
      className="activity-center">
      {jobs.isLoading ? <div className="activity-center-loading"><Spin /></div> : items.length === 0
        ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无消息" />
        : <div className="activity-list">{items.map((item) => {
          const status = statusPresentation[item.status];
          const busy = (cancel.isPending && cancel.variables === item.resourceId) || (retry.isPending && retry.variables === item.resourceId);
          return <section className={`activity-item activity-${item.status.toLowerCase()}`} key={item.id}>
            <div className="activity-item-heading"><div><Typography.Text strong>{item.title}</Typography.Text>
              <Typography.Text type="secondary" className="activity-time">{new Date(item.createdAt).toLocaleString()}</Typography.Text></div>
              <Tag color={status.color}>{status.label}</Tag></div>
            <Typography.Paragraph type={item.status === "FAILED" ? "danger" : "secondary"}>{item.description}</Typography.Paragraph>
            {item.progress !== undefined && <Progress percent={item.progress} size="small"
              status={item.canceling ? "exception" : item.status === "RUNNING" ? "active" : "normal"} />}
            {item.actions.length > 0 && <Space wrap>
              {item.actions.includes("DOWNLOAD") && <Button type="primary" size="small" icon={<DownloadOutlined />}
                onClick={() => void api.downloadJob(item.resourceId).catch((error) => message.error(error.message))}>下载</Button>}
              {item.actions.includes("OPEN_DOCUMENT") && <Button type="primary" size="small" icon={<FolderOpenOutlined />}
                onClick={() => { setOpen(false); navigate(`/documents/${item.documentId}`); }}>打开文档</Button>}
              {item.actions.includes("CANCEL") && <Button size="small" danger icon={<StopOutlined />} loading={busy}
                onClick={() => cancel.mutate(item.resourceId)}>取消</Button>}
              {item.actions.includes("RETRY") && <Button size="small" icon={<ReloadOutlined />} loading={busy}
                onClick={() => retry.mutate(item.resourceId)}>重试</Button>}
            </Space>}
          </section>;
        })}</div>}
    </Drawer>
  </>;
}
