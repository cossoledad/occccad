# 曲线与曲面建模参考

本页记录商业 CAD 的公开功能与操作依据；本项目的取舍见[线框与曲面设计方案](../../plans/wireframe-surface-design.md)。商业软件的专有内核算法没有据此验证，不能从功能名称反推其内部实现。本轮未运行 CATIA 或其他商业 CAD 的实际模型。

## CATIA：工作台定位与共同流程

依据本机 B33 文档，Wireframe and Surface Design 面向前期构造几何、基本曲面及与 Part Design 配合的混合建模；Generative Shape Design 面向更完整的线框、曲面构造与修改。两者有大量功能重叠：WFS 的曲面目录已经列出 Extrude、Revolve、Offset、Sweep、Fill、Multi-sections 和 Blend，也列出连接与曲率分析。不能把它们简单分成“没有连续性控制”和“具有连续性控制”两套工具。

GSD 目录进一步列出导向/隐式截面扫掠、变偏移、更多曲线派生、法则、外推及几何操作。具体配置/许可仍影响选项；例如 Multi-sections 页面对特定单截面/单导向配置另列 HD2 要求。本轮比较是手册行为范围，不是许可组合的完整审计。

| 能力族 | 手册中的功能与操作重点 | 对工程建模的意义 |
|---|---|---|
| 构造线框 | 点、有限线、圆/圆锥曲线、样条、螺旋线；曲线上点、投影、交线、平行/偏移、连接曲线、等参数曲线 | 将设计意图建立为可重用、有方向及关联来源的输入 |
| 基础曲面 | 拉伸、旋转、球/柱面、偏移 | 先形成独立面片，再用于其他曲面或实体 |
| Sweep | 区分 profile、guide 和 spine；显式截面可使用一/二条导向，spine 决定扫掠截面平面 | 路径、截面姿态与边界控制是不同输入职责 |
| Multi-sections | 截面有序；导向需与截面相交；支持闭合点、方向、coupling、端部支撑及连续性 | 减少扭转和错误对应，支持上游编辑后的重算 |
| Fill / Blend | 边界与支撑分别选择，逐边设置连续性；Fill 可加入内部通过点/曲线及偏差控制 | 补面需要明确约束和可接受误差 |
| Split / Trim | 选择保留/移除区域，多连通交线可能要求消歧；自动外推是显式选项 | 修剪同时具有几何、分区和用户意图语义 |
| Join / Extract / Boundary | 连接性、流形和相切检查；传播范围、自由边界、合并距离及结果方向可控制 | 整理输入集合、识别开口并建立后续稳定引用 |
| 分析与实体化 | 曲线/曲面连接检查、曲率分析；Part Design 的 Thick Surface、Close Surface | 从质量检查和曲面结果继续形成实体 |

### 值得借鉴的交互

- 输入按角色收集，截面/导向列表可定位、替换、反向与重排；三维标记与列表使用一致的颜色。闭合点、对应点和曲面侧向显式可见。
- Join 的预览可显示自由边界，错误对象在视区高亮；连接、流形、相切是不同检查。合并距离是算法输入，不能被当作模型本来就精确连接的证据。
- Sweep 的 guide 与 spine 不应合成一个含糊的“路径”字段。Multi-sections 的自动对应只是初始建议，已接受的人工对应应保留。
- Trim 有保留/移除侧及结果分区选择；靠近既有边、相切或产生多个交区时，需要明确消歧。
- Geometrical Set 是无序组织；Ordered Geometrical Set 还带有步骤、当前对象、吸收和可见/可访问范围。其插入/删除可触发引用重路由，是独立语义，不应只通过给普通集合加排序号模拟。
- Connect Checker 区分几何邻近与已连接拓扑，并提供距离、相切、曲率等分析。分析结果和构造时要求的连续性需要分别表达。

### 本机 SourceLocation

先检索 `control/WfsEnglishC2.viewdoc`、`SdgEnglishC2.viewdoc`，共用页面再核对 `CfyEnglishC2.viewdoc`，实体化核对 `PrtEnglishC2.viewdoc`。以下路径相对 `/mnt/c/UserApp/3D/DS/Dassault Systemes/B33doc/English/`；MCP 库标识为 `6d5aed22-2df7-427c-83ae-0b4926af15ef`，检索 revision 为 `9058175bec4c4d87f0f646ae`。临时检索 ID 不作为持久来源。

