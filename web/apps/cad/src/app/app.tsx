import {exchangeNotices} from "../features/activity/exchange-notices";
import {engineeringProblem} from "../cad/command/operation-failure";
import type {Job} from "../types";
import {mergeJobEvent} from "../features/activity/jobs-query";
import { LogoutOutlined, QuestionCircleOutlined, ToolOutlined, UserOutlined } from "@ant-design/icons";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { App as AntApp, Avatar, Button, Dropdown, Layout, Spin } from "antd";
import { lazy, Suspense, useEffect, useState } from "react";
import { Navigate, Outlet, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { api, isMockMode } from "../api/client";
import { realtime } from "../api/realtime-client";
import { Brand } from "../components/brand";
import { AuthScreen } from "../features/auth/auth-screen";
import { ActivityCenter } from "../features/activity/activity-center";
import { exchangeCompletionWarnings } from "../features/activity/activity-model";

import { DocumentTabsController } from "../features/workbench/document-tabs-controller";
import {GlobalOptionsProvider,GlobalOptionsButton} from "../features/preferences/global-options-center";
import { queryKeys } from "./query-keys";
import { UIHelpProvider, useUIHelp } from "../cad/help/ui-help-context";
import { useApplicationContext } from "../state/application-context";
import { DocumentSessionProvider } from "../cad/document/document-session-context";

const DocumentCenter = lazy(() => import("../features/documents/document-center").then(module => ({default: module.DocumentCenter})));
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
  const activeWorkbenchDocumentID = useApplicationContext((state) => state.activeDocumentID);
  const session = useQuery({ queryKey: queryKeys.session, queryFn: api.session, retry: false });
  const health = useQuery({ queryKey: queryKeys.health, queryFn: api.health, retry: false, refetchInterval: 30_000,
    enabled: Boolean(session.data) });
  useEffect(() => {
    if (!session.data || isMockMode) return;
    const stopNotices=exchangeNotices.listen(jobs=>{
      const failed=jobs.filter(job=>job.state==='FAILED');
      const warnings=jobs.flatMap(job=>exchangeCompletionWarnings(job).map(warning=>`${job.fileName??'文件'}：${warning}`));
      const first=jobs[0],single=jobs.length===1;
      const title=single?(failed.length?`${first.type==='EXCHANGE_IMPORT'?'导入':'导出'}失败`:first.state==='CANCELED'?'任务已取消':`${first.type==='EXCHANGE_IMPORT'?'导入':'导出'}完成${warnings.length?'，有警告':''}`):`导入完成 · ${jobs.filter(job=>job.state==='SUCCEEDED').length}/${jobs.length}`;
      const key=single?first.id:`import-batch:${first.id}`;
      const action=single&&first.state==='SUCCEEDED'?(first.type==='EXCHANGE_IMPORT'&&first.documentId?<Button type="primary" onClick={()=>{notification.destroy(key);navigate(`/documents/${first.documentId}`)}}>打开文档</Button>:first.type==='EXCHANGE_EXPORT'?<Button type="primary" onClick={()=>void api.downloadJob(first.id).catch(error=>message.error(engineeringProblem(error)))}>下载文件</Button>:undefined):undefined;
      notification[failed.length?'error':warnings.length?'warning':'success']({key,title,className:'exchange-completion-notice',
        description:<div>{single&&first.fileName&&<p>{first.fileName}</p>}{failed.map(job=><p key={job.id}>{job.fileName}：{engineeringProblem(job.errorMessage??'任务失败')}</p>)}{warnings.map((warning,index)=><p key={index}>{warning}</p>)}</div>,
        duration:failed.length||warnings.length?0:5,actions:action});
    });
    let refreshTimer:ReturnType<typeof setTimeout>|undefined;
    const refreshJobs=()=>{if(refreshTimer===undefined)refreshTimer=setTimeout(()=>{refreshTimer=undefined;void client.invalidateQueries({queryKey:queryKeys.jobs})},200)};
    const unsubscribe = realtime.onJobEvent((job) => {
      const previous=client.getQueryData<Job[]>(queryKeys.jobs),next=mergeJobEvent(previous,job);
      if(next!==previous)client.setQueryData(queryKeys.jobs,next);
      if(!previous?.some(item=>item.id===job.id)||['SUCCEEDED','FAILED','CANCELED'].includes(job.state))refreshJobs();
      if(job.type==='EXCHANGE_IMPORT'&&['SUCCEEDED','FAILED','CANCELED'].includes(job.state))void client.invalidateQueries({queryKey:['documents']});
      exchangeNotices.receive(job);
    });
    const reconcile=client.getQueryCache().subscribe(event=>{
      if(event.type==='updated'&&event.action.type==='success'&&event.query.queryKey[0]==='jobs')exchangeNotices.reconcile((event.query.state.data??[]) as Job[]);
    });
    const recover = realtime.onRecovery(() => { void client.invalidateQueries({ queryKey: queryKeys.jobs }); });
    realtime.start();
    return () => { if(refreshTimer!==undefined)clearTimeout(refreshTimer);unsubscribe(); recover(); reconcile();stopNotices();realtime.stop(); };
  }, [client, message, navigate, notification, session.data]);

  if (session.isLoading) return <div className="application-loading"><Brand /><Spin size="large" /></div>;
  if (!session.data) return <AuthScreen />;
  const user = session.data.user;
  const inWorkbench = location.pathname.startsWith("/documents/");
  const activeDocumentID = location.pathname.match(/^\/documents\/([^/]+)/)?.[1];
  const logout = async () => { realtime.stop(); await api.logout(); client.clear(); navigate("/"); };

  return <DocumentSessionProvider key={user.id}><GlobalOptionsProvider activeDocumentID={inWorkbench?activeWorkbenchDocumentID ?? activeDocumentID:undefined}><Layout className={`application-shell${inWorkbench ? " workbench-shell" : " library-shell"}`}>
    <Layout.Header className="global-header">
      <button className="brand-button" aria-label="文档中心" title="文档中心" onClick={() => navigate("/")}><Brand compact={inWorkbench} /></button>
      {inWorkbench && <DocumentTabsController />}
      {!inWorkbench && <div className="global-header-spacer" />}
      <div className="global-header-actions">
        <i role="status" aria-label={isMockMode ? "Mock" : health.isSuccess ? `OCCT ${health.data.occtVersion}` : "离线"}
          className={`service-state ${isMockMode ? "mock" : health.isSuccess ? "online" : "offline"}`} />
        <ActivityCenter />
        <GlobalOptionsButton/>
        {user.platformRole === "ADMIN" && <Button ghost aria-label="管理" icon={<ToolOutlined />} onClick={() => setAdminOpen(true)} />}
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
    {adminOpen && <Suspense fallback={null}><AdminDrawer open onClose={() => setAdminOpen(false)} /></Suspense>}
  </Layout></GlobalOptionsProvider></DocumentSessionProvider>;
}

export function App() {
  return <UIHelpProvider><Routes>
    <Route element={<ApplicationShell />}>
      <Route index element={<Suspense fallback={<RouteLoading />}><DocumentCenter /></Suspense>} />
      <Route path="documents/:documentID" element={<Suspense fallback={<RouteLoading />}><Workbench /></Suspense>} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Route>
  </Routes></UIHelpProvider>;
}
