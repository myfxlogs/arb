# ARB 设计文档（从头设计版）

> 本目录是 ARB 系统**重新设计后的权威规范**。源自 2026-08-07 的第一性审视（见 `../handoff/decisions.md` D-003/D-004 与 `discussion-log.md`）。

## 与旧文档的关系
- **本目录是权威**。与 `docs/evaluation-framework.md` / `docs/implementation.md` 冲突处，**以本目录为准**。
- 旧文档保留为**历史设计快照**，不删（保留推演痕迹），但不再指导实现。
- 技术硬约束（`docs/constraints.md`：Go/gRPC/PG/精度/并发/Push-First）**仍然全部有效**，是本设计的给定边界。

## 北极星
**给"下单者"（用户）提供准确无误的盈利机会。** 一切设计服从这一句。详见 `00-north-star.md`。

## 第一版范围（一句话）
**FX（MT5，IC Markets + Exness 起步）的确定性套利**：跨所价差 / 三角 / 套息。发现→评估→你确认→系统执行。Crypto（Binance）抽象兼容、实现后置（D-004）。

## 文档索引

| 文档 | 内容 | 状态 |
|------|------|------|
| `00-north-star.md` | 北极星 + 第一性推导 + 四公理 + 范围与非目标 | ✅ |
| `01-architecture.md` | 总览：分层 / 数据流 / goroutine 拓扑 / 包依赖 / 与旧架构差异 | ✅ |
| `02-opportunity.md` | 品种模型 + 成本模型 + Opportunity（准确无误的核心） | ✅ |
| `03-strategies.md` | 三类 Detector：跨所价差 / 三角 / 套息 + 确定性分级 | ✅ |
| `04-human-in-loop.md` | 人机闭环：发现→评估→推送→确认→执行→归因 | ✅ |
| `05-data-sources.md` | 动态报价 vs 静态品种参数 / Push-First 分离 / 符号映射 / 时间同步 | ✅ |
| `06-interfaces.md` | gRPC 接口：OpportunityStream / ConfirmOpportunity + 现有 RPC + 完整 proto | ✅ |
| `07-risk-audit.md` | 风控(P1) + 审计 + 归因记账（自适应闭环） | ✅ |
| `08-roadmap.md` | 落地路线：FX 先行的 Phase 顺序 + 给 Windsurf 的施工与验收 | ✅ |
| `09-core-runtime.md` | core 内部运行时：并发模型 / 延迟预算 / 容灾恢复 / 持久化 / 扩展 / 可用性 | ✅ |
| `10-desk-ui.md` | desk UI：arb-cockpit WPF 详细设计（MVVM / 视图 / ViewModel / gRPC / 线程模型） | ✅ |
| `11-testing.md` | 测试策略：单元 / 集成 / 回归基线（02 §7 真实字段）+ `go test -race` 强制 | ✅ |

## 关键决策（见 `../handoff/decisions.md`）
- **D-003**：混合模式（发现+评估+人确认+执行），非全自动；策略聚焦 A+B 确定性套利。
- **D-004**：先 FX MT5，Crypto 留接口；跨所价差优先；broker 重质不重量。

## 真实数据依据
`02/03/05` 的字段与公式基于真实 MT5 探测（ICMarkets + Exness demo，见 `discussion-log.md` 讨论七）——非纸上推断。
