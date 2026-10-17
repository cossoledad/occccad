# 鼠标导航

本模块只修改视图，不修改模型、装配 Placement、选择集或 Revision。用户偏好提供 occccad、3DEXPERIENCE CATIA 和 SOLIDWORKS 三种模式。

## 行为基线与来源

CATIA 基线为 **3DEXPERIENCE CATIA 原生应用**的标准三键导航。用户已确认产品代际；没有指定年度版本或个性化设置。以下链接是达索帮助原文的公开镜像，不是达索官网，公开页未标明年度版本：

- [Basic Viewing Commands](https://help-3dexperience.aesvietnam.com/English/ComUserMap/exp3dbasics-r-BasicViewingCommands.htm)：中键平移/居中、中键加左键旋转、保持中键并单击左键后拖动缩放、Ctrl+中键缩放。
- [Navigation preferences](https://help-3dexperience.aesvietnam.com/English/PreferencesMap/preferences-c-Display-Navigation.htm)：`Disable the rotation sphere display` 默认选中。因此本模式默认隐藏旋转球，偏好中的“显示旋转球”可重新开启，不影响手势数学。
- [Manipulating the Viewpoint](https://help-3dexperience.aesvietnam.com/English/ComUserMap/com-t-View-ReframeOn.htm)：Shift+中键是 Look At 视点操作；尚未实现，不能当作标准平移等价物。
- [Manipulating Viewpoints Using the Mouse and Robot](https://help-3dexperience.aesvietnam.com/English/ComUserMap/com-t-Robot-ViewpointManip.htm)：Robot 的轴/弧/平面是另一组视点操纵器，目前没有复刻。

不要混用 [Dashboard Mouse Controls](https://help-3dexperience.aesvietnam.com/English/SharedUserMap/shared-services-r-NavigationModes.htm) 中名为“3DEXPERIENCE”的网页配置与原生 CATIA 配置。两者文档适用范围不同。原生基本操作表没有列出三维滚轮缩放；当前仍使用组合键缩放，但不再据 V5 文档声称所有 3DEXPERIENCE 应用禁止滚轮。

此前经本机 B33 `English/control/BasEnglishC2.viewdoc` 查阅的 [V5 Activating Viewing Tools](https://catia-v5-help.anarkia333data.center/online/basug_C2/basugbt1201.htm) 与 [V5 Navigation](https://catia-v5-help.anarkia333data.center/online/bascugen_C2/bascudisplay0200.htm) 仅作为兼容补充：右侧键替代左键、Alt+右键两键操作及可选球面导引的外观来自该基线，未核实为所有 3DEXPERIENCE 年度版本的默认配置。可选球面导引保留本项目的虚线圆/切线绘制，不宣称与原生界面像素一致。

SOLIDWORKS 基线为桌面 Design 的 Part/Assembly 三键鼠标默认导航，关闭 Rotate about Scene Floor，滚轮以指针为中心。参考官方文档（可访问内容的版本为 2023/2024；不是 Composer 或 Visualize 的默认模式）：

- [Middle Mouse Button Functions](https://help.solidworks.com/2023/english/Solidworks/sldworks/r_Middle_Mouse_Button.htm)：旋转、Ctrl 平移、Shift 缩放、先中键单击实体再旋转。
- [Rotate View](https://help.solidworks.com/2024/English/SolidWorks/sldworks/t_rotate_view.htm?format=P&value=)：实体旋转与 Scene Floor 的区别。
- [Roll View](https://help.solidworks.com/2021/english/SolidWorks/sldworks/t_roll_view.htm)：Alt+中键滚转。
- [官方视图操作说明](https://blogs.solidworks.com/products/solidworks/how-do-i-manipulate-my-model-view-let-me-count-the-ways/)：完整模型以中心旋转；局部可见时选可见几何作为中心；临时参考的洋红色与带绿色轴线光标；旋转结束解除参考；草图编辑不启用此参考选择。
- [官方教育教材](https://files.solidworks.com/education/curriculum/subscription-curriculum/Fundamentals3DDesign_SIM_ENG_LV_2023_final.pdf)：圆边和圆柱面可以作为旋转轴参考。
- [官方快捷操作说明](https://blogs.solidworks.com/products/solidworks/shortcuts-for-solidworks-beginners/)：中键双击适合窗口。

## 实现映射

| 操作 | 3DEXPERIENCE CATIA | SOLIDWORKS |
|---|---|---|
| 平移 | 中键拖动；Alt+右键拖动 | Ctrl+中键拖动 |
| 自由旋转 | 先中键，再按住左/右侧键拖动 | 中键拖动 |
| 连续缩放 | 保持中键，按下再释放侧键后拖动；Ctrl+中键 | Shift+中键拖动 |
| 缩放方向 | 上移放大，下移缩小 | 上移放大，下移缩小 |
| 居中/选旋转参考 | 中键松开时拾取并居中；平移持续更新中心深度 | 中键单击几何只选临时参考，不跳动视图；随后中键拖动 |
| 滚轮 | 当前禁用，采用原生文档列出的组合键缩放 | 指针锚定，向后放大；支持浏览器 deltaMode 单位 |
| 滚转 | 从轨迹球范围外拖动（提示可隐藏） | Alt+中键水平拖动 |
| 适合窗口 | 现有视图命令 | 中键双击，或现有视图命令 |

CATIA 的侧键释放会锁存缩放；再次按住侧键回到旋转，释放主键结束。两键替代操作中，先 Alt+右键、再 Ctrl/左键进入旋转，释放 Ctrl/左键进入缩放；一开始就按 Ctrl+Alt+右键则直接缩放。

SOLIDWORKS 旋转参考与正式 Selection 分离。平面/直边按显示几何识别轴，圆边与圆柱面通过经过验证的采样拟合识别中心轴，点绕选中位置自由旋转；模型刷新、换模式、取消或旋转结束清理临时引用。拾取排除隐藏对象及被实体遮挡的后方线点。

## 分层与验证边界

- `InputManager` 归一化浏览器 buttons 边沿（组合键不会发第二次 pointerdown）、捕获、焦点、失焦取消。
- `InteractionRouter` 优先路由导航组合，侧键不形成选择或草图实体。
- `NavigationController` 管理模式、共享相机、滚轮与键盘修饰键；两套专用 controller 各自拥有手势状态。
- `ThreeCameraRig` 处理相机数学，`NavigationPicker` 提供显示几何引用。
- `NavigationHUD` 与光标仅展示状态；轨迹球与虚线圆共享半径定义，不成为模型节点。

3DEXPERIENCE CATIA 旋转球默认隐藏，只在用户开启且正在旋转时显示。主键按住期间显示独立的世界方向十字：X/Y 各两段，X 总跨度为旋转球直径的 1/3.5，Y 为 X 的 17/22；中心留白为 X 半长的 5/22。X 为橙色、Y 为青色，正半轴明亮、负半轴压暗，不画箭头；Z 为 X 半长的 8/22 的短黄色线。十字跟随视口尺寸，屏幕中心固定，朝向来自相机逆旋转，不随相机缩放漂移。

可选球面以屏幕中心为圆心，导引十字按轨迹球的半球/外圆映射限制中心，指针越界时沿球边移动。周向导引为球面大圆的屏幕投影，通过积分弧长求显示区间；到球边时与外圆具有相同圆心和半径。径向导引接近球边后向内平移，两段都保持 52 CSS 像素可见长度，全部顶点位于圆内；只有小于 52 像素直径的极小视口才缩短。`hud/navigation-hud-geometry.ts` 仅构造显示几何，不改变轨迹球输入数学。导航光标为透明底、单层圆角描边的 32×32 SVG，热点位于 (16,16)，根据场景明暗选用深色或暖色，无白色外描边。

公开文档没有给出鼠标增益、自动旋转中心排名、虚拟球投影算法和精确像素规格。本实现的球半径、速度曲线与自动选心是明确的应用实现；未通过原版软件同轨迹录制对照，不能称为逐像素/逐数值完全一致。任意曲面/曲线的通用参数轴、CATIA Robot、Shift 视点控制及文档中的键盘旋转/平移、SOLIDWORKS 自定义右键手势菜单、命名 Camera 的 Ctrl+Alt 转头、Scene Floor、工程图和三维鼠标驱动不属于本次基础原生三键/Part/Assembly 导航实现。

`testing/navigation.scenario.mjs` 验证状态转换、旋转/平移/缩放、居中、临时引用、取消、轨迹球与实际显示拾取。`navigation-capture.scenario.mjs` 另外验证浏览器释放侧键时丢失原生捕获的恢复：只有仍持有主键的 CATIA 导航可以续接，工具丢失捕获、pointercancel、blur、Esc 仍取消。`browser/navigation.spec.ts` 通过 Chromium 的真实组合按键验证输入边沿、修饰键切换、失焦和 UI 偏好；输出两套模式的截图。浏览器测试启用已有 `VITE_INPUT_DEBUG`，生产构建不显示调试面板。

## 投影、显示比例与草图会话

视口默认透视相机，35° 纵向视角，ISO 朝向为 (1,-1,1)；全局选项支持平行投影。`projection-camera.ts` 统一投影转换、透视 Fit 和裁剪，`orthographic-view.ts` 的定向/快照适用于两种投影。切换同步 Rig、Picker、Transition，按关注深度保留比例。文档与草图返回快照记录来源投影，恢复时转换到当前全局设置。持久 MotionViewpoint 保留既有平行 framing 合同，捕获/播放在显示边界转换。Fit、顶/前/右视图及等轴测命令先在克隆相机上准备终态，再统一动画；等轴测命令（1）定向后显式 Fit，其他标准方向保留显示比例。首次打开文档的初始化 framing 和已有文档相机恢复保持立即完成。Fit 覆盖所有可见包围盒角点，恒定像素基准只贡献世界原点。

平移和拾取共用 `worldUnitsPerCssPixel`，正交按可见高度/zoom，透视按关注深度与视角计算。CATIA 主键按下只启动手势，不拾取或改变中心；单击松开时优先采用当前可见悬停点，其次取释放位置的显示几何交点，再平移视点将其居中。单击空白保留视图。中键平移时持续采样屏幕中心：表面或邻近顶点提供深度，中心始终位于相机中心射线上，空白沿用当前关注深度；切入旋转时确定中心，缩放锁存沿用它，避免缩放中横向漂移。显示射线不成为持久拓扑身份。Default/SOLIDWORKS 仍保留各自参考点规则。

进入草图保存一次相机位置、四元数、up、pivot、zoom，将当前关注区域投到支撑平面后正对该面，保留比例；同一草图的重复激活/刷新不覆盖快照或重置视角。退出恢复快照；切换文档清除旧会话快照。模型改变后的自动刷新不执行 Fit。

`testing/orthographic-view.scenario.mjs` 验证多数量级缩放时的屏幕平移精度、缩放锚点、重复标准视图、不同宽高比/远离原点的 Fit 和草图视图往返。浏览器另外覆盖快捷键 1、局部放大及基准面/实体面支撑的草图进出（Mock 的面支撑不代表真实后端几何精度）。

默认模式滚轮以鼠标射线上最近的可见 Mesh 表面作为缩放目标，空白使用当前关注深度的视平面；不会采用为旋转中心晋升的邻近顶点或边线。`NavigationPicker.pickSurface` 与点/边优先的 `pickNearest` 分开，后者按最近表面的真实视深度及 camera.zoom 计算 CSS 像素容差，防止高倍率时穿过表面拾取背后点。此引用仅用于显示导航，不形成持久拓扑身份。CATIA/SOLIDWORKS 的按键语义保持各自模式规则。

正交裁剪按当前几何深度和视图尺寸自适应，near=0，可沿视线移动眼点而不改变图像；透视裁剪只调整有限 near/far，禁止借移动眼点改变画面。包围盒接近或跨过眼点时，视口通过中心射线的最近显示表面提供真实视深度参考，再按关注深度和表面深度设置 near；没有命中时使用关注深度，正向包围盒仅作保守约束。包围盒跨过眼点不再直接将 near 压到最小值，否则普通斜视下也会丢失深度精度，背面边线可能穿过不透明实体显示。普通 Fit 视图不需要额外表面查询；裁剪统一在现有渲染帧更新，导航事件仅使帧失效。

`InfiniteGroundGrid` 在重定位后的平面坐标中逐像素计算射线交点与 clip 深度，支持透视地平线和上下两侧观察。地面保留固定世界基准间距，以锚定同一原点的十进制细分/粗级线渐变补充近远参考；地面深度可饱和到实体裁剪边界，保持无限观看环境，不扩大实体相机裁剪。水平视角从可见边界射线选择重定位参考，避免中心交点在无穷远。草图网格按当前平面关注深度自适应，捕捉共用间距。网格不参与包围盒、Fit 或拾取。

裁剪更新同时支持沿视线前移/后移相机；大幅缩小后重新放大时收回多余深度范围，避免裁剪范围随操作历史不断扩大。该沿视线位移不改变正交屏幕投影。

草图工具栏的“正对草图平面”通过 CommandRegistry 调用 `normalToSketch`，仅重新对齐当前视图，不修改草图几何、显示比例或退出时恢复的快照。背景方向参考达索 [Overloading Predefined Ambiences and Cameras](https://help-3dexperience.aesvietnam.com/English/ComUserMap/com-t-View-AmbienceOverload.htm) 的环境上方向与 Zenith/Horizon，以及 [About the Ambience Panel](https://help-3dexperience.aesvietnam.com/English/AstUserMap/ast-c-Ambience.htm) 的穹顶和 Horizon 语义（公开原文镜像）。这些参考用于理解环境概念；当前背景按全局选项使用固定屏幕上下渐变，不随旋转混入地平线/地面色。色值与插值是应用参数，不是 CATIA 专有 shader 的复刻。

3DE Profile 的平移、虚拟球角度和组合键缩放采用 1.15 输入增益，相对原实现小幅提速；平移按当前投影的 CSS 像素尺度换算，不改变其他 Profile。

通用“法线视图”（`view.normal`）位于所有工作台的视图命令组：先选择基准面或实体平面，再执行命令。实体面通过当前选中 Revision 的 topology properties 获取精确 origin/normal/xDirection，曲面明确拒绝；Viewport 将局部平面转换到 occurrence 的世界坐标（包含嵌套 placement），保留 zoom 并将当前关注点投影到平面。请求期间切换选择、文档或 Revision 会废弃旧响应。该命令只改变相机，不创建 Revision、不修改草图退出快照。

平面相关命令使用 `orientPlaneView`：在 ±法向中选择当前视线最近的一侧，再从支撑面的 ±U/±V 向上方向中选择相机总旋转最少的朝向，使坐标轴横平竖直；缺少平面方向时回退到投影世界轴。不修改模型平面方向。标准 Top/Front 等固定视图仍用 `orientView`。

`ViewTransition` 由视口现有 RAF 时钟驱动，280 ms smoothstep 进度，朝向使用 quaternion slerp，相机局部偏移距离与 zoom 使用几何插值；重定位随当前可见跨度推进，避免从细节 Fit 时先将细节移出画面。末帧精确恢复目标快照。Fit、标准视图、进入/正对平面与退出草图共用动画，系统要求 reduced motion 时立即完成；重复命令从当前显示帧接续，pointerdown/wheel/keydown、其他定位命令、换文档和 dispose 取消旧动画。`normal-view.scenario.mjs` 与 `view-command-transition.scenario.mjs` 覆盖轴对齐、两种投影下的全部视图命令、中间帧、接续、取消、细节 Fit 与精确终态。MotionViewpoint 播放仍由其自身时钟逐帧驱动。

本轮另核对本机 B33 `control/BasEnglishC2.viewdoc` 中的 `online/basug_C2/basugbt1201.htm`（中键居中/平移、双键旋转与释放侧键缩放）、`basugbt1208.htm`（旋转视点）、`basugbt1305.htm`（初始地面在模型底部）。文档未规定点/面中心优先级、间隙比例和十字线像素数；这些是结合用户要求明确实现的本软件规则。

投影参考：本机 B33 `control/BasEnglishC2.viewdoc` → `online/basug_C2/basugbt1507.htm`（Perspective/Parallel）；文档确认两种投影的独立选择，默认透视、35° 参数、十字像素形状与取点时机是本次用户要求下明确的项目规则。`projection-camera` 场景覆盖比例、Fit、裁剪、相机往返与 HUD；真实 WebGL 验收覆盖两种投影的网格遮挡。

缩放裁剪参考：同一索引的 `online/basug_C2/basugbt1702.htm`（Setting Depth Effects）说明默认关闭深度效果，缩放时几何不被主动裁剪，Near/Far Limit 随全局包围盒调整；`basugbt1205.htm` 说明增量与连续缩放。文档未公布相机内部公式，本实现的表面锚点、像素容差及 near 约束是项目规则。`perspective-zoom` 场景与浏览器用例覆盖遮挡点、极近表面和大步长缩放。
