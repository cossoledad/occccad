import { LockOutlined, MailOutlined, UserOutlined } from "@ant-design/icons";
import { Alert, Button, Card, Form, Input, Segmented, Typography, message } from "antd";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, isMockMode } from "../../api/client";
import { Brand } from "../../components/brand";
import { queryKeys } from "../../app/query-keys";

type LoginValues = { email: string; password: string };
type RegisterValues = LoginValues & { displayName: string };

export function AuthScreen() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const client = useQueryClient();
  const [messageApi, context] = message.useMessage();
  const login = useMutation({ mutationFn: (values: LoginValues) => api.login(values.email, values.password),
    onSuccess: async () => { await client.invalidateQueries({ queryKey: queryKeys.session }); } });
  const register = useMutation({ mutationFn: (values: RegisterValues) => api.register(values.email, values.displayName, values.password),
    onSuccess: () => { messageApi.success("注册申请已提交，请等待管理员审批"); setMode("login"); } });

  return <main className="auth-page">
    {context}
    <section className="auth-hero">
      <Brand />
      <div className="auth-grid-decoration" />
    </section>
    <section className="auth-panel">
      <Card className="auth-card" bordered={false}>
        <Brand compact />
        <Typography.Title level={2}>{mode === "login" ? "登录" : "注册"}</Typography.Title>
        {isMockMode && <Alert showIcon type="info" message="Mock" />}
        <Segmented block value={mode} onChange={(value) => setMode(value as typeof mode)}
          options={[{ label: "登录", value: "login" }, { label: "注册", value: "register" }]} />
        {mode === "login" ? <Form<LoginValues> layout="vertical" requiredMark={false}
          initialValues={isMockMode ? { email: "admin@occccad.local", password: "mock-password" } : undefined}
          onFinish={(values) => login.mutate(values)}>
          <Form.Item name="email" label="电子邮箱" rules={[{ required: true }, { type: "email" }]}>
            <Input prefix={<MailOutlined />} autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true }]}>
            <Input.Password prefix={<LockOutlined />} autoComplete="current-password" />
          </Form.Item>
          {login.error && <Alert type="error" showIcon message={login.error.message} />}
          <Button block type="primary" htmlType="submit" loading={login.isPending}>登录</Button>
        </Form> : <Form<RegisterValues> layout="vertical" requiredMark={false} onFinish={(values) => register.mutate(values)}>
          <Form.Item name="displayName" label="显示名称" rules={[{ required: true }]}><Input prefix={<UserOutlined />} /></Form.Item>
          <Form.Item name="email" label="电子邮箱" rules={[{ required: true }, { type: "email" }]}><Input prefix={<MailOutlined />} /></Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, min: 10 }]}><Input.Password prefix={<LockOutlined />} /></Form.Item>
          {register.error && <Alert type="error" showIcon message={register.error.message} />}
          <Button block type="primary" htmlType="submit" loading={register.isPending}>提交申请</Button>
        </Form>}
      </Card>
    </section>
  </main>;
}
