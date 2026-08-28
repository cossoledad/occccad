# occccad tests

本目录只保存无法归属单个实现模块的跨包、跨进程和协议 conformance。单元测试及模块级用户场景必须邻近被测实现：

- C++ solver 场景位于 `workers/geometry/sketch/tests`，OCCT 几何场景位于 `kernel/occt/tests`，由所属模块的 CMake 注册；
- Web 场景位于对应 `src/**/testing/*.scenario.mjs`，每个文件在独立 Node 进程运行并由 `pnpm test` 自动发现；
- Go 遵循工具链要求，以邻近 `_test.go` 构建 package-private 白盒测试；本目录的 `go/` 只保留通过公开边界运行的多包/进程测试；
- `../models/` 保存跨实现共享的 STEP/BREP 只读 corpus。

`invoke test` 是统一入口，依次执行模块注册的 CTest、`services/` Go package tests、`tests/go` conformance 和自动发现的 Front 场景。`invoke web.build` 仍只负责类型检查和生产构建。

测试以行为所有权分层，而不是以语言集中：pure model tests 不启动网络或数据库；interaction scenario 用新的 driver/fixture 表达完整手势；adapter conformance 复用 corpus 比较不变量；只有 transport/process boundary 才进入根目录。禁止跨场景共享可变数组、按前一测试产生的下标断言，或把多个工具串成一个依赖执行顺序的脚本。
