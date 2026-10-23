# Assistant 用量、完整导出与选择读取

2026-10-23，当前实现合同见[自动化架构](../architecture/current/automation.md#轮次用量与会话导出)。本轮不扩展 CAD 操作、不增加分类模型/DSL/多轮评测平台，不处理运行中人工编辑。测试使用当前开发配置的数据库、制品与 Worker，新建测试对象，没有重置原有数据。Provider 为受控实现，没有真实或付费模型请求。

## 实现与定向证据

| 内容 | 验证 |
|---|---|
| 移除文字分类 | “如何创建…？”、“查看并修改参数”、“为什么失败？请修复”等均保留实际可写上下文的权限；相同修改文字在历史 Revision 下仍走只读 QUERY。 |
| 稳定轮次 | 普通调用 InputID 为空、turnId 指向创建时用户输入；QUERY 的 InputID/租约语义保留。Epoch 变化而无新输入时归属不变，新输入后旧回执仍计入旧轮。 |
| 用量幂等 | 失败且已报告、未知重试、重复回执、未知后补报及新 Service 恢复：旧轮 160、新轮 40、累计 200 Token，三次实际调用各计一次。待返回与未知不记为已知零消耗。 |
| 完整导出 | 超过 UI 页窗口的 39 条输入、40 条调用、39 条工具结果和真实阶段/Task 全部读取，长公开回复超出显示摘要的尾部仍存在；两种格式、整会话/本轮复用同一记录流。 |
| 快照与历史缺失 | 导出快照建立后的新消息未混入文件；旧调用无 turnId 不猜归属，缺少实际请求快照明确标记。运行状态/截至时间/未完成工作保留；私有 reasoning 哨兵未导出。 |
| CAD 后续结果 | 本轮导出包含实际 sketch.create 的预览/提交及原 receipt；按模型调用的原幂等来源绑定 Task，未用 Epoch 归属。 |
| 下载与权限 | HTTP 支持 Markdown/JSON、会话/本轮；跨账号访问和无效格式拒绝。下载前释放数据库游标，完整快照逐条暂存后流式输出，正常路径清理暂存。 |
| 选择类型 | PAD 展示分类被还原为真实 FILLET；草图圆作为 SKETCH_ENTITY/CIRCLE，保留 Sketch/Body 归属。BRep 拓扑仍为正式 PersistentSelection，默认摘要不包含完整证据。 |
| 实例/子元素 | 同定义多 occurrence 经工作台选择、冻结后仍为两个实例；父 Product 快照不能替代路径接受的 Part Revision。相同草图实体的 START/END 保留不同正式引用。 |
| 紧凑读取 | 同一合成冻结页：默认 529 B，显式正式引用 3553 B；初始上下文未重复注入证据。只是该 fixture 的 UTF-8 字节，不等于模型 Token 节省比例。 |

新增/核心入口：`turn_usage_test.go`、`session_export_test.go`、`selection_context_test.go`、`run_scope_test.go`；HTTP 复用 `assistant_test.go`。原规范化测试显式读取 includeEvidence，继续验证超限结果的在线/持久/恢复/trace 一致性；没有删除原超限断言。既有只读并行测试改用真实历史快照，而非关键词分类。

## 执行范围

原始日志及去重汇总 `summary.json` 在 `build/agent-check/assistant-usage-export/`。Go 定向检查累计通过 38 个顶层测试，包含新增用量/导出/选择，以及受影响的 QUERY、预算、规范工具结果、查询引用、权限、恢复、阶段及原生 CAD 回归。原有合成百万用量、无总步骤/时限和双完成入口的原生回归仍保留。Web TypeScript 检查、6 个 Assistant 场景及 9 个 selection 场景通过；文档审计 8 个测试、context-audit 和 `git diff --check` 通过。首次定向检查中的旧完整证据 fixture 失败已通过显式 includeEvidence 修正，并在后续检查中通过，原始失败日志保留。

未执行全量单元测试、浏览器/WebGL 实机、真实 LLM、PostgreSQL、超大生产会话的容量/吞吐验收。SQL 保持双后端表达式，当前实际执行证据为 SQLite。进程崩溃可能留下 build/assistant-exports 暂存文件；导出依赖可写磁盘，不包含未保存历史或几何二进制。历史轮次归属缺失不会被补猜，历史活动 Run 可通过新消息建立后续明确来源。

选择摘要提供正式对象/归属提示；几何测量与命令执行仍由原查询、选择解析和 Preview/Commit 校验。本轮未证明 LLM 会自动正确选择目标或设计参数。建议实机检查混合树/视图选择、重复实例、轮次用量显示和下载文件的阅读体验。
