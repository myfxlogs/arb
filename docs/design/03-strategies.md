# 03 · 策略（Detector）

> 定义三类 Detector 的**发现逻辑**。Detector 只「发现候选机会」（仅价差），**不算净盈利**——那是 Evaluator 的职责（02）。
> 依据 `02`（Listing/Opportunity）+ `04`（流程）+ 真实探测数据。范围 = A+B 确定性套利（00 §4）。

---

## 1. Detector 的职责边界

```
quotes(QuoteBus) + Listing(缓存) ──► [Detector] ──► 候选机会(Candidate，仅毛价差)
                                            │
                                            ▼
                                   [Evaluator]（02 §6）算净盈利、判可执行
```

- **Detector = 发现**（价差存在吗？）。
- **Evaluator = 评估**（扣成本后还赚吗？可执行吗？）。
- 两者分离：Detector 可以高频轻量扫描，重计算留给 Evaluator 只对候选做。

---

## 2. 三类 Detector

### 2.1 CrossExchange（跨所价差）— ★最稳，第一优先

**逻辑**：同一逻辑品种（经符号映射归一）在两个 broker 间，A 的 Ask < B 的 Bid（买入价低于另一边的卖出价），价差为正。

```
例：EURUSD
  ICMarkets Ask=1.1000   Exness(EURUSDm) Bid=1.1004
  → 候选：ICMarkets 买入 + Exness 卖出，毛价差 0.4 pip
```
- **腿**：2 条（A 买、B 卖，等量对冲）。
- **方向风险**：无（两边对冲，不赌方向）。
- **真实可行性**：ICMarkets + Exness 同品种（EURUSD/EURUSDm）跨所价差实测可得；contractSize 跨 broker 一致（02）→ 同手数对冲可行。
- **执行风险**：跨 broker 同时成交不可能 100%（04 §6 兜底）。

### 2.2 Carry（套息）— swap 差，需扩大覆盖验证

**逻辑**：寻找 swap 结构使对冲后**净 swap 为正**的机会。

**正收益条件（对冲套息）**：`swapLong(做多侧 broker) + swapShort(做空侧 broker) > 0`（等量对冲锁掉汇率敞口后，纯赚净 swap）。

| 模式 | 做法 | 风险 | 取舍 |
|---|---|---|---|
| 裸 carry（方向性） | 单边持仓赚 swap（如 GBPJPY 做多 ICMarkets 收 +11.67/天） | 持仓期汇率波动 | ❌ 不符合"准确无误"，不做 |
| **对冲套息** | 两 broker 等量对冲锁汇率，赚净 swap | swap 政策可变 | 机制保留，Evaluator 过滤 |

**实测发现（诚实记录，审计纠正）**：ICMarkets + Exness 的 EURUSD/GBPJPY 实测，**所有对冲组合净 swap 为负**：
- GBPJPY：ICMarkets 做多(+11.67) + Exness 做空(−39.8) = **−28.13/天**；反向 −23.19/天。
- EURUSD：同理为负。
原因：broker 的 swap 定价使多空持仓整体付 swap（broker 赚 swap 价差），同品种跨 broker 对冲后净 swap 通常为负。

**结论**：
- Carry Detector **仍建**（扫描 broker×品种 swap，Evaluator 算净 swap，**正收益才推机会**）——当前数据无正收益 = 暂无可执行 Carry 机会，**这恰是系统不推假机会的正确表现**。
- 真实 Carry 机会来源：某 broker 对特定品种 swap 倒挂（罕见）、或更多 broker/品种覆盖。需扩大探测验证。
- 因实测机会稀少，Carry 实现优先级降至 Triangular 之后（§5）。
- **腿/周期**（机会出现时）：2 腿等量对冲、长期持仓（天~周）。

### 2.3 Triangular（三角）— 同 broker 三品种，执行最难

**逻辑**：同一 broker 内三个货币对的交叉汇率偏差。如 EURUSD、GBPUSD、EURGBP：`EURUSD` vs `GBPUSD × EURGBP` 不一致。

- **腿**：3 条（同 broker）。
- **方向风险**：无（闭环对冲）。
- **执行风险**：**三腿同时成交比两腿更难**（04 §6 all-or-nothing 失败概率更高）→ 实现优先级最低。
- **跨 broker 风险**：无（同 broker，报价同步）。

---

## 3. 确定性分级（呼应 00 §4 "准确无误"）

| Detector | 确定性 | 风险来源 | 第一版 |
|---|---|---|---|
| CrossExchange | 高（对冲锁价差） | 执行（同时成交） | ✅ 先做 |
| Carry（对冲） | 中高（对冲锁汇率，赚 swap 差） | swap 政策变 + 持仓期残余 | ✅ 次做 |
| Triangular | 高（闭环对冲） | 执行（三腿成交更难） | ✅ 后做 |

三者都属 A+B（确定性），都通过 Evaluator 的同一套成本模型把关。统计/期现/宏观（C/D/E）不在 Detector 范围（00 §4 排除）。

---

## 4. Detector 接口（Windsurf 实现）

```go
// Detector 发现候选机会（仅毛价差，未评估）。
type Detector interface {
    Type() OppType  // CrossExchange / Carry / Triangular
    // Scan 在最新报价 + Listing 上扫描，返回候选。
    Scan(quotes map[quoteKey]bus.Quote, listings map[(broker,canonical)]*Listing) []Candidate
}

type Candidate struct {
    Type        OppType
    Legs        []Leg        // broker/canonical/direction/lots/估价
    GrossProfit decimal.Decimal  // 毛价差（本币，未扣成本、未换算USD）
    QuoteTime   time.Time
}
```

- Detector **纯函数**（无副作用）：输入 quotes+listings，输出候选。易测、可并发。
- 候选交给 Evaluator（02 §6）算净盈利。

---

## 5. 实现顺序（FX MT5，呼应 D-004）

1. **CrossExchange**（跨所价差）：最稳、broker 充足、闭环最短 → 第一优先，验证整套链路。
2. **Carry**（对冲套息）：swap 数据真实可得（02），价值高 → 次之。
3. **Triangular**（三角）：执行风险最高 → 最后，待执行管线稳定。

> 每类 Detector 落地后，必须先用 demo 跑通"发现→评估→推送"（不必真成交），确认候选质量，再开确认→执行。

---

## 6. 回溯
- 候选→Evaluator→Opportunity 流程 → 02 §6、04 §3
- 确定性分级（A+B） → 00 §4
- swap 差真实数据 → discussion-log 讨论七
- 执行 all-or-nothing + 对冲 → 04 §6、现有 pipeline.go
- 对冲套息的长期持仓 → CLAUDE.md 套利类型（套息天~周）
