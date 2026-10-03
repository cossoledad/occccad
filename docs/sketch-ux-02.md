# 草图交互补齐与验收

本页记录当前交互合同及定向验证入口，几何支持组合见[二维草图能力](sketch-capabilities.md)。Node 场景、类型检查和构建不代替浏览器/WebGL 人工验收；本轮未进行浏览器自动测试。

## 当前合同

- 服务端拥有模型、数量/表达式、Revision 和最终求解；工具拥有选择/定义/Placement/提交阶段及冻结手势基线。状态栏显示 `SketchCommandState` 的当前动作、计数及完成入口；inline 只显示当前请求字段，高级选项按需打开，不另建权威状态机。
- 线性尺寸的首条线先是候选定义；定义阶段可明确选择第二点/平行线，锁定长度入口或空白放置接受单线长度。同屏距离重叠候选优先 PROFILE 子边，但 Construction 可单独选择或主动预选。进入 Placement 后才冻结种类与 typed 引用，hover 其它对象不能改写它们。标注位置、几何拖动和数值编辑分别提交。
- 镜像先选轴，再选/切换来源集合，Enter 或状态栏完成入口提交；结果高亮不作为下一命令的主动预选。普通修剪 hover 显示命中区间，点击删除命中；保留命中、仅打断属于明确选项，不作为删除工具的隐藏默认。圆角/倒角接受两条邻边后显示引线数值，点击或键入编辑当前长度，后续 hover 不改写已接受分支。圆角点击决定支撑/保留侧，连接使用 minor 主值扫掠，远离角点点击不能变成互补 major 弧。详见[当前动作与 inline 输入](sketch-inline-input.md)。
- 尺寸对话框以值/表达式、驱动参考状态及引用为主；名称、停用和删除收纳于次级设置。角色槽位 `#1/#2/...`；编号表示当前约束角色，不是实体或点数组位置。编号只存在于编辑槽位，视图仅高亮引用对象，不显示 #1/#2；高亮、定位和更换共享角色。手动拾取与自动吸附独立；不兼容候选在排名前过滤，稳定端点/编辑点身份失效时拒绝替换。
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
| 直接建模主闭环 | `sketch-direct-workflow.scenario.mjs`、Go `TestSketchDirectProductionToolReplay` | 实际 InputManager/Router/ToolManager 手势产生操作，生产 Go/匹配 Worker 回传权威快照；矩形、四角圆角、长度/间距、圆分割/修剪、构造弦、关联镜像、Profile/Pad 及独立体积断言；新增回放运行结果须独立确认 |
| 只读生产候选 | Go `TestSolvedSketchPreviewCandidatesProjection`、`TestSketchProductionPreviewQuickTrimReadOnly`、`TestSketchProductionPreviewRejectsInvalidExtrusionWithoutWrite` | 成功候选来自权威解；Head、模型与历史不变；失败无写入 |

从 `web/apps/cad/` 运行 `pnpm test -- sketch` 和 `pnpm typecheck`；生产构建使用 `pnpm build`。Go 从 `services/` 定向运行 `go test ./internal/workspace -run 'Test(SolvedSketchPreviewCandidatesProjection|SketchProductionPreview)' -count=1`。生产 Preview 场景需要匹配源码的 Worker 和专用隔离测试数据库，配置缺失的 Skip 不计通过。真实工具回放使用 `go test ./internal/workspace -run '^TestSketchDirectProductionToolReplay$' -count=1`，同样需要匹配 Worker 和隔离数据库。实际结果保留在本轮派生验证记录；新增入口不代表回放已通过，也不代表高阶曲线组合已扩展。

## 人工验收

以下主闭环可在 Part 和 Product 内编辑 Part 的相同上下文重复执行；浏览器/WebGL 实机结果仍由维护者确认。

1. 将显示单位设为 mm，新建草图并画约 120×70 的矩形。观察状态栏当前动作和已接受数量；选择、绘制与数字草稿不产生额外 Revision。
2. 双击启动二维圆角，依次选每个角的两条邻边。输入 5 mm 或有效长度表达式并 Enter，四角连续处理；hover 不重置半径草稿/分支。非法值不提交；输入框 Esc 退出整个工具，失焦不取消工具。
3. 给一条水平边标长度：首线后观察候选定义，点击锁定长度或空白放置，输入 80 mm。另选两条长边直接标法向间距 50 mm。进入 Placement 后 hover 其它对象，引用和种类保持不变。
4. 在外轮廓中部画 R8 圆；分割工具在圆上选两个位置，第一次选择后取消不改模型，完成两次选择后恰为两段圆弧。修剪 hover 必须定位命中区间，点击删除其中一段；没有内部交点的独立命中曲线可整条删除。
5. 用直线连接剩余圆弧两端，并显式转为 Construction。启动镜像先选该构造弦作轴，再选剩余圆弧；查看关联副本预览，Enter 完成。结果高亮不能偷换成下一次命令的来源。
6. 退出草图并拉伸 10 mm。检查外廓为 90×50、一个带内孔的 Profile Region 和一个带贯通孔的有效 Solid，无额外 Body、构造弦切分或重叠实体。外环和镜像形成的孔使用正式连接与同一真实曲线验证；修改长度/间距/圆角半径后实体更新。Undo/Redo、重新打开后身份和结果一致。
7. 编辑已有尺寸，输入表达式后在 `#2` 更换引用；候选按角色过滤，草稿和 ParameterID 保持。Esc/右键先取消更换再退出父面板。角色槽位对应视口高亮/定位，不是实体数组序号。
8. 文档单位改为 in，输入/编辑 2.5 in 尺寸并查看参考测量；仅改名称不改原 Quantity/AST。角度仍为 deg。面板开启时拖动和 Delete 不修改其它几何，中键导航可用；IME/候选 Select Enter 不误确认，普通输入 Enter 只提交一次。
9. 在隔离环境模拟服务器已处理但响应丢失，检查“结果待确认”。关闭面板/退出工具后仍查询原 requestId/operations 回执；重试不重复实体或 Revision，确认结果后恢复编辑。
10. 在 Product 内编辑 Part 重复主闭环和撤销/重做，确认宿主标签、编辑归属与显示稳定；引用影响须明确，不静默删除公式或按最近端点重连。高阶曲线组合沿用现有支持边界。
