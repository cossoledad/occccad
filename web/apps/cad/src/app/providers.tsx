import { palette } from "../design/visual-tokens";
import { App as AntdApp, ConfigProvider, theme } from "antd";
import zhCN from "antd/locale/zh_CN";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, type PropsWithChildren } from "react";
import { installNumericInputSelection } from "./numeric-input-selection";

const queryClient = new QueryClient({ defaultOptions: {
  queries: { staleTime: 15_000, retry: 1, refetchOnWindowFocus: false },
  mutations: { retry: 0 },
} });

export function AppProviders({ children }: PropsWithChildren) {
  useEffect(() => installNumericInputSelection(document), []);
  return <QueryClientProvider client={queryClient}>
    <ConfigProvider locale={zhCN} theme={{
      algorithm: theme.defaultAlgorithm,
      token: {
        colorPrimary: palette.primary, colorInfo: palette.primary, colorSuccess: palette.success,
        colorWarning: palette.warning, colorError: palette.danger, borderRadius: 6, borderRadiusLG: 10,
        colorBgLayout: palette.canvas, colorBgContainer: palette.surface, colorBorder: palette.border, colorText: palette.text,
        colorTextSecondary: palette.muted, colorBgElevated: palette.surface, colorPrimaryBg: palette.primarySoft,
        fontFamily: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
        controlHeight: 34,
      },
      components: {
        Button: { primaryShadow: "none", fontWeight: 550 },
        Layout: { bodyBg: palette.canvas, headerBg: palette.chrome, siderBg: palette.chrome },
        Menu: { darkItemBg: palette.chrome, darkItemSelectedBg: palette.primary },
        Modal: { borderRadiusLG: 10 },
        Tree: { nodeHoverBg: palette.subtle, nodeSelectedBg: palette.primarySoft },
      },
    }}>
      <AntdApp>{children}</AntdApp>
    </ConfigProvider>
  </QueryClientProvider>;
}

export { queryClient };
