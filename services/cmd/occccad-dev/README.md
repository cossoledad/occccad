# occccad-dev

一次性开发资源检查 CLI，不是常驻服务。与 API、Jobs 和集成测试共用 `config.LoadEnvironment`；不迁移、不清库、不创建测试数据库或桶。

从仓库根运行 `invoke doctor` 可一并检查工具、构建与资源；底层资源检查可在 `services/` 执行：

```bash
go run ./cmd/occccad-dev doctor
```

输出 JSON，检查当前数据库查询、迁移记录存在及配置的 LOCAL 目录或 S3 桶可达性；缺项退出 2。错误文本移除配置凭据。资源可达不替代业务测试，迁移校验仍由正常启动执行。

任务、验证结果与诊断入口见[Agent 工作流](../../../docs/development/agent-workflow.md)，资源与可丢弃数据范围见[开发环境](../../../docs/development/environment.md)。
