# occccad v0.0.8：服务控制、自动扩缩容与调试路由

> 状态：已实现并完成端到端验收  
> 实现日期：2026-08-09  
> 前置版本：[v0.0.7 账号管理、本地制品与持久任务](occccad_v0.0.7_Distributed_Artifact_Pipeline.md)

v0.0.8 为本地开发和单机部署增加统一控制面 `occccad-control`。用户只启动一个入口，控制面读取
项目 `.env`，启动 API、Job Worker、Geometry Router 和最小 Geometry Worker 集合，并维护子进程
生命周期。浏览器与业务服务只连接稳定入口，具体实例可以扩缩、重启或被调试实例替换。

## 1. 已交付范围

- `invoke run.app` 单命令构建并启动最小完整应用；
- `occccad-control` 自动向上查找并读取 `.env`，已导出的环境变量优先；
- 管理 API、稳定 HTTP 应用入口与稳定 Geometry Router；
- API 和 Job Worker 自动启动，异常退出后自动重启；
- Geometry Worker 最小/最大数量、单实例几何容量和空闲回收；
- 新 Geometry Key 的容量感知路由和 Worker 自动扩容；
- API、Job Worker、Geometry Worker 三类 VS Code 调试配置；
- 动态 Debug Override，业务请求不需要修改地址或代码；
- `GET /control/status` 查看服务 PID、调试覆盖和 Worker 容量。

当前控制面面向本地开发、WSL2 和单机部署。生产集群未来可以由 Kubernetes、Nomad 或其他调度器
实现同样的稳定入口、容量和 Override 语义，但不把本地进程管理器直接当作跨机器编排器。

## 2. 运行拓扑

```text
Browser / API Client
         |
         v
0.0.0.0:8080  occccad-control HTTP entry
         |
         +---- default ----> managed API :18080
         |                         |
         |                         +---- PostgreSQL
         |                         +---- local ArtifactStore
         |                         `---- Geometry Router :51001
         |
         `---- debug -----> VS Code API :18081

Geometry Router :51001
         |
         +---- managed worker :51100
         +---- managed worker :51101 ... automatic
         `---- debug override -> VS Code worker :51999

PostgreSQL jobs
         |
         +---- managed occccad-jobs
         `---- exclusive debug occccad-jobs
```

外部地址与内部实例地址严格分开：

- `OCCCCAD_APP_LISTEN` 是浏览器唯一入口；
- `OCCCCAD_API_INTERNAL_LISTEN` 只给反向代理连接；
- `OCCCCAD_GEOMETRY_ROUTER_LISTEN` 是 API/Jobs 唯一几何地址；
- `OCCCCAD_CONTROL_LISTEN` 默认只绑定 Loopback，不对局域网开放；
- 实际 Geometry Worker 使用 `FIRST_PORT` 开始的独立端口。

## 3. 单命令启动

```bash
invoke run.app
```

该命令依次构建 Geometry Worker、Web、API、Jobs 和 Control Plane，然后只启动 Control Plane。
Control Plane 负责启动最小 Worker、Router、API 和 Jobs；API 继续自动迁移数据库。

直接运行 `build/services/occccad-control` 也会向当前目录的父级查找 `.env`。可以用
`OCCCCAD_ENV_FILE=/absolute/path/.env` 指定其他配置文件。独立的 `invoke run.worker/server/jobs`
仍保留用于底层排障，但不再是推荐的完整应用启动方法。

## 4. Geometry 自动扩缩容

```dotenv
OCCCCAD_GEOMETRY_WORKER_MIN=1
OCCCCAD_GEOMETRY_WORKER_MAX=8
OCCCCAD_GEOMETRY_PER_WORKER=2
OCCCCAD_GEOMETRY_WORKER_FIRST_PORT=51100
OCCCCAD_GEOMETRY_WORKER_IDLE=5m
```

路由规则：

1. 已缓存的 Geometry Key 优先回到同一 Worker；
2. 新 Key 选择 `resident + inFlight` 未达到容量且负载最低的 Worker；
3. 全部达到容量且未到 `MAX` 时，先启动并健康检查新 Worker，再发送请求；
4. 达到 `MAX` 后选择当前并发最低的 Worker，容量从硬隔离退化为受控过载；
5. RPC 完成后通过 Ping 更新真实 Resident Geometry 数；
6. 非最小 Worker 无 In-flight 请求并超过 Idle Timeout 后被回收；
7. Worker 不可达时从池中删除；低于 `MIN` 时自动补充实例。

Resident Geometry 是可重建缓存，不是业务真相。回收 Worker 会丢失内存几何，但 B-Rep/GLB 已由
ArtifactStore 持久化，后续请求可以重新加载或计算。

## 5. Control API

默认监听 `127.0.0.1:19090`：

```text
GET    /control/status
POST   /control/debug/api       {"target":"127.0.0.1:18081"}
DELETE /control/debug/api
POST   /control/debug/geometry  {"target":"127.0.0.1:51999"}
DELETE /control/debug/geometry
POST   /control/debug/jobs
DELETE /control/debug/jobs
```

管理端口必须保持 Loopback 或受信管理网络。它可以改变请求路由和暂停默认 Jobs，不应暴露到公网。

## 6. VS Code 调试

先保持 `invoke run.app` 运行，再从 Run and Debug 中选择：

### API：`occccad: API (routed debug)`

Pre-launch Task 把稳定 HTTP 入口切到 `127.0.0.1:18081`，VS Code 启动调试 API。浏览器继续访问
`http://localhost:8080`，请求进入断点所在进程；停止调试后自动恢复默认 `:18080` API。默认 API
保持运行，但在 Override 期间不接收入口流量。

### Geometry：`occccad: Geometry Worker (routed debug)`

Router 登记 `127.0.0.1:51999`，VS Code/GDB 启动独立 C++ Worker。API 和 Jobs 仍调用固定 Router
`:51001`，但 Geometry RPC 进入调试 Worker；停止调试后恢复容量池。调试目标不可达时不会静默回退，
以免隐藏断点或端口配置错误。

### Jobs：`occccad: Jobs (exclusive debug)`

Pre-launch Task 停止默认 Job Worker，防止两个消费者竞争同一 Job；调试进程独占领取 PostgreSQL
任务。停止调试后 Control Plane 自动重新启动默认 Job Worker。

配置位于 [`.vscode/launch.json`](../.vscode/launch.json) 和 [`.vscode/tasks.json`](../.vscode/tasks.json)。

## 7. 验收结果

2026-08-09 使用独立 PostgreSQL 数据库和独立端口完成：

- 一个 Control 进程自动启动 API、Jobs、Router 和 1 个 Geometry Worker；
- 稳定 HTTP 入口完成登录、文档创建和建模；
- 每 Worker 容量为 2 时，第 3 个不同几何自动创建第二个 Worker；
- Status 显示第一个 Worker 2 个几何、第二个 Worker 1 个几何；
- HTTP 入口动态切到独立调试服务，清除后恢复默认 API，Health 返回 200；
- Geometry Override 后，`debug-pad` 只进入独立调试 Worker；
- Jobs Override 停止默认消费者，清除后启动新的默认 Jobs 进程；
- Control 停止时所有托管子进程均退出；
- Go Race Test、Vet、C++ Test、前端生产构建和配置检查通过。

验收数据库、端口、进程和本地制品目录均为临时资源，完成后已清理。
