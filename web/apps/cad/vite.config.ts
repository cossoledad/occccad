import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

export default defineConfig(({ mode }) => {
  const environment = loadEnv(mode, process.cwd(), "VITE_");
  return {
    plugins: [react()],
    server: {
      host: "0.0.0.0",
      port: Number(environment.VITE_DEV_PORT || 5173),
      strictPort: true,
      proxy: mode === "mock" ? undefined : {
        "/api": {
          target: environment.VITE_API_PROXY_TARGET || "http://127.0.0.1:8080",
          changeOrigin: true,
        },
      },
    },
    preview: { host: "0.0.0.0", port: 4173, strictPort: true },
    build: { target: "es2022", sourcemap: true },
  };
});
