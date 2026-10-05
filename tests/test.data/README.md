# 测试数据

本目录统一保存仓库内文件型测试输入、回归语料和人工验收记录。测试实现、执行入口和生产能力合同仍留在所属模块；运行产生的日志、诊断归档及临时制品不作为静态 fixture 搬入这里。

- 根目录五份 STEP：目标建模验证集与交换回归共用输入，配套 `modeling-assembly-*.md` 与 `analysis-assets/`。
- `assembly-screenshots/`：原根目录 `images/` 中的五张装配实机截图。
- 根目录 `*.3dreplay`：不依赖数据库或 BREP 的数值内核重放；`windmill-cylinder-reference.3dreplay` 对应圆柱同轴回归与同名诊断报告。
- `sketch/`：草图求解回归输入。
- `assembly-contract/`：测试组合索引、证据映射和回归锁；生产语义仍来自 `services/internal/assemblycontract/catalog.json`。

新增文件型测试输入放在本目录，按用途建立子目录，并同步所属测试的加载路径。已有验收记录区分已验证结论与未验证项，不代表完整产品验收。
