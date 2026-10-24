# 工作台 UI 规格与组件入口

保留浅色操作区、深色顶栏与视口。呈现层不改变命令步骤、单次/连续、选择来源、参数原文或事务语义。几何选中仍使用橙黄色，工具激活使用蓝色。

## 操作与源码入口

全局快捷键由 [command-shortcuts.ts](../../web/apps/cad/src/cad/command/command-shortcuts.ts)拥有，经 CommandRegistry 检查可用性：Ctrl/Meta+Z 撤销，Ctrl/Meta+Y 或 Shift+Z 重做，Ctrl/Meta+K 搜索，V 选择，F 适合窗口，1/2/3/4 等轴/前/顶/右视图，L/C/R 直线/圆/矩形，Shift+/ 帮助。输入框和 IME 不触发全局命令；工具内部按键归各工具状态机，导航按键见[导航说明](../../web/apps/cad/src/cad/navigation/README.md)。

命令目录投影从 `workbench-command-model.ts` 进入，命令注册从 `workbench-command-registration.ts` 进入；标签、临时会话、树/Inspector 的边界见[编辑上下文](../architecture/current/product-edit-context.md)，不需要整读主 orchestrator。

## 公共视觉合同

| 公共来源/组件 | 当前合同 |
| --- | --- |
| `web/apps/cad/src/design/visual-tokens.ts` | 唯一 palette 与尺寸/字体/阴影/层级 tokens；CSS、Antd ConfigProvider、Canvas 各自适配 |
| `src/design/design-system.css` | 公共控件、菜单、面板、内联编辑与通知样式；不依赖 Portal 前的父节点 |
| `features/workbench/workbench.css` / `toolbar-pages.ts` | 分类 Tabs 36px；Ribbon 88px、两行30px；分组竖线；按目录顺序整组分页，超宽组按双行列拆分；状态栏32px |
| `cad/overlay/tool-button.tsx` | Antd Tooltip 固定锚点；保留220ms单击/双击分辨与帮助行为 |
| `cad/overlay/cad-split-tool-button.tsx` | Compact + 主按钮88px + Dropdown箭头24px，统一30px高；激活、记住默认、菜单展开独立；箭头取消尚未执行的主区点击 |
| `cad/overlay/floating-panel.tsx` | CommandDialog S380/M520/L960；标题44/body16/footer52；Host内定位/拖动；只在挂载到Host后测量；body字段初始焦点 |
| `cad/sketch/sketch-inline-parameter-input.tsx` | 当前参数160×32白色框；自动聚焦；实测边界避让12px；只读DOM层保持透明；不写模型placement做视觉避让 |
| `cad/rendering/dimension-label-textures.ts` | 14px/500透明字形，不绘制背景或深色轮廓；按文本/字体/DPR复用；有界缓存与引用释放 |
| `cad/command/operation-feedback.tsx` | 业务失败右下360px；显示已知原因与恢复动作；未知提交结果的恢复通知不自动关闭；原始信息集中到属性开发区 |

字体为 Segoe UI、PingFang SC、Microsoft YaHei、Noto Sans CJK SC、system-ui、sans-serif。普通UI13px，工具栏/状态/说明12px，标题14px/600，分组12px/500，尺寸14px/500，内联数值15px/500。数字使用 tabular-nums。公共工具颜色过渡140ms，关闭Antd默认缩放/位移动画；减少动效偏好下关闭剩余非必要过渡。普通控件32px、状态动作24px、控件圆角4px、面板/菜单8px；间距4/8/12/16/24px。

色板、三档阴影均在 tokens 中。primary 从规格 `#3868DF` 最小调整为同色相 `#3666DB`：在 `#EAF0FF` 上对比由4.37:1提高至4.51:1。正文与次级文字在白色/浅色操作区均达到4.5:1。只读尺寸 `#F4E6BC`、参考尺寸 `#CFDBE7` 直接显示透明背景文字，不加深色轮廓；诊断色不被选中着色覆盖。尺寸继续附着支持平面、沿引线并保持正读与像素尺寸。

层级为视口0 < 视图控件20 < 内联30 < 非模态面板40 < 弹层600 < 全局模态1000 < 通知1400。Antd 上下文为模态所属弹层抬升层级；Dropdown挂载于非滚动顶层，不全局指定 trigger.parentElement。菜单/列表键盘先归UI处理，Esc关闭菜单后恢复箭头焦点，下一次Esc才归命令；参数与IME继续走既有输入屏障。

新增面板默认在视口右上内侧16px，大面板工作台水平居中且避开命令区；已有保存位置按实测尺寸夹取。所有普通表单使用原有 Antd/Quantity 输入，表达式不转为InputNumber。尺寸编辑保持值/表达式、名称、定位、参考、停用、删除；逻辑约束合法引用更换继续存在。

## 命令表单与拾取

`cad/overlay/command-form.tsx` 的 `CommandField` / `CommandFormItem`、`CommandGroup`、`CommandStatus` 统一字段、Ant Form 校验、分组与计算状态；`FeaturePickField` 是引用字段和列表条目的唯一基本外观。装配支撑、接合端点、特征修复及有序截面复用它，几何合法性与持久绑定仍由各命令负责。草图连续工具保留自己的阶段操作，字段沿用公共布局。

标签列88px，紧凑数值112px、常用枚举160px，均受可用宽度限制；名称、表达式和选择列表伸展。普通控件32px，引用行内动作24px、最小48px，底部确认/取消最小64px，按钮按内容排布。复选项和开关不拉伸；长名称省略但保留完整 tooltip；多项枚举允许换行。不以父容器拉伸或全局固定宽度替代字段规则。