| 页面 | 证据范围 |
|---|---|
| `online/wfsug_C2/wfsugpr01.htm`，Overview / Wireframe and Surface in a Nutshell | WFS 定位与混合建模 |
| `online/wfsug_C2/wfsugbt0100.htm`、`wfsugbt0200.htm`、`wfsugbt0300.htm` | WFS 线框、曲面、工具/分析目录 |
| `online/sdgug_C2/sdgugpr01.htm`，Overview / Generative Shape Design in a Nutshell | GSD 定位 |
| `online/sdgug_C2/sdgugbt0100.htm`、`sdgugbt0200.htm`、`sdgugbt0300.htm` | GSD 线框、曲面及操作目录 |
| `online/sdgug_C2/sdgugbt0204.htm`，Creating Swept Surfaces | profile / guide / spine、支撑与连续性限制 |
| `online/cfyug_C2/cfyugloft.htm`，Creating Multi-sections Surfaces | 截面、导向相交、支撑、coupling 与闭合点 |
| `online/sdgug_C2/sdgugbt0206.htm`，Creating Fill Surfaces | 边界/支撑、连续性、内部约束与偏差 |
| `online/cfyug_C2/cfyugtrimGSD.htm`，Trimming Geometry | 保留/移除、相交分区与外推 |
| `online/cfyug_C2/cfyugjoin.htm`，Joining Surfaces or Curves | 连接性、流形、相切、容差与自由边界 |
| `online/cfyug_C2/cfyugboundary.htm`、`cfyugextract.htm` | 边界及提取、传播范围 |
| `online/cfyug_C2/cfyugorderedgeomsets.htm`，Managing Ordered Geometrical Sets | 无序集合与有序/吸收语义 |
| `online/cfyugfss_C2/cfyugfssut0302.htm`，Checking Connections Between Surfaces | 连接分析类型、间隙与拓扑/几何连接 |
| `online/prtug_C2/prtugbt0710.htm`、`prtugbt0709.htm` | Thick Surface、Close Surface |

MCP 返回的关键原文已取至 `has_more=false`；目录页另外读取了本机 HTML。手册中的默认数值只证明参考软件行为，不成为本项目默认容差或验收阈值。

## 现有命令的 CATIA 操作对照

下表核对每个现有曲线/曲面命令及参考元素的输入流程；原文经 MCP 分页读至 `has_more=false`，路径均沿用上面的本机根目录。采用的是可重用几何与角色选择的工程步骤；本项目按用户要求不沿用 CATIA 的预选、默认法向或自动换型。各行的实现边界以 [current](../architecture/current/wireframe-surfaces.md) 为准。

