# 100 次连续复杂参数化建模时间基线

[真实测试](../../services/internal/control/continuous_modeling_baseline_test.go)经正式 Router/Worker，在当前 `.env` 的同一开发数据库和制品存储持续修改“板体拉伸→圆截面放样切除→圆周阵列→成员倒角”。每轮 100 次修改，保持定义与历史连续演进。

每步检查新 Revision、精确 GeometryID 改变、实际算子、所有 Feature 无 FAILED/BLOCKED、单实体与正有限体积。末态额外强制冷重建，逐字节比较 BREP/NAMING 且 Head 不变。计时包含 Domain Command、求值、制品、Revision 和 DocumentView。

## 2026-10-16 本机 Release 实测

环境与完整三轮样本归[统一基线](performance-baseline.md)和[数据](performance-baseline.json)。最新第三轮完整步骤归[JSON 基线](../../tests/test.data/continuous-modeling-100-baseline.json)。

| 轮次 | 命令累计 s | 平均 ms | P50 ms | 样本 P95 ms | 样本 P99 ms | 范围 ms |
|---|---|---|---|---|---|---|
| 1 | 11.286 | 112.86 | 120.40 | 135.01 | 139.70 | 69.45–142.14 |
| 2 | 11.171 | 111.71 | 118.51 | 132.33 | 142.93 | 68.98–143.74 |
| 3 | 12.027 | 120.27 | 118.48 | 136.80 | 140.42 | 69.80–879.11 |

| 轮次 | 连续区间 s | 模型准备 ms | 末态冷检查 ms | Worker 生命周期 RSS 高水位 MiB |
|---|---|---|---|---|
| 1 | 11.310 | 644.34 | 174.46 | 266.63 |
| 2 | 11.193 | 658.78 | 180.90 | 210.68 |
| 3 | 12.048 | 627.77 | 180.16 | 282.50 |

迁移与进程启动不计入模型准备；模型准备、末态冷检查不计入 100 步命令累计。阶段预算不是进程 RSS 上限。

| 第 3 轮操作 | 次数 | 平均 ms | 样本 P95 ms |
|---|---|---|---|
| associated-datum-height | 20 | 160.30 | 133.69 |
| loft-profile-radius | 20 | 125.84 | 139.60 |
| member-chamfer | 20 | 76.40 | 89.28 |
| pattern-member-count | 20 | 115.28 | 122.17 |
| stock-height | 20 | 123.55 | 136.80 |

百分位采用 nearest-rank，仅描述固定序列。未测浏览器、并发或工业容量；没有硬件无关的固定秒数门槛。

复测先构建当前 Worker，根目录串行执行下列命令；输出都放 `build/`，每轮路径不同：

```sh
mkdir -p build/performance
for run in 1 2 3; do
  OCCCCAD_MODELING_BASELINE_OUTPUT="$PWD/build/performance/modeling-$run.json" \
  go -C services test ./internal/control -run '^TestContinuousComplexModeling100BaselineThroughRouter$' -count=1 -v -timeout=20m
done
```
