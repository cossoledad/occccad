# 文档维护

| 读者需要知道什么 | 权威位置 |
|---|---|
| 如何启动、配置或运行模块 | 根或模块 README、[开发环境](environment.md) |
| 当前行为、实现入口及限制 | [当前架构](../architecture/current/README.md) |
| 后续实现必须保持的语义 | [架构约定](../architecture/target/README.md) |
| 用户怎样完成操作 | [操作指南](../guides/README.md) |
| 外部软件行为及其来源 | [参考资料](../references/README.md) |
| 怎样重现结果并判断正确性 | [验证入口](../validation/README.md)、邻近测试与夹具 |
| 待实施事项及完成条件 | [CAD 核心主线](../../plans/cad-core-roadmap.md) |

每项行为在所属主题说明一次。其他页面用描述性链接指向它。索引提供任务入口，AGENTS.md 保留会改变开发决策的规则。当前功能以代码、迁移和测试为准；已解决问题的过程由 Git 追溯。耗时对照、日志、临时文档 ID 和截图报告放在构建目录。仍可复现的限制保留在所属主题。

移动或合并文档时，同步调整索引、标题锚点、源码注释和工具中的引用。保留测试实际消费的合同基线、预期输出和语料。运行 `mise run docs:check` 检查路径、锚点、可达性及命令。修改文档审计时再运行 `mise run contract:check`。
