import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./browser",
  fullyParallel: false,
  workers: 1,
  // Software WebGL can stall the renderer while a document scene is rebuilt.
  timeout: 60_000,
  expect: { timeout: 15_000 },
  use: {
    baseURL: "http://127.0.0.1:5174",
    viewport: { width: 1440, height: 1000 },
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    launchOptions: { args: ["--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader"] },
  },
  webServer: {
    command: "VITE_INPUT_DEBUG=true pnpm dev:mock --port 5174",
    url: "http://127.0.0.1:5174",
    reuseExistingServer: false,
  },
});
