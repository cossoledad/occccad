# 开发指南

| 任务 | 入口 |
|---|---|
| 配置、构建、数据库/S3、测试资源、Chromium | [环境准备](environment.md) |
| task 配方、doctor、提交差异验证与 diagnose | [Agent 工作流](agent-workflow.md) |
| 无模型的 CAD 查询、工具输出与固定范围调试 | [只读查询调试](cad-query-debugging.md) |
| 文档归属、命名、迁移、事实/意图边界 | [文档维护](documentation.md) |
| 应用启动与进程列表 | [根 README](../../README.md) |
| 验证分层与跨模块测试 | [测试路由](../../tests/README.md) |

初次了解领域先读[项目地图](../README.md)，局部开发按最近 AGENTS 搜源码与测试。配置复用根 `.env`，不覆盖已有凭据；临时文件与生成报告统一放 `build/`。
