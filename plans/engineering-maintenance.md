# 工程维护支线

> 按真实修改频率与诊断证据触发，不是主线的预先重构门。返回[统一路线](README.md)。

- 新领域进入 `invoke check --changed` 时同步路由测试；只在发生跨层漏检时细化受影响集成。
- Workbench command dialogs、Workspace orchestration/typed handlers、viewport scene/input/rendering 只按稳定职责和重复无关读取证据拆分，不以文件数或行数设 KPI。
- Assembly 源码拆分遵循 [Solver Algorithms 的职责门](../kernel/assembly/SOLVER_ALGORITHMS.md)，首批只移动 equation semantics，不同时调数值算法。
- focused reference 只作为短导航和横切决策摘要；需要新增时指向 current/target 分册，不再复制完整事实。
- 长任务状态留在 Issue/会话；只有跨会话丢失状态反复发生，才评估可替换的任务状态文件。

每项保持等价行为测试、局部导航和 context-audit，通过后把维护事实写到所属 README/架构页面，不追加永久完成日志。
