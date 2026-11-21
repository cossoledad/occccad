# 零件建模性能验证

按用户操作测量完整预览、预览后确认和改参。服务端分段区分依赖计划、Worker、制品接纳和事务提交。Worker 内再观察精确几何、命名、离散和编码。嵌套指标不能相加。浏览器下载、拾取、首次绘制和帧时间需要在真实视口单独测量。

| 场景 | 可执行测试 | 正确性检查 |
|---|---|---|
| STEP 选面建草图并切除 | `TestDenseSTEPFaceSketchAndFirstCutThroughRouter` | 冻结支撑、绘图沿用未变 Body、实际材料变化、确认复用、冷重建 |
| 四边倒圆角、曲线间 G2 过渡 | `TestBoundaryFilletLifecycleThroughRouter`、`TestSurfaceShapeQualityThroughRouter` | 撤销重做、重开、曲率分析、预览与提交的制品 |
| 大平板阵列 100、500、1,000 孔 | `TestPlateHoleArrayThroughRouter` | 在 610 × 60 × 6 mm 板上阵列一列 10 个直径 2 mm 的孔，确认面数和材料去除量，改列距后冷算 |
| 固定与持续增长拓扑的长链 | `TestLongPartChainWithFixedTopologyThroughRouter`、`TestGrowingTopologyPartFeaturesAndNamingThroughRouter` | 分别改变历史长度与边面数量，核对前驱复用、阶段数、持久引用和冷结果 |
| 同一长链的末步编辑与完整冷算 | `TestStoredNamingTipPreviewTimingThroughRouter`、`TestStoredGrowingTopologyNamingReplayThroughRouter` | 固定文档与版本，核对实际 Worker 执行、BREP、Naming 及 Head |
| 上游修改与前驱缺失 | `TestPartFeatureChainIncrementalAndColdThroughRouter`、`TestMissingPartContinuationRecoversThroughRouter` | 受影响后缀重算、缺失制品恢复、几何与引用一致 |

从仓库根目录运行，例如：

```bash
mise run check --scope services --level integration --match '^TestPlateHoleArrayThroughRouter$'
mise run check --scope services --level integration --match '^TestGrowingTopologyPartFeaturesAndNamingThroughRouter$'
```

增长场景默认 120 步，可通过 `OCCCCAD_TEST_NAMING_STEPS` 指定规模。保存的建模文档可供末步测试（`OCCCCAD_TEST_NAMING_TIMING_DOCUMENT`，用新参数触发求值）或冷算测试（`OCCCCAD_TEST_NAMING_REPLAY_DOCUMENT`）使用。记录模型、参数、显示精度、构建版本、冷热状态及样本数。完整冷算、单步续算和已有预览确认分别统计。

[求值与已知限制](../architecture/current/part-feature-evaluation.md) · [测试入口](../../tests/README.md)
