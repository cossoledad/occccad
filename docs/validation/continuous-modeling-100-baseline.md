# 100 次连续复杂参数化建模时间基线

测试：[TestContinuousComplexModeling100BaselineThroughRouter](../../services/internal/control/continuous_modeling_baseline_test.go)。最新第三轮逐步计时、实际执行次数与环境记录：[JSON 基线](../../tests/test.data/continuous-modeling-100-baseline.json)。

真实 Router/Worker + 独立临时 SQLite/ArtifactStore，持续修改同一个“板体拉伸→圆截面放样切除→圆周阵列→成员倒角”零件。20 轮，每轮依次修改板厚、放样截面半径、关联基准高度、阵列数量、成员倒角距离，共 100 次正式参数化建模修改。保持模型定义与历史连续演进，计时包含 Domain Command、求值、制品、Revision 提交和 DocumentView 返回。

每步断言：新 Revision、精确 GeometryID 改变、实际算子执行、所有 Feature 无 FAILED/BLOCKED、单个有效实体与正有限体积。最终另做真正强制冷重建并逐字节比较 BREP/NAMING，检查 Head 未改变。固定输入和更新数列由测试定义；没有重复缓存读取冒充建模，没有采用 mock 几何。模型结构保持固定，测量参数化修改与下游重算；追加超长 Feature 链是独立工作负载。

## 2026-10-16 本机 Release 实测

AMD Ryzen 9 9950X，32 逻辑 CPU，Go 1.26.5、OCCT 7.9.1；当前源码/二进制、完整样本与计时口径见[统一基线](performance-baseline.md)和[数据](performance-baseline.json)。串行运行 3 个独立序列，每个 100 步，均通过末态强制冷重建等价检查。

| 轮次 | 命令累计 s | 平均 ms | P50 ms | 样本 P95 ms | 样本 P99 ms | 范围 ms |
|---|---|---|---|---|---|---|
| 1 | 4.965 | 49.65 | 51.76 | 61.40 | 62.82 | 28.19–63.04 |
| 2 | 5.106 | 51.06 | 52.86 | 64.59 | 68.42 | 29.47–70.24 |
| 3 | 4.894 | 48.94 | 51.21 | 60.67 | 61.66 | 28.97–62.54 |

三轮连续区间（含逐步记录/检查）分别为 4.988 / 5.128 / 4.916 秒；模型准备 248.41 / 232.24 / 229.78 ms，末态冷检查 110.95 / 112.48 / 109.84 ms，均不计入 100 步命令累计时间。数据库迁移与进程启动不属于模型准备计时。

每轮实际执行/复用阶段 260/380；生成器/局部修改/Body 组合 80/100/160；Profile/对应准备 20/0。Worker 生命周期 RSS 在步骤中观测的高水位分别为 206.93 / 207.38 / 207.14 MiB；阶段缓存观测最大约 128.00 MiB。阶段预算不是进程内存上限，跨 RPC 复用计数不等于唯一 Feature 数。

下表对应已更新 JSON 的第 3 轮，每种操作 20 次；其余两轮与全部 300 个命令耗时保存在统一数据。

| 操作 | 次数 | 平均 ms | 样本 P95 ms |
|---|---|---|---|
| associated-datum-height | 20 | 54.27 | 60.21 |
| loft-profile-radius | 20 | 54.88 | 59.79 |
| member-chamfer | 20 | 32.33 | 35.36 |
| pattern-member-count | 20 | 47.14 | 52.37 |
| stock-height | 20 | 56.06 | 61.66 |

百分位采用 nearest-rank，只描述各轮固定序列，不是稳定服务 P95/P99。没有浏览器、并发或工业容量测量，未设置硬件无关的固定秒数门槛。

## 复现

根 `.env` 已提供 Release Worker/SQLite 默认配置；先构建最新 Worker，确认没有 Skip。每轮输出使用不同绝对路径，避免三次覆盖同一个报告：

```sh
mkdir -p build/performance
for run in 1 2 3; do
  OCCCCAD_MODELING_BASELINE_OUTPUT="$PWD/build/performance/modeling-$run.json" \
  go -C services test ./internal/control -run '^TestContinuousComplexModeling100BaselineThroughRouter$' -count=1 -v -timeout=20m
done
```
