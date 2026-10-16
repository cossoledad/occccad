# 增量建模运行时定向验证

## 2026-10-16 本机 Release 复测

同一 `.env`、数据库与制品存储，正常缓存及 1 字节淘汰预算各串行重复 5 次，全部通过。阶段详情及完整样本见[统一基线](performance-baseline.md)与[数据](performance-baseline.json)。下表每格为同一场景 5 次中位数；冷请求真正绕过缓存，仍比较同定义精确/Naming 结果。

| 请求/出现次数 | 执行/复用（热→冷） | 服务 ms（热→冷） | exact ms（热→冷） | Naming ms（热→冷） |
|---|---|---|---|---|
| runtime-append-tail / 1 | 1/2 → 3/0 | 76.44 → 85.09 | 7.83 → 15.51 | 7.61 → 9.91 |
| runtime-edit-middle / 1 | 2/1 → 3/0 | 79.10 → 80.06 | 12.81 → 13.65 | 8.96 → 8.94 |
| runtime-loft-session / 1 | 1/3 → 4/0 | 80.51 → 103.44 | 9.59 → 22.98 | 6.14 → 12.70 |
| runtime-after-worker-restart / 1 | 4/0 → 4/0 | 114.57 → 110.11 | 26.95 → 25.70 | 17.74 → 13.93 |

名称/显隐预览的实际阶段计数也保存在 JSON；无 Worker 调用时 RSS=0 表示未采集。1 字节预算不复用阶段，真实 Worker 重启后重建，均保留冷等价与原 Head。

测试整例（含准备/生命周期/断言）与算子时间分开；三条建模 fixture 各重复 3 次，结果见统一基线。

复现：

```sh
go -C services test ./internal/control -run '^TestIncrementalRuntime(Production|Eviction)ThroughRouter$' -count=5 -v -timeout=20m
```

## 合同与限制

实现、预算及恢复语义归[Part 增量运行时](../architecture/current/jobs-artifacts.md#part-增量运行时)。验证覆盖精确 GeometryID、体积、包围盒及 BREP/NAMING 冷等价、上游不重复执行、取消、淘汰和重启。

原生共享状态串行，取消不能打断正在运行的 OCCT 算子；阶段检查点易失，缺失后由冻结输入恢复。缓存预算不是 RSS 上限；浏览器交互、长会话和高负载排队未在本轮测量。
