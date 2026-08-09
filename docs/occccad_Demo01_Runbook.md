# occccad Demo 01 运行手册

> 最后验证：2026-08-09，Ubuntu 26.04 WSL2、GCC 15、OCCT 7.9.1、PostgreSQL 18

Demo 01 已贯通浏览器、Go API、PostgreSQL、gRPC Geometry Worker 和 OCCT。它创建一个
100×60 mm 的矩形草图并拉伸 40 mm，再把同一 Part 以 4 个实例组成两层 Product。

## 1. 准备配置

复制 `.env.example` 为 `.env`，至少填写 PostgreSQL 连接参数。数据库账号必须能够创建和
使用 `occccad` schema。密码只放在本机 `.env`，不要提交。

首次运行 Server 会自动执行 `services/internal/database/migrations` 中尚未应用的迁移。
也可在 `services/` 目录单独执行：

```bash
go run ./cmd/occccad-migrate
```

## 2. 构建

```bash
source .venv/bin/activate
invoke configure --build-type=Debug
invoke build --build-type=Debug
invoke test --build-type=Debug
invoke web.build
```

首次构建 gRPC 时，如果 ConanCenter 没有匹配当前 GCC/Debug 配置的二进制包，Conan 会在
本地编译依赖，耗时会明显长于后续增量构建。

## 3. 启动

Invoke 会自动加载仓库根目录的 `.env`，因此两个终端可以直接运行下面的命令。外部已经
导出的同名环境变量优先于 `.env`。如果绕过 Invoke 直接运行 Go 或 Worker，再手动执行：

```bash
set -a
source .env
set +a
```

终端 A：

```bash
invoke run.worker --build-type=Debug
```

终端 B：

```bash
invoke run.server
```

Go Server 默认监听 `0.0.0.0:8080`。先从 Windows 浏览器访问
`http://localhost:8080`；如果当前 WSL 网络模式没有转发 localhost，则在 WSL 中运行
`hostname -I`，使用输出的 WSL IPv4 地址访问 `http://<WSL-IP>:8080`。Geometry Worker
仍只监听 `127.0.0.1:51001`，无需暴露给 Windows 或局域网。

打开页面后点击“构建 Demo 01”。也可在 WSL 内直接检查：

```bash
curl --noproxy '*' http://127.0.0.1:8080/api/health
curl --noproxy '*' -X POST http://127.0.0.1:8080/api/demo/seed
```

## 4. 验收结果

一次成功执行应得到：

- Rectangle Sketch → Pad 的体积为 `240000 mm³`；
- 拓扑为 6 面、12 边、8 点、1 个实体，显示网格为 12 个三角形；
- 1 个 Part Document、2 个 Product Document、3 个 Version、3 条 Command；
- Machine → 2 个 Module → 每个 Module 2 个 Part Instance；
- 4 个可见实例共享 1 个 GeometryKey 和 1 份 B-Rep/GLB 制品；
- 重复执行 `/api/demo/seed` 不增加上述记录，Worker 只求值一次。

当前 Demo 为验证架构边界而固定了建模参数。通用参数编辑、约束求解、Undo/Redo、对象
存储和多 Worker 调度将在后续切片中实现。
