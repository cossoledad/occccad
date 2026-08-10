import { App as AntdApp, ConfigProvider, theme } from "antd";
import zhCN from "antd/locale/zh_CN";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { PropsWithChildren } from "react";

const queryClient = new QueryClient({ defaultOptions: {
  queries: { staleTime: 15_000, retry: 1, refetchOnWindowFocus: false },
  mutations: { retry: 0 },
} });

export function AppProviders({ children }: PropsWithChildren) {
  return <QueryClientProvider client={queryClient}>
    <ConfigProvider locale={zhCN} theme={{
      algorithm: theme.defaultAlgorithm,
      token: {
        colorPrimary: "#1683b8", colorInfo: "#1683b8", colorSuccess: "#2f9b73",
        colorWarning: "#d48a20", colorError: "#d85252", borderRadius: 6,
        fontFamily: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
        controlHeight: 34,
      },
      components: {
        Button: { primaryShadow: "none", fontWeight: 550 },
        Layout: { bodyBg: "#eef2f5", headerBg: "#111b24", siderBg: "#17232d" },
        Menu: { darkItemBg: "#17232d", darkItemSelectedBg: "#1683b8" },
        Modal: { borderRadiusLG: 8 },
        Tree: { nodeHoverBg: "#e9f2f7", nodeSelectedBg: "#d9edf7" },
      },
    }}>
      <AntdApp>{children}</AntdApp>
    </ConfigProvider>
  </QueryClientProvider>;
}

export { queryClient };
