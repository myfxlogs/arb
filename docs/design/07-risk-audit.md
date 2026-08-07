# 07 · 风控 + 审计 + 归因

> 风控防「执行失败变方向敞口」+「对手方爆雷」；归因记账是「准确无误」的闭环与 P1 自适应的数据源。
> 依据讨论五（P1 参数）+ 现有 `risk` 包 + `04`（归因）+ constraints。

---

## 1. 风控（套利的真实风险）

套利理论上锁定利润，所以风控**不是防"单笔亏"，是防**：
- **执行失败 → 单边敞口**（一腿成交另一腿没成交 = 变方向性赌注）。
- **对手方风险**（broker 跑路，资金分散的代价）。
- **数据断流 → 假信号**。

### P1 参数（讨论五，初值，自适应）
| 参数 | 初值 | 防的 | 落地组件 |
|---|---|---|---|
| 单机会最大敞口 | ≤ 5% 总资金 | 单机会执行失败敞口 | `CapitalGate` |
| 最大并发未平仓 | ≤ 5 | 风险叠加 + 管道压力 | `CapitalGate` |
| 单平台资金占比 | ≤ 40% | 对手方风险 | `CapitalGate`（新增） |
| 单边敞口存活 | ≤ 3s 自动对冲/平仓 | 一腿失败变赌注 | `pipeline` hedge（04） |
| 日亏损熔断 | 日亏 ≤ 3% 暂停 | 数据/逻辑异常 | `CircuitBreaker` |
| 行情断流 | blind mode 撤单+拒新仓 | 假信号 | `adapter` reconnect + blind |
| revalidate 价偏阈值 MaxSlippage | 见 04 §3 | 执行前价偏放弃 | `pipeline` |

---

## 2. 现有 `risk` 包复用（code-map §2 Layer 3）

| 组件 | 现状 | 新架构用途 |
|---|---|---|
| `CapitalGate` | 资金门禁 | 单机会敞口 / 并发 / **新增单平台占比** |
| `CircuitBreaker` | 连亏/日亏熔断 | 日亏 ≤ 3% / 连续失败 → 暂停（须人工恢复） |
| `KillSwitch` | atomic.Bool 实例，文件存在触发紧急停止 | 你的一键全局撤单平仓（04 §4） |
| `AdaptiveRateLimiter` | 按成败反馈限流 | 执行失败密集时自动降速 |

> 这些组件已实现（Phase 5），新架构**复用 + 参数化**对接 P1，不重写。

---

## 3. 审计（`internal/audit/`，待建）

- code-map §2 Layer 3 列了 `audit/` 但**当前未实现** → Windsurf 新建。
- 职责：每个 Opportunity（从 Detected 到 Filled/Failed）+ 每笔订单，落审计日志（protobuf 格式文件，constraints §二允许的本地文件）。
- 用途：合规追溯 + 归因数据源 + bug 复盘。
- 接口：`audit.Log(Event)`，同步写（不加 goroutine，code-map §4）。

---

## 4. 归因记账（"准确无误"的闭环）

**这是系统持续"准确"的发动机**：预估 vs 实际的偏差，反过来校准 Evaluator。

```
Opportunity(预估: NetProfit/Swap/Commission/滑点)
   │ 确认 → 执行
   ▼
成交结果(实际: 成交价/Order.Swap/Order.Commission/实际滑点)
   │ 归因对比
   ▼
偏差 → 校准 Evaluator 的:
   - 滑点预估(实测分布 P95)  →  机会阈值(讨论五①)
   - swap/commission 预估     →  成本模型(02 §4)
```

- **PG 表 `opportunities`**：存每个机会的预估字段 + 成交后回填实际字段 + 偏差。
- **从第一天起内建**（讨论五原则）——没有归因，P1 参数永远靠拍脑袋，"准确无误"落不了地。

---

## 5. 实现指引（Windsurf）

| 组件 | 动作 |
|---|---|
| `risk/CapitalGate` | 扩展：加"单平台资金占比 ≤ 40%"检查（查各 broker Account 余额）。 |
| `risk/CircuitBreaker` | 对接"日亏 ≤ 3%"；熔断后须人工 Resume（保留）。 |
| `internal/audit/` | 新建：Opportunity/Order 事件 → protobuf 审计日志。 |
| `store` | 新增 `opportunities` 表（预估+实际+偏差字段）+ 读写。 |
| 归因 | pipeline 成交后，回填实际值到 `opportunities`，计算偏差，喂给 Evaluator 的滑点/成本校准。 |

---

## 6. 回溯
- P1 参数 → 讨论五
- 熔断须人工恢复 → `evaluation-framework.md:942`
- 归因 = P1 自适应数据源 → 讨论五贯穿原则
- 审计日志 protobuf 文件 → constraints §二
