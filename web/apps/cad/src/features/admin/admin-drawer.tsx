import { ArrowLeftOutlined, DeleteOutlined, KeyOutlined, MoreOutlined, PlusOutlined, ReloadOutlined, SearchOutlined } from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, App, Button, Col, Drawer, Dropdown, Form, Input, Row, Select, Statistic, Table, Tag } from "antd";
import { useEffect, useState } from "react";
import { engineeringProblem, useOperationFeedback } from "../../cad/command/operation-feedback";
import { api } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import type { User } from "../../types";
import "./admin.css";

export function AdminDrawer({ open, onClose }: { open: boolean; onClose: () => void }) {
  const client = useQueryClient();
  const { message, modal } = App.useApp();
  const feedback = useOperationFeedback();
  const [editing, setEditing] = useState<User>();
  const [creating, setCreating] = useState(false);
  const [resetting, setResetting] = useState<User>();
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [form] = Form.useForm();
  const [resetForm] = Form.useForm<{ password: string }>();
  useEffect(() => { const timer = window.setTimeout(() => setSearch(query.trim()), 250); return () => window.clearTimeout(timer); }, [query]);
  const users = useQuery({ queryKey: [...queryKeys.adminUsers, search, status], queryFn: () => api.adminUsers(search, status), enabled: open });
  const stats = useQuery({ queryKey: queryKeys.adminStats, queryFn: api.adminStats, enabled: open });
  const refresh = async () => { await Promise.all([client.invalidateQueries({ queryKey: queryKeys.adminUsers }), client.invalidateQueries({ queryKey: queryKeys.adminStats })]); };
  const save = useMutation({ mutationFn: async (values: { email: string; displayName: string; password?: string; platformRole: string; status: string }) => {
    if (editing) return api.adminUpdateUser(editing.id, values);
    return api.adminCreateUser({ ...values, password: values.password ?? "", platformRole: values.platformRole, status: values.status });
  }, onSuccess: async () => {
    setEditing(undefined); setCreating(false); form.resetFields(); await refresh();
  }, onError: error => feedback(error, "保存账号") });
  const approve = useMutation({ mutationFn: (user: User) => api.adminUpdateUser(user.id, { displayName: user.displayName, platformRole: user.platformRole ?? "MEMBER", status: "ACTIVE" }),
    onSuccess: refresh, onError: error => feedback(error, "批准账号") });
  const resetPassword = useMutation({ mutationFn: (values: { password: string }) => api.adminResetPassword(resetting!.id, values.password),
    onSuccess: () => { message.success("密码已重置"); setResetting(undefined); resetForm.resetFields(); }, onError: error => feedback(error, "重置密码") });
  const disable = (user: User) => modal.confirm({ title: `禁用账号“${user.displayName}”？`, okButtonProps: { danger: true },
    okText: "禁用", onOk: async () => { try { await api.adminDisableUser(user.id); await refresh(); } catch (error) { feedback(error, "禁用账号"); throw error; } } });
  const showEditor = (user?: User) => {
    setEditing(user); setCreating(!user);
    form.resetFields(); form.setFieldsValue(user ?? { platformRole: "MEMBER", status: "ACTIVE" });
  };

  const editorOpen=creating||Boolean(editing)||Boolean(resetting);
  const pending=save.isPending||resetPassword.isPending;
  const closeEditor=()=>{if(pending)return;setCreating(false);setEditing(undefined);setResetting(undefined);form.resetFields();resetForm.resetFields()};
  return <>
    <Drawer size="min(880px, 100vw)" className="admin-drawer" open={open} onClose={()=>{if(pending)return;closeEditor();onClose()}} closable={!pending}
      title={editorOpen?<span className="admin-editor-title"><Button type="text" aria-label="返回用户列表" disabled={pending} icon={<ArrowLeftOutlined/>} onClick={closeEditor}/>{resetting?`重置密码 · ${resetting.displayName}`:editing?"编辑账号":"添加账号"}</span>:"用户管理"}
      extra={!editorOpen&&<Button type="text" aria-label="刷新账号" icon={<ReloadOutlined spin={users.isFetching}/>} onClick={()=>void refresh()}/>}
      footer={editorOpen&&<div className="admin-editor-actions"><Button disabled={pending} onClick={closeEditor}>取消</Button><Button type="primary" loading={pending} onClick={()=>resetting?resetForm.submit():form.submit()}>{resetting?"重置密码":editing?"保存":"创建"}</Button></div>}>
      <div hidden={editorOpen}>
      <div className="admin-statistics">
        {[{ key: "users", title: "全部账号" }, { key: "pending", title: "待审批" }, { key: "activeSessions", title: "活跃会话" }, { key: "documents", title: "设计文档" }].map(item =>
          <Statistic key={item.key} title={item.title} value={stats.data?.[item.key as keyof NonNullable<typeof stats.data>] ?? "—"} />)}
      </div>
      {(users.error || stats.error) && <Alert showIcon type="error" title="无法加载管理信息" description={engineeringProblem(users.error ?? stats.error)} action={<Button onClick={() => void refresh()}>重试</Button>} />}
      <div className="drawer-toolbar"><Input prefix={<SearchOutlined />} placeholder="搜索账号" aria-label="搜索账号" allowClear value={query} onChange={(event) => setQuery(event.target.value)} />
        <Select aria-label="账号状态筛选" value={status} onChange={setStatus} options={[{ value: "", label: "全部状态" }, { value: "PENDING", label: "待审批" }, { value: "ACTIVE", label: "已启用" }, { value: "DISABLED", label: "已禁用" }]} />
        <Button type="primary" aria-label="添加账号" icon={<PlusOutlined />} onClick={() => showEditor()}>添加账号</Button></div>
      <Table<User> rowKey="id" loading={users.isLoading} dataSource={users.data} pagination={{ pageSize: 10, hideOnSinglePage: true, showSizeChanger: false }} scroll={{ x: 620 }} locale={{ emptyText: "没有找到账号" }} columns={[
        { title: "账号", render: (_, user) => <span className="identity-cell"><strong>{user.displayName}</strong><small>{user.email}</small></span> },
        { title: "角色", dataIndex: "platformRole", width: 100, render: (value) => <span className="admin-role">{value === "ADMIN" ? "管理员" : "成员"}</span> },
        { title: "状态", dataIndex: "status", width: 100, render: (value) => <Tag className={`account-status ${value.toLowerCase()}`}>{value === "ACTIVE" ? "已启用" : value === "PENDING" ? "待审批" : "已禁用"}</Tag> },
        { title: "", width: 120, fixed: "right", render: (_, user) => <div className="admin-row-actions">{user.status === "PENDING"
          ? <Button type="link" loading={approve.isPending && approve.variables?.id === user.id} onClick={() => approve.mutate(user)}>批准</Button>
          : <Button type="link" onClick={() => showEditor(user)}>编辑</Button>}
          <Dropdown trigger={["click"]} menu={{ items: [
            ...(user.status === "PENDING" ? [{ key: "edit", label: "编辑账号", onClick: () => showEditor(user) }] : []),
            { key: "reset", label: "重置密码", icon: <KeyOutlined />, onClick: () => { resetForm.resetFields(); setResetting(user); } },
            ...(user.status !== "DISABLED" ? [{ key: "disable", label: "禁用账号", danger: true, icon: <DeleteOutlined />, onClick: () => disable(user) }] : []),
          ] }}><Button type="text" aria-label={`账号操作 ${user.displayName}`} icon={<MoreOutlined />} /></Dropdown></div> },
      ]} />
      </div>
      {(creating||editing)&&<div className="admin-editor"><Form disabled={pending} form={form} layout="vertical" requiredMark={false} onFinish={(values) => save.mutate(values)}>
        <Form.Item name="displayName" label="显示名称" rules={[{ required: true }]}><Input autoFocus /></Form.Item>
        {!editing && <><Form.Item name="email" label="电子邮箱" rules={[{ required: true }, { type: "email" }]}><Input /></Form.Item>
          <Form.Item name="password" label="初始密码" rules={[{ required: true, min: 10, message: "密码至少 10 位" }]}><Input.Password autoComplete="new-password" placeholder="至少 10 位" /></Form.Item></>}
        <Row gutter={12}><Col span={12}><Form.Item name="platformRole" label="平台角色"><Select options={[{ value: "MEMBER", label: "成员" }, { value: "ADMIN", label: "管理员" }]} /></Form.Item></Col>
          <Col span={12}><Form.Item name="status" label="账号状态"><Select options={[{ value: "ACTIVE", label: "已启用" }, { value: "PENDING", label: "待审批" }, { value: "DISABLED", label: "已禁用" }]} /></Form.Item></Col></Row>
      </Form>
      </div>}
      {resetting&&<div className="admin-editor"><Form disabled={pending} form={resetForm} layout="vertical" requiredMark={false} onFinish={(values) => resetPassword.mutate(values)}>
        <Form.Item name="password" label="新密码" rules={[{ required: true, min: 10, message: "密码至少 10 位" }]}><Input.Password autoFocus autoComplete="new-password" placeholder="至少 10 位" /></Form.Item>
      </Form>
      </div>}
    </Drawer>
  </>;
}
