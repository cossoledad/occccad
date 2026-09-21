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
| 居中/选旋转参考 | 中键单击将该位置居中 | 中键单击几何只选临时参考，不跳动视图；随后中键拖动 |
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

3DEXPERIENCE CATIA 旋转球默认隐藏，只在用户开启且正在旋转时显示；平移和缩放不绘制球面提示。偏好通过现有 UI preferences 持久化，沿用 `catia` 模式 ID。

公开文档没有给出鼠标增益、自动旋转中心排名、虚拟球投影算法和精确像素规格。本实现的球半径、速度曲线与自动选心是明确的应用实现；未通过原版软件同轨迹录制对照，不能称为逐像素/逐数值完全一致。任意曲面/曲线的通用参数轴、CATIA Robot、Shift 视点控制及文档中的键盘旋转/平移、SOLIDWORKS 自定义右键手势菜单、命名 Camera 的 Ctrl+Alt 转头、Scene Floor、工程图和三维鼠标驱动不属于本次基础原生三键/Part/Assembly 导航实现。

`testing/navigation.scenario.mjs` 验证状态转换、旋转/平移/缩放、居中、临时引用、取消、轨迹球与实际显示拾取。`browser/navigation.spec.ts` 通过 Chromium 的真实组合按键验证输入边沿、修饰键切换、失焦和 UI 偏好；输出两套模式的截图。浏览器测试启用已有 `VITE_INPUT_DEBUG`，生产构建不显示调试面板。

## 投影、显示比例与草图会话

视口使用正交相机，初始方向为 (1, -1, 1) 的等轴测。旋转后的任意方向仍使用正交投影；等轴测是特定朝向，不是对自由旋转的限制。`orthographic-view.ts` 统一标准方向、屏幕关注点、相机空间 Fit 与草图前视图快照。等轴测命令（1）在更新 quaternion 后执行可见内容 Fit；其他标准方向只调整朝向。Fit 只计算可见内容，按宽高比容纳相机空间的八个包围盒角点。

平移和拾取共用 `worldUnitsPerCssPixel`，正交相机按可见高度/zoom/视口高度计算，避免世界原点距离影响操作精度。锚定缩放同时更新相机与 pivot 的平面位移。局部放大时 Default 优先绕命中的局部几何旋转；CATIA 在开始旋转时使用屏幕中心可见几何的深度，避免位于实体内部的旧旋转中心甩动表面细节。旋转角度按屏幕输入计算，不按模型坐标距离任意放大速度。

进入草图保存一次相机位置、四元数、up、pivot、zoom，将当前关注区域投到支撑平面后正对该面，保留比例；同一草图的重复激活/刷新不覆盖快照或重置视角。退出恢复快照；切换文档清除旧会话快照。模型改变后的自动刷新不执行 Fit。

`testing/orthographic-view.scenario.mjs` 验证多数量级缩放时的屏幕平移精度、缩放锚点、重复标准视图、不同宽高比/远离原点的 Fit 和草图视图往返。浏览器另外覆盖快捷键 1、局部放大及基准面/实体面支撑的草图进出（Mock 的面支撑不代表真实后端几何精度）。

正交裁剪按当前几何的相机空间深度和视图尺寸自适应扩展，near 为 0；若几何在旧相机位置之后，只沿视线后移相机，保持屏幕位置/比例。far 使用覆盖实际深度的有限值，避免 Infinity 破坏投影矩阵或过大的常数降低深度精度。`InfiniteGroundGrid` 在背景通道投影无限 XY 平面，不参与模型包围盒、拾取或深度裁剪；随显示比例调整网格密度，几何正常覆盖网格。完全侧视地面时其投影面积为零，网格隐藏；草图编辑使用相同无限网格实现，基于活动支撑平面的 origin/u/v/normal 投影；世界网格持续保留。

裁剪更新同时支持沿视线前移/后移相机；大幅缩小后重新放大时收回多余深度范围，避免裁剪范围随操作历史不断扩大。该沿视线位移不改变正交屏幕投影。

草图工具栏的“正对草图平面”通过 CommandRegistry 调用 `normalToSketch`，仅重新对齐当前视图，不修改草图几何、显示比例或退出时恢复的快照。背景方向参考达索 [Overloading Predefined Ambiences and Cameras](https://help-3dexperience.aesvietnam.com/English/ComUserMap/com-t-View-AmbienceOverload.htm) 的环境上方向与 Zenith/Horizon，以及 [About the Ambience Panel](https://help-3dexperience.aesvietnam.com/English/AstUserMap/ast-c-Ambience.htm) 的穹顶和 Horizon 语义（公开原文镜像）。实现用世界 Z 方向的环境采样区分天空、地平线和地面；正交视图背景使用固定角域，避免缩放改变环境方向。色值、角域和插值是应用参数，不是 CATIA 专有 shader 的复刻。

3DE Profile 的平移、虚拟球角度和组合键缩放采用 1.15 输入增益，相对原实现小幅提速；平移仍按当前正交 zoom 换算，不改变其他 Profile。

通用“法线视图”（`view.normal`）位于所有工作台的视图命令组：先选择基准面或实体平面，再执行命令。实体面通过当前选中 Revision 的 topology properties 获取精确 origin/normal/xDirection，曲面明确拒绝；Viewport 将局部平面转换到 occurrence 的世界坐标（包含嵌套 placement），保留 zoom 并将当前关注点投影到平面。请求期间切换选择、文档或 Revision 会废弃旧响应。该命令只改变相机，不创建 Revision、不修改草图退出快照。

平面相关命令使用 `orientPlaneView`：在 ±法向中选择当前视线最近的一侧，以最短 quaternion 旋转携带相机 up，避免进入草图或正对面时出现背面翻转/无谓滚转；不修改模型平面方向。标准 Top/Front 等固定视图仍用 `orientView`。
