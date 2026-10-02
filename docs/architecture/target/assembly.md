# 工程连接与装配扩展设计

> 本页只保留未实现扩展边界，不重复当前六类、组、Session 或诊断合同。当前实现见[Product](../current/product-assembly.md)、[六类合同](../current/assembly-constraints.md)与[算法](../../../kernel/assembly/SOLVER_ALGORITHMS.md)。

## 工程连接（暂缓）

Engineering Connection 是稳定的用户关系实体，不是 solver residual 或瞬时 DOF 类型。若实施，应组合明确的支持/frame、若干原子约束、branch、适用参数/模式和生命周期，原子编辑整个关系；内部多条方程保留连接/约束来源，诊断能回到可修复对象。

Connector/frame 必须来自 typed stable occurrence/Publication/PersistentSelection 与可重放来源，不能用 mesh 猜测、显示名称或临时 local ID 作身份。关节类型与方向/frame 明确；limits、gear/rack、driver/law 若引入应独立定义参数域、单位、时间/运动状态与失败行为，不能用静态角 winding 代替累计运动。不得因已有 Revolute/Prismatic 有限运动测试宣称这些持久实体已交付。

## 柔性与运动学边界

当前 rigid 子 Product 是 owning Product 中的运动单元。flexible expansion 若实施，必须定义可编辑 scope、外部/内部自由度、组/闭环/接地、occurrence 版本和原子提交边界；不通过拖动暗中展开或把多 CAD Body 拆成独立刚体。

运动学/动力学需要独立时间状态、driver/limits 与能量/碰撞/摩擦语义。当前 Contact 是解析无限支撑关系，不是 mesh collision、动力学响应或任意 NURBS 接触。DMU/clearance 不得回写参数模型，修复仍经正式命令。

## 进入条件

真实产品用例、稳定连接/frame 合同、参数/支持/生命周期/历史/发布与独立几何 corpus 先明确，再决定实施。当前明确暂缓，没有虚构恢复日期；优先级只见[后续工作](../../../plans/README.md)。共享 Revision、CAS、命令幂等、失败候选隔离与冻结 replay 保持，不创建并行历史体系。
