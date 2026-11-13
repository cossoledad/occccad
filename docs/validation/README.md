# 验证与测量

可执行检查由[测试路由](../../tests/README.md)维护；当前功能及限制归[当前架构](../architecture/current/README.md)，下一步归 [CAD 核心主线](../../plans/cad-core-roadmap.md)。

必要测量见[性能基线](performance-baseline.md)及其[机器样本](performance-baseline.json)、[草图与实体检验与测量](sketch-solid-workflow.md)、[复杂导入件的切除性能](part-feature-performance.md)。均注明日期、环境、输入与计时范围，不作为当前全量通过声明。自动化的定向证据边界与重建入口并入[所属主题](../architecture/current/automation.md#验证入口)。

[自然边线](natural-edges.md)记录真实 OCCT 模型的三阶段 WebGL 对比、颜色/交互/稳定性检查、装配缓冲共享与实际导入链路；图片和原始报告按该页命令生成到 build。

交付过程、重复分析、旧任务提示和临时业务 ID 不另存文档；历史由 Git 保留，运行日志与派生报告放 build/。活跃测试、夹具和静态数据仍由 [test.data](../../tests/test.data/README.md)拥有。
