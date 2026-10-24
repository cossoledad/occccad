import {CommandDialog} from "../../cad/overlay/floating-panel";
import {engineeringProblem,useOperationFeedback} from "../../cad/command/operation-feedback";
import { CheckCircleOutlined, SafetyCertificateOutlined } from "@ant-design/icons";
import { Alert, Button, Empty, Input, List, Space, Spin, Tag, Typography } from "antd";
import { useState } from "react";
import type { ProductRelease } from "../../types";

export function ProductReleaseCenter({ open, releases, loading, creating, error, canCreate=true, onRetry, onClose, onCreate, onReplay }: {
  open: boolean; releases: ProductRelease[];error?:unknown;canCreate?:boolean;onRetry?:()=>void; loading?: boolean; creating?: boolean;
  onClose: () => void; onCreate: (name: string) => Promise<void>; onReplay: (release: ProductRelease) => Promise<void>;
}) {
  const [name, setName] = useState("");
  const feedback=useOperationFeedback();
  const create = async () => {
    try { await onCreate(name.trim()); setName(""); }
    catch (cause) { feedback(cause,"产品版本"); }
  };
  const replay = async (release: ProductRelease) => {
    try { await onReplay(release); }
    catch (cause) { feedback(cause,"产品版本"); }
  };
  return <CommandDialog id="product-release-center" open={open} title="产品版本中心" size="M" footer={false} onClose={onClose} onConfirm={create}>
    <section className="product-release-hero">
      <SafetyCertificateOutlined />
      <div><Typography.Title level={4}>保存产品里程碑</Typography.Title>
        <Typography.Paragraph type="secondary">保存当前产品及其引用的确切版本，可在以后重新检查。</Typography.Paragraph></div>
    </section>
    <Alert type="info" showIcon message="发布前检查模型完整性"
      description="发布要求引用保持最新、零件计算成功且装配约束已满足。" />
    <Space.Compact block style={{ margin: "18px 0 24px" }}>
      <Input value={name} disabled={!canCreate} onChange={(event) => setName(event.target.value)} placeholder="例如 ToyCar V1" maxLength={120}
        onPressEnter={() => { if (name.trim()) void create(); }} />
      <Button type="primary" loading={creating} disabled={!name.trim()||!canCreate}
        onClick={() => void create()}>发布新版本</Button>
    </Space.Compact>
    <Typography.Title level={5}>已发布版本</Typography.Title>
    {error?<Alert type="error" title="无法读取产品版本" description={engineeringProblem(error)} action={<Button onClick={onRetry}>重试</Button>}/>:loading ? <Spin /> : releases.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚无发布版本" />
      : <List dataSource={releases} renderItem={(release) => <List.Item actions={[
        <Button key="replay" size="small" onClick={() => void replay(release)}>检查版本</Button>,
      ]}>
        <List.Item.Meta avatar={<CheckCircleOutlined className="product-release-ok" />}
          title={<Space><strong>{release.name}</strong><Tag color="success">已冻结</Tag></Space>}
          description={<Space direction="vertical" size={1}>
            <Typography.Text type="secondary">{new Date(release.createdAt).toLocaleString()}</Typography.Text>


          </Space>} />
      </List.Item>} />}
  </CommandDialog>;
}
