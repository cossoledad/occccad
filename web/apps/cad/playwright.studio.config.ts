import {defineConfig} from '@playwright/test';
import path from 'node:path';
import base from './playwright.edges.config';
const reference=process.env.OCCCCAD_STUDIO_REFERENCE_WEB;
const quote=(value:string)=>"'"+value.replaceAll("'", "'\\''")+"'";
export default defineConfig({...base,testMatch:'visual-studio-render.spec.ts',timeout:480_000,outputDir:'../../../build/agent-check/visual-studio/browser',
 webServer:reference?{...base.webServer,command:`pnpm exec vite ${quote(reference)} --config ${quote(path.join(reference,'vite.config.ts'))} --mode development --port 5176`}:base.webServer,
});
