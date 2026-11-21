# 测试数据

| 输入 | 用途 |
|---|---|
| 根目录的五份 STEP | 零件建模及交换回归，配套参考图在 `analysis-assets/` |
| `*.3dreplay` | 装配求解器的冻结输入、约束和原始状态，可离线重放 |
| `sketch/` | 草图求解回归输入 |
| `assembly-contract/` | 组合索引、证据映射与回归锁，由合同检查读取 |
| `assembly-screenshots/`、`dmu/` | CATIA 操作和结构树参考图 |

新增夹具时标明消费它的测试或指南。运行生成的截图、制品、耗时与诊断保存在 `build/`。

[测试路由](../README.md) · [装配重放工具](../../services/cmd/occccad-3dreplay/README.md) · [装配合同](../assembly-contract/README.md)
