# 透视导航与列表缩略图验收

日期：2026-10-17；基线 `92f9032e61eae126460c33e9337c02751c905138`，本次工作树。当前规则见[导航](../../web/apps/cad/src/cad/navigation/README.md)、[全局选项](../architecture/current/object-scene-properties.md)、[颜色与缩略图](../architecture/current/surface-appearance.md)。

本轮默认透视 ISO、全局透视/平行选项，支持 Fit、有限裁剪、无限网格深度、相机/草图返回快照；CATIA 中键松开时居中，平移持续更新中心深度，屏幕中央绘制固定尺寸的分段 X/Y 与短黄色 Z。结构栏省略重复标题和文档名称。缩略图缺失时幂等补排任务，列表等待刷新；默认图是空场景。

## 参考与实现边界

本机 B33 `English/control/BasEnglishC2.viewdoc` → `online/basug_C2/basugbt1507.htm` 明确分别选择 Perspective/Parallel。默认透视、35° 视角、十字线尺寸与具体取点时机来自本次用户要求和项目选择，不声称是全部 CATIA 版本的统一参数。

两种投影共用当前全局设置，不进入模型 Revision。持久 MotionViewpoint 保留原平行 framing 数据合同，显示边界转换。法线视图复用当前显示投影的 occurrence snapshot，只补齐省略的 Revision；已提供的旧 Revision、owner/occurrence 不匹配不重绑。

## 实际验证

日志相对 `build/agent-check/`；Mock/WebGL、真实后台、源代码检查是不同证据层。

| 范围 | 结果与证据 |
|---|---|
| 完整 Web | `invoke check --scope web`：115/115 场景、类型检查和生产构建通过，`20261017T071302-1d6b0c6a/report.json`；检查前后 source 相同，`viewport-perspective-web-accepted.log`。 |
| Mock 浏览器 | `viewport-perspective-browser-accepted.log`：navigation、normal-view、properties、viewport-rendering 共 12 项通过，包含两种投影的生产 WebGL 网格深度与透明边线。 |
| 真实浏览器 | `viewport-perspective-live-accepted.log`：3 项通过，当前 `.env` SQLite、LOCAL、真实 API/Release Worker、现有 Chromium 软件 WebGL；包含图形属性/分面/透明度 Preview、Undo/Redo/CAS、全局新建外观，以及新建后留在列表取得缩略图。 |
| Go 所属包 | `viewport-perspective-go.log`：完整 `internal/thumbnail`、`internal/api`、`cmd/occccad-jobs` 通过。 |
| API 定向复测 | `viewport-perspective-api-repeat.log`：缺失预览/PNG 和 `TestRealtimeEditingRecoveryAndPreviewDatabase` 连续 3 次通过。 |
| 知识入口 | 文档审计 8 项、`invoke context-audit`、`git diff --check` 通过；`viewport-perspective-doc-tests.log`、`viewport-perspective-context-audit.log`。 |

- Node 场景校验中键按下不拾取、松开拾取、平移中心保持主射线、缩放中心无横向漂移；投影转换保持焦点比例，透视 Fit 覆盖八个包围盒角点，裁剪不移动透视眼点，文档/草图快照跨投影恢复；固定像素十字的分段与颜色；两种投影中网格相位/深度与 CPU 射线交点一致。
- 浏览器使用生产导航组合键和全局选项，包含预览取消、持久化、草图往返和旋转 occurrence 的法线视图。隔离 WebGL fixture 对两种投影分别验证上方遮挡地面、下方地面覆盖实体，并检查透明表面后细线/宽线的对比；不将其冒充真实 API/Worker 几何验收。
- `TestMissingThumbnailRepairsWithoutOpeningDocument` 用测试自有短期 SQLite/制品对象验证直接 Workspace 创建的 Part/Product，不调用打开/编辑即可得到 pending；重复请求只创建一个 Job，ready PNG 随后可读且 Head 不变。真实浏览器额外将首个列表响应保持 pending，验证卡片发起后续读取。
- 已存在的真实文档 `01a14949-cfc1-7954-b280-aa632796fbaa` 的预览读取实测 `pending → ready`，Head 前后均为 `01a14a21-3d62-7ab5-81b6-51094a539eb6`。证据 `viewport-perspective/preview-result.json`、`empty-scene.png`、`document-preview.png`；没有进入编辑器或写入模型命令。

### 扩大检查限制

`20261017T070510-89fc9cab/report.json` 的 services scope 原生构建通过；Go packages 出现两项失败，conformance 未执行；期间继续修正 Web 导致总状态为 `SOURCE_CHANGED`，因此不作为完整服务端通过证据。

- 实时数据库用例等待 `job.state.changed.v1` 超时；代码固定后，该用例与预览用例连续 3 次定向复测通过，不据此宣称消除了所有时序问题。
- `TestCppWorkerShellAfterFilletAndMixedLoft/offset-rectangle-larger-circle` 返回 `SHELL_OFFSET_FAILED`、`SHELL_OFFSET_TOLERANCE_EXCEEDED`、`EMPTY_RESULT`。本轮没有修改 Shell/OCCT/Geometry Worker 算法；这条失败保留在完整原始日志，不把它报告为通过。

未执行完整仓库 scope all、PostgreSQL 专项或所有浏览器用例。使用现有浏览器，没有下载依赖或重置开发数据库/制品；按需生成缩略图并创建本轮测试文档，后台已重启应用新实现。
