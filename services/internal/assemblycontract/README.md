# 生产装配能力合同

`catalog.json` 唯一拥有六族、关系/精确支持、参数、模式、方向、branch、秩及失败规则。它不含测试路径、环境、执行结果、实现批次或人工验收记录。

`catalog.go` 使用 sync.Once 校验并加载嵌入合同；ForFamily / ForCapability 按索引返回脱离缓存的副本，Read 提供完整副本。调用方不能修改跨请求共享权威数据。能力 API 仅投影生产字段，不用测试 PASS 决定开放能力。

按需查看：

```sh
python tests/assembly-contract/catalog.py --capability offset.plane-plane
python tests/assembly-contract/catalog.py --family Angle
python tests/assembly-contract/catalog.py --case offset.plane-plane.first-normal-editor
```

命令在仓库根运行；零匹配明确失败。测试映射由[合同执行器](../../../tests/assembly-contract/README.md)组合，不能覆盖生产语义。

```sh
cd services
go test ./internal/assemblycontract
go test ./internal/assemblycontract -run '^$' -bench BenchmarkCatalog -benchmem
```

基准分别测量族查询副本和生产 JSON 解析，不表示端到端交互加速。
