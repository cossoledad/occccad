# 抽壳算法与功能验收

2026-10-17，基线 `4fc1302` 加本次工作区修改。当前合同见[实体 Feature](../architecture/current/solid-features.md)；本页记录特定输入的执行证据，不代表任意曲面均可抽壳。

## 基线与修复

基线四组原生 Shell 测试中，两组失败：`ShellAfterLargeModifiersAndMixedLoft`、`ShellOffsetRectangleToLargerCircle`，均含偏置精度错误。原始报告：`build/agent-check/20261017T093618-c87383f9/report.json`。

混合尖角/相切放样被 OCCT 整体偏置拒绝；一次多参数分区也可能丢失重合壁层。补充路径改用开口局部框架中的独立面精确偏置、二元裁剪与顺序并集，合成实际历史。局部交点在既定误差内重定位，真实材料轮廓与孔洞保留；不增加厚度容差。UV 邻近合并源三维边界点还会造成显示边界不一致，消费端通过公开网格扩展点保留不同源采样，并继续执行全部显示门禁。详见 `kernel/occt/src/shell_junction.cpp`、`mesh_boundary.cpp` 和 `occt_kernel.cpp`。

## 原生专项

`invoke check --scope geometry --match Shell`：5/5 组通过，无跳过，报告 `build/agent-check/20261017T120334-73c5b631/report.json`。收尾后专项再次 5/5 通过，报告 `build/agent-check/20261017T121537-362f9b5b/report.json`。

- 长方体向内/向外，单开口/双开口：验证解析体积、一个 Solid、完整 Naming、选择顺序冷重算、上游不变；零厚度、负厚度及缺少开口被拒绝。
- 大倒角、全棱圆角、倒角后底棱圆角、矩形到圆放样：验证开口、底厚、法向两侧材料、边/点记录与实际 pcurve 偏差、全部面有网格、有限坐标、GLB、上游 BREP 不变及冷重算 Naming。
- 偏置矩形 `(-82.8172482034993,-87.47795914135841)` 到 `(111.25953546530698,69.55214784189991)`，圆半径 90、截面间距 100 mm：0.5/1/2 mm；另加旋转平移后的 1 mm。同样检查实际厚度与完整制品。

## 真实 Worker、应用与浏览器

| 验证 | 结果与证据 |
|---|---|
| `invoke check --scope geometry` | 原生 100/100；Go 集成 220 通过、4 跳过、0 失败。`build/agent-check/20261017T120545-8c29ff9f/report.json` |
| 真实 Worker | 三个前置模型 × 0.5/1/2 mm 共九组抽壳均通过，收紧包含性门禁后九组再次通过。检查阶段面引用、完整 Naming、非空 BREP/GLB、体积减少和同输入缓存复用。最终日志 `build/agent-check/shell/worker-delivery.log` |
| 收尾后专项 | `invoke check --scope geometry --match Shell`：5/5；日志 `build/agent-check/shell/shell-containment.log` |
| 失败显示/缓存键/抑制定向 | `go test ./internal/workspace -run 'TestFailedBodyDisplayProvenanceAndRecovery|TestPartGeometryKeyIncludesFeatureIdentityAndTopologyPolicy|TestSolidFeatureSuppressionUsesDefinitionHistory' -count=1`：通过；`build/agent-check/shell/workspace-final.log` |
| Chromium，真实 API/Router/Worker | `pnpm exec playwright test --config playwright.live.config.ts --grep 'loft shell|viewport Boolean|chamfered box fillet' --output ../../../build/acceptance/playwright-shell-final`：4/4；`build/agent-check/shell/browser-delivery.log` |

完整领域回归之后，收尾修复了失败 Visual 的重复投影、更新旧浏览器用例的对象子菜单定位、消除了 C++ 参数遮蔽警告，并禁止复用包含证明的分区路径重定位端点。该收尾由最终原生专项、九组 Worker、Workspace 定向和真实浏览器覆盖；未在收尾后重复整个领域回归，也未运行全仓测试。包含性门禁收紧后，两类放样的浏览器创建/失败恢复/编辑/Undo/Redo/重开再次 2/2 通过，日志 `build/agent-check/shell/browser-containment.log`。

四项跳过均为显式 `NOT_APPLICABLE`：当前存储不是 S3；装配性能由独立性能入口测量；两个大型 STEP 用例未选择可选 corpus。不存在抽壳测试跳过。

两类放样在视图区选择开口，验证 0.5 mm 预览不推进 Head、1000 mm 无效厚度清除旧候选并禁止提交、修复后创建、编辑为 1 mm、Undo/Redo 新 Revision 及重开持久选择。另两项覆盖布尔视图区拾取/编辑及倒角→圆角→抽壳的精确体积、上游失败、旧成功结果显示、对象子菜单抑制和恢复。

首次浏览器组合链停在已移入对象子菜单的“抑制”定位（旧脚本仍在顶层查找），同次日志还暴露失败显示的 `visual naming digest mismatch`。重复投影测试先复现摘要变空，再修复为保留已有摘要；最终浏览器额外断言该摘要非空且实际 Visual 下载成功，没有通过绕过来源检查解决。旧失败日志保留于 `build/agent-check/shell/browser-final.log`。

最终留存文档：

- 小型放样：`/documents/01a14b4b-31d1-7a20-8f3f-f620c49312c8`。
- 偏置大截面放样：`/documents/01a14b4b-6963-74cc-bea0-dadb6f3eb632`。
- 两者的 `shell-reopened.png` 位于 `build/acceptance/playwright-shell-containment/` 对应用例目录；组合链的 `failed-body-display.png` 位于 `build/acceptance/playwright-shell-final/`。已检查开口、内壁及失败显示截图。

## 证据边界

共享当前 `.env` 的开发数据库、ArtifactStore 与 Worker 构建，未重置数据、未下载浏览器或修改第三方 OCCT。新增测试的对象键使用独立命名空间；浏览器文档留存供查看。最终九组 Worker 单次运行中，复杂放样的三个厚度冷 RPC 约 6.0/9.8/15.1 秒，缓存命中约毫秒级；当时与原生专项并行，属于单次样本，不是稳定延迟分位数。复杂曲面的冷计算仍需优化。专项耗时含冷重算及多次原生候选，不能据此声明吞吐或任意复杂曲面的工业可靠性。
