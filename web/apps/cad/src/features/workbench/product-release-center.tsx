import { CheckCircleOutlined, SafetyCertificateOutlined } from "@ant-design/icons";
import { Alert, App, Button, Drawer, Empty, Input, List, Space, Spin, Tag, Typography } from "antd";
import { useState } from "react";
import type { ProductRelease } from "../../types";

export function ProductReleaseCenter({ open, releases, loading, creating, onClose, onCreate, onReplay }: {
  open: boolean; releases: ProductRelease[]; loading?: boolean; creating?: boolean;
  onClose: () => void; onCreate: (name: string) => Promise<void>; onReplay: (release: ProductRelease) => Promise<void>;
}) {
  const [name, setName] = useState("");
  const { message } = App.useApp();
  const create = async () => {
    try { await onCreate(name.trim()); setName(""); }
    catch (cause) { message.error(cause instanceof Error ? cause.message : String(cause)); }
  };
  const replay = async (release: ProductRelease) => {
    try { await onReplay(release); }
    catch (cause) { message.error(cause instanceof Error ? cause.message : String(cause)); }
  };
  return <Drawer open={open} width={620} title="产品版本中心" onClose={onClose} destroyOnHidden>
    <section className="product-release-hero">
      <SafetyCertificateOutlined />
      <div><Typography.Title level={4}>冻结可重放的产品里程碑</Typography.Title>
        <Typography.Paragraph type="secondary">发布版本保存完整 occurrence、关联输入、几何求值与装配求解证据；文件交换属于独立工作流。</Typography.Paragraph></div>
    </section>
    <Alert type="info" showIcon message="发布前自动执行完整性 Gate"
      description="引用必须为 CURRENT、occurrence 求值 READY、装配约束 VERIFIED，并具有成功的 SolveManifest。" />
    <Space.Compact block style={{ margin: "18px 0 24px" }}>
      <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="例如 ToyCar V1" maxLength={120}
        onPressEnter={() => { if (name.trim()) void create(); }} />
      <Button type="primary" loading={creating} disabled={!name.trim()}
        onClick={() => void create()}>发布新版本</Button>
    </Space.Compact>
    <Typography.Title level={5}>不可变版本</Typography.Title>
    {loading ? <Spin /> : releases.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚无发布版本" />
      : <List dataSource={releases} renderItem={(release) => <List.Item actions={[
        <Button key="replay" size="small" onClick={() => void replay(release)}>验证可重放性</Button>,
      ]}>
        <List.Item.Meta avatar={<CheckCircleOutlined className="product-release-ok" />}
          title={<Space><strong>{release.name}</strong><Tag color="success">已冻结</Tag></Space>}
          description={<Space direction="vertical" size={1}>
            <Typography.Text type="secondary">{new Date(release.createdAt).toLocaleString()}</Typography.Text>
            <Typography.Text code copyable={{ text: release.manifest.digest }}>{release.manifest.digest.slice(0, 16)}</Typography.Text>
            <Typography.Text type="secondary">{release.manifest.gates.length} 项 Gate · Root Revision {release.manifest.rootProductRevisionId.slice(0, 12)}</Typography.Text>
          </Space>} />
      </List.Item>} />}
  </Drawer>;
}
