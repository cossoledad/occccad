# 文档 Agent Guide

本目录整理项目知识。先从[项目地图](README.md)找主题，再读对应 current、target 或指南；事实冲突时搜源码与邻近测试。移动链接无需读取二进制 corpus。

- 一个主题一个权威来源：current 写当前行为与限制，target 只留仍成立的架构约定，guides 写操作与人工检查，validation 只留必要测量。[CAD 核心主线](../plans/cad-core-roadmap.md)是唯一 CAD 主线任务入口。
- 不把测试存在写成执行通过，不把定向/Mock 验证写成工业、浏览器或完整链路验收。说明只保留最终算法、接口和限制；验证结论注明范围与环境，重复分析与原始过程归 build/Git，不依赖可清除的开发文档或 Revision ID。
- 使用职责清楚的目录、`README.md` 索引、语义化 kebab-case 文件名和可搜索标题；新架构文件不用任务号、阶段号、版本号命名。稳定 API、schema、case、fixture ID 保持原值。
- 新主题由索引链接；移动时同步全仓 Markdown、代码注释和工具路径，修正相对链接、图片与锚点。删除重复页前合并独有知识，不保留重定向空壳。
- 常读主题以简短摘要、源码/测试入口开头，再展开细节；索引只保存路由与边界，不复制完整能力清单或执行日志。
- 模块 README 保留职责、启动、配置、接口和故障语义，领域细节链接到 current。源码目录不因文档整理改名。

验证：`python -m unittest discover -s tests/python -p 'test_documentation_audit.py'`、`invoke context-audit`、`git diff --check`。修改审计/Invoke 时补运行路由测试。完整规则见[文档维护](development/documentation.md)。
