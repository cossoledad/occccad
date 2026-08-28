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
        colorPrimary: "#176b87", colorInfo: "#176b87", colorSuccess: "#327b5f",
        colorWarning: "#a96d1f", colorError: "#b64b4b", borderRadius: 2, borderRadiusLG: 3,
        colorBgLayout: "#dfe4e7", colorBorder: "#aeb8be", colorText: "#202b31",
        fontFamily: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
        controlHeight: 34,
      },
      components: {
        Button: { primaryShadow: "none", fontWeight: 550 },
        Layout: { bodyBg: "#dfe4e7", headerBg: "#17232b", siderBg: "#1d2a32" },
        Menu: { darkItemBg: "#1d2a32", darkItemSelectedBg: "#176b87" },
        Modal: { borderRadiusLG: 3 },
        Tree: { nodeHoverBg: "#e9f2f7", nodeSelectedBg: "#d9edf7" },
      },
    }}>
      <AntdApp>{children}</AntdApp>
    </ConfigProvider>
  </QueryClientProvider>;
}

export { queryClient };
