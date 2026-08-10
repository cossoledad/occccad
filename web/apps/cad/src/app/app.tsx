import { DashboardOutlined, HomeOutlined, LogoutOutlined, SettingOutlined, UserOutlined } from "@ant-design/icons";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Avatar, Button, Dropdown, Layout, Space, Spin, Tag, Typography } from "antd";
import { lazy, Suspense, useState } from "react";
import { Navigate, Outlet, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { api, isMockMode } from "../api/client";
import { Brand } from "../components/brand";
import { AuthScreen } from "../features/auth/auth-screen";
import { DocumentCenter } from "../features/documents/document-center";
import { queryKeys } from "./query-keys";

const AdminDrawer = lazy(() => import("../features/admin/admin-drawer").then((module) => ({ default: module.AdminDrawer })));
const Workbench = lazy(() => import("../features/workbench/workbench").then((module) => ({ default: module.Workbench })));

function RouteLoading() {
  return <div className="application-loading"><Spin size="large" /></div>;
}

function ApplicationShell() {
  const location = useLocation();
  const navigate = useNavigate();
  const client = useQueryClient();
  const [adminOpen, setAdminOpen] = useState(false);
  const session = useQuery({ queryKey: queryKeys.session, queryFn: api.session, retry: false });
  const health = useQuery({ queryKey: queryKeys.health, queryFn: api.health, retry: false, refetchInterval: 30_000,
    enabled: Boolean(session.data) });

  if (session.isLoading) return <div className="application-loading"><Brand /><Spin size="large" /></div>;
  if (!session.data) return <AuthScreen />;
  const user = session.data.user;
  const inWorkbench = location.pathname.startsWith("/documents/");
  const logout = async () => { await api.logout(); client.clear(); navigate("/"); };

  return <Layout className="application-shell">
    <Layout.Header className="global-header">
      <button className="brand-button" onClick={() => navigate("/")}><Brand /></button>
      {inWorkbench && <><span className="header-divider" /><Typography.Text className="header-context"><DashboardOutlined /> CAD Workbench</Typography.Text></>}
      <div className="global-header-spacer" />
      <Space size="middle">
        <Tag color={isMockMode ? "gold" : health.isSuccess ? "green" : "red"}>{isMockMode ? "MOCK" : health.isSuccess ? `OCCT ${health.data.occtVersion}` : "OFFLINE"}</Tag>
        {user.platformRole === "ADMIN" && <Button ghost icon={<SettingOutlined />} onClick={() => setAdminOpen(true)}>管理</Button>}
        <Dropdown menu={{ items: [
          { key: "home", icon: <HomeOutlined />, label: "文档中心", onClick: () => navigate("/") },
          { key: "logout", icon: <LogoutOutlined />, label: "退出登录", danger: true, onClick: () => void logout() },
        ] }}>
          <button className="user-menu"><Avatar icon={<UserOutlined />}>{user.displayName[0]}</Avatar><span><strong>{user.displayName}</strong><small>{user.platformRole}</small></span></button>
        </Dropdown>
      </Space>
    </Layout.Header>
    <Layout.Content className="application-content"><Outlet /></Layout.Content>
    {adminOpen && <Suspense fallback={null}><AdminDrawer open onClose={() => setAdminOpen(false)} /></Suspense>}
  </Layout>;
}

export function App() {
  return <Routes>
    <Route element={<ApplicationShell />}>
      <Route index element={<DocumentCenter />} />
      <Route path="documents/:documentID" element={<Suspense fallback={<RouteLoading />}><Workbench /></Suspense>} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Route>
  </Routes>;
}
