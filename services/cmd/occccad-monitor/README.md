# occcccad-monitor

`occccad-monitor` 是独立、只读的本地 TUI 监控适配器。它每秒从 `occccad-control` 的版本化快照接口读取一次数据，不直接读取 PostgreSQL、ArtifactStore 或其他进程内存，因此采集契约以后可被 Web 监控面板复用。

```bash
# 终端 1
invoke run.app

# 终端 2
invoke run.web --mode=api

# 终端 3
invoke run.monitor
```

默认连接 `http://127.0.0.1:19090`；独立控制进程使用其他地址时设置 `OCCCCAD_CONTROL_URL`。按 `←/→`、`h/l` 或 `1..4` 切换 Overview、Processes、Geometry、Business，`r` 立即刷新，`q` 退出。

`invoke run.monitor` 在构建后会以 TUI 替换 Invoke 进程，使 Bubble Tea 直接拥有当前终端。不要在自定义包装脚本中为它再创建一层 PTY，否则外层 canonical input 可能缓存单键操作。

当前 Linux/WSL 通过 `/proc` 展示 Control、API、Jobs 和每个 Geometry Worker 的 CPU、RSS、虚拟内存、线程与 uptime；非 Linux 平台保留进程存活和业务指标，但资源字段暂为零。CPU 首次采样为零，第二次快照开始显示区间使用率。Business 页显示实时连接、订阅/在线文档会话，以及持久文档、Revision、排队任务和制品数量。管理 API 仍必须绑定 loopback；Control 到 API 的内部业务快照使用每次启动随机生成的令牌，外部请求不会得到该数据。
