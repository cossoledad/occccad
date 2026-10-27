# CAD Web Agent Guide

## Scope

本目录拥有 React 工作台、CAD Tool/Input/Selection、Three.js viewport、API adapters 与邻近 Node scenarios。浏览器状态分为服务端权威状态与短期 interaction/render 状态；后者不得回写 Revision 或充当 B-Rep/权限真相。

## Invariants

- 页面/Feature 不直接散布 Scene、Renderer 或全局事件生命周期；复用 Viewport、Input、Tool、Selection 和 Command 边界。
- Pointer 工具拥有 down/move/up/cancel、capture/release、lost capture、blur 与 Esc。两点/两选择工具第一次 pointerup 后保持配对状态，Selection/Navigation 不接管事件。
- Instance move 冻结 pointerdown baseline，coalesce preview、丢弃 stale generation；pointerup 后最终求解/提交独立于普通选择变化，模型修改经结束屏障。显式取消和上下文失效仍严格生效。
- Three.js Group 不保证有 `material`；Point/Line/Mesh 使用各自 primitive、尺寸、depth/render order 与 sketch-plane transform。
- render transition 只插值 TRS，rotation 用 quaternion slerp；同批 occurrence 共用时钟，从当前显示帧重定向，不写入服务端 state。
- Realtime/鉴权改动同时验证 API 直连与浏览器实际 Vite `/api/realtime` Upgrade 入口，包括 Origin、cookie/CSRF 和 `occccad.realtime.v1` subprotocol；直连成功不能覆盖代理 403。

## Context route

先从 `src/**/testing/*.scenario.mjs`、具体 tool/store/adapter 的符号搜索进入。属性/历史面板从 `workbench-inspector.tsx` 进入，结构树 mapping 从 `workbench-tree-model.tsx`、装配表单从 `assembly-constraint-fields.tsx`、提交回调生命周期从 `assembly-edit-intent.ts` 进入。以下大文件禁止默认整读：`viewport/cad-viewport-engine.ts`、`features/workbench/workbench.tsx`、`cad/tool/cad-tool.ts`、`api/mock-api.ts`、`styles.css`。按 selection、tool gesture、rendering、state/query 或 API 生命周期分别展开。

[README](README.md)是运行和验证入口，领域事实由其链接的 current 主题拥有。公共领域/历史语义转到 `services/AGENTS.md`；复杂架构问题按 [项目地图](../../../docs/README.md) 定位。锁文件仅用于依赖版本/解析问题。

`mise run web:dev` / `pnpm dev` 默认连接根配置的真实 API，与人工/Agent 共用数据。Mock 仅由显式隔离场景选择；它不证明真实后台链路。

## Validation

- 列出场景：在本目录运行 `pnpm test -- --list [filter]`
- 单域筛选：`pnpm test -- sketch` 或任意路径/文件名片段；成功只输出摘要，失败展开该场景输出；`--verbose` 流式显示。
- 完整 Web：`mise run check --scope web`（scenarios + typecheck/production build）
- 稳定精确入口：`mise run check --scope web --match <substring>`；完成后再运行完整 Web scope
- 纯展示样式无需强造单元测试；状态机、命令、选择引用和 render data 分层必须有轻量场景。
- 复杂视觉/拾取/交互需重启浏览器人工验收；Node scenario 和 build 不能替代 WebGL acceptance。
