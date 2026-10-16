import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";
import {fileURLToPath} from "node:url";
import {readDevelopmentEnvironment} from "../../../tools/development_environment.mjs";

export default defineConfig(({ mode }) => {
  const environment = loadEnv(mode, process.cwd(), "VITE_");
  const root = fileURLToPath(new URL("../../../", import.meta.url));
  const runtime = readDevelopmentEnvironment(root);
  const listen = process.env.OCCCCAD_APP_LISTEN || runtime.OCCCCAD_APP_LISTEN || "127.0.0.1:8080";
  const target = listen.replace(/^0\.0\.0\.0:/, "127.0.0.1:").replace(/^\[::\]:/, "[::1]:");
  const hmrPort = Number(environment.VITE_HMR_CLIENT_PORT || 0) || undefined;
  return {
    plugins: [react()],
    server: {
      host: "0.0.0.0",
      port: Number(environment.VITE_DEV_PORT || 5173),
      strictPort: true,
      hmr: {
        protocol: environment.VITE_HMR_PROTOCOL === "wss" ? "wss" : "ws",
        host: environment.VITE_HMR_HOST || undefined,
        clientPort: hmrPort,
        overlay: true,
      },
      watch: {
        usePolling: environment.VITE_USE_POLLING === "true",
        interval: Number(environment.VITE_WATCH_INTERVAL || 100),
      },
      proxy: mode === "mock" ? undefined : {
        "/api": {
          target: environment.VITE_API_PROXY_TARGET || `http://${target}`,
          changeOrigin: false,
          ws: true,
        },
      },
    },
    preview: { host: "0.0.0.0", port: 4173, strictPort: true },
    build: { target: "es2022", sourcemap: true },
  };
});
