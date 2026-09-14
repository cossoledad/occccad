# occccad tests

本目录只保存无法归属单个实现模块的跨包、跨进程和协议 conformance。单元测试及模块级用户场景必须邻近被测实现：

- C++ solver 场景位于 `workers/geometry/sketch/tests`，OCCT 几何场景位于 `kernel/occt/tests`，由所属模块的 CMake 注册；
- Web 场景位于对应 `src/**/testing/*.scenario.mjs`，每个文件在独立 Node 进程运行并由 `pnpm test` 自动发现；
- Go 遵循工具链要求，以邻近 `_test.go` 构建 package-private 白盒测试；本目录的 `go/` 只保留通过公开边界运行的多包/进程测试；
- `../models/` 保存跨实现共享的 STEP/BREP 只读 corpus。

`invoke test` 是全量入口，依次执行开发入口路由单测、模块注册的 CTest、`services/` Go package tests、`tests/go` conformance 和自动发现的 Front 场景。Agent/局部开发使用 `invoke check --scope <domain>`，或直接运行 `invoke check` 让 Git changed-file mapping 保守选择 `assembly / geometry / sketch / workspace / services / web / all`。公共 Proto、数据库迁移、共享构建/Worker 边界和未知路径自动升级到 `all`。成功输出被压缩为每步 PASS 与耗时；失败保留完整 stdout/stderr 和独立复现命令，`--verbose` 可流式显示。

`invoke check --scope <domain> --match <pattern>` 提供 Level 1 精确验证：Assembly/Geometry/Sketch 将 pattern 限制在所属 CTest 前缀，Workspace/Services 传给 Go `-run`，Web 作为场景路径 substring。`--match` 只接受一个非 `all` scope，并有意跳过跨层集成与 production build；行为完成后按风险升级到无 `--match` 的 domain scope。

Front runner 支持路径/文件名片段筛选，例如在 `web/apps/cad` 运行 `pnpm test -- sketch`；`--list` 只列命中场景，`--verbose` 显示成功场景的原始输出。筛选为 OR 语义，无匹配会失败而不是假装通过。

测试以行为所有权分层，而不是以语言集中：pure model tests 不启动网络或数据库；interaction scenario 用新的 driver/fixture 表达完整手势；adapter conformance 复用 corpus 比较不变量；只有 transport/process boundary 才进入根目录。禁止跨场景共享可变数组、按前一测试产生的下标断言，或把多个工具串成一个依赖执行顺序的脚本。

`python/test_validation_routing.py` 固定验证 changed-file 到 scope 的路由与保守升级规则；它测试开发入口本身，不改变产品测试所有权。
