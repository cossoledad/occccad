# occccad tests

仓库级测试资产集中在此目录，按运行时语言划分：

- `cpp/`：C++/OCCT、Geometry Worker 和 PlaneGCS 单元及交换格式回归，由 CMake/CTest 构建；
- `go/`：跨包黑盒、协议和进程级 Go 测试，使用独立 test-only module；
- `front/`：不进入 Web 生产 bundle 的 TypeScript/交互状态机测试脚本；
- `../models/`：维护者提供的 STEP/BREP 等只读交换语料，不复制到各语言目录。

`invoke test` 是统一入口，依次执行 CTest、`services/` 原生 Go package tests、`tests/go` 和 Front 单元测试。`invoke web.build` 仍只负责类型检查和生产构建。

Go 的 package-private 白盒测试必须与被测 package 同目录，这是 Go 工具链按目录编译 package 的语义；这些 `_test.go` 文件不会进入生产二进制。新测试优先通过导出边界写入 `tests/go`，只有必须验证 package-private 不变量时才允许邻近 `_test.go`，不得通过 test-only export、复制源码或 `go:linkname` 伪造集中目录。
