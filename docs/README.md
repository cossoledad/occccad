# Architecture knowledge router

本文只负责把任务路由到最小必要的架构上下文。普通局部修复先读根/局部 `AGENTS.md`、实现符号和邻近测试；不要默认通读 `CURRENT_ARCHITECTURE.md`（事实全景）或 `TARGET_ARCHITECTURE.md`（长期设计书）。

## 先判断要回答什么

| 问题 | 入口 |
|---|---|
| 当前代码已实现什么、进程怎样连接 | `CURRENT_ARCHITECTURE.md` 的对应章节 |
| 长期领域语义或跨模块边界应是什么 | `TARGET_ARCHITECTURE.md` 的对应章节 |
| 可执行单元如何运行、配置、排障 | 所属模块 README |
| 普通局部 bug、测试或实现细节 | 局部 `AGENTS.md` → `rg` 符号 → 邻近测试，通常无需架构文档 |

## 按任务定位

| 任务 | 当前事实 | 目标语义 |
|---|---|---|
| 仓库/进程拓扑 | Current 2–3 | Target 3、5.1 |
| Domain Command、Workspace、Revision、Undo/Redo、参数依赖 | Current 4.1–4.4 | Target 4.3 |
| Sketch、Constraint、Profile、PlaneGCS | Current 5、5.1 | Target 5.3 |
| Part Feature、Body、Boolean | Current 5 | Target 5.4 |
| Surface/Wireframe | 当前尚无完整实现 | Target 5.5 |
| Product、Assembly、DOF、DMU | Current 4、5、8 | Target 5.6；数值细节另看 `kernel/assembly/` 文档 |
| PersistentSelection、TopologyHistory、naming | Current 5 | Target 5.7 |
| Worker、Router、Jobs、Artifact | Current 3、5.2、6–7 | Target 5.1、6–9 |
| Web interaction、selection、rendering、realtime | Current 8 | Target 10 与对应 CAD 领域章节 |
| 数据库、兼容、发布边界 | Current 4、7 | Target 4、8、16、19 |
| 安全、SLO、可观测性 | Current 9 | Target 11–13 |
| 测试、Definition of Ready/Done、架构变更 | Current 9 | Target 17、19 |
| Agent 上下文、scoped validation、quiet output、Token 效率计划 | Current 9 | [`architecture/agent-efficiency.md`](architecture/agent-efficiency.md) |

## 高频 focused reference

- [Model、Command 与 History](architecture/model-history.md)：Revision、ChangeSet、Undo/Redo、依赖和最终 CAS；
- [Persistent Naming](architecture/persistent-naming.md)：TopologyHistory、PersistentSelection、Reconnect 与歧义；
- [Worker Contracts](architecture/worker-contracts.md)：Proto、Worker/Router、Artifact、Job 与迟到结果门禁；
- [Agent Efficiency](architecture/agent-efficiency.md)：上下文、验证、输出、指标与后续结构优化。

这些页面是面向任务的短投影。领域语义仍以表格所指向的 Target 章节为准，当前交付状态仍以 Current Architecture 和代码/测试为准。

章节用标题和 `rg -n '^#{2,4} .*关键词'` 定位，不依赖易漂移行号。只有任务改变稳定身份、历史、单位、拓扑引用、公共协议、数据库或 Worker/服务一致性边界时，才继续扩展到交叉章节。

## 知识职责

- `AGENTS.md`：稳定的 Agent 行为、模块不变量、导航和验证入口；
- 模块 `README.md`：当前职责、公共接口、配置、运行和故障语义；
- `CURRENT_ARCHITECTURE.md`：可由当前代码验证的系统事实；
- `TARGET_ARCHITECTURE.md`：长期领域语义、平台边界、候选与阶段门；
- 临时任务上下文：留在 Issue/会话，不沉积到上述长期入口。

## 停止条件

当所属模块、当前行为、实现位置、相关测试、最小变更和验证命令都已明确时，停止继续读架构。需要更深语义时按上表精确升级，而不是扩大为全仓扫描。

## Token-efficiency 代理指标

无需估算模型内部 token；定期用以下可复现信号检查本结构是否退化：

- persistent context：根 `AGENTS.md` 保持 router 量级，模块细节只进入最近的 local guide；
- discovery：普通 Assembly/Web/Workspace 局部任务的默认路径应是 root + 1 个 local guide + 命中实现/测试区间，不先读两份完整架构或生成代码；
- validation：`invoke check --scope <domain>` 不运行无关语言/领域，`invoke check --changed` 对共享契约保守升级；路由由 `tests/python/test_validation_routing.py` 固定；
- output：成功的每个底层步骤只产生一行 PASS，Web 全场景成功只产生一行计数；失败保留原始 stdout/stderr 和复现命令；
- hotspots：用 `git ls-files -z | xargs -0 stat -c '%s %n' | sort -nr` 识别新增大文本，但只按稳定职责拆分。

若一次普通局部任务仍需要整读大型 architecture/source、重复寻找测试入口或默认运行全仓，应先修正路由/所有权，而不是删除知识、测试或诊断。
