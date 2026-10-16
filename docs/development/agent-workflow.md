# Agent 上下文与验证工作流

目标是降低一次正确完成任务所需的无关上下文。保留领域合同、测试与完整诊断，通过明确归属和按需读取减少消耗；运行时日志质量不因 Agent 输出压缩下降。

## 上下文分层

```text
根 AGENTS → 最近局部 AGENTS → 符号/调用方/邻近测试 → 命中架构或证据
首次了解项目：根 AGENTS → docs/README.md 项目地图
```

局部任务通常只需一个领域指南、1–4 个实现区间和 1–2 个测试。按 Search → Read → Expand → Architecture 探索；所属模块、当前行为、实现/测试、最小变更和验证入口明确后停止。跨 stable identity、Revision/Undo、Naming、单位、数据库、公共协议或 Worker 边界时，按[项目地图](../README.md)读取命中主题。

默认排除与全局不变量由[根指南](../../AGENTS.md)拥有。文本大于约 40 KB 先搜符号；80 KB/120 KB 是职责拆分候选/强候选信号，不是自动拆分条件。只有独立职责与反复无关读取的证据才拆分实现。

知识归属见[文档维护](documentation.md)。五个代码领域指南为 Assembly、OCCT、Worker、Services、Web；文档维护另有 `docs/AGENTS.md`。不为每个目录新增指南。

## 渐进验证 API

```bash
invoke check --scope assembly --match DirectedAngle
invoke check --scope web --match realtime
invoke check --scope workspace --match '<test regex>'
invoke check --scope <assembly|geometry|sketch|workspace|services|web>
invoke check --changed --plan
invoke check
invoke check --scope all
invoke context-audit
```

`invoke test` 保持人类/CI 全量入口。`check` 未指定 scope 时与 `--changed` 一样按 Git 路径保守选域。`--plan` 只解释 scope、升级原因、cwd 和命令，不构建/执行测试。多域合并，all 覆盖其他选择。

`--match` 只允许一个非 all 的显式 scope：C++ 为域前缀内 CTest regex，Go 为 `-run`，Web 为场景路径/文件名 substring。零匹配必须失败。精确验证跳过跨层集成与 production build；公共行为完成后按风险升级到无 match 的领域 scope → 受影响集成 → all。

| 改动路径 | 默认 scope |
|---|---|
| `kernel/assembly/`、assembly corpus | assembly |
| `kernel/occt/`、几何输入 | geometry |
| Worker sketch | sketch |
| Workspace/Model Core | workspace |
| 其他 Go、Go conformance | services |
| Web | web |
| Proto/生成协议、迁移、通用 Worker、kernel/api、共享构建/验证、未知路径 | all |
| Markdown | 无可执行验证；知识入口仍须 context-audit |

准确映射以 [tasks.py](../../tasks.py) 和 [路由测试](../../tests/python/test_validation_routing.py)为准；路由只读路径，不读内容。公共协议修改必须通过正式 Router，直连 Worker 不证明调用链完整。

## 成功与失败输出

- 成功：每步一行 PASS 与耗时，最后汇总；Web 场景只给计数。
- 失败：终端保留有界首尾和 error/fail/expected/actual 等高信号行，完整 stdout/stderr、cwd、复现命令写入 `build/agent-logs/`。
- `--verbose` 恢复流式输出，供观察 warning、进度或卡顿。

Web runner 可按多个 OR substring 筛选、`--list` 只列场景、每个场景用独立进程。成功安静不表示删除日志或减少测试覆盖。

## 静态治理与测量

`invoke context-audit` 检查根/局部指南尺寸、必需知识入口、全仓自有 Markdown 的本地链接/锚点/围栏、从根入口的可达性及知识目录归属，并统计大于 40 KB 的文本热点。`--verbose` 列出完整热点；热点本身不是失败。

[documentation_audit.py](../../tools/documentation_audit.py) 可输出完整 JSON；`--baseline <git-revision>` 比较文档总字节、根指南、局部入口与首次地图读取。指标按实际读取路径区分，不把所有入口加在一起当作每次必读。

字节是静态代理指标，不是模型 token；可达性不是内容正确性；入口变小不证明端到端任务更快。一次任务实际消耗还包含源码/测试/工具输出与系统注入。验证结论应声明实际执行范围，Node/SSR、RPC、定向通过均不能扩大为浏览器或工业验收。

| 静态演练 | 最短路径 |
|---|---|
| Directed Angle | 根 → Assembly guide → solver/test → assembly match → scope |
| 选择问题 | 根 → Web guide → selection/tool/scenario → web match → scope |
| Undo/Redo | 根 → Services guide → history/test → workspace match → scope |
| 新 RPC | 根 → Worker/Services guide → Proto/adapter/Router → all |

若局部任务仍反复整读架构、generated output 或运行全仓，先修所有权/路由。后续维护统一进入[plans](../../plans/README.md)，本页不保留执行日志。
