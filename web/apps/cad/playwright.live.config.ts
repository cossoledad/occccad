import {defineConfig} from "@playwright/test";
export default defineConfig({
 testDir:"./browser",testMatch:"solid-feature-live.spec.ts",workers:1,timeout:180_000,
 expect:{timeout:20_000},
 use:{baseURL:"http://127.0.0.1:5175",viewport:{width:1440,height:1000},trace:"retain-on-failure",screenshot:"only-on-failure",
 launchOptions:{args:["--use-gl=angle","--use-angle=swiftshader","--enable-unsafe-swiftshader"]}},
 webServer:{command:"pnpm dev:api --port 5175",url:"http://127.0.0.1:5175",reuseExistingServer:false},
});