| 本项目命令 | CATIA 页面与参考行为 | 当前采用及范围 |
|---|---|---|
| 参考点、曲线上点 | `online/cfyug_C2/cfyugpoint3D.htm`：按坐标、曲线/平面、两点之间或中心构造 | 坐标保留于独立参考点；曲线上点选曲线并设置弧长比例；未覆盖任意曲面上点 |
| 参考线、直线 | `online/cfyug_C2/cfyugline3D.htm`：两点、点与方向、支撑法向等构造 | 有限直线选两端点；参考轴独立构造，可供下游方向/旋转轴引用；未覆盖全部相切/角度线型 |
| 参考面 | `online/cfyug_C2/cfyugplane3D.htm`：偏移、过点平行、三点、绕轴成角或方程 | 所需点、面和轴为空时等待拾取；不预选 XY；只显示所选构造方式需要的量值 |
| 圆、圆弧 | `online/cfyug_C2/cfyugcircle3D.htm`：圆心、支撑、半径及圆弧范围 | 选择圆心和支撑面，再设置半径/角度；未覆盖三点和多种相切构造 |
| 样条 | `online/sdgug_C2/sdgugbt0104.htm`：逐点选择，点列替换/移除/插入，闭合及可选切线/曲率约束 | 空点列开始，连续选择、插前/插后、更换/移除；插值或有理控制点；未提供逐点切线/曲率约束 |
| 提取 | `online/cfyug_C2/cfyugextract.htm`：选择对象及传播方式 | 明确选定边、面或整体结果；未提供相切传播菜单 |
| 边界 | `online/cfyug_C2/cfyugboundary.htm`：曲面边界及传播/界限 | 选择曲面后提取自由边界；未提供局部起止界限 |
| 复合曲线 | `online/cfyug_C2/cfyugjoin.htm`：成员列表、连接/相切/流形检查 | 列表显示、移除及反向；只接收连通无分叉 Wire；曲面连接为独立命令 |
| 投影 | `online/cfyug_C2/cfyugprojections.htm`：对象、支撑及方向方式 | 曲线、支撑、关联方向分别拾取；无手填方向分量 |
| 交线 | `online/cfyug_C2/cfyugintersections.htm`：选择两个相交支撑 | 依次选择两个有限面/基准面；多支路须满足稳定 Naming |
| 偏移曲线 | `online/cfyug_C2/cfyugparallelcurves.htm`：曲线、支撑及距离/侧向 | 选择曲线与平面，距离可为表达式；未实现一般空间支撑曲面偏移 |
| 连接曲线 | `online/cfyug_C2/cfyugconnectcurves.htm`：连接端点/曲线及连续性 | 选择两曲线，反向控制连接端，设置切线系数；限现有 G1 Bezier |
| 分割 | `online/cfyug_C2/cfyugsplitGSD.htm`：切割对象与保留侧 | 统一曲线选择与预览；当前仍按弧长比例分割，不声称支持 CATIA 切割对象/保留侧 |
| 裁剪 | `online/cfyug_C2/cfyugtrimGSD.htm`：互裁对象与保留区域 | 当前仍保留单曲线弧长区间；通用互裁和区域选择留在后续阶段 |
| 延伸 | `online/cfyug_C2/cfyugextrapolcurves.htm`：端点、待延伸曲线及长度/终止对象 | 曲线、起端/末端/两端和长度；限直线/圆，未实现 up-to 或通用曲线外推 |
| 平移 | `online/cfyug_C2/cfyugtranslateGSD.htm`：对象、方向和距离 | 方向引用直线、轴或平面法向；上游变化后重新解析 |
| 旋转 | `online/cfyug_C2/cfyugrotateGSD.htm`：对象、轴和角度 | 所选轴的原点和方向共同驱动旋转，不保存独立原点分量 |
| 镜像 | `online/cfyug_C2/cfyugsymmetryGSD.htm`：点/线/面作为对称参考 | 当前只支持关联平面镜像，未扩展到点/线对称 |
| 拉伸曲面 | `online/cfyug_C2/cfyugextrude.htm`：轮廓、直线方向或平面法向、限值和反向 | 轮廓与方向分别选择，上游方向线可驱动重算；当前为单向长度，无双向/up-to 限值 |
| 加厚 | `online/prtug_C2/prtugbt0710.htm`：曲面、厚度与方向形成材料 | 选曲面及有符号常厚度；新实体/添加/切除/交集；未提供双侧独立厚度 |
| 曲面扫掠 | `online/sdgug_C2/sdgugbt0204.htm`：显式/隐式截面，guide/spine、限界、扭转处理和姿态 | 显式截面和 G1 路径；D 增加单条姿态导向及直线截面/直线 spine 的精确边界导向，未交付一般截面接触、多导向/法则 |
| 曲面放样 | `online/cfyug_C2/cfyugloft.htm`：截面列表、反向、闭合点、coupling、支撑连续性 | 顺序/反向/直纹；D 增加显式闭合点、内部点对应、spine/站位及姿态/有界边界导向 |
| 填充曲面 | `online/sdgug_C2/sdgugbt0206.htm`：闭合边界、逐边支撑、偏差 | 一般闭合边界为 G0；D 对四条有序多项式边增加逐边 G1/受限 G2，不自动修补支撑 |
| 裁剪曲面 | `online/cfyug_C2/cfyugtrimGSD.htm`：可选点决定保留区域，歧义时指定保留/移除，自动外推可关闭 | C 使用显式关联点与真实切割交线确定保留区域，不保存点击坐标或分区编号 |
| 延长曲面 | `online/cfyug_C2/cfyugextrapolsurfaces.htm`：边界/源面、长度/up-to、Tangent/Curvature、近似边界开关 | C 限矩形参数域的等参数自由边，按物理长度 G1 外推，不宣称任意修剪面通用外推 |
| 连接、缝合 | `online/cfyug_C2/cfyugjoin.htm`：连接/流形/相切检查、合并距离及 federation | C 区分关联面集与共享边界 Shell，检查连接和流形；不实现 federation/自动删除错误子面 |
| 偏移曲面 | `online/sdgug_C2/sdgugbt0203.htm`：有符号距离、反向、平滑及子面失败管理 | C 限常量偏移，失败整体拒绝；不静默删除错误面 |
| 封闭曲面 | `online/prtug_C2/prtugbt0709.htm`：所选曲面形成实体特征 | C 要求已缝合的闭合壳，复用原实体操作；不默认补盖 |

