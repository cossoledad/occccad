# 验证记录

这里归档定向实测、失败调查与人工验收记录，绑定各报告中的版本、输入、环境及执行日期，不是当前版本的全量通过声明。复现入口归[测试路由](../../tests/README.md)，当前合同归[current](../architecture/current/README.md)，未完成工作归[plans](../../plans/README.md)。

| 领域 | 报告 |
|---|---|
| Agent 开发流程 | [五类任务实测](agent-workflow-baseline.md)、[机器可读数据](agent-workflow-baseline.json) |
| 最新本机基线 | [全仓性能入口与样本](performance-baseline.md)、[机器可读数据](performance-baseline.json) |
| 装配诊断 | [风车同轴诊断](windmill-assembly-reference-diagnostic.md) |
| 建模性能 | [连续 100 次建模](continuous-modeling-100-baseline.md)、[增量运行时](incremental-modeling-runtime-validation.md) |
| 模型分析与现场反馈 | [目标模型分析](modeling-assembly-analysis.md)、[实机反馈](modeling-assembly-field-review.md) |
| Feature 与几何 | [特征选择](feature-selection-validation.md)、[有限面域圆角](fillet-boundary-validation.md)、[底座建模体验](bottom-support-parametric-validation.md) |
| 属性/外观/场景 | [导航十字、光标与结构树手柄](navigation-affordances-validation.md)、[地面网格连续性](ground-grid-continuity-validation.md)、[透视导航与列表缩略图](viewport-projection-thumbnail-validation.md)、[视口、导航与透明边线](viewport-navigation-validation.md)、[全局选项与菜单整理](global-options-validation.md)、[正式历史、RGBA 交换与真实视口](properties-scene-validation.md) |
| Sketch/显示/参数 | [草图与输入](sketch-display-parameter-experience-validation.md) |

静态输入和截图仍由 [test.data](../../tests/test.data/README.md)拥有；临时报告、截图和诊断输出放 `build/`。本次整理的尺寸比较也是派生报告，不写入长期能力保证。
