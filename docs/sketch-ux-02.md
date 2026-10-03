# 草图交互补齐与验收

本页记录当前交互合同及定向验证入口，几何支持组合见[二维草图能力](sketch-capabilities.md)。Node 场景、类型检查和构建不代替浏览器/WebGL 人工验收；本轮未进行浏览器自动测试。

## 当前合同

- 服务端拥有模型、数量/表达式、Revision 和最终求解；工具拥有选择/定义/Placement/提交阶段及冻结手势基线。工作台面板显示 `SketchCommandState`，不另建权威状态机。
- 尺寸 Placement 冻结种类与 typed 引用；标注位置、几何拖动和数值编辑分别提交。单线长度和两点/两线距离通过明确选择定义，hover 其它对象不能偷偷改定义。
- 尺寸对话框以值/表达式、驱动参考状态及引用为主；名称、停用和删除收纳于次级设置。角色槽位 `#1/#2/...`；编号表示当前约束角色，不是实体或点数组位置。高亮、定位和更换共享角色。手动拾取与自动吸附独立；不兼容候选在排名前过滤，稳定端点/编辑点身份失效时拒绝替换。
- 更换、取消和迟到响应不覆盖数值/表达式草稿或 ParameterID。参考草稿随引用重新测量；提交后仍以服务端测量为准。长度输入和显示使用文档单位，角度为 deg；仅重命名不重写原 Quantity/AST。
- 面板开启时阻止其它几何拖动，保留导航。Esc/右键先退出更换子会话；IME 和输入框键盘不泄漏到工具。输入框 Enter 通过公共确认防重，Select、textarea 和 IME 保留自身 Enter。
- 精确候选来自只读生产 Preview，不产生 Revision、历史或业务写入。近似预览明确标识；无效或迟到候选不保留为可确认结果。
- 提交未知时保存原 operations、requestId 和 baseVersionId；重试显式 `retryReceipt=true`。关闭面板不撤销服务器请求，Engine 保留回执并阻止新编辑。已确认提交后的渲染错误不重新执行业务命令。

## 定向验证入口

| 合同 | 生产场景/测试 | 核心断言 |
| --- | --- | --- |
| 工具阶段与指针归属 | `sketch-input-session.scenario.mjs`、`sketch-edit-session.scenario.mjs` | 真实 InputManager → Router → ToolManager；非空预选、Placement 不变、完成后 pointerup 不进入新 Select、Esc/blur |
| 尺寸编辑及引用更换 | `sketch-dimension-lifecycle.scenario.mjs` | 真实角色验证、stable refs、替换后测量、表达式/ParameterID 不变；版本/代际/取消；英寸、重命名；生产 React 面板和 Enter 防重 |
| 面板屏障及未知回执 | 同上、`sketch-input-session.scenario.mjs` | 面板/未知回执阻止拖动和 Delete，导航可用；取消/丢失 capture/迟到 up；原操作和请求身份重试、不重复创建 |
| 命令入口与 External Projection | `sketch-input-session.scenario.mjs` | 全部暴露编辑命令均发布阶段并由 Esc 退出；外部投影确认失败保留原 stable topology ref、成功后才完成、取消后不接迟到结果 |
| Engine 上下文与回执 | `sketch-engine-commit.scenario.mjs` | 真实 Engine 提交/渲染失败区分、同请求回执；编辑文档/草图/版本变化隔离与角色筛选 |
| 编辑面板与预览 | `sketch-edit.scenario.mjs`、`sketch-edit-preview.scenario.mjs` | 正式工具选项/字段、近似几何与精确候选区分、取消与版本变化 |
| 只读生产候选 | Go `TestSolvedSketchPreviewCandidatesProjection`、`TestSketchProductionPreviewQuickTrimReadOnly`、`TestSketchProductionPreviewRejectsInvalidExtrusionWithoutWrite` | 成功候选来自权威解；Head、模型与历史不变；失败无写入 |

从 `web/apps/cad/` 运行 `pnpm test -- sketch` 和 `pnpm typecheck`；生产构建使用 `pnpm build`。Go 从 `services/` 定向运行 `go test ./internal/workspace -run 'Test(SolvedSketchPreviewCandidatesProjection|SketchProductionPreview)' -count=1`。生产 Preview 场景需要匹配源码的 Worker 和专用隔离测试数据库，配置缺失的 Skip 不计通过。实际结果保留在本轮派生验证记录，不以测试入口存在推断全部组合通过。

## 人工验收

1. 关闭自动吸附，仍手动选择端点、中心或直线。双预选平行线创建距离；单线创建长度。进入放置后 hover 其它曲线，定义和高亮保持原引用。
2. 进入尺寸编辑，输入表达式但不提交；在 `#2` 点击更换。候选须按角色过滤；确认后草稿表达式不丢失。Esc 或右键取消更换保留父面板，再一次 Esc 关闭父面板。
3. 定位/悬停每个角色槽位，检查画面对应编号。验证端点与整体曲线、FIT 拟合点与 CONTROL 极点名称，没有实体数组编号或 UUID。
4. 文档长度单位改为 in，创建/编辑 2.5 in 尺寸，参考测量显示 in。仅改名称后确认并重新打开，原 ParameterID、数量/表达式来源保持。角度仍使用 deg。
5. 在面板开启时尝试拖动已选择实体、按 Delete；模型不变。中键导航仍可用。IME 输入、Select 候选 Enter、textarea Enter 不确认模型；普通尺寸输入框 Enter 只提交一次。
6. Trim、Mirror、Corner、Chamfer 等编辑通过工具面板填写字段/选项，查看近似或精确候选状态。非法值、取消、快速切换草图/版本后不留下旧候选，也不误提交。
7. 在隔离测试环境模拟服务端已处理但响应丢失，检查“结果待确认”。关闭父面板后仍能查询原请求回执；重试不新增重复实体或 Revision。恢复后新的普通编辑可用。
8. 在 Product 内编辑 Part 草图重复上述操作，确认宿主、标签、编辑归属和显示稳定；Undo/Redo、重新打开后正式结果一致。
