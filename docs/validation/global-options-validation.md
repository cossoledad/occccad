# 全局选项、场景对比与树菜单验收

日期：2026-10-17；基线 `28f38e48c5c9b93b5f981c74d51bd57de642d699`，本次工作树。当前合同见[对象属性与全局选项](../architecture/current/object-scene-properties.md)，上一轮历史/交换验收见[属性验收](properties-scene-validation.md)。

使用当前 `.env` 的 SQLite、LOCAL 制品和 Release Worker，以及已有 Chromium 软件 WebGL。本轮停止并重启后端以加载最新命令目录，没有重置数据库或删除开发制品；真实验收创建自己的文档。全局选项当前保存于浏览器本地，尚无账号跨设备同步。

## 实际执行

| 范围 | 结果与证据 |
|---|---|
| 完整 Web | `invoke check --scope web`：112/112 scenarios、类型检查及生产构建通过；`build/agent-check/20261017T042839-34c3b793/report.json`。场景覆盖草稿与持久状态隔离、迁移、自动网格颜色、树分组/多选、只读机构属性；旧菜单与工具按钮场景已对齐新结构/名称，权限和提交屏障断言保留。 |
| Workspace | `invoke check --scope workspace`：656 个 test/subtest verdict 通过，0 跳过/失败；`build/agent-check/20261017T042244-e0ebfe00/report.json`。定向创建/属性检查另有 9 项通过，`20261017T041701-8d42241c/report.json`。 |
| API 与目录 | `go test ./internal/api ./internal/workbenchconfig` 通过；`build/agent-check/global-options-api-check.log`。 |
| 真实浏览器 | 最新重启运行上的 `properties-live.spec.ts`：2 项通过；`build/agent-check/global-options-live-rebuilt.log`。真实建模后设置 Body/面外观、预览取消、Undo/Redo、CAS 草稿恢复、负 Z 自动地面/世界地面、光照取消不产生 Revision；普通/装配中新建 Part 均保存新默认色，之后修改全局值不改变原文档。 |
| Mock 浏览器 | 最后一轮 8 项全部通过：树菜单/Alt+Enter、无视区右键菜单、Body 历史、全局场景预览/取消/跨文档与刷新持久化、机构名称属性提交、CATIA/SOLIDWORKS 导航、选面进入草图及跨文档相机恢复；`build/agent-check/global-options-browser-accepted.log`。此前扩大导航回归有 2 项失败，具体边界见下节；日志 `global-options-browser-final.log`。 |
| 视觉检查 | 人工查看软件 WebGL 截图：深色/明亮背景能切换浅/深网格，主线与细线区分，模型保留哑光层次；树菜单按实际操作分组，属性在底部。截图 `build/acceptance/global-options-dark.png`、`global-options-light.png`、`global-options-tree-menu.png`、`global-options-visual-dialog.png`。 |
| 环境 | `invoke doctor --scope services,web --browser --json`：PASS，含构建新鲜度和现有 Chromium 启动；`build/agent-check/global-options-doctor.json`。 |
| 文档 | `invoke context-audit`、8 个文档审计测试和 `git diff --check` 通过；`build/agent-check/global-options-context-audit.log`、`global-options-documentation-tests.log`。 |

视觉证据证明具体软件 WebGL 场景的显示和交互，不代表硬件 GPU、复杂透明面排序、性能或全浏览器验收。本轮未改 Proto、数据库迁移、原生交换算法，未重跑全仓/原生交换/STEP 大模型；不将上一轮记录视为本轮完整重验。

## 导航回归的已知失败

完整记录包含 `orthographic detail navigation, isometric shortcut and sketch view round trip` 和 `sketch normal view, persistent world context and exit selection synchronization` 两项失败。前者重复等轴测后相机位置字符串有轴向差异（`157.3831` 对 `157.4643`），方向和 zoom 相同；后者记录了草图进入动画中的初始四元数，而正视命令最终返回单位四元数。

在未修改 HEAD 的临时 worktree 中，使用同一依赖版本、独立 Vite 缓存和 Chromium 重新运行这两个原始用例，复现相同断言：`build/agent-check/global-options-baseline-navigation-isolated.log`。只添加端口/输出目录配置，没有修改应用源码或用例断言；原始 trace 在 `build/acceptance/baseline-navigation/`。本轮没有调整相机算法、同步条件或放宽精度；临时 worktree 已清理，原工作区依赖链接已恢复。不能声明完整导航套件通过。

浏览器仍有原有 Ant Design 弃用与 PMREM 采样提示；上述真实/Mock 功能验收没有未捕获 pageerror。
