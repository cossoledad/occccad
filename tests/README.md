# occccad tests

测试邻近所属实现：C++ CTest 在 kernel/Worker，Go 白盒在各包，Node scenarios 在 Web。根 tests 仅保存跨模块设施、[装配合同](assembly-contract/README.md)、[装配 corpus](assembly-corpus/README.md)、[NIST STEP corpus](exchange-corpus/README.md)、[静态数据](test.data/README.md)、Go conformance 和开发工具回归。

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

CTest 的完整归属由 `dev/validation.json` 的 ctestScopes 拥有：assembly/assembly-corpus → assembly，geometry/worker（包括全部 NamingArtifact 用例）→ geometry，sketch → sketch。每次 CTest 发现会审计实际注册用例恰有一个验证归属；不能仅构建 Naming 二进制或运行 fixture 生产者就声称 Naming 回归通过。

services unit 使用 `./cmd/...`、`./internal/...`、`./gen/...` 三个源码根，避免让 Go 的 `./...` 遍历运行数据/日志目录；定向发现再将命中包显式传给 go test。所有受版本控制及非忽略的 Go 源码必须落在这些根中，工具审计会阻止遗漏；原生包集合对比用于确认覆盖未缩减。

## 草图与 Solid Feature 组合检验

从[整体设计流程](../docs/architecture/current/part-design-workflow.md)进入合同，从[检验与测量](../docs/validation/sketch-solid-workflow.md)查看具体证据与测量边界。资源统一登记在 `dev/test-resources.json`；不另设测试数据库或桶。

- `services/internal/control/sketch_solid_workflow_integration_test.go`：主草图局部区域/关联偏移/抽壳、多 Body 共享参数、重复 occurrence 的薄壁/重执行/Sweep/孔组、历史 Sweep 及冷 Naming。
- `associated_extrude`、`thin_feature`、`sweep`、`engineering_hole`、`loft_boundary`、`pattern_reexecution` 的 integration 文件：各生成器参数、失败/修复、预览、补偿历史、重开与冷重建；重执行文件还输出分阶段小样本测量。
- `sketch_design_input`、`sketch_inference`、`sketch_linked_offset`、`profile_available_regions`、`feature_repair_context`、`feature_insert_generators`：显式尺寸/公式、诊断、来源和历史边界；`TestSketchDimensionsDriveSolidThroughRouter` 不依赖绘制时的数值框。
- `kernel/occt/tests/geometry_exchange_scenarios.cpp`：精确体积、导数、实体有效性、真实拓扑历史、取消与检查点；`web/apps/cad/browser/solid-feature-live.spec.ts`：真实视区/树拾取、工作台连续操作和显示。

Go match 使用正则；Web 单元选择使用路径片段，多个选择由路由负责，不能把正则当作 Web 路径片段。普通 `--level unit` 不启动集成或浏览器。需要实际交互时使用已安装浏览器和正常配置的 Control/API/Jobs，原始输出留在 build。
