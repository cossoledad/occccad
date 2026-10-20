# CAD Web

独立 React 应用，提供文档中心、Part/Product 工作台与 Three.js 视口，默认连接统一开发 API，隔离前端场景可显式使用 Mock。浏览器拥有交互与显示；参数模型、权限、Revision 和最终 B-Rep 求值属于服务端。

## 职责与入口

| 修改对象 | 从这里搜符号/场景 |
|---|---|
| 页面与工作台布局 | `src/features/`；`workbench-layout.tsx`、`workbench.css` |
| 文档接入与命令 | `src/cad/document/`、`src/cad/command/`、`workbench-command-registration.ts` |
| 自动化建模流程 | `src/features/assistant/`；[操作与任务合同](../../../docs/architecture/current/automation.md) |
| 选择、工具与手势 | `src/cad/interaction/`、`src/cad/tool/`、`src/cad/input/` 及邻近 `testing/` |
| 编辑宿主、树、Inspector | `product-edit-context.ts`、`workbench-tree-model.tsx`、`workbench-inspector.tsx` |
| 显示与资源 | `src/viewport/`、`src/cad/visual/`、`src/cad/rendering/` |
| 服务端/短期状态 | TanStack Query / `src/state/` Zustand；`src/api/` 适配 |
| 视觉系统 | `src/design/visual-tokens.ts`、`design-system.css` |

先读 [AGENTS](AGENTS.md)，按命中区间展开，不默认整读 `workbench.tsx`、Viewport Engine 或 Mock API。依赖版本以本目录 `package.json` 和 workspace 锁文件为准。

属性、外观与全局选项见[统一属性](../../../docs/architecture/current/object-scene-properties.md)。领域行为只维护于[Web 架构](../../../docs/architecture/current/web.md)、[编辑上下文](../../../docs/architecture/current/product-edit-context.md)、[公共命令框架](../../../docs/architecture/current/document-command-framework.md)。UI 与人工步骤见[操作指南](../../../docs/guides/README.md)，导航表见[导航 README](src/cad/navigation/README.md)，机构范围见[当前 DMU](../../../docs/architecture/current/kinematics-dmu.md)。

## 运行

仓库根目录：

```bash
invoke run.web
# 隔离前端场景
invoke run.web --mode=mock
```

默认前端通过 `/api` 代理根 `.env` 的 `OCCCCAD_APP_LISTEN`，与正常应用和 Agent 验证共用数据。也可在 `web/` 安装依赖后执行 `pnpm dev`；`dev:mock` 是显式隔离入口。默认地址为 `http://localhost:5173`。前端 `.env.development` / `.env.mock` 只保存 Vite 参数；后端连接由根配置决定，凭据不进入浏览器变量。


## 验证

```bash
# 仓库根：精确场景及完整 Web scope
invoke check --scope web --match <substring>
invoke check --scope web
invoke web.build
```

`web.build` 做类型检查与生产构建。邻近场景为 `src/**/testing/*.scenario.mjs`，每个在独立 Node 进程执行。在本目录用 `pnpm test -- --list [filter]` 预览，`pnpm test -- sketch` 按路径/文件名筛选（多个筛选 OR），零匹配失败；默认成功只输出汇总，失败展开诊断，`--verbose` 流式输出。

浏览器环境依赖按[环境准备](../../../docs/development/environment.md#chromium-与浏览器验证)复用系统配置。在本目录：

```bash
# 仅在缺少 Chromium 时安装
pnpm exec playwright install chromium
pnpm test:browser
```

Mock Playwright 自动在 5174 启动独立 Vite，使用 Chromium/软件 WebGL，结束关闭进程；失败截图和 trace 在配置的构建输出目录。`browser/workbench.spec.ts` 覆盖布局/命令/树，`browser/navigation.spec.ts` 覆盖组合手势；Mock/Node/build 不证明真实几何、权限、后端装配或原生 GPU 通过。

真实服务验收先启动最新 API/Worker（`invoke run --build-type=Release`），Live 配置自动读取根 `.env` 或 `OCCCCAD_ENV_FILE` 的同一登录环境，再在本目录运行：

```bash
pnpm exec playwright test --config playwright.live.config.ts
# 只跑机构试动/录制
pnpm exec playwright test --config playwright.live.config.ts browser/motion-live.spec.ts
```

Live 用例创建独立验收文档，不重置数据库；其具体覆盖由测试定义，不能把入口存在当作本次已执行。运行产生的验收截图放 `build/acceptance/`。

## 故障与性能边界

API 请求诊断在内存保留最近 200 项，携带浏览器总耗时和 Server-Timing；侧栏显示最后完成请求的毫秒耗时，不是命令总时长或后端 CPU 时间。复杂失败使用现有诊断包和原请求回执恢复，不能重新生成未知提交的操作身份。

GLB 按对象摘要去重，解码数据归视口，不回填 Query 模型。可丢弃 render pose 不回写权威快照。大树分页、渐进工作集与工业规模性能仍须独立验证；长期方向见[大模型设计](../../../docs/architecture/target/large-models.md)。
