import {defineConfig} from "@playwright/test";
import {fileURLToPath} from "node:url";
import {readDevelopmentEnvironment} from "../../../tools/development_environment.mjs";

Object.assign(process.env, readDevelopmentEnvironment(fileURLToPath(new URL("../../../", import.meta.url))));
export default defineConfig({
 testDir:"./browser",outputDir:"../../../build/acceptance/playwright-live",testMatch:["solid-feature-live.spec.ts","motion-live.spec.ts","properties-live.spec.ts","automation-live.spec.ts"],workers:1,timeout:180_000,
 expect:{timeout:20_000},
 use:{baseURL:"http://127.0.0.1:5175",viewport:{width:1440,height:1000},trace:"retain-on-failure",screenshot:"only-on-failure",
 launchOptions:{args:["--use-gl=angle","--use-angle=swiftshader","--enable-unsafe-swiftshader"]}},
 webServer:{command:"VITE_INPUT_DEBUG=true pnpm dev:api --port 5175",url:"http://127.0.0.1:5175",reuseExistingServer:false},
});
