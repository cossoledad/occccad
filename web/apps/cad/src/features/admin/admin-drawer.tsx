import { DeleteOutlined, KeyOutlined, PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { App, Button, Col, Drawer, Form, Input, Modal, Row, Select, Space, Statistic, Table, Tag } from "antd";
import { useState } from "react";
import { api } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import type { User } from "../../types";

export function AdminDrawer({ open, onClose }: { open: boolean; onClose: () => void }) {
  const client = useQueryClient();
  const { message, modal } = App.useApp();
  const [editing, setEditing] = useState<User>();
  const [creating, setCreating] = useState(false);
  const [resetting, setResetting] = useState<User>();
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [form] = Form.useForm();
  const [resetForm] = Form.useForm<{ password: string }>();
  const users = useQuery({ queryKey: [...queryKeys.adminUsers, query, status], queryFn: () => api.adminUsers(query, status), enabled: open });
  const stats = useQuery({ queryKey: queryKeys.adminStats, queryFn: api.adminStats, enabled: open });
  const save = useMutation({ mutationFn: async (values: { email: string; displayName: string; password?: string; platformRole: string; status: string }) => {
    if (editing) return api.adminUpdateUser(editing.id, values);
    return api.adminCreateUser({ ...values, password: values.password ?? "", platformRole: values.platformRole, status: values.status });
  }, onSuccess: async () => {
    message.success("账号已保存"); setEditing(undefined); setCreating(false); form.resetFields();
    await Promise.all([client.invalidateQueries({ queryKey: queryKeys.adminUsers }), client.invalidateQueries({ queryKey: queryKeys.adminStats })]);
  }});
  const resetPassword = useMutation({ mutationFn: (values: { password: string }) => api.adminResetPassword(resetting!.id, values.password),
    onSuccess: () => { message.success("密码已重置"); setResetting(undefined); resetForm.resetFields(); } });
  const disable = (user: User) => modal.confirm({ title: `禁用账号“${user.displayName}”？`, okButtonProps: { danger: true },
    onOk: async () => { await api.adminDisableUser(user.id); await client.invalidateQueries({ queryKey: queryKeys.adminUsers }); message.success("账号已禁用"); } });
  const showEditor = (user?: User) => {
    setEditing(user); setCreating(!user);
    form.setFieldsValue(user ?? { platformRole: "MEMBER", status: "ACTIVE" });
  };

  return <>
    <Drawer width={760} open={open} onClose={onClose} title="管理后台" extra={<Button icon={<ReloadOutlined />}
      onClick={() => void client.invalidateQueries({ queryKey: queryKeys.adminUsers })}>刷新</Button>}>
      <Row gutter={12} className="admin-statistics">
        <Col span={6}><Statistic title="全部账号" value={stats.data?.users ?? 0} /></Col>
        <Col span={6}><Statistic title="待审批" value={stats.data?.pending ?? 0} /></Col>
        <Col span={6}><Statistic title="活跃会话" value={stats.data?.activeSessions ?? 0} /></Col>
        <Col span={6}><Statistic title="设计文档" value={stats.data?.documents ?? 0} /></Col>
      </Row>
      <div className="drawer-toolbar"><Input.Search placeholder="搜索账号" allowClear value={query} onChange={(event) => setQuery(event.target.value)} />
        <Select value={status} onChange={setStatus} options={[{ value: "", label: "全部状态" }, { value: "PENDING", label: "待审批" }, { value: "ACTIVE", label: "已启用" }, { value: "DISABLED", label: "已禁用" }]} />
        <Button type="primary" icon={<PlusOutlined />} onClick={() => showEditor()}>添加账号</Button></div>
      <Table<User> rowKey="id" loading={users.isLoading} dataSource={users.data} pagination={false} columns={[
        { title: "账号", render: (_, user) => <span className="identity-cell"><strong>{user.displayName}</strong><small>{user.email}</small></span> },
        { title: "角色", dataIndex: "platformRole", render: (value) => <Tag color={value === "ADMIN" ? "blue" : "default"}>{value}</Tag> },
        { title: "状态", dataIndex: "status", render: (value) => <Tag color={value === "ACTIVE" ? "green" : value === "PENDING" ? "gold" : "red"}>{value}</Tag> },
        { title: "操作", width: 220, render: (_, user) => <Space size={0}><Button type="link" onClick={() => showEditor(user)}>{user.status === "PENDING" ? "审批" : "编辑"}</Button>
          <Button type="link" icon={<KeyOutlined />} onClick={() => setResetting(user)}>重置密码</Button>
          {user.status !== "DISABLED" && <Button danger type="link" icon={<DeleteOutlined />} onClick={() => disable(user)}>禁用</Button>}</Space> },
      ]} />
    </Drawer>
    <Modal title={editing ? "编辑账号" : "创建账号"} open={creating || Boolean(editing)} onCancel={() => { setCreating(false); setEditing(undefined); }}
      onOk={() => form.submit()} confirmLoading={save.isPending} destroyOnHidden>
      <Form form={form} layout="vertical" onFinish={(values) => save.mutate(values)}>
        <Form.Item name="displayName" label="显示名称" rules={[{ required: true }]}><Input /></Form.Item>
        {!editing && <><Form.Item name="email" label="电子邮箱" rules={[{ required: true }, { type: "email" }]}><Input /></Form.Item>
          <Form.Item name="password" label="初始密码" rules={[{ required: true, min: 10 }]}><Input.Password /></Form.Item></>}
        <Row gutter={12}><Col span={12}><Form.Item name="platformRole" label="平台角色"><Select options={[{ value: "MEMBER", label: "成员" }, { value: "ADMIN", label: "管理员" }]} /></Form.Item></Col>
          <Col span={12}><Form.Item name="status" label="账号状态"><Select options={[{ value: "ACTIVE", label: "已启用" }, { value: "PENDING", label: "待审批" }, { value: "DISABLED", label: "已禁用" }]} /></Form.Item></Col></Row>
      </Form>
    </Modal>
    <Modal title={`重置密码 · ${resetting?.displayName ?? ""}`} open={Boolean(resetting)} onCancel={() => setResetting(undefined)}
      onOk={() => resetForm.submit()} confirmLoading={resetPassword.isPending} destroyOnHidden>
      <Form form={resetForm} layout="vertical" onFinish={(values) => resetPassword.mutate(values)}>
        <Form.Item name="password" label="新密码" rules={[{ required: true, min: 10 }]}><Input.Password autoFocus /></Form.Item>
      </Form>
    </Modal>
  </>;
}
