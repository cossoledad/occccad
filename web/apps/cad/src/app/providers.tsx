import { palette, uiTokens } from "../design/visual-tokens";
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
        colorWarning: palette.warning, colorError: palette.danger, borderRadius: uiTokens.radius, borderRadiusLG: uiTokens.radiusPanel,
        colorBgLayout: palette.canvas, colorBgContainer: palette.surface, colorBorder: palette.border, colorText: palette.text,
        colorTextSecondary: palette.textSecondary, colorTextDisabled: palette.textDisabled, colorBgElevated: palette.surface, colorPrimaryBg: palette.primarySoft,
        colorPrimaryBorder: palette.primaryBorder, colorPrimaryHover: palette.primaryHover,
        fontFamily: uiTokens.fontFamily, fontSize: uiTokens.fontSize, fontWeightStrong: uiTokens.fontTitleWeight,
        controlHeight: uiTokens.controlHeight, controlHeightSM: uiTokens.statusActionHeight,
        boxShadow: uiTokens.shadowPanel, boxShadowSecondary: uiTokens.shadowPopup, boxShadowTertiary: uiTokens.shadowInline,
        motion:false, motionDurationMid: "0.14s", motionDurationSlow: "0.16s", zIndexPopupBase: uiTokens.layerPopup,
      },
      components: {
        Button: { primaryShadow: "none", defaultShadow: "none", fontWeight: uiTokens.fontWeight },
        Layout: { bodyBg: palette.canvas, headerBg: palette.chrome, siderBg: palette.chrome },
        Menu: { darkItemBg: palette.chrome, darkItemSelectedBg: palette.primary },
        Modal: { borderRadiusLG: uiTokens.radiusPanel, zIndexPopupBase: uiTokens.layerModal },
        Dropdown: { paddingBlock: 4, controlHeight: 32 },
        Tooltip: { colorBgSpotlight: palette.chrome, fontSize: uiTokens.fontSmall },
        Notification: { width: 360, fontSize:uiTokens.fontSize, fontSizeLG:uiTokens.fontSize, zIndexPopup: uiTokens.layerNotification },
        Form: { itemMarginBottom: 12, labelColor: palette.textSecondary, labelFontSize: 13 },
        Table: { cellPaddingBlock: 8, cellPaddingBlockSM: 8, headerBg: palette.subtle, fontSize: 13 },
        Tree: { nodeHoverBg: palette.subtle, nodeSelectedBg: palette.primarySoft },
      },
    }}>
      <AntdApp notification={{placement:"bottomRight",bottom:uiTokens.statusHeight+16,top:16,duration:6,styles:{list:{right:16},root:{marginBottom:0}}}}>{children}</AntdApp>
    </ConfigProvider>
  </QueryClientProvider>;
}

export { queryClient };
