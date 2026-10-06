# 100 次连续复杂参数化建模时间基线

测试：[TestContinuousComplexModeling100BaselineThroughRouter](../../services/internal/control/continuous_modeling_baseline_test.go)。逐步计时、实际执行次数与环境记录：[JSON 基线](continuous-modeling-100-baseline.json)。

真实 Router/Worker + 独立临时 SQLite/ArtifactStore，持续修改同一个“板体拉伸→圆截面放样切除→圆周阵列→成员倒角”零件。20 轮，每轮依次修改板厚、放样截面半径、关联基准高度、阵列数量、成员倒角距离，共 100 次正式参数化建模修改。保持模型定义与历史连续演进，计时包含 Domain Command、求值、制品、Revision 提交和 DocumentView 返回。

每步断言：新 Revision、精确 GeometryID 改变、实际算子执行、所有 Feature 无 FAILED/BLOCKED、单个有效实体与正有限体积。最终另做真正强制冷重建并逐字节比较 BREP/NAMING，检查 Head 未改变。固定输入和更新数列由测试定义；没有重复缓存读取冒充建模，没有采用 mock 几何。模型结构保持固定，测量参数化修改与下游重算；追加超长 Feature 链是独立工作负载。

## 本次实测

- 环境：linux/amd64，8 逻辑 CPU，go1.26.5，Debug Worker / OCCT 7.9.1，evaluator `part-solid-generators-v33-stage-runtime`；UTC 2026-10-06T07:26:16Z。
- 100 个建模命令累计：**42.59s**；连续计时区间（含逐步记录检查）：42.65s。
- 平均：**425.92ms**；P50：457.22ms；P95：**593.60ms**；P99：608.86ms；范围：182.56–645.48ms。
- 模型准备：1.05s；最终冷等价检查：1.19s，均在 100 步计时之外。进程启动、数据库迁移不包含在模型准备时间内。
- 实际阶段执行/复用：260/380；生成器/局部修改/Body 组合入口：80/100/160；Profile/对应准备：20/0。跨多个 RPC 的重复阶段复用分别计数，不能把复用总数当作唯一 Feature 数量。
- Worker 生命周期 RSS 高水位：263.97MiB；阶段缓存观测最大值：127.98MiB。RSS 包含工作集与多类缓存，阶段预算不等于进程内存上限。

| 操作 | 次数 | 平均 ms | P95 ms |
|---|---:|---:|---:|
| associated-datum-height | 20 | 492.58 | 566.71 |
| loft-profile-radius | 20 | 493.73 | 567.83 |
| member-chamfer | 20 | 216.54 | 244.40 |
| pattern-member-count | 20 | 395.73 | 466.80 |
| stock-height | 20 | 531.01 | 608.86 |

百分位采用 nearest-rank。单次实测作为后续同环境、同工作负载对照；不设置固定秒数失败门槛，不作为硬件无关性能保证。每步 exact、Naming、mesh、encoding、排队、I/O、缓存与 RSS 指标在 JSON 中；各耗时有包含关系，不直接相加推断总时间。未运行浏览器或全量单测，也未使用或清理用户数据。

## 复现

在 `services/` 中执行；默认无真实 Worker 环境变量时按既有集成测试约定跳过，因此计时验证必须指定 Worker。`-count=1` 禁止 Go 测试结果缓存。JSON 输出路径可省略（只输出日志），指定时需为绝对路径。

```sh
OCCCCAD_TEST_GEOMETRY_WORKER=/home/ganjb/project/occccad/build/cmake/debug/workers/geometry/occccad_geometry_worker \
OCCCCAD_MODELING_BASELINE_OUTPUT=/home/ganjb/project/occccad/tests/test.data/continuous-modeling-100-baseline.json \
go test ./internal/control -run '^TestContinuousComplexModeling100BaselineThroughRouter$' -count=1 -timeout=20m -v
```