同一命令只有一个拾取字段。单选成功结束该字段；无序多选点击增减，有序截面按加入顺序保存并提供换序。更换开始时保留原引用，只有合法新选择才替换；取消更换保留原值，移除／清空显式清除并结束拾取，必需输入缺失时不能确认。集合入口显示“增减”，单项入口显示“更换”。定位结束拾取并高亮原对象，不作为新输入。树与视图共用 `FeatureSelectionSession` 校验；异步绑定受字段、会话及模型版本约束，迟到结果不得写回或结束新字段。

Esc 依次交给菜单、当前拾取、命令。Enter 排除 IME、重复按键、下拉候选和多行输入；普通输入在解析与状态更新后至多确认一次，非法数值中间串不能确认旧值。失焦只更新草稿／预览，不提交模型。确定仍依赖当前输入的有效候选，关闭清理临时引用与预览。重新预览、应用并继续等辅助操作归入 `CommandDialog` 底部；只读分析保留刷新／取消分析语义。

字段附近显示明确的本地校验原因，服务失败经公共错误映射；不把本地错误字符串交给文案白名单猜测。仅保留缺失输入、计算中、过期和实际失败，移除“点击添加选择”“自动预览”“预览成功可确认”等常驻教学。

## 工程信息表达

- 模型命令与分析使用 `CommandDialog` 的焦点、拖动、位置记忆、边界和 Esc；文档导航／全局选项仍用全局容器，不另造命令弹窗。视口拾取不被遮罩阻断。
- 操作失败统一进入右下角公共通知，包括进入／退出机构、异步更新及命令失败；禁止把错误 Alert 插在工具栏与工作台布局之间。字段校验保留在所属字段，持续工程状态留在所属面板；历史查看等模式提示不占用页面布局高度。
- 首屏先给对象名称、工程状态／数值和下一步动作。名称由对应文档、实例路径和稳定引用解析；重名实例补充路径或实例序号，缺失引用明确标为丢失，禁止用完整／截断 ID 兜底。
- 普通表格、选择提示、属性、通知、tooltip 不呈现 UUID、Revision ID、digest、原始枚举／异常、JSON、诊断复制和实现过程；版本任务保留名称、时间及更新状态。映射来自现有状态和错误证据，未知如实说明，不猜测错误原因。
- 属性面板仅保留一个默认收起、按需读取的“开发信息”，容纳完整对象、最近结果／失败、请求耗时、查询调试及助手技术调用记录；其余界面不再放技术弹窗或折叠区。开发数据不替代普通工程属性。
- 机构打开／切换自动分析或复用当前版本结果；同一输入合并在途计算，取消、关闭和版本变化阻止迟到结果恢复。相对自由度与整体运动分开。选中运动零件联动直线平移箭头／真实轴线处的旋转弧箭头；固定、未知、构型不匹配不显示误导标记。
- 所有精确值、引用身份、权限、求值、预览确认、未知回执恢复、Undo/Redo 均保留原合同；展示名称和舍入值不得回写计算输入。

## 定向验证与人工检查

公共交互入口：`workbench-visual`、`workbench-more-commands`、`sketch-inline-ui`、`sketch-input-session`、`sketch-linear-input`、`sketch-continuous-selection`、`sketch-direct-workflow`、`sketch-feedback`、`sketch-dimension-lifecycle`、`assembly-engineering-panel` 场景；执行 `pnpm typecheck` 与 `pnpm build`。测试通过不等于WebGL实机验收通过。

人工在1920×1080、1366×768、1024×768及125%/150%缩放检查：

1. Part/Sketcher/Assembly切换分类、逐页切换到末组；圆/圆弧/样条菜单靠左右边缘均可打开。主区单击/双击、主区后立即点击箭头、禁用/帮助模式保持原行为。菜单Esc只关闭菜单，Tooltip不挡点击。
2. 打开拉伸M、装配约束M、分析L、尺寸S；初始焦点为字段，Enter一次提交，IME不提交；拖动后重新打开保留位置，小窗口关闭入口可达。
3. 圆角R和尺寸内联输入在边缘稳定避让；输入表达式与非法中间串不丢草稿，失焦不提交。只读文字没有重复DOM副本；观察深背景、浅实体、选中、参考、错误、斜视与缩放。
4. 错误说明位于右下状态栏上方；正常通知没有技术详情或诊断复制入口。未知结果通知不自动消失，关闭通知后状态栏恢复动作仍在。正常完成/取消不新增成功或错误通知。
5. 沿既有矩形→四角R5→长度80/间距50→圆分割/修剪→构造弦镜像→拉伸流程检查，确认没有新增步骤。

运行截图和临时检查记录放在派生 `build/ui-unification/`，不加入长期架构历史。Mock/软件WebGL截图只能证明呈现与前端接线；真实服务、原生GPU及几何/历史闭环另行实机验收。

工具按钮宽88px，分体按钮宽112px（箭头24px）；命令区按可用宽度分页，上一页/下一页替换整页内容；整组优先，单组超宽时按双行列拆分。切页不改变工具状态，所有命令继续可搜索。组间分隔使用蓝灰色 `#8A9FBE`。

数值显示统一使用 `utils/display-number.ts`，最多两位小数，省略尾零；输入中的草稿、表达式原文和模型计算精度保留。普通数字输入使用 `cad/overlay/cad-number-input.tsx`；formatter 保持稳定身份，失焦或 Enter 对未修改的舍入显示不形成新数值意图；实际输入和原有 parser、范围校验继续生效。未修改的舍入显示不得写回参数或拉伸长度。
