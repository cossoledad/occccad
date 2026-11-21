# 验证入口

| 要验证的行为 | 入口 | 观察结果 |
|---|---|---|
| 全仓单元测试 | `mise run check --scope all --level unit` | 分域通过、失败与跳过，报告位于 `build/agent-check/` |
| 零件持续建模 | [工程场景与复测方法](part-feature-performance.md) | 精确结果、引用、历史和用户操作耗时 |
| 装配与机构 | [求解器](../../kernel/assembly/README.md)、[合同](../../tests/assembly-contract/README.md)、[重放](../../tests/assembly-corpus/README.md) | 约束残差、分支、自由度、预算与失败分类 |
| 几何交换 | [交换语料](../../tests/exchange-corpus/README.md) | 单位、结构、表示类型、颜色、可恢复的警告 |
| 浏览器操作与显示 | [Web 测试](../../web/apps/cad/README.md)、[工作台](../guides/workbench.md) | 树与视区选择、预览取消、真实绘制及提交回执 |

## 工作台与显示

草图和特征交互分别用 `solid-feature-live.spec.ts`、`geometric-feature-live.spec.ts`、`part-objects-live.spec.ts` 检查预选、创建、重选与修复。自然边线用 `EdgeProfile` 原生场景和 `playwright.edges.config.ts` 检查采样、遮挡和高亮。曲面照明与缩略图用 `SurfaceDisplay`、`TestStudioMeasurements` 和 `playwright.studio.config.ts` 检查。真实文档另用 `playwright.live.config.ts`，需启动正式后端。

## 性能测量

`mise run benchmark` 测装配原生热路径，`mise run benchmark:baseline` 运行 Go 基准。结果保存在 `build/performance/`。比较同机、同输入和同构建配置的多个样本，分开记录内核时间、完整请求、浏览器绘制和内存。将一次运行的输出留在构建目录。

[验证层级与状态](../development/agent-workflow.md) · [测试资源](../../tests/test.data/README.md)
