# occccad Agent Router

occccad 是早期可运行的云原生参数化 CAD。参数模型与不可变 Revision 是业务真相；B-Rep、Mesh、Naming、缩略图与分析是带来源的可重建制品。当前行为以代码、迁移和测试为准；目标设计不代表已交付。

## 最小上下文

先检查 `git status --short`，保留无关改动。初次了解项目读[项目地图](docs/README.md)；局部任务直接进入下表最近的指南，不默认通读架构。

| 任务 | 入口 |
|---|---|
| Assembly、DOF、约束运动 | [Assembly](kernel/assembly/AGENTS.md) |
| OCCT、B-Rep、拓扑命名 | [OCCT](kernel/occt/AGENTS.md) |
| Geometry Worker、Proto/RPC、PlaneGCS | [Worker](workers/geometry/AGENTS.md) |
| Go、Command、Revision、Jobs、API、数据库 | [Services](services/AGENTS.md) |
| React、工具、选择、Three.js | [Web](web/apps/cad/AGENTS.md) |
| 文档、目录、知识入口 | [文档指南](docs/AGENTS.md) |
| 跨模块测试 | [测试路由](tests/README.md) |
| 启动、环境、资源 | [README](README.md)、[开发环境](docs/development/README.md) |

遵循 Search → Read → Expand → Architecture：先用 `rg` 定位符号和邻近测试，只读命中区间及直接调用方；跨稳定身份、历史、数据库、公共协议、Worker 或新 CAD 领域时，才按项目地图展开对应主题。所属模块、当前行为、实现/测试、最小变更和验证入口明确后停止扩张。

默认跳过 `build/`、`node_modules/`、`dist/`、日志、`services/gen/`、`tests/test.data/`、锁文件、`go.sum`、编译数据库与 STEP/BREP/GLB/3dreplay；依赖、codegen、真实 corpus 或制品问题需要时再读。协议先看 `proto/`。文本超过约 40 KB 先搜符号；超过约 80 KB 只在职责边界需要时扩大，不机械拆文件。

## 平台不变量

- 持久引用用 typed stable ID、InstancePath、Publication、PersistentSelection；禁止保存数组位置、显示名、OCCT local ID、指针或 mesh 序号作为身份。
- Revision 不可变、Workspace 显式可变；写入经 versioned Domain Command、幂等 Transaction、ChangeSet、CAS。Undo/Redo 补偿/重放，不抹历史。
- 区分 UI Command、Interaction/Preview、Domain Command、Compute Job；鼠标轨迹和预览不形成 Revision。
- 单位、容差、分支意图、失败诊断显式；缓存包含规范输入、依赖快照、evaluator/kernel/solver 版本与策略 digest。
- Worker 消费不可变粗粒度输入，不持有业务数据库真相；第三方类型不进入领域 Proto/持久模型。至少一次投递按效果幂等；Job 有 attempt/lease/deadline/cancel，迟到结果不能覆盖新 Head。
- 浏览器负责交互、近似预览与显示，提交由权威 evaluator 验证。文件、插件、表达式、Worker 请求不可信，须限制资源、权限、网络及副作用。

## 当前开发数据边界

人工运行、Agent 验证与真实集成共用当前 `.env` 的数据库、ArtifactStore、暂存、日志和 Worker 构建；不另设测试连接或测试桶。已长期授权使用这些资源，无需重复确认，不默认下载临时浏览器库。单元测试的短期对象与故障注入仍由测试拥有。加载规则见[开发环境](docs/development/README.md)，文档不复制密码。

项目未发布，人工与 Agent 生成的开发数据均可由任何开发者或 Agent 丢弃。按需直接运行 `invoke data.reset --yes` 或 `invoke run --reset-data`，无需确认；不为开发历史数据增加兼容分支、adapter、双写或过渡迁移，直接修正唯一实现及调用方，未发布迁移可重写并从空库验证。

重置前停止占用进程；仅清理命令报告的当前 PostgreSQL `occccad` schema 或专用 SQLite 的全部表、视图及迁移记录、本地 ArtifactStore/暂存目录、专用 `OCCCCAD_S3_BUCKET` 全部对象（含历史版本、删除标记、未完成分片，保留桶）。交付说明清理范围，不扩大到无关资源。维护者宣布发布基线或须保留外部数据后，此授权终止；迁移只追加，旧 Revision 必须可读。

## 变更与验证

完成任务内调用链、根因、测试和文档闭环，不顺手重写模块、升级全仓依赖或改部署。新增外部服务、重大依赖/许可证、公开兼容承诺，或改变稳定身份、历史、单位、拓扑引用、跨文档一致性时先澄清。明确要求分析/设计时，只交付分析/设计。

按风险渐进验证：`invoke check --scope <domain> --match <test>` → 领域 scope → 受影响集成 → `--scope all`；不确定先 `--plan`。公共 Proto、迁移、共享构建、未知所有权必须升级。`invoke test` 是全量入口；知识入口修改运行 `invoke context-audit`。复杂 Web 交互需浏览器验收，未运行必须说明。

高频任务用 `invoke task --recipe <property|rpc|selection|feature|diagnostic>` 定位；环境用 `invoke doctor`，提交差异用 `check --base ... --head ... --plan --json`，失败引用用 `invoke diagnose`。跳过、未执行或源码变化不能冒充通过；原始日志与报告在 `build/agent-check/`。

## 知识维护与交付

当前事实归 [current](docs/architecture/current/README.md)，长期契约归 [target](docs/architecture/target/README.md)，未完成工作归 [plans](plans/README.md)。AGENTS 放稳定规则/路由，模块 README 放职责/接口/运行，验证记录不冒充当前保证。

CATIA 对照用本机 `/mnt/c/UserApp/3D/DS/Dassault Systemes/B33doc/English/`：先搜 `control/*.viewdoc`，再读引用的 `online/*`；记录具体页面，并区分参考行为、当前实现和目标语义。

交付先写结果，再给关键判断、变更文件、实际验证与限制；区分已实现、设计、候选、未验证假设。上下文与验证机制详见[Agent 工作流](docs/development/agent-workflow.md)。
