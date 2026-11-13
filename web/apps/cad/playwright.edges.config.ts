import {defineConfig} from '@playwright/test';
export default defineConfig({testDir:'./browser',testMatch:'natural-edges-render.spec.ts',workers:1,timeout:480000,
 outputDir:'../../../build/agent-check/natural-edges/browser',expect:{timeout:20000},
 use:{baseURL:'http://127.0.0.1:5176',viewport:{width:1280,height:900},trace:'retain-on-failure',screenshot:'only-on-failure',
 launchOptions:{args:['--use-gl=angle','--use-angle=swiftshader','--enable-unsafe-swiftshader']}},
 webServer:{command:'pnpm dev:api --port 5176',url:'http://127.0.0.1:5176',reuseExistingServer:false},
});
