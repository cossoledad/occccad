# 验证与测量

验证说明保留执行范围、环境、结果和遗留限制；可重建测试入口由[测试路由](../../tests/README.md)维护，当前算法归[current](../architecture/current/README.md)，未完成工作归[plans](../../plans/README.md)。不依赖临时开发文档、Revision 或诊断编号，不把旧结果当作当前全量通过声明。原始日志和排错步骤放 `build/`，历史由 Git 追溯。

| 类型 | 入口 |
|---|---|
| 全仓性能测量 | [范围与结果](performance-baseline.md)、[机器样本](performance-baseline.json) |
| 持续 AI Run | [容量、恢复、选择与装配验收](ai-native-runs.md) |
| 连续建模 | [固定建模序列](continuous-modeling-100-baseline.md) |
| 增量运行时 | [冷/热与淘汰范围](incremental-modeling-runtime-validation.md) |
| Agent 工作流 | [定位/验证测量](agent-workflow-baseline.md)、[机器样本](agent-workflow-baseline.json) |

抽壳的最终构造及验收入口统一见[抽壳算法](../architecture/current/shell-algorithm.md)；圆角、配色、导航、选择和实体特征分别进入当前架构的所属主题，不维护重复的实现过程说明。静态几何输入与合同由 [test.data](../../tests/test.data/README.md)拥有。
