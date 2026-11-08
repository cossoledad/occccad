# 草图交互与验收

模型、曲线/约束能力、Profile 与限制由 [Part/Sketch](../architecture/current/part-sketch.md)维护；后续建模见[唯一 Feature 能力表](../architecture/current/solid-features.md#feature-能力表)。本页只保存交互与验证入口，测试存在不等于执行通过。

## 当前动作与输入

ToolManager/Input Router 拥有选择、定义、放置、提交和未知回执阶段，工作台只显示 SketchCommandState。单击单次、双击连续沿现有工具策略；状态栏显示当前动作与计数。inline 仅显示工具请求的字段，input.id 控制焦点代际，hover 不重置草稿；Tab 切字段，Enter 接受一次，IME/下拉消费自己的键盘，失焦不提交。

线性/角度/半径尺寸创建和编辑共用尺寸面板。工具确定引用、线长/点距/平行线距和放置位置后打开面板；值/表达式、名称、参考、停用与删除直接呈现。尺寸和普通逻辑约束的引用均可定位并按角色合法更换，更换尺寸引用保留原参数身份与公式；内部生成尺寸的引用由其来源操作拥有，取消保留已接受引用和草稿。长度使用文档单位、角度用 deg，显示格式不改 Quantity/AST 或 ParameterID。

文字拖动只调整本地布局，不改变尺寸值、角度区域或正式历史；偏移附着几何锚点，几何移动时文字跟随。角度通过弧线方向/区域操作修改其持久语义，反角不以移动文字偷换。

镜像先选轴再选来源集合；修剪默认删除命中区间，保留命中/仅打断是明确变体。圆角/倒角按邻边点击确定分支，接受引线参数，hover 不改已接受分支。多边形在光标处调边数后一次创建。真实捕获、稳定端点和正式约束的生成合同见 [Part/Sketch](../architecture/current/part-sketch.md#草图模型与求解)。

直线、圆和矩形按鼠标点位创建；绘制时不显示尺寸数值框，数字键不创建尺寸约束。需要精确尺寸和公式时使用正式尺寸工具。编辑工具的距离/圆角数量、多边形边数仍可输入。

退出草图后再启动拉伸/切除/旋转；草图工具栏、搜索与树插入菜单不混合实体生成命令。测量、参数与接口、视图是独立分类。生成器的“编辑输入草图”及新建时的“创建输入草图”会保留当前草稿，结束草图后返回；取消特征草稿不会撤销期间已经确认的草图操作。

## 预览与取消

只读生产 Preview 返回精确候选，不写 Revision；近似候选不能作为正式结果。旧文档/草图/Revision/会话的结果丢弃。面板输入屏障阻止几何拖动/Delete，保留导航；Esc/右键先退出更换子会话，菜单/IME 消费的 Esc 不级联。取消工具清理选择、hover 与临时显示。

提交未知时 Engine 保存原 requestId、baseVersionId 和 operations；只查询/重试原回执，不生成重复操作。关闭面板不撤销已发送请求，待确认期间仍阻止新编辑；已确认提交后显示失败不能重新执行业务命令。

在“零件依赖”中先选失效特征或草图，使用“健康历史对照”查看原引用、尺寸来源和历史形状。历史几何只读；切回当前模型后进入原对象修复，重新选择合法支撑、投影来源或轮廓区域。搜索范围可调整，未找到历史不代表此前所有版本均无健康定义。

## 验证入口

| 范围 | 可执行入口 |
|---|---|
| 指针绘图、权威预览和草图/实体往返 | Web `sketch-creation`、`sketch-creation-preview`、`solid-sketch-workflow`；Control `TestSketchDimensionsDriveSolidThroughRouter` 检验显式尺寸；浏览器 `sketch drawing stays pointer based`、`a solid draft creates` |
| 工具、输入路由与取消 | Web `sketch-input-session`、`sketch-edit-session`、`sketch-continuous-selection`、`sketch-inline-ui` |
| 捕获、角色优先、交点邻域与迟到响应 | Web `sketch-capture-angle`；Control `sketch_interaction_integration_test.go` |
| 尺寸表达式、逻辑引用更换和回执 | Web `sketch-dimension-lifecycle`、`sketch-engine-commit`、`sketch-edit-preview` |
| 精确曲线、闭合与拉伸 | Workspace `sketch_workflow_integration_test.go`、`TestSketchDirectProductionToolReplay`；Control `sketch_split_delete_integration_test.go` |
| 显示、文字布局与手柄 | Web `sketch-feedback`、`sketch-manipulator`；浏览器 `solid-feature-live.spec.ts` |

运行 `mise run check --scope sketch`、`mise run check --scope web`；真实 Router/Worker 及浏览器配置见[测试路由](../../tests/README.md)与[Web README](../../web/apps/cad/README.md)。Skip、未运行、Node/Mock 通过均不等于浏览器/WebGL 验收。

## 人工验收

1. 捕获直线内部/中点、圆心/端点后检查正式关系；A/Alt 不产生重复关系。给线上点添加端点到点距离，交换创建顺序和缩放后仍能选点。
2. 两圆或两条开口弧在内部交点分割一条/两条；删除指定段，检查剩余端点正式连接、精确闭合并拉伸。高缩放与重叠候选下检查交点，不按坐标补连接。
3. 长度/平行线距、半径及四区域角度（含反角）使用统一面板；反转线端点、交换引用后显示与求解一致。移动文字只排版，移动图元则文字跟随。
4. 连续直线/圆弧、圆角/倒角、快速修剪、延伸、关联镜像、椭圆弧/样条和多边形变体；取消不留半成品，非法参数原子拒绝。
5. 删除投影、分割段或整个对象，确认引用影响和约束连接；视图区/树结果相同，Undo/Redo、重开后身份和几何保持。Product 内编辑 Part 重复关键步骤。
6. 尺寸/命令面板打开时导航、IME、Enter/Esc、未知回执恢复；检查小窗口、缩放、长名称、构造线、点、捕获和尺寸层级。真实操作及故障注入分别记录实测范围。

从拉伸/旋转面板选择“创建输入草图”时，新特征在当前创建位置建立草图；既有特征在自身之前建立输入。结束草图后返回原面板，保留未提交尺寸和公式。取消面板会保留已确认的草图操作。结构树“在此步骤后插入”也提供旋转、孔、放样及更多合法拉伸范围；其视图区只显示、捕获该插入位置的上游。

画普通直线时，状态区会显示水平、竖直、平行或垂直推断。确认即创建该正式关系；可在“工具选项”关闭方向推断，或按住 Alt 临时关闭。已捕获端点优先。“草图诊断”提供待处理问题、全部约束、自由度三个表格页签；选中高亮，定位调整视图，行内编辑或停用/启用关系。

实体尺寸与公式在表单修改，不提供视区尺寸手柄。扫掠路径草图/曲线和薄壁线链从视区选择；失败与下游修复警告走全局提示。

Sketcher 的测量分类集中现有线性、角度及圆/椭圆尺寸工具；几何约束仍在草图分类。三维精确测量在 Part/Assembly 使用，不把该入口当作二维草图测量。
