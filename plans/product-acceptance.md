# Product 关联设计验收收口

> 状态：实现与自动化测试资产已存在，真实浏览器/WebGL 纵向验收待完成。此页接收旧 P9G、P10H 和 P10 UX 计划的剩余工作；不重复已实现方案。

## ACCEPT-PRODUCT：单条真实用户路径

在重启后的真实后端与浏览器上建立 ToyCar 产品：Skeleton、Chassis、Axle 与四个 Wheel occurrence。依次验证：

1. 根及嵌套 Product 内新建 Part、插入现有 Reference、激活 occurrence；根装配保持可见，草图命令/约束/参数作用于正确 Part。
2. 创建和选择 Datum、Parameter、拓扑 Publication；视口、所属树节点与 Publication 同步高亮；Context Catalog 只显示当前 Product 内可读、兼容、无环的来源。
3. 创建 ContextInput/Binding，四轮同 base Revision 与相同规范输入共享 Variant；修改其中不同输入不污染共享 Part；任一成员 Undo/Redo 不留下半个绑定或半条祖先 Revision 链。
4. 修改 Skeleton；FOLLOW_HEAD 从叶到根提交新 Revision；Pin 截断传播，恢复 Follow 可更新；来源 Broken/不兼容、stale digest、权限变化及依赖环均有明确诊断，不部分接受一个计划。
5. 创建装配约束、修改和删除来源、Reconnect；Connected/NotConnected 与四种求值状态保持独立。手柄取消、迟到响应、pointerup 最终帧与选择保留符合当前交互合同。
6. 创建 Product Release；移动当前 Head 后 replay 旧 Release，按冻结 GeometryKey 导出，确认 occurrence 平移与旋转保持。产品版本中心与 Exchange 工作流不混用。
7. 偏好重载、文档显示单位覆盖、标题栏标签、结构树调宽、Toolbar 布局和遮挡下基准选择。以当前实现为准，不恢复旧计划已被替换的圆环树图标或 24 px 调宽 grip。

## 证据与退出条件

- 记录源码版本、浏览器/后端模式、实际操作和结果；失败保留截图/trace、稳定错误、manifest/request digest 或 `.3dreplay`。
- Mock Playwright、Node 场景和 production build 各自保留结果；它们不替代真实后端/WebGL 路径。
- 对发现的缺陷先修复并跑最小相关回归，再重跑失败路径；无法运行则保留“待验收”，不按文档标记通过。
- 全路径通过后，把验证日期、证据位置及仍存在的能力边界归入[当前 Product 架构](../docs/architecture/current/product-assembly.md)，从[统一路线](README.md)移除此待办并删除本页。
