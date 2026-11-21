# Web 工作台

React 工作台以文档和编辑会话组织交互。TanStack Query 管理服务端数据，Zustand 管理临时 UI 状态，Three.js 负责显示与拾取。模型修改经版本化命令进入服务端。正式 API 与 Mock API 在 Vite 构建入口选择。[Web 模块](../../../web/apps/cad/README.md)

## 命令、预览与提交

工具带、搜索和上下文菜单由同一命令目录投影。工作台根据文档、目标对象、当前选区和编辑状态启用命令。启动时将兼容的已有选区带入相应角色，用户可以继续从视区或树选择、更换和清除输入。候选归发起它的会话、Revision 和输入代次所有。确认采用匹配候选，取消和过期响应不能推进 Head。[文档与命令框架](document-command-framework.md) · [表单与拾取](../../guides/workbench.md#命令表单与拾取)

## 场景、编辑与选择

- 根 Product 或 Part 提供显示场景。编辑目标包含所属文档及 occurrence 路径。草图、特征和装配工具各自管理输入，退出后恢复浏览上下文。[编辑会话](product-edit-context.md)
- 树与视区投影同一语义选择。局部网格编号只定位当前快照的图元，跨版本引用须使用 PersistentSelection。多实例共享几何，位姿与实例高亮由显示层管理。[语义树](semantic-tree.md) · [显示与过滤](model-display.md)
- 视口按几何键加载 GLB，按对象和场景状态更新外观、选择和预览。参考几何、自然边线与曲面材质共享相机和深度约定。[几何制品](geometry-representations.md) · [自然边线](natural-edges.md) · [对象属性与全局选项](object-scene-properties.md)
- 导航、缩放、投影切换和草图平面相机沿用同一控制器。拖动装配使用独立交互 Session，机构使用已接受的构型与冻结回放。[相机导航](../../../web/apps/cad/src/cad/navigation/README.md) · [装配操纵](product-assembly.md#连续操纵) · [机构研究](kinematics-dmu.md)

## 文档浏览与创建

文档中心管理文件夹、分页、筛选、预览和显式多选。浏览器前进/后退恢复对应页面，工作台内的编辑状态由会话持有。缩略图的读取受文档权限和真实显示依赖约束。对象属性用于查看来源与值，修改由正式对象命令执行。[文档中心与截图](../../guides/model-showcase.md)

## 验证与限制

运行 `mise run check --scope web` 检查命令、工具状态和数据投影。树与视区联动、键鼠导航、遮挡、取消和预览确认需要实际浏览器。树行虚拟化仍需完整节点索引。大场景应分别测量选择索引、React 更新、制品解码与 GPU 绘制。[显示验证](../../validation/README.md#工作台与显示) · [图标指南](../../guides/cad-icons.md)
