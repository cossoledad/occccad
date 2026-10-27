# occccad tests

测试邻近所属实现：C++ CTest 在 kernel/Worker，Go 白盒在各包，Node scenarios 在 Web。根 tests 仅保存跨模块设施、[装配合同](assembly-contract/README.md)、[装配 corpus](assembly-corpus/README.md)、[静态数据](test.data/README.md)、Go conformance 和开发工具回归。

公开入口是 `mise run plan` / `mise run check`，准确选择、层级、状态、报告见[Agent 工作流](../docs/development/agent-workflow.md)，资源规则见[环境](../docs/development/environment.md)。

```bash
mise run --quiet plan --changed --level unit --json
mise run check --scope assembly --level unit --match DirectedAngle --jobs 4
mise run check --scope assembly --level integration --match '^TestAssemblyMotionThroughRealRouter$' --jobs 4 --test-jobs 1
mise run check --scope tooling --level unit
mise run check --scope docs --level unit
```

Go 的配置数据库、ArtifactStore、生产 Worker 证据显式使用 integration build tag，文件资源归属见 [test-resources.json](../dev/test-resources.json)。不带 tag 的单元测试不加载应用配置；允许自己拥有短期 SQLite/RPC 对象。直接原生命令仍可用：在 services 运行 `go test ./internal/modelcore`，真实资源用 `go test -tags=integration -count=1 ./internal/control -run '^TestAssemblyMotionThroughRealRouter$'`。TestMain 复用正式配置加载器，资源依赖不能用测试名称排除规则隐瞒。

`--level full --scope all` 覆盖注册表各层，包括显式浏览器层与真实 realtime conformance；不表示所有可能后端、corpus 或平台。普通局部工作不默认全量，不自动下载浏览器或大型数据。缺资源报告 ENVIRONMENT_BLOCKED，零匹配失败，未执行不算通过。

Node `pnpm --dir web/apps/cad test -- sketch` 按路径片段 OR 筛选，--list 列表，--verbose 展开。复杂交互仍需要浏览器验收，Mock/Node 不证明真实 WebGL 后台链路。`tests/go/realtime` 要求本次正常 Control/API/Jobs 应用已运行，同一 .env，不设置第二套服务/数据库地址。

工具回归已迁至 [Go devtool](../services/internal/devtool/devtool_test.go)、[合同测试](assembly-contract/runner.test.mjs)、[Node 工具测试](tooling/audit.test.mjs)；保留路径/rename/零匹配/skip/锁/脱敏/合同 baseline 的负例。文档审计用 `mise run docs:check`，修改审计后用 `mise run contract:check`。原始日志与结构化报告在 build，源码变更期间的旧结果不能绑定新工作区。
