import { LogoutOutlined, QuestionCircleOutlined, SettingOutlined, UserOutlined } from "@ant-design/icons";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { App as AntApp, Avatar, Button, Dropdown, Layout, Spin } from "antd";
import { lazy, Suspense, useEffect, useState } from "react";
import { Navigate, Outlet, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { api, isMockMode } from "../api/client";
import { realtime } from "../api/realtime-client";
import { Brand } from "../components/brand";
import { AuthScreen } from "../features/auth/auth-screen";
import { ActivityCenter } from "../features/activity/activity-center";
import { DocumentCenter } from "../features/documents/document-center";
import { DocumentOrbController } from "../features/workbench/document-orb-controller";
import { queryKeys } from "./query-keys";
import { UIHelpProvider, useUIHelp } from "../cad/help/ui-help-context";

const AdminDrawer = lazy(() => import("../features/admin/admin-drawer").then((module) => ({ default: module.AdminDrawer })));
const Workbench = lazy(() => import("../features/workbench/workbench").then((module) => ({ default: module.Workbench })));

function RouteLoading() {
  return <div className="application-loading"><Spin size="large" /></div>;
}

function ApplicationShell() {
	const uiHelp = useUIHelp();
  const location = useLocation();
  const navigate = useNavigate();
  const client = useQueryClient();
  const { message, notification } = AntApp.useApp();
  const [adminOpen, setAdminOpen] = useState(false);
  const session = useQuery({ queryKey: queryKeys.session, queryFn: api.session, retry: false });
  const health = useQuery({ queryKey: queryKeys.health, queryFn: api.health, retry: false, refetchInterval: 30_000,
    enabled: Boolean(session.data) });
  useEffect(() => {
    if (!session.data || isMockMode) return;
    const unsubscribe = realtime.onJobEvent((job) => {
      if (job.type !== "EXCHANGE_IMPORT" && job.type !== "EXCHANGE_EXPORT") return;
      void client.invalidateQueries({ queryKey: queryKeys.jobs });
      if (job.type === "EXCHANGE_IMPORT") void client.invalidateQueries({ queryKey: ["documents"] });
      if (job.state === "SUCCEEDED" && job.type === "EXCHANGE_IMPORT") {
        notification.success({ key: job.id, message: "文档导入完成", description: "导入结果已写入文档中心。",
          btn: job.documentId ? <Button type="primary" onClick={() => { notification.destroy(job.id); navigate(`/documents/${job.documentId}`); }}>打开文档</Button> : undefined });
      } else if (job.state === "SUCCEEDED" && job.type === "EXCHANGE_EXPORT") {
        notification.success({ key: job.id, message: "文档导出完成", description: "交换文件已经生成，可以开始下载。",
          btn: <Button type="primary" onClick={() => void api.downloadJob(job.id).catch((error) => message.error(error.message))}>下载文件</Button> });
      } else if (job.state === "FAILED") {
        notification.error({ key: job.id, message: job.type === "EXCHANGE_IMPORT" ? "文档导入失败" : "文档导出失败",
          description: job.errorMessage ?? "后台任务执行失败" });
      }
    });
    realtime.start();
    return () => { unsubscribe(); realtime.stop(); };
  }, [client, message, navigate, notification, session.data]);

  if (session.isLoading) return <div className="application-loading"><Brand /><Spin size="large" /></div>;
  if (!session.data) return <AuthScreen />;
  const user = session.data.user;
  const inWorkbench = location.pathname.startsWith("/documents/");
  const logout = async () => { realtime.stop(); await api.logout(); client.clear(); navigate("/"); };

  return <Layout className="application-shell">
    <Layout.Header className="global-header">
      <button className="brand-button" aria-label="文档中心" onClick={() => navigate("/")}><Brand /></button>
      <div className="global-header-spacer" />
      <div className="global-header-actions">
        <i role="status" aria-label={isMockMode ? "Mock" : health.isSuccess ? `OCCT ${health.data.occtVersion}` : "离线"}
          className={`service-state ${isMockMode ? "mock" : health.isSuccess ? "online" : "offline"}`} />
        <ActivityCenter />
        {user.platformRole === "ADMIN" && <Button ghost aria-label="管理" icon={<SettingOutlined />} onClick={() => setAdminOpen(true)} />}
        {inWorkbench && <Button ghost className={`context-help-button ${uiHelp.active ? "active" : ""}`}
          aria-label="这是什么？" aria-pressed={uiHelp.active} icon={<QuestionCircleOutlined />} onClick={uiHelp.toggle} />}
        <Dropdown menu={{ items: [
          { key: "logout", icon: <LogoutOutlined />, label: "退出登录", danger: true, onClick: () => void logout() },
        ] }}>
          <button className="user-menu" aria-label={user.displayName}>
            <Avatar icon={<UserOutlined />}>{user.displayName[0]}</Avatar>
          </button>
        </Dropdown>
      </div>
    </Layout.Header>
    <Layout.Content className="application-content"><Outlet /></Layout.Content>
    {inWorkbench && <DocumentOrbController />}
    {adminOpen && <Suspense fallback={null}><AdminDrawer open onClose={() => setAdminOpen(false)} /></Suspense>}
  </Layout>;
}

function GlobalBrowserInteractionPolicy() {
  useEffect(() => {
    const suppressNativeContextMenu = (event: MouseEvent) => event.preventDefault();
    document.addEventListener("contextmenu", suppressNativeContextMenu, { capture: true });
    return () => document.removeEventListener("contextmenu", suppressNativeContextMenu, { capture: true });
  }, []);
  return null;
}

export function App() {
  return <UIHelpProvider><GlobalBrowserInteractionPolicy /><Routes>
    <Route element={<ApplicationShell />}>
      <Route index element={<DocumentCenter />} />
      <Route path="documents/:documentID" element={<Suspense fallback={<RouteLoading />}><Workbench /></Suspense>} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Route>
  </Routes></UIHelpProvider>;
}