C 阶段上述八篇原文经 MCP 按同一 article ID 分页读至 `has_more=false`；这验证的是参考操作语义。本项目算法接纳范围由实现和测试确定，未运行商业 CAD 的实际模型对比。

用户指定的 [Extrude 原文](catia-manual://page/v1?library=6d5aed22-2df7-427c-83ae-0b4926af15ef&path=online%2Fcfyug_C2%2Fcfyugextrude.htm&sha256=870676f500d411ab224e4076e26b40092f28e0d2dd65fe017ed3ca6ab024e60f&anchor=CATCCSModelHeader.CATHybridPartExtrude&block=cc747fddb4934109067b)同时描述了上下文菜单中的分量向量与默认轮廓法向；本项目操作表单选择显式关联对象，是本轮产品取舍。手册对照没有验证 CATIA 实际模型或证明本项目具备表中尚未实现的选项。


## 形状与质量控制对照

本轮先搜索本机 `control/SdgEnglishC2.viewdoc`、`CfyEnglishC2.viewdoc`、`FssEnglishC2.viewdoc`，再通过手册工具将以下五篇以同一 article ID 分页读至 `has_more=false`。这些是参考行为；没有运行 CATIA 实际零件进行数值对比。

| CATIA 原文 | 已核对的参考行为 |
|---|---|
| [Creating Swept Surfaces](catia-manual://page/v1?library=6d5aed22-2df7-427c-83ae-0b4926af15ef&path=online%2Fsdgug_C2%2Fsdgugbt0204.htm&sha256=e5e7e26e1c649f0c02a8942b314c7e269ec413408881f963181fa64db56c756c) | 多截面可选用户 spine、闭合点和手工 coupling；手册对 spine 法向与导向交点有明确要求。 |
| [Creating Multi-sections Surfaces](catia-manual://page/v1?library=6d5aed22-2df7-427c-83ae-0b4926af15ef&path=online%2Fcfyug_C2%2Fcfyugloft.htm&sha256=5585b1788e493fafa38755a52e8a241633061f7c133f8af43e62e08a09361d61) | Spine 定义截面法平面，导向须与截面相交；商业功能另有边界接触、隐式截面及多导向。 |
| [Creating Blended Surfaces](catia-manual://page/v1?library=6d5aed22-2df7-427c-83ae-0b4926af15ef&path=online%2Fcfyug_C2%2Fcfyugblendedsurfaces.htm&sha256=94a1721a6a3c822035f79c333b47c47af8b266c4642700b4155712ba46907779) | 两条边界与各自支撑，可选位置/相切/曲率和张力；对应、spine 等商业模式独立存在。 |
| [Creating Fill Surfaces](catia-manual://page/v1?library=6d5aed22-2df7-427c-83ae-0b4926af15ef&path=online%2Fsdgug_C2%2Fsdgugbt0206.htm&sha256=dabd633402e1bb0f09893503123ddc5f83af2b79a2079579d7a15e00b6e3986b) | 逐边选择支撑与位置/相切/曲率；支撑需包含完整边界，另有内部约束及偏差控制。 |
| [Performing a Surface Curvature Analysis](catia-manual://page/v1?library=6d5aed22-2df7-427c-83ae-0b4926af15ef&path=online%2Fcfyugfss_C2%2Fcfyugfssut0301.htm&sha256=60e27c6639c9740fa9d1d3a3e5041c0fbf8085e63dfe1681eb8e39cfb7d055ea) | 主曲率、均值和高斯曲率有不同单位；显示映射受采样/离散精度影响，不能代表连续极值。 |

本项目的工程取舍：空输入、显式拾取及稳定点对应；单边 spine、辅助姿态导向及直线截面的精确边界导向；非有理多项式支撑的完整导数构造；有界曲率采样。Boundary 为本项目四边 Hermite-Coons 曲面命令，区别于已有“边界”自由边提取。CATIA 的通用支撑/边界接触能力不作为这些命令已实现的证明，具体范围以 [current](../architecture/current/wireframe-surfaces.md#形状控制与连续性接纳) 为准。

OCCT 7.9.1 本机源码另确认：`BRepFill_PipeShell::Add` 的 `ContactOnBorder` 自动法则会清空已有截面，且以轮廓造平面面；直线开放轮廓会触及空面。当前姿态模式使用 `BRepFill_NoContact`，贴合边界模式另由本项目受限 B-spline 直纹构造实现，不启用该 OCCT 自动法则分支，也不把姿态模式宣称为贴合边界。G1/G2 使用本项目受限多项式导数构造，而非仅依赖 Filling 的采样误差或选项标签。

## 其他商业 CAD

| 软件 | 已核对的公开行为 | 本项目可采用的设计要点 |
|---|---|---|
| SOLIDWORKS | Boundary Surface 具有两个方向的输入与相切/曲率条件；列表、曲线标记及连接对应联动。[Boundary Surface](https://help.solidworks.com/2025/english/solidworks/sldworks/c_Boundary_Surface_Concept_surfaces.htm)、[PropertyManager](https://help.solidworks.com/2024/English/SolidWorks/sldworks/r_boundary_pm.htm) | 区分沿截面放样和两个方向的边界曲面；逐边支撑与对应控制直接呈现 |
| Creo Parametric | Quilt 表达非实体曲面结果；Boundary Blend 使用一/二方向边界和约束；Thicken / Solidify 将曲面用于材料操作。[Quilts](https://support.ptc.com/help/creo/creo_pma/r12/usascii/surfacing/surface/About_Quilts.html)、[边界条件](https://support.ptc.com/help/creo/creo_pma/r12/usascii/surfacing/surface/Setting_Boundary_Conditions.html)、[Thicken](https://support.ptc.com/help/creo/creo_pma/r12/usascii/part_modeling/part_modeling/About_the_Thicken_Feature.html) | 曲面结果需有独立类型、可见性和下游用途；实体化须是明确操作 |
| Onshape | Composite Curve 可产生多条独立曲线；Boundary Surface 使用 U/V 边界，Fill 支持多段闭合边界和逐边条件，且区分精确导向与采样导向。[Composite Curve](https://cad.onshape.com/help/Content/PartStudio/composite_curve.htm)、[Boundary Surface](https://cad.onshape.com/help/Content/PartStudio/boundary_surface.htm)、[Fill](https://cad.onshape.com/help/Content/PartStudio/fill.htm) | 支持稳定多结果和链选择；记录实际精度/近似方式，不能将采样拟合标为精确满足 |
| NX | Siemens 的 June 2025 高级能力说明提到 Through Curve Mesh / Studio Surface 的边界拆分及多面输出。[官方说明](https://blogs.sw.siemens.com/designcenter/whats-new-nx-advanced-capabilities/) | 一个构造特征可以产生多面结果；不要强制“一个曲面特征等于一个 Face”。本次未审计 NX 全部选项 |
| Rhino | Trim 保留底层曲面及裁剪边界；MatchSrf 提供位置、相切、曲率匹配，Surface Tools 提供偏移、混接、延长和收缩。[Trim](https://docs.mcneel.com/rhino/8/help/en-us/commands/trim.htm)、[MatchSrf](https://docs.mcneel.com/rhino/8mac/help/en-us/commands/matchsrf.htm) | 底层参数曲面与裁剪 Face 要分开；自由编辑的便利性值得参考，历史/引用规则仍按本项目参数化合同设计 |

这些来源覆盖可观察功能、对象模型及操作，并不构成相同输入下的质量、速度或鲁棒性横向测试；不同版本/许可可能提供不同范围。

## 算法候选的证据边界

本地构建依赖的 `Standard_Version.hxx` 为 OCCT 7.9.1。本轮核对了该版本头文件及官方源码：[BRepFill_Filling](https://github.com/Open-Cascade-SAS/OCCT/blob/V7_9_1/src/BRepFill/BRepFill_Filling.hxx)、[BRepOffsetAPI_MakePipeShell](https://github.com/Open-Cascade-SAS/OCCT/blob/V7_9_1/src/BRepOffsetAPI/BRepOffsetAPI_MakePipeShell.hxx)。前者声明带支撑的 G1/G2 边界约束及误差查询，后者区分扫掠壳构造与 MakeSolid；本地 Sewing 头文件提供自由边、多重边及修改历史查询。

这只证明候选 API 存在。算法接纳范围、完整 Naming、误差上界、取消响应及工程模型成功率仍需后续实现与检验。OCCT 当前在线文档会重定向到较新版本，不能把其页面版本当成本项目运行版本。Onshape 的公开 [FeatureScript 几何操作文档](https://cad.onshape.com/FsDoc/library.html)也仅用于理解 wire/sheet 输出及边界输入合同，不用于推断其专有内核实现。
