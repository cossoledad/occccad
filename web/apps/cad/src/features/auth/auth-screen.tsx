import { LockOutlined, MailOutlined, UserOutlined } from "@ant-design/icons";
import { Alert, Button, Form, Input, Typography, message } from "antd";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, isMockMode } from "../../api/client";
import { Brand } from "../../components/brand";
import { queryKeys } from "../../app/query-keys";
import "./auth.css";

type LoginValues = { email: string; password: string };
type RegisterValues = LoginValues & { displayName: string };

const examples = [
  { image: "impeller.png", name: "径流叶轮", features: "旋转 · 放样 · 圆周阵列" },
  { image: "enclosure.png", name: "过渡壳体", features: "多截面放样 · 抽壳" },
  { image: "assembly.png", name: "法兰联轴器", features: "多零件 · 装配约束" },
];

export function AuthScreen() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [example, setExample] = useState(0);
  const selected = examples[example];
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
      <figure className="auth-showcase">
        <img key={selected.image} src={`${import.meta.env.BASE_URL}showcase/${selected.image}`}
          alt={`在 occccad 中建模的${selected.name}`} fetchPriority="high" />
        <figcaption><div><span className="showcase-kicker">MADE IN OCCCCAD</span><h2>{selected.name}</h2><p>{selected.features}</p></div>
          <div className="showcase-pagination" role="group" aria-label="建模示例">
            {examples.map((item, index) => <button key={item.image} type="button" aria-label={item.name}
              aria-pressed={index === example} onClick={() => setExample(index)}><span /></button>)}
          </div>
        </figcaption>
      </figure>
    </section>
    <section className="auth-panel">
      <div className="auth-card">
        <Brand compact />
        <Typography.Title level={2}>{mode === "login" ? "登录" : "注册"}</Typography.Title>
        {isMockMode && <span className="auth-mock">Mock</span>}
        {mode === "login" ? <Form<LoginValues> key="login" layout="vertical" requiredMark={false} size="large"
          initialValues={isMockMode ? { email: "admin@occccad.local", password: "mock-password" } : undefined}
          onFinish={(values) => login.mutate(values)}>
          <Form.Item name="email" label="电子邮箱" rules={[{ required: true, message: "请输入邮箱" }, { type: "email", message: "请输入有效邮箱" }]}>
            <Input prefix={<MailOutlined />} autoComplete="username" placeholder="you@example.com" />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, message: "请输入密码" }]}>
            <Input.Password prefix={<LockOutlined />} autoComplete="current-password" />
          </Form.Item>
          {login.error && <Alert type="error" showIcon message={login.error.message} />}
          <Button block type="primary" htmlType="submit" loading={login.isPending}>登录</Button>
        </Form> : <Form<RegisterValues> key="register" layout="vertical" requiredMark={false} size="large" onFinish={(values) => register.mutate(values)}>
          <Form.Item name="displayName" label="显示名称" rules={[{ required: true, message: "请输入名称" }]}><Input prefix={<UserOutlined />} autoComplete="nickname" /></Form.Item>
          <Form.Item name="email" label="电子邮箱" rules={[{ required: true, message: "请输入邮箱" }, { type: "email", message: "请输入有效邮箱" }]}><Input prefix={<MailOutlined />} autoComplete="username" placeholder="you@example.com" /></Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, min: 10, message: "密码至少 10 位" }]}><Input.Password prefix={<LockOutlined />} autoComplete="new-password" placeholder="至少 10 位" /></Form.Item>
          {register.error && <Alert type="error" showIcon message={register.error.message} />}
          <Button block type="primary" htmlType="submit" loading={register.isPending}>提交申请</Button>
        </Form>}
        <div className="auth-switch">{mode === "login" ? "还没有账号？" : "已有账号？"}<Button type="link" onClick={() => { login.reset(); register.reset(); setMode(mode === "login" ? "register" : "login"); }}>{mode === "login" ? "注册" : "登录"}</Button></div>
      </div>
      <span className="auth-footer">Open-source parametric CAD</span>
    </section>
  </main>;
}
