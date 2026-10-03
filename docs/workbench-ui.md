# 工作台 UI 规格与组件入口

保留浅色操作区、深色顶栏与视口。呈现层不改变命令步骤、单次/连续、选择来源、参数原文或事务语义。几何选中仍使用橙黄色，工具激活使用蓝色。

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
| `cad/command/operation-feedback.tsx` | 业务失败右下360px；恢复通知不自动关闭；技术诊断640px、日志最多320px；保留回执入口 |

字体为 Segoe UI、PingFang SC、Microsoft YaHei、Noto Sans CJK SC、system-ui、sans-serif。普通UI13px，工具栏/状态/说明12px，标题14px/600，分组12px/500，尺寸14px/500，内联数值15px/500。数字使用 tabular-nums。公共工具颜色过渡140ms，关闭Antd默认缩放/位移动画；减少动效偏好下关闭剩余非必要过渡。普通控件32px、状态动作24px、控件圆角4px、面板/菜单8px；间距4/8/12/16/24px。

色板、三档阴影均在 tokens 中。primary 从规格 `#3868DF` 最小调整为同色相 `#3666DB`：在 `#EAF0FF` 上对比由4.37:1提高至4.51:1。正文与次级文字在白色/浅色操作区均达到4.5:1。只读尺寸 `#F4E6BC`、参考尺寸 `#CFDBE7` 直接显示透明背景文字，不加深色轮廓；诊断色不被选中着色覆盖。尺寸继续附着支持平面、沿引线并保持正读与像素尺寸。

层级为视口0 < 视图控件20 < 内联30 < 非模态面板40 < 弹层600 < 全局模态1000 < 通知1400。Antd 上下文为模态所属弹层抬升层级；Dropdown挂载于非滚动顶层，不全局指定 trigger.parentElement。菜单/列表键盘先归UI处理，Esc关闭菜单后恢复箭头焦点，下一次Esc才归命令；参数与IME继续走既有输入屏障。

新增面板默认在视口右上内侧16px，大面板工作台水平居中且避开命令区；已有保存位置按实测尺寸夹取。所有普通表单使用原有 Antd/Quantity 输入，表达式不转为InputNumber。尺寸编辑保持值/表达式、名称、定位、参考、停用、删除；逻辑约束合法引用更换继续存在。

## 定向验证与人工检查

公共交互入口：`workbench-visual`、`workbench-more-commands`、`sketch-inline-ui`、`sketch-input-session`、`sketch-linear-input`、`sketch-continuous-selection`、`sketch-direct-workflow`、`sketch-feedback`、`sketch-dimension-lifecycle`、`assembly-engineering-panel` 场景；执行 `pnpm typecheck` 与 `pnpm build`。测试通过不等于WebGL实机验收通过。

人工在1920×1080、1366×768、1024×768及125%/150%缩放检查：

1. Part/Sketcher/Assembly切换分类、逐页切换到末组；圆/圆弧/样条菜单靠左右边缘均可打开。主区单击/双击、主区后立即点击箭头、禁用/帮助模式保持原行为。菜单Esc只关闭菜单，Tooltip不挡点击。
2. 打开拉伸M、装配约束M、分析L、尺寸S；初始焦点为字段，Enter一次提交，IME不提交；拖动后重新打开保留位置，小窗口关闭入口可达。
3. 圆角R和尺寸内联输入在边缘稳定避让；输入表达式与非法中间串不丢草稿，失焦不提交。只读文字没有重复DOM副本；观察深背景、浅实体、选中、参考、错误、斜视与缩放。
4. 长错误位于右下状态栏上方，诊断可滚动/复制；未知结果通知不自动消失，关闭通知后状态栏恢复动作仍在。正常完成/取消不新增成功或错误通知。
5. 沿既有矩形→四角R5→长度80/间距50→圆分割/修剪→构造弦镜像→拉伸流程检查，确认没有新增步骤。

运行截图和临时检查记录放在派生 `build/ui-unification/`，不加入长期架构历史。Mock/软件WebGL截图只能证明呈现与前端接线；真实服务、原生GPU及几何/历史闭环另行实机验收。

工具按钮宽88px，分体按钮宽112px（箭头24px）；命令区按可用宽度分页，上一页/下一页替换整页内容；整组优先，单组超宽时按双行列拆分。切页不改变工具状态，所有命令继续可搜索。组间分隔使用蓝灰色 `#8A9FBE`。

数值显示统一使用 `utils/display-number.ts`，最多两位小数，省略尾零；输入中的草稿、表达式原文和模型计算精度保留。普通数字输入使用 `cad/overlay/cad-number-input.tsx`；未修改的舍入显示不得写回参数或拉伸长度。
