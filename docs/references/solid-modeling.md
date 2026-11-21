# Part 实体建模能力对照

审计基线为 14aa68e2。当前行为由[实体能力表](../architecture/current/solid-features.md)、[生成器](../architecture/current/solid-generators.md)和[特征复制](../architecture/current/feature-replication.md)维护。[CAD 主线](../../plans/cad-core-roadmap.md)管理后续工作。本页记录审计发现与商业软件的操作语义，范围为通用 Part 材料历史，含导入与曲面实体化。

| 用户的建模步骤 | 基线能表达的设计意图 | 未覆盖的设计意图 |
|---|---|---|
| 拉伸、切除 | 整草图／区域、有限／双侧／对称、关联终止、起止偏移、薄壁 | 两侧独立关联终止、不同区域分别定高、斜向和自带拔模、薄壁自动贴壁 |
| 旋转、旋转切除 | 区域、轴、角度与反向 | 薄壁旋转、独立双端及关联角终止 |
| 扫掠 | 可含内环的截面、单条草图或三维路径、两种标架 | 多导向、变截面、沿程扭转／缩放、一般空间折角 |
| 放样 | 多截面、点端、对应、接缝、受限端部 G1/G2 | 非点截面须单区域无孔；通用导向线与自由曲面连续性 |
| 工程孔 | 显式草图点、同规格孔组、沉孔／沉头、钻尖、螺纹标注 | 端点与圆心定位、规格表驱动、跨 Body 配合孔、建模螺纹 |
| 圆角、倒角 | 恒半径、等距、正式边集 | 变半径、局部／全圆角、双距离与距离角度倒角 |
| 拔模、抽壳 | 中性平面拔模、开口面集与恒定壁厚 | 分型元素及变角拔模、封闭空腔、局部壁厚；复杂内抽壳有局部材料超出原实体的已记录案例 |
| 加厚、封闭 | 有界 Sheet 恒厚实体化、已缝合闭壳成实体 | 双侧独立厚度、由交叉面组选择围合体积、开放面直接改写材料 |
| Boolean、多 Body | 明确目标与工具阶段、并差交、单工具显式保留体积 | 开放平面／曲面分割实体、分割材料表面、分割后独立加工、多工具显式选区 |
| 直接编辑、导入 | 移动单个平面、改完整圆柱孔段直径、冻结导入实体上继续加工 | 面组变换、删面修补、换面、加工特征识别 |
| 阵列、镜像 | 同 Body 多种子线性／圆周／镜像、成员身份；生成工具、净材料、成员重执行、整体阶段 | 二维／路径／点表分布、逐成员变尺寸、面组复制、通用关联特征组重执行、独立 Body 输出 |
| 工程结构与派生 | 薄壁拉伸、扫掠、独立曲面变换 | 加强筋贴壁、实体变换／缩放、螺旋轨迹、包覆凸字或刻槽、可重新放置的参数特征模板 |

镜像已允许拉伸、旋转、扫掠、放样与孔逐成员重执行。有限种子及盲孔可与连续的圆角／倒角／拔模一起提取净材料范围。抽壳、布尔、加厚、封闭和直接编辑只能显式复制整个成功的上游 Body 阶段。工序换序、历史插入、装配上下文、作用于多个目标以及导入实体的修复能力要分别检查。创建命令、编辑和精确重建均应引用合法前缀。

## 商业对照

- CATIA V5 B33 Part Design 的 Stiffener 以轮廓、方向和厚度表达加强筋；Rib 表示沿中心曲线扫掠。Multi-Pad 支持同草图多区分别定高。来源为 PrtEnglishC2.viewdoc 中的 prtug_C2/prtugbt0506.htm、prtugbt0507.htm、prtugbt0524.htm。
- CATIA 的 Split 可用平面／面／曲面切 Body，Replace Face 可按目标面调整已有实体边界。来源为同一索引下的 prtugbt0706.htm、prtugbt0715.htm。
- Onshape 区分 Part、Feature、Face 镜像及成员重新求值；对工程孔可定义跨件配合。[Mirror](https://cad.onshape.com/help/Content/PartStudio/mirror.htm) · [Hole](https://cad.onshape.com/help/Content/PartStudio/hole.htm) · [Split](https://cad.onshape.com/help/Content/PartStudio/split.htm) · [Chamfer](https://cad.onshape.com/help/Content/PartStudio/chamfer.htm)
- Creo 的轨迹加强筋按目标 Body 贴壁，并可指定拔模及根部圆角；变截面扫掠通过其他轨迹约束截面。[Trajectory Rib](https://support.ptc.com/help/creo/creo_pma/r12/usascii/part_modeling/part_modeling/To_Create_a_Trajectory_Rib.html) · [Variable Section Sweep](https://support.ptc.com/help/creo/creo_pma/r12/usascii/part_modeling/part_modeling/Workflow_for_Variable_Section_Sweeps.html)

## 两批实施与验收

| 工序 | 实现决定 | 接纳条件 |
|---|---|---|
| 既有结果 | 精确上游与内抽壳候选作材料差检查，保留开口、壁厚、精度及拓扑历史的独立检查。复制编辑器直接显示服务端推导的种子类型，整体阶段始终由用户显式选择 | 已知坏壳不能发布；对成形壳核验开口外侧截面。种子改参后，工具、净材料、成员重执行、阶段复制各自保留原语义 |
| 加强筋 | 保存中心线稳定引用、厚度及方向；在目标材料中裁切薄筋棱柱，取接到截面且被目标截住的唯一空腔。多个有效区要求重新定义 | 直筋实测体积；中心线跨过两侧斜壁时，两端和筋根贴合实际内壁与槽底 |
| 工程孔 | 规格键与径值由服务端统一解释，定位保存草图实体、子元素及端点身份。首批覆盖 M3、M4、M5、M6、M8、M10 中等间隙，分别为 3.4、4.5、5.5、6.6、9、11 mm | 改规格后重算所有孔，保留孔位身份与可被下游公式引用的直径参数。尺寸对照见 [Metric clearance hole chart](https://www.noramark.com/resources/charts/clearance-hole-chart/) |
| 非对称倒角 | 一组稳定边加一个共同邻接面，定义第一距离方向；第二量为长度或角度 | 换参照侧后几何相应改变；非邻面和非法尺寸不得提交 |
| 分割面 | 复用局部修改及上游输入编辑，求选中面与平面的精确交线；分区由源面和有向平面两侧识别 | 实体数量与体积守恒，两侧独立选择，平面错开时明确失败 |
| 分割实体 | 平面必须穿过材料，按有向两侧分别产生可追溯结果。定义显式保存原 Body 保留的正侧或负侧。另一侧由同一命令分配独立 Body ID，两侧在同一 Revision 生效，分别接受后续加工 | 按减速箱流程核验双预览、下游孔、切割外移、抑制及历史恢复，跟踪两侧 Body ID 和各自精确体积 |
